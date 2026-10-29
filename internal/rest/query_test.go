package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flowable-go/flowable/internal/engine"
)

// TestREST_QueryClosedEngine 验证关闭引擎后的查询错误。
// @param t 测试上下文
func TestREST_QueryClosedEngine(t *testing.T) {
	app, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	server := httptest.NewServer(New(app).Handler())
	defer server.Close()
	_ = app.Close(context.Background())
	paths := []string{
		"/service/query/process-instances",
		"/service/query/executions",
		"/service/query/tasks",
		"/service/query/historic-process-instances",
		"/service/query/historic-task-instances",
		"/service/query/historic-activity-instances",
	}
	for _, path := range paths {
		response := postQuery(t, server.URL, path, map[string]any{}, "")
		if response.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("%s 关闭后应 503: %d", path, response.StatusCode)
		}
		_ = response.Body.Close()
	}
}

// queryPageResult 是查询分页响应。
type queryPageResult[T any] struct {
	Data  []T    `json:"data"`
	Total int    `json:"total"`
	Start int    `json:"start"`
	Size  int    `json:"size"`
	Sort  string `json:"sort"`
	Order string `json:"order"`
}

// postQuery 发送 POST 查询请求。
// @param t 测试上下文
// @param server 测试服务器
// @param path 路径
// @param body 查询体
// @param tenant 请求租户
// @return 响应
// @param serverURL HTTP 服务地址
func postQuery(t *testing.T, serverURL, path string, body any, tenant string) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("序列化查询体失败: %v", err)
	}
	request, err := http.NewRequest(http.MethodPost, serverURL+path, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("创建请求失败: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if tenant != "" {
		request.Header.Set("X-Tenant-Id", tenant)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("查询请求失败: %v", err)
	}
	return response
}

// decodeQueryPage 解析查询分页响应。
// @param t 测试上下文
// @param response 响应
// @return 分页结果
func decodeQueryPage[T any](t *testing.T, response *http.Response) queryPageResult[T] {
	t.Helper()
	var page queryPageResult[T]
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		t.Fatalf("解析查询响应失败: %v", err)
	}
	_ = response.Body.Close()
	return page
}

// startQueryInstance 通过 REST 启动流程实例并返回实例 ID。
// @param t 测试上下文
// @param server 测试服务器
// @param key 流程定义 Key
// @param businessKey 业务键
// @param name 实例名称
// @return 实例 ID
// @param serverURL HTTP 服务地址
func startQueryInstance(t *testing.T, serverURL, key, businessKey, name string) string {
	t.Helper()
	response := postJSON(t, serverURL+"/service/runtime/process-instances", map[string]any{
		"processDefinitionKey": key,
		"businessKey":          businessKey,
		"name":                 name,
		"variables":            []map[string]any{{"name": "initiator", "value": "alice"}},
	})
	defer response.Body.Close()
	var instance struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&instance); err != nil {
		t.Fatalf("解析启动响应失败: %v", err)
	}
	if instance.ID == "" {
		t.Fatalf("启动实例失败: %d", response.StatusCode)
	}
	return instance.ID
}

// completeQueryTask 完成指定实例的第一个任务。
// @param t 测试上下文
// @param server 测试服务器
// @param instanceID 实例 ID
// @param serverURL HTTP 服务地址
func completeQueryTask(t *testing.T, serverURL, instanceID string) {
	t.Helper()
	response, err := http.Get(serverURL + "/service/runtime/tasks?processInstanceId=" + instanceID)
	if err != nil {
		t.Fatalf("查询任务失败: %v", err)
	}
	var page struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		t.Fatalf("解析任务失败: %v", err)
	}
	_ = response.Body.Close()
	if len(page.Data) == 0 {
		t.Fatalf("实例没有待办任务")
	}
	response = postJSON(t, serverURL+"/service/runtime/tasks/"+page.Data[0].ID, map[string]any{"action": "complete"})
	if response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusOK {
		t.Fatalf("完成任务失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_QueryProcessInstances 验证流程实例查询。
// @param t 测试上下文
func TestREST_QueryProcessInstances(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	startQueryInstance(t, server.URL, "rest", "bk1", "Order A")
	startQueryInstance(t, server.URL, "rest", "bk2", "Order B")

	response := postQuery(t, server.URL, "/service/query/process-instances", map[string]any{"processDefinitionKey": "rest"}, "")
	page := decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total != 2 {
		t.Fatalf("按定义查询失败: %d %+v", response.StatusCode, page)
	}

	response = postQuery(t, server.URL, "/service/query/process-instances", map[string]any{"processBusinessKey": "bk1"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 {
		t.Fatalf("按业务键查询失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/query/process-instances", map[string]any{"processInstanceNameLike": "%order%"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 2 {
		t.Fatalf("按名称模糊查询失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/query/process-instances", map[string]any{"startedAfter": "2020-01-01T00:00:00Z"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 2 {
		t.Fatalf("按开始时间查询失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/query/process-instances", map[string]any{"startedBefore": "2000-01-01T00:00:00Z"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("开始时间上界查询失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/query/process-instances", map[string]any{"processDefinitionKeyIn": []string{"rest", "other"}}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 2 {
		t.Fatalf("定义 Key 集合查询失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/query/process-instances", map[string]any{"finished": false, "start": 1, "size": 1}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 2 || len(page.Data) != 1 || page.Start != 1 || page.Size != 1 {
		t.Fatalf("分页查询失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/query/process-instances", map[string]any{"sort": "businessKey", "order": "desc"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 2 || len(page.Data) != 2 || page.Data[0]["businessKey"] != "bk2" || page.Order != "desc" {
		t.Fatalf("排序查询失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/query/process-instances", map[string]any{"tenantId": "tenant-a"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("租户过滤失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/query/process-instances", map[string]any{"variables": []any{map[string]any{"name": "x"}}}, "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("未知字段应忽略: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response = postQuery(t, server.URL, "/service/query/process-instances", map[string]any{"sort": "missing"}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法排序字段应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response = postQuery(t, server.URL, "/service/query/process-instances", map[string]any{"order": "sideways"}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法排序方向应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response = postQuery(t, server.URL, "/service/query/process-instances", map[string]any{"startedAfter": "not-a-time"}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法时间应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	request, _ := http.NewRequest(http.MethodPost, server.URL+"/service/query/process-instances", bytes.NewBufferString("{"))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法 JSON 应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_QueryTasks 验证任务查询。
// @param t 测试上下文
func TestREST_QueryTasks(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	startQueryInstance(t, server.URL, "rest", "bk1", "Order A")
	startQueryInstance(t, server.URL, "rest", "bk2", "Order B")

	response := postQuery(t, server.URL, "/service/query/tasks", map[string]any{"assignee": "alice"}, "")
	page := decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total != 2 {
		t.Fatalf("按处理人查询失败: %d %+v", response.StatusCode, page)
	}

	response = postQuery(t, server.URL, "/service/query/tasks", map[string]any{"processInstanceBusinessKey": "bk1"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 {
		t.Fatalf("按业务键查询任务失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/query/tasks", map[string]any{"processDefinitionKey": "rest", "taskDefinitionKey": "task"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 2 {
		t.Fatalf("按定义与任务节点查询失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/query/tasks", map[string]any{"nameLike": "%approve%"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 2 {
		t.Fatalf("按任务名模糊查询失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/query/tasks", map[string]any{"unassigned": true}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("未签收任务查询失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/query/tasks", map[string]any{"priority": 50, "minimumPriority": 50, "maximumPriority": 60}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 2 {
		t.Fatalf("优先级查询失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/query/tasks", map[string]any{"candidateUser": "bob"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("候选人查询失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/query/tasks", map[string]any{"sort": "priority", "order": "desc", "size": 1}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 2 || len(page.Data) != 1 {
		t.Fatalf("任务分页排序失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/query/tasks", map[string]any{"unassigned": "yes"}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法布尔字段应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response = postQuery(t, server.URL, "/service/query/tasks", map[string]any{}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 2 {
		t.Fatalf("空查询失败: %+v", page)
	}
}

// TestREST_QueryHistoricProcesses 验证历史流程实例查询。
// @param t 测试上下文
func TestREST_QueryHistoricProcesses(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	firstID := startQueryInstance(t, server.URL, "rest", "bk1", "Order A")
	startQueryInstance(t, server.URL, "rest", "bk2", "Order B")
	completeQueryTask(t, server.URL, firstID)

	response := postQuery(t, server.URL, "/service/query/historic-process-instances", map[string]any{"finished": true}, "")
	page := decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total != 1 {
		t.Fatalf("已完成实例查询失败: %d %+v", response.StatusCode, page)
	}

	response = postQuery(t, server.URL, "/service/query/historic-process-instances", map[string]any{"finished": false}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 {
		t.Fatalf("运行中历史实例查询失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/query/historic-process-instances", map[string]any{"businessKey": "bk1"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 || page.Data[0]["businessKey"] != "bk1" {
		t.Fatalf("历史实例业务键查询失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/query/historic-process-instances", map[string]any{"endedAfter": "2020-01-01T00:00:00Z"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 {
		t.Fatalf("结束时间查询失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/query/historic-process-instances", map[string]any{"sort": "endTime", "order": "desc"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 2 {
		t.Fatalf("历史实例排序失败: %+v", page)
	}
}

// TestREST_QueryHistoricTasks 验证历史任务查询。
// @param t 测试上下文
func TestREST_QueryHistoricTasks(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	instanceID := startQueryInstance(t, server.URL, "rest", "bk1", "Order A")
	completeQueryTask(t, server.URL, instanceID)

	response := postQuery(t, server.URL, "/service/query/historic-task-instances", map[string]any{"finished": true}, "")
	page := decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total != 1 {
		t.Fatalf("已完成历史任务查询失败: %d %+v", response.StatusCode, page)
	}

	response = postQuery(t, server.URL, "/service/query/historic-task-instances", map[string]any{"finished": false}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("未完成历史任务查询失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/query/historic-task-instances", map[string]any{"taskAssignee": "alice", "processInstanceId": instanceID}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 {
		t.Fatalf("历史任务处理人查询失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/query/historic-task-instances", map[string]any{"taskNameLike": "%approve%"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 {
		t.Fatalf("历史任务名称查询失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/query/historic-task-instances", map[string]any{"startedAfter": "bad"}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法时间应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_QueryHistoricActivities 验证历史活动查询。
// @param t 测试上下文
func TestREST_QueryHistoricActivities(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	instanceID := startQueryInstance(t, server.URL, "rest", "bk1", "Order A")

	response := postQuery(t, server.URL, "/service/query/historic-activity-instances", map[string]any{"processInstanceId": instanceID}, "")
	page := decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total == 0 {
		t.Fatalf("按实例查询活动失败: %d %+v", response.StatusCode, page)
	}

	response = postQuery(t, server.URL, "/service/query/historic-activity-instances", map[string]any{"activityId": "task"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 {
		t.Fatalf("按活动 ID 查询失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/query/historic-activity-instances", map[string]any{"activityId": "missing"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("不存在活动查询失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/query/historic-activity-instances", map[string]any{"sort": "activityId"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total == 0 {
		t.Fatalf("活动排序查询失败: %+v", page)
	}
}

// TestREST_QueryExecutions 验证执行查询。
// @param t 测试上下文
func TestREST_QueryExecutions(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	instanceID := startQueryInstance(t, server.URL, "rest", "bk1", "Order A")

	response := postQuery(t, server.URL, "/service/query/executions", map[string]any{"processInstanceId": instanceID}, "")
	page := decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total == 0 {
		t.Fatalf("按实例查询执行失败: %d %+v", response.StatusCode, page)
	}

	response = postQuery(t, server.URL, "/service/query/executions", map[string]any{"activityId": "task"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 {
		t.Fatalf("按活动查询执行失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/query/executions", map[string]any{"ended": true}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("已结束执行查询失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/query/executions", map[string]any{"sort": "processInstanceId", "order": "desc"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total == 0 {
		t.Fatalf("执行排序查询失败: %+v", page)
	}
}
