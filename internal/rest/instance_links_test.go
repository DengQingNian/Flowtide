package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// TestREST_InstanceIdentityLinks 验证流程实例身份链接端点。
// @param t 测试上下文
func TestREST_InstanceIdentityLinks(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/service/runtime/process-instances", bytes.NewBufferString("{\"processDefinitionKey\":\"rest\",\"variables\":[{\"name\":\"initiator\",\"value\":\"alice\"}]}"))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-User-Id", "alice")
	response, _ := http.DefaultClient.Do(request)
	var instance struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()
	if instance.ID == "" {
		t.Fatalf("启动实例失败")
	}

	response, _ = http.Get(server.URL + "/service/runtime/process-instances/" + instance.ID + "/identitylinks")
	var links []map[string]any
	_ = json.NewDecoder(response.Body).Decode(&links)
	_ = response.Body.Close()
	// 原版发起人在运行时身份链接中类型为 starter。
	if response.StatusCode != http.StatusOK || len(links) != 1 || links[0]["type"] != "starter" || links[0]["userId"] != "alice" {
		t.Fatalf("发起人链接错误: %d %+v", response.StatusCode, links)
	}

	response = postQuery(t, server.URL, "/service/runtime/process-instances/"+instance.ID+"/identitylinks", map[string]any{"type": "candidate", "userId": "bob"}, "")
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("添加身份链接失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/process-instances/" + instance.ID + "/identitylinks")
	_ = json.NewDecoder(response.Body).Decode(&links)
	_ = response.Body.Close()
	if len(links) != 2 {
		t.Fatalf("链接数量错误: %+v", links)
	}

	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/runtime/process-instances/"+instance.ID+"/identitylinks/users/bob/candidate", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除身份链接失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/runtime/process-instances/"+instance.ID+"/identitylinks/users/bob/candidate", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("重复删除应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/runtime/process-instances/"+instance.ID+"/identitylinks/teams/bob/candidate", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法参与者类型应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response = postQuery(t, server.URL, "/service/runtime/process-instances/"+instance.ID+"/identitylinks", map[string]any{"type": "candidate"}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("缺少身份应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postQuery(t, server.URL, "/service/runtime/process-instances/"+instance.ID+"/identitylinks", map[string]any{"type": "candidate", "userId": "a", "groupId": "g"}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("同时指定用户与组应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postQuery(t, server.URL, "/service/runtime/process-instances/missing/identitylinks", map[string]any{"type": "candidate", "userId": "a"}, "")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失实例应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response, _ = http.Get(server.URL + "/service/history/historic-process-instances/" + instance.ID + "/identitylinks")
	// 原版返回裸数组而非分页对象。
	var historicLinks []map[string]any
	_ = json.NewDecoder(response.Body).Decode(&historicLinks)
	_ = response.Body.Close()
	if len(historicLinks) == 0 {
		t.Fatalf("历史身份链接为空")
	}

	request, _ = http.NewRequest(http.MethodGet, server.URL+"/service/runtime/process-instances/"+instance.ID+"/identitylinks", nil)
	request.Header.Set("X-Tenant-Id", "t2")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("跨租户查询应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
