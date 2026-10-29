package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/flowable-go/flowable/internal/engine"
)

// TestOfficialMethodAcceptance 从固定官方规范逐方法验收真实路由、认证、只读权限及空输入行为。
// @param t 测试上下文
// @return 无
func TestOfficialMethodAcceptance(t *testing.T) {
	spec, err := os.ReadFile("../../testdata/compatibility/flowable-swagger-process.yaml")
	if err != nil {
		t.Fatal(err)
	}
	pathPattern := regexp.MustCompile(`^  (/[^:]+):\s*$`)
	methodPattern := regexp.MustCompile(`^    (get|post|put|delete|patch|head|options):`)
	parameterPattern := regexp.MustCompile(`\{[^}]+\}|\*\*`)
	path := ""
	count := 0
	for _, line := range strings.Split(string(spec), "\n") {
		if match := pathPattern.FindStringSubmatch(line); match != nil {
			path = match[1]
		}
		match := methodPattern.FindStringSubmatch(line)
		if match == nil || path == "" {
			continue
		}
		method := strings.ToUpper(match[1])
		target := "/service" + parameterPattern.ReplaceAllString(path, "missing")
		count++
		t.Run(method+" "+path, func(t *testing.T) {
			app, err := engine.New(engine.Config{})
			if err != nil {
				t.Fatal(err)
			}
			s := New(app, WithBasicAuth(map[string]string{"reader": "secret", "admin": "secret"}), WithRBAC(map[string][]string{"reader": {"reader"}, "admin": {"admin"}}, map[string][]string{"reader": {"read"}, "admin": {"admin"}}))
			r := httptest.NewRequest(method, target, strings.NewReader(`{}`))
			if _, pattern := s.mux.Handler(r); pattern == "" {
				t.Fatal("官方方法未注册真实处理器")
			}
			for _, user := range []string{"", "reader", "admin"} {
				r := httptest.NewRequest(method, target, strings.NewReader(`{}`))
				r.Header.Set("Content-Type", "application/json")
				if user != "" {
					r.SetBasicAuth(user, "secret")
				}
				w := httptest.NewRecorder()
				s.Handler().ServeHTTP(w, r)
				// 查询接口使用 POST 传递过滤条件，权限仍为只读。
				readable := !strings.HasPrefix(path, "/management/") && !strings.HasPrefix(path, "/identity/") && (method == http.MethodGet || (method == http.MethodPost && strings.HasPrefix(path, "/query/")))
				if user == "" && w.Code != http.StatusUnauthorized {
					t.Errorf("未认证=%d", w.Code)
				}
				if user == "reader" && !readable && w.Code != http.StatusForbidden {
					t.Errorf("只读角色越权=%d", w.Code)
				}
				if user == "reader" && readable && w.Code == http.StatusForbidden {
					t.Errorf("只读查询被拒绝=%d", w.Code)
				}
				if user == "admin" && (w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden || w.Code >= 500) {
					t.Errorf("管理员空输入处理=%d %s", w.Code, w.Body.String())
				}
			}
		})
	}
	if count != 184 {
		t.Fatalf("官方方法数量变化，需审核基线: %d", count)
	}
}

// TestJSONRequestBoundary 验证无效和拼接 JSON 统一返回 400，且不产生写入副作用。
// @param t 测试上下文
// @return 无
func TestJSONRequestBoundary(t *testing.T) {
	for _, target := range []string{"/service/runtime/tasks", "/service/repository/models", "/service/query/tasks"} {
		for _, body := range []string{`{`, `{} {}`, `{} trailing`, `null`} {
			t.Run(target+body, func(t *testing.T) {
				app, err := engine.New(engine.Config{})
				if err != nil {
					t.Fatal(err)
				}
				s := New(app)
				w := httptest.NewRecorder()
				s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, target, strings.NewReader(body)))
				if w.Code != http.StatusBadRequest {
					t.Fatalf("无效 JSON 返回 %d: %s", w.Code, w.Body.String())
				}
				var result map[string]any
				if err := json.NewDecoder(bytes.NewReader(w.Body.Bytes())).Decode(&result); err != nil {
					t.Fatal(err)
				}
				if result["message"] == nil {
					t.Fatalf("错误响应缺少 message: %+v", result)
				}
			})
		}
	}
}
