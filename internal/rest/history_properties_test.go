package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/engine"
)

// TestREST_HistoricQueryEndpoints 验证历史变量与明细查询端点。
// @param t 测试上下文
func TestREST_HistoricQueryEndpoints(t *testing.T) {
	server, _ := newHistoryTestServer(t, engine.HistoryFull)
	deployREST(t, server)
	instanceID := startQueryInstance(t, server.URL, "rest", "bk1", "Order")

	response := postQuery(t, server.URL, "/service/query/historic-variable-instances", map[string]any{"processInstanceId": instanceID}, "")
	page := decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total == 0 {
		t.Fatalf("历史变量查询失败: %d %+v", response.StatusCode, page)
	}
	for _, query := range []map[string]any{
		{"processInstanceId": instanceID, "variableName": "initiator"},
		{"processInstanceId": instanceID, "nameLike": "%iti%"},
		{"processInstanceId": instanceID, "variableType": "string"},
	} {
		response = postQuery(t, server.URL, "/service/query/historic-variable-instances", query, "")
		page = decodeQueryPage[map[string]any](t, response)
		if page.Total == 0 {
			t.Fatalf("历史变量过滤失败 %+v: %+v", query, page)
		}
	}
	for _, field := range []string{"id", "name", "type", "createTime", "lastUpdatedTime"} {
		response = postQuery(t, server.URL, "/service/query/historic-variable-instances", map[string]any{"sort": field}, "")
		page = decodeQueryPage[map[string]any](t, response)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("历史变量排序失败 %s: %d", field, response.StatusCode)
		}
	}
	response = postQuery(t, server.URL, "/service/query/historic-variable-instances", map[string]any{"sort": "missing"}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法排序应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	for _, body := range []map[string]any{{"order": "sideways"}, {"start": "x"}} {
		response = postQuery(t, server.URL, "/service/query/historic-variable-instances", body, "")
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("非法参数应 400 %+v: %d", body, response.StatusCode)
		}
		_ = response.Body.Close()
	}

	response = postQuery(t, server.URL, "/service/query/historic-detail", map[string]any{"processInstanceId": instanceID}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total == 0 {
		t.Fatalf("历史明细查询失败: %d %+v", response.StatusCode, page)
	}
	for _, query := range []map[string]any{
		{"processInstanceId": instanceID, "variableName": "initiator"},
		{"processInstanceId": instanceID, "nameLike": "%iti%"},
		{"processInstanceId": instanceID, "timeAfter": "2020-01-01T00:00:00Z"},
		{"processInstanceId": instanceID, "timeBefore": "2040-01-01T00:00:00Z"},
	} {
		response = postQuery(t, server.URL, "/service/query/historic-detail", query, "")
		page = decodeQueryPage[map[string]any](t, response)
		if page.Total == 0 {
			t.Fatalf("历史明细过滤失败 %+v: %+v", query, page)
		}
	}
	for _, field := range []string{"id", "name", "variableType", "time"} {
		response = postQuery(t, server.URL, "/service/query/historic-detail", map[string]any{"sort": field}, "")
		page = decodeQueryPage[map[string]any](t, response)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("历史明细排序失败 %s: %d", field, response.StatusCode)
		}
	}
	for _, body := range []map[string]any{{"sort": "missing"}, {"order": "sideways"}, {"start": "x"}, {"timeAfter": "bad"}} {
		response = postQuery(t, server.URL, "/service/query/historic-detail", body, "")
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("非法参数应 400 %+v: %d", body, response.StatusCode)
		}
		_ = response.Body.Close()
	}
	invalid, _ := http.NewRequest(http.MethodPost, server.URL+"/service/query/historic-detail", bytes.NewBufferString("{"))
	invalid.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(invalid)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法查询体应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response = postQuery(t, server.URL, "/service/runtime/process-instances", map[string]any{
		"processDefinitionKey": "rest",
		"variables":            []map[string]any{{"name": "initiator", "value": "alice"}},
	}, "tenant-a")
	var instance struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()
	response = postQuery(t, server.URL, "/service/query/historic-variable-instances", map[string]any{"processInstanceId": instance.ID}, "tenant-b")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("跨租户历史变量应为空: %+v", page)
	}
}

// TestHistoricQuerySortKeys 验证历史查询排序字段。
// @param t 测试上下文
func TestHistoricQuerySortKeys(t *testing.T) {
	now := time.Now()
	variable := api.HistoricVariableInstance{ID: "v", Name: "n", Type: "string", CreateTime: now, LastUpdatedTime: now}
	for _, field := range []string{"id", "name", "type", "createTime", "lastUpdatedTime"} {
		key, ok := historicVariableSortKey(field)
		if !ok {
			t.Fatalf("历史变量排序字段缺失: %s", field)
		}
		_ = key(variable)
	}
	if _, ok := historicVariableSortKey("missing"); ok {
		t.Fatalf("未知历史变量排序字段应不支持")
	}
	detail := api.HistoricDetail{ID: "d", Name: "n", VariableType: "string", Time: now}
	for _, field := range []string{"id", "name", "variableType", "time"} {
		key, ok := historicDetailSortKey(field)
		if !ok {
			t.Fatalf("历史明细排序字段缺失: %s", field)
		}
		_ = key(detail)
	}
	if _, ok := historicDetailSortKey("missing"); ok {
		t.Fatalf("未知历史明细排序字段应不支持")
	}
}

// TestREST_HistoricQueryClosedEngine 验证关闭引擎后的历史查询错误。
// @param t 测试上下文
func TestREST_HistoricQueryClosedEngine(t *testing.T) {
	app, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	server := httptest.NewServer(New(app).Handler())
	defer server.Close()
	_ = app.Close(context.Background())
	for _, path := range []string{"/service/query/historic-variable-instances", "/service/query/historic-detail"} {
		response := postQuery(t, server.URL, path, map[string]any{}, "")
		if response.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("%s 关闭后应 503: %d", path, response.StatusCode)
		}
		_ = response.Body.Close()
	}
}

// TestREST_EngineProperties 验证自定义引擎属性端点。
// @param t 测试上下文
func TestREST_EngineProperties(t *testing.T) {
	server, _ := newTestServer(t)
	response, _ := http.Get(server.URL + "/service/management/engine-properties")
	var properties map[string]string
	_ = json.NewDecoder(response.Body).Decode(&properties)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || properties["schema.version"] == "" {
		t.Fatalf("引擎属性错误: %d %+v", response.StatusCode, properties)
	}

	response = postQuery(t, server.URL, "/service/management/engine-properties", map[string]any{"name": "feature.x", "value": "1"}, "")
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("创建属性失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postQuery(t, server.URL, "/service/management/engine-properties", map[string]any{"name": "feature.x", "value": "2"}, "")
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("重复属性应 409: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postQuery(t, server.URL, "/service/management/engine-properties", map[string]any{"name": "schema.version", "value": "x"}, "")
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("内置属性创建应 409: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postQuery(t, server.URL, "/service/management/engine-properties", map[string]any{"value": "x"}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("空属性名应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	for _, body := range []string{"{", `{"name":1,"value":"x"}`} {
		request, _ := http.NewRequest(http.MethodPost, server.URL+"/service/management/engine-properties", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		response, _ = http.DefaultClient.Do(request)
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("非法属性请求应 400: %d", response.StatusCode)
		}
		_ = response.Body.Close()
	}

	response, _ = http.Get(server.URL + "/service/management/engine-properties")
	_ = json.NewDecoder(response.Body).Decode(&properties)
	_ = response.Body.Close()
	if properties["feature.x"] != "1" {
		t.Fatalf("属性未生效: %+v", properties)
	}

	request, _ := http.NewRequest(http.MethodPut, server.URL+"/service/management/engine-properties/feature.x", bytes.NewBufferString(`{"value":"2"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("更新属性失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/management/engine-properties/feature.x", bytes.NewBufferString(`{"name":"other","value":"2"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("属性名不一致应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/management/engine-properties/missing", bytes.NewBufferString(`{"value":"2"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失属性更新应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/management/engine-properties/schema.version", bytes.NewBufferString(`{"value":"2"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("内置属性更新应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/management/engine-properties/feature.x", bytes.NewBufferString("{"))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法更新请求应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/management/engine-properties/feature.x", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除属性失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/management/engine-properties/feature.x", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("重复删除应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/management/engine-properties/schema.version", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("内置属性删除应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
