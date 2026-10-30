package xml

import (
	"testing"

	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// TestParse_EventGateway 验证事件网关解析。
// @param t 测试上下文
func TestParse_EventGateway(t *testing.T) {
	data := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="gw"/><eventBasedGateway id="gw"/><sequenceFlow id="f2" sourceRef="gw" targetRef="c"/><intermediateCatchEvent id="c"/><endEvent id="e"/></process></definitions>`
	parsed, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if _, ok := parsed.Processes[0].Elements["gw"].(*model.EventGateway); !ok {
		t.Fatalf("事件网关类型错误")
	}
}
