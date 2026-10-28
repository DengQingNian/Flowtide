package xml

import (
	"strings"
	"testing"

	"github.com/flowable-go/flowable/bpmn/model"
)

const sampleBPMN = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="demo" name="Demo" isExecutable="true">
    <documentation>示例流程</documentation>
    <startEvent id="start" name="Start">
      <timerEventDefinition>
        <timeDuration>PT5M</timeDuration>
      </timerEventDefinition>
    </startEvent>
    <sequenceFlow id="f1" sourceRef="start" targetRef="gw"/>
    <exclusiveGateway id="gw" default="f3"/>
    <sequenceFlow id="f2" sourceRef="gw" targetRef="approve">
      <conditionExpression>amount &gt; 100</conditionExpression>
    </sequenceFlow>
    <sequenceFlow id="f3" sourceRef="gw" targetRef="reject"/>
    <userTask id="approve" name="Approve" flowable:assignee="${initiator}"
              flowable:candidateUsers="alice,bob" flowable:formKey="approveForm"/>
    <userTask id="reject" name="Reject"/>
    <sequenceFlow id="f4" sourceRef="approve" targetRef="end"/>
    <sequenceFlow id="f5" sourceRef="reject" targetRef="end"/>
    <endEvent id="end" name="End"/>
  </process>
</definitions>`

// TestParse_Sample 验证基础元素与连线解析。
// @param t 测试上下文
func TestParse_Sample(t *testing.T) {
	parsed, err := Parse([]byte(sampleBPMN))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(parsed.Processes) != 1 {
		t.Fatalf("期望 1 个流程，实际 %d", len(parsed.Processes))
	}
	process := parsed.Processes[0]
	if process.ID != "demo" || process.Name != "Demo" || !process.Executable {
		t.Fatalf("流程属性错误: %+v", process)
	}
	if process.Documentation != "示例流程" {
		t.Fatalf("文档说明错误: %s", process.Documentation)
	}
	start, ok := process.Elements["start"].(*model.StartEvent)
	if !ok {
		t.Fatalf("start 元素类型错误")
	}
	if start.Timer == nil || start.Timer.TimeDuration != "PT5M" {
		t.Fatalf("定时器解析错误: %+v", start.Timer)
	}
	gateway, ok := process.Elements["gw"].(*model.ExclusiveGateway)
	if !ok || gateway.DefaultFlow != "f3" {
		t.Fatalf("网关默认流错误")
	}
	flow, _ := process.GetSequenceFlow("f3")
	if !flow.IsDefault {
		t.Fatalf("默认流未标记")
	}
	userTask, ok := process.Elements["approve"].(*model.UserTask)
	if !ok {
		t.Fatalf("用户任务类型错误")
	}
	if userTask.Assignee != "${initiator}" || len(userTask.CandidateUsers) != 2 || userTask.FormKey != "approveForm" {
		t.Fatalf("用户任务扩展属性错误: %+v", userTask)
	}
	if len(gateway.GetOutgoing()) != 2 || len(gateway.GetIncoming()) != 1 {
		t.Fatalf("连线数量错误: in=%d out=%d", len(gateway.GetIncoming()), len(gateway.GetOutgoing()))
	}
	conditionFlow, _ := process.GetSequenceFlow("f2")
	if conditionFlow.Condition != "amount > 100" {
		t.Fatalf("条件解析错误: %q", conditionFlow.Condition)
	}
}

// TestParse_MultipleProcesses 验证多流程解析。
// @param t 测试上下文
func TestParse_MultipleProcesses(t *testing.T) {
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="a" isExecutable="true"><startEvent id="s1"/><endEvent id="e1"/><sequenceFlow id="f1" sourceRef="s1" targetRef="e1"/></process>
  <process id="b" isExecutable="true"><startEvent id="s2"/><endEvent id="e2"/><sequenceFlow id="f2" sourceRef="s2" targetRef="e2"/></process>
</definitions>`
	parsed, err := Parse([]byte(xml))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(parsed.Processes) != 2 {
		t.Fatalf("期望 2 个流程，实际 %d", len(parsed.Processes))
	}
}

// TestParse_Errors 验证错误场景。
// @param t 测试上下文
func TestParse_Errors(t *testing.T) {
	cases := []struct {
		name string
		xml  string
		want string
	}{
		{"空内容", "", "BPMN 内容为空"},
		{"根元素错误", `<foo/>`, "根元素必须是 definitions"},
		{"缺少流程", `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"/>`, "未找到 process 元素"},
		{"缺少流程 ID", `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process><startEvent id="s"/></process></definitions>`, "process 缺少 id"},
		{"缺少开始事件", `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><endEvent id="e"/></process></definitions>`, "缺少开始事件"},
		{"连线目标不存在", `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="s"/><sequenceFlow id="f" sourceRef="s" targetRef="missing"/></process></definitions>`, "目标节点不存在"},
		{"DTD", `<!DOCTYPE foo [<!ENTITY xxe SYSTEM "file:///etc/passwd">]><definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="s"/></process></definitions>`, "DTD"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.xml))
			if err == nil {
				t.Fatalf("期望错误")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("期望错误包含 %q，实际 %v", tc.want, err)
			}
		})
	}
}

// TestParse_LinkEvents 验证链接事件解析。
// @param t 测试上下文
func TestParse_LinkEvents(t *testing.T) {
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p" isExecutable="true"><startEvent id="s"/><intermediateThrowEvent id="throw"><linkEventDefinition name="toTask"/></intermediateThrowEvent><intermediateCatchEvent id="catch"><linkEventDefinition name="toTask"/></intermediateCatchEvent><endEvent id="e"/></process></definitions>`
	parsed, err := Parse([]byte(xml))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	process := parsed.Processes[0]
	throw, ok := process.Elements["throw"].(*model.IntermediateThrowEvent)
	if !ok || throw.LinkName != "toTask" {
		t.Fatalf("抛出链接解析错误: %+v", throw)
	}
	catch, ok := process.Elements["catch"].(*model.IntermediateCatchEvent)
	if !ok || catch.LinkName != "toTask" {
		t.Fatalf("捕获链接解析错误: %+v", catch)
	}
}

// TestParse_CandidateStarters 验证候选发起人解析。
// @param t 测试上下文
func TestParse_CandidateStarters(t *testing.T) {
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="p" isExecutable="true" flowable:candidateStarterUsers="alice, bob" flowable:candidateStarterGroups="g1,,g2"><startEvent id="s"/></process></definitions>`
	parsed, err := Parse([]byte(xml))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(parsed.Processes) != 1 {
		t.Fatalf("流程数量错误")
	}
	process := parsed.Processes[0]
	if len(process.CandidateStarterUsers) != 2 || process.CandidateStarterUsers[0] != "alice" || process.CandidateStarterUsers[1] != "bob" {
		t.Fatalf("候选用户解析错误: %+v", process.CandidateStarterUsers)
	}
	if len(process.CandidateStarterGroups) != 2 || process.CandidateStarterGroups[1] != "g2" {
		t.Fatalf("候选组解析错误: %+v", process.CandidateStarterGroups)
	}
}

// TestParse_DuplicateID 验证重复 ID。
// @param t 测试上下文
func TestParse_DuplicateID(t *testing.T) {
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="same"/><endEvent id="same"/></process></definitions>`
	_, err := Parse([]byte(xml))
	if err == nil || !strings.Contains(err.Error(), "元素 ID 重复") {
		t.Fatalf("期望重复 ID 错误，实际 %v", err)
	}
}
