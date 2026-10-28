package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flowable-go/flowable/internal/migration"
)

// TestRunPreflight 验证只读预检输出、退出码及显式默认租户。
// @param t 测试上下文
// @return 无
func TestRunPreflight(t *testing.T) {
	// 两类报告使用相同输出契约，仅退出码区分阻断状态。
	for _, blocked := range []bool{false, true} {
		var stdout, stderr bytes.Buffer
		calls := 0
		deps := commandDependencies{scan: func(ctx context.Context, envPath, dialect string) (migration.SourceSnapshot, error) {
			calls++
			if envPath != "配置.env" || dialect != "postgres" {
				t.Fatal("配置未传递")
			}
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("缺少超时")
			}
			return migration.SourceSnapshot{Dialect: dialect}, nil
		}, analyze: func(snapshot migration.SourceSnapshot, options migration.Options) migration.Report {
			if options.TenantID == nil || *options.TenantID != "" {
				t.Fatal("显式默认租户丢失")
			}
			return migration.Report{Dialect: snapshot.Dialect, Blocked: blocked}
		}}
		got := runWithDependencies([]string{"-preflight", "-dialect", "postgres", "-env", "配置.env", "-tenant="}, &stdout, &stderr, deps)
		want := 0
		if blocked {
			want = 2
		}
		if got != want || calls != 1 || stderr.Len() != 0 || !strings.Contains(stdout.String(), `"blocked":`) {
			t.Fatalf("退出码=%d 输出=%s 错误=%s", got, stdout.String(), stderr.String())
		}
	}
}

// TestRunRejectsInvalidArguments 验证冲突参数在扫描数据库之前失败。
// @param t 测试上下文
// @return 无
func TestRunRejectsInvalidArguments(t *testing.T) {
	cases := [][]string{
		{"-preflight", "-apply"}, {"-preflight", "-tables"}, {"-apply", "-out", "schema.sql"},
		{"-apply", "-dialect", "postgres"}, {"-tenant", "租户甲"}, {"-preflight", "-dialect", "sqlite"},
		{"-timeout", "1s"},
		{"-preflight", "-timeout", "0s"}, {"-preflight", "-timeout", "-1s"}, {"-preflight", "多余参数"},
		{"-preflight", "-out", ".env"}, {"-preflight", "-env", "配置.env", "-out", "配置.env"},
	}
	// 所有无效参数必须在触发数据库依赖前拒绝。
	for _, args := range cases {
		var stdout, stderr bytes.Buffer
		deps := commandDependencies{scan: func(context.Context, string, string) (migration.SourceSnapshot, error) {
			t.Fatal("非法参数触发数据库连接")
			return migration.SourceSnapshot{}, nil
		}}
		if got := runWithDependencies(args, &stdout, &stderr, deps); got != 1 || stderr.Len() == 0 {
			t.Fatalf("参数=%v 退出码=%d 错误=%s", args, got, stderr.String())
		}
	}
}

// TestRunPreflightAtomicReport 验证报告权限、替换及扫描失败保留旧报告。
// @param t 测试上下文
// @return 无
func TestRunPreflightAtomicReport(t *testing.T) {
	// 先保留旧报告，再验证失败不覆盖、成功原子替换。
	path := filepath.Join(t.TempDir(), "报告.json")
	if err := os.WriteFile(path, []byte("旧报告"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	deps := commandDependencies{scan: func(context.Context, string, string) (migration.SourceSnapshot, error) {
		return migration.SourceSnapshot{}, errors.New("password=秘密")
	}}
	if got := runWithDependencies([]string{"-preflight", "-out", path}, &stdout, &stderr, deps); got != 1 || strings.Contains(stderr.String(), "秘密") {
		t.Fatalf("错误=%s 退出码=%d", stderr.String(), got)
	}
	content, _ := os.ReadFile(path)
	if string(content) != "旧报告" {
		t.Fatal("扫描失败破坏原报告")
	}
	deps.scan = func(context.Context, string, string) (migration.SourceSnapshot, error) {
		return migration.SourceSnapshot{}, nil
	}
	deps.analyze = func(migration.SourceSnapshot, migration.Options) migration.Report {
		return migration.Report{Blocked: true}
	}
	stdout.Reset()
	stderr.Reset()
	if got := runWithDependencies([]string{"-preflight", "-out", path}, &stdout, &stderr, deps); got != 2 || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("退出码=%d 输出=%s 错误=%s", got, stdout.String(), stderr.String())
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("报告权限=%v 错误=%v", info, err)
	}
	content, _ = os.ReadFile(path)
	if !strings.Contains(string(content), `"blocked": true`) {
		t.Fatalf("报告=%s", content)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatal("临时文件未清理")
	}
}

// TestRunSchemaAndHelp 验证原有 Schema 导出、表名和帮助入口。
// @param t 测试上下文
// @return 无
func TestRunSchemaAndHelp(t *testing.T) {
	// 原有只读命令无需提供任何数据库依赖。
	for _, args := range [][]string{nil, {"-tables"}, {"-h"}} {
		var stdout, stderr bytes.Buffer
		if code := runWithDependencies(args, &stdout, &stderr, commandDependencies{}); code != 0 {
			t.Fatalf("参数=%v 退出码=%d", args, code)
		}
		if stdout.Len()+stderr.Len() == 0 {
			t.Fatal("缺少输出")
		}
	}
	var stdout, stderr bytes.Buffer
	called := false
	deps := commandDependencies{apply: func(string) error { called = true; return nil }}
	if code := runWithDependencies([]string{"-apply"}, &stdout, &stderr, deps); code != 0 || !called {
		t.Fatal("原有初始化未调用")
	}
}

// TestReportRejectsConfigurationAliases 验证配置硬链接、软链接和不可用目录均提前拒绝。
// @param t 测试上下文
// @return 无
func TestReportRejectsConfigurationAliases(t *testing.T) {
	// 同一配置文件的不同路径必须统一拒绝。
	directory := t.TempDir()
	config := filepath.Join(directory, "database.conf")
	if err := os.WriteFile(config, []byte("秘密配置"), 0600); err != nil {
		t.Fatal(err)
	}
	hardlink := filepath.Join(directory, "报告硬链接.json")
	symlink := filepath.Join(directory, "报告软链接.json")
	if err := os.Link(config, hardlink); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(config, symlink); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{config, hardlink, symlink, directory, filepath.Join(directory, "不存在", "报告.json")} {
		if err := validateReportPath(path, config); err == nil {
			t.Fatalf("错误允许报告路径 %s", path)
		}
	}
	if err := validateReportPath(filepath.Join(directory, "新报告.json"), config); err != nil {
		t.Fatal(err)
	}
}

// TestRunFailures 验证参数解析、数据库初始化及输出失败的退出码。
// @param t 测试上下文
// @return 无
func TestRunFailures(t *testing.T) {
	// 各失败入口都返回操作失败，不输出底层敏感错误。
	var stdout, stderr bytes.Buffer
	if code := runWithDependencies([]string{"-不存在"}, &stdout, &stderr, commandDependencies{}); code != 1 {
		t.Fatal(code)
	}
	stderr.Reset()
	deps := commandDependencies{apply: func(string) error { return errors.New("password=秘密") }}
	if code := runWithDependencies([]string{"-apply"}, &stdout, &stderr, deps); code != 1 || strings.Contains(stderr.String(), "秘密") {
		t.Fatal("初始化错误泄漏配置")
	}
	stderr.Reset()
	deps.scan = func(context.Context, string, string) (migration.SourceSnapshot, error) {
		return migration.SourceSnapshot{}, nil
	}
	deps.analyze = func(migration.SourceSnapshot, migration.Options) migration.Report { return migration.Report{} }
	if code := runWithDependencies([]string{"-preflight"}, failingWriter{}, &stderr, deps); code != 1 || stderr.Len() == 0 {
		t.Fatal("未处理输出失败")
	}
}

// failingWriter 模拟报告输出失败。
type failingWriter struct{}

// Write 返回固定写入错误。
// @param content 待写内容
// @return 已写字节数
// @return 输出错误
func (failingWriter) Write(content []byte) (int, error) { return 0, errors.New("输出失败") }

// TestRunSchemaFile 验证普通 Schema 文件导出的既有行为。
// @param t 测试上下文
// @return 无
func TestRunSchemaFile(t *testing.T) {
	// 验证成功导出与目标不可写两条既有路径。
	path := filepath.Join(t.TempDir(), "schema.sql")
	var stdout, stderr bytes.Buffer
	if code := runWithDependencies([]string{"-dialect", "postgres", "-out", path}, &stdout, &stderr, commandDependencies{}); code != 0 {
		t.Fatal(stderr.String())
	}
	content, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(content), "ACT_RU_EXECUTION") {
		t.Fatal("Schema 文件内容错误")
	}
	if code := runWithDependencies([]string{"-out", filepath.Dir(path)}, &stdout, &stderr, commandDependencies{}); code != 1 {
		t.Fatal("未处理 Schema 输出失败")
	}
}

// TestScanSourceConfigurationErrors 验证预检配置错误不携带连接凭据。
// @param t 测试上下文
// @return 无
func TestScanSourceConfigurationErrors(t *testing.T) {
	// 两类数据库配置均以通用错误代替驱动原始信息。
	t.Setenv("db_mysql_host", "")
	if _, err := scanSource(context.Background(), filepath.Join(t.TempDir(), "不存在.env"), "mysql"); err == nil {
		t.Fatal("缺失 MySQL 配置未失败")
	}
	t.Setenv("FLOWABLE_POSTGRES_DSN", "")
	if _, err := scanSource(context.Background(), ".env", "postgres"); err == nil {
		t.Fatal("缺失 PostgreSQL 配置未失败")
	}
	t.Setenv("FLOWABLE_POSTGRES_DSN", "user=演示 password=秘密 port=错误")
	if _, err := scanSource(context.Background(), ".env", "postgres"); err == nil || strings.Contains(err.Error(), "秘密") {
		t.Fatal("无效 PostgreSQL 配置未脱敏")
	}
}

// TestRunPreflightTenantScopes 验证未指定租户与指定租户具有不同语义。
// @param t 测试上下文
// @return 无
func TestRunPreflightTenantScopes(t *testing.T) {
	// 未指定租户检查全库；只有显式参数才传递非空指针。
	for _, tenant := range []*string{nil, new("租户甲")} {
		args := []string{"-preflight"}
		if tenant != nil {
			args = append(args, "-tenant", *tenant)
		}
		var stdout, stderr bytes.Buffer
		deps := commandDependencies{
			scan: func(context.Context, string, string) (migration.SourceSnapshot, error) {
				return migration.SourceSnapshot{}, nil
			},
			analyze: func(_ migration.SourceSnapshot, options migration.Options) migration.Report {
				if tenant == nil && options.TenantID != nil {
					t.Fatal("全库范围错误缩小")
				}
				if tenant != nil && (options.TenantID == nil || *options.TenantID != *tenant) {
					t.Fatal("指定租户丢失")
				}
				return migration.Report{}
			},
		}
		if code := runWithDependencies(args, &stdout, &stderr, deps); code != 0 {
			t.Fatal(stderr.String())
		}
	}
}

// TestWriteReportFailureCleanup 验证输出目录失效或原子替换失败时清理临时文件。
// @param t 测试上下文
// @return 无
func TestWriteReportFailureCleanup(t *testing.T) {
	// 目标为目录时重命名失败，原目录及目录内数据必须保留。
	directory := t.TempDir()
	target := filepath.Join(directory, "已有目录")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeReport(target, []byte("报告")); err == nil {
		t.Fatal("目录错误替换为报告")
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 || entries[0].Name() != "已有目录" {
		t.Fatalf("临时文件未清理：%v %v", entries, err)
	}
	if err := writeReport(filepath.Join(directory, "缺失目录", "报告.json"), nil); err == nil {
		t.Fatal("缺失目录未失败")
	}
}
