package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// TestREST_InjectActivity 验证动态活动注入端点。
// @param t 测试上下文
func TestREST_InjectActivity(t *testing.T) {
	server, _ := newTestServer(t)
	response, _ := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(restChangeStateXML))
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{"processDefinitionKey": "changeState"})
	var instance struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()

	response = postJSON(t, server.URL+"/service/runtime/process-instances/"+instance.ID+"/inject", map[string]any{
		"injectionType": "task",
		"name":          "Review",
		"assignee":      "bob",
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("注入失败: %d", response.StatusCode)
	}
	var injected map[string]any
	_ = json.NewDecoder(response.Body).Decode(&injected)
	_ = response.Body.Close()
	injectedID, _ := injected["id"].(string)
	if injectedID == "" || injected["name"] != "Review" {
		t.Fatalf("注入任务错误: %+v", injected)
	}
	response, _ = http.Get(server.URL + "/service/runtime/tasks?processInstanceId=" + instance.ID)
	var tasks struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&tasks)
	_ = response.Body.Close()
	if len(tasks.Data) != 2 {
		t.Fatalf("注入后任务数量错误: %+v", tasks)
	}
	response = postJSON(t, server.URL+"/service/runtime/tasks/"+injectedID, map[string]any{"action": "complete", "userId": "bob"})
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("完成注入任务失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/process-instances/" + instance.ID)
	var got map[string]any
	_ = json.NewDecoder(response.Body).Decode(&got)
	_ = response.Body.Close()
	if got["ended"] == true {
		t.Fatalf("注入任务完成不应结束流程")
	}

	for _, body := range []map[string]any{
		{"name": " "},
		{"injectionType": "subprocess", "name": "X"},
	} {
		response = postJSON(t, server.URL+"/service/runtime/process-instances/"+instance.ID+"/inject", body)
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("非法注入应 400 %+v: %d", body, response.StatusCode)
		}
		_ = response.Body.Close()
	}
	response = postJSON(t, server.URL+"/service/runtime/process-instances/missing/inject", map[string]any{"name": "X"})
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失实例应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
