package rest

import (
	"context"
	"net/http"
	"testing"
)

// TestREST_QueryPushdownFilterBranches 验证实例与历史查询接入下推过滤后的参数解析分支。
// @param t 测试上下文
func TestREST_QueryPushdownFilterBranches(t *testing.T) {
	server, app := newTestServer(t)
	deployREST(t, server)
	firstID := startQueryInstance(t, server.URL, "rest", "bk1", "Order A")
	startQueryInstance(t, server.URL, "rest", "bk2", "Order B")
	if err := app.SuspendProcessInstance(context.Background(), firstID); err != nil {
		t.Fatalf("挂起实例失败: %v", err)
	}

	// 挂起过滤由引擎下推或内存回退处理后，REST 再按同一语义复核
	response := postQuery(t, server.URL, "/service/query/process-instances", map[string]any{"suspended": true}, "")
	page := decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total != 1 || page.Data[0]["id"] != firstID {
		t.Fatalf("挂起实例查询失败: %d %+v", response.StatusCode, page)
	}
	response = postQuery(t, server.URL, "/service/query/process-instances", map[string]any{"suspended": false}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 {
		t.Fatalf("活动实例查询失败: %+v", page)
	}

	// 非法布尔与时间参数必须在进入引擎前被拒绝
	response = postQuery(t, server.URL, "/service/query/process-instances", map[string]any{"suspended": "yes"}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法挂起参数应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postQuery(t, server.URL, "/service/query/process-instances", map[string]any{"startedAfter": "not-a-time"}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法开始时间应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	// 历史查询的结束状态与结束区间由引擎过滤，REST 复核剩余字段；挂起实例需先恢复才能完成
	if err := app.ActivateProcessInstance(context.Background(), firstID); err != nil {
		t.Fatalf("恢复实例失败: %v", err)
	}
	completeQueryTask(t, server.URL, firstID)
	response = postQuery(t, server.URL, "/service/query/historic-process-instances", map[string]any{"finished": true, "businessKey": "bk1"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total != 1 || page.Data[0]["id"] != firstID {
		t.Fatalf("历史结束实例查询失败: %d %+v", response.StatusCode, page)
	}
	response = postQuery(t, server.URL, "/service/query/historic-process-instances", map[string]any{"finished": "x"}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法结束参数应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postQuery(t, server.URL, "/service/query/historic-process-instances", map[string]any{"endedBefore": "bad"}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法结束时间应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	// GET 历史列表在定义 Key 与业务键过滤下仍返回完整分页外壳
	response, err := http.Get(server.URL + "/service/history/historic-process-instances?processDefinitionKey=rest&businessKey=bk1")
	if err != nil {
		t.Fatalf("GET 历史列表失败: %v", err)
	}
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 || page.Data[0]["businessKey"] != "bk1" {
		t.Fatalf("GET 历史列表过滤失败: %+v", page)
	}
}
