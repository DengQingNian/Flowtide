package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DengQingNian/Flowtide/internal/engine"
)

// TestAuditAuthenticatedIdentity 验证审计记录可信身份与绑定租户，失败凭据不能冒充用户。
// @param t 测试上下文
// @return 无
func TestAuditAuthenticatedIdentity(t *testing.T) {
	app, err := engine.New(engine.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, auth, user string
		status           int
	}{
		{"基本认证", "basic", "alice", 200},
		{"令牌认证", "jwt", "alice", 200},
		{"伪造身份", "invalid", "", 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(app, WithAuditLog(10), WithBasicAuth(map[string]string{"alice": "secret"}), WithTenantBindings(map[string]string{"alice": "tenant-a"}), WithJWT(JWTConfig{HS256Secret: "secret", TenantClaim: "tenant"}))
			r := httptest.NewRequest(http.MethodGet, "/service/repository/deployments", nil)
			switch tc.auth {
			case "basic":
				r.SetBasicAuth("alice", "secret")
			case "invalid":
				r.SetBasicAuth("alice", "wrong")
			case "jwt":
				r.Header.Set("Authorization", "Bearer "+testSignHS256(t, "secret", map[string]any{"sub": "alice", "tenant": "tenant-a"}))
			}
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			events := s.auditLog.list()
			if w.Code != tc.status || len(events) != 1 {
				t.Fatalf("审计请求: %d %+v", w.Code, events)
			}
			wantTenant := ""
			if tc.user != "" {
				wantTenant = "tenant-a"
			}
			if events[0].UserID != tc.user || events[0].TenantID != wantTenant {
				t.Fatalf("审计身份不可信: %+v", events[0])
			}
		})
	}
}

// TestStatusWriterFirstHeader 验证指标与审计遵循 HTTP 首次终态响应码。
// @param t 测试上下文
// @return 无
func TestStatusWriterFirstHeader(t *testing.T) {
	w := &statusWriter{ResponseWriter: httptest.NewRecorder()}
	w.WriteHeader(http.StatusCreated)
	w.WriteHeader(http.StatusInternalServerError)
	if w.status != http.StatusCreated {
		t.Fatalf("记录了未发送的状态码: %d", w.status)
	}
}
