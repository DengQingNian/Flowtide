package engine

import (
	"context"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
	"github.com/flowable-go/flowable/internal/clock"
)

// TestVariableResolutionDirect 验证变量作用域解析。
// @param t 测试上下文
func TestVariableResolutionDirect(t *testing.T) {
	app, _ := New(Config{Clock: clock.NewFake(time.Now())})
	app.state.Executions["parent"] = &executionState{ID: "parent", Variables: map[string]any{"a": 1}}
	app.state.Executions["child"] = &executionState{ID: "child", ParentID: "parent", Variables: map[string]any{"b": 2}}
	app.state.Executions["broken"] = &executionState{ID: "broken", ParentID: "missing"}

	child := app.state.Executions["child"]
	if value, ok := app.getVariable(child, "a"); !ok || value != 1 {
		t.Fatalf("父作用域变量读取失败: %v %v", value, ok)
	}
	if value, ok := app.getVariable(child, "b"); !ok || value != 2 {
		t.Fatalf("子作用域变量读取失败")
	}
	if _, ok := app.getVariable(child, "missing"); ok {
		t.Fatalf("不存在变量应返回 false")
	}
	if _, ok := app.getVariable(app.state.Executions["broken"], "a"); ok {
		t.Fatalf("断链执行不应读取到变量")
	}

	app.setVariable(child, "a", 10)
	if app.state.Executions["parent"].Variables["a"] != 10 {
		t.Fatalf("应写入已有父作用域")
	}
	app.setVariable(child, "c", 3)
	if app.state.Executions["parent"].Variables["c"] != 3 {
		t.Fatalf("新变量应写入根作用域")
	}
	app.state.Executions["parent"].Variables = nil
	app.setVariable(app.state.Executions["parent"], "d", 4)
	if app.state.Executions["parent"].Variables["d"] != 4 {
		t.Fatalf("根变量 map 应被初始化")
	}
	app.setVariableLocal(&executionState{ID: "x"}, "e", 5)
	if app.state.Executions["parent"].Variables["e"] != nil {
		t.Fatalf("局部变量不应写入根")
	}
	if _, ok := app.getVariable(child, "e"); ok {
		t.Fatalf("局部变量不应被其他执行读取")
	}
}

// TestVariablesOfOverride 验证变量快照覆盖顺序。
// @param t 测试上下文
func TestVariablesOfOverride(t *testing.T) {
	app, _ := New(Config{})
	app.state.Executions["p"] = &executionState{ID: "p", Variables: map[string]any{"a": 1, "b": 1}}
	app.state.Executions["c"] = &executionState{ID: "c", ParentID: "p", Variables: map[string]any{"b": 2}}
	values := app.variablesOf(app.state.Executions["c"])
	if values["a"] != 1 || values["b"] != 2 {
		t.Fatalf("变量覆盖顺序错误: %+v", values)
	}
}

// TestTenantAndPageHelpers 验证租户与分页辅助函数。
// @param t 测试上下文
func TestTenantAndPageHelpers(t *testing.T) {
	app, _ := New(Config{TenantID: "default"})
	if app.tenantOf("") != "default" || app.tenantOf("custom") != "custom" {
		t.Fatalf("租户解析错误")
	}
	if start, size := normalizePage(-1, 0); start != 0 || size != 10 {
		t.Fatalf("分页默认值错误: %d %d", start, size)
	}
	if _, size := normalizePage(0, 5000); size != 1000 {
		t.Fatalf("分页上限错误: %d", size)
	}
}

// TestProcessHelpersInternal 验证定义流程选择。
// @param t 测试上下文
func TestProcessHelpersInternal(t *testing.T) {
	if processOfDefinition(&definitionState{Info: api.ProcessDefinition{Key: "x"}}) != nil {
		t.Fatalf("空模型应返回 nil")
	}
	first := &model.Process{ID: "a"}
	second := &model.Process{ID: "b"}
	definition := &definitionState{Info: api.ProcessDefinition{Key: "b"}, Model: &model.BpmnModel{Processes: []*model.Process{first, second}}}
	if processOfDefinition(definition) != second {
		t.Fatalf("应按 Key 选择流程")
	}
	definition.Info.Key = "missing"
	if processOfDefinition(definition) != first {
		t.Fatalf("应回退到第一个流程")
	}
	if firstStartEvent(&model.Process{}) != nil {
		t.Fatalf("无开始事件应返回 nil")
	}
}

// TestDelegateNameResolution 验证委托名解析。
// @param t 测试上下文
func TestDelegateNameResolution(t *testing.T) {
	if got := resolveDelegateName("${name}", map[string]any{"name": "noop"}); got != "noop" {
		t.Fatalf("变量委托名错误: %s", got)
	}
	if got := resolveDelegateName("${missing}", nil); got != "missing" {
		t.Fatalf("缺省委托名错误: %s", got)
	}
	if got := resolveDelegateName("  noop  ", nil); got != "noop" {
		t.Fatalf("裸委托名错误: %s", got)
	}
	if got := resolveDelegateName("${name}", map[string]any{"name": 1}); got != "name" {
		t.Fatalf("非字符串变量应回退到表达式名: %s", got)
	}
}

// TestSelectFlowsDirect 验证条件出线选择。
// @param t 测试上下文
func TestSelectFlowsDirect(t *testing.T) {
	app, _ := New(Config{})
	execution := &executionState{ID: "e", Variables: map[string]any{"amount": 200}}
	flows := []*model.SequenceFlow{
		{BaseElement: model.BaseElement{ID: "a"}, Condition: "amount > 100"},
		{BaseElement: model.BaseElement{ID: "b"}, Condition: "amount < 100"},
		{BaseElement: model.BaseElement{ID: "c"}, IsDefault: true},
	}
	selected, err := app.selectFlows(execution, flows)
	if err != nil || len(selected) != 1 || selected[0].ID != "a" {
		t.Fatalf("条件选择错误: %v %+v", err, selected)
	}
	selected, err = app.selectFlows(execution, []*model.SequenceFlow{{BaseElement: model.BaseElement{ID: "d"}, IsDefault: true}})
	if err != nil || len(selected) != 1 || selected[0].ID != "d" {
		t.Fatalf("默认流选择错误")
	}
	bad := []*model.SequenceFlow{{BaseElement: model.BaseElement{ID: "x"}, Condition: "1 +"}}
	if _, err := app.selectFlows(execution, bad); err == nil {
		t.Fatalf("非法条件应报错")
	}
	if _, err := app.selectExclusive(execution, []*model.SequenceFlow{{BaseElement: model.BaseElement{ID: "x"}}}); err != nil {
		t.Fatalf("无默认流应返回错误而非 panic: %v", err)
	}
}

// TestEndExecutionDirect 验证执行结束分支。
// @param t 测试上下文
func TestEndExecutionDirect(t *testing.T) {
	app, _ := New(Config{})
	app.state.Instances["i"] = &instanceState{ID: "i", RootExecutionID: "root"}
	app.state.Executions["root"] = &executionState{ID: "root", ProcessInstanceID: "i", Active: true}
	app.state.Executions["child"] = &executionState{ID: "child", ProcessInstanceID: "i", ParentID: "root"}
	app.state.Executions["root"].PendingBranches = 1
	cc := &commandContext{engine: app, ctx: context.Background(), now: time.Now()}
	app.endExecution(cc, app.state.Executions["child"], false)
	if !app.state.Executions["child"].Ended || !app.state.Instances["i"].Ended {
		t.Fatalf("子执行结束应触发根结束")
	}
	app.state.Instances["i2"] = &instanceState{ID: "i2", RootExecutionID: "root2"}
	app.state.Executions["root2"] = &executionState{ID: "root2", ProcessInstanceID: "i2"}
	app.endExecution(cc, app.state.Executions["root2"], true)
	if !app.state.Instances["i2"].Ended {
		t.Fatalf("终止结束事件应结束实例")
	}
}

// TestHistoryHelpersDirect 验证历史辅助函数空路径。
// @param t 测试上下文
func TestHistoryHelpersDirect(t *testing.T) {
	app, _ := New(Config{})
	cc := &commandContext{engine: app, now: time.Now()}
	app.recordActivityEnd(cc, "")
	app.recordActivityEnd(cc, "missing")
	app.recordTaskEnd(cc, &taskState{})
	app.recordProcessEnd(cc, &instanceState{ID: "missing"})
	if _, err := app.ListHistoricProcessInstances(context.Background()); err != nil {
		t.Fatalf("空历史查询失败: %v", err)
	}
}

// TestEvalIntBranches 验证整数字面量解析。
// @param t 测试上下文
func TestEvalIntBranches(t *testing.T) {
	if got, err := evalInt("${1 + 1}", nil); err != nil || got != 2 {
		t.Fatalf("浮点转整数错误: %v %v", got, err)
	}
	if got, err := evalInt("${'5'}", nil); err != nil || got != 5 {
		t.Fatalf("字符串转整数错误: %v %v", got, err)
	}
	if _, err := evalInt("${'x'}", nil); err == nil {
		t.Fatalf("非法整数字符串应报错")
	}
	if _, err := evalInt("${1 +}", nil); err == nil {
		t.Fatalf("非法表达式应报错")
	}
	if _, err := evalInt("${true}", nil); err == nil {
		t.Fatalf("布尔值应报错")
	}
}
