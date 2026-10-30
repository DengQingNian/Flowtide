package xml

import (
	"testing"

	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// TestParse_SendReceiveTask 验证发送/接收任务解析。
// @param t 测试上下文
func TestParse_SendReceiveTask(t *testing.T) {
	data := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="r"/><receiveTask id="r" messageRef="m"/><sequenceFlow id="f2" sourceRef="r" targetRef="send"/><sendTask id="send" signalRef="sig"/><sequenceFlow id="f3" sourceRef="send" targetRef="e"/><endEvent id="e"/></process></definitions>`
	parsed, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	receive := parsed.Processes[0].Elements["r"].(*model.ReceiveTask)
	if receive.MessageRef != "m" {
		t.Fatalf("接收任务解析错误: %+v", receive)
	}
	send := parsed.Processes[0].Elements["send"].(*model.SendTask)
	if send.SignalRef != "sig" {
		t.Fatalf("发送任务解析错误: %+v", send)
	}
}
