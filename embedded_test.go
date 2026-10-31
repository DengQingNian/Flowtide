package flowtide_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide"
	"github.com/DengQingNian/Flowtide/api"
)

// TestEmbeddedMemory 验证显式内存模式能够提供引擎、workflow 门面和已接线 HTTP 入口。
// @param t 测试上下文
// @return 无
func TestEmbeddedMemory(t *testing.T) {
	ctx := context.Background()
	app, err := flowtide.NewEmbedded(ctx, flowtide.EmbeddedConfig{InMemory: true, TenantID: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := app.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	if app.Engine() == nil || app.Workflow() == nil {
		t.Fatal("公共服务未装配")
	}
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/workflow/dict/flowStatus", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"success":true`) {
		t.Fatalf("workflow HTTP 未接线：%d %s", rec.Code, rec.Body.String())
	}
	_, err = app.Engine().TaskService().CreateTask(ctx, api.CreateTaskRequest{Name: "公共入口"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.RunDueJobs(ctx); err != nil {
		t.Fatal(err)
	}
	if err := app.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := app.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

// TestEmbeddedRejectsImplicitMemory 验证生产入口不能因连接缺失或配置冲突静默转成内存。
// @param t 测试上下文
// @return 无
func TestEmbeddedRejectsImplicitMemory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if app, err := flowtide.NewEmbedded(ctx, flowtide.EmbeddedConfig{InMemory: true}); err == nil {
		_ = app.Close(context.Background())
		t.Fatal("已取消的启动上下文被忽略")
	}
	for _, cfg := range []flowtide.EmbeddedConfig{{}, {Database: "db"}, {InMemory: true, Database: "db"}, {InMemory: true, HistoryLevel: "wrong"}} {
		if app, err := flowtide.NewEmbedded(context.Background(), cfg); err == nil {
			_ = app.Close(context.Background())
			t.Fatal("非法嵌入配置被接受")
		}
	}
}

// TestEmbeddedContext 验证公共上下文适配可从仓库外模块调用。
// @param t 测试上下文
// @return 无
func TestEmbeddedContext(t *testing.T) {
	if err := flowtide.EnsureMySQLSchema(context.Background(), nil, false); err == nil {
		t.Fatal("空数据库未拒绝")
	}
	ctx := flowtide.WithExecutionContext(context.Background(), "demo", "101")
	if ctx == nil {
		t.Fatal("未返回上下文")
	}
}

// TestEmbeddedIdentityDecoration 验证宿主身份服务注入后同时供公共引擎与 workflow 使用，拒绝空装饰结果。
// @param t 测试上下文
// @return 无
func TestEmbeddedIdentityDecoration(t *testing.T) {
	var selected api.IdentityService
	app, err := flowtide.NewEmbedded(context.Background(), flowtide.EmbeddedConfig{InMemory: true, DecorateIdentity: func(inner api.IdentityService) api.IdentityService { selected = inner; return inner }})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close(context.Background())
	if selected == nil || app.Engine().IdentityService() != selected {
		t.Fatal("宿主身份服务未接线")
	}
	if app, err := flowtide.NewEmbedded(context.Background(), flowtide.EmbeddedConfig{InMemory: true, DecorateIdentity: func(api.IdentityService) api.IdentityService { return nil }}); err == nil {
		_ = app.Close(context.Background())
		t.Fatal("空身份服务未拒绝")
	}
}
