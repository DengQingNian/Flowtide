package xml

import (
	"strings"
	"testing"

	"github.com/flowable-go/flowable/bpmn/model"
)

// TestParse_SubProcess 验证子流程解析。
// @param t 测试上下文
func TestParse_SubProcess(t *testing.T) {
	data := `<?xml version="1.0"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="p" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="sub"/>
    <subProcess id="sub" name="Sub">
      <startEvent id="ss"/>
      <sequenceFlow id="sf1" sourceRef="ss" targetRef="t"/>
      <userTask id="t" name="Inner"/>
      <sequenceFlow id="sf2" sourceRef="t" targetRef="se"/>
      <endEvent id="se"/>
    </subProcess>
    <sequenceFlow id="f2" sourceRef="sub" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`
	parsed, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	sub, ok := parsed.Processes[0].Elements["sub"].(*model.SubProcess)
	if !ok {
		t.Fatalf("子流程类型错误")
	}
	if sub.Name != "Sub" || len(sub.StartEvents()) != 1 {
		t.Fatalf("子流程属性错误: %+v", sub)
	}
	if _, ok := sub.GetFlowNode("t"); !ok {
		t.Fatalf("子流程内部节点未建立")
	}
	flow, _ := sub.GetSequenceFlow("sf1")
	if flow == nil || len(flow.TargetRef) == 0 {
		t.Fatalf("子流程内部连线未建立")
	}
}

// TestParse_SubProcessErrors 验证子流程错误。
// @param t 测试上下文
func TestParse_SubProcessErrors(t *testing.T) {
	cases := []struct {
		name string
		xml  string
		want string
	}{
		{"缺少开始事件", `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="s"/><subProcess id="sub"><userTask id="t"/></subProcess></process></definitions>`, "缺少开始事件"},
		{"内部连线错误", `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="s"/><subProcess id="sub"><startEvent id="ss"/><sequenceFlow id="f" sourceRef="ss" targetRef="missing"/></subProcess></process></definitions>`, "目标节点不存在"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse([]byte(tc.xml)); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("期望错误包含 %q，实际 %v", tc.want, err)
			}
		})
	}
}

// TestParse_MultiInstance 验证多实例解析。
// @param t 测试上下文
func TestParse_MultiInstance(t *testing.T) {
	data := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="p"><startEvent id="s"/><sequenceFlow id="f" sourceRef="s" targetRef="t"/>
    <userTask id="t" flowable:assignee="${approver}">
      <multiInstanceLoopCharacteristics isSequential="true" flowable:collection="${approvers}" flowable:elementVariable="approver">
        <loopCardinality>3</loopCardinality>
        <completionCondition>${nrOfCompletedInstances &gt;= 2}</completionCondition>
      </multiInstanceLoopCharacteristics>
    </userTask>
    <sequenceFlow id="f2" sourceRef="t" targetRef="e"/><endEvent id="e"/>
  </process>
</definitions>`
	parsed, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	task := parsed.Processes[0].Elements["t"].(*model.UserTask)
	loop := task.LoopCharacteristics
	if loop == nil || !loop.Sequential || loop.Collection != "${approvers}" || loop.ElementVariable != "approver" {
		t.Fatalf("多实例配置错误: %+v", loop)
	}
	if loop.LoopCardinality != "3" || loop.CompletionCondition == "" {
		t.Fatalf("多实例子元素错误: %+v", loop)
	}
}
