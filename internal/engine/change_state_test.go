package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

const changeStateXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="changeState" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="taskA"/>
    <userTask id="taskA" name="A" flowable:assignee="alice"/>
    <sequenceFlow id="f2" sourceRef="taskA" targetRef="e"/>
    <userTask id="taskB" name="B" flowable:assignee="alice"/>
    <sequenceFlow id="f3" sourceRef="taskB" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestChangeActivityState 验证执行活动迁移。
// @param t 测试上下文
func TestChangeActivityState(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, changeStateXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "changeState"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	executions, err := engine.ListExecutions(context.Background(), instance.ID)
	if err != nil || len(executions) == 0 {
		t.Fatalf("执行树为空: %v", err)
	}
	executionID := executions[0].ID
	if err := engine.ChangeActivityState(context.Background(), executionID, []string{"taskB"}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "taskB" {
		t.Fatalf("迁移后任务错误: %+v", tasks)
	}
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{UserID: "alice"}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("流程应结束")
	}
}

// TestChangeRootCancelsActivityTree 验证实例级迁移保留根作用域并清理被替换活动容器的全部后代。
// @param t 测试上下文
// @return 无
func TestChangeRootCancelsActivityTree(t *testing.T) {
	ctx := context.Background()
	app := newTestEngine(t)
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="root_change" isExecutable="true"><startEvent id="s"/><sequenceFlow id="a" sourceRef="s" targetRef="fork"/><parallelGateway id="fork"/><sequenceFlow id="b" sourceRef="fork" targetRef="left"/><sequenceFlow id="c" sourceRef="fork" targetRef="right"/><userTask id="left"/><userTask id="right"/><sequenceFlow id="d" sourceRef="left" targetRef="join"/><sequenceFlow id="e" sourceRef="right" targetRef="join"/><parallelGateway id="join"/><sequenceFlow id="f" sourceRef="join" targetRef="target"/><userTask id="target"/><sequenceFlow id="g" sourceRef="target" targetRef="end"/><endEvent id="end"/></process></definitions>`
	deployXML(t, app, xml)
	instance, err := app.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "root_change", Variables: []api.Variable{{Name: "root", Value: "保留"}}})
	if err != nil {
		t.Fatal(err)
	}
	before, err := app.ListTasks(ctx, api.TaskFilter{})
	if err != nil || len(before.Data) != 2 {
		t.Fatalf("并行等待未建立: %+v %v", before, err)
	}
	if err := app.ChangeProcessInstanceState(ctx, instance.ID, []string{"target"}); err != nil {
		t.Fatal(err)
	}
	after, err := app.ListTasks(ctx, api.TaskFilter{})
	if err != nil || len(after.Data) != 1 || after.Data[0].TaskDefinitionKey != "target" {
		t.Fatalf("迁移遗留旧分支: %+v %v", after, err)
	}
	root := app.state.Executions[instance.ID]
	if root.ActivityID != "" || !root.Scope || root.Variables["root"] != "保留" {
		t.Fatalf("根作用域被活动覆盖: %+v", root)
	}
	for _, old := range before.Data {
		history, err := app.GetHistoricTaskInstance(ctx, old.ID)
		if err != nil || history.EndTime == nil || history.DeleteReason != "change-state" {
			t.Fatalf("取消历史未闭合: %+v %v", history, err)
		}
	}
	if err := app.Complete(ctx, after.Data[0].ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatal(err)
	}
	if result, err := app.GetProcessInstance(ctx, instance.ID); err != nil || !result.Ended {
		t.Fatalf("迁移后的根作用域未结束: %+v %v", result, err)
	}
}

// TestChangeProcessInstanceState 验证实例级活动迁移。
// @param t 测试上下文
func TestChangeProcessInstanceState(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, changeStateXML)
	instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "changeState"})
	if err := engine.ChangeProcessInstanceState(context.Background(), instance.ID, []string{"taskB"}); err != nil {
		t.Fatalf("实例迁移失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "taskB" {
		t.Fatalf("实例迁移后任务错误: %+v", tasks)
	}
}

// TestChangeActivityStateErrors 验证活动迁移错误。
// @param t 测试上下文
func TestChangeActivityStateErrors(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, changeStateXML)
	instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "changeState"})
	executions, _ := engine.ListExecutions(context.Background(), instance.ID)
	executionID := executions[0].ID
	if err := engine.ChangeActivityState(context.Background(), "missing", []string{"taskB"}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失执行应 404: %v", err)
	}
	if err := engine.ChangeActivityState(context.Background(), executionID, nil); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("空目标应 400: %v", err)
	}
	if err := engine.ChangeActivityState(context.Background(), executionID, []string{"taskA", "taskB"}); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("多目标应 400: %v", err)
	}
	if err := engine.ChangeActivityState(context.Background(), executionID, []string{"missing"}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失活动应 404: %v", err)
	}
	if err := engine.ChangeActivityState(context.Background(), executionID, []string{"f1"}); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("非节点目标应 400: %v", err)
	}
	if err := engine.SuspendProcessInstance(context.Background(), instance.ID); err != nil {
		t.Fatalf("挂起失败: %v", err)
	}
	if err := engine.ChangeActivityState(context.Background(), executionID, []string{"taskB"}); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("挂起实例应 409: %v", err)
	}
	if err := engine.ChangeProcessInstanceState(context.Background(), "missing", []string{"taskB"}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失实例应 404: %v", err)
	}

	ended, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "changeState"})
	endedExecutions, _ := engine.ListExecutions(context.Background(), ended.ID)
	endedExecutionID := endedExecutions[0].ID
	endedTaskID := ""
	for _, task := range listTasks(t, engine) {
		if task.ProcessInstanceID == ended.ID {
			endedTaskID = task.ID
		}
	}
	if endedTaskID == "" {
		t.Fatalf("结束场景任务缺失")
	}
	if err := engine.Complete(context.Background(), endedTaskID, api.CompleteTaskRequest{UserID: "alice"}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	if err := engine.ChangeActivityState(context.Background(), endedExecutionID, []string{"taskB"}); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("已结束执行应 409: %v", err)
	}
	if err := engine.ChangeProcessInstanceState(context.Background(), ended.ID, []string{"taskB"}); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("已结束实例应 409: %v", err)
	}
}
