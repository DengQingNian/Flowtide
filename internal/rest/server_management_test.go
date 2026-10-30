package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/DengQingNian/Flowtide/internal/engine"
)

// TestREST_JobAdmin 验证作业查询与重试。
// @param t 测试上下文
func TestREST_JobAdmin(t *testing.T) {
	server, app := newTestServer(t)
	response, _ := http.Get(server.URL + "/service/management/jobs/missing")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失作业应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/deadletter-jobs/missing")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失死信应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/management/jobs/missing", map[string]any{"action": "setRetries", "retries": 1})
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失作业设置重试应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	app.RegisterDelegate("noop", func(ctx context.Context, execution *engine.Execution) error { return nil })
	response, _ = http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(asyncXML))
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{"processDefinitionKey": "asyncAdmin"})
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/jobs")
	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	if len(page.Data) == 0 {
		t.Fatalf("作业列表为空")
	}
	jobID, _ := page.Data[0]["id"].(string)
	response, _ = http.Get(server.URL + "/service/management/jobs/" + jobID)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("作业查询失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/management/jobs/"+jobID, map[string]any{"action": "setRetries", "retries": 2})
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("设置重试失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_UserGroups 验证用户组端点。
// @param t 测试上下文
func TestREST_UserGroups(t *testing.T) {
	server, _ := newTestServer(t)
	response := postJSON(t, server.URL+"/service/identity/users", map[string]any{"id": "alice"})
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/identity/groups", map[string]any{"id": "dev"})
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/identity/users/alice/groups", map[string]any{"groupId": "dev"})
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("添加用户组失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/identity/users/alice/groups")
	var groups []map[string]any
	_ = json.NewDecoder(response.Body).Decode(&groups)
	_ = response.Body.Close()
	if len(groups) != 1 {
		t.Fatalf("用户组数量错误: %+v", groups)
	}
	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/service/identity/users/alice/groups/dev", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("移除用户组失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

const asyncXML = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="asyncAdmin" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="svc"/><serviceTask id="svc" flowable:delegateExpression="${{noop}" flowable:asyncBefore="true"/><sequenceFlow id="f2" sourceRef="svc" targetRef="e"/><endEvent id="e"/></process></definitions>`
