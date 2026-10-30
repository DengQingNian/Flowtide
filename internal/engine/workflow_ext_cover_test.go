package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// 覆盖补齐语料：无结束事件的流程。
const noEndEventXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="noEndEvent" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="A"/>
    <userTask id="A" name="A" flowable:assignee="alice"/>
  </process>
</definitions>`

// 覆盖补齐语料：并行网关后继为接收任务（残留网关执行清理场景）。
const rollbackGatewayResidualXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="rollbackGatewayResidual" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="A"/>
    <userTask id="A" name="A" flowable:assignee="alice"/>
    <sequenceFlow id="f2" sourceRef="A" targetRef="gp"/>
    <parallelGateway id="gp"/>
    <sequenceFlow id="f3" sourceRef="gp" targetRef="r1"/>
    <sequenceFlow id="f4" sourceRef="gp" targetRef="r2"/>
    <receiveTask id="r1"/>
    <receiveTask id="r2"/>
    <sequenceFlow id="f5" sourceRef="r1" targetRef="join"/>
    <sequenceFlow id="f6" sourceRef="r2" targetRef="join"/>
    <parallelGateway id="join"/>
    <sequenceFlow id="f7" sourceRef="join" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestBackUserTaskMoreErrors 验证退回入口其余校验：任务不存在、实例结束、实例挂起。
// @param t 测试上下文
func TestBackUserTaskMoreErrors(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, backLinearXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backLinear"})
	taskA := taskOfKey(t, eng, "A")
	// 任务不存在
	if _, err := eng.BackUserTaskToActivity(context.Background(), "missing", "A"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("任务不存在应 404: %v", err)
	}
	// 实例挂起
	if err := eng.SuspendProcessInstance(context.Background(), instance.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.BackUserTaskToActivity(context.Background(), taskOfKey(t, eng, "A").ID, "A"); err == nil || !strings.Contains(err.Error(), "挂起") {
		t.Fatalf("挂起实例应拒绝退回: %v", err)
	}
	_ = eng.ActivateProcessInstance(context.Background(), instance.ID)
	// 实例结束
	completeByKey(t, eng, fake, "A", "alice")
	completeByKey(t, eng, fake, "B", "bob")
	completeByKey(t, eng, fake, "C", "carl")
	tick(fake)
	if _, err := eng.BackUserTaskToActivity(context.Background(), taskA.ID, "A"); err == nil {
		t.Fatalf("实例结束后退回应拒绝")
	}
}

// TestCompleteToTargetMoreErrors 验证直送其余校验：挂起实例、无执行任务。
// @param t 测试上下文
func TestCompleteToTargetMoreErrors(t *testing.T) {
	eng, _ := newExtEngine(t)
	deployXML(t, eng, completeToTargetXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "completeToTarget"})
	// 挂起实例
	if err := eng.SuspendProcessInstance(context.Background(), instance.ID); err != nil {
		t.Fatal(err)
	}
	if err := eng.CompleteToTarget(context.Background(), taskOfKey(t, eng, "A").ID, "B", nil); err == nil || !strings.Contains(err.Error(), "挂起") {
		t.Fatalf("挂起实例应拒绝直送: %v", err)
	}
	_ = eng.ActivateProcessInstance(context.Background(), instance.ID)
	// 独立注入任务没有执行
	injected, err := eng.InjectUserTask(context.Background(), instance.ID, api.InjectRequest{Name: "standalone"})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.CompleteToTarget(context.Background(), injected.ID, "B", nil); err == nil {
		t.Fatalf("无执行任务直送应报错")
	}
}

// TestResetProcessErrors 验证重置校验：实例不存在、实例结束、实例挂起。
// @param t 测试上下文
func TestResetProcessErrors(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, stopResetXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backParallelPlain"})
	// 不存在
	if err := eng.ResetProcess(context.Background(), "missing", "op"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("实例不存在应 404: %v", err)
	}
	// 挂起
	if err := eng.SuspendProcessInstance(context.Background(), instance.ID); err != nil {
		t.Fatal(err)
	}
	if err := eng.ResetProcess(context.Background(), instance.ID, "op"); err == nil || !strings.Contains(err.Error(), "挂起") {
		t.Fatalf("挂起实例应拒绝重置: %v", err)
	}
	_ = eng.ActivateProcessInstance(context.Background(), instance.ID)
	// 结束
	completeByKey(t, eng, fake, "A", "alice")
	completeByKey(t, eng, fake, "B1", "bob")
	completeByKey(t, eng, fake, "B2", "carl")
	completeByKey(t, eng, fake, "C", "dave")
	if err := eng.ResetProcess(context.Background(), instance.ID, "op"); err == nil {
		t.Fatalf("已结束实例应拒绝重置")
	}
}

// TestStopProcessToEventErrors 验证软终止校验：实例不存在、无结束事件。
// @param t 测试上下文
func TestStopProcessToEventErrors(t *testing.T) {
	eng, _ := newExtEngine(t)
	deployXML(t, eng, noEndEventXML)
	deployXML(t, eng, stopResetXML)
	// 实例不存在
	if err := eng.StopProcessToEvent(context.Background(), "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("实例不存在应 404: %v", err)
	}
	// 无结束事件
	if _, err := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "noEndEvent"}); err != nil {
		t.Fatal(err)
	}
	if err := eng.StopProcessToEvent(context.Background(), listProcessInstanceIDs(t, eng)[0]); err == nil || !strings.Contains(err.Error(), "结束事件") {
		t.Fatalf("无结束事件应报错: %v", err)
	}
}

// listProcessInstanceIDs 返回全部运行中实例 ID。
// @param t 测试上下文
// @param eng 引擎
// @return 实例 ID 列表
func listProcessInstanceIDs(t *testing.T, eng *Engine) []string {
	t.Helper()
	instances, err := eng.ListProcessInstances(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(instances))
	for _, instance := range instances {
		ids = append(ids, instance.ID)
	}
	return ids
}

// TestRollbackInSubProcessTask 验证撤回任务位于子流程内的场景：重做执行继承作用域根并可重走。
// @param t 测试上下文
func TestRollbackInSubProcessTask(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, backSubProcessXML)
	var seen []string
	newRollbackHook(eng, &seen)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backSubProcess"})
	completeByKey(t, eng, fake, "A", "alice")
	hisSA := firstHistoricTaskID(t, eng, instance.ID, "sT1", "bob")
	completeByKey(t, eng, fake, "sT1", "bob")
	tick(fake)
	// 撤回子流程内 sT1（办理人传空取历史执行人）
	if err := eng.RollbackTask(context.Background(), hisSA, ""); err != nil {
		t.Fatalf("子流程内任务撤回失败: %v", err)
	}
	tasks := listTasks(t, eng)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "sT1" || tasks[0].Assignee != "bob" {
		t.Fatalf("撤回后应只剩 sT1 任务: %+v", tasks)
	}
	// 撤回任务在子流程内：重走子流程并恢复父执行
	completeByKey(t, eng, fake, "sT1", "bob")
	completeByKey(t, eng, fake, "sT2", "carl")
	completeByKey(t, eng, fake, "C", "dave")
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("子流程内撤回重走应正常结束")
	}
}

// TestRollbackGatewayResidualCleanup 验证网关后任务未生成时清理残留网关执行。
// @param t 测试上下文
func TestRollbackGatewayResidualCleanup(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackGatewayResidualXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "rollbackGatewayResidual"})
	instanceID := instance.ID
	taskA := taskOfKey(t, eng, "A")
	completeByKey(t, eng, fake, "A", "alice")
	tick(fake)
	if err := eng.RollbackTask(context.Background(), taskA.ID, "carol"); err != nil {
		t.Fatalf("残留网关撤回失败: %v", err)
	}
	// 仅剩撤回任务（未注册钩子时办理人为 BPMN 定义值）
	tasks := listTasks(t, eng)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "A" || tasks[0].Assignee != "alice" {
		t.Fatalf("残留清理撤回后应只剩 A 任务: %+v", tasks)
	}
	// 残留网关执行已清理
	for _, execution := range eng.state.Executions {
		if execution.ProcessInstanceID == instanceID && !execution.Ended && execution.ActivityID == "gp" {
			t.Fatalf("残留网关执行未清理: %+v", execution)
		}
	}
	// 撤回后重走重新分叉为两个接收等待执行
	completeByKey(t, eng, fake, "A", "alice")
	waiting := 0
	for _, execution := range eng.state.Executions {
		if execution.ProcessInstanceID == instanceID && !execution.Ended && execution.Waiting {
			waiting++
		}
	}
	if waiting != 2 {
		t.Fatalf("重走后应有 2 个接收等待执行，实际 %d", waiting)
	}
}

// 覆盖补齐语料：并行会签（集合变量名非 executorList）。
const rollbackParallelNoListXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="rollbackParaNoList" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="review"/>
    <userTask id="review" name="review" flowable:assignee="${executor}">
      <multiInstanceLoopCharacteristics flowable:collection="${approvers}" flowable:elementVariable="executor"/>
    </userTask>
    <sequenceFlow id="f2" sourceRef="review" targetRef="next"/>
    <userTask id="next" name="next" flowable:assignee="dave"/>
    <sequenceFlow id="f3" sourceRef="next" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestRollbackMissingExecutorList 验证会签人集合缺失时撤回拒绝。
// @param t 测试上下文
func TestRollbackMissingExecutorList(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackParallelNoListXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "rollbackParaNoList",
		// 集合变量名非 executorList，撤回时无法定位会签人集合
		Variables: []api.Variable{{Name: "approvers", Value: []string{"a", "b"}}},
	})
	completeByAssignee(t, eng, fake, "review", "a")
	hisA := firstHistoricTaskID(t, eng, instance.ID, "review", "a")
	tick(fake)
	if err := eng.RollbackTask(context.Background(), hisA, "carol"); err == nil || !strings.Contains(err.Error(), "没有可用会签参数") {
		t.Fatalf("缺少会签人集合应拒绝: %v", err)
	}
}

// TestRollbackUnknownTaskKey 验证撤回任务节点无法定位时拒绝。
// @param t 测试上下文
func TestRollbackUnknownTaskKey(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, backLinearXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backLinear"})
	_ = instance
	taskA := taskOfKey(t, eng, "A")
	completeByKey(t, eng, fake, "A", "alice")
	// 白盒改写历史任务 Key 为模型中不存在的节点
	eng.state.HistoricTasks[taskA.ID].TaskDefinitionKey = "nope"
	tick(fake)
	if err := eng.RollbackTask(context.Background(), taskA.ID, "carol"); err == nil || !strings.Contains(err.Error(), "没有获取撤回任务节点") {
		t.Fatalf("未知节点应拒绝: %v", err)
	}
}

// TestWorkflowExtValueHelpers 覆盖撤回辅助函数的类型分支。
// @param t 测试上下文
func TestWorkflowExtValueHelpers(t *testing.T) {
	eng, _ := newExtEngine(t)
	// WorkflowExtService 门面
	if eng.WorkflowExtService() == nil {
		t.Fatalf("WorkflowExtService 不应为空")
	}
	// assigneeListOf 类型分支
	if list := assigneeListOf([]string{"x"}); len(list) != 1 || list[0] != "x" {
		t.Fatalf("[]string 转换错误: %v", list)
	}
	if list := assigneeListOf([]any{"x", 1}); len(list) != 1 || list[0] != "x" {
		t.Fatalf("[]any 转换错误: %v", list)
	}
	if list := assigneeListOf("not-list"); list != nil {
		t.Fatalf("非列表应返回 nil: %v", list)
	}
	// toIntValue 类型分支
	if got := toIntValue(int64(3)); got != 3 {
		t.Fatalf("int64 转换错误: %v", got)
	}
	if got := toIntValue(float64(4)); got != 4 {
		t.Fatalf("float64 转换错误: %v", got)
	}
	if got := toIntValue("x"); got != 0 {
		t.Fatalf("非法类型应为 0: %v", got)
	}
	// indexOfAssignee
	if got := indexOfAssignee([]string{"a", "b"}, "b"); got != 1 {
		t.Fatalf("下标错误: %v", got)
	}
	if got := indexOfAssignee([]string{"a"}, "z"); got != -1 {
		t.Fatalf("缺失应 -1: %v", got)
	}
	// tasksOnExecution
	deployXML(t, eng, backLinearXML)
	_, _ = eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backLinear"})
	task := taskOfKey(t, eng, "A")
	if tasks := eng.tasksOnExecution(task.ExecutionID); len(tasks) != 1 {
		t.Fatalf("执行上应有 1 个任务: %v", tasks)
	}
	if tasks := eng.tasksOnExecution("missing"); len(tasks) != 0 {
		t.Fatalf("未知执行应无任务: %v", tasks)
	}
}

// TestGraphUtilEventSubProcess 覆盖事件子流程容器识别与嵌套容器查找。
// @param t 测试上下文
func TestGraphUtilEventSubProcess(t *testing.T) {
	process := &model.Process{ID: "p", Elements: map[string]model.FlowElement{}, Order: []string{}}
	// 事件子流程内嵌普通子流程，形成三层容器
	eventSub := &model.SubProcess{FlowNodeBase: model.FlowNodeBase{BaseElement: model.BaseElement{ID: "evSub"}}, TriggeredByEvent: true}
	eventSub.AddElement(&model.StartEvent{FlowNodeBase: model.FlowNodeBase{BaseElement: model.BaseElement{ID: "evStart"}}})
	inner := &model.SubProcess{FlowNodeBase: model.FlowNodeBase{BaseElement: model.BaseElement{ID: "inner"}}}
	inner.AddElement(&model.UserTask{FlowNodeBase: model.FlowNodeBase{BaseElement: model.BaseElement{ID: "deep"}}})
	eventSub.AddElement(inner)
	process.AddElement(eventSub)
	process.AddElement(&model.UserTask{FlowNodeBase: model.FlowNodeBase{BaseElement: model.BaseElement{ID: "outer"}}})
	// 事件子流程容器识别
	if !isInEventSubProcess(process, "evStart") {
		t.Fatalf("evStart 应位于事件子流程内")
	}
	if isInEventSubProcess(process, "outer") {
		t.Fatalf("outer 不应位于事件子流程内")
	}
	// 嵌套容器查找
	if got := containerOfElement(process, "deep"); got == nil || got.ID != "inner" {
		t.Fatalf("deep 的直接容器应为 inner: %v", got)
	}
	if got := containerOfElement(process, "inner"); got == nil || got.ID != "evSub" {
		t.Fatalf("inner 的直接容器应为 evSub: %v", got)
	}
	if got := containerOfElement(process, "outer"); got != nil {
		t.Fatalf("outer 应直属主流程: %v", got)
	}
	// 事件子流程开始事件不可作为可达起点
	evStart := mustFlowNode(t, process, "evStart")
	outer := mustFlowNode(t, process, "outer")
	if IsReachable(process, evStart, outer) {
		t.Fatalf("事件子流程开始事件不应作为可达起点")
	}
	// diffLevel 补充分支
	if got := diffLevel([]string{"m", "s1"}, []string{"m", "s2"}); got != 1 {
		t.Fatalf("分叉下标错误: %d", got)
	}
	if got := diffLevel([]string{"m"}, []string{"m", "s1"}); got != 1 {
		t.Fatalf("浅方长度错误: %d", got)
	}
}
