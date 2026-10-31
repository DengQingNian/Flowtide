package sdk_test

import (
	"context"
	"testing"

	"github.com/DengQingNian/Flowtide"
	"github.com/DengQingNian/Flowtide/api"
)

// TestPublicSDK 验证仓库外模块可以构造引擎并完成服务调用。
// @param t 测试上下文
// @return 无
func TestPublicSDK(t *testing.T) {
	app, err := flowtide.New(flowtide.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close(context.Background())
	task, err := app.TaskService().CreateTask(context.Background(), api.CreateTaskRequest{Name: "独立模块任务"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := app.TaskService().GetTask(context.Background(), task.ID); err != nil || got.Name != task.Name {
		t.Fatalf("公共 SDK 服务调用失败: %v", err)
	}
}

// TestPublicEmbeddedSDK 验证独立模块能够使用完整 workflow 门面与 HTTP 入口。
// @param t 测试上下文
// @return 无
func TestPublicEmbeddedSDK(t *testing.T) {
	app, err := flowtide.NewEmbedded(context.Background(), flowtide.EmbeddedConfig{InMemory: true, TenantID: "oa"})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close(context.Background())
	if app.Engine() == nil || app.Workflow() == nil || app.Handler() == nil {
		t.Fatal("完整公共 SDK 入口缺失")
	}
	result, err := app.Workflow().IsProcessDeployed(flowtide.WithExecutionContext(context.Background(), "oa", "101"), "oa", "missing")
	if err != nil || !result.Success {
		t.Fatalf("门面调用失败：%+v %v", result, err)
	}
}
