package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/engine"
)

// newClosedServer 创建已关闭引擎的服务。
// @param t 测试上下文
// @return 测试服务器
func newClosedServer(t *testing.T) *httptest.Server {
	t.Helper()
	app, err := engine.New(engine.Config{})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	if err := app.Close(context.Background()); err != nil {
		t.Fatalf("关闭引擎失败: %v", err)
	}
	server := httptest.NewServer(New(app).Handler())
	t.Cleanup(server.Close)
	return server
}

// TestREST_ClosedEngine 验证关闭引擎的 503 响应。
// @param t 测试上下文
func TestREST_ClosedEngine(t *testing.T) {
	server := newClosedServer(t)
	paths := []string{
		"/readyz",
		"/service/repository/process-definitions",
		"/service/repository/deployments",
		"/service/identity/users",
		"/service/management/jobs",
		"/service/runtime/executions",
		"/service/form/form-data?processDefinitionId=x",
		"/service/runtime/tasks/x/identitylinks",
		"/service/runtime/tasks/x/comments",
		"/service/runtime/tasks/x/attachments",
		"/service/history/historic-process-instances",
		"/service/management/deadletter-jobs",
		"/service/identity/groups",
		"/service/runtime/process-instances",
		"/service/runtime/tasks",
		"/service/history/historic-process-instances",
		"/service/history/historic-activity-instances",
		"/service/history/historic-task-instances",
		"/service/history/historic-variable-instances",
	}
	posts := []string{"/service/runtime/messages", "/service/runtime/signals", "/service/management/jobs/run"}
	for _, path := range posts {
		response, err := http.Post(server.URL+path, "application/json", bytes.NewBufferString("{}"))
		if err != nil {
			t.Fatalf("%s 请求失败: %v", path, err)
		}
		if response.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("%s 期望 503，实际 %d", path, response.StatusCode)
		}
		_ = response.Body.Close()
	}
	others := []struct {
		method string
		path   string
	}{
		{"DELETE", "/service/identity/users/x"},
		{"DELETE", "/service/identity/groups/x"},
		{"DELETE", "/service/identity/groups/x/members/y"},
		{"PUT", "/service/runtime/tasks/x/variables/y"},
		{"DELETE", "/service/runtime/tasks/x/variables/y"},
		{"GET", "/service/history/historic-task-instances/x"},
		{"GET", "/service/runtime/tasks/x/variables"},
	}
	for _, item := range others {
		request, err := http.NewRequest(item.method, server.URL+item.path, bytes.NewBufferString("{}"))
		if err != nil {
			t.Fatalf("构造请求失败: %v", err)
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("%s 请求失败: %v", item.path, err)
		}
		if response.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("%s 期望 503，实际 %d", item.path, response.StatusCode)
		}
		_ = response.Body.Close()
	}
	for _, path := range paths {
		response, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatalf("%s 请求失败: %v", path, err)
		}
		if response.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("%s 期望 503，实际 %d", path, response.StatusCode)
		}
		_ = response.Body.Close()
	}
}

// TestREST_EmptyPages 验证空分页。
// @param t 测试上下文
func TestREST_EmptyPages(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	paths := []string{
		"/service/repository/process-definitions?start=100",
		"/service/runtime/process-instances?start=100",
		"/service/runtime/tasks?start=100",
		"/service/history/historic-process-instances?start=100",
	}
	for _, path := range paths {
		response, err := http.Get(server.URL + path)
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("%s 请求失败: %v %d", path, err, response.StatusCode)
		}
		_ = response.Body.Close()
	}
}

// TestREST_InvalidDeploy 验证非法部署。
// @param t 测试上下文
func TestREST_InvalidDeploy(t *testing.T) {
	server, _ := newTestServer(t)
	response, err := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString("<bad>"))
	if err != nil || response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法部署应返回 400: %v %d", err, response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_XMLMissing 验证缺失 XML。
// @param t 测试上下文
func TestREST_XMLMissing(t *testing.T) {
	server, _ := newTestServer(t)
	response, err := http.Get(server.URL + "/service/repository/process-definitions/missing/xml")
	if err != nil || response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失 XML 应返回 404: %v %d", err, response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_MultipartNoFile 验证 multipart 无文件。
// @param t 测试上下文
func TestREST_MultipartNoFile(t *testing.T) {
	server, _ := newTestServer(t)
	buffer := &bytes.Buffer{}
	writer := multipart.NewWriter(buffer)
	_ = writer.WriteField("deploymentName", "empty")
	_ = writer.Close()
	response, err := http.Post(server.URL+"/service/repository/deployments", writer.FormDataContentType(), buffer)
	if err != nil || response.StatusCode != http.StatusBadRequest {
		t.Fatalf("无文件部署应返回 400: %v %d", err, response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_TaskLocalVariables 验证任务局部变量完成。
// @param t 测试上下文
func TestREST_TaskLocalVariables(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	response := postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{
		"processDefinitionKey": "rest",
		"variables":            []map[string]any{{"name": "initiator", "value": "alice"}},
	})
	var instance map[string]any
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks")
	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	taskID, _ := page.Data[0]["id"].(string)
	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID, map[string]any{
		"action":         "complete",
		"userId":         "alice",
		"localVariables": []map[string]any{{"name": "local", "value": "v"}},
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("局部变量完成失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_WriteErrorBody 验证错误响应体。
// @param t 测试上下文
func TestREST_WriteErrorBody(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeError(recorder, api.NewError("TEST-1", "测试错误", errors.New("原因")))
	var body map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("解析错误体失败: %v", err)
	}
	if body["code"] != "TEST-1" || body["exception"] != "原因" {
		t.Fatalf("错误体字段错误: %+v", body)
	}
}

// TestWritePageUnknownType 验证未知类型分页。
// @param t 测试上下文
func TestWritePageUnknownType(t *testing.T) {
	recorder := httptest.NewRecorder()
	writePage(recorder, 1, 0, 10)
	if recorder.Code != http.StatusOK {
		t.Fatalf("未知类型分页状态错误: %d", recorder.Code)
	}
}
