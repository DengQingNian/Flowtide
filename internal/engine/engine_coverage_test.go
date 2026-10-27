package engine

import (
	"context"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
)

// TestClosedEngineQueries 验证关闭引擎的查询错误。
// @param t 测试上下文
func TestClosedEngineQueries(t *testing.T) {
	app := newTestEngine(t)
	_ = app.Close(context.Background())
	if _, err := app.ListProcessDefinitions(context.Background()); err == nil {
		t.Fatalf("定义列表应报错")
	}
	if _, err := app.ListProcessInstances(context.Background()); err == nil {
		t.Fatalf("实例列表应报错")
	}
	if _, err := app.ListTasks(context.Background(), api.TaskFilter{}); err == nil {
		t.Fatalf("任务列表应报错")
	}
	if _, err := app.ListHistoricProcessInstances(context.Background()); err == nil {
		t.Fatalf("历史流程应报错")
	}
	if _, err := app.ListHistoricActivityInstances(context.Background(), ""); err == nil {
		t.Fatalf("历史活动应报错")
	}
	if _, err := app.ListHistoricTaskInstances(context.Background(), ""); err == nil {
		t.Fatalf("历史任务应报错")
	}
	if _, err := app.EngineInfo(context.Background()); err != nil {
		t.Fatalf("引擎信息不应受关闭影响: %v", err)
	}
}

// TestSortInstancesTie 验证实例排序。
// @param t 测试上下文
func TestSortInstancesTie(t *testing.T) {
	now := time.Now()
	items := []api.ProcessInstance{
		{ID: "b", StartTime: now},
		{ID: "a", StartTime: now},
		{ID: "c", StartTime: now.Add(time.Hour)},
	}
	sortInstances(items)
	if items[0].ID != "a" || items[1].ID != "b" || items[2].ID != "c" {
		t.Fatalf("排序错误: %+v", items)
	}
}

// TestEndProcessInstanceBranches 验证实例结束分支。
// @param t 测试上下文
func TestEndProcessInstanceBranches(t *testing.T) {
	app, _ := New(Config{})
	app.state.Instances["i"] = &instanceState{ID: "i", RootExecutionID: "root"}
	app.state.Executions["root"] = &executionState{ID: "root", ProcessInstanceID: "i"}
	app.state.Tasks["t"] = &taskState{Task: api.Task{ID: "t", ProcessInstanceID: "i"}}
	cc := &commandContext{engine: app, ctx: context.Background(), now: time.Now()}
	app.endProcessInstance(cc, app.state.Instances["i"])
	if !app.state.Instances["i"].Ended {
		t.Fatalf("实例应结束")
	}
	app.endProcessInstance(cc, app.state.Instances["i"])
}

// TestTakeOutgoingDirect 验证出线处理错误分支。
// @param t 测试上下文
func TestTakeOutgoingDirect(t *testing.T) {
	app, _ := New(Config{})
	process := &model.Process{ID: "p", Elements: map[string]model.FlowElement{}}
	task := &model.UserTask{FlowNodeBase: model.FlowNodeBase{BaseElement: model.BaseElement{ID: "task"}}}
	process.AddElement(task)
	model := &model.BpmnModel{Processes: []*model.Process{process}}
	app.state.Definitions["d"] = &definitionState{Info: api.ProcessDefinition{ID: "d", Key: "p"}, Model: model}
	app.state.Instances["i"] = &instanceState{ID: "i", RootExecutionID: "e"}
	app.state.Executions["e"] = &executionState{ID: "e", ProcessInstanceID: "i", DefinitionID: "d", Active: true}
	cc := &commandContext{engine: app, ctx: context.Background(), now: time.Now()}

	if err := app.takeOutgoing(cc, "missing", "task", false); err != nil {
		t.Fatalf("缺失执行不应报错: %v", err)
	}
	if err := app.takeOutgoing(cc, "e", "missing", false); err == nil {
		t.Fatalf("缺失节点应报错")
	}
	if err := app.takeOutgoing(cc, "e", "task", false); err != nil {
		t.Fatalf("无出线应结束执行: %v", err)
	}
	if !app.state.Instances["i"].Ended {
		t.Fatalf("无出线应结束实例")
	}

	app.state.Instances["i2"] = &instanceState{ID: "i2", RootExecutionID: "e2"}
	app.state.Executions["e2"] = &executionState{ID: "e2", ProcessInstanceID: "i2", DefinitionID: "missing"}
	if err := app.takeOutgoing(cc, "e2", "task", false); err == nil {
		t.Fatalf("缺失定义应报错")
	}
}

// TestContinueAtErrors 验证节点进入错误分支。
// @param t 测试上下文
func TestContinueAtErrors(t *testing.T) {
	app, _ := New(Config{})
	cc := &commandContext{engine: app, ctx: context.Background(), now: time.Now()}
	if err := app.continueAt(cc, "missing", "x"); err != nil {
		t.Fatalf("缺失执行不应报错")
	}
	app.state.Executions["ended"] = &executionState{ID: "ended", Ended: true}
	if err := app.continueAt(cc, "ended", "x"); err != nil {
		t.Fatalf("已结束执行不应报错")
	}
	app.state.Executions["waiting"] = &executionState{ID: "waiting", Waiting: true}
	if err := app.continueAt(cc, "waiting", "x"); err != nil {
		t.Fatalf("等待执行不应报错")
	}
	app.state.Executions["noInstance"] = &executionState{ID: "noInstance", ProcessInstanceID: "missing"}
	if err := app.continueAt(cc, "noInstance", "x"); err != nil {
		t.Fatalf("缺失实例不应报错")
	}
	app.state.Instances["i"] = &instanceState{ID: "i", RootExecutionID: "e"}
	app.state.Executions["e"] = &executionState{ID: "e", ProcessInstanceID: "i", DefinitionID: "missing"}
	if err := app.continueAt(cc, "e", "x"); err == nil {
		t.Fatalf("缺失定义应报错")
	}
	app.state.Definitions["d"] = &definitionState{Info: api.ProcessDefinition{ID: "d", Key: "p"}, Model: &model.BpmnModel{Processes: []*model.Process{{ID: "p", Elements: map[string]model.FlowElement{}}}}}
	app.state.Executions["e"].DefinitionID = "d"
	if err := app.continueAt(cc, "e", "missing"); err == nil {
		t.Fatalf("缺失元素应报错")
	}
}

// TestSetVariableBrokenParent 验证断链变量写入。
// @param t 测试上下文
func TestSetVariableBrokenParent(t *testing.T) {
	app, _ := New(Config{})
	execution := &executionState{ID: "e", ParentID: "missing"}
	app.setVariable(execution, "x", 1)
	if execution.Variables["x"] != 1 {
		t.Fatalf("断链执行应写入自身")
	}
}

// TestEvalIntVariableTypes 验证整数变量类型。
// @param t 测试上下文
func TestEvalIntVariableTypes(t *testing.T) {
	if got, err := evalInt("${v}", map[string]any{"v": int(3)}); err != nil || got != 3 {
		t.Fatalf("int 转换错误: %v %v", got, err)
	}
	if got, err := evalInt("${v}", map[string]any{"v": int64(4)}); err != nil || got != 4 {
		t.Fatalf("int64 转换错误: %v %v", got, err)
	}
}

// TestParseDueDateBranches 验证到期时间解析。
// @param t 测试上下文
func TestParseDueDateBranches(t *testing.T) {
	if parseDueDate("", nil) != nil {
		t.Fatalf("空表达式应返回 nil")
	}
	if parseDueDate("${1 +}", nil) != nil {
		t.Fatalf("非法表达式应返回 nil")
	}
	if parseDueDate("not-a-date", nil) != nil {
		t.Fatalf("非法日期应返回 nil")
	}
	if got := parseDueDate("2026-12-01", nil); got == nil {
		t.Fatalf("合法日期应解析")
	}
}

// TestCreateUserTaskErrors 验证任务创建错误。
// @param t 测试上下文
func TestCreateUserTaskErrors(t *testing.T) {
	cases := []string{
		`flowable:assignee="${1 +}"`,
		`flowable:priority="${'x'}"`,
		`flowable:candidateUsers="${1 +}"`,
	}
	for _, attrs := range cases {
		engine := newTestEngine(t)
		xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="taskError" isExecutable="true"><startEvent id="start"/><sequenceFlow id="f1" sourceRef="start" targetRef="task"/><userTask id="task" ` + attrs + `/><sequenceFlow id="f2" sourceRef="task" targetRef="end"/><endEvent id="end"/></process></definitions>`
		deployXML(t, engine, xml)
		if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "taskError"}); err == nil {
			t.Fatalf("任务属性 %q 应报错", attrs)
		}
	}
}

// TestMatchesTaskBranches 验证任务过滤分支。
// @param t 测试上下文
func TestMatchesTaskBranches(t *testing.T) {
	app, _ := New(Config{})
	task := &taskState{Task: api.Task{ID: "t", Owner: "owner", Assignee: "alice", ProcessInstanceID: "i"}, CandidateUsers: []string{"bob"}}
	if !app.matchesTask(task, api.TaskFilter{Owner: "owner"}) {
		t.Fatalf("Owner 过滤失败")
	}
	if app.matchesTask(task, api.TaskFilter{Owner: "other"}) {
		t.Fatalf("错误 Owner 不应匹配")
	}
	if app.matchesTask(task, api.TaskFilter{CandidateUser: "bob"}) {
		t.Fatalf("已分配任务不应匹配候选人")
	}
	if app.matchesTask(task, api.TaskFilter{ProcessDefinitionKey: "x"}) {
		t.Fatalf("缺失实例不应匹配定义 Key")
	}
}

// TestCloneHelpers 验证拷贝辅助函数。
// @param t 测试上下文
func TestCloneHelpers(t *testing.T) {
	if cloneBytesMap(nil) != nil {
		t.Fatalf("nil 字节映射应返回 nil")
	}
	source := map[string][]byte{"a": []byte("x")}
	copied := cloneBytesMap(source)
	copied["a"][0] = 'y'
	if source["a"][0] != 'x' {
		t.Fatalf("字节映射应为深拷贝")
	}
}

// TestSuspendedDefinition 验证挂起定义启动失败。
// @param t 测试上下文
func TestSuspendedDefinition(t *testing.T) {
	app, _ := New(Config{})
	process := &model.Process{ID: "p", Elements: map[string]model.FlowElement{}, Order: []string{}}
	start := &model.StartEvent{FlowNodeBase: model.FlowNodeBase{BaseElement: model.BaseElement{ID: "s"}}}
	process.AddElement(start)
	app.state.Definitions["d"] = &definitionState{Info: api.ProcessDefinition{ID: "d", Key: "p", Suspended: true}, Model: &model.BpmnModel{Processes: []*model.Process{process}}}
	if _, err := app.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "p"}); err == nil {
		t.Fatalf("挂起定义应拒绝启动")
	}
}
