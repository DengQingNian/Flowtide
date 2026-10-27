package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flowable-go/flowable/internal/engine"
)

// newTestServer 创建测试 HTTP 服务。
// @param t 测试上下文
// @return 测试服务器
// @return 引擎
func newTestServer(t *testing.T) (*httptest.Server, *engine.Engine) {
	t.Helper()
	app, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	server := httptest.NewServer(New(app).Handler())
	t.Cleanup(func() {
		server.Close()
		_ = app.Close(context.Background())
	})
	return server, app
}

const restProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <process id="rest" name="Rest" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="task"/>
    <userTask id="task" name="Approve" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f2" sourceRef="task" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// TestREST_DeployStartComplete 验证 REST 主流程。
// @param t 测试上下文
func TestREST_DeployStartComplete(t *testing.T) {
	server, _ := newTestServer(t)

	response, err := http.Post(server.URL+"/service/repository/deployments?deploymentName=test", "application/xml", bytes.NewBufferString(restProcess))
	if err != nil {
		t.Fatalf("部署请求失败: %v", err)
	}
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("期望 201，实际 %d: %s", response.StatusCode, body)
	}
	_ = response.Body.Close()

	startBody := map[string]any{
		"processDefinitionKey": "rest",
		"businessKey":          "biz-1",
		"variables":            []map[string]any{{"name": "initiator", "value": "alice"}},
	}
	response = postJSON(t, server.URL+"/service/runtime/process-instances", startBody)
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("启动失败: %d %s", response.StatusCode, body)
	}
	var instance map[string]any
	if err := json.NewDecoder(response.Body).Decode(&instance); err != nil {
		t.Fatalf("解析启动响应失败: %v", err)
	}
	_ = response.Body.Close()
	if instance["ended"] == true {
		t.Fatalf("启动后不应结束")
	}

	response, err = http.Get(server.URL + "/service/runtime/tasks?assignee=alice")
	if err != nil {
		t.Fatalf("查询任务失败: %v", err)
	}
	var page struct {
		Data  []map[string]any `json:"data"`
		Total int              `json:"total"`
	}
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		t.Fatalf("解析任务失败: %v", err)
	}
	_ = response.Body.Close()
	if page.Total != 1 {
		t.Fatalf("期望 1 个任务，实际 %d", page.Total)
	}
	taskID, _ := page.Data[0]["id"].(string)

	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID, map[string]any{
		"action": "complete",
		"userId": "alice",
	})
	if response.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("完成任务失败: %d %s", response.StatusCode, body)
	}
	_ = response.Body.Close()

	instanceID, _ := instance["id"].(string)
	response, err = http.Get(server.URL + "/service/runtime/process-instances/" + instanceID)
	if err != nil {
		t.Fatalf("查询实例失败: %v", err)
	}
	var got map[string]any
	_ = json.NewDecoder(response.Body).Decode(&got)
	_ = response.Body.Close()
	if got["ended"] != true {
		t.Fatalf("期望实例结束，实际 %+v", got)
	}
}

// TestREST_MultipartDeploy 验证 multipart 部署。
// @param t 测试上下文
func TestREST_MultipartDeploy(t *testing.T) {
	server, _ := newTestServer(t)
	buffer := &bytes.Buffer{}
	writer := multipart.NewWriter(buffer)
	part, err := writer.CreateFormFile("file", "process.bpmn20.xml")
	if err != nil {
		t.Fatalf("创建表单失败: %v", err)
	}
	if _, err := part.Write([]byte(restProcess)); err != nil {
		t.Fatalf("写表单失败: %v", err)
	}
	_ = writer.Close()
	response, err := http.Post(server.URL+"/service/repository/deployments?deploymentName=multi", writer.FormDataContentType(), buffer)
	if err != nil {
		t.Fatalf("部署请求失败: %v", err)
	}
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("期望 201，实际 %d: %s", response.StatusCode, body)
	}
	_ = response.Body.Close()
}

// TestREST_NotFound 验证 404 映射。
// @param t 测试上下文
func TestREST_NotFound(t *testing.T) {
	server, _ := newTestServer(t)
	response, err := http.Get(server.URL + "/service/runtime/process-instances/missing")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("期望 404，实际 %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_History 验证历史查询。
// @param t 测试上下文
func TestREST_History(t *testing.T) {
	server, _ := newTestServer(t)
	response, err := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(restProcess))
	if err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{
		"processDefinitionKey": "rest",
		"variables":            []map[string]any{{"name": "initiator", "value": "alice"}},
	})
	var instance map[string]any
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()
	instanceID, _ := instance["id"].(string)

	response, err = http.Get(server.URL + "/service/history/historic-activity-instances?processInstanceId=" + instanceID)
	if err != nil {
		t.Fatalf("查询历史失败: %v", err)
	}
	var page struct {
		Total int `json:"total"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	if page.Total != 2 {
		t.Fatalf("期望 2 条历史活动（开始事件与用户任务），实际 %d", page.Total)
	}
}

// postJSON 发送 JSON POST 请求。
// @param t 测试上下文
// @param url 地址
// @param body 请求体
// @return 响应
func postJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("序列化请求失败: %v", err)
	}
	response, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	return response
}
