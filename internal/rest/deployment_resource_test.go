package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// deploymentResourceResponseBody 是资源元数据响应。
type deploymentResourceResponseBody struct {
	ID         string `json:"id"`
	URL        string `json:"url"`
	ContentURL string `json:"contentUrl"`
	MediaType  string `json:"mediaType"`
	Type       string `json:"type"`
}

// TestREST_DeploymentResourceMetadata 验证资源元数据、原始内容与媒体类型。
// @param t 测试上下文
func TestREST_DeploymentResourceMetadata(t *testing.T) {
	server, _ := newTestServer(t)
	cases := []struct {
		name      string
		body      string
		mediaType string
		kind      string
	}{
		{"process.bpmn20.xml", restProcess, "text/xml", "processDefinition"},
		{"form.form", `{"key":"form1","name":"Form"}`, "application/json", "resource"},
		{"data.bin", "binary", "application/octet-stream", "resource"},
		{"diagram.png", "png", "image/png", "processImage"},
	}
	for _, tc := range cases {
		response, _ := http.Post(server.URL+"/service/repository/deployments?resourceName="+tc.name, "application/octet-stream", bytes.NewBufferString(tc.body))
		if response.StatusCode != http.StatusCreated {
			t.Fatalf("部署 %s 失败: %d", tc.name, response.StatusCode)
		}
		var deployment struct {
			ID string `json:"id"`
		}
		_ = json.NewDecoder(response.Body).Decode(&deployment)
		_ = response.Body.Close()

		response, _ = http.Get(server.URL + "/service/repository/deployments/" + deployment.ID + "/resources")
		var resources []deploymentResourceResponseBody
		_ = json.NewDecoder(response.Body).Decode(&resources)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK || len(resources) != 1 {
			t.Fatalf("%s 资源列表错误: %d %+v", tc.name, response.StatusCode, resources)
		}
		resource := resources[0]
		if resource.ID != tc.name || resource.MediaType != tc.mediaType || resource.Type != tc.kind {
			t.Fatalf("%s 元数据错误: %+v", tc.name, resource)
		}
		if !strings.HasPrefix(resource.URL, server.URL+"/service/repository/deployments/"+deployment.ID+"/resources/") ||
			!strings.HasPrefix(resource.ContentURL, server.URL+"/service/repository/deployments/"+deployment.ID+"/resourcedata/") {
			t.Fatalf("%s 资源地址错误: %+v", tc.name, resource)
		}

		response, _ = http.Get(server.URL + "/service/repository/deployments/" + deployment.ID + "/resources/" + tc.name)
		var single deploymentResourceResponseBody
		_ = json.NewDecoder(response.Body).Decode(&single)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK || single.ID != tc.name || single.MediaType != tc.mediaType {
			t.Fatalf("%s 单资源元数据错误: %d %+v", tc.name, response.StatusCode, single)
		}

		response, _ = http.Get(server.URL + "/service/repository/deployments/" + deployment.ID + "/resourcedata/" + tc.name)
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK || body.String() != tc.body {
			t.Fatalf("%s 原始内容错误: %d %q", tc.name, response.StatusCode, body.String())
		}
		if response.Header.Get("Content-Type") != tc.mediaType {
			t.Fatalf("%s Content-Type 期望 %s 实际 %s", tc.name, tc.mediaType, response.Header.Get("Content-Type"))
		}
		if !strings.Contains(response.Header.Get("Content-Disposition"), "attachment") {
			t.Fatalf("%s 缺少 Content-Disposition: %s", tc.name, response.Header.Get("Content-Disposition"))
		}
	}
}

// TestREST_DeploymentResourceEncodedSlash 验证 URL 编码斜杠资源名。
// @param t 测试上下文
func TestREST_DeploymentResourceEncodedSlash(t *testing.T) {
	server, _ := newTestServer(t)
	response, _ := http.Post(server.URL+"/service/repository/deployments?resourceName=forms/nested.bpmn20.xml", "application/xml", bytes.NewBufferString(restProcess))
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("嵌套资源部署失败: %d", response.StatusCode)
	}
	var deployment struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(response.Body).Decode(&deployment)
	_ = response.Body.Close()

	encoded := url.PathEscape("forms/nested.bpmn20.xml")
	response, _ = http.Get(server.URL + "/service/repository/deployments/" + deployment.ID + "/resourcedata/" + encoded)
	var body bytes.Buffer
	_, _ = body.ReadFrom(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(body.String(), "definitions") {
		t.Fatalf("编码斜杠资源读取失败: %d %q", response.StatusCode, body.String())
	}
	if response.Header.Get("Content-Type") != "text/xml" {
		t.Fatalf("编码斜杠媒体类型错误: %s", response.Header.Get("Content-Type"))
	}

	response, _ = http.Get(server.URL + "/service/repository/deployments/" + deployment.ID + "/resources/" + encoded)
	var resource deploymentResourceResponseBody
	_ = json.NewDecoder(response.Body).Decode(&resource)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || resource.ID != "forms/nested.bpmn20.xml" || resource.Type != "processDefinition" {
		t.Fatalf("编码斜杠元数据错误: %d %+v", response.StatusCode, resource)
	}
	if !strings.Contains(resource.ContentURL, "forms%2Fnested.bpmn20.xml") {
		t.Fatalf("contentUrl 未编码资源名: %+v", resource)
	}

	response, _ = http.Get(server.URL + "/service/repository/deployments/" + deployment.ID + "/resourcedata/missing.bpmn20.xml")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失资源应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/repository/deployments/missing/resources")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失部署应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
