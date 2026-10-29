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

// verifyStateReport 返回固定字段的恢复校验报告，供 CLI 输出断言使用。
// @return 恢复校验报告
func verifyStateReport() migration.StateReport {
	return migration.StateReport{
		Format:                   migration.StateFormat,
		Version:                  2,
		Bytes:                    128,
		SHA256:                   "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		Deployments:              2,
		Definitions:              3,
		RunningInstances:         4,
		EndedInstances:           5,
		Tasks:                    6,
		HistoricProcessInstances: 7,
		HistoricTaskInstances:    8,
		HistoricVariables:        9,
		SequenceFlowHistory:      true,
		Limitations:              []string{"只读校验边界"},
	}
}

// TestRunVerifyState 验证恢复校验的依赖参数、JSON 报告、只读性与退出码。
// @param t 测试上下文
// @return 无
func TestRunVerifyState(t *testing.T) {
	directory := t.TempDir()
	statePath := filepath.Join(directory, "状态.json")
	state := []byte("秘密变量值")
	if err := os.WriteFile(statePath, state, 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	deps := commandDependencies{verifyState: func(ctx context.Context, data []byte) (migration.StateReport, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("缺少恢复校验超时")
		}
		if !bytes.Equal(data, state) {
			t.Fatal("状态字节未完整传递")
		}
		return verifyStateReport(), nil
	}}
	code := runWithDependencies([]string{"-verify-state", "-state", statePath, "-timeout", "1m"}, &stdout, &stderr, deps)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("退出码=%d 错误=%s", code, stderr.String())
	}
	// 报告字段固定，且不得回显状态文件中的业务内容。
	for _, field := range []string{`"format"`, `"version"`, `"bytes"`, `"sha256"`, `"deployments"`, `"definitions"`, `"runningInstances"`, `"endedInstances"`, `"tasks"`, `"historicProcessInstances"`, `"historicTaskInstances"`, `"historicVariables"`, `"sequenceFlowHistory"`, `"limitations"`} {
		if !strings.Contains(stdout.String(), field) {
			t.Fatalf("报告缺少字段 %s: %s", field, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "秘密变量值") {
		t.Fatal("报告泄漏状态内容")
	}
	content, err := os.ReadFile(statePath)
	if err != nil || !bytes.Equal(content, state) {
		t.Fatal("校验修改了状态文件")
	}
}

// TestRunVerifyStateRejectsInvalidArguments 验证模式互斥、状态必填与租户范围在读取文件前拒绝。
// @param t 测试上下文
// @return 无
func TestRunVerifyStateRejectsInvalidArguments(t *testing.T) {
	cases := [][]string{
		{"-verify-state"},
		{"-verify-state", "-state="},
		{"-verify-state", "-preflight", "-state", "状态.json"},
		{"-verify-state", "-convert", "-state", "状态.json", "-out", "新状态.json"},
		{"-verify-state", "-cutover-check", "-state", "状态.json"},
		{"-verify-state", "-apply", "-state", "状态.json"},
		{"-verify-state", "-tables", "-state", "状态.json"},
		{"-verify-state", "-state", "状态.json", "-tenant", "租户甲"},
		{"-verify-state", "-state", "状态.json", "-timeout", "0s"},
		{"-verify-state", "-state", "状态.json", "-quiesce-window", "1s"},
		{"-verify-state", "-state", "状态.json", "-dialect", "sqlite"},
		{"-state", "状态.json"},
		{"-state="},
	}
	// 所有无效参数必须在读取状态文件或调用恢复依赖之前失败。
	for _, args := range cases {
		var stdout, stderr bytes.Buffer
		called := false
		deps := commandDependencies{verifyState: func(context.Context, []byte) (migration.StateReport, error) {
			called = true
			return migration.StateReport{}, nil
		}}
		if code := runWithDependencies(args, &stdout, &stderr, deps); code != 1 || called || stderr.Len() == 0 {
			t.Fatalf("参数=%v 退出码=%d 调用=%v 错误=%s", args, code, called, stderr.String())
		}
	}
}

// TestRunVerifyStateFailures 验证文件缺失、恢复错误、取消与输出失败均返回一且不泄漏内容。
// @param t 测试上下文
// @return 无
func TestRunVerifyStateFailures(t *testing.T) {
	var stdout, stderr bytes.Buffer
	called := false
	deps := commandDependencies{verifyState: func(context.Context, []byte) (migration.StateReport, error) {
		called = true
		return migration.StateReport{}, nil
	}}
	missing := filepath.Join(t.TempDir(), "机密状态.json")
	if code := runWithDependencies([]string{"-verify-state", "-state", missing}, &stdout, &stderr, deps); code != 1 || called || strings.Contains(stderr.String(), "机密状态") {
		t.Fatalf("缺失文件处理错误: 退出码=%d 调用=%v 错误=%s", code, called, stderr.String())
	}
	statePath := filepath.Join(t.TempDir(), "状态.json")
	if err := os.WriteFile(statePath, []byte("状态"), 0600); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	deps.verifyState = func(context.Context, []byte) (migration.StateReport, error) {
		return migration.StateReport{}, errors.New("password=秘密 路径=/秘密/状态.json")
	}
	if code := runWithDependencies([]string{"-verify-state", "-state", statePath}, &stdout, &stderr, deps); code != 1 || stdout.Len() != 0 || strings.Contains(stderr.String(), "秘密") {
		t.Fatalf("依赖错误泄漏: 退出码=%d 输出=%s 错误=%s", code, stdout.String(), stderr.String())
	}
	stderr.Reset()
	deps.verifyState = func(context.Context, []byte) (migration.StateReport, error) {
		return verifyStateReport(), nil
	}
	// 输出管道失败不得返回成功。
	if code := runWithDependencies([]string{"-verify-state", "-state", statePath}, failingWriter{}, &stderr, deps); code != 1 || stderr.Len() == 0 {
		t.Fatal("未处理校验报告输出失败")
	}
	// 依赖即使返回成功，已取消的上下文也不能发布报告。
	stderr.Reset()
	stdout.Reset()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if code := runVerifyState(ctx, statePath, "", &stdout, &stderr, deps); code != 1 || stdout.Len() != 0 {
		t.Fatalf("取消后发布报告: 退出码=%d 输出=%s", code, stdout.String())
	}
}

// TestRunVerifyStateOutput 验证报告拒绝覆盖已有文件、目录与配置，并保持私有权限。
// @param t 测试上下文
// @return 无
func TestRunVerifyStateOutput(t *testing.T) {
	directory := t.TempDir()
	statePath := filepath.Join(directory, "状态.json")
	if err := os.WriteFile(statePath, []byte("状态"), 0600); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(directory, "旧报告.json")
	if err := os.WriteFile(existing, []byte("旧报告"), 0644); err != nil {
		t.Fatal(err)
	}
	called := false
	deps := commandDependencies{verifyState: func(context.Context, []byte) (migration.StateReport, error) {
		called = true
		return verifyStateReport(), nil
	}}
	var stdout, stderr bytes.Buffer
	// 已存在目标必须在读取状态之前拒绝，且原文保持不变。
	if code := runWithDependencies([]string{"-verify-state", "-state", statePath, "-out", existing}, &stdout, &stderr, deps); code != 1 || called {
		t.Fatalf("覆盖已有报告未被拒绝: 退出码=%d 调用=%v", code, called)
	}
	if content, err := os.ReadFile(existing); err != nil || string(content) != "旧报告" {
		t.Fatal("已有报告被破坏")
	}
	// 目录与配置文件同样不能作为报告目标。
	for _, output := range []string{directory, ".env"} {
		stdout.Reset()
		stderr.Reset()
		if code := runWithDependencies([]string{"-verify-state", "-state", statePath, "-out", output}, &stdout, &stderr, deps); code != 1 || called {
			t.Fatalf("输出目标 %s 未被拒绝: 退出码=%d 调用=%v", output, code, called)
		}
	}
	// 新目标写入完整报告且权限为 0600，标准输出保持为空。
	newPath := filepath.Join(directory, "新报告.json")
	stdout.Reset()
	stderr.Reset()
	if code := runWithDependencies([]string{"-verify-state", "-state", statePath, "-out", newPath}, &stdout, &stderr, deps); code != 0 || !called || stdout.Len() != 0 {
		t.Fatalf("报告输出失败: 退出码=%d 调用=%v 错误=%s", code, called, stderr.String())
	}
	info, err := os.Stat(newPath)
	// Windows 不表达 Unix 权限位，仅要求文件创建成功。
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0600) {
		t.Fatalf("报告权限错误: %v %v", info, err)
	}
	content, err := os.ReadFile(newPath)
	if err != nil || !strings.Contains(string(content), `"format": "flowable-go-state"`) || !strings.Contains(string(content), `"deployments": 2`) {
		t.Fatalf("报告内容错误: %s %v", content, err)
	}
	var report migration.StateReport
	if err := json.Unmarshal(content, &report); err != nil || report.HistoricVariables != 9 {
		t.Fatalf("报告不可解析: %+v %v", report, err)
	}
	entries, _ := os.ReadDir(directory)
	if len(entries) != 3 {
		t.Fatalf("临时文件未清理: %v", entries)
	}
}

// TestRunVerifyStateWithRealVerify 验证 CLI 与真实 VerifyState 的接线可恢复合法状态。
// @param t 测试上下文
// @return 无
func TestRunVerifyStateWithRealVerify(t *testing.T) {
	path := filepath.Join(t.TempDir(), "状态.json")
	if err := os.WriteFile(path, []byte(`{"version":0}`), 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	deps := commandDependencies{verifyState: migration.VerifyState}
	if code := runWithDependencies([]string{"-verify-state", "-state", path}, &stdout, &stderr, deps); code != 0 || stderr.Len() != 0 {
		t.Fatalf("真实校验接线失败: 退出码=%d 错误=%s", code, stderr.String())
	}
	var report migration.StateReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("报告不可解析: %v", err)
	}
	if report.Format != migration.StateFormat || report.Version != 0 || report.Deployments != 0 || len(report.Limitations) == 0 {
		t.Fatalf("真实校验报告错误: %+v", report)
	}
}

// TestWriteVerifyReportNeverOverwrites 验证校验报告完整发布且拒绝重复覆盖和缺失目录。
// @param t 测试上下文
// @return 无
func TestWriteVerifyReportNeverOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "报告.json")
	if err := writeVerifyReport(path, []byte("完整报告")); err != nil {
		t.Fatal(err)
	}
	if err := writeVerifyReport(path, []byte("覆盖报告")); err == nil {
		t.Fatal("允许重复覆盖")
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "完整报告" {
		t.Fatalf("报告内容被破坏: %s %v", content, err)
	}
	if err := writeVerifyReport(filepath.Join(t.TempDir(), "缺失目录", "报告.json"), nil); err == nil {
		t.Fatal("缺失目录未失败")
	}
}
