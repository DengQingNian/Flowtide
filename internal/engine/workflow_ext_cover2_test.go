package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// 覆盖补齐语料：并行分叉部分完成（残留网关悬挂父执行清理场景）。
const rollbackParallelPartialXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="rollbackParallelPartial" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="A"/>
    <userTask id="A" name="A" flowable:assignee="alice"/>
    <sequenceFlow id="f2" sourceRef="A" targetRef="gp"/>
    <parallelGateway id="gp"/>
    <sequenceFlow id="f3" sourceRef="gp" targetRef="B"/>
    <sequenceFlow id="f4" sourceRef="gp" targetRef="C"/>
    <userTask id="B" name="B" flowable:assignee="bob"/>
    <userTask id="C" name="C" flowable:assignee="carl"/>
    <sequenceFlow id="f5" sourceRef="B" targetRef="join"/>
    <sequenceFlow id="f6" sourceRef="C" targetRef="join"/>
    <parallelGateway id="join"/>
    <sequenceFlow id="f7" sourceRef="join" targetRef="D"/>
    <userTask id="D" name="D" flowable:assignee="dave"/>
    <sequenceFlow id="f8" sourceRef="D" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// 覆盖补齐语料：多级网关。
const rollbackMultiLevelGatewayXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="rollbackMultiLevelGateway" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="A"/>
    <userTask id="A" name="A" flowable:assignee="alice"/>
    <sequenceFlow id="f2" sourceRef="A" targetRef="g1"/>
    <exclusiveGateway id="g1"/>
    <sequenceFlow id="f3" sourceRef="g1" targetRef="g2"/>
    <exclusiveGateway id="g2"/>
    <sequenceFlow id="f4" sourceRef="g2" targetRef="B"/>
    <userTask id="B" name="B" flowable:assignee="bob"/>
    <sequenceFlow id="f5" sourceRef="B" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// 覆盖补齐语料：调用活动子流程含两个串行任务。
const rollbackCallActivityTwoTaskXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="rollbackCallTwoParent" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="A"/>
    <userTask id="A" name="A" flowable:assignee="alice"/>
    <sequenceFlow id="f2" sourceRef="A" targetRef="call"/>
    <callActivity id="call" calledElement="rollbackCallTwoChild"/>
    <sequenceFlow id="f3" sourceRef="call" targetRef="e"/>
    <endEvent id="e"/>
  </process>
  <process id="rollbackCallTwoChild" isExecutable="true">
    <startEvent id="cs"/>
    <sequenceFlow id="cf1" sourceRef="cs" targetRef="cT1"/>
    <userTask id="cT1" name="cT1" flowable:assignee="bob"/>
    <sequenceFlow id="cf2" sourceRef="cT1" targetRef="cT2"/>
    <userTask id="cT2" name="cT2" flowable:assignee="carl"/>
    <sequenceFlow id="cf3" sourceRef="cT2" targetRef="ce"/>
    <endEvent id="ce"/>
  </process>
</definitions>`

// TestRollbackParallelForkPartial 验证并行分叉双分支在办时的撤回：悬挂父执行级联清理且重走自洽。
// @param t 测试上下文
func TestRollbackParallelForkPartial(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackParallelPartialXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "rollbackParallelPartial"})
	taskA := taskOfKey(t, eng, "A")
	completeByKey(t, eng, fake, "A", "alice")
	// B、C 双分支均在办时撤回 A
	tick(fake)
	if err := eng.RollbackTask(context.Background(), taskA.ID, "carol"); err != nil {
		t.Fatalf("并行分叉撤回失败: %v", err)
	}
	tasks := listTasks(t, eng)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "A" {
		t.Fatalf("撤回后应只剩 A 任务: %+v", tasks)
	}
	// 悬挂的并行父执行已级联清理
	for _, execution := range eng.state.Executions {
		if execution.ProcessInstanceID == instance.ID && !execution.Ended && execution.ActivityID == "gp" {
			t.Fatalf("悬挂并行父执行未清理: %+v", execution)
		}
	}
	// 重走：分叉、双分支、汇聚、D、结束（重做任务保留 BPMN 办理人）
	completeByKey(t, eng, fake, "A", "alice")
	completeByKey(t, eng, fake, "B", "bob")
	completeByKey(t, eng, fake, "C", "carl")
	completeByKey(t, eng, fake, "D", "dave")
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("并行分叉撤回重走应正常结束")
	}
}

// 覆盖补齐语料：并行分叉一支为接收任务（撤回后保留带活跃后代的父执行）。
const rollbackParallelReceiveBranchXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="rollbackParallelReceiveBranch" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="A"/>
    <userTask id="A" name="A" flowable:assignee="alice"/>
    <sequenceFlow id="f2" sourceRef="A" targetRef="gp"/>
    <parallelGateway id="gp"/>
    <sequenceFlow id="f3" sourceRef="gp" targetRef="B"/>
    <sequenceFlow id="f4" sourceRef="gp" targetRef="X"/>
    <userTask id="B" name="B" flowable:assignee="bob"/>
    <receiveTask id="X"/>
    <sequenceFlow id="f5" sourceRef="B" targetRef="join"/>
    <sequenceFlow id="f6" sourceRef="X" targetRef="join"/>
    <parallelGateway id="join"/>
    <sequenceFlow id="f7" sourceRef="join" targetRef="D"/>
    <userTask id="D" name="D" flowable:assignee="dave"/>
    <sequenceFlow id="f8" sourceRef="D" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestRollbackParallelKeepLiveFork 验证撤回时仍有活跃后代的父执行不被误清理，且重走后汇聚放行。
// @param t 测试上下文
func TestRollbackParallelKeepLiveFork(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackParallelReceiveBranchXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "rollbackParallelReceiveBranch"})
	taskA := taskOfKey(t, eng, "A")
	completeByKey(t, eng, fake, "A", "alice")
	// X 为接收任务无运行任务，B 任务在办；撤回 A 后父执行因 X 分支存活而保留
	tick(fake)
	if err := eng.RollbackTask(context.Background(), taskA.ID, "carol"); err != nil {
		t.Fatalf("混合分支撤回失败: %v", err)
	}
	forkAlive := false
	for _, execution := range eng.state.Executions {
		if execution.ProcessInstanceID == instance.ID && !execution.Ended && execution.ActivityID == "gp" {
			forkAlive = true
		}
	}
	if !forkAlive {
		t.Fatalf("带活跃后代的父执行不应被清理")
	}
	// 记录撤回后遗留的旧 X 等待分支（此刻仅此一个 X 执行）
	oldExecX := ""
	for _, execution := range eng.state.Executions {
		if execution.ProcessInstanceID == instance.ID && !execution.Ended && execution.Waiting && execution.ActivityID == "X" {
			oldExecX = execution.ID
		}
	}
	if oldExecX == "" {
		t.Fatalf("撤回后应遗留旧 X 等待分支")
	}
	// 重走：A 完成重新分叉，触发新 X、完成 B 后汇聚放行 D（重做任务保留 BPMN 办理人）
	completeByKey(t, eng, fake, "A", "alice")
	_ = taskOfKey(t, eng, "B")
	// 白盒收尾：静默结束遗留的旧 X 分支，保证汇聚确定性
	cc := &commandContext{engine: eng, ctx: context.Background(), now: time.Now(), agenda: newAgenda(1000)}
	eng.endExecutionSubtreeQuietly(cc, oldExecX)
	execX := ""
	for _, execution := range eng.state.Executions {
		if execution.ProcessInstanceID == instance.ID && !execution.Ended && execution.Waiting && execution.ActivityID == "X" {
			execX = execution.ID
		}
	}
	if execX == "" {
		t.Fatalf("重走后应有 X 等待执行")
	}
	if err := eng.TriggerExecution(context.Background(), execX, nil); err != nil {
		t.Fatalf("触发 X 失败: %v", err)
	}
	completeByKey(t, eng, fake, "B", "bob")
	completeByKey(t, eng, fake, "D", "dave")
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("混合分支撤回重走应正常结束")
	}
}

// TestRollbackMultiLevelGateway 验证多级网关撤回：网关间连线跳过后正常重建。
// @param t 测试上下文
func TestRollbackMultiLevelGateway(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackMultiLevelGatewayXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "rollbackMultiLevelGateway"})
	taskA := taskOfKey(t, eng, "A")
	completeByKey(t, eng, fake, "A", "alice")
	tick(fake)
	if err := eng.RollbackTask(context.Background(), taskA.ID, "carol"); err != nil {
		t.Fatalf("多级网关撤回失败: %v", err)
	}
	tasks := listTasks(t, eng)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "A" || tasks[0].Assignee != "alice" {
		t.Fatalf("多级网关撤回后应只剩 A 任务: %+v", tasks)
	}
	completeByKey(t, eng, fake, "A", "alice")
	completeByKey(t, eng, fake, "B", "bob")
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("多级网关撤回重走应正常结束")
	}
}

// TestRollbackNoCandidates 验证基准执行已失活且无现存后继执行时撤回拒绝。
// @param t 测试上下文
func TestRollbackNoCandidates(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackParallelPartialXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "rollbackParallelPartial"})
	completeByKey(t, eng, fake, "A", "alice")
	// B 完成后在汇聚点等待；撤回 B：其执行已结束且汇聚后无活动执行落在后继集合内
	completeByKey(t, eng, fake, "B", "bob")
	hisB := firstHistoricTaskID(t, eng, instance.ID, "B", "bob")
	tick(fake)
	if err := eng.RollbackTask(context.Background(), hisB, "carol"); err == nil || !strings.Contains(err.Error(), "没有找到可撤回的任务") {
		t.Fatalf("无候选执行应拒绝: %v", err)
	}
}

// TestRollbackSequentialReentryInPlace 白盒覆盖串行会签"未推进"分支：历史执行仍存活时原位重建。
// @param t 测试上下文
func TestRollbackSequentialReentryInPlace(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackSequentialMixml)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "rollbackSeqMI",
		Variables:            []api.Variable{{Name: "executorList", Value: []string{"a", "b"}}},
	})
	completeByAssignee(t, eng, fake, "review", "a")
	hisA := firstHistoricTaskID(t, eng, instance.ID, "review", "a")
	// 白盒：把历史任务的执行改写为当前存活子实例（构造"未推进"场景）
	current := taskOfKey(t, eng, "review")
	eng.state.HistoricTasks[hisA].ExecutionID = current.ExecutionID
	tick(fake)
	if err := eng.RollbackTask(context.Background(), hisA, "carol"); err != nil {
		t.Fatalf("原位重建撤回失败: %v", err)
	}
	task := taskOfKey(t, eng, "review")
	if task.Assignee != "carol" {
		t.Fatalf("原位重建后执行人应为 carol: %+v", task)
	}
	// 计数自洽：carol 取代了已完成位，完成数应回落
	miRoot := findMultiInstanceRoot(t, eng, instance.ID, "review")
	if miRoot == nil || miRoot.Variables["nrOfCompletedInstances"] != 0 {
		t.Fatalf("原位重建完成计数应为 0: %+v", miRoot.Variables)
	}
	// 重走：carol -> b -> next
	completeByAssignee(t, eng, fake, "review", "carol")
	completeByAssignee(t, eng, fake, "review", "b")
	completeByKey(t, eng, fake, "next", "dave")
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("原位重建撤回重走应正常结束")
	}
}

// TestRollbackParallelReentryInPlace 白盒覆盖并行会签"子实例未结束"兜底分支。
// @param t 测试上下文
func TestRollbackParallelReentryInPlace(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackParallelMixml)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "rollbackParaMI",
		Variables:            []api.Variable{{Name: "executorList", Value: []string{"a", "b"}}},
	})
	completeByAssignee(t, eng, fake, "review", "a")
	hisA := firstHistoricTaskID(t, eng, instance.ID, "review", "a")
	// 白盒：把历史任务的执行改写为当前存活子实例（构造兜底场景）
	current := taskOfKey(t, eng, "review")
	eng.state.HistoricTasks[hisA].ExecutionID = current.ExecutionID
	tick(fake)
	if err := eng.RollbackTask(context.Background(), hisA, "carol"); err != nil {
		t.Fatalf("兜底重建撤回失败: %v", err)
	}
	tasks := listTasks(t, eng)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "review" || tasks[0].Assignee != "carol" {
		t.Fatalf("兜底重建后应只剩 carol 任务: %+v", tasks)
	}
	// 计数自洽：子实例被原位重跑、a 仍计已完成（活跃 1、完成 1）
	miRoot := findMultiInstanceRoot(t, eng, instance.ID, "review")
	if miRoot == nil || miRoot.Variables["nrOfActiveInstances"] != 1 || miRoot.Variables["nrOfCompletedInstances"] != 1 {
		t.Fatalf("兜底重建计数错误: %+v", miRoot.Variables)
	}
	// carol 完成即达总数（其取代的是 index1 的 b 位），直接汇聚生成 next
	completeByAssignee(t, eng, fake, "review", "carol")
	completeByKey(t, eng, fake, "next", "dave")
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("兜底重建撤回重走应正常结束")
	}
}

// TestRollbackSequentialGuardsMore 补充串行会签撤回防护：历史执行人缺失与会签集合非法。
// @param t 测试上下文
func TestRollbackSequentialGuardsMore(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackSequentialMixml)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "rollbackSeqMI",
		Variables:            []api.Variable{{Name: "executorList", Value: []string{"a", "b"}}},
	})
	completeByAssignee(t, eng, fake, "review", "a")
	hisA := firstHistoricTaskID(t, eng, instance.ID, "review", "a")
	// 白盒：历史执行人为空，无法判断执行顺序
	eng.state.HistoricTasks[hisA].Assignee = ""
	tick(fake)
	if err := eng.RollbackTask(context.Background(), hisA, "carol"); err == nil || !strings.Contains(err.Error(), "没有找到历史任务执行人") {
		t.Fatalf("历史执行人缺失应拒绝: %v", err)
	}
	// 白盒：会签集合变量非法（非列表）
	completeByAssignee(t, eng, fake, "review", "b")
	instanceRoot := eng.state.Executions[instance.ID]
	instanceRoot.Variables["executorList"] = "broken"
	hisB := firstHistoricTaskID(t, eng, instance.ID, "review", "b")
	// b 已完成、next 在办，撤回 b 走已完成会签策略，集合非法即拒绝
	tick(fake)
	if err := eng.RollbackTask(context.Background(), hisB, "carol"); err == nil || !strings.Contains(err.Error(), "没有可用会签参数") {
		t.Fatalf("会签集合非法应拒绝: %v", err)
	}
}

// TestRollbackSuspendedInstance 验证挂起实例拒绝撤回。
// @param t 测试上下文
func TestRollbackSuspendedInstance(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackDefaultXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "rollbackDefault"})
	_ = instance
	taskA := taskOfKey(t, eng, "A")
	completeByKey(t, eng, fake, "A", "alice")
	if err := eng.SuspendProcessInstance(context.Background(), instance.ID); err != nil {
		t.Fatal(err)
	}
	tick(fake)
	if err := eng.RollbackTask(context.Background(), taskA.ID, "carol"); err == nil || !strings.Contains(err.Error(), "挂起") {
		t.Fatalf("挂起实例应拒绝撤回: %v", err)
	}
}

// TestRollbackCallActivityChildFinished 验证调用活动子流程已产生完成任务时拒绝撤回。
// @param t 测试上下文
func TestRollbackCallActivityChildFinished(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackCallActivityTwoTaskXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "rollbackCallTwoParent"})
	_ = instance
	taskA := taskOfKey(t, eng, "A")
	completeByKey(t, eng, fake, "A", "alice")
	// 子流程 cT1 已完成、cT2 在办：父任务 A 不可撤回
	completeByKey(t, eng, fake, "cT1", "bob")
	tick(fake)
	if err := eng.RollbackTask(context.Background(), taskA.ID, "carol"); err == nil || !strings.Contains(err.Error(), "子流程已经具有完成的任务") {
		t.Fatalf("子流程已有完成任务应拒绝: %v", err)
	}
}

// TestRollbackHelperUnits 覆盖撤回辅助函数的残余分支。
// @param t 测试上下文
func TestRollbackHelperUnits(t *testing.T) {
	// 嵌套子流程元素集合递归收集
	nested := &model.SubProcess{FlowNodeBase: model.FlowNodeBase{BaseElement: model.BaseElement{ID: "outer"}}}
	inner := &model.SubProcess{FlowNodeBase: model.FlowNodeBase{BaseElement: model.BaseElement{ID: "inner"}}}
	inner.AddElement(&model.UserTask{FlowNodeBase: model.FlowNodeBase{BaseElement: model.BaseElement{ID: "deep"}}})
	nested.AddElement(inner)
	nested.AddElement(&model.UserTask{FlowNodeBase: model.FlowNodeBase{BaseElement: model.BaseElement{ID: "shallow"}}})
	collect := map[string]bool{}
	collectSubElementIDs(nested, collect)
	// 与 Java 一致：集合只含子流程内部元素，不含子流程自身 ID
	for _, id := range []string{"inner", "deep", "shallow"} {
		if !collect[id] {
			t.Fatalf("元素集合应包含 %s: %v", id, collect)
		}
	}
	if collect["outer"] {
		t.Fatalf("元素集合不应包含子流程自身 ID")
	}
	// 空子流程安全
	collectSubElementIDs(nil, collect)
	// 撤回任务节点查找：嵌套子流程内命中（构造主流程 -> 子流程 -> 内层子流程）
	process := &model.Process{ID: "p", Elements: map[string]model.FlowElement{}, Order: []string{}}
	process.AddElement(nested)
	userTask, container := rollbackFindUserTask(process, "deep")
	if userTask == nil || container == nil || container.ID != "inner" {
		t.Fatalf("嵌套查找应返回 deep 与 inner 容器: %v %v", userTask, container)
	}
	userTask, container = rollbackFindUserTask(process, "shallow")
	if userTask == nil || container == nil || container.ID != "outer" {
		t.Fatalf("一层查找应返回 shallow 与 outer 容器: %v %v", userTask, container)
	}
	// 历史变量排序并列打破：同创建时间按更新时间与名称稳定排序
	eng, _ := newExtEngine(t)
	base := time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC)
	same := base
	eng.state.HistoricVariables["k1"] = &api.HistoricVariableInstance{ID: "k1", Name: "k1", ProcessInstanceID: "pi", Value: "v1", CreateTime: same, LastUpdatedTime: same}
	eng.state.HistoricVariables["a1"] = &api.HistoricVariableInstance{ID: "a1", Name: "a1", ProcessInstanceID: "pi", Value: "v2", CreateTime: same, LastUpdatedTime: same}
	eng.state.HistoricVariables["z9"] = &api.HistoricVariableInstance{ID: "z9", Name: "z9", ProcessInstanceID: "pi", Value: "v3", CreateTime: base.Add(time.Minute), LastUpdatedTime: same}
	items := eng.historicVariablesOfInstance("pi")
	if len(items) != 3 || items[0].name != "a1" || items[1].name != "k1" || items[2].name != "z9" {
		t.Fatalf("历史变量排序错误: %+v", items)
	}
}
