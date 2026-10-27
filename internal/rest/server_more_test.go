package rest

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flowable-go/flowable/api"
)

// TestREST_HistoricQueries 验证历史端点。
// @param t 测试上下文
func TestREST_HistoricQueries(t *testing.T) {
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

	response, err := http.Get(server.URL + "/service/history/historic-process-instances")
	if err != nil {
		t.Fatalf("历史流程查询失败: %v", err)
	}
	var processPage struct {
		Total int `json:"total"`
	}
	_ = json.NewDecoder(response.Body).Decode(&processPage)
	_ = response.Body.Close()
	if processPage.Total != 1 {
		t.Fatalf("期望 1 个历史流程，实际 %d", processPage.Total)
	}

	response, err = http.Get(server.URL + "/service/history/historic-task-instances?processInstanceId=" + instanceID)
	if err != nil {
		t.Fatalf("历史任务查询失败: %v", err)
	}
	var taskPage struct {
		Total int `json:"total"`
	}
	_ = json.NewDecoder(response.Body).Decode(&taskPage)
	_ = response.Body.Close()
	if taskPage.Total != 1 {
		t.Fatalf("期望 1 个历史任务，实际 %d", taskPage.Total)
	}
}

// TestREST_MoreErrors 验证更多错误响应。
// @param t 测试上下文
func TestREST_MoreErrors(t *testing.T) {
	server, _ := newTestServer(t)
	requests := []struct {
		method string
		path   string
		body   string
		want   int
	}{
		{"GET", "/service/runtime/process-instances/missing/variables", "", http.StatusNotFound},
		{"DELETE", "/service/runtime/process-instances/missing", "", http.StatusNotFound},
		{"GET", "/service/runtime/tasks/missing", "", http.StatusNotFound},
		{"POST", "/service/runtime/tasks/missing", `{"action":"complete"}`, http.StatusNotFound},
		{"POST", "/service/runtime/tasks/missing", `{"action":"unknown"}`, http.StatusBadRequest},
		{"POST", "/service/runtime/process-instances", `{"processDefinitionKey":"missing"}`, http.StatusNotFound},
	}
	for _, tc := range requests {
		request, err := http.NewRequest(tc.method, server.URL+tc.path, bytes.NewBufferString(tc.body))
		if err != nil {
			t.Fatalf("构造请求失败: %v", err)
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("请求失败: %v", err)
		}
		if response.StatusCode != tc.want {
			t.Fatalf("%s %s 期望 %d，实际 %d", tc.method, tc.path, tc.want, response.StatusCode)
		}
		_ = response.Body.Close()
	}
}

// TestStatusFor 验证状态码映射。
// @param t 测试上下文
func TestStatusFor(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{api.ErrBadRequest, http.StatusBadRequest},
		{api.ErrForbidden, http.StatusForbidden},
		{api.ErrNotFound, http.StatusNotFound},
		{api.ErrConflict, http.StatusConflict},
		{api.ErrEngineClosed, http.StatusServiceUnavailable},
		{api.NewError("BPMN-1", "x", nil), http.StatusBadRequest},
		{api.NewError("REST-1", "x", nil), http.StatusBadRequest},
		{errors.New("unknown"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		if got := statusFor(tc.err); got != tc.want {
			t.Fatalf("statusFor(%v) 期望 %d，实际 %d", tc.err, tc.want, got)
		}
	}
}

// TestReflectSliceDefault 验证未知切片类型。
// @param t 测试上下文
func TestReflectSliceDefault(t *testing.T) {
	if got := reflectSlice(1); len(got) != 0 {
		t.Fatalf("未知类型应返回空切片")
	}
}

// TestRequestUserID 验证操作人解析。
// @param t 测试上下文
func TestRequestUserID(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := requestUserID(request); got != "" {
		t.Fatalf("无认证应返回空")
	}
	request.Header.Set("X-User-Id", "alice")
	if got := requestUserID(request); got != "alice" {
		t.Fatalf("请求头解析错误: %s", got)
	}
	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.SetBasicAuth("bob", "secret")
	if got := requestUserID(request); got != "bob" {
		t.Fatalf("Basic Auth 解析错误: %s", got)
	}
}

// TestParseVariablesEdges 验证变量解析边界。
// @param t 测试上下文
func TestParseVariablesEdges(t *testing.T) {
	if values, err := parseVariables(nil); err != nil || values != nil {
		t.Fatalf("nil 变量应返回空")
	}
	if values, err := parseVariables(json.RawMessage("null")); err != nil || values != nil {
		t.Fatalf("null 变量应返回空")
	}
	if _, err := parseVariables(json.RawMessage("{")); err == nil {
		t.Fatalf("非法变量应报错")
	}
	values, err := parseVariables(json.RawMessage(`{"b":2,"a":1}`))
	if err != nil || len(values) != 2 || values[0].Name != "a" {
		t.Fatalf("对象变量解析错误: %v %+v", err, values)
	}
}

// TestDecodeJSONUnknownField 验证未知字段。
// @param t 测试上下文
func TestDecodeJSONUnknownField(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"unknown":1}`))
	var payload startPayload
	if err := decodeJSON(request, &payload); err == nil {
		t.Fatalf("未知字段应报错")
	}
}

// TestPageParams 验证分页参数。
// @param t 测试上下文
func TestPageParams(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/?start=-1&size=2000", nil)
	start, size := pageParams(request)
	if start != 0 || size != 1000 {
		t.Fatalf("分页参数规范化错误: %d %d", start, size)
	}
}

// TestServerString 验证服务描述。
// @param t 测试上下文
func TestServerString(t *testing.T) {
	server := New(nil)
	if server.String() == "" {
		t.Fatalf("服务描述为空")
	}
}

// TestREST_RunJobs 验证定时器执行端点。
// @param t 测试上下文
func TestREST_RunJobs(t *testing.T) {
	server, _ := newTestServer(t)
	response := postJSON(t, server.URL+"/service/management/jobs/run", map[string]any{})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("执行作业失败: %d", response.StatusCode)
	}
	var body map[string]any
	_ = json.NewDecoder(response.Body).Decode(&body)
	_ = response.Body.Close()
	if body["executed"] != float64(0) {
		t.Fatalf("期望执行 0 个作业，实际 %+v", body)
	}
}

const restMessageXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <message id="rm" name="restMessage"/>
  <signal id="rs" name="restSignal"/>
  <process id="restMessageProcess" isExecutable="true">
    <startEvent id="s"><messageEventDefinition messageRef="rm"/></startEvent>
    <sequenceFlow id="f1" sourceRef="s" targetRef="task"/>
    <userTask id="task" name="Task" flowable:assignee="§{assignee}"/>
    <sequenceFlow id="f2" sourceRef="task" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestREST_MessagesAndSignals 验证消息与信号端点。
// @param t 测试上下文
func TestREST_MessagesAndSignals(t *testing.T) {
	server, _ := newTestServer(t)
	response, err := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(restMessageXML))
	if err != nil || response.StatusCode != http.StatusCreated {
		t.Fatalf("部署失败: %v %d", err, response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/messages", map[string]any{
		"messageName": "restMessage",
		"variables":   []map[string]any{{"name": "assignee", "value": "alice"}},
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("消息投递失败: %d", response.StatusCode)
	}
	var result map[string]any
	_ = json.NewDecoder(response.Body).Decode(&result)
	_ = response.Body.Close()
	if result["triggered"] != float64(1) {
		t.Fatalf("期望触发 1 个实例: %+v", result)
	}
	response = postJSON(t, server.URL+"/service/runtime/signals", map[string]any{"signalName": "unknown"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("信号端点失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, err = http.Post(server.URL+"/service/runtime/messages", "application/json", bytes.NewBufferString("{"))
	if err != nil || response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法消息请求应返回 400: %v %d", err, response.StatusCode)
	}
	_ = response.Body.Close()
}
