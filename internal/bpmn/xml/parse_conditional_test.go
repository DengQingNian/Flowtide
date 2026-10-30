package xml

import (
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// TestParse_ConditionalEvents 验证边界与中间捕获条件事件解析。
// @param t 测试上下文
func TestParse_ConditionalEvents(t *testing.T) {
	data := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="p" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="task"/>
    <userTask id="task"/>
    <boundaryEvent id="b" attachedToRef="task">
      <conditionalEventDefinition><condition>${amount > 100}</condition></conditionalEventDefinition>
    </boundaryEvent>
    <sequenceFlow id="f2" sourceRef="task" targetRef="catch"/>
    <intermediateCatchEvent id="catch">
      <conditionalEventDefinition><condition>${ready == true}</condition></conditionalEventDefinition>
    </intermediateCatchEvent>
    <sequenceFlow id="f3" sourceRef="catch" targetRef="e"/>
    <sequenceFlow id="f4" sourceRef="b" targetRef="cancelEnd"/>
    <endEvent id="e"/>
    <endEvent id="cancelEnd"/>
  </process>
</definitions>`
	parsed, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	boundary, ok := parsed.Processes[0].Elements["b"].(*model.BoundaryEvent)
	if !ok || boundary.Condition != "${amount > 100}" {
		t.Fatalf("边界条件事件解析错误: %+v", boundary)
	}
	catch, ok := parsed.Processes[0].Elements["catch"].(*model.IntermediateCatchEvent)
	if !ok || catch.Condition != "${ready == true}" {
		t.Fatalf("中间捕获条件事件解析错误: %+v", catch)
	}
}

// TestParse_ConditionalExpressionFallback 验证 conditionExpression 兼容解析。
// @param t 测试上下文
func TestParse_ConditionalExpressionFallback(t *testing.T) {
	data := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="p" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="task"/>
    <userTask id="task"/>
    <boundaryEvent id="b" attachedToRef="task">
      <conditionalEventDefinition><conditionExpression>${flag}</conditionExpression></conditionalEventDefinition>
    </boundaryEvent>
    <sequenceFlow id="f2" sourceRef="task" targetRef="e"/>
    <sequenceFlow id="f3" sourceRef="b" targetRef="cancelEnd"/>
    <endEvent id="e"/>
    <endEvent id="cancelEnd"/>
  </process>
</definitions>`
	parsed, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	boundary := parsed.Processes[0].Elements["b"].(*model.BoundaryEvent)
	if boundary.Condition != "${flag}" {
		t.Fatalf("conditionExpression 解析错误: %+v", boundary)
	}
}

// TestParse_ConditionalEventErrors 验证条件事件缺少或空表达式时拒绝解析。
// @param t 测试上下文
func TestParse_ConditionalEventErrors(t *testing.T) {
	cases := []struct {
		name  string
		event string
	}{
		{"缺少 condition", `<conditionalEventDefinition></conditionalEventDefinition>`},
		{"空 condition", `<conditionalEventDefinition><condition>   </condition></conditionalEventDefinition>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="p" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="task"/>
    <userTask id="task"/>
    <boundaryEvent id="b" attachedToRef="task">` + tc.event + `</boundaryEvent>
    <sequenceFlow id="f2" sourceRef="task" targetRef="e"/>
    <sequenceFlow id="f3" sourceRef="b" targetRef="cancelEnd"/>
    <endEvent id="e"/>
    <endEvent id="cancelEnd"/>
  </process>
</definitions>`
			if _, err := Parse([]byte(data)); err == nil || !strings.Contains(err.Error(), "condition") {
				t.Fatalf("期望条件表达式错误，实际 %v", err)
			}
		})
	}
}
