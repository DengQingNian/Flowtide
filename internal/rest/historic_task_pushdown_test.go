package rest

import (
	"net/http"
	"testing"
)

// TestREST_HistoricTaskQueryPushdown 验证历史任务查询接入引擎分页下推后过滤语义不变。
// @param t 测试上下文
func TestREST_HistoricTaskQueryPushdown(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	firstID := startQueryInstance(t, server.URL, "rest", "bk1", "Order A")
	secondID := startQueryInstance(t, server.URL, "rest", "bk2", "Order B")
	completeQueryTask(t, server.URL, firstID)

	// 结束状态过滤经引擎下推与内存复核后命中对应实例
	response := postQuery(t, server.URL, "/service/query/historic-task-instances", map[string]any{"finished": true, "processInstanceId": firstID}, "")
	page := decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total != 1 || page.Data[0]["processInstanceId"] != firstID {
		t.Fatalf("已完成历史任务查询失败: %d %+v", response.StatusCode, page)
	}
	taskID, _ := page.Data[0]["id"].(string)
	response = postQuery(t, server.URL, "/service/query/historic-task-instances", map[string]any{"finished": false}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 || page.Data[0]["processInstanceId"] != secondID {
		t.Fatalf("未完成历史任务查询失败: %+v", page)
	}

	// 双名与节点过滤字段按下推语义返回与之前一致的结果
	response = postQuery(t, server.URL, "/service/query/historic-task-instances", map[string]any{"taskId": taskID}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 || page.Data[0]["id"] != taskID {
		t.Fatalf("按任务 ID 查询历史任务失败: %+v", page)
	}
	response = postQuery(t, server.URL, "/service/query/historic-task-instances", map[string]any{"taskAssignee": "alice", "processInstanceId": secondID}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 {
		t.Fatalf("按处理人查询历史任务失败: %+v", page)
	}
	response = postQuery(t, server.URL, "/service/query/historic-task-instances", map[string]any{"taskName": "Approve"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 2 {
		t.Fatalf("按任务名查询历史任务失败: %+v", page)
	}
	response = postQuery(t, server.URL, "/service/query/historic-task-instances", map[string]any{"taskDefinitionKey": "task"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 2 {
		t.Fatalf("按任务节点查询历史任务失败: %+v", page)
	}

	// 定义 Key 下推后仍命中两条历史任务，陌生 Key 无结果
	response = postQuery(t, server.URL, "/service/query/historic-task-instances", map[string]any{"processDefinitionKey": "rest"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 2 {
		t.Fatalf("按定义 Key 查询历史任务失败: %+v", page)
	}
	response = postQuery(t, server.URL, "/service/query/historic-task-instances", map[string]any{"processDefinitionKey": "missing"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("陌生定义 Key 查询应无结果: %+v", page)
	}

	// 时间区间过滤与完成状态一致收敛
	response = postQuery(t, server.URL, "/service/query/historic-task-instances", map[string]any{"startedAfter": "2020-01-01T00:00:00Z"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 2 {
		t.Fatalf("开始时间下界查询失败: %+v", page)
	}
	response = postQuery(t, server.URL, "/service/query/historic-task-instances", map[string]any{"startedBefore": "2000-01-01T00:00:00Z"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("开始时间上界查询应无结果: %+v", page)
	}
	response = postQuery(t, server.URL, "/service/query/historic-task-instances", map[string]any{"endedAfter": "2020-01-01T00:00:00Z"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 {
		t.Fatalf("结束时间下界查询失败: %+v", page)
	}

	// 非法布尔与时间参数必须在进入引擎前被拒绝
	response = postQuery(t, server.URL, "/service/query/historic-task-instances", map[string]any{"finished": "x"}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法结束参数应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postQuery(t, server.URL, "/service/query/historic-task-instances", map[string]any{"startedAfter": "bad"}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法开始时间应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postQuery(t, server.URL, "/service/query/historic-task-instances", map[string]any{"endedBefore": "bad"}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法结束时间应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	// GET 历史任务列表按实例与 finished 参数过滤后返回完整分页外壳
	getResponse, err := http.Get(server.URL + "/service/history/historic-task-instances?processInstanceId=" + firstID + "&finished=true")
	if err != nil {
		t.Fatalf("GET 历史任务列表失败: %v", err)
	}
	page = decodeQueryPage[map[string]any](t, getResponse)
	if page.Total != 1 || page.Data[0]["processInstanceId"] != firstID {
		t.Fatalf("GET 历史任务列表过滤失败: %+v", page)
	}
	getResponse, err = http.Get(server.URL + "/service/history/historic-task-instances?finished=false")
	if err != nil {
		t.Fatalf("GET 历史任务列表失败: %v", err)
	}
	page = decodeQueryPage[map[string]any](t, getResponse)
	if page.Total != 1 || page.Data[0]["processInstanceId"] != secondID {
		t.Fatalf("GET 未完成历史任务列表过滤失败: %+v", page)
	}
}
