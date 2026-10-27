package engine

import (
	"context"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
)

// TestEventSubProcess_Errors 验证事件子流程错误分支。
// @param t 测试上下文
func TestEventSubProcess_Errors(t *testing.T) {
	engine, _ := New(Config{})
	cc := &commandContext{engine: engine, ctx: context.Background(), now: time.Now()}
	subscription := &subscriptionState{ID: "s", ScopeExecutionID: "missing", EventSubProcessID: "evt"}
	engine.state.Subscriptions[subscription.ID] = subscription
	if err := engine.triggerEventSubProcess(cc, subscription); err != nil {
		t.Fatalf("缺失作用域不应报错: %v", err)
	}
	if _, ok := engine.state.Subscriptions["s"]; ok {
		t.Fatalf("缺失作用域订阅应删除")
	}

	engine.state.Instances["i"] = &instanceState{ID: "i", RootExecutionID: "root", Suspended: true}
	engine.state.Executions["root"] = &executionState{ID: "root", ProcessInstanceID: "i"}
	suspended := &subscriptionState{ID: "s2", ScopeExecutionID: "root", EventSubProcessID: "evt"}
	engine.state.Subscriptions["s2"] = suspended
	if err := engine.triggerEventSubProcess(cc, suspended); err != nil {
		t.Fatalf("挂起实例不应报错: %v", err)
	}

	engine.endEventSubProcess(cc, &executionState{ID: "e", EventSubProcessRoot: true, EventSubProcessInterrupting: true, ParentID: "missing"})
}

// TestCancelScopeContent 验证取消作用域内容。
// @param t 测试上下文
func TestCancelScopeContent(t *testing.T) {
	engine, _ := New(Config{})
	engine.state.Instances["i"] = &instanceState{ID: "i", RootExecutionID: "root"}
	engine.state.Executions["root"] = &executionState{ID: "root", ProcessInstanceID: "i", Active: true}
	engine.state.Executions["child"] = &executionState{ID: "child", ProcessInstanceID: "i", ParentID: "root", Active: true, CurrentHistActivityID: "h"}
	engine.state.Tasks["t"] = &taskState{Task: api.Task{ID: "t", ProcessInstanceID: "i"}, ExecutionID: "child"}
	engine.state.Jobs["j"] = &jobState{ID: "j", ExecutionID: "child", ProcessInstanceID: "i"}
	engine.state.Subscriptions["s"] = &subscriptionState{ID: "s", ExecutionID: "child", ProcessInstanceID: "i"}
	cc := &commandContext{engine: engine, ctx: context.Background(), now: time.Now()}
	engine.cancelScopeContent(cc, engine.state.Executions["root"])
	if !engine.state.Executions["child"].Ended {
		t.Fatalf("子执行应结束")
	}
	if len(engine.state.Tasks) != 0 || len(engine.state.Jobs) != 0 || len(engine.state.Subscriptions) != 0 {
		t.Fatalf("作用域内容应清理")
	}
}

// TestScheduleEventSubProcesses_Skip 验证非事件子流程跳过。
// @param t 测试上下文
func TestScheduleEventSubProcesses_Skip(t *testing.T) {
	engine, _ := New(Config{})
	cc := &commandContext{engine: engine, ctx: context.Background(), now: time.Now()}
	definition := &definitionState{Info: api.ProcessDefinition{ID: "d"}, Model: &model.BpmnModel{}}
	sub := &model.SubProcess{TriggeredByEvent: false}
	elements := map[string]model.FlowElement{"sub": sub}
	engine.scheduleEventSubProcesses(cc, &executionState{ID: "e"}, elements, []string{"sub"}, definition)
	if len(engine.state.Subscriptions) != 0 {
		t.Fatalf("非事件子流程不应创建订阅")
	}
}
