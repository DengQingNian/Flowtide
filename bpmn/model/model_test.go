package model

import "testing"

// TestElementTypes 验证元素类型。
// @param t 测试上下文
func TestElementTypes(t *testing.T) {
	elements := []FlowElement{
		&StartEvent{}, &EndEvent{}, &UserTask{}, &ServiceTask{}, &ScriptTask{},
		&ManualTask{}, &ExclusiveGateway{}, &ParallelGateway{}, &SequenceFlow{},
	}
	want := []string{
		"startEvent", "endEvent", "userTask", "serviceTask", "scriptTask",
		"manualTask", "exclusiveGateway", "parallelGateway", "sequenceFlow",
	}
	for i, element := range elements {
		if element.ElementType() != want[i] {
			t.Fatalf("元素 %T 类型错误: %s", element, element.ElementType())
		}
	}
}

// TestProcessHelpers 验证流程元素索引与连线。
// @param t 测试上下文
func TestProcessHelpers(t *testing.T) {
	process := &Process{ID: "p", Elements: make(map[string]FlowElement)}
	start := &StartEvent{FlowNodeBase: FlowNodeBase{BaseElement: BaseElement{ID: "start", Name: "Start"}}}
	end := &EndEvent{FlowNodeBase: FlowNodeBase{BaseElement: BaseElement{ID: "end", Name: "End"}}}
	flow := &SequenceFlow{BaseElement: BaseElement{ID: "f", Name: "Flow"}, SourceRef: "start", TargetRef: "end"}
	process.AddElement(start)
	process.AddElement(end)
	process.AddElement(flow)

	if len(process.Order) != 3 {
		t.Fatalf("元素顺序数量错误")
	}
	if start.GetID() != "start" || start.GetName() != "Start" {
		t.Fatalf("基础元素方法错误")
	}
	if got, ok := process.GetFlowNode("start"); !ok || got.GetID() != "start" {
		t.Fatalf("GetFlowNode 失败")
	}
	if _, ok := process.GetFlowNode("missing"); ok {
		t.Fatalf("不存在的节点应返回 false")
	}
	if got, ok := process.GetSequenceFlow("f"); !ok || got.GetID() != "f" {
		t.Fatalf("GetSequenceFlow 失败")
	}
	if _, ok := process.GetSequenceFlow("start"); ok {
		t.Fatalf("非顺序流应返回 false")
	}
	if len(process.StartEvents()) != 1 {
		t.Fatalf("开始事件数量错误")
	}

	start.AddOutgoing(flow)
	end.AddIncoming(flow)
	if len(start.GetOutgoing()) != 1 || len(end.GetIncoming()) != 1 {
		t.Fatalf("连线添加失败")
	}
}
