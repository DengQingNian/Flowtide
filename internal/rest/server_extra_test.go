package rest

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestREST_InstanceTasksAndExecutionVars 验证实例任务与执行变量。
// @param t 测试上下文
func TestREST_InstanceTasksAndExecutionVars(t *testing.T) {
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
	response, _ = http.Get(server.URL + "/service/runtime/process-instances/" + instanceID + "/tasks")
	var tasks struct {
		Total int `json:"total"`
	}
	_ = json.NewDecoder(response.Body).Decode(&tasks)
	_ = response.Body.Close()
	if tasks.Total != 1 {
		t.Fatalf("实例任务数量错误: %d", tasks.Total)
	}
	response, _ = http.Get(server.URL + "/service/runtime/executions?processInstanceId=" + instanceID)
	var executions struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&executions)
	_ = response.Body.Close()
	executionID, _ := executions.Data[0]["id"].(string)
	response, _ = http.Get(server.URL + "/service/runtime/executions/" + executionID + "/variables")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("执行变量查询失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/executions/missing/variables")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失执行变量应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/process-instances/missing/tasks")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("缺失实例任务应 200: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/history/historic-task-instances/missing/identitylinks")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("缺失历史任务链接应 200: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
