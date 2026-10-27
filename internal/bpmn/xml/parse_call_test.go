package xml

import (
	"testing"

	"github.com/flowable-go/flowable/bpmn/model"
)

// TestParse_CallActivity 验证调用活动解析。
// @param t 测试上下文
func TestParse_CallActivity(t *testing.T) {
	data := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="p" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="call"/>
    <callActivity id="call" calledElement="child" flowable:inheritVariables="true" flowable:businessKey="${bizKey}" flowable:processInstanceName="Child Instance">
      <extensionElements>
        <flowable:in source="a" target="b"/>
        <flowable:in sourceExpression="${c + 1}" target="d"/>
        <flowable:out source="x" target="y"/>
      </extensionElements>
    </callActivity>
    <sequenceFlow id="f2" sourceRef="call" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`
	parsed, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	call := parsed.Processes[0].Elements["call"].(*model.CallActivity)
	if call.CalledElement != "child" || !call.InheritVariables {
		t.Fatalf("调用活动属性错误: %+v", call)
	}
	if call.BusinessKey != "${bizKey}" || call.NameExpression != "Child Instance" {
		t.Fatalf("调用活动表达式错误: %+v", call)
	}
	if len(call.InParameters) != 2 || len(call.OutParameters) != 1 {
		t.Fatalf("入参出参数量错误: %+v %+v", call.InParameters, call.OutParameters)
	}
	if call.InParameters[0].Source != "a" || call.InParameters[0].Target != "b" {
		t.Fatalf("入参映射错误: %+v", call.InParameters[0])
	}
	if call.InParameters[1].SourceExpression == "" {
		t.Fatalf("入参表达式错误")
	}
}
