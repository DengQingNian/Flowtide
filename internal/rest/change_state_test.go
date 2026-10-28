package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

const restChangeStateXML = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="changeState" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="taskA"/><userTask id="taskA" name="A" flowable:assignee="alice"/><sequenceFlow id="f2" sourceRef="taskA" targetRef="e"/><userTask id="taskB" name="B" flowable:assignee="alice"/><sequenceFlow id="f3" sourceRef="taskB" targetRef="e"/><endEvent id="e"/></process></definitions>`

// TestREST_ChangeActivityState 验证活动迁移端点。
// @param t 测试上下文
func TestREST_ChangeActivityState(t *testing.T) {
	server, _ := newTestServer(t)
	response, _ := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(restChangeStateXML))
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{"processDefinitionKey": "changeState"})
	var instance struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/executions?processInstanceId=" + instance.ID)
	var executions struct {
		Data []struct {
			ID       string `json:"id"`
			ParentID string `json:"parentId"`
		} `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&executions)
	_ = response.Body.Close()
	if len(executions.Data) == 0 {
		t.Fatalf("执行树为空")
	}
	executionID := executions.Data[0].ID

	response = postJSON(t, server.URL+"/service/runtime/executions/"+executionID+"/change-state", map[string]any{"startActivityIds": []string{"taskB"}})
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("执行迁移失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks?processInstanceId=" + instance.ID)
	var tasks struct {
		Data []struct {
			TaskDefinitionKey string `json:"taskDefinitionKey"`
		} `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&tasks)
	_ = response.Body.Close()
	if len(tasks.Data) != 1 || tasks.Data[0].TaskDefinitionKey != "taskB" {
		t.Fatalf("执行迁移后任务错误: %+v", tasks)
	}

	response = postJSON(t, server.URL+"/service/runtime/process-instances/"+instance.ID+"/change-state", map[string]any{"startActivityIds": []string{"taskA"}})
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("实例迁移失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks?processInstanceId=" + instance.ID)
	_ = json.NewDecoder(response.Body).Decode(&tasks)
	_ = response.Body.Close()
	if len(tasks.Data) != 1 || tasks.Data[0].TaskDefinitionKey != "taskA" {
		t.Fatalf("实例迁移后任务错误: %+v", tasks)
	}

	for _, body := range []map[string]any{
		{},
		{"startActivityIds": []string{"taskA", "taskB"}},
		{"startActivityIds": []string{"missing"}},
	} {
		response = postJSON(t, server.URL+"/service/runtime/executions/"+executionID+"/change-state", body)
		want := http.StatusBadRequest
		if len(body) > 0 {
			if ids, ok := body["startActivityIds"].([]string); ok && len(ids) == 1 {
				want = http.StatusNotFound
			}
		}
		if response.StatusCode != want {
			t.Fatalf("非法迁移请求期望 %d 实际 %d: %+v", want, response.StatusCode, body)
		}
		_ = response.Body.Close()
	}
	response = postJSON(t, server.URL+"/service/runtime/executions/missing/change-state", map[string]any{"startActivityIds": []string{"taskA"}})
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失执行应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances/missing/change-state", map[string]any{"startActivityIds": []string{"taskA"}})
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失实例应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
