package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/flowable-go/flowable/internal/migration"
)

// TestRunConvertCreatesPrivateState 验证离线转换只发布私有状态文件且标准输出仅含报告。
// @param t 测试上下文
// @return 无
func TestRunConvertCreatesPrivateState(t *testing.T) {
	// 状态包含业务值，只有输出文件可持有该内容，报告必须保持独立。
	path := filepath.Join(t.TempDir(), "state.json")
	var stdout, stderr bytes.Buffer
	deps := commandDependencies{
		scan: func(ctx context.Context, envPath, dialect string) (migration.SourceSnapshot, error) {
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("缺少转换超时")
			}
			if dialect != "postgres" || envPath != "配置.env" {
				t.Fatal("源库参数丢失")
			}
			return migration.SourceSnapshot{Dialect: dialect}, nil
		},
		convert: func(ctx context.Context, snapshot migration.SourceSnapshot, options migration.Options) ([]byte, migration.Report, error) {
			if options.TenantID == nil || *options.TenantID != "租户甲" {
				t.Fatal("转换租户范围丢失")
			}
			return []byte(`{"variable":"秘密业务值"}`), migration.Report{Dialect: snapshot.Dialect}, nil
		},
	}
	code := runWithDependencies([]string{"-convert", "-out", path, "-dialect", "postgres", "-env", "配置.env", "-tenant", "租户甲", "-timeout", "1m"}, &stdout, &stderr, deps)
	if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), `"dialect": "postgres"`) || strings.Contains(stdout.String(), "秘密业务值") {
		t.Fatalf("退出码=%d 输出=%s 错误=%s", code, stdout.String(), stderr.String())
	}
	var report migration.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal("标准输出混入非报告内容")
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != `{"variable":"秘密业务值"}` {
		t.Fatal("状态文件内容错误")
	}
	info, err := os.Stat(path)
	// Windows 不表达 Unix 权限位，仅要求文件创建成功。
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0600) {
		t.Fatal("状态文件权限错误")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatal("状态临时文件未清理")
	}
}

// TestRunConvertRejectsInvalidArguments 验证转换模式互斥及输出位置在扫描之前拒绝。
// @param t 测试上下文
// @return 无
func TestRunConvertRejectsInvalidArguments(t *testing.T) {
	directory := t.TempDir()
	existing := filepath.Join(directory, "已有状态.json")
	if err := os.WriteFile(existing, []byte("原状态"), 0600); err != nil {
		t.Fatal(err)
	}
	dangling := filepath.Join(directory, "悬空链接.json")
	if err := os.Symlink(filepath.Join(directory, "缺失状态.json"), dangling); err != nil {
		t.Fatal(err)
	}
	// 任何冲突与目标存在情况均不得触发源库连接。
	cases := [][]string{
		{"-convert"}, {"-convert", "-out", ".env"}, {"-convert", "-env", "配置.env", "-out", "配置.env"},
		{"-convert", "-out", existing}, {"-convert", "-out", dangling},
		{"-convert", "-out", filepath.Join(directory, "新状态.json"), "-preflight"},
		{"-convert", "-out", filepath.Join(directory, "新状态.json"), "-apply"},
		{"-convert", "-out", filepath.Join(directory, "新状态.json"), "-tables"},
		{"-convert", "-out", filepath.Join(directory, "新状态.json"), "-timeout", "0s"},
	}
	for _, args := range cases {
		var stdout, stderr bytes.Buffer
		deps := commandDependencies{scan: func(context.Context, string, string) (migration.SourceSnapshot, error) {
			t.Fatal("非法转换参数触发扫描")
			return migration.SourceSnapshot{}, nil
		}}
		if code := runWithDependencies(args, &stdout, &stderr, deps); code != 1 || stderr.Len() == 0 {
			t.Fatalf("参数=%v 退出码=%d 错误=%s", args, code, stderr.String())
		}
	}
	content, _ := os.ReadFile(existing)
	if string(content) != "原状态" {
		t.Fatal("已有状态遭破坏")
	}
}

// TestRunConvertFailureAndBlock 验证阻断与转换失败均不产生状态文件。
// @param t 测试上下文
// @return 无
func TestRunConvertFailureAndBlock(t *testing.T) {
	// 阻断输出报告和退出二，底层错误只输出通用说明。
	for _, blocked := range []bool{false, true} {
		path := filepath.Join(t.TempDir(), "state.json")
		var stdout, stderr bytes.Buffer
		deps := commandDependencies{
			scan: func(context.Context, string, string) (migration.SourceSnapshot, error) {
				return migration.SourceSnapshot{}, nil
			},
			convert: func(context.Context, migration.SourceSnapshot, migration.Options) ([]byte, migration.Report, error) {
				if blocked {
					return []byte("被阻断的状态也不能发布"), migration.Report{Blocked: true}, nil
				}
				return nil, migration.Report{}, errors.New("password=秘密连接信息")
			},
		}
		code := runWithDependencies([]string{"-convert", "-out", path}, &stdout, &stderr, deps)
		want := 1
		if blocked {
			want = 2
		}
		if code != want || strings.Contains(stderr.String(), "秘密连接信息") {
			t.Fatalf("退出码=%d 错误=%s", code, stderr.String())
		}
		if blocked && !strings.Contains(stdout.String(), `"blocked": true`) {
			t.Fatal("阻断报告未输出")
		}
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("失败或阻断产生状态文件")
		}
	}
}

// TestRunConvertDoesNotReplaceRacingTarget 验证扫描后新建的目标也不能被转换结果覆盖。
// @param t 测试上下文
// @return 无
func TestRunConvertDoesNotReplaceRacingTarget(t *testing.T) {
	// 在前置检查之后、最终发布之前模拟另一个执行者创建目标。
	path := filepath.Join(t.TempDir(), "state.json")
	var stdout, stderr bytes.Buffer
	deps := commandDependencies{
		scan: func(context.Context, string, string) (migration.SourceSnapshot, error) {
			return migration.SourceSnapshot{}, nil
		},
		convert: func(context.Context, migration.SourceSnapshot, migration.Options) ([]byte, migration.Report, error) {
			if err := os.WriteFile(path, []byte("竞争者状态"), 0600); err != nil {
				t.Fatal(err)
			}
			return []byte("新状态"), migration.Report{}, nil
		},
	}
	if code := runWithDependencies([]string{"-convert", "-out", path}, &stdout, &stderr, deps); code != 1 {
		t.Fatal("竞争目标未拒绝")
	}
	content, _ := os.ReadFile(path)
	if string(content) != "竞争者状态" {
		t.Fatal("竞争目标遭覆盖")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatal("失败转换遗留临时文件")
	}
}

// TestWriteNewStateNeverOverwrites 验证完整发布和重复发布冲突。
// @param t 测试上下文
// @return 无
func TestWriteNewStateNeverOverwrites(t *testing.T) {
	// 第二次发布必须失败，并完整保留第一次发布的数据。
	path := filepath.Join(t.TempDir(), "state.json")
	if err := writeNewState(path, []byte("完整状态")); err != nil {
		t.Fatal(err)
	}
	if err := writeNewState(path, []byte("覆盖状态")); err == nil {
		t.Fatal("允许重复覆盖")
	}
	content, _ := os.ReadFile(path)
	if string(content) != "完整状态" {
		t.Fatal("原状态已损坏")
	}
	if err := writeNewState(filepath.Join(t.TempDir(), "缺失目录", "state.json"), nil); err == nil {
		t.Fatal("无效输出目录未失败")
	}
}

// TestRunConversionRejectsEmptyOrCancelledState 验证空结果及已取消上下文不能发布状态。
// @param t 测试上下文
// @return 无
func TestRunConversionRejectsEmptyOrCancelledState(t *testing.T) {
	// 转换依赖即使返回成功，也必须遵守结果完整性及截止时间。
	for _, cancelled := range []bool{false, true} {
		path := filepath.Join(t.TempDir(), "state.json")
		ctx, cancel := context.WithCancel(context.Background())
		if cancelled {
			cancel()
		}
		deps := commandDependencies{convert: func(context.Context, migration.SourceSnapshot, migration.Options) ([]byte, migration.Report, error) {
			if cancelled {
				return []byte("完整状态"), migration.Report{}, nil
			}
			return nil, migration.Report{}, nil
		}}
		var stdout, stderr bytes.Buffer
		code := runConversion(ctx, migration.SourceSnapshot{}, migration.Options{}, path, &stdout, &stderr, deps)
		cancel()
		if code != 1 || stderr.Len() == 0 || stdout.Len() != 0 {
			t.Fatalf("退出码=%d 输出=%s 错误=%s", code, stdout.String(), stderr.String())
		}
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("空状态或取消操作产生输出")
		}
	}
}

// TestRunConversionReportFailureKeepsPublishedState 验证报告输出失败时保留已完整发布的状态。
// @param t 测试上下文
// @return 无
func TestRunConversionReportFailureKeepsPublishedState(t *testing.T) {
	// 报告管道失败不应删除可能已被其他进程读取的有效状态文件。
	path := filepath.Join(t.TempDir(), "state.json")
	deps := commandDependencies{convert: func(context.Context, migration.SourceSnapshot, migration.Options) ([]byte, migration.Report, error) {
		return []byte("完整状态"), migration.Report{}, nil
	}}
	var stderr bytes.Buffer
	if code := runConversion(context.Background(), migration.SourceSnapshot{}, migration.Options{}, path, failingWriter{}, &stderr, deps); code != 1 {
		t.Fatal("报告输出失败未报告")
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "完整状态" || !strings.Contains(stderr.String(), "如状态已创建") {
		t.Fatal("已发布状态被删除或缺少明确提示")
	}
}
