package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

const restMigrationV1 = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="migrate" name="V1" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="oldTask"/><userTask id="oldTask" name="Old"/><sequenceFlow id="f2" sourceRef="oldTask" targetRef="e"/><endEvent id="e"/></process></definitions>`

const restMigrationV2 = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="migrate" name="V2" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="newTask"/><userTask id="newTask" name="New"/><sequenceFlow id="f2" sourceRef="newTask" targetRef="e"/><endEvent id="e"/></process></definitions>`

// definitionIDs 返回流程定义版本到 ID 的映射。
// @param t 测试上下文
// @param serverURL 服务地址
// @return 映射
func definitionIDs(t *testing.T, serverURL string) map[int]string {
	t.Helper()
	response, _ := http.Get(serverURL + "/service/repository/process-definitions?key=migrate")
	var page struct {
		Data []struct {
			ID      string `json:"id"`
			Version int    `json:"version"`
		} `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	out := make(map[int]string)
	for _, item := range page.Data {
		out[item.Version] = item.ID
	}
	return out
}

// TestREST_MigrationValidateAndMigrate 验证迁移预检与批量迁移。
// @param t 测试上下文
func TestREST_MigrationValidateAndMigrate(t *testing.T) {
	server, _ := newTestServer(t)
	for _, xml := range []string{restMigrationV1, restMigrationV2} {
		response, _ := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(xml))
		_ = response.Body.Close()
	}
	ids := definitionIDs(t, server.URL)
	v1, v2 := ids[1], ids[2]
	if v1 == "" || v2 == "" {
		t.Fatalf("流程定义缺失: %+v", ids)
	}
	response := postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{"processDefinitionId": v1})
	var instance struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()
	if instance.ID == "" {
		t.Fatalf("启动失败")
	}

	response = postJSON(t, server.URL+"/service/runtime/process-instances/"+instance.ID+"/validate-migration", map[string]any{
		"targetProcessDefinitionId": v2,
		"activityMappings":          map[string]string{"oldTask": "newTask"},
	})
	var validation struct {
		Messages []string `json:"validationMessages"`
	}
	_ = json.NewDecoder(response.Body).Decode(&validation)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || len(validation.Messages) != 0 {
		t.Fatalf("迁移预检失败: %d %+v", response.StatusCode, validation)
	}
	response = postJSON(t, server.URL+"/service/runtime/process-instances/"+instance.ID+"/validate-migration", map[string]any{
		"targetProcessDefinitionId": v2,
		"activityMappings":          map[string]string{"oldTask": "missing"},
	})
	_ = json.NewDecoder(response.Body).Decode(&validation)
	_ = response.Body.Close()
	if len(validation.Messages) == 0 {
		t.Fatalf("非法映射应返回消息")
	}
	response = postJSON(t, server.URL+"/service/runtime/process-instances/"+instance.ID+"/validate-migration", map[string]any{})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("缺少目标应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances/missing/validate-migration", map[string]any{"targetProcessDefinitionId": v2})
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失实例预检应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response = postQuery(t, server.URL, "/service/repository/process-definitions/"+v2+"/migrate", map[string]any{
		"processInstanceIds": []string{instance.ID},
		"activityMappings":   map[string]string{"oldTask": "newTask"},
	}, "")
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("批量迁移失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response, _ = http.Get(server.URL + "/service/runtime/tasks?processInstanceId=" + instance.ID)
	var tasks struct {
		Data []struct {
			TaskDefinitionKey string `json:"taskDefinitionKey"`
		} `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&tasks)
	_ = response.Body.Close()
	if len(tasks.Data) != 1 || tasks.Data[0].TaskDefinitionKey != "newTask" {
		t.Fatalf("迁移后任务错误: %+v", tasks)
	}

	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{"processDefinitionId": v1})
	var second struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(response.Body).Decode(&second)
	_ = response.Body.Close()
	response = postQuery(t, server.URL, "/service/repository/process-definitions/"+v2+"/batch-migrate", map[string]any{
		"processInstanceIds": []string{second.ID},
		"activityMappings":   map[string]string{"oldTask": "newTask"},
	}, "")
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("批处理迁移失败: %d", response.StatusCode)
	}
	var batch struct {
		Total     int    `json:"total"`
		Completed int    `json:"completed"`
		Status    string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&batch); err != nil {
		t.Fatalf("解析批次响应失败: %v", err)
	}
	_ = response.Body.Close()
	if batch.Total != 1 || batch.Completed != 1 || batch.Status != "completed" {
		t.Fatalf("批次统计错误: %+v", batch)
	}
	response = postQuery(t, server.URL, "/service/repository/process-definitions/"+v2+"/migrate", map[string]any{}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("空实例列表应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postQuery(t, server.URL, "/service/repository/process-definitions/"+v2+"/migrate", map[string]any{
		"processInstanceIds": []string{"missing"},
	}, "")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失实例迁移应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	for _, path := range []string{
		"/service/runtime/process-instances/" + instance.ID + "/validate-migration",
		"/service/repository/process-definitions/" + v2 + "/migrate",
	} {
		request, _ := http.NewRequest(http.MethodPost, server.URL+path, bytes.NewBufferString("{"))
		request.Header.Set("Content-Type", "application/json")
		response, _ = http.DefaultClient.Do(request)
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("非法请求体应 400 %s: %d", path, response.StatusCode)
		}
		_ = response.Body.Close()
	}
}
