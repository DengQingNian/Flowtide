package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/flowable-go/flowable/api"
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
