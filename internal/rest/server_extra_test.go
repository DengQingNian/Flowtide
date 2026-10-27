package rest

import (
	"bytes"
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

// TestREST_SingleVariablesAndStartForm 验证单变量与启动表单。
// @param t 测试上下文
func TestREST_SingleVariablesAndStartForm(t *testing.T) {
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
	response, _ = http.Get(server.URL + "/service/runtime/process-instances/" + instanceID + "/variables/initiator")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("单变量查询失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/process-instances/" + instanceID + "/variables/missing")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失变量应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks")
	var tasks struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&tasks)
	_ = response.Body.Close()
	taskID, _ := tasks.Data[0]["id"].(string)
	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID+"/variables", map[string]any{"name": "local", "value": "v"})
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/variables/local")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("任务单变量查询失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/repository/process-definitions")
	var definitions struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&definitions)
	_ = response.Body.Close()
	definitionID, _ := definitions.Data[0]["id"].(string)
	response, _ = http.Get(server.URL + "/service/repository/process-definitions/" + definitionID + "/start-form")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("启动表单失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_ProcessComments 验证流程实例评论。
// @param t 测试上下文
func TestREST_ProcessComments(t *testing.T) {
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
	response = postJSON(t, server.URL+"/service/runtime/process-instances/"+instanceID+"/comments", map[string]any{"message": "ok", "userId": "alice"})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("添加流程评论失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/process-instances/" + instanceID + "/comments")
	var page struct {
		Total int `json:"total"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	if page.Total != 1 {
		t.Fatalf("流程评论数量错误: %d", page.Total)
	}
	response = postJSON(t, server.URL+"/service/runtime/process-instances/"+instanceID+"/comments", map[string]any{"message": ""})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("空评论应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/service/runtime/process-instances/"+instanceID+"/comments", bytes.NewBufferString("{"))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法评论体应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/process-instances/missing/comments")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("缺失实例评论列表应 200: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances/missing/comments", map[string]any{"message": "x"})
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失实例评论应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_ProcessAttachments 验证流程实例附件。
// @param t 测试上下文
func TestREST_ProcessAttachments(t *testing.T) {
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
	response = postJSON(t, server.URL+"/service/runtime/process-instances/"+instanceID+"/attachments", map[string]any{"name": "doc"})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("添加流程附件失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/process-instances/" + instanceID + "/attachments")
	var page struct {
		Total int `json:"total"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	if page.Total != 1 {
		t.Fatalf("流程附件数量错误: %d", page.Total)
	}
	response = postJSON(t, server.URL+"/service/runtime/process-instances/missing/attachments", map[string]any{"name": "x"})
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失实例附件应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances/"+instanceID+"/attachments", map[string]any{"description": "x"})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("空附件名应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/service/runtime/process-instances/"+instanceID+"/attachments", bytes.NewBufferString("{"))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法附件体应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/process-instances/missing/attachments")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("缺失实例附件列表应 200: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
