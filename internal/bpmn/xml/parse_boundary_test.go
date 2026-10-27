package xml

import (
	"strings"
	"testing"

	"github.com/flowable-go/flowable/bpmn/model"
)

// TestParse_BoundaryTimer 验证边界定时器解析。
// @param t 测试上下文
func TestParse_BoundaryTimer(t *testing.T) {
	data := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="p" isExecutable="true">
    <startEvent id="s"><documentation>开始</documentation></startEvent>
    <sequenceFlow id="f1" sourceRef="s" targetRef="task"/>
    <userTask id="task"/>
    <boundaryEvent id="b" attachedToRef="task" cancelActivity="false">
      <timerEventDefinition><timeDuration>PT5M</timeDuration></timerEventDefinition>
    </boundaryEvent>
    <sequenceFlow id="f2" sourceRef="task" targetRef="e"/>
    <sequenceFlow id="f3" sourceRef="b" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`
	parsed, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	event, ok := parsed.Processes[0].Elements["b"].(*model.BoundaryEvent)
	if !ok || event.Timer == nil || event.Timer.TimeDuration != "PT5M" {
		t.Fatalf("边界定时器解析错误: %+v", event)
	}
	if event.CancelActivity {
		t.Fatalf("cancelActivity=false 解析错误")
	}
	task := parsed.Processes[0].Elements["task"].(*model.UserTask)
	if len(task.GetBoundaryEvents()) != 1 {
		t.Fatalf("边界事件未挂载")
	}
	start := parsed.Processes[0].Elements["s"].(*model.StartEvent)
	if start.Documentation != "开始" {
		t.Fatalf("节点文档解析错误")
	}
}

// TestParse_BoundaryErrors 验证边界事件错误。
// @param t 测试上下文
func TestParse_BoundaryErrors(t *testing.T) {
	cases := []struct {
		name string
		xml  string
		want string
	}{
		{"缺少 attachedToRef", `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="s"/><boundaryEvent id="b"/><endEvent id="e"/></process></definitions>`, "attachedToRef"},
		{"依附活动不存在", `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="s"/><boundaryEvent id="b" attachedToRef="missing"/><endEvent id="e"/></process></definitions>`, "依附活动不存在"},
		{"子流程重复 ID", `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="s"/><subProcess id="sp"><startEvent id="ss"/><userTask id="ss"/><endEvent id="se"/></subProcess></process></definitions>`, "元素 ID 重复"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse([]byte(tc.xml)); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("期望错误包含 %q，实际 %v", tc.want, err)
			}
		})
	}
}

// TestParse_BoundaryMessageSignal 验证边界消息/信号解析。
// @param t 测试上下文
func TestParse_BoundaryMessageSignal(t *testing.T) {
	data := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <message id="m" name="M"/><signal id="s" name="S"/>
  <process id="p"><startEvent id="start"/><sequenceFlow id="f1" sourceRef="start" targetRef="task"/>
    <userTask id="task"/>
    <boundaryEvent id="bm" attachedToRef="task"><messageEventDefinition messageRef="m"/></boundaryEvent>
    <boundaryEvent id="bs" attachedToRef="task" cancelActivity="false"><signalEventDefinition signalRef="s"/></boundaryEvent>
    <sequenceFlow id="f2" sourceRef="task" targetRef="e"/><endEvent id="e"/>
  </process>
</definitions>`
	parsed, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	messageBoundary := parsed.Processes[0].Elements["bm"].(*model.BoundaryEvent)
	if messageBoundary.MessageRef != "m" {
		t.Fatalf("边界消息解析错误: %+v", messageBoundary)
	}
	signalBoundary := parsed.Processes[0].Elements["bs"].(*model.BoundaryEvent)
	if signalBoundary.SignalRef != "s" || signalBoundary.CancelActivity {
		t.Fatalf("边界信号解析错误: %+v", signalBoundary)
	}
}
