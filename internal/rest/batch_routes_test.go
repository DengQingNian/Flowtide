package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// TestREST_BatchRoutes 验证批处理管理端点。
// @param t 测试上下文
func TestREST_BatchRoutes(t *testing.T) {
	server, _ := newTestServer(t)
	for _, xml := range []string{restMigrationV1, restMigrationV2} {
		response, _ := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(xml))
		_ = response.Body.Close()
	}
	ids := definitionIDs(t, server.URL)
	v1, v2 := ids[1], ids[2]
	response := postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{"processDefinitionId": v1})
	var instance struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()

	response = postQuery(t, server.URL, "/service/repository/process-definitions/"+v2+"/batch-migrate", map[string]any{
		"processInstanceIds": []string{instance.ID},
		"activityMappings":   map[string]string{"oldTask": "newTask"},
	}, "")
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("批处理迁移失败: %d", response.StatusCode)
	}
	var batch struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		Total     int    `json:"total"`
		Completed int    `json:"completed"`
	}
	_ = json.NewDecoder(response.Body).Decode(&batch)
	_ = response.Body.Close()
	if batch.ID == "" || batch.Total != 1 || batch.Completed != 1 {
		t.Fatalf("批次结果错误: %+v", batch)
	}

	response, _ = http.Get(server.URL + "/service/management/batches?type=process-instance-migration")
	var page queryPageResult[map[string]any]
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || len(page.Data) == 0 {
		t.Fatalf("批次列表错误: %d %+v", response.StatusCode, page)
	}
	response, _ = http.Get(server.URL + "/service/management/batches/" + batch.ID)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("批次详情失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/batches/" + batch.ID + "/batch-parts")
	var parts queryPageResult[map[string]any]
	_ = json.NewDecoder(response.Body).Decode(&parts)
	_ = response.Body.Close()
	if len(parts.Data) != 1 {
		t.Fatalf("批次分片错误: %+v", parts)
	}
	partID, _ := parts.Data[0]["id"].(string)
	response, _ = http.Get(server.URL + "/service/management/batch-parts/" + partID)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("分片详情失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/batches/" + batch.ID + "/summary")
	var summary map[string]any
	_ = json.NewDecoder(response.Body).Decode(&summary)
	_ = response.Body.Close()
	if summary["completed"] != float64(1) {
		t.Fatalf("批次统计错误: %+v", summary)
	}
	for _, path := range []string{
		"/service/management/batches/missing",
		"/service/management/batches/missing/batch-parts",
		"/service/management/batches/missing/summary",
		"/service/management/batch-parts/missing",
	} {
		response, _ = http.Get(server.URL + path)
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("%s 应 404: %d", path, response.StatusCode)
		}
		_ = response.Body.Close()
	}
}
