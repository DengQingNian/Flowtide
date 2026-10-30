package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
)

const adHocParallelXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="adhocParallel" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="adhoc"/>
    <adHocSubProcess id="adhoc" name="AdHoc" ordering="Parallel">
      <userTask id="a" name="A"/>
      <userTask id="b" name="B"/>
    </adHocSubProcess>
    <sequenceFlow id="f2" sourceRef="adhoc" targetRef="after"/>
    <userTask id="after" name="After"/>
    <sequenceFlow id="f3" sourceRef="after" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

const adHocSequentialXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="adhocSequential" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="adhoc"/>
    <adHocSubProcess id="adhoc" ordering="Sequential">
      <userTask id="s1" name="S1"/>
      <userTask id="s2" name="S2"/>
      <userTask id="s3" name="S3"/>
    </adHocSubProcess>
    <sequenceFlow id="f2" sourceRef="adhoc" targetRef="after"/>
    <userTask id="after" name="After"/>
    <sequenceFlow id="f3" sourceRef="after" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

const adHocCompletionXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="adhocCompletion" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="adhoc"/>
    <adHocSubProcess id="adhoc" ordering="Parallel">
      <userTask id="a" name="A"/>
      <userTask id="b" name="B"/>
      <completionCondition>${approved == true}</completionCondition>
    </adHocSubProcess>
    <sequenceFlow id="f2" sourceRef="adhoc" targetRef="after"/>
    <userTask id="after" name="After"/>
    <sequenceFlow id="f3" sourceRef="after" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

const adHocEmptyXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="adhocEmpty" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="adhoc"/>
    <adHocSubProcess id="adhoc"/>
    <sequenceFlow id="f2" sourceRef="adhoc" targetRef="after"/>
    <userTask id="after" name="After"/>
    <sequenceFlow id="f3" sourceRef="after" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

const adHocAttributeXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <process id="adhocAttribute" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="adhoc"/>
    <subProcess id="adhoc" adHoc="true" ordering="Sequential">
      <userTask id="x1" name="X1"/>
      <userTask id="x2" name="X2"/>
    </subProcess>
    <sequenceFlow id="f2" sourceRef="adhoc" targetRef="after"/>
    <userTask id="after" name="After"/>
    <sequenceFlow id="f3" sourceRef="after" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

const adHocInvalidOrderingXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="adhocInvalidOrdering" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="adhoc"/>
    <adHocSubProcess id="adhoc" ordering="Bogus">
      <userTask id="a" name="A"/>
    </adHocSubProcess>
    <sequenceFlow id="f2" sourceRef="adhoc" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

const adHocMissingIDXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="adhocMissingID" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="adhoc"/>
    <adHocSubProcess ordering="Parallel">
      <userTask id="a" name="A"/>
    </adHocSubProcess>
    <sequenceFlow id="f2" sourceRef="adhoc" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

const adHocConditionErrorXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="adhocConditionError" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="adhoc"/>
    <adHocSubProcess id="adhoc" ordering="Parallel">
      <userTask id="a" name="A"/>
      <completionCondition>${1 +}</completionCondition>
    </adHocSubProcess>
    <sequenceFlow id="f2" sourceRef="adhoc" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// deployAdHocXML 部署 Ad-hoc 测试流程，返回部署错误。
// @param t 测试上下文
// @param engine 引擎
// @param xml BPMN 内容
// @return 部署错误
func deployAdHocXML(t *testing.T, engine *Engine, xml string) error {
	t.Helper()
	_, err := engine.Deploy(context.Background(), api.DeploymentRequest{
		Name:      "adhoc-test",
		Resources: map[string][]byte{"adhoc.bpmn20.xml": []byte(xml)},
	})
	return err
}

// adHocTaskByKey 按任务定义 Key 查找任务。
// @param t 测试上下文
// @param engine 引擎
// @param key 任务定义 Key
// @return 任务
func adHocTaskByKey(t *testing.T, engine *Engine, key string) api.Task {
	t.Helper()
	tasks := listTasks(t, engine)
	for _, task := range tasks {
		if task.TaskDefinitionKey == key {
			return task
		}
	}
	t.Fatalf("未找到任务 %s，实际任务: %+v", key, tasks)
	return api.Task{}
}

// adHocAssertTaskKeys 断言当前任务 Key 集合。
// @param t 测试上下文
// @param engine 引擎
// @param expected 期望的任务 Key
func adHocAssertTaskKeys(t *testing.T, engine *Engine, expected ...string) {
	t.Helper()
	tasks := listTasks(t, engine)
	actual := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		actual[task.TaskDefinitionKey] = true
	}
	if len(actual) != len(expected) {
		t.Fatalf("期望任务 %v，实际 %+v", expected, tasks)
	}
	for _, key := range expected {
		if !actual[key] {
			t.Fatalf("缺少任务 %s，实际 %+v", key, tasks)
		}
	}
}

// completeAdHocTask 完成指定任务。
// @param t 测试上下文
// @param engine 引擎
// @param key 任务定义 Key
// @param variables 完成变量
func completeAdHocTask(t *testing.T, engine *Engine, key string, variables ...api.Variable) {
	t.Helper()
	task := adHocTaskByKey(t, engine, key)
	if err := engine.Complete(context.Background(), task.ID, api.CompleteTaskRequest{Variables: variables}); err != nil {
		t.Fatalf("完成任务 %s 失败: %v", key, err)
	}
}

// assertAPIError 断言错误码。
// @param t 测试上下文
// @param err 错误
// @param code 期望错误码
func assertAPIError(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("期望错误 %s，实际无错误", code)
	}
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("期望 api.Error，实际 %T: %v", err, err)
	}
	if apiErr.Code != code {
		t.Fatalf("期望错误码 %s，实际 %s", code, apiErr.Code)
	}
}

// TestAdHoc_ParallelTasksComplete 验证并行 Ad-hoc 子流程为每个内部任务创建任务，全部完成后离开。
// @param t 测试上下文
func TestAdHoc_ParallelTasksComplete(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, adHocParallelXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "adhocParallel"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	// 并行模式同时激活两个用户任务
	adHocAssertTaskKeys(t, engine, "a", "b")
	completeAdHocTask(t, engine, "a")
	adHocAssertTaskKeys(t, engine, "b")
	completeAdHocTask(t, engine, "b")
	// 全部内部任务完成后沿子流程出线继续
	adHocAssertTaskKeys(t, engine, "after")
	completeAdHocTask(t, engine, "after")
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("全部任务完成后流程应结束")
	}
}

// TestAdHoc_SequentialOrder 验证串行 Ad-hoc 子流程按文档顺序一次激活一个内部活动。
// @param t 测试上下文
func TestAdHoc_SequentialOrder(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, adHocSequentialXML)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "adhocSequential"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	// 只激活第一个内部活动
	adHocAssertTaskKeys(t, engine, "s1")
	completeAdHocTask(t, engine, "s1")
	adHocAssertTaskKeys(t, engine, "s2")
	completeAdHocTask(t, engine, "s2")
	adHocAssertTaskKeys(t, engine, "s3")
	completeAdHocTask(t, engine, "s3")
	adHocAssertTaskKeys(t, engine, "after")
	completeAdHocTask(t, engine, "after")
}

// TestAdHoc_CompletionConditionStopsRemaining 验证完成条件为真时取消剩余内部活动并离开子流程。
// @param t 测试上下文
func TestAdHoc_CompletionConditionStopsRemaining(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, adHocCompletionXML)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "adhocCompletion"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	adHocAssertTaskKeys(t, engine, "a", "b")
	// 完成 a 并设置 approved=true，完成条件为真，b 应被取消
	completeAdHocTask(t, engine, "a", api.Variable{Name: "approved", Value: true})
	adHocAssertTaskKeys(t, engine, "after")
	completeAdHocTask(t, engine, "after")
}

// TestAdHoc_CompletionConditionNotMatched 验证完成条件不满足时继续执行剩余内部活动。
// @param t 测试上下文
func TestAdHoc_CompletionConditionNotMatched(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, adHocCompletionXML)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "adhocCompletion"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	completeAdHocTask(t, engine, "a")
	adHocAssertTaskKeys(t, engine, "b")
	completeAdHocTask(t, engine, "b")
	adHocAssertTaskKeys(t, engine, "after")
	completeAdHocTask(t, engine, "after")
}

// TestAdHoc_NoActivities 验证没有内部活动的 Ad-hoc 子流程直接沿出线继续。
// @param t 测试上下文
func TestAdHoc_NoActivities(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, adHocEmptyXML)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "adhocEmpty"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	adHocAssertTaskKeys(t, engine, "after")
	completeAdHocTask(t, engine, "after")
}

// TestAdHoc_SubProcessAttribute 验证 subProcess adHoc="true" 兼容写法。
// @param t 测试上下文
func TestAdHoc_SubProcessAttribute(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, adHocAttributeXML)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "adhocAttribute"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	adHocAssertTaskKeys(t, engine, "x1")
	completeAdHocTask(t, engine, "x1")
	adHocAssertTaskKeys(t, engine, "x2")
	completeAdHocTask(t, engine, "x2")
	adHocAssertTaskKeys(t, engine, "after")
	completeAdHocTask(t, engine, "after")
}

// TestAdHoc_InvalidOrderingDeploy 验证非法 ordering 在部署阶段报错。
// @param t 测试上下文
func TestAdHoc_InvalidOrderingDeploy(t *testing.T) {
	engine := newTestEngine(t)
	assertAPIError(t, deployAdHocXML(t, engine, adHocInvalidOrderingXML), "BPMN-1010")
}

// TestAdHoc_MissingIDDeploy 验证 adHocSubProcess 缺少 id 时部署报错。
// @param t 测试上下文
func TestAdHoc_MissingIDDeploy(t *testing.T) {
	engine := newTestEngine(t)
	assertAPIError(t, deployAdHocXML(t, engine, adHocMissingIDXML), "BPMN-1011")
}

// TestAdHoc_RuntimeInvalidOrdering 验证运行时模型中的非法 ordering 被拒绝。
// @param t 测试上下文
func TestAdHoc_RuntimeInvalidOrdering(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, adHocSequentialXML)
	definition, ok := engine.findLatestDefinition("adhocSequential")
	if !ok {
		t.Fatalf("流程定义不存在")
	}
	process := processOfDefinition(definition)
	element, ok := process.Elements["adhoc"]
	if !ok {
		t.Fatalf("未找到 ad-hoc 子流程")
	}
	sub, ok := element.(*model.SubProcess)
	if !ok {
		t.Fatalf("ad-hoc 子流程类型错误: %T", element)
	}
	// 模拟持久化或外部构造的非法模型
	sub.AdHocOrdering = "Bogus"
	_, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "adhocSequential"})
	assertAPIError(t, err, "ENGINE-0513")
}

// TestAdHoc_CompletionConditionError 验证完成条件表达式错误时命令失败。
// @param t 测试上下文
func TestAdHoc_CompletionConditionError(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, adHocConditionErrorXML)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "adhocConditionError"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	task := adHocTaskByKey(t, engine, "a")
	err := engine.Complete(context.Background(), task.ID, api.CompleteTaskRequest{})
	assertAPIError(t, err, "ENGINE-0514")
}

// TestAdHoc_HelperRules 单元验证 Ad-hoc 顺序规范化与活动筛选规则。
// @param t 测试上下文
func TestAdHoc_HelperRules(t *testing.T) {
	cases := []struct {
		raw       string
		expected  string
		expectErr bool
	}{
		{raw: "", expected: adHocOrderingParallel},
		{raw: " parallel ", expected: adHocOrderingParallel},
		{raw: "SEQUENTIAL", expected: adHocOrderingSequential},
		{raw: "Bogus", expectErr: true},
	}
	for _, item := range cases {
		got, err := normalizeAdHocOrdering(item.raw)
		if item.expectErr {
			if err == nil {
				t.Fatalf("ordering %q 期望错误，实际 %q", item.raw, got)
			}
			continue
		}
		if err != nil || got != item.expected {
			t.Fatalf("ordering %q 期望 %q，实际 %q 错误 %v", item.raw, item.expected, got, err)
		}
	}
	if activities := adHocActivities(nil); activities != nil {
		t.Fatalf("nil 子流程应返回 nil，实际 %+v", activities)
	}
	// 开始/结束/边界事件与事件子流程不参与激活，普通用户任务与嵌入式子流程参与
	if isAdHocActivity(&model.StartEvent{}) || isAdHocActivity(&model.EndEvent{}) || isAdHocActivity(&model.BoundaryEvent{}) {
		t.Fatalf("事件元素不应作为 Ad-hoc 活动")
	}
	if isAdHocActivity(&model.SubProcess{TriggeredByEvent: true}) {
		t.Fatalf("事件子流程不应作为 Ad-hoc 活动")
	}
	if !isAdHocActivity(&model.SubProcess{}) || !isAdHocActivity(&model.UserTask{}) {
		t.Fatalf("普通子流程与用户任务应作为 Ad-hoc 活动")
	}
}

const adHocServiceXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <process id="adhocService" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="adhoc"/>
    <adHocSubProcess id="adhoc" ordering="Sequential">
      <serviceTask id="svc1" name="Svc1" flowable:delegateExpression="record"/>
      <serviceTask id="svc2" name="Svc2" flowable:delegateExpression="record"/>
    </adHocSubProcess>
    <sequenceFlow id="f2" sourceRef="adhoc" targetRef="after"/>
    <userTask id="after" name="After"/>
    <sequenceFlow id="f3" sourceRef="after" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// TestAdHoc_SequentialServiceTasks 验证串行 Ad-hoc 内的服务任务按文档顺序同步执行后离开。
// @param t 测试上下文
func TestAdHoc_SequentialServiceTasks(t *testing.T) {
	engine := newTestEngine(t)
	order := make([]string, 0, 2)
	engine.RegisterDelegate("record", func(ctx context.Context, execution *Execution) error {
		order = append(order, execution.state.ActivityID)
		return nil
	})
	deployXML(t, engine, adHocServiceXML)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "adhocService"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if len(order) != 2 || order[0] != "svc1" || order[1] != "svc2" {
		t.Fatalf("期望服务任务顺序 [svc1 svc2]，实际 %v", order)
	}
	adHocAssertTaskKeys(t, engine, "after")
	completeAdHocTask(t, engine, "after")
}

// TestAdHoc_MissingSubProcessDefinition 验证定义中缺少 Ad-hoc 子流程时启动报错。
// @param t 测试上下文
func TestAdHoc_MissingSubProcessDefinition(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, adHocParallelXML)
	definition, ok := engine.findLatestDefinition("adhocParallel")
	if !ok {
		t.Fatalf("流程定义不存在")
	}
	process := processOfDefinition(definition)
	// 模拟持久化/外部构造模型中缺失 Ad-hoc 子流程定义
	delete(process.Elements, "adhoc")
	_, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "adhocParallel"})
	assertAPIError(t, err, "ENGINE-0501")
}

const adHocDefaultOrderingXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="adhocDefaultOrdering" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="adhoc"/>
    <adHocSubProcess id="adhoc">
      <userTask id="d1" name="D1"/>
      <userTask id="d2" name="D2"/>
    </adHocSubProcess>
    <sequenceFlow id="f2" sourceRef="adhoc" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// TestAdHoc_DefaultOrderingIsParallel 验证未配置 ordering 时按并行激活全部内部活动。
// @param t 测试上下文
func TestAdHoc_DefaultOrderingIsParallel(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, adHocDefaultOrderingXML)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "adhocDefaultOrdering"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	adHocAssertTaskKeys(t, engine, "d1", "d2")
	completeAdHocTask(t, engine, "d1")
	completeAdHocTask(t, engine, "d2")
}
