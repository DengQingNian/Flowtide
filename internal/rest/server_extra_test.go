package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
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

// TestREST_DeleteExtras 验证删除附件与流程评论。
// @param t 测试上下文
func TestREST_DeleteExtras(t *testing.T) {
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
	response = postJSON(t, server.URL+"/service/runtime/process-instances/"+instanceID+"/attachments", map[string]any{"name": "x"})
	var attachment map[string]any
	_ = json.NewDecoder(response.Body).Decode(&attachment)
	_ = response.Body.Close()
	attachmentID, _ := attachment["id"].(string)
	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/service/runtime/process-instances/"+instanceID+"/attachments/"+attachmentID, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除流程附件失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances/"+instanceID+"/comments", map[string]any{"message": "x"})
	var comment map[string]any
	_ = json.NewDecoder(response.Body).Decode(&comment)
	_ = response.Body.Close()
	commentID, _ := comment["id"].(string)
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/runtime/process-instances/"+instanceID+"/comments/"+commentID, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除流程评论失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/runtime/process-instances/"+instanceID+"/comments/missing", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失流程评论应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/runtime/tasks/"+instanceID+"/attachments/missing", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失附件应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_DeleteHistoric 验证历史记录删除。
// @param t 测试上下文
func TestREST_DeleteHistoric(t *testing.T) {
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
	response, _ = http.Get(server.URL + "/service/runtime/tasks")
	var tasks struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&tasks)
	_ = response.Body.Close()
	taskID, _ := tasks.Data[0]["id"].(string)
	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID, map[string]any{"action": "complete"})
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/history/historic-task-log-entries?taskId=" + taskID)
	var logs struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&logs)
	_ = response.Body.Close()
	if len(logs.Data) == 0 {
		t.Fatalf("任务日志为空")
	}
	logID, _ := logs.Data[0]["id"].(float64)
	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/service/history/historic-task-log-entries/"+strconv.FormatInt(int64(logID), 10), nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除任务日志失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/history/historic-detail?processInstanceId=" + instanceID)
	var details struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&details)
	_ = response.Body.Close()
	if len(details.Data) == 0 {
		t.Fatalf("历史明细为空")
	}
	detailID, _ := details.Data[0]["id"].(string)
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/history/historic-detail/"+detailID, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除历史明细失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/history/historic-detail/"+detailID, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("重复删除明细应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/history/historic-task-instances/"+taskID, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除历史任务失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/history/historic-task-instances/"+taskID, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("重复删除历史任务应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/history/historic-process-instances/"+instanceID, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除历史流程失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/history/historic-process-instances/" + instanceID)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("删除后历史流程应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/history/historic-process-instances/missing", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失历史流程删除应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/history/historic-task-log-entries/bad", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法日志 ID 应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_HistoricDetailLookup 验证历史活动/变量详情。
// @param t 测试上下文
func TestREST_HistoricDetailLookup(t *testing.T) {
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
	response, _ = http.Get(server.URL + "/service/history/historic-activity-instances?processInstanceId=" + instanceID)
	var activities struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&activities)
	_ = response.Body.Close()
	activityID, _ := activities.Data[0]["id"].(string)
	response, _ = http.Get(server.URL + "/service/history/historic-activity-instances/" + activityID)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("历史活动详情失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/history/historic-variable-instances?processInstanceId=" + instanceID)
	var variables struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&variables)
	_ = response.Body.Close()
	variableID, _ := variables.Data[0]["id"].(string)
	response, _ = http.Get(server.URL + "/service/history/historic-variable-instances/" + variableID)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("历史变量详情失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/service/history/historic-variable-instances/"+variableID, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除历史变量失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/history/historic-variable-instances/" + variableID)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("删除后历史变量应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/history/historic-activity-instances/missing")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失历史活动应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/history/historic-variable-instances/missing")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失历史变量应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/history/historic-variable-instances/missing", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失历史变量删除应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_ListFilters 验证列表过滤。
// @param t 测试上下文
func TestREST_ListFilters(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	response := postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{
		"processDefinitionKey": "rest",
		"businessKey":          "bk1",
		"variables":            []map[string]any{{"name": "initiator", "value": "alice"}},
	})
	_ = response.Body.Close()
	cases := []struct {
		path string
		want int
	}{
		{"/service/repository/process-definitions?key=rest", 1},
		{"/service/repository/process-definitions?key=other", 0},
		{"/service/repository/process-definitions?version=1", 1},
		{"/service/runtime/process-instances?processDefinitionKey=rest", 1},
		{"/service/runtime/process-instances?businessKey=bk1", 1},
		{"/service/runtime/process-instances?businessKey=missing", 0},
		{"/service/history/historic-process-instances?processDefinitionKey=rest", 1},
		{"/service/history/historic-process-instances?businessKey=bk1", 1},
		{"/service/history/historic-process-instances?ended=true", 0},
	}
	for _, tc := range cases {
		response, _ := http.Get(server.URL + tc.path)
		var page struct {
			Total int `json:"total"`
		}
		_ = json.NewDecoder(response.Body).Decode(&page)
		_ = response.Body.Close()
		if page.Total != tc.want {
			t.Fatalf("%s 期望 %d，实际 %d", tc.path, tc.want, page.Total)
		}
	}
}
