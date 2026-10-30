package adminpage_test

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/DengQingNian/Flowtide/internal/engine"
	"github.com/DengQingNian/Flowtide/internal/rest"
	"github.com/DengQingNian/Flowtide/internal/rest/adminpage"
)

// wantCSP 是管理台期望的内容安全策略。
const wantCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'"

// fixture 构造覆盖各类静态资源的测试文件系统。
// @return 测试文件系统
func fixture() fstest.MapFS {
	return fstest.MapFS{
		"index.html":            {Data: []byte("<html><body>flowtide-admin-index</body></html>")},
		"assets/app-abc123.js":  {Data: []byte("console.log('flowtide-admin');")},
		"assets/app-def456.css": {Data: []byte("body{color:#222}")},
		"assets/logo-111.svg":   {Data: []byte("<svg xmlns='http://www.w3.org/2000/svg'></svg>")},
		"assets/font-222.woff2": {Data: []byte("wOF2")},
		"robots.txt":            {Data: []byte("User-agent: *")},
		"secret.txt":            {Data: []byte("SECRET-MARKER")},
		"data.json/file.txt":    {Data: []byte("nested")},
	}
}

// serve 使用给定文件系统执行一次管理台请求。
// @param t 测试上下文
// @param fsys 静态资源文件系统
// @param method HTTP 方法
// @param target 请求目标
// @return 响应记录器
func serve(t *testing.T, fsys fs.FS, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	adminpage.NewHandler(fsys).ServeHTTP(recorder, httptest.NewRequest(method, target, nil))
	return recorder
}

// assertSecurityHeaders 校验管理台统一安全响应头。
// @param t 测试上下文
// @param header 响应头
func assertSecurityHeaders(t *testing.T, header http.Header) {
	t.Helper()
	expected := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "same-origin",
		"Content-Security-Policy": wantCSP,
	}
	for name, want := range expected {
		if got := header.Get(name); got != want {
			t.Errorf("响应头 %s 期望 %q，实际 %q", name, want, got)
		}
	}
}

// fetch 执行一次不跟随跳转的请求。
// @param t 测试上下文
// @param client HTTP 客户端
// @param target 请求地址
// @return 状态码
// @return 响应头
// @return 响应体
func fetch(t *testing.T, client *http.Client, target string) (int, http.Header, string) {
	t.Helper()
	response, err := client.Get(target)
	if err != nil {
		t.Fatalf("请求 %s 失败: %v", target, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("读取 %s 响应失败: %v", target, err)
	}
	return response.StatusCode, response.Header, string(body)
}

// TestServeIndex 验证根路径返回 index.html 且禁止缓存。
// @param t 测试上下文
func TestServeIndex(t *testing.T) {
	recorder := serve(t, fixture(), http.MethodGet, "/adminpage/")
	if recorder.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d", recorder.Code)
	}
	if cache := recorder.Header().Get("Cache-Control"); cache != "no-store" {
		t.Errorf("期望 no-store，实际 %q", cache)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
		t.Errorf("期望 HTML Content-Type，实际 %q", contentType)
	}
	if body := recorder.Body.String(); !strings.Contains(body, "flowtide-admin-index") {
		t.Errorf("响应体未包含 index.html 内容: %q", body)
	}
	assertSecurityHeaders(t, recorder.Header())
}

// TestServeIndexHead 验证 HEAD 请求返回相同头部但没有响应体。
// @param t 测试上下文
func TestServeIndexHead(t *testing.T) {
	recorder := serve(t, fixture(), http.MethodHead, "/adminpage/")
	if recorder.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d", recorder.Code)
	}
	if recorder.Body.Len() != 0 {
		t.Errorf("HEAD 响应不应包含响应体，实际 %q", recorder.Body.String())
	}
	if cache := recorder.Header().Get("Cache-Control"); cache != "no-store" {
		t.Errorf("期望 no-store，实际 %q", cache)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
		t.Errorf("期望 HTML Content-Type，实际 %q", contentType)
	}
	assertSecurityHeaders(t, recorder.Header())
}

// TestServeHashedAssets 验证指纹资源使用长缓存且 MIME 正确。
// @param t 测试上下文
func TestServeHashedAssets(t *testing.T) {
	cases := []struct {
		name        string // 子测试名称
		target      string // 请求路径
		contentType string // 期望 MIME 前缀
	}{
		{"javascript", "/adminpage/assets/app-abc123.js", "text/javascript"},
		{"stylesheet", "/adminpage/assets/app-def456.css", "text/css"},
		{"svg", "/adminpage/assets/logo-111.svg", "image/svg+xml"},
		{"font", "/adminpage/assets/font-222.woff2", "font/woff2"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := serve(t, fixture(), http.MethodGet, testCase.target)
			if recorder.Code != http.StatusOK {
				t.Fatalf("期望 200，实际 %d", recorder.Code)
			}
			if cache := recorder.Header().Get("Cache-Control"); cache != "public, max-age=31536000, immutable" {
				t.Errorf("期望长缓存，实际 %q", cache)
			}
			if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, testCase.contentType) {
				t.Errorf("期望 Content-Type 以 %q 开头，实际 %q", testCase.contentType, contentType)
			}
			assertSecurityHeaders(t, recorder.Header())
		})
	}
}

// TestServeNonAssetFile 验证非资源目录文件不启用长缓存。
// @param t 测试上下文
func TestServeNonAssetFile(t *testing.T) {
	recorder := serve(t, fixture(), http.MethodGet, "/adminpage/robots.txt")
	if recorder.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d", recorder.Code)
	}
	if cache := recorder.Header().Get("Cache-Control"); cache != "no-store" {
		t.Errorf("期望 no-store，实际 %q", cache)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/plain") {
		t.Errorf("期望 text/plain，实际 %q", contentType)
	}
}

// TestSPAFallback 验证无扩展名路径回退到 index.html。
// @param t 测试上下文
func TestSPAFallback(t *testing.T) {
	targets := []string{
		"/adminpage/dashboard",
		"/adminpage/dashboard/instances/42",
		"/adminpage/index",
	}
	for _, target := range targets {
		recorder := serve(t, fixture(), http.MethodGet, target)
		if recorder.Code != http.StatusOK {
			t.Fatalf("请求 %s 期望 200，实际 %d", target, recorder.Code)
		}
		if body := recorder.Body.String(); !strings.Contains(body, "flowtide-admin-index") {
			t.Errorf("请求 %s 未回退到 index.html: %q", target, body)
		}
		if cache := recorder.Header().Get("Cache-Control"); cache != "no-store" {
			t.Errorf("请求 %s 期望 no-store，实际 %q", target, cache)
		}
	}
}

// TestMissingAsset 验证缺失资源返回中文 HTML 404 而不是 JSON。
// @param t 测试上下文
func TestMissingAsset(t *testing.T) {
	targets := []string{
		"/adminpage/assets/missing-999.js",
		"/adminpage/missing.css",
		"/adminpage/data.json",
	}
	for _, target := range targets {
		recorder := serve(t, fixture(), http.MethodGet, target)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("请求 %s 期望 404，实际 %d", target, recorder.Code)
		}
		if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
			t.Errorf("请求 %s 期望 HTML 错误页，实际 %q", target, contentType)
		}
		if cache := recorder.Header().Get("Cache-Control"); cache != "no-store" {
			t.Errorf("请求 %s 期望 no-store，实际 %q", target, cache)
		}
		body := recorder.Body.String()
		if !strings.Contains(body, "管理台资源不存在") {
			t.Errorf("请求 %s 缺少中文错误文案: %q", target, body)
		}
		if strings.HasPrefix(strings.TrimSpace(body), "{") {
			t.Errorf("请求 %s 返回了 JSON 而不是 HTML: %q", target, body)
		}
		assertSecurityHeaders(t, recorder.Header())
	}
}

// TestPlaceholderWhenNotBuilt 验证只有占位文件时返回未构建提示页。
// @param t 测试上下文
func TestPlaceholderWhenNotBuilt(t *testing.T) {
	fsys := fstest.MapFS{".keep": {Data: []byte("# keep")}}
	recorder := serve(t, fsys, http.MethodGet, "/adminpage/")
	if recorder.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d", recorder.Code)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "管理台未构建") {
		t.Errorf("占位页缺少未构建文案: %q", body)
	}
	if !strings.Contains(body, "make admin-build") {
		t.Errorf("占位页缺少构建命令说明: %q", body)
	}
	if cache := recorder.Header().Get("Cache-Control"); cache != "no-store" {
		t.Errorf("期望 no-store，实际 %q", cache)
	}
	assertSecurityHeaders(t, recorder.Header())

	// 未构建时静态资源仍应返回 404，避免把占位页当作脚本下发
	missing := serve(t, fsys, http.MethodGet, "/adminpage/assets/app-abc123.js")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("未构建时静态资源期望 404，实际 %d", missing.Code)
	}
	if !strings.Contains(missing.Body.String(), "管理台资源不存在") {
		t.Errorf("未构建时静态资源缺少中文 404 文案: %q", missing.Body.String())
	}

	// 未知扩展名使用二进制兜底 MIME
	keep := serve(t, fsys, http.MethodGet, "/adminpage/.keep")
	if keep.Code != http.StatusOK {
		t.Fatalf("占位文件期望 200，实际 %d", keep.Code)
	}
	if contentType := keep.Header().Get("Content-Type"); contentType != "application/octet-stream" {
		t.Errorf("未知扩展名期望 application/octet-stream，实际 %q", contentType)
	}

	empty := serve(t, fstest.MapFS{}, http.MethodGet, "/adminpage/")
	if empty.Code != http.StatusOK || !strings.Contains(empty.Body.String(), "管理台未构建") {
		t.Errorf("空文件系统应返回占位页，实际 %d: %q", empty.Code, empty.Body.String())
	}

	// 占位页同样支持 HEAD，只返回头部
	head := serve(t, fsys, http.MethodHead, "/adminpage/")
	if head.Code != http.StatusOK {
		t.Errorf("占位页 HEAD 期望 200，实际 %d", head.Code)
	}
	if head.Body.Len() != 0 {
		t.Errorf("占位页 HEAD 不应包含响应体，实际 %q", head.Body.String())
	}
}

// TestEmbeddedDistLayout 验证与 go:embed all:dist 一致的目录布局可直接挂载。
// @param t 测试上下文
func TestEmbeddedDistLayout(t *testing.T) {
	fsys := fstest.MapFS{
		"dist/index.html":           {Data: []byte("<html><body>embedded-admin-index</body></html>")},
		"dist/assets/app-789abc.js": {Data: []byte("console.log('embedded');")},
	}
	recorder := serve(t, fsys, http.MethodGet, "/adminpage/")
	if recorder.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d", recorder.Code)
	}
	if body := recorder.Body.String(); !strings.Contains(body, "embedded-admin-index") {
		t.Errorf("未从 dist 子目录读取 index.html: %q", body)
	}

	asset := serve(t, fsys, http.MethodGet, "/adminpage/assets/app-789abc.js")
	if asset.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d", asset.Code)
	}
	if cache := asset.Header().Get("Cache-Control"); cache != "public, max-age=31536000, immutable" {
		t.Errorf("dist 资源期望长缓存，实际 %q", cache)
	}
	if contentType := asset.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/javascript") {
		t.Errorf("dist 资源期望 JavaScript，实际 %q", contentType)
	}

	fallback := serve(t, fsys, http.MethodGet, "/adminpage/runtime/instances")
	if body := fallback.Body.String(); !strings.Contains(body, "embedded-admin-index") {
		t.Errorf("SPA 回退未使用 dist/index.html: %q", body)
	}
}

// TestPathTraversal 验证目录遍历、绝对路径与跨平台分隔符不会泄露文件。
// @param t 测试上下文
func TestPathTraversal(t *testing.T) {
	targets := []string{
		"/adminpage/../secret.txt",
		"/adminpage/%2e%2e/secret.txt",
		"/adminpage/%2e%2e%2fsecret.txt",
		"/adminpage/assets/..%2f..%2fsecret.txt",
		"/adminpage/..%5csecret.txt",
		"/adminpage/%5c..%5csecret.txt",
		"/adminpage//etc/passwd",
		"/adminpage/C:/secret.txt",
		"/adminpage/.",
		"/adminpage/%00secret.txt",
		"/adminpageevil/secret.txt",
	}
	for _, target := range targets {
		recorder := serve(t, fixture(), http.MethodGet, target)
		if recorder.Code != http.StatusNotFound {
			t.Errorf("请求 %s 期望 404，实际 %d", target, recorder.Code)
		}
		if body := recorder.Body.String(); strings.Contains(body, "SECRET-MARKER") {
			t.Errorf("请求 %s 泄露了根目录外部文件: %q", target, body)
		}
		assertSecurityHeaders(t, recorder.Header())
	}
}

// TestPrefixRedirect 验证缺少斜杠的前缀会重定向到管理台根路径。
// @param t 测试上下文
func TestPrefixRedirect(t *testing.T) {
	recorder := serve(t, fixture(), http.MethodGet, "/adminpage")
	if recorder.Code != http.StatusMovedPermanently {
		t.Fatalf("期望 301，实际 %d", recorder.Code)
	}
	if location := recorder.Header().Get("Location"); location != "/adminpage/" {
		t.Errorf("期望跳转到 /adminpage/，实际 %q", location)
	}

	query := serve(t, fixture(), http.MethodGet, "/adminpage?tenant=demo")
	if location := query.Header().Get("Location"); location != "/adminpage/?tenant=demo" {
		t.Errorf("期望保留查询串，实际 %q", location)
	}
}

// TestMethodNotAllowed 验证非 GET/HEAD 请求被拒绝。
// @param t 测试上下文
func TestMethodNotAllowed(t *testing.T) {
	recorder := serve(t, fixture(), http.MethodPost, "/adminpage/")
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("期望 405，实际 %d", recorder.Code)
	}
	if allow := recorder.Header().Get("Allow"); allow != "GET, HEAD" {
		t.Errorf("期望 Allow 为 GET, HEAD，实际 %q", allow)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
		t.Errorf("期望 HTML 提示页，实际 %q", contentType)
	}
	if !strings.Contains(recorder.Body.String(), "请求方法不受支持") {
		t.Errorf("缺少中文方法提示: %q", recorder.Body.String())
	}
}

// TestOutsidePrefix 验证前缀之外的路径返回中文 404。
// @param t 测试上下文
func TestOutsidePrefix(t *testing.T) {
	for _, target := range []string{"/", "/adminpage-index", "/adminpage-index/"} {
		recorder := serve(t, fixture(), http.MethodGet, target)
		if recorder.Code != http.StatusNotFound {
			t.Errorf("请求 %s 期望 404，实际 %d", target, recorder.Code)
		}
		if !strings.Contains(recorder.Body.String(), "管理台资源不存在") {
			t.Errorf("请求 %s 缺少中文 404 文案: %q", target, recorder.Body.String())
		}
	}
}

// TestServerRegistersAdminPageRoutes 验证 REST 服务注册了管理台与根路径跳转。
// @param t 测试上下文
func TestServerRegistersAdminPageRoutes(t *testing.T) {
	application, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	t.Cleanup(func() {
		_ = application.Close(context.Background())
	})
	server := httptest.NewServer(rest.New(application).Handler())
	t.Cleanup(server.Close)
	client := server.Client()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	// 根路径跳转到管理台，且只匹配精确根路径
	status, header, _ := fetch(t, client, server.URL+"/")
	if status != http.StatusFound {
		t.Errorf("根路径期望 302，实际 %d", status)
	}
	if location := header.Get("Location"); location != "/adminpage/" {
		t.Errorf("根路径期望跳转 /adminpage/，实际 %q", location)
	}

	// ServeMux 为缺少斜杠的管理台前缀补跳转
	status, header, _ = fetch(t, client, server.URL+"/adminpage")
	if status != http.StatusMovedPermanently {
		t.Errorf("/adminpage 期望 301，实际 %d", status)
	}
	if location := header.Get("Location"); location != "/adminpage/" {
		t.Errorf("/adminpage 期望跳转 /adminpage/，实际 %q", location)
	}

	// 补斜杠跳转保留查询串
	status, header, _ = fetch(t, client, server.URL+"/adminpage?tenant=demo")
	if status != http.StatusMovedPermanently {
		t.Errorf("/adminpage?tenant=demo 期望 301，实际 %d", status)
	}
	if location := header.Get("Location"); location != "/adminpage/?tenant=demo" {
		t.Errorf("补斜杠跳转期望保留查询串，实际 %q", location)
	}

	// 管理台首页与静态资源均由嵌入处理器响应
	status, header, body := fetch(t, client, server.URL+"/adminpage/")
	if status != http.StatusOK {
		t.Errorf("/adminpage/ 期望 200，实际 %d", status)
	}
	if contentType := header.Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
		t.Errorf("/adminpage/ 期望 HTML，实际 %q", contentType)
	}
	if body == "" {
		t.Error("/adminpage/ 响应体不应为空")
	}

	status, header, body = fetch(t, client, server.URL+"/adminpage/assets/missing-999.js")
	if status != http.StatusNotFound {
		t.Errorf("缺失资源期望 404，实际 %d", status)
	}
	if !strings.HasPrefix(header.Get("Content-Type"), "text/html") {
		t.Errorf("缺失资源期望 HTML，实际 %q", header.Get("Content-Type"))
	}
	if !strings.Contains(body, "管理台资源不存在") {
		t.Errorf("缺失资源缺少中文 404 文案: %q", body)
	}
}
