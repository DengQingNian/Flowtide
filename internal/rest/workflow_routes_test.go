package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/internal/engine"
)

// workflowRBody 是 501 响应的协议体结构（对齐 Java enm.R 字段名）。
type workflowRBody struct {
	Code         string `json:"code"`
	Msg          string `json:"msg"`
	ErrorMessage string `json:"ErrorMessage"`
	Data         any    `json:"data"`
	Success      bool   `json:"success"`
}

// newWorkflowTestServer 创建供 workflow 路由测试用的 REST 服务。
// @param t 测试上下文
// @param options REST 选项
// @return 测试 HTTP 服务
// @return 引擎（测试结束时关闭）
func newWorkflowTestServer(t *testing.T, options ...Option) (*httptest.Server, *engine.Engine) {
	t.Helper()
	app, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	server := httptest.NewServer(New(app, options...).Handler())
	t.Cleanup(func() {
		server.Close()
		_ = app.Close(context.Background())
	})
	return server, app
}

// workflowSamplePath 将路由占位符替换为具体值，得到可请求的样例路径。
// @param path 路由模板路径
// @return 样例路径
func workflowSamplePath(path string) string {
	replacer := strings.NewReplacer(
		"{id}", "t1", "{ids}", "t1", "{pid}", "t1",
		"{taskId}", "t1", "{processInstanceId}", "t1", "{code}", "t1",
	)
	return replacer.Replace(path)
}

// assertWorkflowNotImplemented 校验响应为 501 + R 失败体。
// @param t 测试上下文
// @param response HTTP 响应
func assertWorkflowNotImplemented(t *testing.T, response *http.Response) {
	t.Helper()
	if response.StatusCode != http.StatusNotImplemented {
		t.Fatalf("应返回 501，实际 %d", response.StatusCode)
	}
	var body workflowRBody
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("解析 JSON 失败: %v", err)
	}
	if body.Code != "500" || body.ErrorMessage != "接口未实现" || body.Success || body.Msg != "" {
		t.Errorf("协议体应为目标失败结构: %+v", body)
	}
}

// TestWorkflowRouteTableAllNotImplemented 逐条冒烟：路由表内全部端点返回 501 + 失败 R。
// @param t 测试上下文
func TestWorkflowRouteTableAllNotImplemented(t *testing.T) {
	server, _ := newWorkflowTestServer(t)
	for _, route := range workflowRouteTable {
		path := workflowSamplePath(route.path)
		methods := []string{route.method}
		// ANY 路由（Java @RequestMapping 无 method）同时验证 GET 与 POST
		if route.method == "" {
			methods = []string{http.MethodGet, http.MethodPost}
		}
		for _, method := range methods {
			request, _ := http.NewRequest(method, server.URL+path, nil)
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatalf("%s %s 请求失败: %v", method, path, err)
			}
			if response.StatusCode != http.StatusNotImplemented {
				_ = response.Body.Close()
				t.Errorf("%s %s 应返回 501，实际 %d", method, path, response.StatusCode)
				continue
			}
			assertWorkflowNotImplemented(t, response)
			_ = response.Body.Close()
		}
	}
}

// TestWorkflowSmokeModelList 验证冒烟端点 GET /workflow/model/list 的状态码与协议体。
// @param t 测试上下文
func TestWorkflowSmokeModelList(t *testing.T) {
	server, _ := newWorkflowTestServer(t)
	response, err := http.Get(server.URL + "/workflow/model/list")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusNotImplemented {
		t.Fatalf("应返回 501，实际 %d", response.StatusCode)
	}
	var body workflowRBody
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("解析 JSON 失败: %v", err)
	}
	if body.Code != "500" || body.ErrorMessage != "接口未实现" || body.Success {
		t.Errorf("协议体应为 {500, 接口未实现, success=false}: %+v", body)
	}
}

// TestWorkflowRoutesAuthConsistentWithService 验证认证开启时 /workflow 与 /service 行为一致：
// 未认证 401，认证后 /workflow 为 501、/service 为 200。
// @param t 测试上下文
func TestWorkflowRoutesAuthConsistentWithService(t *testing.T) {
	server, _ := newWorkflowTestServer(t, WithBasicAuth(map[string]string{"alice": "secret"}))
	// 未认证：/workflow 与 /service 一致 401
	for _, path := range []string{"/workflow/model/list", "/service/management/engine"} {
		response, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatalf("请求失败: %v", err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Errorf("未认证 %s 应 401，实际 %d", path, response.StatusCode)
		}
	}
	// 认证后：/workflow 501（未实现），/service 200
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/workflow/model/list", nil)
	request.SetBasicAuth("alice", "secret")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	assertWorkflowNotImplemented(t, response)
	_ = response.Body.Close()

	request, _ = http.NewRequest(http.MethodGet, server.URL+"/service/management/engine", nil)
	request.SetBasicAuth("alice", "secret")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Errorf("认证后 /service/management/engine 应 200，实际 %d", response.StatusCode)
	}
}

// TestWorkflowRoutesAuthDisabled 验证未启用认证选项时 /workflow 与 /service 均直接可达。
// @param t 测试上下文
func TestWorkflowRoutesAuthDisabled(t *testing.T) {
	server, _ := newWorkflowTestServer(t)
	response, err := http.Get(server.URL + "/workflow/model/list")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	assertWorkflowNotImplemented(t, response)
	_ = response.Body.Close()
	response, err = http.Get(server.URL + "/service/management/engine")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Errorf("无认证时 /service/management/engine 应 200，实际 %d", response.StatusCode)
	}
}

// TestWorkflowAuditApplied 验证审计中间件对 /workflow 请求生效（与 /service 同策略）。
// @param t 测试上下文
func TestWorkflowAuditApplied(t *testing.T) {
	server, _ := newWorkflowTestServer(t, WithAuditLog(10))
	response, err := http.Get(server.URL + "/workflow/model/list")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	_ = response.Body.Close()
	response, err = http.Get(server.URL + "/service/management/audit-logs")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("审计日志应 200，实际 %d", response.StatusCode)
	}
	var page struct {
		Data []struct {
			Path   string `json:"path"`
			Status int    `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		t.Fatalf("解析审计日志失败: %v", err)
	}
	found := false
	for _, event := range page.Data {
		if event.Path == "/workflow/model/list" && event.Status == http.StatusNotImplemented {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("审计日志应记录 /workflow 请求: %+v", page.Data)
	}
}

// TestWorkflowRateLimitApplied 验证限流中间件对 /workflow 请求生效（与 /service 同策略）。
// @param t 测试上下文
func TestWorkflowRateLimitApplied(t *testing.T) {
	// 令牌桶容量 1：第二个请求立即到达应被限流
	server, _ := newWorkflowTestServer(t, WithRateLimit(1, 1))
	for i := 0; i < 2; i++ {
		response, err := http.Get(server.URL + "/workflow/model/list")
		if err != nil {
			t.Fatalf("请求失败: %v", err)
		}
		status := response.StatusCode
		_ = response.Body.Close()
		if i == 0 && status != http.StatusNotImplemented {
			t.Fatalf("第一次请求应 501，实际 %d", status)
		}
		if i == 1 && status != http.StatusTooManyRequests {
			t.Fatalf("第二次请求应 429，实际 %d", status)
		}
	}
}
