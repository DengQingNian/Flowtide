package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/tenant"
)

// TestTaskEvents 验证任务事件列表、详情与删除。
// @param t 测试上下文
func TestTaskEvents(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	ctx := context.Background()
	if _, err := engine.StartProcessInstance(ctx, api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) == 0 {
		t.Fatalf("任务为空")
	}
	taskID := tasks[0].ID
	events, err := engine.ListTaskEvents(ctx, taskID)
	if err != nil || len(events) == 0 || events[0].Action != "create" || events[0].TaskID != taskID {
		t.Fatalf("任务事件列表错误: %v %+v", err, events)
	}
	eventID := events[0].ID
	event, err := engine.GetTaskEvent(ctx, taskID, eventID)
	if err != nil || event.ID != eventID || event.Action != "create" {
		t.Fatalf("任务事件查询失败: %v %+v", err, event)
	}
	if err := engine.Delegate(ctx, taskID, "bob"); err != nil {
		t.Fatalf("委派失败: %v", err)
	}
	events, _ = engine.ListTaskEvents(ctx, taskID)
	if len(events) < 2 {
		t.Fatalf("委派后事件未增加: %+v", events)
	}
	if _, err := engine.GetTaskEvent(ctx, taskID, 9999); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失事件应 404: %v", err)
	}
	if err := engine.DeleteTaskEvent(ctx, taskID, eventID); err != nil {
		t.Fatalf("删除任务事件失败: %v", err)
	}
	if _, err := engine.GetTaskEvent(ctx, taskID, eventID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("删除后事件应 404: %v", err)
	}
	if err := engine.DeleteTaskEvent(ctx, taskID, eventID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("重复删除应 404: %v", err)
	}
	if _, err := engine.ListTaskEvents(ctx, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失任务事件应 404: %v", err)
	}

	// 完成任务后仍可查询历史任务事件，但不能删除
	if err := engine.Complete(ctx, taskID, api.CompleteTaskRequest{UserID: "bob"}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	if events, err := engine.ListTaskEvents(ctx, taskID); err != nil || len(events) == 0 {
		t.Fatalf("完成后事件查询失败: %v %+v", err, events)
	}
	if err := engine.DeleteTaskEvent(ctx, taskID, events[0].ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("完成后删除事件应 404: %v", err)
	}

	tenantEngine := newTestEngine(t)
	tenantEngine.cfg.TenantID = "t1"
	deployXML(t, tenantEngine, simpleProcess)
	instance, err := tenantEngine.StartProcessInstance(ctx, api.StartRequest{
		ProcessDefinitionKey: "simple",
		TenantID:             "t1",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("启动租户实例失败: %v", err)
	}
	_ = instance
	tenantTasks := listTasks(t, tenantEngine)
	if _, err := tenantEngine.ListTaskEvents(tenant.With(ctx, "t2"), tenantTasks[0].ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户事件查询应 404: %v", err)
	}
}

// TestHistoricTaskForm 验证历史任务表单。
// @param t 测试上下文
func TestHistoricTaskForm(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, formXML)
	ctx := context.Background()
	if _, err := engine.StartProcessInstance(ctx, api.StartRequest{
		ProcessDefinitionKey: "formProcess",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	taskID := tasks[0].ID
	form, err := engine.GetHistoricTaskFormData(ctx, taskID)
	if err != nil || form.FormKey != "taskForm" || len(form.Properties) != 3 {
		t.Fatalf("历史任务表单错误: %v %+v", err, form)
	}
	if err := engine.SubmitTaskFormData(ctx, taskID, map[string]any{"approved": true, "amount": 10}); err != nil {
		t.Fatalf("提交表单失败: %v", err)
	}
	form, err = engine.GetHistoricTaskFormData(ctx, taskID)
	if err != nil || form.FormKey != "taskForm" {
		t.Fatalf("完成后历史表单错误: %v %+v", err, form)
	}
	if _, err := engine.GetHistoricTaskFormData(ctx, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失任务应 404: %v", err)
	}

	deployXML(t, engine, simpleProcess)
	if _, err := engine.StartProcessInstance(ctx, api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	allTasks := listTasks(t, engine)
	var plainTask string
	for _, task := range allTasks {
		if task.ProcessDefinitionID != tasks[0].ProcessDefinitionID {
			plainTask = task.ID
		}
	}
	if plainTask == "" {
		t.Fatalf("未找到无表单任务")
	}
	if _, err := engine.GetHistoricTaskFormData(ctx, plainTask); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("无表单任务应 400: %v", err)
	}
}
