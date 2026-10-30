package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/engine"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// TestAuditJWTTenantBinding 验证 JWT 身份约束租户选择、查询和写入。
// @param t 测试上下文
// @return 无
func TestAuditJWTTenantBinding(t *testing.T) {
	app, _ := engine.New(engine.Config{})
	model, err := app.CreateModel(tenant.With(context.Background(), "tenant-b"), api.CreateModelRequest{Name: "模型", Key: "b", TenantID: "tenant-b"})
	if err != nil {
		t.Fatal(err)
	}
	server := New(app, WithJWT(JWTConfig{HS256Secret: "secret", TenantClaim: "tenant_id"})).Handler()
	token := testSignHS256(t, "secret", map[string]any{"sub": "alice", "tenant_id": "tenant-a"})
	for _, target := range []string{"/service/repository/models/" + model.ID, "/service/repository/models/" + model.ID + "?tenantId=tenant-b", "/service/repository/models"} {
		for _, header := range []string{"", "tenant-b"} {
			request := httptest.NewRequest(http.MethodGet, target, nil)
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("X-Tenant-Id", header)
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if strings.Contains(target, "models/") && response.Code == http.StatusOK {
				t.Fatalf("跨租户读取成功: %s %q", target, header)
			}
			if target == "/service/repository/models" && header == "" && strings.Contains(response.Body.String(), model.ID) {
				t.Fatal("查询泄露其他租户模型")
			}
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/service/repository/models", strings.NewReader(`{"name":"越权","key":"bad","tenantId":"tenant-b"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code < 400 {
		t.Fatalf("跨租户创建成功: %d", response.Code)
	}
}

// TestAuditDefaultTenantIsolation 验证未绑定租户的身份只能访问默认租户。
// @param t 测试上下文
// @return 无
func TestAuditDefaultTenantIsolation(t *testing.T) {
	app, _ := engine.New(engine.Config{})
	model, err := app.CreateModel(tenant.With(context.Background(), "other"), api.CreateModelRequest{Name: "模型", Key: "other", TenantID: "other"})
	if err != nil {
		t.Fatal(err)
	}
	server := New(app, WithBasicAuth(map[string]string{"alice": "secret"})).Handler()
	for _, header := range []string{"", "other"} {
		request := httptest.NewRequest(http.MethodGet, "/service/repository/models/"+model.ID, nil)
		request.SetBasicAuth("alice", "secret")
		request.Header.Set("X-Tenant-Id", header)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code == http.StatusOK {
			t.Fatalf("默认身份跨租户读取成功: %q", header)
		}
	}
}

// TestAuditManagementTenant 验证管理表分页不会绕过 JWT 的租户边界。
// @param t 测试上下文
// @return 无
func TestAuditManagementTenant(t *testing.T) {
	app, _ := engine.New(engine.Config{})
	model, err := app.CreateModel(tenant.With(context.Background(), "b"), api.CreateModelRequest{Name: "其他租户", Key: "foreign", TenantID: "b"})
	if err != nil {
		t.Fatal(err)
	}
	server := New(app, WithJWT(JWTConfig{HS256Secret: "secret"})).Handler()
	request := httptest.NewRequest(http.MethodGet, "/service/management/tables/ACT_RE_MODEL/data", nil)
	request.Header.Set("Authorization", "Bearer "+testSignHS256(t, "secret", map[string]any{"sub": "alice", "tenant_id": "a"}))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), model.ID) {
		t.Fatalf("管理表数据泄露租户: %d %s", response.Code, response.Body.String())
	}
}
