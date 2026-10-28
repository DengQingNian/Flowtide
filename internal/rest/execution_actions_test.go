package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

const restActionMessageXML = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><message id="m" name="msg"/><process id="msgAction" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="wait"/><intermediateCatchEvent id="wait"><messageEventDefinition messageRef="m"/></intermediateCatchEvent><sequenceFlow id="f2" sourceRef="wait" targetRef="task"/><userTask id="task" name="Task" flowable:assignee="alice"/><sequenceFlow id="f3" sourceRef="task" targetRef="e"/><endEvent id="e"/></process></definitions>`

const restActionSignalXML = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><signal id="s" name="sig"/><process id="sigAction" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="wait"/><intermediateCatchEvent id="wait"><signalEventDefinition signalRef="s"/></intermediateCatchEvent><sequenceFlow id="f2" sourceRef="wait" targetRef="task"/><userTask id="task" name="Task"/><sequenceFlow id="f3" sourceRef="task" targetRef="e"/><endEvent id="e"/></process></definitions>`

// postJSONMethod 用指定方法发送 JSON 请求。
// @param t 测试上下文
// @param method HTTP 方法
// @param url 地址
// @param body 请求体
// @return 响应
func postJSONMethod(t *testing.T, method, url string, body any) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	request, err := http.NewRequest(method, url, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("创建请求失败: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	return response
}

// startActionInstance 部署并启动执行动作流程。
// @param t 测试上下文
// @param serverURL 服务地址
// @param xml BPMN
// @param key 流程 Key
// @return 实例 ID
func startActionInstance(t *testing.T, serverURL, xml, key string) string {
	t.Helper()
	response, _ := http.Post(serverURL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(xml))
	_ = response.Body.Close()
	response = postJSON(t, serverURL+"/service/runtime/process-instances", map[string]any{"processDefinitionKey": key})
	var instance struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()
	return instance.ID
}

// executionIDFor 返回实例根执行 ID。
// @param t 测试上下文
// @param serverURL 服务地址
// @param instanceID 实例 ID
// @return 执行 ID
func executionIDFor(t *testing.T, serverURL, instanceID string) string {
	t.Helper()
	response, _ := http.Get(serverURL + "/service/runtime/executions?processInstanceId=" + instanceID)
	var executions struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&executions)
	_ = response.Body.Close()
	if len(executions.Data) == 0 {
		t.Fatalf("执行树为空")
	}
	return executions.Data[0].ID
}

// TestREST_ExecutionActions 验证执行动作端点。
// @param t 测试上下文
func TestREST_ExecutionActions(t *testing.T) {
	server, _ := newTestServer(t)
	instanceID := startActionInstance(t, server.URL, restActionMessageXML, "msgAction")
	executionID := executionIDFor(t, server.URL, instanceID)
	response := postJSONMethod(t, http.MethodPut, server.URL+"/service/runtime/executions/"+executionID, map[string]any{
		"action":      "message_event_received",
		"messageName": "msg",
		"variables":   []map[string]any{{"name": "x", "value": 1}},
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("定向消息失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks?processInstanceId=" + instanceID)
	var tasks struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&tasks)
	_ = response.Body.Close()
	if len(tasks.Data) != 1 {
		t.Fatalf("消息后任务错误: %+v", tasks)
	}

	signalInstance := startActionInstance(t, server.URL, restActionSignalXML, "sigAction")
	signalExecution := executionIDFor(t, server.URL, signalInstance)
	response = postJSONMethod(t, http.MethodPut, server.URL+"/service/runtime/executions/"+signalExecution, map[string]any{
		"action":     "signal_event_received",
		"signalName": "sig",
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("定向信号失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response = postJSONMethod(t, http.MethodPut, server.URL+"/service/runtime/executions", map[string]any{
		"action":     "signal_event_received",
		"signalName": "sig",
	})
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("广播信号失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	for _, body := range []map[string]any{
		{"action": "unknown"},
		{"action": "signal_event_received"},
		{"action": "message_event_received"},
	} {
		response = postJSONMethod(t, http.MethodPut, server.URL+"/service/runtime/executions/"+executionID, body)
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("非法执行动作应 400 %+v: %d", body, response.StatusCode)
		}
		_ = response.Body.Close()
	}
	response = postJSONMethod(t, http.MethodPut, server.URL+"/service/runtime/executions/missing", map[string]any{
		"action":     "signal_event_received",
		"signalName": "sig",
	})
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失执行应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSONMethod(t, http.MethodPut, server.URL+"/service/runtime/executions", map[string]any{"action": "message_event_received"})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("集合非法动作应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
