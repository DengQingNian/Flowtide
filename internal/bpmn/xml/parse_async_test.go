package xml

import (
	"testing"

	"github.com/flowable-go/flowable/bpmn/model"
)

// TestParse_AsyncAttributes 验证异步属性解析。
// @param t 测试上下文
func TestParse_AsyncAttributes(t *testing.T) {
	data := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="p"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="a"/>
    <serviceTask id="a" flowable:asyncBefore="true" flowable:exclusive="false"/>
    <sequenceFlow id="f2" sourceRef="a" targetRef="b"/>
    <userTask id="b" flowable:asyncAfter="true"/>
    <sequenceFlow id="f3" sourceRef="b" targetRef="c"/>
    <manualTask id="c" flowable:async="true"/>
    <sequenceFlow id="f4" sourceRef="c" targetRef="e"/><endEvent id="e"/>
  </process>
</definitions>`
	parsed, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	process := parsed.Processes[0]
	a := process.Elements["a"].(*model.ServiceTask)
	if !a.IsAsyncBefore() || a.IsExclusive() {
		t.Fatalf("服务任务异步属性错误")
	}
	b := process.Elements["b"].(*model.UserTask)
	if !b.IsAsyncAfter() || b.IsAsyncBefore() {
		t.Fatalf("用户任务异步属性错误")
	}
	c := process.Elements["c"].(*model.ManualTask)
	if !c.IsAsyncBefore() {
		t.Fatalf("async 应等价于 asyncBefore")
	}
}
