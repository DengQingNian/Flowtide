package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// 覆盖补齐语料：会签节点后继为网关（触发不支持撤回的工厂兜底分支）。
const rollbackMIGatewayXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="rollbackMIGateway" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="review"/>
    <userTask id="review" name="review" flowable:assignee="${executor}">
      <multiInstanceLoopCharacteristics flowable:collection="${executorList}" flowable:elementVariable="executor"/>
    </userTask>
    <sequenceFlow id="f2" sourceRef="review" targetRef="g"/>
    <exclusiveGateway id="g"/>
    <sequenceFlow id="f3" sourceRef="g" targetRef="B"/>
    <userTask id="B" name="B" flowable:assignee="bob"/>
    <sequenceFlow id="f4" sourceRef="B" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestBackUserTaskGuardBranches 白盒覆盖退回的实例缺失与实例结束防护。
// @param t 测试上下文
func TestBackUserTaskGuardBranches(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, backLinearXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backLinear"})
	taskA := taskOfKey(t, eng, "A")
	// 实例行缺失（白盒：暂存后删除再校验）
	saved := eng.state.Instances[instance.ID]
	delete(eng.state.Instances, instance.ID)
	if _, err := eng.BackUserTaskToActivity(context.Background(), taskA.ID, "B"); !strings.Contains(err.Error(), "流程实例不存在") {
		t.Fatalf("实例缺失应拒绝: %v", err)
	}
	eng.state.Instances[instance.ID] = saved
	// 实例已结束但任务行仍存在（白盒构造）
	eng.state.Instances[instance.ID].Ended = true
	if _, err := eng.BackUserTaskToActivity(context.Background(), taskA.ID, "B"); err == nil || !strings.Contains(err.Error(), "流程实例已结束") {
		t.Fatalf("实例结束应拒绝: %v", err)
	}
	_ = fake
}

// TestRollbackEndedInstance 验证实例已结束后撤回拒绝。
// @param t 测试上下文
func TestRollbackEndedInstance(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, backLinearXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backLinear"})
	_ = instance
	taskA := taskOfKey(t, eng, "A")
	completeByKey(t, eng, fake, "A", "alice")
	completeByKey(t, eng, fake, "B", "bob")
	completeByKey(t, eng, fake, "C", "carl")
	tick(fake)
	if err := eng.RollbackTask(context.Background(), taskA.ID, "carol"); err == nil || !strings.Contains(err.Error(), "该流程已完成") {
		t.Fatalf("实例结束应拒绝撤回: %v", err)
	}
}

// TestRollbackUnsupportedStrategy 验证已完成会签且后继为网关时撤回拒绝（工厂兜底分支）。
// @param t 测试上下文
func TestRollbackUnsupportedStrategy(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackMIGatewayXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "rollbackMIGateway",
		Variables:            []api.Variable{{Name: "executorList", Value: []string{"a"}}},
	})
	_ = instance
	completeByAssignee(t, eng, fake, "review", "a")
	// 会签已完成且后继为网关（B 在办，实例未结束）：工厂无匹配策略
	hisA := firstHistoricTaskID(t, eng, instance.ID, "review", "a")
	tick(fake)
	if err := eng.RollbackTask(context.Background(), hisA, "carol"); err == nil || !strings.Contains(err.Error(), "当前流程节点不支持撤回") {
		t.Fatalf("不支持策略应拒绝: %v", err)
	}
}

// TestRollbackNextSubProcessGuard 验证子流程内已出现更晚完成任务时撤回拒绝。
// @param t 测试上下文
func TestRollbackNextSubProcessGuard(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, backSubProcessXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backSubProcess"})
	_ = instance
	taskA := taskOfKey(t, eng, "A")
	completeByKey(t, eng, fake, "A", "alice")
	// sT1 已完成、sT2 在办：A 撤回被子流程内完成任务拦截
	completeByKey(t, eng, fake, "sT1", "bob")
	tick(fake)
	if err := eng.RollbackTask(context.Background(), taskA.ID, "carol"); err == nil || !strings.Contains(err.Error(), "任务已被处理") {
		t.Fatalf("子流程内已有完成任务应拒绝: %v", err)
	}
}

// TestRollbackParallelRestoreFromHistory 白盒覆盖并行会签撤回的历史变量恢复路径与错位防护。
// @param t 测试上下文
func TestRollbackParallelRestoreFromHistory(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackParallelMixml)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "rollbackParaMI",
		Variables:            []api.Variable{{Name: "executorList", Value: []string{"a", "b"}}},
	})
	completeByAssignee(t, eng, fake, "review", "a")
	hisA := firstHistoricTaskID(t, eng, instance.ID, "review", "a")
	hisExec := eng.state.HistoricTasks[hisA].ExecutionID
	// 白盒：补写该子实例的历史变量（模拟 Java 历史变量可查的场景）
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	eng.state.HistoricVariables["his-exec-loop"] = &api.HistoricVariableInstance{ID: "his-exec-loop", Name: "loopCounter", ProcessInstanceID: instance.ID, ExecutionID: hisExec, Value: 0, CreateTime: now, LastUpdatedTime: now}
	eng.state.HistoricVariables["his-exec-exec"] = &api.HistoricVariableInstance{ID: "his-exec-exec", Name: "executor", ProcessInstanceID: instance.ID, ExecutionID: hisExec, Value: "a", CreateTime: now, LastUpdatedTime: now}
	// 白盒：补充任务局部变量行，验证 historicVariablesOfExecution 过滤 TaskID 非空行
	eng.state.HistoricVariables["his-exec-task"] = &api.HistoricVariableInstance{ID: "his-exec-task", Name: "loopCounter", ProcessInstanceID: instance.ID, ExecutionID: hisExec, TaskID: "t1", Value: 99, CreateTime: now, LastUpdatedTime: now}
	tick(fake)
	if err := eng.RollbackTask(context.Background(), hisA, "carol"); err != nil {
		t.Fatalf("历史变量恢复撤回失败: %v", err)
	}
	miRoot := findMultiInstanceRoot(t, eng, instance.ID, "review")
	if miRoot == nil || miRoot.Variables["nrOfActiveInstances"] != 2 || miRoot.Variables["nrOfCompletedInstances"] != 0 {
		t.Fatalf("历史变量恢复计数错误: %+v", miRoot.Variables)
	}
	completeByAssignee(t, eng, fake, "review", "carol")
	completeByAssignee(t, eng, fake, "review", "b")
	completeByKey(t, eng, fake, "next", "dave")
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("历史变量恢复撤回重走应正常结束")
	}
}

// TestRollbackParallelAssigneeMismatch 验证并行会签撤回执行人不在集合时拒绝。
// @param t 测试上下文
func TestRollbackParallelAssigneeMismatch(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackParallelMixml)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "rollbackParaMI",
		Variables:            []api.Variable{{Name: "executorList", Value: []string{"a", "b"}}},
	})
	completeByAssignee(t, eng, fake, "review", "a")
	hisA := firstHistoricTaskID(t, eng, instance.ID, "review", "a")
	// 白盒：历史执行人不在集合
	eng.state.HistoricTasks[hisA].Assignee = "zzz"
	tick(fake)
	if err := eng.RollbackTask(context.Background(), hisA, "carol"); err == nil || !strings.Contains(err.Error(), "缺少会签任务变量") {
		t.Fatalf("执行人不在集合应拒绝: %v", err)
	}
}

// TestRollbackCompletedMultiInstanceAssigneeMissing 验证已完成会签撤回执行人缺失时拒绝。
// @param t 测试上下文
func TestRollbackCompletedMultiInstanceAssigneeMissing(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackParallelMixml)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "rollbackParaMI",
		Variables:            []api.Variable{{Name: "executorList", Value: []string{"a", "b"}}},
	})
	completeByAssignee(t, eng, fake, "review", "a")
	completeByAssignee(t, eng, fake, "review", "b")
	hisA := firstHistoricTaskID(t, eng, instance.ID, "review", "a")
	// 白盒：历史执行人为空且历史变量缺失，兜底推导失败
	eng.state.HistoricTasks[hisA].Assignee = ""
	tick(fake)
	if err := eng.RollbackTask(context.Background(), hisA, "carol"); err == nil || !strings.Contains(err.Error(), "缺少会签任务变量") {
		t.Fatalf("执行人缺失应拒绝: %v", err)
	}
}

// TestRollbackSequentialReentryClamp 白盒覆盖串行原位重建的完成计数下限钳制。
// @param t 测试上下文
func TestRollbackSequentialReentryClamp(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackSequentialMixml)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "rollbackSeqMI",
		Variables:            []api.Variable{{Name: "executorList", Value: []string{"a", "b"}}},
	})
	completeByAssignee(t, eng, fake, "review", "a")
	hisA := firstHistoricTaskID(t, eng, instance.ID, "review", "a")
	current := taskOfKey(t, eng, "review")
	// 白盒：构造"未推进"场景并把完成计数清零，验证钳制不为负
	eng.state.HistoricTasks[hisA].ExecutionID = current.ExecutionID
	miRoot := findMultiInstanceRoot(t, eng, instance.ID, "review")
	miRoot.Variables["nrOfCompletedInstances"] = 0
	tick(fake)
	if err := eng.RollbackTask(context.Background(), hisA, "carol"); err != nil {
		t.Fatalf("钳制场景撤回失败: %v", err)
	}
	if got := miRoot.Variables["nrOfCompletedInstances"]; got != 0 {
		t.Fatalf("完成计数应钳制为 0: %v", got)
	}
}

// TestCompleteToTargetEndedGuard 白盒覆盖直送的实例结束防护。
// @param t 测试上下文
func TestCompleteToTargetEndedGuard(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, completeToTargetXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "completeToTarget"})
	taskA := taskOfKey(t, eng, "A")
	completeByKey(t, eng, fake, "A", "alice")
	completeByKey(t, eng, fake, "B", "bob")
	completeByKey(t, eng, fake, "C", "carl")
	// 白盒：实例结束后残留任务行（结束流程会清理任务，重建一行指向已结束实例）
	tick(fake)
	eng.state.Tasks[taskA.ID] = &taskState{Task: api.Task{ID: taskA.ID, ProcessInstanceID: instance.ID, ProcessDefinitionID: instance.ProcessDefinitionID, TaskDefinitionKey: "A"}, LocalVariables: map[string]any{}}
	if err := eng.CompleteToTarget(context.Background(), taskA.ID, "B", nil); err == nil || !strings.Contains(err.Error(), "流程实例已结束") {
		t.Fatalf("实例结束应拒绝直送: %v", err)
	}
}

// TestReactivateNonNodeTarget 验证复活目标为非节点元素（连线）时拒绝。
// @param t 测试上下文
func TestReactivateNonNodeTarget(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, reactivateXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "reactivate"})
	completeByKey(t, eng, fake, "A", "alice")
	completeByKey(t, eng, fake, "B", "bob")
	tick(fake)
	if err := eng.ReactivateProcessInstance(context.Background(), instance.ID, "f1", "admin"); err == nil || !strings.Contains(err.Error(), "不是流程节点") {
		t.Fatalf("非节点目标应拒绝: %v", err)
	}
}

// TestRollbackCollectOutgoingDanglingTarget 覆盖后继收集时目标不在容器内的跳过分支。
// @param t 测试上下文
func TestRollbackCollectOutgoingDanglingTarget(t *testing.T) {
	params := &rollbackParams{nextFlowIDList: map[string]bool{}, gatewayMap: map[string]model.FlowNode{}, subProcessMap: map[string]*model.SubProcess{}, callActivityMap: map[string]*model.CallActivity{}}
	// 构造一条指向容器外目标的出线
	flow := &model.SequenceFlow{BaseElement: model.BaseElement{ID: "f1"}, TargetRef: "ghost"}
	rollbackCollectOutgoing(params, map[string]model.FlowElement{}, []*model.SequenceFlow{flow})
	if !params.nextFlowIDList["f1"] || params.nextFlowIDList["ghost"] {
		t.Fatalf("悬空目标应只记录连线: %v", params.nextFlowIDList)
	}
}
