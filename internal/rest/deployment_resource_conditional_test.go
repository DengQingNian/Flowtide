package rest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// deploySingleResource 部署单个资源并返回部署 ID。
// @param t 测试上下文
// @param server 测试服务
// @param name 资源名
// @param body 资源内容
// @return 部署 ID
func deploySingleResource(t *testing.T, server *httptest.Server, name, body string) string {
	t.Helper()
	response, err := http.Post(
		server.URL+"/service/repository/deployments?resourceName="+url.QueryEscape(name),
		"application/octet-stream",
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatalf("部署资源 %s 失败: %v", name, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("部署资源 %s 期望 201 实际 %d", name, response.StatusCode)
	}
	var deployment struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&deployment); err != nil {
		t.Fatalf("解析部署响应失败: %v", err)
	}
	if deployment.ID == "" {
		t.Fatal("部署响应缺少 ID")
	}
	return deployment.ID
}

// httpGetWithHeaders 发起 GET 请求并读取完整响应体。
// @param t 测试上下文
// @param target 请求地址
// @param headers 请求头
// @return HTTP 响应
// @return 响应体
func httpGetWithHeaders(t *testing.T, target string, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("请求 %s 失败: %v", target, err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("读取响应体失败: %v", err)
	}
	return response, body
}

// isQuotedSHA256ETag 判断 ETag 是否为双引号包裹的 SHA-256 小写十六进制。
// @param etag ETag 值
// @return 是否符合格式
func isQuotedSHA256ETag(etag string) bool {
	if len(etag) != 66 || etag[0] != '"' || etag[len(etag)-1] != '"' {
		return false
	}
	digest := etag[1:65]
	if strings.ToLower(digest) != digest {
		return false
	}
	decoded, err := hex.DecodeString(digest)
	return err == nil && len(decoded) == sha256.Size
}

// requestWithHeaders 构造带请求头的 GET 请求。
// @param headers 请求头
// @return 请求
func requestWithHeaders(headers map[string]string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/service/repository/deployments/d1/resources", nil)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	return request
}

// TestREST_DeploymentResourceConditionalGet 验证资源元数据与原始内容的条件缓存。
// @param t 测试上下文
func TestREST_DeploymentResourceConditionalGet(t *testing.T) {
	server, app := newTestServer(t)
	const resourceName = "conditional.bpmn20.xml"
	deploymentID := deploySingleResource(t, server, resourceName, restProcess)

	deployment, err := app.RepositoryService().GetDeployment(context.Background(), deploymentID)
	if err != nil {
		t.Fatalf("查询部署失败: %v", err)
	}
	lastModified := deployment.DeployTime.UTC().Format(http.TimeFormat)
	sum := sha256.Sum256([]byte(restProcess))
	expectedETag := `"` + hex.EncodeToString(sum[:]) + `"`

	targets := []struct {
		name            string
		url             string
		body            string
		contentTypePart string
	}{
		{
			name:            "元数据",
			url:             server.URL + "/service/repository/deployments/" + deploymentID + "/resources/" + url.PathEscape(resourceName),
			contentTypePart: "application/json",
		},
		{
			name:            "原始内容",
			url:             server.URL + "/service/repository/deployments/" + deploymentID + "/resourcedata/" + url.PathEscape(resourceName),
			body:            restProcess,
			contentTypePart: "text/xml",
		},
	}
	for _, target := range targets {
		response, body := httpGetWithHeaders(t, target.url, nil)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("%s 首次请求期望 200 实际 %d", target.name, response.StatusCode)
		}
		if got := response.Header.Get("ETag"); got != expectedETag {
			t.Fatalf("%s ETag 期望 %s 实际 %s", target.name, expectedETag, got)
		}
		if got := response.Header.Get("Last-Modified"); got != lastModified {
			t.Fatalf("%s Last-Modified 期望 %s 实际 %s", target.name, lastModified, got)
		}
		if got := response.Header.Get("Cache-Control"); got != "no-cache" {
			t.Fatalf("%s Cache-Control 期望 no-cache 实际 %s", target.name, got)
		}
		if !strings.Contains(response.Header.Get("Content-Type"), target.contentTypePart) {
			t.Fatalf("%s Content-Type 期望包含 %s 实际 %s", target.name, target.contentTypePart, response.Header.Get("Content-Type"))
		}
		if target.body != "" && string(body) != target.body {
			t.Fatalf("%s 响应体不符: %q", target.name, string(body))
		}
		if target.body == "" && !strings.Contains(string(body), resourceName) {
			t.Fatalf("%s 元数据缺少资源名: %s", target.name, string(body))
		}

		cases := []struct {
			name    string
			headers map[string]string
			want    int
		}{
			{"相同 ETag", map[string]string{"If-None-Match": expectedETag}, http.StatusNotModified},
			{"不同 ETag", map[string]string{"If-None-Match": `"other"`}, http.StatusOK},
			{"多值 ETag", map[string]string{"If-None-Match": `"other", ` + expectedETag}, http.StatusNotModified},
			{"星号 ETag", map[string]string{"If-None-Match": "*"}, http.StatusNotModified},
			{"弱 ETag", map[string]string{"If-None-Match": "W/" + expectedETag}, http.StatusNotModified},
			{"弱 ETag 不匹配", map[string]string{"If-None-Match": `W/"other"`}, http.StatusOK},
			{"修改时间相等", map[string]string{"If-Modified-Since": lastModified}, http.StatusNotModified},
			{"修改时间更早", map[string]string{"If-Modified-Since": deployment.DeployTime.Add(-time.Hour).UTC().Format(http.TimeFormat)}, http.StatusOK},
			{"修改时间更晚", map[string]string{"If-Modified-Since": deployment.DeployTime.Add(time.Hour).UTC().Format(http.TimeFormat)}, http.StatusNotModified},
			{"修改时间非法", map[string]string{"If-Modified-Since": "not-a-date"}, http.StatusOK},
			{"ETag 优先于修改时间", map[string]string{"If-None-Match": `"other"`, "If-Modified-Since": lastModified}, http.StatusOK},
		}
		for _, tc := range cases {
			conditional, conditionalBody := httpGetWithHeaders(t, target.url, tc.headers)
			if conditional.StatusCode != tc.want {
				t.Fatalf("%s %s 期望 %d 实际 %d", target.name, tc.name, tc.want, conditional.StatusCode)
			}
			if tc.want == http.StatusNotModified {
				if len(conditionalBody) != 0 {
					t.Fatalf("%s %s 304 响应不应有 body: %q", target.name, tc.name, string(conditionalBody))
				}
				if got := conditional.Header.Get("ETag"); got != expectedETag {
					t.Fatalf("%s %s 304 丢失 ETag: %s", target.name, tc.name, got)
				}
				if got := conditional.Header.Get("Last-Modified"); got != lastModified {
					t.Fatalf("%s %s 304 丢失 Last-Modified: %s", target.name, tc.name, got)
				}
				if got := conditional.Header.Get("Cache-Control"); got != "no-cache" {
					t.Fatalf("%s %s 304 丢失 Cache-Control: %s", target.name, tc.name, got)
				}
				continue
			}
			if target.body != "" && string(conditionalBody) != target.body {
				t.Fatalf("%s %s 响应体不符: %q", target.name, tc.name, string(conditionalBody))
			}
			if target.body == "" && len(conditionalBody) == 0 {
				t.Fatalf("%s %s 200 响应体为空", target.name, tc.name)
			}
		}
	}

	missingTargets := []string{
		server.URL + "/service/repository/deployments/" + deploymentID + "/resources/missing.bpmn20.xml",
		server.URL + "/service/repository/deployments/" + deploymentID + "/resourcedata/missing.bpmn20.xml",
		server.URL + "/service/repository/deployments/missing/resources/" + resourceName,
		server.URL + "/service/repository/deployments/missing/resourcedata/" + resourceName,
	}
	for _, missing := range missingTargets {
		response, _ := httpGetWithHeaders(t, missing, map[string]string{"If-None-Match": "*"})
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("缺失目标 %s 期望 404 实际 %d", missing, response.StatusCode)
		}
	}
}

// TestREST_DeploymentResourceEncodedSlashConditional 验证编码斜杠资源的条件缓存。
// @param t 测试上下文
func TestREST_DeploymentResourceEncodedSlashConditional(t *testing.T) {
	server, _ := newTestServer(t)
	const resourceName = "forms/nested-conditional.bpmn20.xml"
	deploymentID := deploySingleResource(t, server, resourceName, restProcess)
	encoded := url.PathEscape(resourceName)

	for _, target := range []string{
		server.URL + "/service/repository/deployments/" + deploymentID + "/resources/" + encoded,
		server.URL + "/service/repository/deployments/" + deploymentID + "/resourcedata/" + encoded,
	} {
		response, _ := httpGetWithHeaders(t, target, nil)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("编码斜杠首次请求期望 200 实际 %d", response.StatusCode)
		}
		etag := response.Header.Get("ETag")
		if !isQuotedSHA256ETag(etag) {
			t.Fatalf("编码斜杠 ETag 格式错误: %q", etag)
		}
		conditional, body := httpGetWithHeaders(t, target, map[string]string{"If-None-Match": etag})
		if conditional.StatusCode != http.StatusNotModified {
			t.Fatalf("编码斜杠条件请求期望 304 实际 %d", conditional.StatusCode)
		}
		if len(body) != 0 {
			t.Fatalf("编码斜杠 304 响应不应有 body: %q", string(body))
		}
		if conditional.Header.Get("ETag") != etag {
			t.Fatalf("编码斜杠 304 丢失 ETag")
		}
	}
}

// TestREST_DeploymentResourcesConditionalList 验证资源列表 ETag 与条件请求。
// @param t 测试上下文
func TestREST_DeploymentResourcesConditionalList(t *testing.T) {
	server, _ := newTestServer(t)
	firstID := deploySingleResource(t, server, "list.bpmn20.xml", restProcess)
	listURL := server.URL + "/service/repository/deployments/" + firstID + "/resources"

	response, body := httpGetWithHeaders(t, listURL, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("资源列表期望 200 实际 %d", response.StatusCode)
	}
	if !json.Valid(body) {
		t.Fatalf("资源列表不是合法 JSON: %s", string(body))
	}
	etag := response.Header.Get("ETag")
	if !isQuotedSHA256ETag(etag) {
		t.Fatalf("资源列表 ETag 格式错误: %q", etag)
	}
	if got := response.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("资源列表 Cache-Control 期望 no-cache 实际 %s", got)
	}

	again, _ := httpGetWithHeaders(t, listURL, nil)
	if again.Header.Get("ETag") != etag {
		t.Fatalf("资源列表 ETag 不稳定: %s != %s", again.Header.Get("ETag"), etag)
	}

	cases := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"相同 ETag", map[string]string{"If-None-Match": etag}, http.StatusNotModified},
		{"弱 ETag", map[string]string{"If-None-Match": "W/" + etag}, http.StatusNotModified},
		{"多值 ETag", map[string]string{"If-None-Match": `"other", ` + etag}, http.StatusNotModified},
		{"星号 ETag", map[string]string{"If-None-Match": "*"}, http.StatusNotModified},
		{"不同 ETag", map[string]string{"If-None-Match": `"other"`}, http.StatusOK},
	}
	for _, tc := range cases {
		conditional, conditionalBody := httpGetWithHeaders(t, listURL, tc.headers)
		if conditional.StatusCode != tc.want {
			t.Fatalf("资源列表 %s 期望 %d 实际 %d", tc.name, tc.want, conditional.StatusCode)
		}
		if tc.want == http.StatusNotModified {
			if len(conditionalBody) != 0 {
				t.Fatalf("资源列表 %s 304 响应不应有 body: %q", tc.name, string(conditionalBody))
			}
			if conditional.Header.Get("ETag") != etag {
				t.Fatalf("资源列表 %s 304 丢失 ETag", tc.name)
			}
			if conditional.Header.Get("Cache-Control") != "no-cache" {
				t.Fatalf("资源列表 %s 304 丢失 Cache-Control", tc.name)
			}
			continue
		}
		if !json.Valid(conditionalBody) {
			t.Fatalf("资源列表 %s 200 响应非法: %s", tc.name, string(conditionalBody))
		}
	}

	secondID := deploySingleResource(t, server, "list.bpmn20.xml", restProcess)
	secondResponse, _ := httpGetWithHeaders(t, server.URL+"/service/repository/deployments/"+secondID+"/resources", nil)
	if secondResponse.Header.Get("ETag") == etag {
		t.Fatal("不同部署的资源列表 ETag 不应相同")
	}

	missing, _ := httpGetWithHeaders(t, server.URL+"/service/repository/deployments/missing/resources", map[string]string{"If-None-Match": "*"})
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失部署列表期望 404 实际 %d", missing.StatusCode)
	}
}

// TestREST_DeploymentResourceETagHelpers 验证资源 ETag 与条件判断纯函数。
// @param t 测试上下文
func TestREST_DeploymentResourceETagHelpers(t *testing.T) {
	etag := deploymentResourceETag([]byte("alpha"))
	if !isQuotedSHA256ETag(etag) {
		t.Fatalf("资源 ETag 格式错误: %q", etag)
	}
	if etag != deploymentResourceETag([]byte("alpha")) {
		t.Fatal("相同内容 ETag 应稳定")
	}
	if etag == deploymentResourceETag([]byte("beta")) {
		t.Fatal("不同内容 ETag 不应相同")
	}

	matchCases := []struct {
		name   string
		header string
		want   bool
	}{
		{"空头", "", false},
		{"精确匹配", etag, true},
		{"弱前缀", "W/" + etag, true},
		{"小写弱前缀", "w/" + etag, true},
		{"多值命中", `"other", ` + etag, true},
		{"多值弱前缀命中", `"other", W/` + etag, true},
		{"星号", "*", true},
		{"星号带空白", "  *  ", true},
		{"不匹配", `"other"`, false},
		{"弱前缀不匹配", `W/"other"`, false},
		{"空标签", `"other",`, false},
	}
	for _, tc := range matchCases {
		if got := ifNoneMatchMatches(tc.header, etag); got != tc.want {
			t.Fatalf("ifNoneMatchMatches %s 期望 %v 实际 %v", tc.name, tc.want, got)
		}
	}

	if ifNoneMatchNotModified(requestWithHeaders(nil), etag) {
		t.Fatal("无 If-None-Match 时不应命中 304")
	}
	if !ifNoneMatchNotModified(requestWithHeaders(map[string]string{"If-None-Match": etag}), etag) {
		t.Fatal("相同 If-None-Match 应命中 304")
	}
	if ifNoneMatchNotModified(requestWithHeaders(map[string]string{"If-None-Match": `"other"`}), etag) {
		t.Fatal("不同 If-None-Match 不应命中 304")
	}
}

// TestREST_DeploymentResourcesETag 验证列表 ETag 对资源顺序不敏感且随输入变化。
// @param t 测试上下文
func TestREST_DeploymentResourcesETag(t *testing.T) {
	deployTime := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	names := []string{"b.bpmn20.xml", "a.form"}
	base := deploymentResourcesETag("dep-1", deployTime, names)
	if !isQuotedSHA256ETag(base) {
		t.Fatalf("列表 ETag 格式错误: %q", base)
	}
	if base != deploymentResourcesETag("dep-1", deployTime, []string{"a.form", "b.bpmn20.xml"}) {
		t.Fatal("列表 ETag 应与资源名顺序无关")
	}
	if base == deploymentResourcesETag("dep-2", deployTime, names) {
		t.Fatal("部署 ID 不同时列表 ETag 不应相同")
	}
	if base == deploymentResourcesETag("dep-1", deployTime.Add(time.Second), names) {
		t.Fatal("部署时间不同时列表 ETag 不应相同")
	}
	if base == deploymentResourcesETag("dep-1", deployTime, []string{"b.bpmn20.xml"}) {
		t.Fatal("资源集合不同时列表 ETag 不应相同")
	}
	if base == deploymentResourcesETag("dep-1", deployTime, []string{"a.form\nb.bpmn20.xml"}) {
		t.Fatal("资源名含分隔符时列表 ETag 不应与两个资源冲突")
	}
}

// TestREST_ParseConditionalTime 验证条件请求时间解析。
// @param t 测试上下文
func TestREST_ParseConditionalTime(t *testing.T) {
	parsed, ok := parseConditionalTime("Wed, 28 Oct 2026 12:00:00 GMT")
	want := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	if !ok || !parsed.Equal(want) || parsed.Location() != time.UTC {
		t.Fatalf("HTTP 时间解析错误: %v %v", parsed, ok)
	}
	if _, ok := parseConditionalTime("not-a-date"); ok {
		t.Fatal("非法时间不应解析成功")
	}
	if _, ok := parseConditionalTime("   "); ok {
		t.Fatal("空白时间不应解析成功")
	}
}

// TestREST_ResourceNotModified 验证 If-None-Match 与 If-Modified-Since 的 304 规则。
// @param t 测试上下文
func TestREST_ResourceNotModified(t *testing.T) {
	lastModified := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	etag := `"abc"`
	cases := []struct {
		name    string
		headers map[string]string
		want    bool
	}{
		{"无条件", nil, false},
		{"ETag 命中", map[string]string{"If-None-Match": etag}, true},
		{"弱 ETag 命中", map[string]string{"If-None-Match": "W/" + etag}, true},
		{"多值命中", map[string]string{"If-None-Match": `"other", ` + etag}, true},
		{"星号命中", map[string]string{"If-None-Match": "*"}, true},
		{"ETag 不命中", map[string]string{"If-None-Match": `"other"`}, false},
		{"ETag 不命中时忽略修改时间", map[string]string{"If-None-Match": `"other"`, "If-Modified-Since": lastModified.Format(http.TimeFormat)}, false},
		{"修改时间相等", map[string]string{"If-Modified-Since": lastModified.Format(http.TimeFormat)}, true},
		{"修改时间更早", map[string]string{"If-Modified-Since": lastModified.Add(-time.Hour).Format(http.TimeFormat)}, false},
		{"修改时间更晚", map[string]string{"If-Modified-Since": lastModified.Add(time.Hour).Format(http.TimeFormat)}, true},
		{"修改时间非法", map[string]string{"If-Modified-Since": "not-a-date"}, false},
	}
	for _, tc := range cases {
		if got := resourceNotModified(requestWithHeaders(tc.headers), etag, lastModified); got != tc.want {
			t.Fatalf("resourceNotModified %s 期望 %v 实际 %v", tc.name, tc.want, got)
		}
	}

	withNanos := lastModified.Add(500 * time.Millisecond)
	if !resourceNotModified(
		requestWithHeaders(map[string]string{"If-Modified-Since": lastModified.Format(http.TimeFormat)}),
		etag,
		withNanos,
	) {
		t.Fatal("部署时间含纳秒时仍应按秒比较命中 304")
	}
}
