package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// TestInjectUserTask 验证动态用户任务注入。
// @param t 测试上下文
func TestInjectUserTask(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, changeStateXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "changeState"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 {
		t.Fatalf("初始任务错误: %+v", tasks)
	}
	injected, err := engine.InjectUserTask(context.Background(), instance.ID, api.InjectRequest{
		InjectionType: "task",
		Name:          "Review",
		Assignee:      "bob",
		TaskID:        tasks[0].ID,
	})
	if err != nil {
		t.Fatalf("注入失败: %v", err)
	}
	if injected.Name != "Review" || injected.Assignee != "bob" || injected.ProcessInstanceID != instance.ID {
		t.Fatalf("注入任务错误: %+v", injected)
	}
	if all := listTasks(t, engine); len(all) != 2 {
		t.Fatalf("注入后任务数量错误: %+v", all)
	}
	if err := engine.Complete(context.Background(), injected.ID, api.CompleteTaskRequest{UserID: "bob"}); err != nil {
		t.Fatalf("完成注入任务失败: %v", err)
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if got.Ended {
		t.Fatalf("注入任务完成不应结束流程")
	}
	if all := listTasks(t, engine); len(all) != 1 || all[0].TaskDefinitionKey != "taskA" {
		t.Fatalf("原任务应保留: %+v", all)
	}
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{UserID: "alice"}); err != nil {
		t.Fatalf("完成原任务失败: %v", err)
	}
	got, _ = engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("原任务完成后应结束")
	}
}

// TestInjectUserTaskErrors 验证动态注入错误。
// @param t 测试上下文
func TestInjectUserTaskErrors(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, changeStateXML)
	instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "changeState"})
	if _, err := engine.InjectUserTask(context.Background(), "missing", api.InjectRequest{Name: "X"}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失实例应 404: %v", err)
	}
	if _, err := engine.InjectUserTask(context.Background(), instance.ID, api.InjectRequest{Name: " "}); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("空名称应 400: %v", err)
	}
	if _, err := engine.InjectUserTask(context.Background(), instance.ID, api.InjectRequest{InjectionType: "subprocess", Name: "X"}); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("不支持类型应 400: %v", err)
	}
	if _, err := engine.InjectUserTask(context.Background(), instance.ID, api.InjectRequest{ID: "fixed", Name: "X"}); err != nil {
		t.Fatalf("指定 ID 注入失败: %v", err)
	}
	if _, err := engine.InjectUserTask(context.Background(), instance.ID, api.InjectRequest{ID: "fixed", Name: "Y"}); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("重复 ID 应 409: %v", err)
	}
	if _, err := engine.InjectUserTask(context.Background(), instance.ID, api.InjectRequest{Name: "Y", TaskID: "missing"}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失参考任务应 404: %v", err)
	}
	if err := engine.SuspendProcessInstance(context.Background(), instance.ID); err != nil {
		t.Fatalf("挂起失败: %v", err)
	}
	if _, err := engine.InjectUserTask(context.Background(), instance.ID, api.InjectRequest{Name: "X"}); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("挂起实例应 409: %v", err)
	}
}
