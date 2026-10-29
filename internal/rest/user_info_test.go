package rest

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"testing"
)

// putUserPicture 上传用户头像。
// @param t 测试上下文
// @param url 地址
// @param data 图片数据
// @param mimeType MIME 类型
// @return 响应
func putUserPicture(t *testing.T, url string, data []byte, mimeType string) *http.Response {
	t.Helper()
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	part, err := writer.CreatePart(textproto.MIMEHeader{"Content-Disposition": {`form-data; name="file"; filename="avatar.png"`}, "Content-Type": {mimeType}})
	if err != nil {
		t.Fatalf("创建表单失败: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("写入图片失败: %v", err)
	}
	if mimeType != "" {
		_ = writer.WriteField("mimeType", mimeType)
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

// TestREST_UserInfoAndPicture 验证用户信息与头像端点。
// @param t 测试上下文
func TestREST_UserInfoAndPicture(t *testing.T) {
	server, _ := newTestServer(t)
	response := postJSON(t, server.URL+"/service/identity/users", map[string]any{"id": "alice"})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("创建用户失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response, _ = http.Get(server.URL + "/service/identity/users/alice/info")
	var info []map[string]any
	_ = json.NewDecoder(response.Body).Decode(&info)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || len(info) != 0 {
		t.Fatalf("初始用户信息错误: %d %+v", response.StatusCode, info)
	}
	response = postJSON(t, server.URL+"/service/identity/users/alice/info", map[string]any{"key": "title", "value": "Engineer"})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("创建用户信息失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/identity/users/alice/info", map[string]any{"key": "title", "value": "Other"})
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("重复用户信息应 409: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/identity/users/alice/info", map[string]any{"value": "x"})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("缺少 key 应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response, _ = http.Get(server.URL + "/service/identity/users/alice/info")
	_ = json.NewDecoder(response.Body).Decode(&info)
	_ = response.Body.Close()
	if len(info) != 1 || info[0]["key"] != "title" || info[0]["value"] != nil || info[0]["url"] == "" {
		t.Fatalf("用户信息列表错误: %+v", info)
	}
	response, _ = http.Get(server.URL + "/service/identity/users/alice/info/title")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("用户信息详情失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	request, _ := http.NewRequest(http.MethodPut, server.URL+"/service/identity/users/alice/info/title", bytes.NewBufferString(`{"value":"Senior"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("更新用户信息失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/identity/users/alice/info/title", bytes.NewBufferString(`{"key":"other","value":"x"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("key 不一致应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/identity/users/alice/info/missing", bytes.NewBufferString(`{"value":"x"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失用户信息更新应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/identity/users/alice/info/title", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除用户信息失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/identity/users/alice/info/title", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("重复删除应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response, _ = http.Get(server.URL + "/service/identity/users/alice/picture")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("未上传头像应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	picture := []byte{0x89, 0x50, 0x4e, 0x47}
	response = putUserPicture(t, server.URL+"/service/identity/users/alice/picture", picture, "image/png")
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("上传头像失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/identity/users/alice/picture")
	data, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !bytes.Equal(data, picture) || response.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("头像读取错误: %d %s %v", response.StatusCode, response.Header.Get("Content-Type"), data)
	}
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/identity/users/alice/picture", bytes.NewBufferString("x"))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非 multipart 头像应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response, _ = http.Get(server.URL + "/service/identity/users/missing/info")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失用户信息应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/identity/users/missing/info", map[string]any{"key": "k", "value": "v"})
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失用户创建信息应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/identity/users/alice/info", map[string]any{"key": 1})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法用户信息请求应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = putEmptyMultipart(t, server.URL+"/service/identity/users/alice/picture")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("空头像 multipart 应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = putUserPicture(t, server.URL+"/service/identity/users/missing/picture", picture, "image/png")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失用户头像上传应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
