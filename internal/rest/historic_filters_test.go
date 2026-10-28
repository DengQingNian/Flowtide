package rest

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestREST_TaskEventAndLogFilters 验证任务事件与历史日志过滤。
// @param t 测试上下文
func TestREST_TaskEventAndLogFilters(t *testing.T) {
	server, _ := newTestServer(t)
	task := createRESTTask(t, server.URL, map[string]any{"name": "Filtered"}, "")
	taskID, _ := task["id"].(string)
	response := postJSON(t, server.URL+"/service/runtime/tasks/"+taskID+"?action=claim", map[string]any{"action": "claim", "userId": "alice"})
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("签收失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID, map[string]any{"action": "complete", "userId": "alice"})
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("完成任务失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/events?action=complete&userId=alice")
	var events []map[string]any
	_ = json.NewDecoder(response.Body).Decode(&events)
	_ = response.Body.Close()
	if len(events) != 1 || events[0]["action"] != "complete" {
		t.Fatalf("任务事件过滤错误: %+v", events)
	}

	for _, query := range []string{
		"taskId=" + taskID + "&type=complete&userId=alice",
		"taskId=" + taskID + "&type=complete",
		"taskId=" + taskID,
	} {
		response, _ = http.Get(server.URL + "/service/history/historic-task-log-entries?" + query)
		var page queryPageResult[map[string]any]
		_ = json.NewDecoder(response.Body).Decode(&page)
		_ = response.Body.Close()
		if page.Total == 0 {
			t.Fatalf("历史任务日志过滤为空: %s %+v", query, page)
		}
	}
	response, _ = http.Get(server.URL + "/service/history/historic-task-log-entries?taskId=" + taskID + "&type=missing")
	var empty queryPageResult[map[string]any]
	_ = json.NewDecoder(response.Body).Decode(&empty)
	_ = response.Body.Close()
	if empty.Total != 0 {
		t.Fatalf("不存在类型应过滤为空: %+v", empty)
	}
	response, _ = http.Get(server.URL + "/service/history/historic-task-log-entries?taskId=" + taskID + "&from=bad")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法 from 应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
