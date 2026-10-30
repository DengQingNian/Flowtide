package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/engine"
)

// TestBasicAuth 验证 Basic Auth。
// @param t 测试上下文
func TestBasicAuth(t *testing.T) {
	app, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	server := httptest.NewServer(New(app, WithBasicAuth(map[string]string{"alice": "secret"})).Handler())
	defer func() {
		server.Close()
		_ = app.Close(context.Background())
	}()
	response, err := http.Get(server.URL + "/service/management/engine")
	if err != nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("未认证应 401: %v %d", err, response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/service/management/engine", nil)
	request.SetBasicAuth("alice", "secret")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("认证后应 200: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodGet, server.URL+"/service/management/engine", nil)
	request.SetBasicAuth("alice", "bad")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("错误密码应 401: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/healthz")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("健康检查应公开: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestBearerAuth 验证 Bearer Token。
// @param t 测试上下文
func TestBearerAuth(t *testing.T) {
	app, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	server := httptest.NewServer(New(app, WithBearerToken("token-1")).Handler())
	defer func() {
		server.Close()
		_ = app.Close(context.Background())
	}()
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/service/management/engine", nil)
	request.Header.Set("Authorization", "Bearer token-1")
	response, _ := http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Bearer 认证后应 200: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodGet, server.URL+"/service/management/engine", nil)
	request.Header.Set("Authorization", "Bearer bad")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("错误令牌应 401: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestRBAC 验证基于角色的端点授权。
// @param t 测试上下文
func TestRBAC(t *testing.T) {
	app, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	options := []Option{
		WithBasicAuth(map[string]string{"alice": "secret", "root": "root"}),
		WithRBAC(
			map[string][]string{"alice": {"reader"}, "root": {"admin"}},
			map[string][]string{"reader": {"read"}, "admin": {"admin"}},
		),
	}
	handler := New(app, options...)
	if len(handler.rolePermissions) != 2 {
		t.Fatalf("RBAC 配置未生效: %+v", handler.rolePermissions)
	}
	server := httptest.NewServer(handler.Handler())
	defer func() {
		server.Close()
		_ = app.Close(context.Background())
	}()
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/service/repository/deployments", nil)
	request.SetBasicAuth("alice", "secret")
	response, _ := http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("只读角色应可查询: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodPost, server.URL+"/service/repository/deployments", strings.NewReader(restProcess))
	request.SetBasicAuth("alice", "secret")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("只读角色写入应 403: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodGet, server.URL+"/service/management/engine", nil)
	request.SetBasicAuth("alice", "secret")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("只读角色管理端点应 403: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodGet, server.URL+"/service/management/engine", nil)
	request.SetBasicAuth("root", "root")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("管理员应可访问: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestRequiredPermission 验证端点权限映射。
// @param t 测试上下文
func TestRequiredPermission(t *testing.T) {
	cases := []struct {
		method string
		path   string
		want   string
	}{
		{http.MethodGet, "/service/runtime/tasks", "read"},
		{http.MethodPost, "/service/runtime/tasks", "write"},
		{http.MethodGet, "/service/management/engine", "admin"},
		{http.MethodGet, "/service/identity/users", "admin"},
	}
	for _, tc := range cases {
		if got := requiredPermission(tc.method, tc.path); got != tc.want {
			t.Fatalf("%s %s 期望 %s，实际 %s", tc.method, tc.path, tc.want, got)
		}
	}
}

// TestPermitted 直接验证权限判断。
// @param t 测试上下文
func TestPermitted(t *testing.T) {
	server := &Server{
		userRoles:       map[string][]string{"alice": {"reader"}},
		rolePermissions: map[string][]string{"reader": {"read"}},
	}
	request := httptest.NewRequest(http.MethodPost, "/service/repository/deployments", nil)
	request.SetBasicAuth("alice", "secret")
	if server.permitted(request) {
		t.Fatalf("只读角色写入应拒绝")
	}
	request = httptest.NewRequest(http.MethodGet, "/service/repository/deployments", nil)
	request.SetBasicAuth("alice", "secret")
	if !server.permitted(request) {
		t.Fatalf("只读角色查询应允许")
	}
}

// TestIdentityAuth 验证身份库认证与 RBAC。
// @param t 测试上下文
func TestIdentityAuth(t *testing.T) {
	app, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	if _, err := app.IdentityService().CreateUser(context.Background(), api.User{ID: "alice", Password: "secret", Roles: []string{"reader"}}); err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	if _, err := app.IdentityService().CreateUser(context.Background(), api.User{ID: "root", Password: "root", Roles: []string{"admin"}}); err != nil {
		t.Fatalf("创建管理员失败: %v", err)
	}
	handler := New(app, WithIdentityAuth(), WithRBAC(nil, map[string][]string{
		"reader": {"read"}, "admin": {"admin"},
	}))
	server := httptest.NewServer(handler.Handler())
	defer func() {
		server.Close()
		_ = app.Close(context.Background())
	}()
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/service/repository/deployments", nil)
	request.SetBasicAuth("alice", "secret")
	response, _ := http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("身份库只读用户应可查询: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodPost, server.URL+"/service/repository/deployments", strings.NewReader(restProcess))
	request.SetBasicAuth("alice", "secret")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("身份库只读用户写入应 403: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodGet, server.URL+"/service/repository/deployments", nil)
	request.SetBasicAuth("alice", "bad")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("错误密码应 401: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodGet, server.URL+"/service/management/engine", nil)
	request.SetBasicAuth("root", "root")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("身份库管理员应可访问: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
