package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/internal/migration"
)

// TestRunVerifySource 验证源库核对的参数、差异退出码及报告不覆盖约束。
// @param t 测试上下文
// @return 无
func TestRunVerifySource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("私有状态"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, matched := range []bool{true, false} {
		var stdout, stderr bytes.Buffer
		deps := commandDependencies{
			scan: func(ctx context.Context, env, dialect string) (migration.SourceSnapshot, error) {
				if dialect != "postgres" || env != "demo.env" {
					t.Fatal("源库参数丢失")
				}
				return migration.SourceSnapshot{Dialect: dialect}, nil
			},
			reconcile: func(ctx context.Context, source migration.SourceSnapshot, data []byte, options migration.Options) (migration.ReconcileReport, error) {
				if string(data) != "私有状态" || options.TenantID == nil || *options.TenantID != "a" {
					t.Fatal("核对状态或租户丢失")
				}
				return migration.ReconcileReport{Matched: matched, StateSHA256: "digest"}, nil
			},
		}
		want := 0
		if !matched {
			want = 2
		}
		if code := runWithDependencies([]string{"-verify-state", "-verify-source", "-state", path, "-dialect", "postgres", "-env", "demo.env", "-tenant", "a"}, &stdout, &stderr, deps); code != want || stderr.Len() != 0 || !strings.Contains(stdout.String(), `"stateSha256"`) || strings.Contains(stdout.String(), "私有状态") {
			t.Fatalf("源库核对退出码=%d 输出=%s 错误=%s", code, stdout.String(), stderr.String())
		}
	}
}

// TestRunVerifySourceFailures 验证错误不回显源库凭据，非法组合不触发扫描。
// @param t 测试上下文
// @return 无
func TestRunVerifySourceFailures(t *testing.T) {
	var stdout, stderr bytes.Buffer
	deps := commandDependencies{scan: func(context.Context, string, string) (migration.SourceSnapshot, error) {
		return migration.SourceSnapshot{}, errors.New("秘密凭据")
	}}
	if code := runWithDependencies([]string{"-verify-source"}, &stdout, &stderr, deps); code != 1 {
		t.Fatal("无效组合未拒绝")
	}
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if code := runWithDependencies([]string{"-verify-state", "-verify-source", "-state", path}, &stdout, &stderr, deps); code != 1 || strings.Contains(stderr.String(), "秘密") {
		t.Fatal("扫描失败未脱敏")
	}
}

// TestRunPostgresApply 验证 PostgreSQL 初始化只在显式 apply 时分派。
// @param t 测试上下文
// @return 无
func TestRunPostgresApply(t *testing.T) {
	var stdout, stderr bytes.Buffer
	called := false
	deps := commandDependencies{applyPostgres: func(path string) error {
		called = true
		if path != "pg.env" {
			t.Fatal("配置路径丢失")
		}
		return nil
	}}
	if code := runWithDependencies([]string{"-apply", "-dialect", "postgres", "-env", "pg.env"}, &stdout, &stderr, deps); code != 0 || !called {
		t.Fatalf("PG 初始化未分派: %d %s", code, stderr.String())
	}
}
