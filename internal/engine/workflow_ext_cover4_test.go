package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// TestBackUserTaskElementGuards 白盒覆盖退回的定义缺失、流程缺失与元素类型防护。
// @param t 测试上下文
func TestBackUserTaskElementGuards(t *testing.T) {
	eng, _ := newExtEngine(t)
	deployXML(t, eng, backLinearXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backLinear"})
	_ = instance
	taskA := taskOfKey(t, eng, "A")
	definitionID := taskA.ProcessDefinitionID
	// 定义缺失
	eng.state.Tasks[taskA.ID].Task.ProcessDefinitionID = "dead"
	if _, err := eng.BackUserTaskToActivity(context.Background(), taskA.ID, "B"); err == nil || !strings.Contains(err.Error(), "流程定义不存在") {
		t.Fatalf("定义缺失应拒绝: %v", err)
	}
	// 模型缺失（定义存在但无解析模型）
	eng.state.Tasks[taskA.ID].Task.ProcessDefinitionID = definitionID
	savedModel := eng.state.Definitions[definitionID].Model
	eng.state.Definitions[definitionID].Model = nil
	if _, err := eng.BackUserTaskToActivity(context.Background(), taskA.ID, "B"); err == nil || !strings.Contains(err.Error(), "不包含流程") {
		t.Fatalf("模型缺失应拒绝: %v", err)
	}
	// 恢复模型后：来源活动缺失
	eng.state.Definitions[definitionID].Model = savedModel
	eng.state.Tasks[taskA.ID].Task.TaskDefinitionKey = "ghost"
	if _, err := eng.BackUserTaskToActivity(context.Background(), taskA.ID, "B"); err == nil || !strings.Contains(err.Error(), "当前任务活动不存在") {
		t.Fatalf("来源活动缺失应拒绝: %v", err)
	}
	// 来源为连线（非节点）
	eng.state.Tasks[taskA.ID].Task.TaskDefinitionKey = "f1"
	if _, err := eng.BackUserTaskToActivity(context.Background(), taskA.ID, "B"); err == nil || !strings.Contains(err.Error(), "不是流程节点") {
		t.Fatalf("来源非节点应拒绝: %v", err)
	}
	// 来源为开始事件（节点但非用户任务）
	eng.state.Tasks[taskA.ID].Task.TaskDefinitionKey = "s"
	if _, err := eng.BackUserTaskToActivity(context.Background(), taskA.ID, "B"); err == nil || !strings.Contains(err.Error(), "只支持从用户任务退回") {
		t.Fatalf("来源非用户任务应拒绝: %v", err)
	}
	// 目标为连线（非节点）
	eng.state.Tasks[taskA.ID].Task.TaskDefinitionKey = "A"
	if _, err := eng.BackUserTaskToActivity(context.Background(), taskA.ID, "f1"); err == nil || !strings.Contains(err.Error(), "目标活动不是流程节点") {
		t.Fatalf("目标非节点应拒绝: %v", err)
	}
}

// TestBackUserTaskNoRealExecutions 白盒覆盖退回执行收集失败分支。
// @param t 测试上下文
func TestBackUserTaskNoRealExecutions(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, backLinearXML)
	_, _ = eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backLinear"})
	completeByKey(t, eng, fake, "A", "alice")
	taskB := taskOfKey(t, eng, "B")
	// 任务执行行缺失（来源 B 退回 A 可达，但执行收集失败）
	eng.state.Tasks[taskB.ID].ExecutionID = "dead"
	if _, err := eng.BackUserTaskToActivity(context.Background(), taskB.ID, "A"); err == nil || !strings.Contains(err.Error(), "没有找到可退回的执行") {
		t.Fatalf("执行收集失败应拒绝: %v", err)
	}
	// 任务执行存在但父链不在候选父集合内
	eng.state.Tasks[taskB.ID].ExecutionID = taskB.ExecutionID
	eng.state.Executions[taskB.ExecutionID].ParentID = "orphan"
	if _, err := eng.BackUserTaskToActivity(context.Background(), taskB.ID, "A"); err == nil || !strings.Contains(err.Error(), "没有找到可退回的执行") {
		t.Fatalf("父链断裂应拒绝: %v", err)
	}
}

// TestRollbackTaskEntryGuards 白盒覆盖撤回入口的空任务 ID 与办理人缺失防护。
// @param t 测试上下文
func TestRollbackTaskEntryGuards(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, backLinearXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backLinear"})
	taskA := taskOfKey(t, eng, "A")
	completeByKey(t, eng, fake, "A", "alice")
	// 空 ID
	if err := eng.RollbackTask(context.Background(), " ", "carol"); err == nil || !strings.Contains(err.Error(), "撤回任务Id不存在") {
		t.Fatalf("空 ID 应拒绝: %v", err)
	}
	// 历史任务与入参均无办理人
	eng.state.HistoricTasks[taskA.ID].Assignee = ""
	tick(fake)
	if err := eng.RollbackTask(context.Background(), taskA.ID, ""); err == nil || !strings.Contains(err.Error(), "撤回任务办理人不存在") {
		t.Fatalf("办理人缺失应拒绝: %v", err)
	}
	// 定义缺失
	eng.state.HistoricTasks[taskA.ID].Assignee = "alice"
	eng.state.HistoricTasks[taskA.ID].ProcessDefinitionID = "dead"
	tick(fake)
	if err := eng.RollbackTask(context.Background(), taskA.ID, "carol"); err == nil || !strings.Contains(err.Error(), "流程定义不存在") {
		t.Fatalf("定义缺失应拒绝: %v", err)
	}
	_ = instance
}

// TestRollbackCreateExecutionNoBase 白盒覆盖撤回基准执行与兜底执行均缺失的拒绝分支。
// @param t 测试上下文
func TestRollbackCreateExecutionNoBase(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, backLinearXML)
	_, _ = eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backLinear"})
	taskA := taskOfKey(t, eng, "A")
	completeByKey(t, eng, fake, "A", "alice")
	taskB := taskOfKey(t, eng, "B")
	// 白盒：历史执行失活，且现存执行不再匹配后继节点集合
	eng.state.HistoricTasks[taskA.ID].ExecutionID = "dead"
	eng.state.Executions[taskB.ExecutionID].ActivityID = "zzz"
	tick(fake)
	if err := eng.RollbackTask(context.Background(), taskA.ID, "carol"); err == nil || !strings.Contains(err.Error(), "没有找到可撤回的任务") {
		t.Fatalf("基准执行缺失应拒绝: %v", err)
	}
}

// TestRollbackSerialCurrentExecMissing 白盒覆盖串行会签当前执行缺失的拒绝分支。
// @param t 测试上下文
func TestRollbackSerialCurrentExecMissing(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackSequentialMixml)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "rollbackSeqMI",
		Variables:            []api.Variable{{Name: "executorList", Value: []string{"a", "b"}}},
	})
	completeByAssignee(t, eng, fake, "review", "a")
	hisA := firstHistoricTaskID(t, eng, instance.ID, "review", "a")
	// 白盒：当前任务指向的执行行缺失
	current := taskOfKey(t, eng, "review")
	delete(eng.state.Executions, current.ExecutionID)
	tick(fake)
	if err := eng.RollbackTask(context.Background(), hisA, "carol"); err == nil || !strings.Contains(err.Error(), "没有找到可撤回的任务") {
		t.Fatalf("当前执行缺失应拒绝: %v", err)
	}
}

// TestRollbackSerialAssigneeListMissing 白盒覆盖串行会签会签集合缺失的拒绝分支。
// @param t 测试上下文
func TestRollbackSerialAssigneeListMissing(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackSequentialMixml)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "rollbackSeqMI",
		Variables:            []api.Variable{{Name: "executorList", Value: []string{"a", "b"}}},
	})
	completeByAssignee(t, eng, fake, "review", "a")
	hisA := firstHistoricTaskID(t, eng, instance.ID, "review", "a")
	// 白盒：删除实例级会签集合
	delete(eng.state.Executions[instance.ID].Variables, "executorList")
	tick(fake)
	if err := eng.RollbackTask(context.Background(), hisA, "carol"); err == nil || !strings.Contains(err.Error(), "没有可用会签参数") {
		t.Fatalf("会签集合缺失应拒绝: %v", err)
	}
}

// 覆盖补齐语料：调用活动后继仍有任务。
const rollbackCallDoneXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="rollbackCallDoneParent" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="A"/>
    <userTask id="A" name="A" flowable:assignee="alice"/>
    <sequenceFlow id="f2" sourceRef="A" targetRef="call"/>
    <callActivity id="call" calledElement="rollbackCallDoneChild"/>
    <sequenceFlow id="f3" sourceRef="call" targetRef="C"/>
    <userTask id="C" name="C" flowable:assignee="carl"/>
    <sequenceFlow id="f4" sourceRef="C" targetRef="e"/>
    <endEvent id="e"/>
  </process>
  <process id="rollbackCallDoneChild" isExecutable="true">
    <startEvent id="cs"/>
    <sequenceFlow id="cf1" sourceRef="cs" targetRef="cT1"/>
    <userTask id="cT1" name="cT1" flowable:assignee="bob"/>
    <sequenceFlow id="cf2" sourceRef="cT1" targetRef="ce"/>
    <endEvent id="ce"/>
  </process>
</definitions>`

// TestRollbackNextCallActivityCompleted 白盒覆盖调用活动已完结后的撤回（调用执行已离开活动）。
// @param t 测试上下文
func TestRollbackNextCallActivityCompleted(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackCallDoneXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "rollbackCallDoneParent"})
	_ = instance
	taskA := taskOfKey(t, eng, "A")
	completeByKey(t, eng, fake, "A", "alice")
	// 子流程完成后调用执行已离开 call 活动并推进到 C
	completeByKey(t, eng, fake, "cT1", "bob")
	if task := taskOfKey(t, eng, "C"); task.ID == "" {
		t.Fatalf("调用完成后应有 C 任务")
	}
	tick(fake)
	if err := eng.RollbackTask(context.Background(), taskA.ID, "carol"); err != nil {
		t.Fatalf("调用活动完结撤回失败: %v", err)
	}
	// 撤回重建的 A 任务生成（与 Java 一致：调用活动策略不清理 call 之后的任务）
	tasks := listTasks(t, eng)
	var recreated *api.Task
	for i := range tasks {
		if tasks[i].TaskDefinitionKey == "A" {
			recreated = &tasks[i]
		}
	}
	if recreated == nil || recreated.Variables[rollbackTaskTypePrefixKey+instance.ID+"A"] != rollbackTypeNextCallActivity {
		t.Fatalf("撤回重建任务缺失或策略变量不符: %+v", tasks)
	}
}

// TestRollbackInternalHelpersDirect 白盒直调内部函数的防护分支。
// @param t 测试上下文
func TestRollbackInternalHelpersDirect(t *testing.T) {
	eng, _ := newExtEngine(t)
	deployXML(t, eng, rollbackParallelMixml)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "rollbackParaMI",
		Variables:            []api.Variable{{Name: "executorList", Value: []string{"a", "b"}}},
	})
	cc := &commandContext{engine: eng, ctx: context.Background(), now: time.Now(), agenda: newAgenda(1000)}
	// 静默结束：已结束执行直接返回
	execution := eng.state.Executions[instance.ID]
	eng.endExecutionSubtreeQuietly(cc, execution.ID)
	// 清理子实例：不存在的实例直接返回
	eng.destroyChildProcessInstance(cc, "missing-child")
	// 会签策略直调：无运行中会签任务时拒绝
	hisTask := &api.HistoricTaskInstance{ID: "h1", ProcessInstanceID: instance.ID, TaskDefinitionKey: "review", EndTime: cloneTime(&cc.now)}
	params := &rollbackParams{hisTask: hisTask, process: definitionModel(t, eng, "rollbackParaMI"), currentTaskElement: mustUserTask(t, definitionModel(t, eng, "rollbackParaMI"), "review")}
	if err := eng.rollbackActiveMultiInstance(cc, params, "x"); err == nil || !strings.Contains(err.Error(), "当前会签任务已经完成") {
		t.Fatalf("无会签任务应拒绝: %v", err)
	}
	// multiInstanceRootOf：任务执行缺失与父非多实例根时返回 nil
	if got := eng.multiInstanceRootOf([]*taskState{{Task: api.Task{ID: "t"}, ExecutionID: "dead"}}); got != nil {
		t.Fatalf("缺失执行应返回 nil: %v", got)
	}
	plain := &taskState{Task: api.Task{ID: "t2"}, ExecutionID: execution.ID}
	if got := eng.multiInstanceRootOf([]*taskState{plain}); got != nil {
		t.Fatalf("非多实例父应返回 nil: %v", got)
	}
	// rollbackAssigneeList：实例缺失与根缺失
	if _, _, err := eng.rollbackAssigneeList("missing-pi"); err == nil {
		t.Fatalf("实例缺失应拒绝")
	}
	// historicVariablesOfInstance 更新时间并列打破
	eng.state.HistoricVariables["u1"] = &api.HistoricVariableInstance{ID: "u1", Name: "u1", ProcessInstanceID: instance.ID, Value: "1", CreateTime: cc.now, LastUpdatedTime: cc.now}
	eng.state.HistoricVariables["u2"] = &api.HistoricVariableInstance{ID: "u2", Name: "u2", ProcessInstanceID: instance.ID, Value: "2", CreateTime: cc.now, LastUpdatedTime: cc.now.Add(time.Minute)}
	items := eng.historicVariablesOfInstance(instance.ID)
	posU1, posU2 := -1, -1
	for i, item := range items {
		if item.name == "u1" {
			posU1 = i
		}
		if item.name == "u2" {
			posU2 = i
		}
	}
	if posU1 == -1 || posU2 == -1 || posU1 > posU2 {
		t.Fatalf("更新时间排序错误: %+v", items)
	}
	// isAncestor 中途断裂返回 false
	child := &executionState{ID: "child", ParentID: "ghost"}
	if eng.isAncestor(execution, child) {
		t.Fatalf("断裂父链不应判定为祖先")
	}
	// findCallActivityExecution：调用活动为 nil 时返回 nil
	if got := eng.findCallActivityExecution(instance.ID, nil); got != nil {
		t.Fatalf("nil 调用活动应返回 nil: %v", got)
	}
}

// TestCompleteToTargetDefinitionGuards 白盒覆盖直送的定义缺失与目标类型防护。
// @param t 测试上下文
func TestCompleteToTargetDefinitionGuards(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, completeToTargetXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "completeToTarget"})
	_ = instance
	taskA := taskOfKey(t, eng, "A")
	definitionID := taskA.ProcessDefinitionID
	// 定义缺失
	eng.state.Executions[taskA.ExecutionID].DefinitionID = "dead"
	if err := eng.CompleteToTarget(context.Background(), taskA.ID, "B", nil); err == nil || !strings.Contains(err.Error(), "流程定义不存在") {
		t.Fatalf("定义缺失应拒绝: %v", err)
	}
	// 模型缺失
	eng.state.Executions[taskA.ExecutionID].DefinitionID = definitionID
	savedModel := eng.state.Definitions[definitionID].Model
	eng.state.Definitions[definitionID].Model = nil
	if err := eng.CompleteToTarget(context.Background(), taskA.ID, "B", nil); err == nil || !strings.Contains(err.Error(), "不包含流程") {
		t.Fatalf("模型缺失应拒绝: %v", err)
	}
	// 目标为连线
	eng.state.Definitions[definitionID].Model = savedModel
	if err := eng.CompleteToTarget(context.Background(), taskA.ID, "f1", nil); err == nil || !strings.Contains(err.Error(), "目标活动不是流程节点") {
		t.Fatalf("目标非节点应拒绝: %v", err)
	}
	_ = fake
}

// TestResetProcessModelGuards 白盒覆盖重置的定义缺失、模型缺失、缺开始事件与无活动执行防护。
// @param t 测试上下文
func TestResetProcessModelGuards(t *testing.T) {
	eng, _ := newExtEngine(t)
	deployXML(t, eng, completeToTargetXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "completeToTarget"})
	definitionID := instance.ProcessDefinitionID
	_ = instance
	// 定义缺失
	eng.state.Instances[instance.ID].ProcessDefinitionID = "dead"
	if err := eng.ResetProcess(context.Background(), instance.ID, "op"); err == nil || !strings.Contains(err.Error(), "流程定义不存在") {
		t.Fatalf("定义缺失应拒绝: %v", err)
	}
	// 模型缺失
	eng.state.Instances[instance.ID].ProcessDefinitionID = definitionID
	savedModel := eng.state.Definitions[definitionID].Model
	eng.state.Definitions[definitionID].Model = nil
	if err := eng.ResetProcess(context.Background(), instance.ID, "op"); err == nil || !strings.Contains(err.Error(), "不包含流程") {
		t.Fatalf("模型缺失应拒绝: %v", err)
	}
	// 恢复模型；无活动执行
	eng.state.Definitions[definitionID].Model = savedModel
	// 无活动执行（全部执行静默结束）
	for _, execution := range eng.state.Executions {
		if execution.ProcessInstanceID == instance.ID && execution.ID != instance.ID {
			execution.Ended = true
		}
	}
	if err := eng.ResetProcess(context.Background(), instance.ID, "op"); err == nil || !strings.Contains(err.Error(), "没有可重置的活动执行") {
		t.Fatalf("无活动执行应拒绝: %v", err)
	}
	// 软终止同样拒绝无活动执行
	if err := eng.StopProcessToEvent(context.Background(), instance.ID); err == nil || !strings.Contains(err.Error(), "没有可终止的活动执行") {
		t.Fatalf("软终止无活动执行应拒绝: %v", err)
	}
}

// TestStopProcessDefinitionGuards 白盒覆盖软终止的定义缺失与模型缺失防护。
// @param t 测试上下文
func TestStopProcessDefinitionGuards(t *testing.T) {
	eng, _ := newExtEngine(t)
	deployXML(t, eng, stopResetXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backParallelPlain"})
	definitionID := instance.ProcessDefinitionID
	// 定义缺失
	eng.state.Instances[instance.ID].ProcessDefinitionID = "dead"
	if err := eng.StopProcessToEvent(context.Background(), instance.ID); err == nil || !strings.Contains(err.Error(), "流程定义不存在") {
		t.Fatalf("定义缺失应拒绝: %v", err)
	}
	// 模型缺失
	eng.state.Instances[instance.ID].ProcessDefinitionID = definitionID
	savedModel := eng.state.Definitions[definitionID].Model
	eng.state.Definitions[definitionID].Model = nil
	if err := eng.StopProcessToEvent(context.Background(), instance.ID); err == nil || !strings.Contains(err.Error(), "不包含流程") {
		t.Fatalf("模型缺失应拒绝: %v", err)
	}
	eng.state.Definitions[definitionID].Model = savedModel
}

// TestReactivateDefinitionGuards 白盒覆盖复活时落位目标防护的补充分支。
// @param t 测试上下文
func TestReactivateDefinitionGuards(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, reactivateXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "reactivate"})
	completeByKey(t, eng, fake, "A", "alice")
	completeByKey(t, eng, fake, "B", "bob")
	tick(fake)
	// 定义缺失
	eng.state.HistoricProcesses[instance.ID].ProcessDefinitionID = "dead"
	if err := eng.ReactivateProcessInstance(context.Background(), instance.ID, "", "admin"); err == nil || !strings.Contains(err.Error(), "流程定义不存在") {
		t.Fatalf("定义缺失应拒绝: %v", err)
	}
	eng.state.HistoricProcesses[instance.ID].ProcessDefinitionID = instance.ProcessDefinitionID
	// 模型缺失
	savedModel := eng.state.Definitions[instance.ProcessDefinitionID].Model
	eng.state.Definitions[instance.ProcessDefinitionID].Model = nil
	if err := eng.ReactivateProcessInstance(context.Background(), instance.ID, "", "admin"); err == nil || !strings.Contains(err.Error(), "不包含流程") {
		t.Fatalf("模型缺失应拒绝: %v", err)
	}
	eng.state.Definitions[instance.ProcessDefinitionID].Model = savedModel
	// 复活成功收尾
	if err := eng.ReactivateProcessInstance(context.Background(), instance.ID, "", "admin"); err != nil {
		t.Fatalf("复活失败: %v", err)
	}
}

// TestRollbackDeleteRuntimeTasksAfterFilter 白盒覆盖网关策略的时间过滤删除。
// @param t 测试上下文
func TestRollbackDeleteRuntimeTasksAfterFilter(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackGatewayXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "rollbackGateway",
		Variables:            []api.Variable{{Name: "x", Value: 1}},
	})
	completeByKey(t, eng, fake, "A", "alice")
	taskB := taskOfKey(t, eng, "B")
	// 白盒：撤回时间点设在后继任务创建之后，时间过滤应跳过删除
	now := time.Now().Add(2 * time.Hour)
	cc := &commandContext{engine: eng, ctx: context.Background(), now: now, agenda: newAgenda(1000)}
	definition := definitionModel(t, eng, "rollbackGateway")
	hisTask := &api.HistoricTaskInstance{ID: "h1", ProcessInstanceID: instance.ID, TaskDefinitionKey: "A", StartTime: now.Add(-3 * time.Hour), EndTime: cloneTime(&now)}
	params := &rollbackParams{hisTask: hisTask, process: definition, currentTaskElement: mustUserTask(t, definition, "A"), nextFlowIDList: map[string]bool{"B": true}}
	eng.rollbackDeleteRuntimeTasksAfter(cc, params)
	if _, ok := eng.state.Tasks[taskB.ID]; !ok {
		t.Fatalf("时间点之前的任务不应被删除")
	}
}

// mustUserTask 按ID取用户任务节点。
// @param t 测试上下文
// @param process 流程
// @param id 元素 ID
// @return 用户任务节点
func mustUserTask(t *testing.T, process *model.Process, id string) *model.UserTask {
	t.Helper()
	element, ok := findElement(process, id)
	if !ok {
		t.Fatalf("节点不存在: %s", id)
	}
	userTask, isTask := element.(*model.UserTask)
	if !isTask {
		t.Fatalf("节点不是用户任务: %s", id)
	}
	return userTask
}
