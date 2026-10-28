package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/tenant"
)

// TestActivityInstances 验证运行中活动实例查询。
// @param t 测试上下文
func TestActivityInstances(t *testing.T) {
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
	items, err := engine.ListActivityInstances(ctx, instance.ID)
	if err != nil || len(items) == 0 {
		t.Fatalf("活动实例为空: %v %+v", err, items)
	}
	var taskInstance *api.ActivityInstance
	for index := range items {
		if items[index].ActivityID == "approve" {
			taskInstance = &items[index]
		}
	}
	if taskInstance == nil {
		t.Fatalf("未找到用户任务活动: %+v", items)
	}
	if taskInstance.ActivityType != "userTask" || taskInstance.ActivityName != "Approve" ||
		taskInstance.TaskID == "" || taskInstance.Assignee != "alice" || taskInstance.StartTime == nil {
		t.Fatalf("活动实例字段错误: %+v", taskInstance)
	}
	if taskInstance.ProcessInstanceID != instance.ID || taskInstance.ExecutionID == "" {
		t.Fatalf("活动实例关联错误: %+v", taskInstance)
	}

	executions, _ := engine.ListExecutions(ctx, instance.ID)
	if len(executions) == 0 {
		t.Fatalf("执行为空")
	}
	activities, err := engine.GetActiveActivityIDs(ctx, executions[0].ID)
	if err != nil || !contains(activities, "approve") {
		t.Fatalf("活动 ID 错误: %v %+v", err, activities)
	}
	if _, err := engine.GetActiveActivityIDs(ctx, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失执行应 404: %v", err)
	}
	if list, err := engine.ListActivityInstances(ctx, "missing"); err != nil || len(list) != 0 {
		t.Fatalf("缺失实例应返回空: %v %+v", err, list)
	}

	tasks := listTasks(t, engine)
	if err := engine.Complete(ctx, tasks[0].ID, api.CompleteTaskRequest{UserID: "alice"}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	if items, err := engine.ListActivityInstances(ctx, instance.ID); err != nil || len(items) != 0 {
		t.Fatalf("结束后活动实例应为空: %v %+v", err, items)
	}
}

// TestActivityInstanceTenant 验证活动实例租户隔离。
// @param t 测试上下文
func TestActivityInstanceTenant(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	ctx := context.Background()
	instance, err := engine.StartProcessInstance(ctx, api.StartRequest{
		ProcessDefinitionKey: "simple",
		TenantID:             "t1",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if items, err := engine.ListActivityInstances(tenant.With(ctx, "t2"), instance.ID); err != nil || len(items) != 0 {
		t.Fatalf("跨租户活动应为空: %v %+v", err, items)
	}
	executions, _ := engine.ListExecutions(ctx, instance.ID)
	if _, err := engine.GetActiveActivityIDs(tenant.With(ctx, "t2"), executions[0].ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户执行活动应 404: %v", err)
	}
}
