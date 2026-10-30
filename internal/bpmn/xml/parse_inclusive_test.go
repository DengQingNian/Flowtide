package xml

import (
	"testing"

	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// TestParse_InclusiveGateway 验证包容网关解析。
// @param t 测试上下文
func TestParse_InclusiveGateway(t *testing.T) {
	data := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="gw"/><inclusiveGateway id="gw" default="f3"/><sequenceFlow id="f2" sourceRef="gw" targetRef="a"/><sequenceFlow id="f3" sourceRef="gw" targetRef="b"/><userTask id="a"/><userTask id="b"/></process></definitions>`
	parsed, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	gateway, ok := parsed.Processes[0].Elements["gw"].(*model.InclusiveGateway)
	if !ok || gateway.DefaultFlow != "f3" {
		t.Fatalf("包容网关解析错误")
	}
	flow, _ := parsed.Processes[0].GetSequenceFlow("f3")
	if !flow.IsDefault {
		t.Fatalf("默认流未标记")
	}
}
