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

	"github.com/DengQingNian/Flowtide/internal/engine"
)

// newTestServer 创建测试 HTTP 服务。
// @param t 测试上下文
// @param tenants 可选的默认租户
// @return 测试服务器
// @return 引擎
func newTestServer(t *testing.T, tenants ...string) (*httptest.Server, *engine.Engine) {
	return newHistoryTestServer(t, engine.HistoryAudit, tenants...)
}

// newHistoryTestServer 创建显式历史级别的测试服务，变量明细测试必须选择 full。
// @param t 测试上下文
// @param historyLevel 历史级别
// @param tenants 可选默认租户
// @return 测试服务器
// @return 引擎
func newHistoryTestServer(t *testing.T, historyLevel string, tenants ...string) (*httptest.Server, *engine.Engine) {
	t.Helper()
	tenantID := ""
	if len(tenants) > 0 {
		tenantID = tenants[0]
	}
	app, err := engine.New(engine.Config{HistoryLevel: historyLevel, TenantID: tenantID})
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
	if response.StatusCode != http.StatusOK {
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
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("已结束实例应从运行时资源消失: %d %+v", response.StatusCode, got)
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
	// 原版把顺序流也记为历史活动。
	if page.Total != 3 {
		t.Fatalf("期望 3 条历史活动（开始、顺序流、任务），实际 %d", page.Total)
	}
	response, _ = http.Get(server.URL + "/service/runtime/tasks?processInstanceId=" + instanceID)
	var tasks struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&tasks)
	_ = response.Body.Close()
	taskID, _ := tasks.Data[0]["id"].(string)
	response, _ = http.Get(server.URL + "/service/history/historic-task-log-entries?taskId=" + taskID)
	var logs struct {
		Total int `json:"total"`
	}
	_ = json.NewDecoder(response.Body).Decode(&logs)
	_ = response.Body.Close()
	if logs.Total == 0 {
		t.Fatalf("任务日志为空")
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

// deployREST 通过 REST 部署测试流程。
// @param t 测试上下文
// @param server 测试服务器
func deployREST(t *testing.T, server *httptest.Server) {
	t.Helper()
	response, err := http.Post(server.URL+"/service/repository/deployments?deploymentName=test", "application/xml", bytes.NewBufferString(restProcess))
	if err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("部署状态错误: %d %s", response.StatusCode, body)
	}
	_ = response.Body.Close()
}

// TestREST_RepositoryQueries 验证定义查询。
// @param t 测试上下文
func TestREST_RepositoryQueries(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	response, err := http.Get(server.URL + "/service/repository/process-definitions")
	if err != nil {
		t.Fatalf("查询定义失败: %v", err)
	}
	var page struct {
		Data  []map[string]any `json:"data"`
		Total int              `json:"total"`
	}
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		t.Fatalf("解析定义失败: %v", err)
	}
	_ = response.Body.Close()
	if page.Total != 1 {
		t.Fatalf("期望 1 个定义，实际 %d", page.Total)
	}
	id, _ := page.Data[0]["id"].(string)
	response, err = http.Get(server.URL + "/service/repository/process-definitions/" + id)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("查询定义详情失败: %v %d", err, response.StatusCode)
	}
	_ = response.Body.Close()
	response, err = http.Get(server.URL + "/service/repository/process-definitions/" + id + "/xml")
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("查询 XML 失败: %v %d", err, response.StatusCode)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if !bytes.Contains(body, []byte("<process")) {
		t.Fatalf("XML 内容错误: %s", body)
	}
}

// TestREST_InstanceLifecycle 验证实例生命周期。
// @param t 测试上下文
func TestREST_InstanceLifecycle(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	response := postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{
		"processDefinitionKey": "rest",
		"variables":            []map[string]any{{"name": "initiator", "value": "alice"}},
	})
	var instance map[string]any
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()
	instanceID, _ := instance["id"].(string)

	response, err := http.Get(server.URL + "/service/runtime/process-instances")
	if err != nil {
		t.Fatalf("查询实例列表失败: %v", err)
	}
	var page struct {
		Total int `json:"total"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	if page.Total != 1 {
		t.Fatalf("期望 1 个实例，实际 %d", page.Total)
	}

	response, err = http.Get(server.URL + "/service/runtime/process-instances/" + instanceID + "/variables")
	if err != nil {
		t.Fatalf("查询变量失败: %v", err)
	}
	var variables []map[string]any
	_ = json.NewDecoder(response.Body).Decode(&variables)
	_ = response.Body.Close()
	if len(variables) != 1 || variables[0]["name"] != "initiator" {
		t.Fatalf("变量响应错误: %+v", variables)
	}

	response, err = http.Get(server.URL + "/service/runtime/tasks?processInstanceId=" + instanceID)
	if err != nil {
		t.Fatalf("查询任务失败: %v", err)
	}
	var taskPage struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&taskPage)
	_ = response.Body.Close()
	if len(taskPage.Data) != 1 {
		t.Fatalf("期望 1 个任务")
	}
	taskID, _ := taskPage.Data[0]["id"].(string)
	response, err = http.Get(server.URL + "/service/runtime/tasks/" + taskID)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("查询任务详情失败: %v %d", err, response.StatusCode)
	}
	_ = response.Body.Close()

	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/service/runtime/process-instances/"+instanceID+"?deleteReason=test", nil)
	response, err = http.DefaultClient.Do(request)
	if err != nil || response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除实例失败: %v %d", err, response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_TaskActions 验证任务动作。
// @param t 测试上下文
func TestREST_TaskActions(t *testing.T) {
	server, _ := newTestServer(t)
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="actions" isExecutable="true"><startEvent id="start"/><sequenceFlow id="f1" sourceRef="start" targetRef="task"/><userTask id="task" name="Task"/><sequenceFlow id="f2" sourceRef="task" targetRef="end"/><endEvent id="end"/></process></definitions>`
	response, err := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(xml))
	if err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{"processDefinitionKey": "actions"})
	_ = response.Body.Close()

	response, _ = http.Get(server.URL + "/service/runtime/tasks")
	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	taskID, _ := page.Data[0]["id"].(string)

	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID, map[string]any{"action": "claim", "userId": "alice"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("签收失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID, map[string]any{"action": "delegate", "userId": "bob"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("委派失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID, map[string]any{
		"action": "complete", "userId": "bob", "variables": []map[string]any{{"name": "approved", "value": true}},
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("完成失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_Errors 验证错误响应。
// @param t 测试上下文
func TestREST_Errors(t *testing.T) {
	server, _ := newTestServer(t)
	response, err := http.Post(server.URL+"/service/runtime/process-instances", "application/json", bytes.NewBufferString("{"))
	if err != nil || response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法 JSON 应返回 400: %v %d", err, response.StatusCode)
	}
	_ = response.Body.Close()

	response, err = http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(""))
	if err != nil || response.StatusCode != http.StatusBadRequest {
		t.Fatalf("空部署应返回 400: %v %d", err, response.StatusCode)
	}
	_ = response.Body.Close()

	response, err = http.Get(server.URL + "/service/nope")
	if err != nil || response.StatusCode != http.StatusNotFound {
		t.Fatalf("未知路由应返回 404: %v %d", err, response.StatusCode)
	}
	_ = response.Body.Close()

	response, err = http.Get(server.URL + "/service/repository/process-definitions/missing")
	if err != nil || response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失定义应返回 404: %v %d", err, response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_Metadata 验证健康检查与引擎信息。
// @param t 测试上下文
func TestREST_Metadata(t *testing.T) {
	server, _ := newTestServer(t)
	for _, path := range []string{"/healthz", "/readyz", "/service/management/engine"} {
		response, err := http.Get(server.URL + path)
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("%s 请求失败: %v %d", path, err, response.StatusCode)
		}
		_ = response.Body.Close()
	}
}

// TestREST_ObjectVariables 验证对象形式变量。
// @param t 测试上下文
func TestREST_ObjectVariables(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	response := postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{
		"processDefinitionKey": "rest",
		"variables":            map[string]any{"initiator": "alice"},
	})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("对象变量启动失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_BasicAuth 验证 Basic Auth 操作人。
// @param t 测试上下文
func TestREST_BasicAuth(t *testing.T) {
	server, _ := newTestServer(t)
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="auth" isExecutable="true"><startEvent id="start"/><sequenceFlow id="f1" sourceRef="start" targetRef="task"/><userTask id="task" name="Task"/><sequenceFlow id="f2" sourceRef="task" targetRef="end"/><endEvent id="end"/></process></definitions>`
	response, _ := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(xml))
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{"processDefinitionKey": "auth"})
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks")
	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	taskID, _ := page.Data[0]["id"].(string)
	body := bytes.NewBufferString(`{"action":"claim"}`)
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/service/runtime/tasks/"+taskID, body)
	request.Header.Set("Content-Type", "application/json")
	request.SetBasicAuth("carol", "secret")
	response, err := http.DefaultClient.Do(request)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("Basic Auth 签收失败: %v %d", err, response.StatusCode)
	}
	_ = response.Body.Close()
}
