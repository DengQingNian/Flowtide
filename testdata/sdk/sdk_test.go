package sdk_test

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable"
	"github.com/flowable-go/flowable/api"
)

// TestPublicSDK 验证仓库外模块可以构造引擎并完成服务调用。
// @param t 测试上下文
// @return 无
func TestPublicSDK(t *testing.T) {
	app, err := flowable.New(flowable.Config{})
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
