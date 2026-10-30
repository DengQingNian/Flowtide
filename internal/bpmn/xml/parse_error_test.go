package xml

import (
	"testing"

	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// TestParse_ErrorDefinitions 验证错误定义与引用解析。
// @param t 测试上下文
func TestParse_ErrorDefinitions(t *testing.T) {
	data := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <error id="e1" errorCode="CODE1"/>
  <process id="p" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="sp"/>
    <subProcess id="sp">
      <startEvent id="ss"/>
      <sequenceFlow id="sf" sourceRef="ss" targetRef="ee"/>
      <endEvent id="ee"><errorEventDefinition errorRef="e1"/></endEvent>
    </subProcess>
    <boundaryEvent id="b" attachedToRef="sp"><errorEventDefinition errorRef="e1"/></boundaryEvent>
    <sequenceFlow id="f2" sourceRef="sp" targetRef="e"/>
    <sequenceFlow id="f3" sourceRef="b" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`
	parsed, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(parsed.Errors) != 1 || parsed.Errors[0].ErrorCode != "CODE1" {
		t.Fatalf("错误定义解析失败: %+v", parsed.Errors)
	}
	process := parsed.Processes[0]
	sub := process.Elements["sp"].(*model.SubProcess)
	end := sub.Elements["ee"].(*model.EndEvent)
	if end.ErrorRef != "e1" {
		t.Fatalf("错误结束事件错误: %+v", end)
	}
	boundary := process.Elements["b"].(*model.BoundaryEvent)
	if boundary.ErrorRef != "e1" {
		t.Fatalf("错误边界事件错误: %+v", boundary)
	}
}
