package rest

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestREST_TaskDelegation 验证任务委派 REST 动作。
// @param t 测试上下文
func TestREST_TaskDelegation(t *testing.T) {
	server, _ := newTestServer(t)
	task := createRESTTask(t, server.URL, map[string]any{"name": "Task", "assignee": "alice"}, "")
	taskID, _ := task["id"].(string)
	response := postJSON(t, server.URL+"/service/runtime/tasks/"+taskID, map[string]any{"action": "delegate", "userId": "bob"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("委派失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID)
	var delegated map[string]any
	_ = json.NewDecoder(response.Body).Decode(&delegated)
	_ = response.Body.Close()
	if delegated["assignee"] != "bob" || delegated["owner"] != "alice" || delegated["delegationState"] != "pending" {
		t.Fatalf("委派状态错误: %+v", delegated)
	}
	response, _ = http.Get(server.URL + "/service/runtime/tasks?delegationState=PENDING")
	var page queryPageResult[map[string]any]
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	if len(page.Data) != 1 {
		t.Fatalf("委派过滤错误: %+v", page)
	}
	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID, map[string]any{"action": "resolve", "userId": "carol"})
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("非所有者处理应 403: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID, map[string]any{"action": "resolve", "userId": "alice"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("处理委派失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID)
	var resolved map[string]any
	_ = json.NewDecoder(response.Body).Decode(&resolved)
	_ = response.Body.Close()
	if resolved["assignee"] != "alice" || resolved["delegationState"] != "resolved" {
		t.Fatalf("处理后状态错误: %+v", resolved)
	}
	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID, map[string]any{"action": "resolve", "userId": "alice"})
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("重复处理应 409: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
