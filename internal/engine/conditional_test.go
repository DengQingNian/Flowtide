package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/flowable-go/flowable/api"
)

const conditionalBoundaryXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <process id="conditionalBoundary" name="Conditional Boundary" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="review"/>
    <userTask id="review" name="Review" flowable:assignee="alice"/>
    <boundaryEvent id="amountReached" attachedToRef="review">
      <conditionalEventDefinition>
        <condition>${amount > 100}</condition>
      </conditionalEventDefinition>
    </boundaryEvent>
    <sequenceFlow id="f2" sourceRef="review" targetRef="normalEnd"/>
    <sequenceFlow id="f3" sourceRef="amountReached" targetRef="boundaryEnd"/>
    <endEvent id="normalEnd"/>
    <endEvent id="boundaryEnd"/>
  </process>
</definitions>`

const conditionalTaskVariableXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <process id="conditionalTaskVariable" name="Conditional Task Variable" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="review"/>
    <userTask id="review" name="Review" flowable:assignee="alice"/>
    <boundaryEvent id="approvedBoundary" attachedToRef="review">
      <conditionalEventDefinition>
        <condition>${approved == true}</condition>
      </conditionalEventDefinition>
    </boundaryEvent>
    <sequenceFlow id="f2" sourceRef="review" targetRef="normalEnd"/>
    <sequenceFlow id="f3" sourceRef="approvedBoundary" targetRef="boundaryEnd"/>
    <endEvent id="normalEnd"/>
    <endEvent id="boundaryEnd"/>
  </process>
</definitions>`

const conditionalCatchXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="conditionalCatch" name="Conditional Catch" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="waitForReady"/>
    <intermediateCatchEvent id="waitForReady" name="Wait For Ready">
      <conditionalEventDefinition>
        <condition>${ready == true}</condition>
      </conditionalEventDefinition>
    </intermediateCatchEvent>
    <sequenceFlow id="f2" sourceRef="waitForReady" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

const conditionalParallelXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <process id="conditionalParallel" name="Conditional Parallel" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="split"/>
    <parallelGateway id="split"/>
    <sequenceFlow id="f2" sourceRef="split" targetRef="taskA"/>
    <sequenceFlow id="f3" sourceRef="split" targetRef="taskB"/>
    <userTask id="taskA" name="A" flowable:assignee="alice"/>
    <userTask id="taskB" name="B" flowable:assignee="alice"/>
    <boundaryEvent id="amountBoundary" attachedToRef="taskA">
      <conditionalEventDefinition>
        <condition>${amount > 100}</condition>
      </conditionalEventDefinition>
    </boundaryEvent>
    <boundaryEvent id="amountBoundaryB" attachedToRef="taskB">
      <conditionalEventDefinition>
        <condition>${amount > 100}</condition>
      </conditionalEventDefinition>
    </boundaryEvent>
    <sequenceFlow id="f4" sourceRef="taskA" targetRef="endA"/>
    <sequenceFlow id="f5" sourceRef="taskB" targetRef="endB"/>
    <sequenceFlow id="f6" sourceRef="amountBoundary" targetRef="cancelEnd"/>
    <sequenceFlow id="f7" sourceRef="amountBoundaryB" targetRef="cancelEndB"/>
    <endEvent id="endA"/>
    <endEvent id="endB"/>
    <endEvent id="cancelEnd"/>
    <endEvent id="cancelEndB"/>
  </process>
</definitions>`

const conditionalInvalidXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="conditionalInvalid" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="review"/>
    <userTask id="review" name="Review"/>
    <boundaryEvent id="broken" attachedToRef="review">
      <conditionalEventDefinition>
        <condition>${amount > }</condition>
      </conditionalEventDefinition>
    </boundaryEvent>
    <sequenceFlow id="f2" sourceRef="review" targetRef="normalEnd"/>
    <sequenceFlow id="f3" sourceRef="broken" targetRef="brokenEnd"/>
    <endEvent id="normalEnd"/>
    <endEvent id="brokenEnd"/>
  </process>
</definitions>`

// countConditionSubscriptions 统计实例的条件订阅数量。
// @param engine 引擎
// @param processInstanceID 流程实例 ID
// @return 条件订阅数量
func countConditionSubscriptions(engine *Engine, processInstanceID string) int {
	count := 0
	for _, subscription := range engine.state.Subscriptions {
		if subscription.ProcessInstanceID == processInstanceID && subscription.EventType == conditionEventType {
			count++
		}
	}
	return count
}

// findTaskByKey 按活动 ID 查找运行中任务。
// @param t 测试上下文
// @param engine 引擎
// @param activityID 活动 ID
// @return 任务
func findTaskByKey(t *testing.T, engine *Engine, activityID string) api.Task {
	t.Helper()
	for _, task := range listTasks(t, engine) {
		if task.TaskDefinitionKey == activityID {
			return task
		}
	}
	t.Fatalf("未找到活动 %s 的任务", activityID)
	return api.Task{}
}

// TestConditionalBoundary_WaitsUntilVariableChanges 验证边界条件先不触发、变量变化后触发。
// @param t 测试上下文
func TestConditionalBoundary_WaitsUntilVariableChanges(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, conditionalBoundaryXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "conditionalBoundary",
		Variables:            []api.Variable{{Name: "amount", Value: 50}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if len(listTasks(t, engine)) != 1 {
		t.Fatalf("条件未满足时应保留任务")
	}
	if countConditionSubscriptions(engine, instance.ID) != 1 {
		t.Fatalf("应注册一个条件边界订阅")
	}
	if err := engine.SetVariable(context.Background(), instance.ID, "amount", 80); err != nil {
		t.Fatalf("设置变量失败: %v", err)
	}
	if len(listTasks(t, engine)) != 1 {
		t.Fatalf("条件仍不满足时应保留任务")
	}
	if err := engine.SetVariable(context.Background(), instance.ID, "amount", 200); err != nil {
		t.Fatalf("设置变量失败: %v", err)
	}
	if len(listTasks(t, engine)) != 0 {
		t.Fatalf("条件满足后应取消任务")
	}
	got, err := engine.GetProcessInstance(context.Background(), instance.ID)
	if err != nil {
		t.Fatalf("查询实例失败: %v", err)
	}
	if !got.Ended {
		t.Fatalf("边界条件路径应结束实例")
	}
	if countConditionSubscriptions(engine, instance.ID) != 0 {
		t.Fatalf("触发后应删除条件订阅")
	}
}

// TestConditionalBoundary_ImmediateTrigger 验证进入任务时条件已满足立即触发。
// @param t 测试上下文
func TestConditionalBoundary_ImmediateTrigger(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, conditionalBoundaryXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "conditionalBoundary",
		Variables:            []api.Variable{{Name: "amount", Value: 200}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if len(listTasks(t, engine)) != 0 {
		t.Fatalf("条件已满足不应创建等待任务")
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("立即触发后实例应结束")
	}
}

// TestConditionalBoundary_NonInterruptingTreatedAsInterrupting 验证非中断边界按中断语义处理。
// @param t 测试上下文
func TestConditionalBoundary_NonInterruptingTreatedAsInterrupting(t *testing.T) {
	engine := newTestEngine(t)
	xml := strings.Replace(conditionalBoundaryXML,
		`attachedToRef="review">`,
		`attachedToRef="review" cancelActivity="false">`, 1)
	deployXML(t, engine, xml)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "conditionalBoundary",
		Variables:            []api.Variable{{Name: "amount", Value: 50}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if err := engine.SetVariable(context.Background(), instance.ID, "amount", 200); err != nil {
		t.Fatalf("设置变量失败: %v", err)
	}
	if len(listTasks(t, engine)) != 0 {
		t.Fatalf("非中断条件边界应被安全地按中断处理")
	}
}

// TestConditionalBoundary_SetTaskVariableTriggers 验证任务局部变量变化可触发边界条件。
// @param t 测试上下文
func TestConditionalBoundary_SetTaskVariableTriggers(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, conditionalTaskVariableXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "conditionalTaskVariable",
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	task := findTaskByKey(t, engine, "review")
	if err := engine.SetTaskVariable(context.Background(), task.ID, "approved", true); err != nil {
		t.Fatalf("设置任务变量失败: %v", err)
	}
	if len(listTasks(t, engine)) != 0 {
		t.Fatalf("任务变量满足条件后应取消任务")
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("任务变量触发的边界路径应结束实例")
	}
}

// TestConditionalBoundary_CompleteTaskVariablesTriggerOtherBranch 验证完成任务变量触发其他分支条件。
// @param t 测试上下文
func TestConditionalBoundary_CompleteTaskVariablesTriggerOtherBranch(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, conditionalParallelXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "conditionalParallel",
		Variables:            []api.Variable{{Name: "amount", Value: 50}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if len(listTasks(t, engine)) != 2 {
		t.Fatalf("并行分支应创建两个任务")
	}
	taskB := findTaskByKey(t, engine, "taskB")
	if err := engine.Complete(context.Background(), taskB.ID, api.CompleteTaskRequest{
		Variables: []api.Variable{{Name: "amount", Value: 200}},
	}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	if len(listTasks(t, engine)) != 0 {
		t.Fatalf("完成任务写入变量后应触发另一分支的边界条件")
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("两个并行分支结束后实例应结束")
	}
}

// TestConditionalBoundary_MultipleSubscriptions 验证同实例多个条件订阅按顺序触发。
// @param t 测试上下文
func TestConditionalBoundary_MultipleSubscriptions(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, conditionalParallelXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "conditionalParallel",
		Variables:            []api.Variable{{Name: "amount", Value: 50}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if countConditionSubscriptions(engine, instance.ID) != 2 {
		t.Fatalf("两个边界事件应各注册一个条件订阅")
	}
	if err := engine.SetVariable(context.Background(), instance.ID, "amount", 200); err != nil {
		t.Fatalf("设置变量失败: %v", err)
	}
	if len(listTasks(t, engine)) != 0 {
		t.Fatalf("多个条件同时满足时应取消全部任务")
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("两个边界分支结束后实例应结束")
	}
}

// TestConditional_SuspendedInstanceDoesNotTrigger 验证挂起实例不触发条件事件。
// @param t 测试上下文
func TestConditional_SuspendedInstanceDoesNotTrigger(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, conditionalCatchXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "conditionalCatch",
		Variables:            []api.Variable{{Name: "ready", Value: false}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if err := engine.SuspendProcessInstance(context.Background(), instance.ID); err != nil {
		t.Fatalf("挂起失败: %v", err)
	}
	if err := engine.SetVariable(context.Background(), instance.ID, "ready", true); err != nil {
		t.Fatalf("挂起实例设置变量失败: %v", err)
	}
	if got, _ := engine.GetProcessInstance(context.Background(), instance.ID); got.Ended {
		t.Fatalf("挂起实例不应触发条件事件")
	}
	if countConditionSubscriptions(engine, instance.ID) != 1 {
		t.Fatalf("挂起期间应保留条件订阅")
	}
	if err := engine.ActivateProcessInstance(context.Background(), instance.ID); err != nil {
		t.Fatalf("激活失败: %v", err)
	}
	if err := engine.EvaluateConditions(context.Background(), instance.ID); err != nil {
		t.Fatalf("激活后显式求值失败: %v", err)
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("激活后显式求值应触发条件并结束实例")
	}
}

// TestConditionalIntermediateCatch_TriggersOnVariableChange 验证中间捕获条件事件等待后触发。
// @param t 测试上下文
func TestConditionalIntermediateCatch_TriggersOnVariableChange(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, conditionalCatchXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "conditionalCatch",
		Variables:            []api.Variable{{Name: "ready", Value: false}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if got, _ := engine.GetProcessInstance(context.Background(), instance.ID); got.Ended {
		t.Fatalf("条件未满足时实例不应结束")
	}
	if countConditionSubscriptions(engine, instance.ID) != 1 {
		t.Fatalf("中间捕获事件应注册条件订阅")
	}
	if err := engine.SetVariable(context.Background(), instance.ID, "ready", true); err != nil {
		t.Fatalf("设置变量失败: %v", err)
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("条件满足后中间捕获事件应继续并结束")
	}
	if countConditionSubscriptions(engine, instance.ID) != 0 {
		t.Fatalf("中间捕获事件触发后应删除订阅")
	}
}

// TestConditionalIntermediateCatch_ImmediateTrigger 验证中间捕获条件事件进入即满足。
// @param t 测试上下文
func TestConditionalIntermediateCatch_ImmediateTrigger(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, conditionalCatchXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "conditionalCatch",
		Variables:            []api.Variable{{Name: "ready", Value: true}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("立即满足的捕获条件应继续并结束")
	}
}

// TestEvaluateConditions_Explicit 验证显式求值触发条件订阅。
// @param t 测试上下文
func TestEvaluateConditions_Explicit(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, conditionalCatchXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "conditionalCatch",
		Variables:            []api.Variable{{Name: "ready", Value: false}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if err := engine.EvaluateConditions(context.Background(), instance.ID); err != nil {
		t.Fatalf("显式求值失败: %v", err)
	}
	if got, _ := engine.GetProcessInstance(context.Background(), instance.ID); got.Ended {
		t.Fatalf("条件不满足时显式求值不应推进流程")
	}
	// 直接写入变量，绕过自动求值，验证显式接口能够触发
	state, _ := engine.findInstance(instance.ID)
	root, _ := engine.rootExecution(state)
	engine.setVariable(root, "ready", true)
	if err := engine.EvaluateConditions(context.Background(), instance.ID); err != nil {
		t.Fatalf("显式求值失败: %v", err)
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("显式求值应触发条件并结束实例")
	}
}

// TestEvaluateConditions_Errors 验证显式求值的 400/404/409。
// @param t 测试上下文
func TestEvaluateConditions_Errors(t *testing.T) {
	engine := newTestEngine(t)
	if err := engine.EvaluateConditions(context.Background(), ""); err == nil || !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("空实例 ID 应返回 400 语义错误: %v", err)
	}
	if err := engine.EvaluateConditions(context.Background(), "missing"); err == nil || !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("实例不存在应返回 404 语义错误: %v", err)
	}
	deployXML(t, engine, conditionalCatchXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "conditionalCatch",
		Variables:            []api.Variable{{Name: "ready", Value: false}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if err := engine.SuspendProcessInstance(context.Background(), instance.ID); err != nil {
		t.Fatalf("挂起失败: %v", err)
	}
	if err := engine.EvaluateConditions(context.Background(), instance.ID); err == nil || !errors.Is(err, api.ErrConflict) {
		t.Fatalf("挂起实例应返回 409 语义错误: %v", err)
	}
	ended, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "conditionalCatch",
		Variables:            []api.Variable{{Name: "ready", Value: true}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if err := engine.EvaluateConditions(context.Background(), ended.ID); err == nil || !errors.Is(err, api.ErrConflict) {
		t.Fatalf("已结束实例应返回 409 语义错误: %v", err)
	}
}

// TestConditionalExpressionError 验证条件表达式错误不会被静默忽略。
// @param t 测试上下文
func TestConditionalExpressionError(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, conditionalInvalidXML)
	_, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "conditionalInvalid"})
	if err == nil || !strings.Contains(err.Error(), "条件表达式求值失败") {
		t.Fatalf("非法条件表达式应在启动时返回错误: %v", err)
	}

	deployXML(t, engine, conditionalBoundaryXML)
	instance, startErr := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "conditionalBoundary",
		Variables:            []api.Variable{{Name: "amount", Value: 50}},
	})
	if startErr != nil {
		t.Fatalf("启动失败: %v", startErr)
	}
	for _, subscription := range engine.state.Subscriptions {
		if subscription.EventType == conditionEventType && subscription.ProcessInstanceID == instance.ID {
			subscription.Condition = "${amount > }"
		}
	}
	if evalErr := engine.EvaluateConditions(context.Background(), instance.ID); evalErr == nil || !strings.Contains(evalErr.Error(), "条件表达式求值失败") {
		t.Fatalf("显式求值应返回表达式错误: %v", evalErr)
	}
}

// TestConditionalSubscription_ClearedOnTaskComplete 验证任务完成后清理条件边界订阅。
// @param t 测试上下文
func TestConditionalSubscription_ClearedOnTaskComplete(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, conditionalBoundaryXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "conditionalBoundary",
		Variables:            []api.Variable{{Name: "amount", Value: 50}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	task := findTaskByKey(t, engine, "review")
	if err := engine.Complete(context.Background(), task.ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	if countConditionSubscriptions(engine, instance.ID) != 0 {
		t.Fatalf("任务完成后应清理条件订阅")
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("正常完成任务后实例应结束")
	}
}
