package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/flowable-go/flowable/api"
)

// TestTaskDelegation 验证任务委派与处理。
// @param t 测试上下文
func TestTaskDelegation(t *testing.T) {
	engine := newTestEngine(t)
	task, err := engine.CreateTask(context.Background(), api.CreateTaskRequest{Name: "Task", Assignee: "alice"})
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	if err := engine.Delegate(context.Background(), task.ID, "bob"); err != nil {
		t.Fatalf("委派失败: %v", err)
	}
	delegated, _ := engine.GetTask(context.Background(), task.ID)
	if delegated.Assignee != "bob" || delegated.Owner != "alice" || delegated.DelegationState != "PENDING" {
		t.Fatalf("委派状态错误: %+v", delegated)
	}
	page, err := engine.ListTasks(context.Background(), api.TaskFilter{DelegationState: "PENDING"})
	if err != nil || len(page.Data) != 1 {
		t.Fatalf("委派过滤错误: %v %+v", err, page)
	}
	if err := engine.Resolve(context.Background(), task.ID, "carol"); !errors.Is(err, api.ErrForbidden) {
		t.Fatalf("非所有者处理应 403: %v", err)
	}
	if err := engine.Resolve(context.Background(), task.ID, "alice"); err != nil {
		t.Fatalf("处理委派失败: %v", err)
	}
	resolved, _ := engine.GetTask(context.Background(), task.ID)
	if resolved.Assignee != "alice" || resolved.Owner != "alice" || resolved.DelegationState != "RESOLVED" {
		t.Fatalf("处理后状态错误: %+v", resolved)
	}
	if err := engine.Resolve(context.Background(), task.ID, "alice"); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("重复处理应 409: %v", err)
	}
	unassigned, _ := engine.CreateTask(context.Background(), api.CreateTaskRequest{Name: "Unassigned"})
	if err := engine.Delegate(context.Background(), unassigned.ID, "bob"); err != nil {
		t.Fatalf("原版允许未签收任务委派: %v", err)
	}
	if err := engine.Resolve(context.Background(), unassigned.ID, ""); err != nil {
		t.Fatalf("无所有者的委派任务应恢复未签收状态: %v", err)
	}
}
