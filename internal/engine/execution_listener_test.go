package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
)

// executionListenerXML 覆盖流程级与活动级执行监听器。
const executionListenerXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="execListenerProcess" isExecutable="true">
    <extensionElements>
      <flowable:executionListener event="start" class="processStart"/>
      <flowable:executionListener event="end" delegateExpression="processEnd"/>
    </extensionElements>
    <startEvent id="start">
      <extensionElements>
        <flowable:executionListener event="start" delegateExpression="startEventStart"/>
      </extensionElements>
    </startEvent>
    <sequenceFlow id="f1" sourceRef="start" targetRef="task"/>
    <userTask id="task" name="Approve" flowable:assignee="alice">
      <extensionElements>
        <flowable:executionListener class="taskStartA"/>
        <flowable:executionListener delegateExpression="taskStartB"/>
        <flowable:executionListener event="end" class="taskEnd"/>
        <flowable:executionListener event="unknown" class="neverRegistered"/>
      </extensionElements>
    </userTask>
    <sequenceFlow id="f2" sourceRef="task" targetRef="svc"/>
    <serviceTask id="svc" flowable:delegateExpression="noop">
      <extensionElements>
        <flowable:executionListener event="start" class="serviceStart"/>
        <flowable:executionListener event="end" class="serviceEnd"/>
      </extensionElements>
    </serviceTask>
    <sequenceFlow id="f3" sourceRef="svc" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// executionListenerServiceXML 是同步结束的服务任务流程，用于流程结束事件。
const executionListenerServiceXML = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="listenerService" isExecutable="true">
    <extensionElements>
      <flowable:executionListener event="end" class="processEndFail"/>
    </extensionElements>
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="svc"/>
    <serviceTask id="svc" flowable:delegateExpression="noop">
      <extensionElements>
        <flowable:executionListener event="start" class="taskStartFail"/>
        <flowable:executionListener event="end" class="taskEndFail"/>
      </extensionElements>
    </serviceTask>
    <sequenceFlow id="f2" sourceRef="svc" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// executionListenerUserTaskXML 是等待用户任务的流程，用于任务结束事件。
const executionListenerUserTaskXML = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="listenerUserTask" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="task"/>
    <userTask id="task" name="T" flowable:assignee="alice">
      <extensionElements>
        <flowable:executionListener event="end" class="userTaskEndFail"/>
      </extensionElements>
    </userTask>
    <sequenceFlow id="f2" sourceRef="task" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestExecutionListenersLifecycle 验证流程级与活动级监听器的顺序、上下文与变量读写。
// @param t 测试上下文
func TestExecutionListenersLifecycle(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, executionListenerXML)
	engine.RegisterDelegate("noop", func(ctx context.Context, execution *Execution) error { return nil })
	events := make([]string, 0, 16)
	record := func(event string) { events = append(events, event) }
	engine.RegisterExecutionListener("processStart", func(ctx context.Context, execution *Execution) error {
		record("process-start")
		if execution.ProcessInstanceID() == "" {
			t.Fatalf("流程开始监听器缺少实例 ID")
		}
		if execution.ActivityID() != "" {
			t.Fatalf("流程开始监听器活动 ID 错误: %s", execution.ActivityID())
		}
		execution.SetVariable("processVar", "p")
		return nil
	})
	engine.RegisterExecutionListener("startEventStart", func(ctx context.Context, execution *Execution) error {
		record("startEvent-start")
		return nil
	})
	engine.RegisterExecutionListener("taskStartA", func(ctx context.Context, execution *Execution) error {
		record("task-start-A")
		if value, ok := execution.GetVariable("processVar"); !ok || value != "p" {
			t.Fatalf("活动开始监听器未读到流程变量: %v %v", value, ok)
		}
		if execution.ActivityID() != "task" {
			t.Fatalf("任务开始监听器活动 ID 错误: %s", execution.ActivityID())
		}
		return nil
	})
	engine.RegisterExecutionListener("taskStartB", func(ctx context.Context, execution *Execution) error {
		record("task-start-B")
		return nil
	})
	engine.RegisterExecutionListener("taskEnd", func(ctx context.Context, execution *Execution) error {
		record("task-end")
		if execution.ActivityID() != "task" {
			t.Fatalf("任务结束监听器活动 ID 错误: %s", execution.ActivityID())
		}
		execution.SetVariable("taskVar", "from-task")
		return nil
	})
	engine.RegisterExecutionListener("serviceStart", func(ctx context.Context, execution *Execution) error {
		record("service-start")
		if value, ok := execution.GetVariable("taskVar"); !ok || value != "from-task" {
			t.Fatalf("服务任务开始监听器未读到任务结束监听器变量: %v %v", value, ok)
		}
		return nil
	})
	engine.RegisterExecutionListener("serviceEnd", func(ctx context.Context, execution *Execution) error {
		record("service-end")
		execution.SetVariable("serviceVar", "from-service")
		return nil
	})
	engine.RegisterExecutionListener("processEnd", func(ctx context.Context, execution *Execution) error {
		record("process-end")
		if value, ok := execution.GetVariable("serviceVar"); !ok || value != "from-service" {
			t.Fatalf("流程结束监听器未读到服务任务变量: %v %v", value, ok)
		}
		return nil
	})
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "execListenerProcess"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	wantStart := []string{"process-start", "startEvent-start", "task-start-A", "task-start-B"}
	if len(events) != len(wantStart) {
		t.Fatalf("启动事件顺序错误: %v", events)
	}
	for i, want := range wantStart {
		if events[i] != want {
			t.Fatalf("启动事件第 %d 个应为 %s，实际 %v", i, want, events)
		}
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 {
		t.Fatalf("任务缺失: %+v", tasks)
	}
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{UserID: "alice"}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	wantAll := []string{"process-start", "startEvent-start", "task-start-A", "task-start-B",
		"task-end", "service-start", "service-end", "process-end"}
	if len(events) != len(wantAll) {
		t.Fatalf("完成事件顺序错误: %v", events)
	}
	for i, want := range wantAll {
		if events[i] != want {
			t.Fatalf("事件第 %d 个应为 %s，实际 %v", i, want, events)
		}
	}
	got, err := engine.GetProcessInstance(context.Background(), instance.ID)
	if err != nil || !got.Ended {
		t.Fatalf("流程应结束: %v %v", got.Ended, err)
	}
	variables, err := engine.GetVariables(context.Background(), instance.ID)
	if err != nil {
		t.Fatalf("查询变量失败: %v", err)
	}
	if variables["processVar"] != "p" || variables["taskVar"] != "from-task" || variables["serviceVar"] != "from-service" {
		t.Fatalf("监听器写入变量丢失: %+v", variables)
	}
}

// TestExecutionListenerErrors 验证缺失、表达式与空实现的统一 400 与回滚。
// @param t 测试上下文
func TestExecutionListenerErrors(t *testing.T) {
	base := func(listener string) string {
		return `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="listenerError" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="task"/>
    <userTask id="task" name="T">` + listener + `</userTask>
    <sequenceFlow id="f2" sourceRef="task" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`
	}
	cases := []struct {
		name     string
		listener string
		complete bool
	}{
		{name: "未注册", listener: `<extensionElements><flowable:executionListener class="missing"/></extensionElements>`},
		{name: "表达式", listener: `<extensionElements><flowable:executionListener event="start" expression="${notify}"/></extensionElements>`},
		{name: "空实现", listener: `<extensionElements><flowable:executionListener event="end"/></extensionElements>`, complete: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := newTestEngine(t)
			deployXML(t, engine, base(tc.listener))
			_, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "listenerError"})
			if tc.complete {
				if err != nil {
					t.Fatalf("启动不应失败: %v", err)
				}
				tasks := listTasks(t, engine)
				if len(tasks) != 1 {
					t.Fatalf("任务缺失: %+v", tasks)
				}
				err = engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{})
			}
			if !errors.Is(err, api.ErrBadRequest) {
				t.Fatalf("应返回 400: %v", err)
			}
			// 启动阶段失败回滚到无实例；任务完成阶段失败回滚到任务等待状态
			if tc.complete {
				if len(listTasks(t, engine)) != 1 {
					t.Fatalf("任务完成失败应保留任务")
				}
			} else if instances, _ := engine.ListProcessInstances(context.Background()); len(instances) != 0 {
				t.Fatalf("失败后应回滚实例: %+v", instances)
			}
		})
	}
}

// TestExecutionListenerFailureRollback 验证监听器执行失败时的命令回滚。
// @param t 测试上下文
func TestExecutionListenerFailureRollback(t *testing.T) {
	// 开始监听器失败：启动整体回滚
	engine := newTestEngine(t)
	deployXML(t, engine, executionListenerServiceXML)
	engine.RegisterDelegate("noop", func(ctx context.Context, execution *Execution) error { return nil })
	engine.RegisterExecutionListener("taskStartFail", func(ctx context.Context, execution *Execution) error {
		return errors.New("start failed")
	})
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "listenerService"}); err == nil {
		t.Fatalf("开始监听器失败应返回错误")
	}
	if instances, _ := engine.ListProcessInstances(context.Background()); len(instances) != 0 {
		t.Fatalf("开始监听器失败应回滚实例: %+v", instances)
	}

	// 服务任务结束监听器失败：启动整体回滚
	engine = newTestEngine(t)
	deployXML(t, engine, executionListenerServiceXML)
	engine.RegisterDelegate("noop", func(ctx context.Context, execution *Execution) error { return nil })
	engine.RegisterExecutionListener("taskStartFail", func(ctx context.Context, execution *Execution) error { return nil })
	engine.RegisterExecutionListener("taskEndFail", func(ctx context.Context, execution *Execution) error {
		return errors.New("end failed")
	})
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "listenerService"}); err == nil {
		t.Fatalf("服务任务结束监听器失败应返回错误")
	}
	if instances, _ := engine.ListProcessInstances(context.Background()); len(instances) != 0 {
		t.Fatalf("服务任务结束监听器失败应回滚实例: %+v", instances)
	}

	// 流程结束监听器失败：启动整体回滚
	engine = newTestEngine(t)
	deployXML(t, engine, executionListenerServiceXML)
	engine.RegisterDelegate("noop", func(ctx context.Context, execution *Execution) error { return nil })
	engine.RegisterExecutionListener("taskStartFail", func(ctx context.Context, execution *Execution) error { return nil })
	engine.RegisterExecutionListener("taskEndFail", func(ctx context.Context, execution *Execution) error { return nil })
	engine.RegisterExecutionListener("processEndFail", func(ctx context.Context, execution *Execution) error {
		return errors.New("process end failed")
	})
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "listenerService"}); err == nil {
		t.Fatalf("流程结束监听器失败应返回错误")
	}
	if instances, _ := engine.ListProcessInstances(context.Background()); len(instances) != 0 {
		t.Fatalf("流程结束监听器失败应回滚实例: %+v", instances)
	}

	// 用户任务结束监听器失败：完成任务回滚，任务保留
	engine = newTestEngine(t)
	deployXML(t, engine, executionListenerUserTaskXML)
	engine.RegisterExecutionListener("userTaskEndFail", func(ctx context.Context, execution *Execution) error {
		return errors.New("user task end failed")
	})
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "listenerUserTask"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 {
		t.Fatalf("任务缺失: %+v", tasks)
	}
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{UserID: "alice"}); err == nil {
		t.Fatalf("用户任务结束监听器失败应返回错误")
	}
	if len(listTasks(t, engine)) != 1 {
		t.Fatalf("任务结束监听器失败应回滚任务删除")
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if got.Ended {
		t.Fatalf("监听器失败不应结束流程")
	}
}

// TestExecutionListenerHistoryDisabled 验证历史关闭时开始监听器仍执行、结束监听器安全跳过。
// @param t 测试上下文
func TestExecutionListenerHistoryDisabled(t *testing.T) {
	engine, err := New(Config{HistoryLevel: HistoryNone})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="noHistory" isExecutable="true">
    <extensionElements>
      <flowable:executionListener event="start" class="noHistoryProcessStart"/>
      <flowable:executionListener event="end" class="noHistoryProcessEnd"/>
    </extensionElements>
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="task"/>
    <userTask id="task" name="T" flowable:assignee="alice">
      <extensionElements>
        <flowable:executionListener class="noHistoryStart"/>
        <flowable:executionListener event="end" class="noHistoryEnd"/>
      </extensionElements>
    </userTask>
    <sequenceFlow id="f2" sourceRef="task" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`
	deployXML(t, engine, xml)
	processStarts, processEnds, nodeStarts, nodeEnds := 0, 0, 0, 0
	engine.RegisterExecutionListener("noHistoryProcessStart", func(ctx context.Context, execution *Execution) error {
		processStarts++
		return nil
	})
	engine.RegisterExecutionListener("noHistoryProcessEnd", func(ctx context.Context, execution *Execution) error {
		processEnds++
		return nil
	})
	engine.RegisterExecutionListener("noHistoryStart", func(ctx context.Context, execution *Execution) error {
		nodeStarts++
		return nil
	})
	engine.RegisterExecutionListener("noHistoryEnd", func(ctx context.Context, execution *Execution) error {
		nodeEnds++
		return nil
	})
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "noHistory"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if processStarts != 1 || nodeStarts != 1 {
		t.Fatalf("历史关闭时开始监听器应执行: %d %d", processStarts, nodeStarts)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 {
		t.Fatalf("任务缺失: %+v", tasks)
	}
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{UserID: "alice"}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	if processEnds != 1 {
		t.Fatalf("历史关闭时流程结束监听器应执行: %d", processEnds)
	}
	if nodeEnds != 0 {
		t.Fatalf("历史关闭时活动结束监听器不应执行: %d", nodeEnds)
	}
}

// TestExecutionListenerHelperBranches 验证监听器查找的缺失路径不产生 panic。
// @param t 测试上下文
func TestExecutionListenerHelperBranches(t *testing.T) {
	app, err := New(Config{})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	cc := &commandContext{engine: app, ctx: context.Background(), now: time.Now()}
	app.invokeActivityEndExecutionListeners(cc, nil)
	if executionListenersOf(nil) != nil {
		t.Fatalf("空元素不应有监听器")
	}
	app.invokeActivityEndExecutionListeners(cc, &api.HistoricActivityInstance{ProcessInstanceID: "missing"})
	if err := app.invokeProcessExecutionListeners(cc, &instanceState{ID: "missing"}, "start"); err != nil {
		t.Fatalf("缺失定义的流程监听器应跳过: %v", err)
	}
	// 实例存在但根执行缺失
	app.state.Instances["i"] = &instanceState{ID: "i", RootExecutionID: "root", ProcessDefinitionID: "d"}
	app.invokeActivityEndExecutionListeners(cc, &api.HistoricActivityInstance{ProcessInstanceID: "i", ActivityID: "task"})
	if err := app.invokeProcessExecutionListeners(cc, app.state.Instances["i"], "start"); err != nil {
		t.Fatalf("缺失根执行应跳过: %v", err)
	}
	// 定义缺失
	app.state.Executions["root"] = &executionState{ID: "root", ProcessInstanceID: "i"}
	app.invokeActivityEndExecutionListeners(cc, &api.HistoricActivityInstance{ProcessInstanceID: "i", ActivityID: "task"})
	// 定义存在但流程为空
	app.state.Definitions["d"] = &definitionState{Info: api.ProcessDefinition{ID: "d"}}
	app.invokeActivityEndExecutionListeners(cc, &api.HistoricActivityInstance{ProcessInstanceID: "i", ActivityID: "task"})
	if err := app.invokeProcessExecutionListeners(cc, app.state.Instances["i"], "start"); err != nil {
		t.Fatalf("空流程模型应跳过: %v", err)
	}
	// 流程存在但活动缺失 / 活动无监听器
	process := &model.Process{ID: "d", Elements: map[string]model.FlowElement{
		"flow": &model.SequenceFlow{BaseElement: model.BaseElement{ID: "flow"}},
	}}
	app.state.Definitions["d"] = &definitionState{
		Info:  api.ProcessDefinition{ID: "d"},
		Model: &model.BpmnModel{Processes: []*model.Process{process}},
	}
	app.invokeActivityEndExecutionListeners(cc, &api.HistoricActivityInstance{ProcessInstanceID: "i", ActivityID: "missing"})
	app.invokeActivityEndExecutionListeners(cc, &api.HistoricActivityInstance{ProcessInstanceID: "i", ActivityID: "flow"})
	if err := app.invokeProcessExecutionListeners(cc, app.state.Instances["i"], "start"); err != nil {
		t.Fatalf("无流程级监听器应跳过: %v", err)
	}
	// 流程定义了监听器但根执行缺失
	app.state.Instances["i"].RootExecutionID = "absent"
	app.state.Definitions["d"] = &definitionState{
		Info: api.ProcessDefinition{ID: "d"},
		Model: &model.BpmnModel{Processes: []*model.Process{{
			ID:                 "d",
			ExecutionListeners: []model.ExecutionListenerDefinition{{Event: "start", ClassName: "any"}},
		}}},
	}
	if err := app.invokeProcessExecutionListeners(cc, app.state.Instances["i"], "start"); err != nil {
		t.Fatalf("根执行缺失应跳过: %v", err)
	}
	// 直接调用时缺省事件同样按 start 处理
	fired := 0
	app.executionListeners["defaultStart"] = func(ctx context.Context, execution *Execution) error {
		fired++
		return nil
	}
	definitions := []model.ExecutionListenerDefinition{{ClassName: "defaultStart"}}
	if err := app.invokeExecutionListeners(cc, newExecution(app, app.state.Executions["root"]), definitions, "start"); err != nil {
		t.Fatalf("缺省事件监听器执行失败: %v", err)
	}
	if fired != 1 {
		t.Fatalf("缺省事件监听器应执行: %d", fired)
	}
	if err := app.invokeExecutionListeners(cc, newExecution(app, app.state.Executions["root"]), definitions, "end"); err != nil {
		t.Fatalf("不匹配事件应跳过: %v", err)
	}
	if fired != 1 {
		t.Fatalf("不匹配事件不应执行: %d", fired)
	}
}
