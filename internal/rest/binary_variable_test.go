package rest

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
)

// multipartVariable 发送二进制变量 multipart 请求。
// @param t 测试上下文
// @param method 方法
// @param target 地址
// @param name 变量名
// @param data 内容
// @return 响应
func multipartVariable(t *testing.T, method, target, name string, data []byte) *http.Response {
	t.Helper()
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	if name != "" {
		_ = writer.WriteField("name", name)
	}
	part, err := writer.CreateFormFile("file", "variable.bin")
	if err != nil {
		t.Fatalf("创建表单失败: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}
	_ = writer.Close()
	request, err := http.NewRequest(method, target, &buffer)
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

// TestREST_BinaryVariables 验证二进制变量上传与下载。
// @param t 测试上下文
func TestREST_BinaryVariables(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	instanceID := startQueryInstance(t, server.URL, "rest", "bk1", "Order")
	executionID := executionIDFor(t, server.URL, instanceID)
	payload := []byte{0x00, 0x01, 0xfe, 0xff}

	response := multipartVariable(t, http.MethodPut, server.URL+"/service/runtime/executions/"+executionID+"/variables/avatar", "", payload)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("执行二进制变量上传失败: %d", response.StatusCode)
	}
	var variable map[string]any
	_ = json.NewDecoder(response.Body).Decode(&variable)
	_ = response.Body.Close()
	if variable["type"] != "binary" {
		t.Fatalf("变量类型错误: %+v", variable)
	}
	response, _ = http.Get(server.URL + "/service/runtime/executions/" + executionID + "/variables/avatar/data")
	data, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !bytes.Equal(data, payload) || response.Header.Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("执行二进制变量下载错误: %d %v", response.StatusCode, data)
	}

	response = multipartVariable(t, http.MethodPut, server.URL+"/service/runtime/process-instances/"+instanceID+"/variables/doc", "", []byte("pdf"))
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("实例二进制变量上传失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/process-instances/" + instanceID + "/variables/doc/data")
	data, _ = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if string(data) != "pdf" {
		t.Fatalf("实例二进制变量下载错误: %s", data)
	}

	response = multipartVariable(t, http.MethodPost, server.URL+"/service/runtime/process-instances/"+instanceID+"/variables", "blob", []byte("blob-data"))
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("实例二进制变量创建失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/process-instances/" + instanceID + "/variables/blob/data")
	data, _ = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if string(data) != "blob-data" {
		t.Fatalf("实例二进制变量创建下载错误: %s", data)
	}

	task := createRESTTask(t, server.URL, map[string]any{"name": "Task"}, "")
	taskID, _ := task["id"].(string)
	response = multipartVariable(t, http.MethodPut, server.URL+"/service/runtime/tasks/"+taskID+"/variables/note", "", []byte("note"))
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("任务二进制变量上传失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response = multipartVariable(t, http.MethodPut, server.URL+"/service/runtime/process-instances/"+instanceID+"/variables/empty", "", nil)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("空文件应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = multipartVariable(t, http.MethodPost, server.URL+"/service/runtime/process-instances/"+instanceID+"/variables", "", []byte("x"))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("缺少变量名应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
