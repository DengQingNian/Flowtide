package rest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

const restTransientXML = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="restTransient" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="taskA"/><userTask id="taskA" name="A"/><sequenceFlow id="f2" sourceRef="taskA" targetRef="taskB"><conditionExpression>${approved == true}</conditionExpression></sequenceFlow><sequenceFlow id="f3" sourceRef="taskA" targetRef="e"/><userTask id="taskB" name="B"/><sequenceFlow id="f4" sourceRef="taskB" targetRef="e"/><endEvent id="e"/></process></definitions>`

const restTransientTriggerXML = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="transientTrigger" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="recv"/><receiveTask id="recv" name="Wait"/><sequenceFlow id="f2" sourceRef="recv" targetRef="task"/><userTask id="task" name="Task" flowable:assignee="${assignee}"/><sequenceFlow id="f3" sourceRef="task" targetRef="e"/><endEvent id="e"/></process></definitions>`

// TestREST_TransientVariables 验证 REST 瞬态变量。
// @param t 测试上下文
func TestREST_TransientVariables(t *testing.T) {
	server, _ := newTestServer(t)
	response, _ := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(restTransientXML))
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{
		"processDefinitionKey": "restTransient",
		"variables":            []map[string]any{{"name": "approved", "value": false}},
	})
	var instance struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks?processInstanceId=" + instance.ID)
	var tasks struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&tasks)
	_ = response.Body.Close()
	if len(tasks.Data) != 1 {
		t.Fatalf("初始任务错误: %+v", tasks)
	}
	response = postJSON(t, server.URL+"/service/runtime/tasks/"+tasks.Data[0].ID, map[string]any{
		"action":             "complete",
		"transientVariables": []map[string]any{{"name": "approved", "value": true}},
	})
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("完成任务失败: %d %s", response.StatusCode, body)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks?processInstanceId=" + instance.ID)
	_ = json.NewDecoder(response.Body).Decode(&tasks)
	_ = response.Body.Close()
	if len(tasks.Data) != 1 {
		t.Fatalf("瞬态变量未参与条件: %+v", tasks)
	}
	response, _ = http.Get(server.URL + "/service/runtime/process-instances/" + instance.ID + "/variables")
	var variables []map[string]any
	_ = json.NewDecoder(response.Body).Decode(&variables)
	_ = response.Body.Close()
	found := false
	for _, variable := range variables {
		if variable["name"] == "approved" {
			found = true
			if variable["value"] != false {
				t.Fatalf("瞬态变量污染持久变量: %+v", variable)
			}
		}
	}
	if !found {
		t.Fatalf("持久变量丢失: %+v", variables)
	}
}

// TestREST_TransientTrigger 验证触发动作的瞬态变量。
// @param t 测试上下文
func TestREST_TransientTrigger(t *testing.T) {
	server, _ := newTestServer(t)
	response, _ := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(restTransientTriggerXML))
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{"processDefinitionKey": "transientTrigger"})
	var instance struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()
	executionID := executionIDFor(t, server.URL, instance.ID)
	response = postJSONMethod(t, http.MethodPut, server.URL+"/service/runtime/executions/"+executionID, map[string]any{
		"action":             "trigger",
		"transientVariables": []map[string]any{{"name": "assignee", "value": "bob"}},
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("触发失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks?processInstanceId=" + instance.ID)
	var tasks struct {
		Data []struct {
			Assignee string `json:"assignee"`
		} `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&tasks)
	_ = response.Body.Close()
	if len(tasks.Data) != 1 || tasks.Data[0].Assignee != "bob" {
		t.Fatalf("瞬态变量未参与处理人解析: %+v", tasks)
	}
}
