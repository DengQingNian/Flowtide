package engine

import (
	"context"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
)

// TestThrowCancel_Errors 验证取消传播错误分支。
// @param t 测试上下文
func TestThrowCancel_Errors(t *testing.T) {
	engine, _ := New(Config{})
	cc := &commandContext{engine: engine, ctx: context.Background(), now: time.Now()}
	if err := engine.throwCancel(cc, &executionState{ID: "e", ProcessInstanceID: "missing"}); err != nil {
		t.Fatalf("缺失实例不应报错: %v", err)
	}
	engine.state.Instances["i"] = &instanceState{ID: "i", RootExecutionID: "scope"}
	engine.state.Executions["scope"] = &executionState{ID: "scope", ProcessInstanceID: "i", ScopeRoot: true}
	if err := engine.throwCancel(cc, engine.state.Executions["scope"]); err != nil {
		t.Fatalf("缺失父执行不应报错: %v", err)
	}

	engine.state.Instances["i2"] = &instanceState{ID: "i2", RootExecutionID: "parent"}
	engine.state.Executions["scope2"] = &executionState{ID: "scope2", ProcessInstanceID: "i2", ParentID: "parent", ScopeRoot: true}
	engine.state.Executions["parent"] = &executionState{ID: "parent", ProcessInstanceID: "i2", DefinitionID: "missing"}
	engine.state.Instances["i2"].RootExecutionID = "parent"
	if err := engine.throwCancel(cc, engine.state.Executions["scope2"]); err != nil {
		t.Fatalf("缺失定义不应报错: %v", err)
	}

	process := &model.Process{ID: "p", Elements: map[string]model.FlowElement{}}
	sub := &model.SubProcess{FlowNodeBase: model.FlowNodeBase{BaseElement: model.BaseElement{ID: "tx"}}, Transaction: true}
	process.AddElement(sub)
	engine.state.Definitions["d"] = &definitionState{Info: api.ProcessDefinition{ID: "d", Key: "p"}, Model: &model.BpmnModel{Processes: []*model.Process{process}}}
	engine.state.Executions["parent"].DefinitionID = "d"
	engine.state.Executions["parent"].ActivityID = "missing"
	if err := engine.throwCancel(cc, engine.state.Executions["scope2"]); err != nil {
		t.Fatalf("缺失活动不应报错: %v", err)
	}

	engine.state.Executions["parent"].ActivityID = "tx"
	if err := engine.throwCancel(cc, engine.state.Executions["scope2"]); err != nil {
		t.Fatalf("无边界应结束实例: %v", err)
	}
	if !engine.state.Instances["i2"].Ended {
		t.Fatalf("无边界应结束实例")
	}
}
