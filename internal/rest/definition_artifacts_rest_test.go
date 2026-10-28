package rest

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestREST_DefinitionArtifactsRoutes 验证定义模型与制品路由。
// @param t 测试上下文
func TestREST_DefinitionArtifactsRoutes(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	response, _ := http.Get(server.URL + "/service/repository/process-definitions?key=rest")
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
	id := page.Data[0].ID

	response, _ = http.Get(server.URL + "/service/repository/process-definitions/" + id + "/model")
	var model struct {
		ID        string `json:"id"`
		Processes []struct {
			FlowElements []struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			} `json:"flowElements"`
		} `json:"processes"`
	}
	_ = json.NewDecoder(response.Body).Decode(&model)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || model.ID != id || len(model.Processes) == 0 || len(model.Processes[0].FlowElements) == 0 {
		t.Fatalf("流程模型端点错误: %d %+v", response.StatusCode, model)
	}
	for _, suffix := range []string{"decisions", "decision-tables", "form-definitions"} {
		response, _ = http.Get(server.URL + "/service/repository/process-definitions/" + id + "/" + suffix)
		var items []any
		_ = json.NewDecoder(response.Body).Decode(&items)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK || len(items) != 0 {
			t.Fatalf("%s 应返回空数组: %d %+v", suffix, response.StatusCode, items)
		}
		response, _ = http.Get(server.URL + "/service/repository/process-definitions/missing/" + suffix)
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("%s 缺失定义应 404: %d", suffix, response.StatusCode)
		}
		_ = response.Body.Close()
	}
	response, _ = http.Get(server.URL + "/service/repository/process-definitions/missing/model")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失定义模型应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
