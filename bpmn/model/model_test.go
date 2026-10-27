package model

import "testing"

// TestElementTypes 验证元素类型。
// @param t 测试上下文
func TestElementTypes(t *testing.T) {
	elements := []FlowElement{
		&StartEvent{}, &EndEvent{}, &UserTask{}, &ServiceTask{}, &ScriptTask{},
		&ManualTask{}, &ExclusiveGateway{}, &ParallelGateway{}, &SequenceFlow{}, &SubProcess{}, &BoundaryEvent{}, &CallActivity{},
	}
	want := []string{
		"startEvent", "endEvent", "userTask", "serviceTask", "scriptTask",
		"manualTask", "exclusiveGateway", "parallelGateway", "sequenceFlow", "subProcess", "boundaryEvent", "callActivity",
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

// TestSubProcessHelpers 验证子流程容器方法。
// @param t 测试上下文
func TestSubProcessHelpers(t *testing.T) {
	sub := &SubProcess{TriggeredByEvent: true}
	start := &StartEvent{FlowNodeBase: FlowNodeBase{BaseElement: BaseElement{ID: "s"}}}
	end := &EndEvent{FlowNodeBase: FlowNodeBase{BaseElement: BaseElement{ID: "e"}}}
	flow := &SequenceFlow{BaseElement: BaseElement{ID: "f"}, SourceRef: "s", TargetRef: "e"}
	sub.AddElement(start)
	sub.AddElement(end)
	sub.AddElement(flow)
	if len(sub.Order) != 3 || !sub.TriggeredByEvent {
		t.Fatalf("子流程字段错误")
	}
	if node, ok := sub.GetFlowNode("s"); !ok || node.GetID() != "s" {
		t.Fatalf("子流程节点查询失败")
	}
	if _, ok := sub.GetFlowNode("f"); ok {
		t.Fatalf("顺序流不是流程节点")
	}
	if got, ok := sub.GetSequenceFlow("f"); !ok || got.ID != "f" {
		t.Fatalf("子流程顺序流查询失败")
	}
	if _, ok := sub.GetSequenceFlow("s"); ok {
		t.Fatalf("节点不是顺序流")
	}
	if len(sub.StartEvents()) != 1 {
		t.Fatalf("子流程开始事件数量错误")
	}
}

// TestBoundaryEventHelpers 验证边界事件挂载。
// @param t 测试上下文
func TestBoundaryEventHelpers(t *testing.T) {
	task := &UserTask{FlowNodeBase: FlowNodeBase{BaseElement: BaseElement{ID: "task"}}}
	event := &BoundaryEvent{FlowNodeBase: FlowNodeBase{BaseElement: BaseElement{ID: "b"}}, AttachedToRef: "task", CancelActivity: true}
	task.AddBoundaryEvent(event)
	if len(task.GetBoundaryEvents()) != 1 || task.GetBoundaryEvents()[0].ID != "b" {
		t.Fatalf("边界事件挂载失败")
	}
}

// TestAsyncFlags 验证异步标记。
// @param t 测试上下文
func TestAsyncFlags(t *testing.T) {
	node := &FlowNodeBase{AsyncBefore: true, AsyncAfter: true, Exclusive: true}
	if !node.IsAsyncBefore() || !node.IsAsyncAfter() || !node.IsExclusive() {
		t.Fatalf("异步标记读取错误")
	}
}
