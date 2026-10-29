package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flowable-go/flowable/internal/engine"
	"github.com/flowable-go/flowable/internal/rest"
)

// TestSecurityConfig 验证标准服务通过配置接入认证、租户、权限、审计和指标。
// @param t 测试上下文
// @return 无
func TestSecurityConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "security.json")
	data := `{"basicUsers":{"alice":"secret"},"userRoles":{"alice":["reader"]},"rolePermissions":{"reader":["read"]},"tenantBindings":{"alice":"tenant-a"},"auditCapacity":10,"metrics":true}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	options, err := loadSecurityOptions(path)
	if err != nil {
		t.Fatal(err)
	}
	app, err := engine.New(engine.Config{})
	if err != nil {
		t.Fatal(err)
	}
	handler := rest.New(app, options...).Handler()
	for _, tc := range []struct {
		method, path, tenant string
		auth                 bool
		status               int
	}{
		{"GET", "/service/repository/deployments", "", false, 401},
		{"GET", "/service/repository/deployments", "", true, 200},
		{"GET", "/service/repository/deployments", "tenant-b", true, 403},
		{"POST", "/service/query/tasks", "", true, 200},
		{"POST", "/service/runtime/tasks", "", true, 403},
		{"GET", "/metrics", "", false, 200},
	} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		if tc.auth {
			r.SetBasicAuth("alice", "secret")
		}
		r.Header.Set("X-Tenant-Id", tc.tenant)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s %s=%d: %s", tc.method, tc.path, w.Code, w.Body.String())
		}
		if tc.path == "/metrics" && !strings.Contains(w.Body.String(), "flowable_http_requests_total") {
			t.Fatal("未接入 HTTP 指标")
		}
	}
}

// TestSecurityConfigValidation 验证非法配置在创建服务前失败且不泄露文件内容。
// @param t 测试上下文
// @return 无
func TestSecurityConfigValidation(t *testing.T) {
	if options, err := loadSecurityOptions(""); err != nil || len(options) != 0 {
		t.Fatalf("空配置: %v", err)
	}
	if _, err := loadSecurityOptions(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("缺失配置未拒绝")
	}
	for _, data := range []string{`{`, `null`, `{} {}`, `{"unknown":"不可泄露"}`, `{"auditCapacity":-1}`, `{"rolePermissions":{"r":["unknown"]}}`, `{"jwt":{}}`, `{"jwt":{"jwksUrl":"ftp://invalid"}}`, `{"userRoles":{"alice":["missing"]}}`, `{"basicUsers":{"":"secret"}}`} {
		path := filepath.Join(t.TempDir(), "security.json")
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadSecurityOptions(path); err == nil || strings.Contains(err.Error(), "不可泄露") {
			t.Fatalf("无效配置未安全拒绝: %s %v", data, err)
		}
	}
}
