package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/flowable-go/flowable/internal/engine"
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
	server, _ := newHistoryTestServer(t, engine.HistoryFull)
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

// TestREST_ManagementMetadata 验证管理属性与表元数据端点。
// @param t 测试上下文
func TestREST_ManagementMetadata(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)

	response, _ := http.Get(server.URL + "/service/management/properties")
	var properties map[string]string
	_ = json.NewDecoder(response.Body).Decode(&properties)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || properties["schema.version"] == "" {
		t.Fatalf("引擎属性错误: %d %+v", response.StatusCode, properties)
	}
	response, _ = http.Get(server.URL + "/service/management/engine-properties")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("引擎属性端点错误: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response, _ = http.Get(server.URL + "/service/management/tables")
	var tables []struct {
		Name  string `json:"name"`
		Count int64  `json:"count"`
	}
	_ = json.NewDecoder(response.Body).Decode(&tables)
	_ = response.Body.Close()
	found := false
	for _, table := range tables {
		if table.Name == "ACT_RE_DEPLOYMENT" && table.Count == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("表列表缺少部署表: %+v", tables)
	}

	response, _ = http.Get(server.URL + "/service/management/tables/act_re_deployment")
	var table struct {
		Name  string `json:"name"`
		Count int64  `json:"count"`
	}
	_ = json.NewDecoder(response.Body).Decode(&table)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || table.Count != 1 || table.Name != "ACT_RE_DEPLOYMENT" {
		t.Fatalf("单表查询错误: %d %+v", response.StatusCode, table)
	}

	response, _ = http.Get(server.URL + "/service/management/tables/ACT_RE_PROCDEF/columns")
	var meta struct {
		TableName   string   `json:"tableName"`
		ColumnNames []string `json:"columnNames"`
	}
	_ = json.NewDecoder(response.Body).Decode(&meta)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || len(meta.ColumnNames) == 0 {
		t.Fatalf("列元数据错误: %d %+v", response.StatusCode, meta)
	}

	response, _ = http.Get(server.URL + "/service/management/tables/ACT_RE_PROCDEF/data?orderDescendingColumn=KEY_")
	var page struct {
		Data  []map[string]any `json:"data"`
		Total int              `json:"total"`
		Sort  string           `json:"sort"`
		Order string           `json:"order"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || page.Total != 1 || len(page.Data) != 1 {
		t.Fatalf("表数据错误: %d %+v", response.StatusCode, page)
	}
	if page.Sort != "KEY_" || page.Order != "desc" || page.Data[0]["KEY_"] != "rest" {
		t.Fatalf("表数据排序错误: %+v", page)
	}

	response, _ = http.Get(server.URL + "/service/management/tables/ACT_UNKNOWN")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("未知表应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/tables/ACT_UNKNOWN/columns")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("未知表列应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/tables/ACT_RE_PROCDEF/data?orderAscendingColumn=ID_&orderDescendingColumn=KEY_")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("同时排序应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_ManagementErrors 验证关闭引擎后的管理端点错误。
// @param t 测试上下文
func TestREST_ManagementErrors(t *testing.T) {
	app, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	server := httptest.NewServer(New(app).Handler())
	defer server.Close()
	_ = app.Close(context.Background())
	paths := []string{
		"/service/management/properties",
		"/service/management/tables",
		"/service/management/tables/ACT_RE_PROCDEF",
		"/service/management/tables/ACT_RE_PROCDEF/data",
	}
	for _, path := range paths {
		response, _ := http.Get(server.URL + path)
		if response.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("%s 关闭后应 503: %d", path, response.StatusCode)
		}
		_ = response.Body.Close()
	}
}

// TestREST_HistoricTenantIsolation 验证历史子资源租户隔离。
// @param t 测试上下文
func TestREST_HistoricTenantIsolation(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/service/runtime/process-instances", bytes.NewBufferString("{\"processDefinitionKey\":\"rest\",\"variables\":[{\"name\":\"initiator\",\"value\":\"alice\"}]}"))
	request.Header.Set("Content-Type", "application/json")
	response, _ := http.DefaultClient.Do(request)
	var instance struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()
	if instance.ID == "" {
		t.Fatalf("启动实例失败")
	}
	for _, suffix := range []string{"variables", "identitylinks", "comments"} {
		request, _ = http.NewRequest(http.MethodGet, server.URL+"/service/history/historic-process-instances/"+instance.ID+"/"+suffix, nil)
		request.Header.Set("X-Tenant-Id", "tenant-b")
		response, _ = http.DefaultClient.Do(request)
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("%s 跨租户应 404: %d", suffix, response.StatusCode)
		}
		_ = response.Body.Close()
	}
}

// TestREST_TaskFormAndHistoricSubresources 验证任务表单与历史子资源端点。
// @param t 测试上下文
func TestREST_TaskFormAndHistoricSubresources(t *testing.T) {
	server, _ := newTestServer(t)
	response, _ := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(restFormXML))
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{
		"processDefinitionKey": "restForm",
		"variables":            []map[string]any{{"name": "initiator", "value": "alice"}},
	})
	var instance struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()

	response, _ = http.Get(server.URL + "/service/runtime/tasks")
	var tasks struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&tasks)
	_ = response.Body.Close()
	if len(tasks.Data) == 0 {
		t.Fatalf("任务为空")
	}
	taskID := tasks.Data[0].ID

	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/form")
	var form struct {
		FormKey    string `json:"formKey"`
		Properties []struct {
			ID string `json:"id"`
		} `json:"formProperties"`
	}
	_ = json.NewDecoder(response.Body).Decode(&form)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || form.FormKey != "task" || len(form.Properties) != 1 {
		t.Fatalf("任务表单错误: %d %+v", response.StatusCode, form)
	}

	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID+"/comments", map[string]any{"message": "hello", "userId": "alice"})
	_ = response.Body.Close()

	response, _ = http.Get(server.URL + "/service/history/historic-process-instances/" + instance.ID + "/comments")
	var comments struct {
		Total int `json:"total"`
	}
	_ = json.NewDecoder(response.Body).Decode(&comments)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || comments.Total != 1 {
		t.Fatalf("历史评论错误: %d %+v", response.StatusCode, comments)
	}

	response, _ = http.Get(server.URL + "/service/history/historic-process-instances/" + instance.ID + "/identitylinks")
	var links struct {
		Total int `json:"total"`
	}
	_ = json.NewDecoder(response.Body).Decode(&links)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || links.Total == 0 {
		t.Fatalf("历史身份链接错误: %d %+v", response.StatusCode, links)
	}

	response, _ = http.Get(server.URL + "/service/history/historic-process-instances/" + instance.ID + "/variables")
	var variables struct {
		Total int `json:"total"`
	}
	_ = json.NewDecoder(response.Body).Decode(&variables)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || variables.Total == 0 {
		t.Fatalf("历史变量错误: %d %+v", response.StatusCode, variables)
	}

	response, _ = http.Get(server.URL + "/service/history/historic-process-instances/missing/comments")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失历史实例应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/history/historic-process-instances/missing/identitylinks")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失历史实例链接应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/history/historic-process-instances/missing/variables")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失历史实例变量应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_TaskFormMissing 验证无表单任务返回 400。
// @param t 测试上下文
func TestREST_TaskFormMissing(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	response := postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{
		"processDefinitionKey": "rest",
		"variables":            []map[string]any{{"name": "initiator", "value": "alice"}},
	})
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks")
	var tasks struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&tasks)
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + tasks.Data[0].ID + "/form")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("无表单任务应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks/missing/form")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失任务应 404: %d", response.StatusCode)
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
