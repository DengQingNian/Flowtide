package engine

import (
	"context"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
)

// TestCompensation_NoEntries 验证无补偿登记。
// @param t 测试上下文
func TestCompensation_NoEntries(t *testing.T) {
	engine, _ := New(Config{})
	cc := &commandContext{engine: engine, ctx: context.Background(), now: time.Now()}
	execution := &executionState{ID: "e", ProcessInstanceID: "missing"}
	if err := engine.compensate(cc, execution); err != nil {
		t.Fatalf("无登记不应报错: %v", err)
	}
	engine.registerCompensation(cc, execution, "work")
}

// TestCompensation_MissingHandler 验证处理器缺失。
// @param t 测试上下文
func TestCompensation_MissingHandler(t *testing.T) {
	engine, _ := New(Config{})
	process := &model.Process{ID: "p", Elements: map[string]model.FlowElement{}}
	engine.state.Definitions["d"] = &definitionState{Info: api.ProcessDefinition{ID: "d", Key: "p"}, Model: &model.BpmnModel{Processes: []*model.Process{process}}}
	engine.state.Instances["i"] = &instanceState{ID: "i", RootExecutionID: "e"}
	engine.state.Executions["e"] = &executionState{ID: "e", ProcessInstanceID: "i", DefinitionID: "d"}
	engine.state.Compensations["i"] = []*compensationState{{ID: "c", HandlerID: "missing"}}
	cc := &commandContext{engine: engine, ctx: context.Background(), now: time.Now()}
	if err := engine.compensate(cc, engine.state.Executions["e"]); err != nil {
		t.Fatalf("缺失处理器应跳过: %v", err)
	}
	if len(engine.state.Compensations["i"]) != 0 {
		t.Fatalf("补偿登记应清空")
	}
}

// TestCompensation_ManualHandler 验证手动补偿处理器。
// @param t 测试上下文
func TestCompensation_ManualHandler(t *testing.T) {
	engine, _ := New(Config{})
	process := &model.Process{ID: "p", Elements: map[string]model.FlowElement{}}
	handler := &model.ManualTask{FlowNodeBase: model.FlowNodeBase{BaseElement: model.BaseElement{ID: "undo"}}}
	process.AddElement(handler)
	engine.state.Definitions["d"] = &definitionState{Info: api.ProcessDefinition{ID: "d", Key: "p"}, Model: &model.BpmnModel{Processes: []*model.Process{process}}}
	engine.state.Instances["i"] = &instanceState{ID: "i", RootExecutionID: "e"}
	engine.state.Executions["e"] = &executionState{ID: "e", ProcessInstanceID: "i", DefinitionID: "d"}
	engine.state.Compensations["i"] = []*compensationState{{ID: "c", HandlerID: "undo"}}
	cc := &commandContext{engine: engine, ctx: context.Background(), now: time.Now()}
	if err := engine.compensate(cc, engine.state.Executions["e"]); err != nil {
		t.Fatalf("手动补偿失败: %v", err)
	}
}
