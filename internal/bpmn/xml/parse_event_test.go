package xml

import (
	"testing"

	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// TestParse_EventDefinitions 验证消息、信号与中间捕获事件解析。
// @param t 测试上下文
func TestParse_EventDefinitions(t *testing.T) {
	data := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <message id="m1" name="MessageOne"/>
  <message id="m2"/>
  <signal id="s1" name="SignalOne"/>
  <signal id="s2"/>
  <process id="p" isExecutable="true">
    <startEvent id="start1"><messageEventDefinition messageRef="m1"/></startEvent>
    <startEvent id="start2"><signalEventDefinition signalRef="s1"/></startEvent>
    <startEvent id="start3"><timerEventDefinition><timeCycle>R3/PT1M</timeCycle></timerEventDefinition></startEvent>
    <intermediateCatchEvent id="catch1"><messageEventDefinition messageRef="m2"/></intermediateCatchEvent>
    <intermediateCatchEvent id="catch2"><signalEventDefinition signalRef="s2"/></intermediateCatchEvent>
    <intermediateCatchEvent id="catch3"><timerEventDefinition><timeDate>2026-12-01T00:00:00Z</timeDate></timerEventDefinition></intermediateCatchEvent>
    <endEvent id="e"/>
  </process>
</definitions>`
	parsed, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(parsed.Messages) != 2 || parsed.Messages[0].Name != "MessageOne" {
		t.Fatalf("消息定义解析错误: %+v", parsed.Messages)
	}
	if len(parsed.Signals) != 2 || parsed.Signals[0].Name != "SignalOne" {
		t.Fatalf("信号定义解析错误: %+v", parsed.Signals)
	}
	process := parsed.Processes[0]
	start1 := process.Elements["start1"].(*model.StartEvent)
	if start1.MessageRef != "m1" {
		t.Fatalf("消息开始事件错误: %+v", start1)
	}
	start2 := process.Elements["start2"].(*model.StartEvent)
	if start2.SignalRef != "s1" {
		t.Fatalf("信号开始事件错误: %+v", start2)
	}
	start3 := process.Elements["start3"].(*model.StartEvent)
	if start3.Timer == nil || start3.Timer.TimeCycle != "R3/PT1M" {
		t.Fatalf("定时开始事件错误: %+v", start3)
	}
	catch1 := process.Elements["catch1"].(*model.IntermediateCatchEvent)
	if catch1.MessageRef != "m2" {
		t.Fatalf("消息捕获事件错误: %+v", catch1)
	}
	catch2 := process.Elements["catch2"].(*model.IntermediateCatchEvent)
	if catch2.SignalRef != "s2" {
		t.Fatalf("信号捕获事件错误: %+v", catch2)
	}
	catch3 := process.Elements["catch3"].(*model.IntermediateCatchEvent)
	if catch3.Timer == nil || catch3.Timer.TimeDate == "" {
		t.Fatalf("定时捕获事件错误: %+v", catch3)
	}
}

// TestParse_EventDefinitionEmpty 验证无事件定义的节点。
// @param t 测试上下文
func TestParse_EventDefinitionEmpty(t *testing.T) {
	data := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="s"/><intermediateCatchEvent id="c"/><endEvent id="e"/></process></definitions>`
	parsed, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	catch := parsed.Processes[0].Elements["c"].(*model.IntermediateCatchEvent)
	if catch.MessageRef != "" || catch.SignalRef != "" || catch.Timer != nil {
		t.Fatalf("空事件定义应无引用: %+v", catch)
	}
}

// TestParse_EventSubProcess 验证事件子流程标志。
// @param t 测试上下文
func TestParse_EventSubProcess(t *testing.T) {
	data := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <message id="m" name="M"/>
  <process id="p"><startEvent id="s"/>
    <subProcess id="evt" triggeredByEvent="true">
      <startEvent id="es" isInterrupting="false"><messageEventDefinition messageRef="m"/></startEvent>
      <sequenceFlow id="f" sourceRef="es" targetRef="e"/><endEvent id="e"/>
    </subProcess>
  </process>
</definitions>`
	parsed, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	sub := parsed.Processes[0].Elements["evt"].(*model.SubProcess)
	if !sub.TriggeredByEvent {
		t.Fatalf("triggeredByEvent 解析错误")
	}
	start := sub.Elements["es"].(*model.StartEvent)
	if start.Interrupting {
		t.Fatalf("isInterrupting=false 解析错误")
	}
	if start.MessageRef != "m" {
		t.Fatalf("消息引用错误")
	}
}
