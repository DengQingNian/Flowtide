package rest

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
)

// putEmptyMultipart 发送不含文件的 multipart 请求。
// @param t 测试上下文
// @param url 地址
// @return 响应
func putEmptyMultipart(t *testing.T, url string) *http.Response {
	t.Helper()
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	_ = writer.Close()
	request, err := http.NewRequest(http.MethodPut, url, &buffer)
	if err != nil {
		t.Fatalf("创建请求失败: %v", err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	return response
}

// TestModelSortKey 验证模型排序字段。
// @param t 测试上下文
func TestModelSortKey(t *testing.T) {
	item := api.Model{ID: "m", Category: "c", CreateTime: time.Now(), Key: "k", LastUpdateTime: time.Now(), Name: "n", Version: 1, TenantID: "t"}
	for _, field := range []string{"id", "category", "createTime", "key", "lastUpdateTime", "name", "version", "tenantId"} {
		key, ok := modelSortKey(field)
		if !ok {
			t.Fatalf("模型排序字段缺失: %s", field)
		}
		_ = key(item)
	}
	if _, ok := modelSortKey("missing"); ok {
		t.Fatalf("未知模型排序字段应不支持")
	}
}

// putModelSource 以 multipart 上传模型源。
// @param t 测试上下文
// @param url 地址
// @param content 文件内容
// @return 响应
func putModelSource(t *testing.T, url string, content []byte) *http.Response {
	t.Helper()
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	part, err := writer.CreateFormFile("file", "source.bpmn")
	if err != nil {
		t.Fatalf("创建表单失败: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}
	_ = writer.Close()
	request, err := http.NewRequest(http.MethodPut, url, &buffer)
	if err != nil {
		t.Fatalf("创建请求失败: %v", err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("上传请求失败: %v", err)
	}
	return response
}

// TestREST_ModelLifecycle 验证模型 CRUD、源码与列表过滤。
// @param t 测试上下文
func TestREST_ModelLifecycle(t *testing.T) {
	server, _ := newTestServer(t, "t1")
	response := postQuery(t, server.URL, "/service/repository/models", map[string]any{
		"name": "Order Model", "key": "orderModel", "category": "sales", "metaInfo": "{}",
	}, "t1")
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("创建模型失败: %d", response.StatusCode)
	}
	var model map[string]any
	_ = json.NewDecoder(response.Body).Decode(&model)
	_ = response.Body.Close()
	modelID, _ := model["id"].(string)
	if modelID == "" || model["version"].(float64) != 1 {
		t.Fatalf("模型字段错误: %+v", model)
	}

	response, _ = http.Get(server.URL + "/service/repository/models/" + modelID)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("查询模型失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	var page queryPageResult[map[string]any]
	for _, query := range []string{
		"nameLike=%25order%25&key=orderModel&version=1",
		"category=sales",
		"categoryLike=%25sal%25",
		"categoryNotEquals=other",
		"name=Order+Model",
		"tenantIdLike=%25t1%25",
		"deployed=false",
	} {
		response, _ = http.Get(server.URL + "/service/repository/models?" + query)
		page = decodeQueryPage[map[string]any](t, response)
		if response.StatusCode != http.StatusOK || page.Total != 1 {
			t.Fatalf("模型过滤失败 %s: %d %+v", query, response.StatusCode, page)
		}
	}
	for _, field := range []string{"id", "category", "createTime", "key", "lastUpdateTime", "name", "version", "tenantId"} {
		response, _ = http.Get(server.URL + "/service/repository/models?sort=" + field)
		page = decodeQueryPage[map[string]any](t, response)
		if response.StatusCode != http.StatusOK || page.Total != 1 {
			t.Fatalf("模型排序失败 %s: %d %+v", field, response.StatusCode, page)
		}
	}
	response, _ = http.Get(server.URL + "/service/repository/models?deployed=true")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("已部署过滤失败: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/repository/models", map[string]any{
		"name": "Order Model v2", "key": "orderModel", "version": 2,
	}, "t1")
	var modelV2 map[string]any
	_ = json.NewDecoder(response.Body).Decode(&modelV2)
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/repository/models?latestVersion=true")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 || page.Data[0]["version"].(float64) != 2 {
		t.Fatalf("最新版本过滤失败: %+v", page)
	}

	updateBody, _ := json.Marshal(map[string]any{"name": "Renamed", "category": nil, "version": 3, "deploymentId": "d1"})
	request, _ := http.NewRequest(http.MethodPut, server.URL+"/service/repository/models/"+modelID, bytes.NewReader(updateBody))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	var updated map[string]any
	_ = json.NewDecoder(response.Body).Decode(&updated)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || updated["name"] != "Renamed" || updated["category"] != nil || updated["version"].(float64) != 3 {
		t.Fatalf("更新模型失败: %d %+v", response.StatusCode, updated)
	}

	response = putModelSource(t, server.URL+"/service/repository/models/"+modelID+"/source", []byte("<xml/>"))
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("上传源码失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/repository/models/" + modelID + "/source")
	var source bytes.Buffer
	_, _ = source.ReadFrom(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || source.String() != "<xml/>" || response.Header.Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("读取源码失败: %d %s", response.StatusCode, source.String())
	}

	response = putModelSource(t, server.URL+"/service/repository/models/"+modelID+"/source-extra", []byte("{}"))
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("上传扩展源码失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/repository/models/" + modelID + "/source-extra")
	var extra bytes.Buffer
	_, _ = extra.ReadFrom(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || extra.String() != "{}" {
		t.Fatalf("读取扩展源码失败: %d %s", response.StatusCode, extra.String())
	}

	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/repository/models/"+modelID, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除模型失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/repository/models/" + modelID)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("删除后模型应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_ModelErrors 验证模型端点错误与租户隔离。
// @param t 测试上下文
func TestREST_ModelErrors(t *testing.T) {
	server, _ := newTestServer(t)
	response := postQuery(t, server.URL, "/service/repository/models", map[string]any{"name": "x"}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("缺少 Key 应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postQuery(t, server.URL, "/service/repository/models", map[string]any{"name": "x", "key": "k"}, "t1")
	var model map[string]any
	_ = json.NewDecoder(response.Body).Decode(&model)
	_ = response.Body.Close()
	modelID, _ := model["id"].(string)

	request, _ := http.NewRequest(http.MethodGet, server.URL+"/service/repository/models/"+modelID, nil)
	request.Header.Set("X-Tenant-Id", "t2")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("跨租户查询应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response, _ = http.Get(server.URL + "/service/repository/models?version=bad")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法版本应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/repository/models?sort=missing")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法排序应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/repository/models?order=sideways")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法排序方向应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/repository/models/missing/source")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失模型源码应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postQuery(t, server.URL, "/service/repository/models", map[string]any{"name": "x", "key": "k", "tenantId": "t1"}, "t2")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("跨租户创建应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/repository/models/missing", bytes.NewBufferString(`{"name":"x"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("更新缺失模型应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/repository/models/missing", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("删除缺失模型应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/repository/models/"+modelID, bytes.NewBufferString("{"))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法更新体应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = putEmptyMultipart(t, server.URL+"/service/repository/models/"+modelID+"/source")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("空 multipart 应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/repository/models/"+modelID+"/source", strings.NewReader("{}"))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非 multipart 上传应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/repository/models/"+modelID, bytes.NewBufferString(`{"version":"x"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法版本更新应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
