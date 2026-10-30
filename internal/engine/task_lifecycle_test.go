package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// TestStandaloneTaskLifecycle 验证独立任务创建、更新与完成。
// @param t 测试上下文
func TestStandaloneTaskLifecycle(t *testing.T) {
	engine := newTestEngine(t)
	ctx := context.Background()
	dueDate := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	task, err := engine.CreateTask(ctx, api.CreateTaskRequest{
		Name:        "Review",
		Description: "desc",
		Assignee:    "alice",
		Owner:       "bob",
		DueDate:     &dueDate,
		Category:    "cat",
		FormKey:     "form",
	})
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	if task.ID == "" || task.Priority != 50 || task.DueDate == nil || !task.DueDate.Equal(dueDate) {
		t.Fatalf("任务默认值错误: %+v", task)
	}
	loaded, err := engine.GetTask(ctx, task.ID)
	if err != nil || loaded.Name != "Review" || loaded.Assignee != "alice" {
		t.Fatalf("查询任务失败: %v %+v", err, loaded)
	}

	name := "Review v2"
	assignee := "carol"
	priority := 80
	err = func() error {
		updated, updateErr := engine.UpdateTask(ctx, task.ID, api.TaskUpdate{
			Name:         &name,
			Assignee:     &assignee,
			Priority:     &priority,
			ClearDueDate: true,
		})
		if updateErr != nil {
			return updateErr
		}
		if updated.Name != name || updated.Assignee != assignee || updated.Priority != priority || updated.DueDate != nil {
			t.Fatalf("更新结果错误: %+v", updated)
		}
		return nil
	}()
	if err != nil {
		t.Fatalf("更新任务失败: %v", err)
	}
	if err := engine.Claim(ctx, task.ID, "dave"); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("已签收任务应冲突: %v", err)
	}
	if err := engine.Complete(ctx, task.ID, api.CompleteTaskRequest{
		UserID:    assignee,
		Variables: []api.Variable{{Name: "approved", Value: true}},
	}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	if _, err := engine.GetTask(ctx, task.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("完成后任务应不存在: %v", err)
	}
	historic, err := engine.GetHistoricTaskInstance(ctx, task.ID)
	if err != nil || historic.EndTime == nil || historic.Assignee != assignee {
		t.Fatalf("历史任务错误: %v %+v", err, historic)
	}
	variables, err := engine.ListHistoricVariableInstances(ctx, "")
	if err != nil || len(variables) == 0 {
		t.Fatalf("完成变量未记录: %v %+v", err, variables)
	}
}

// TestStandaloneTaskSubtasks 验证子任务查询与级联删除。
// @param t 测试上下文
func TestStandaloneTaskSubtasks(t *testing.T) {
	engine := newTestEngine(t)
	ctx := context.Background()
	parent, err := engine.CreateTask(ctx, api.CreateTaskRequest{Name: "Parent"})
	if err != nil {
		t.Fatalf("创建父任务失败: %v", err)
	}
	child, err := engine.CreateTask(ctx, api.CreateTaskRequest{Name: "Child", ParentTaskID: parent.ID})
	if err != nil {
		t.Fatalf("创建子任务失败: %v", err)
	}
	subtasks, err := engine.ListSubTasks(ctx, parent.ID)
	if err != nil || len(subtasks) != 1 || subtasks[0].ID != child.ID {
		t.Fatalf("子任务查询错误: %v %+v", err, subtasks)
	}
	page, err := engine.ListTasks(ctx, api.TaskFilter{ParentTaskID: parent.ID, Size: 10})
	if err != nil || page.Total != 1 {
		t.Fatalf("按父任务过滤错误: %v %+v", err, page)
	}
	if err := engine.DeleteTask(ctx, parent.ID, false, "cleanup"); err != nil {
		t.Fatalf("删除任务失败: %v", err)
	}
	if _, err := engine.GetTask(ctx, child.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("子任务应被级联删除: %v", err)
	}
	historic, err := engine.GetHistoricTaskInstance(ctx, parent.ID)
	if err != nil || historic.DeleteReason != "cleanup" || historic.EndTime == nil {
		t.Fatalf("删除后历史应保留: %v %+v", err, historic)
	}
	if err := engine.DeleteTask(ctx, parent.ID, true, "cleanup"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("重复删除应 404: %v", err)
	}

	newParent, err := engine.CreateTask(ctx, api.CreateTaskRequest{Name: "Parent2"})
	if err != nil {
		t.Fatalf("创建父任务失败: %v", err)
	}
	second, err := engine.CreateTask(ctx, api.CreateTaskRequest{Name: "Second", ParentTaskID: newParent.ID})
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	_ = engine.DeleteTask(ctx, newParent.ID, true, "")
	if _, err := engine.GetHistoricTaskInstance(ctx, second.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("级联历史删除失败: %v", err)
	}
}

// TestStandaloneTaskValidation 验证独立任务参数与租户校验。
// @param t 测试上下文
func TestStandaloneTaskValidation(t *testing.T) {
	engine := newTestEngine(t)
	engine.cfg.TenantID = "t1"
	ctx := context.Background()
	if _, err := engine.CreateTask(ctx, api.CreateTaskRequest{Name: "  "}); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("空名称应 400: %v", err)
	}
	if _, err := engine.CreateTask(ctx, api.CreateTaskRequest{Name: "x", ParentTaskID: "missing"}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("父任务不存在应 404: %v", err)
	}
	parent, err := engine.CreateTask(ctx, api.CreateTaskRequest{Name: "Parent", TenantID: "t1"})
	if err != nil {
		t.Fatalf("创建租户任务失败: %v", err)
	}
	if _, err := engine.CreateTask(ctx, api.CreateTaskRequest{Name: "Child", ParentTaskID: parent.ID, TenantID: "t2"}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户子任务应冲突: %v", err)
	}
	if _, err := engine.GetTask(tenant.With(ctx, "t2"), parent.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户查询应 404: %v", err)
	}
	if _, err := engine.ListSubTasks(ctx, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("父任务不存在应 404: %v", err)
	}
	if _, err := engine.UpdateTask(ctx, "missing", api.TaskUpdate{}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("更新缺失任务应 404: %v", err)
	}
	if err := engine.DeleteTask(ctx, "missing", false, ""); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("删除缺失任务应 404: %v", err)
	}
}

// TestDeleteProcessTaskForbidden 验证流程内任务不可直接删除。
// @param t 测试上下文
func TestDeleteProcessTaskForbidden(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) == 0 {
		t.Fatalf("任务为空")
	}
	if err := engine.DeleteTask(context.Background(), tasks[0].ID, false, ""); !errors.Is(err, api.ErrForbidden) {
		t.Fatalf("流程任务删除应 403: %v", err)
	}
}

// TestStandaloneTaskDetails 验证字段更新、租户与历史清理。
// @param t 测试上下文
func TestStandaloneTaskDetails(t *testing.T) {
	engine := newTestEngine(t)
	engine.cfg.TenantID = "t1"
	ctx := context.Background()
	task, err := engine.CreateTask(ctx, api.CreateTaskRequest{Name: "Task", TenantID: "t1"})
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	foreign := tenant.With(ctx, "t2")
	if _, err := engine.CreateTask(foreign, api.CreateTaskRequest{Name: "x", TenantID: "t1"}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户创建应 404: %v", err)
	}
	if _, err := engine.UpdateTask(foreign, task.ID, api.TaskUpdate{}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户更新应 404: %v", err)
	}
	if err := engine.DeleteTask(foreign, task.ID, false, ""); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户删除应 404: %v", err)
	}
	if _, err := engine.ListSubTasks(foreign, task.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户子任务应 404: %v", err)
	}

	description, owner, category, formKey, dueDate := "new desc", "owner", "cat", "form", time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	updated, err := engine.UpdateTask(ctx, task.ID, api.TaskUpdate{
		Description: &description, Owner: &owner, Category: &category, FormKey: &formKey, DueDate: &dueDate,
	})
	if err != nil || updated.Description != description || updated.Owner != owner ||
		updated.Category != category || updated.FormKey != formKey || updated.DueDate == nil {
		t.Fatalf("完整字段更新失败: %v %+v", err, updated)
	}
	if _, err := engine.AddComment(ctx, task.ID, "alice", "hello"); err != nil {
		t.Fatalf("添加评论失败: %v", err)
	}
	if _, err := engine.AddAttachment(ctx, task.ID, api.Attachment{Name: "a.txt"}); err != nil {
		t.Fatalf("添加附件失败: %v", err)
	}
	if err := engine.AddCandidateUser(ctx, task.ID, "bob"); err != nil {
		t.Fatalf("添加候选人失败: %v", err)
	}

	child, err := engine.CreateTask(ctx, api.CreateTaskRequest{Name: "Child", ParentTaskID: task.ID})
	if err != nil {
		t.Fatalf("创建子任务失败: %v", err)
	}
	if err := engine.SetTaskVariable(ctx, child.ID, "x", 1); err != nil {
		t.Fatalf("设置变量失败: %v", err)
	}
	if err := engine.SetTaskVariable(ctx, child.ID, "x", 2); err != nil {
		t.Fatalf("更新变量失败: %v", err)
	}
	if err := engine.Complete(ctx, child.ID, api.CompleteTaskRequest{
		LocalVariables: []api.Variable{{Name: "y", Value: "z"}},
	}); err != nil {
		t.Fatalf("完成子任务失败: %v", err)
	}
	variables, err := engine.ListHistoricVariableInstances(ctx, "")
	if err != nil || len(variables) < 2 {
		t.Fatalf("独立任务历史变量错误: %v %+v", err, variables)
	}

	if err := engine.DeleteTask(ctx, task.ID, true, "cleanup"); err != nil {
		t.Fatalf("级联删除失败: %v", err)
	}
	if comments, _ := engine.ListComments(ctx, task.ID); len(comments) != 0 {
		t.Fatalf("级联删除后评论应清空: %+v", comments)
	}
	if attachments, _ := engine.ListAttachments(ctx, task.ID); len(attachments) != 0 {
		t.Fatalf("级联删除后附件应清空: %+v", attachments)
	}
	if links, _ := engine.ListHistoricIdentityLinks(ctx, task.ID); len(links) != 0 {
		t.Fatalf("级联删除后身份链接应清空: %+v", links)
	}
	if logs, _ := engine.ListHistoricTaskLogs(ctx, task.ID); len(logs) != 0 {
		t.Fatalf("级联删除后任务日志应清空: %+v", logs)
	}
}

// TestStandaloneTaskPersistence 验证独立任务行导入导出。
// @param t 测试上下文
func TestStandaloneTaskPersistence(t *testing.T) {
	engine := newTestEngine(t)
	engine.cfg.TenantID = "t1"
	ctx := context.Background()
	parent, err := engine.CreateTask(ctx, api.CreateTaskRequest{Name: "Parent", TenantID: "t1"})
	if err != nil {
		t.Fatalf("创建父任务失败: %v", err)
	}
	child, err := engine.CreateTask(ctx, api.CreateTaskRequest{Name: "Child", ParentTaskID: parent.ID})
	if err != nil {
		t.Fatalf("创建子任务失败: %v", err)
	}
	rows, err := engine.ExportRuntimeRows(ctx)
	if err != nil {
		t.Fatalf("导出行失败: %v", err)
	}
	found := false
	for _, row := range rows {
		if row.Table == "ACT_RU_TASK" && row.Values["ID_"] == child.ID {
			found = true
			if row.Values["PARENT_TASK_ID_"] != parent.ID || row.Values["TENANT_ID_"] != "t1" {
				t.Fatalf("任务行字段错误: %+v", row.Values)
			}
		}
	}
	if !found {
		t.Fatalf("未导出独立任务行")
	}
	restored := newTestEngine(t)
	restored.cfg.TenantID = "t1"
	if err := restored.ImportRuntimeRows(ctx, rows); err != nil {
		t.Fatalf("导入行失败: %v", err)
	}
	restoredChild, err := restored.GetTask(ctx, child.ID)
	if err != nil || restoredChild.ParentTaskID != parent.ID || restoredChild.TenantID != "t1" {
		t.Fatalf("恢复任务错误: %v %+v", err, restoredChild)
	}
}
