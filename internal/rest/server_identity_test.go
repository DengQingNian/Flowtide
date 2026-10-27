package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/engine"
)

// TestREST_UpdateUser 验证用户更新与角色。
// @param t 测试上下文
func TestREST_UpdateUser(t *testing.T) {
	server, _ := newTestServer(t)
	response := postJSON(t, server.URL+"/service/identity/users", map[string]any{"id": "alice", "displayName": "A"})
	_ = response.Body.Close()
	request, _ := http.NewRequest(http.MethodPut, server.URL+"/service/identity/users/alice", bytes.NewBufferString(`{"displayName":"Alice","email":"a@example.com","roles":["reader"]}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("更新用户失败: %d", response.StatusCode)
	}
	var user map[string]any
	_ = json.NewDecoder(response.Body).Decode(&user)
	_ = response.Body.Close()
	if user["displayName"] != "Alice" {
		t.Fatalf("用户更新错误: %+v", user)
	}
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/identity/users/alice/roles", bytes.NewBufferString(`{"roles":["admin"]}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("设置角色失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_UserErrors 验证用户更新错误。
// @param t 测试上下文
func TestREST_UserErrors(t *testing.T) {
	server, _ := newTestServer(t)
	request, _ := http.NewRequest(http.MethodPut, server.URL+"/service/identity/users/missing", bytes.NewBufferString("{"))
	request.Header.Set("Content-Type", "application/json")
	response, _ := http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法用户体应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/identity/users/missing", bytes.NewBufferString(`{"displayName":"x"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失用户应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/identity/users/missing/roles", bytes.NewBufferString(`{"roles":["admin"]}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失用户角色应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_RoleCRUD 验证角色端点。
// @param t 测试上下文
func TestREST_RoleCRUD(t *testing.T) {
	server, _ := newTestServer(t)
	response := postJSON(t, server.URL+"/service/identity/roles", map[string]any{"id": "reader", "name": "Reader", "permissions": []string{"read"}})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("创建角色失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/identity/roles/reader")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("角色查询失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ := http.NewRequest(http.MethodPut, server.URL+"/service/identity/roles/reader", bytes.NewBufferString(`{"name":"R2","permissions":["read","write"]}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("角色更新失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/identity/roles/reader", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("角色删除失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/identity/roles")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("角色列表失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/identity/roles", map[string]any{"id": "reader"})
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/identity/roles", map[string]any{"id": "reader"})
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("重复角色应 409: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/identity/roles", map[string]any{})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("空角色应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/identity/roles/missing")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失角色应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/identity/roles/missing", bytes.NewBufferString(`{"name":"x"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失角色更新应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/identity/roles/missing", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失角色删除应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestIdentityRolePermissions 验证引擎角色权限。
// @param t 测试上下文
func TestIdentityRolePermissions(t *testing.T) {
	app, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	if _, err := app.IdentityService().CreateUser(context.Background(), api.User{ID: "alice", Password: "secret", Roles: []string{"reader"}}); err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	if _, err := app.IdentityService().CreateRole(context.Background(), api.Role{ID: "reader", Permissions: []string{"read"}}); err != nil {
		t.Fatalf("创建角色失败: %v", err)
	}
	handler := New(app, WithIdentityAuth())
	server := httptest.NewServer(handler.Handler())
	defer func() {
		server.Close()
		_ = app.Close(context.Background())
	}()
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/service/repository/deployments", nil)
	request.SetBasicAuth("alice", "secret")
	response, _ := http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("角色权限查询应允许: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodPost, server.URL+"/service/repository/deployments", bytes.NewBufferString(restProcess))
	request.SetBasicAuth("alice", "secret")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("角色权限写入应 403: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
