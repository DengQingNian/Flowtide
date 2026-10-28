package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/tenant"
)

// TestVariableInstances 验证运行时变量实例查询。
// @param t 测试上下文
func TestVariableInstances(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	ctx := context.Background()
	instance, err := engine.StartProcessInstance(ctx, api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	items, err := engine.ListVariableInstances(ctx, api.VariableInstanceFilter{ProcessInstanceID: instance.ID})
	if err != nil || len(items) == 0 {
		t.Fatalf("变量实例为空: %v %+v", err, items)
	}
	var initiator *api.VariableInstance
	for index := range items {
		if items[index].Name == "initiator" {
			initiator = &items[index]
		}
	}
	if initiator == nil {
		t.Fatalf("未找到 initiator: %+v", items)
	}
	if initiator.ID == "" || initiator.Type != "string" || initiator.Value != "alice" ||
		initiator.Scope != "global" || initiator.ExecutionID == "" {
		t.Fatalf("变量实例字段错误: %+v", initiator)
	}
	if filtered, err := engine.ListVariableInstances(ctx, api.VariableInstanceFilter{Name: "initiator"}); err != nil || len(filtered) != 1 {
		t.Fatalf("按名称过滤失败: %v %+v", err, filtered)
	}
	if filtered, err := engine.ListVariableInstances(ctx, api.VariableInstanceFilter{NameLike: "%iti%"}); err != nil || len(filtered) == 0 {
		t.Fatalf("模糊过滤失败: %v %+v", err, filtered)
	}
	if filtered, err := engine.ListVariableInstances(ctx, api.VariableInstanceFilter{ExecutionID: initiator.ExecutionID}); err != nil || len(filtered) == 0 {
		t.Fatalf("按执行过滤失败: %v %+v", err, filtered)
	}
	loaded, err := engine.GetVariableInstance(ctx, initiator.ID)
	if err != nil || loaded.Name != "initiator" {
		t.Fatalf("变量实例查询失败: %v %+v", err, loaded)
	}
	if _, err := engine.GetVariableInstance(ctx, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失变量实例应 404: %v", err)
	}

	task, err := engine.CreateTask(ctx, api.CreateTaskRequest{Name: "Task"})
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	if err := engine.SetTaskVariable(ctx, task.ID, "payload", "body"); err != nil {
		t.Fatalf("设置任务变量失败: %v", err)
	}
	taskItems, err := engine.ListVariableInstances(ctx, api.VariableInstanceFilter{TaskID: task.ID})
	if err != nil || len(taskItems) != 1 || taskItems[0].Scope != "local" || taskItems[0].Value != "body" {
		t.Fatalf("任务变量实例错误: %v %+v", err, taskItems)
	}

	if filtered, err := engine.ListVariableInstances(tenant.With(ctx, "t2"), api.VariableInstanceFilter{}); err != nil || len(filtered) != 0 {
		t.Fatalf("跨租户变量应为空: %v %+v", err, filtered)
	}
	if _, err := engine.GetVariableInstance(tenant.With(ctx, "t2"), initiator.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户变量查询应 404: %v", err)
	}

	tasks := listTasks(t, engine)
	for _, item := range tasks {
		if item.ProcessInstanceID == instance.ID {
			if err := engine.Complete(ctx, item.ID, api.CompleteTaskRequest{UserID: "alice"}); err != nil {
				t.Fatalf("完成任务失败: %v", err)
			}
		}
	}
	if items, err := engine.ListVariableInstances(ctx, api.VariableInstanceFilter{ProcessInstanceID: instance.ID}); err != nil || len(items) != 0 {
		t.Fatalf("结束后变量实例应为空: %v %+v", err, items)
	}
}
