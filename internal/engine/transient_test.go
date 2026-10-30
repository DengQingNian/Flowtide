package engine

import (
	"context"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

const transientCompleteXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="transientComplete" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="taskA"/>
    <userTask id="taskA" name="A"/>
    <sequenceFlow id="f2" sourceRef="taskA" targetRef="taskB">
      <conditionExpression>${approved == true}</conditionExpression>
    </sequenceFlow>
    <sequenceFlow id="f3" sourceRef="taskA" targetRef="e"/>
    <userTask id="taskB" name="B"/>
    <sequenceFlow id="f4" sourceRef="taskB" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestTransientVariablesOnComplete 验证完成任务时的瞬态变量。
// @param t 测试上下文
func TestTransientVariablesOnComplete(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, transientCompleteXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "transientComplete",
		Variables:            []api.Variable{{Name: "approved", Value: false}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 {
		t.Fatalf("初始任务错误: %+v", tasks)
	}
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{
		TransientVariables: []api.Variable{{Name: "approved", Value: true}},
	}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	next := listTasks(t, engine)
	if len(next) != 1 || next[0].TaskDefinitionKey != "taskB" {
		t.Fatalf("瞬态变量未参与条件判断: %+v", next)
	}
	value, err := engine.GetVariable(context.Background(), instance.ID, "approved")
	if err != nil || value != false {
		t.Fatalf("瞬态变量不应覆盖持久变量: %v %v", err, value)
	}
	variables, _ := engine.GetVariables(context.Background(), instance.ID)
	if _, exists := variables["approved"]; !exists || variables["approved"] != false {
		t.Fatalf("持久变量被污染: %+v", variables)
	}
	executions, _ := engine.ListExecutions(context.Background(), instance.ID)
	for _, execution := range executions {
		state := engine.state.Executions[execution.ID]
		if state.Transient != nil {
			t.Fatalf("命令结束后瞬态变量应清理: %+v", state.Transient)
		}
	}
}

// TestTransientVariablesOnTrigger 验证触发等待执行时的瞬态变量。
// @param t 测试上下文
func TestTransientVariablesOnTrigger(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, receiveTaskTriggerXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "receiveTrigger"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if err := engine.TriggerExecutionWithTransient(context.Background(), waitingActivityExecutionID(t, engine, instance.ID), nil,
		[]api.Variable{{Name: "assignee", Value: "bob"}}); err != nil {
		t.Fatalf("触发失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].Assignee != "bob" {
		t.Fatalf("瞬态变量未参与处理人解析: %+v", tasks)
	}
	variables, _ := engine.GetVariables(context.Background(), instance.ID)
	if _, exists := variables["assignee"]; exists {
		t.Fatalf("瞬态变量不应持久化: %+v", variables)
	}
}
