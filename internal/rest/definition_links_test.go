package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

const restStarterXML = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="starter" isExecutable="true" flowable:candidateStarterUsers="alice" flowable:candidateStarterGroups="g1"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="task"/><userTask id="task" name="Task"/><sequenceFlow id="f2" sourceRef="task" targetRef="e"/><endEvent id="e"/></process></definitions>`

// TestREST_DefinitionIdentityLinks 验证候选发起人端点。
// @param t 测试上下文
func TestREST_DefinitionIdentityLinks(t *testing.T) {
	server, _ := newTestServer(t)
	response, _ := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(restStarterXML))
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/repository/process-definitions")
	var page struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	if len(page.Data) == 0 {
		t.Fatalf("流程定义为空")
	}
	definitionID := page.Data[0].ID

	response, _ = http.Get(server.URL + "/service/repository/process-definitions/" + definitionID + "/identitylinks")
	var links []map[string]any
	_ = json.NewDecoder(response.Body).Decode(&links)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || len(links) != 2 {
		t.Fatalf("候选发起人列表错误: %d %+v", response.StatusCode, links)
	}
	response, _ = http.Get(server.URL + "/service/repository/process-definitions/" + definitionID + "/identitylinks/users/alice")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("候选用户查询失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/repository/process-definitions/" + definitionID + "/identitylinks/groups/g1")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("候选组查询失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/repository/process-definitions/" + definitionID + "/identitylinks/users/missing")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失候选用户应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response = postQuery(t, server.URL, "/service/repository/process-definitions/"+definitionID+"/identitylinks", map[string]any{"userId": "carol"}, "")
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("添加候选发起人失败: %d", response.StatusCode)
	}
	var created map[string]any
	_ = json.NewDecoder(response.Body).Decode(&created)
	_ = response.Body.Close()
	response = postQuery(t, server.URL, "/service/repository/process-definitions/"+definitionID+"/identitylinks", map[string]any{"userId": "carol"}, "")
	var duplicate map[string]any
	_ = json.NewDecoder(response.Body).Decode(&duplicate)
	_ = response.Body.Close()
	if created["id"] != duplicate["id"] {
		t.Fatalf("重复添加应幂等: %+v %+v", created, duplicate)
	}
	response, _ = http.Get(server.URL + "/service/repository/process-definitions/" + definitionID + "/identitylinks")
	_ = json.NewDecoder(response.Body).Decode(&links)
	_ = response.Body.Close()
	if len(links) != 3 {
		t.Fatalf("添加后链接数量错误: %+v", links)
	}

	response = postQuery(t, server.URL, "/service/repository/process-definitions/"+definitionID+"/identitylinks", map[string]any{}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("缺少身份应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postQuery(t, server.URL, "/service/repository/process-definitions/"+definitionID+"/identitylinks", map[string]any{"userId": "a", "groupId": "g"}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("同时指定用户与组应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postQuery(t, server.URL, "/service/repository/process-definitions/missing/identitylinks", map[string]any{"userId": "a"}, "")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失定义应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/service/repository/process-definitions/"+definitionID+"/identitylinks/users/carol", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除候选发起人失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/repository/process-definitions/"+definitionID+"/identitylinks/users/carol", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("重复删除应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/repository/process-definitions/"+definitionID+"/identitylinks/teams/carol", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法参与者类型应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	request, _ = http.NewRequest(http.MethodGet, server.URL+"/service/repository/process-definitions/"+definitionID+"/identitylinks", nil)
	request.Header.Set("X-Tenant-Id", "t2")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("跨租户查询应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
