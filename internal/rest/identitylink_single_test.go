package rest

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestREST_IdentityLinkSingle 验证任务/实例单个身份链接读取与删除。
// @param t 测试上下文
func TestREST_IdentityLinkSingle(t *testing.T) {
	server, _ := newTestServer(t)
	task := createRESTTask(t, server.URL, map[string]any{"name": "Task"}, "")
	taskID, _ := task["id"].(string)
	response := postJSON(t, server.URL+"/service/runtime/tasks/"+taskID+"/identitylinks", map[string]any{
		"type": "candidate", "userId": "alice",
	})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("添加任务链接失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/identitylinks/users/alice")
	var link map[string]any
	_ = json.NewDecoder(response.Body).Decode(&link)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || link["userId"] != "alice" {
		t.Fatalf("任务链接读取错误: %d %+v", response.StatusCode, link)
	}
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/identitylinks/users/alice/candidate")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("带类型任务链接应 200: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/identitylinks/users/missing")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失任务链接应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	deployREST(t, server)
	instanceID := startQueryInstance(t, server.URL, "rest", "bk1", "Order")
	response = postJSON(t, server.URL+"/service/runtime/process-instances/"+instanceID+"/identitylinks", map[string]any{
		"type": "participant", "userId": "alice",
	})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("添加实例链接失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/process-instances/" + instanceID + "/identitylinks/users/alice")
	_ = json.NewDecoder(response.Body).Decode(&link)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || link["userId"] != "alice" {
		t.Fatalf("实例链接读取错误: %d %+v", response.StatusCode, link)
	}
	response, _ = http.Get(server.URL + "/service/runtime/process-instances/" + instanceID + "/identitylinks/groups/missing")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失实例链接应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/service/runtime/tasks/"+taskID+"/identitylinks/users/alice/candidate", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("带类型删除任务链接失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
