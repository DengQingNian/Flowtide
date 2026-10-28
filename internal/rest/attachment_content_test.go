package rest

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
)

// postEmptyMultipart 发送不含文件的 POST multipart 请求。
// @param t 测试上下文
// @param url 地址
// @return 响应
func postEmptyMultipart(t *testing.T, url string) *http.Response {
	t.Helper()
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	_ = writer.Close()
	request, err := http.NewRequest(http.MethodPost, url, &buffer)
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

// postAttachmentFile 以 multipart 上传附件。
// @param t 测试上下文
// @param url 地址
// @param fields 表单字段
// @param data 文件内容
// @return 响应
func postAttachmentFile(t *testing.T, url string, fields map[string]string, data []byte) *http.Response {
	t.Helper()
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	for key, value := range fields {
		_ = writer.WriteField(key, value)
	}
	part, err := writer.CreateFormFile("file", "attachment.txt")
	if err != nil {
		t.Fatalf("创建表单失败: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}
	_ = writer.Close()
	request, err := http.NewRequest(http.MethodPost, url, &buffer)
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

// TestREST_AttachmentContent 验证附件内容上传与下载。
// @param t 测试上下文
func TestREST_AttachmentContent(t *testing.T) {
	server, _ := newTestServer(t)
	task := createRESTTask(t, server.URL, map[string]any{"name": "Task"}, "")
	taskID, _ := task["id"].(string)
	other := createRESTTask(t, server.URL, map[string]any{"name": "Other"}, "")
	otherID, _ := other["id"].(string)

	response := postAttachmentFile(t, server.URL+"/service/runtime/tasks/"+taskID+"/attachments", map[string]string{
		"name": "file.txt", "description": "desc", "type": "text/plain",
	}, []byte("hello"))
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("上传附件失败: %d", response.StatusCode)
	}
	var attachment map[string]any
	_ = json.NewDecoder(response.Body).Decode(&attachment)
	_ = response.Body.Close()
	attachmentID, _ := attachment["id"].(string)
	contentURL, _ := attachment["contentUrl"].(string)
	if attachmentID == "" || contentURL == "" {
		t.Fatalf("附件 contentUrl 缺失: %+v", attachment)
	}
	response, _ = http.Get(server.URL + contentURL)
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || string(body) != "hello" || response.Header.Get("Content-Type") != "text/plain" {
		t.Fatalf("附件内容下载错误: %d %s %s", response.StatusCode, response.Header.Get("Content-Type"), body)
	}
	if response.Header.Get("Content-Disposition") == "" {
		t.Fatalf("附件下载缺少 Content-Disposition")
	}

	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/attachments")
	var page queryPageResult[map[string]any]
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	if len(page.Data) != 1 || page.Data[0]["contentUrl"] == "" {
		t.Fatalf("附件列表 contentUrl 缺失: %+v", page)
	}
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + otherID + "/attachments/" + attachmentID + "/content")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("任务不匹配应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID+"/attachments", map[string]any{
		"name": "link", "url": "http://example.com",
	})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("外部链接附件失败: %d", response.StatusCode)
	}
	var linkAttachment map[string]any
	_ = json.NewDecoder(response.Body).Decode(&linkAttachment)
	_ = response.Body.Close()
	if linkAttachment["contentUrl"] != nil {
		t.Fatalf("外部链接附件不应有 contentUrl: %+v", linkAttachment)
	}
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/attachments/" + linkAttachment["id"].(string) + "/content")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("无内容附件应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/attachments/missing/content")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失附件内容应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postEmptyMultipart(t, server.URL+"/service/runtime/tasks/"+taskID+"/attachments")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("空附件 multipart 应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postAttachmentFile(t, server.URL+"/service/runtime/tasks/"+taskID+"/attachments", map[string]string{"type": "text/plain"}, []byte("x"))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("缺少附件名应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postAttachmentFile(t, server.URL+"/service/runtime/tasks/"+taskID+"/attachments", map[string]string{"name": "empty.txt"}, []byte{})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("空附件内容应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
