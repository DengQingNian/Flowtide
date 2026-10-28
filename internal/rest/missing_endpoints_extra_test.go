package rest

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestREST_SuspendedJobDeleteMissing 验证删除不存在的挂起作业返回 404。
// @param t 测试上下文
func TestREST_SuspendedJobDeleteMissing(t *testing.T) {
	server, _ := newTestServer(t)
	request, err := http.NewRequest(http.MethodDelete, server.URL+"/service/management/suspended-jobs/missing", nil)
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("删除请求失败: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失挂起作业应 404: %d", response.StatusCode)
	}
}

// TestREST_InstanceUserIdentityLinkMissing 验证缺失的用户身份链接返回 404。
// @param t 测试上下文
func TestREST_InstanceUserIdentityLinkMissing(t *testing.T) {
	server, _ := newTestServer(t)
	response, err := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(restProcess))
	if err != nil || response.StatusCode != http.StatusCreated {
		t.Fatalf("部署失败: %v %d", err, response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{"processDefinitionKey": "rest"})
	var instance struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&instance); err != nil {
		t.Fatalf("解析实例响应失败: %v", err)
	}
	_ = response.Body.Close()
	response, err = http.Get(server.URL + "/service/runtime/process-instances/" + instance.ID + "/identitylinks/users/missing/participant")
	if err != nil {
		t.Fatalf("查询身份链接失败: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失身份链接应 404: %d", response.StatusCode)
	}
}

// TestREST_DeploymentResourceMissing 验证已有部署下缺失资源返回 404。
// @param t 测试上下文
func TestREST_DeploymentResourceMissing(t *testing.T) {
	server, _ := newTestServer(t)
	response, err := http.Post(server.URL+"/service/repository/deployments?resourceName=process.bpmn20.xml", "application/xml", bytes.NewBufferString(restProcess))
	if err != nil || response.StatusCode != http.StatusCreated {
		t.Fatalf("部署失败: %v %d", err, response.StatusCode)
	}
	var deployment struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&deployment); err != nil {
		t.Fatalf("解析部署响应失败: %v", err)
	}
	_ = response.Body.Close()
	response, err = http.Get(server.URL + "/service/repository/deployments/" + deployment.ID + "/resources/missing.bpmn20.xml")
	if err != nil {
		t.Fatalf("查询资源元数据失败: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusNotFound {
		body := new(bytes.Buffer)
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("缺失资源元数据应 404: %d %s", response.StatusCode, body.String())
	}
}

// TestREST_JobActionInvalidJSON 验证非法动作请求体返回 400。
// @param t 测试上下文
func TestREST_JobActionInvalidJSON(t *testing.T) {
	server, _ := newTestServer(t)
	for _, path := range []string{
		"/service/management/deadletter-jobs/job",
		"/service/management/timer-jobs/job",
	} {
		response, err := http.Post(server.URL+path, "application/json", bytes.NewBufferString("{"))
		if err != nil {
			t.Fatalf("%s 请求失败: %v", path, err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s 非法请求体应 400: %d", path, response.StatusCode)
		}
	}
}

// TestDeploymentResourceMediaType 验证部署资源扩展名到媒体类型的映射。
// @param t 测试上下文
func TestDeploymentResourceMediaType(t *testing.T) {
	cases := map[string]string{
		"process.bpmn20.xml": "text/xml",
		"model.dmn":          "text/xml",
		"form.form":          "application/json",
		"view.json":          "application/json",
		"diagram.png":        "image/png",
		"photo.jpg":          "image/jpeg",
		"anim.gif":           "image/gif",
		"vector.svg":         "image/svg+xml",
		"bundle.zip":         "application/zip",
		"readme.txt":         "text/plain",
		"data.bin":           "application/octet-stream",
		"no-extension":       "application/octet-stream",
	}
	for name, expected := range cases {
		if actual := deploymentResourceMediaType(name); actual != expected {
			t.Fatalf("%s 媒体类型期望 %s 实际 %s", name, expected, actual)
		}
	}
}

// TestRequestBaseURL 验证代理协议与 TLS 场景的基础地址。
// @param t 测试上下文
func TestRequestBaseURL(t *testing.T) {
	plain := httptest.NewRequest(http.MethodGet, "http://example.com/path", nil)
	if actual := requestBaseURL(plain); actual != "http://example.com" {
		t.Fatalf("普通请求基础地址错误: %s", actual)
	}
	forwarded := httptest.NewRequest(http.MethodGet, "http://example.com/path", nil)
	forwarded.Header.Set("X-Forwarded-Proto", "https")
	if actual := requestBaseURL(forwarded); actual != "https://example.com" {
		t.Fatalf("代理请求基础地址错误: %s", actual)
	}
	secure := httptest.NewRequest(http.MethodGet, "https://example.com/path", nil)
	secure.TLS = &tls.ConnectionState{}
	if actual := requestBaseURL(secure); actual != "https://example.com" {
		t.Fatalf("TLS 请求基础地址错误: %s", actual)
	}
}
