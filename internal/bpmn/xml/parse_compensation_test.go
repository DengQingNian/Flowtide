package xml

import (
	"testing"

	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// TestParse_Compensation 验证补偿边界与处理器绑定。
// @param t 测试上下文
func TestParse_Compensation(t *testing.T) {
	data := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="p"><startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="work"/>
    <serviceTask id="work" flowable:delegateExpression="${doWork}"/>
    <boundaryEvent id="comp" attachedToRef="work"><compensateEventDefinition/></boundaryEvent>
    <serviceTask id="undo" isForCompensation="true" flowable:delegateExpression="${undo}"/>
    <association id="assoc" sourceRef="comp" targetRef="undo"/>
    <sequenceFlow id="f2" sourceRef="work" targetRef="throwComp"/>
    <intermediateThrowEvent id="throwComp"><compensateEventDefinition/></intermediateThrowEvent>
    <sequenceFlow id="f3" sourceRef="throwComp" targetRef="e"/><endEvent id="e"/>
  </process>
</definitions>`
	parsed, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	process := parsed.Processes[0]
	boundary := process.Elements["comp"].(*model.BoundaryEvent)
	if !boundary.Compensate || boundary.CompensationHandlerID != "undo" {
		t.Fatalf("补偿边界解析错误: %+v", boundary)
	}
	handler := process.Elements["undo"].(*model.ServiceTask)
	if !handler.ForCompensation {
		t.Fatalf("补偿处理器标记错误")
	}
	throw := process.Elements["throwComp"].(*model.IntermediateThrowEvent)
	if !throw.Compensate {
		t.Fatalf("补偿抛出事件错误")
	}
}
