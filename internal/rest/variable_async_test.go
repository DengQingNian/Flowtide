package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// TestREST_VariableAsyncEndpoints 验证变量异步兼容端点。
// @param t 测试上下文
func TestREST_VariableAsyncEndpoints(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	instanceID := startQueryInstance(t, server.URL, "rest", "bk1", "Order")
	response, _ := http.Get(server.URL + "/service/runtime/executions?processInstanceId=" + instanceID)
	var executions struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&executions)
	_ = response.Body.Close()
	if len(executions.Data) == 0 {
		t.Fatalf("执行为空")
	}
	executionID := executions.Data[0].ID

	response = postJSON(t, server.URL+"/service/runtime/executions/"+executionID+"/variables", []map[string]any{{"name": "a", "value": "A"}})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("创建执行变量失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/executions/" + executionID + "/variables/a")
	var variable map[string]any
	_ = json.NewDecoder(response.Body).Decode(&variable)
	_ = response.Body.Close()
	if variable["value"] != "A" {
		t.Fatalf("执行变量未写入: %+v", variable)
	}

	request, _ := http.NewRequest(http.MethodPut, server.URL+"/service/runtime/executions/"+executionID+"/variables-async", bytes.NewBufferString(`[{"name":"b","value":"B"}]`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("异步执行变量失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/runtime/executions/"+executionID+"/variables-async/c", bytes.NewBufferString(`{"value":"C"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("异步单变量失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response = postJSON(t, server.URL+"/service/runtime/process-instances/"+instanceID+"/variables-async", []map[string]any{{"name": "d", "value": "D"}})
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("异步实例变量失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/runtime/process-instances/"+instanceID+"/variables-async/e", bytes.NewBufferString(`{"value":"E"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("异步单实例变量失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/process-instances/" + instanceID + "/variables")
	var variables []map[string]any
	_ = json.NewDecoder(response.Body).Decode(&variables)
	_ = response.Body.Close()
	names := map[string]bool{}
	for _, item := range variables {
		name, _ := item["name"].(string)
		names[name] = true
	}
	if !names["b"] || !names["c"] || !names["d"] || !names["e"] {
		t.Fatalf("异步变量未全部写入: %+v", names)
	}

	response = postJSON(t, server.URL+"/service/runtime/executions/"+executionID+"/variables-async", map[string]any{})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("空变量体应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/executions/missing/variables", []map[string]any{{"name": "x", "value": 1}})
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失执行应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	invalidBodies := []struct {
		method string
		path   string
		body   string
		want   int
	}{
		{"POST", "/service/runtime/executions/" + executionID + "/variables", "{", http.StatusBadRequest},
		{"PUT", "/service/runtime/executions/" + executionID + "/variables-async", "{", http.StatusBadRequest},
		{"PUT", "/service/runtime/executions/" + executionID + "/variables-async/x", "{", http.StatusBadRequest},
		{"POST", "/service/runtime/process-instances/" + instanceID + "/variables-async", "{", http.StatusBadRequest},
		{"PUT", "/service/runtime/process-instances/" + instanceID + "/variables-async/x", "{", http.StatusBadRequest},
		{"POST", "/service/runtime/executions/missing/variables-async", `[{"name":"x","value":1}]`, http.StatusNotFound},
		{"PUT", "/service/runtime/executions/missing/variables-async/x", `{"value":1}`, http.StatusNotFound},
		{"POST", "/service/runtime/process-instances/missing/variables-async", `[{"name":"x","value":1}]`, http.StatusNotFound},
		{"PUT", "/service/runtime/process-instances/missing/variables-async/x", `{"value":1}`, http.StatusNotFound},
	}
	for _, tc := range invalidBodies {
		request, _ := http.NewRequest(tc.method, server.URL+tc.path, bytes.NewBufferString(tc.body))
		request.Header.Set("Content-Type", "application/json")
		response, _ := http.DefaultClient.Do(request)
		if response.StatusCode != tc.want {
			t.Fatalf("%s %s 期望 %d 实际 %d", tc.method, tc.path, tc.want, response.StatusCode)
		}
		_ = response.Body.Close()
	}
}
