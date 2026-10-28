package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

const restTriggerXML = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="restTrigger" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="recv"/><receiveTask id="recv" name="Wait"/><sequenceFlow id="f2" sourceRef="recv" targetRef="task"/><userTask id="task" name="Task" flowable:assignee="alice"/><sequenceFlow id="f3" sourceRef="task" targetRef="e"/><endEvent id="e"/></process></definitions>`

// TestREST_TriggerExecution 验证执行触发动作。
// @param t 测试上下文
func TestREST_TriggerExecution(t *testing.T) {
	server, _ := newTestServer(t)
	response, _ := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(restTriggerXML))
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{"processDefinitionKey": "restTrigger"})
	var instance struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()
	executionID := executionIDFor(t, server.URL, instance.ID)

	response = postJSONMethod(t, http.MethodPut, server.URL+"/service/runtime/executions/"+executionID, map[string]any{
		"action":    "trigger",
		"variables": []map[string]any{{"name": "note", "value": "ok"}},
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("触发失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks?processInstanceId=" + instance.ID)
	var tasks struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&tasks)
	_ = response.Body.Close()
	if len(tasks.Data) != 1 {
		t.Fatalf("触发后任务错误: %+v", tasks)
	}

	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{"processDefinitionKey": "restTrigger"})
	var second struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(response.Body).Decode(&second)
	_ = response.Body.Close()
	secondExecution := executionIDFor(t, server.URL, second.ID)
	response = postJSONMethod(t, http.MethodPut, server.URL+"/service/runtime/executions/"+secondExecution, map[string]any{"action": "signal"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("signal 别名触发失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	deployREST(t, server)
	normalInstance := startQueryInstance(t, server.URL, "rest", "bk1", "Order")
	normalExecution := executionIDFor(t, server.URL, normalInstance)
	response = postJSONMethod(t, http.MethodPut, server.URL+"/service/runtime/executions/"+normalExecution, map[string]any{"action": "trigger"})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("用户任务触发应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSONMethod(t, http.MethodPut, server.URL+"/service/runtime/executions/missing", map[string]any{"action": "trigger"})
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失执行应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
