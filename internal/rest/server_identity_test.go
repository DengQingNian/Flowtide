package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
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
