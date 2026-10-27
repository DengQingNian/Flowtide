package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
)

const parallelMIXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <process id="miParallel" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="review"/>
    <userTask id="review" name="Review" flowable:assignee="${approver}">
      <multiInstanceLoopCharacteristics flowable:collection="${approvers}" flowable:elementVariable="approver"/>
    </userTask>
    <sequenceFlow id="f2" sourceRef="review" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// TestMultiInstance_Parallel 验证并行多实例。
// @param t 测试上下文
func TestMultiInstance_Parallel(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, parallelMIXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "miParallel",
		Variables:            []api.Variable{{Name: "approvers", Value: []string{"alice", "bob"}}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 2 {
		t.Fatalf("期望 2 个任务，实际 %d", len(tasks))
	}
	assignees := map[string]bool{}
	for _, task := range tasks {
		assignees[task.Assignee] = true
	}
	if !assignees["alice"] || !assignees["bob"] {
		t.Fatalf("处理人错误: %+v", assignees)
	}
	for _, task := range tasks {
		if err := engine.Complete(context.Background(), task.ID, api.CompleteTaskRequest{}); err != nil {
			t.Fatalf("完成任务失败: %v", err)
		}
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("并行多实例完成后应结束")
	}
}

const sequentialMIXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <process id="miSequential" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="review"/>
    <userTask id="review" name="Review" flowable:assignee="${approver}">
      <multiInstanceLoopCharacteristics isSequential="true" flowable:collection="${approvers}" flowable:elementVariable="approver"/>
    </userTask>
    <sequenceFlow id="f2" sourceRef="review" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// TestMultiInstance_Sequential 验证串行多实例。
// @param t 测试上下文
func TestMultiInstance_Sequential(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, sequentialMIXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "miSequential",
		Variables:            []api.Variable{{Name: "approvers", Value: []string{"alice", "bob"}}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	for i := 0; i < 2; i++ {
		tasks := listTasks(t, engine)
		if len(tasks) != 1 {
			t.Fatalf("第 %d 轮期望 1 个任务，实际 %d", i, len(tasks))
		}
		if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{}); err != nil {
			t.Fatalf("完成任务失败: %v", err)
		}
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("串行多实例完成后应结束")
	}
}

const cardinalityMIXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="miCardinality" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="review"/>
    <userTask id="review" name="Review">
      <multiInstanceLoopCharacteristics>
        <loopCardinality>3</loopCardinality>
        <completionCondition>${nrOfCompletedInstances &gt;= 2}</completionCondition>
      </multiInstanceLoopCharacteristics>
    </userTask>
    <sequenceFlow id="f2" sourceRef="review" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// TestMultiInstance_CompletionCondition 验证完成条件提前结束。
// @param t 测试上下文
func TestMultiInstance_CompletionCondition(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, cardinalityMIXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "miCardinality"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 3 {
		t.Fatalf("期望 3 个任务，实际 %d", len(tasks))
	}
	for i := 0; i < 2; i++ {
		if err := engine.Complete(context.Background(), tasks[i].ID, api.CompleteTaskRequest{}); err != nil {
			t.Fatalf("完成任务失败: %v", err)
		}
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("完成条件满足后应结束")
	}
	if len(listTasks(t, engine)) != 0 {
		t.Fatalf("剩余任务应被取消")
	}
}

const zeroMIXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <process id="miZero" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="review"/>
    <userTask id="review" name="Review">
      <multiInstanceLoopCharacteristics flowable:collection="${approvers}" flowable:elementVariable="approver"/>
    </userTask>
    <sequenceFlow id="f2" sourceRef="review" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// TestMultiInstance_Zero 验证零实例直接完成。
// @param t 测试上下文
func TestMultiInstance_Zero(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, zeroMIXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "miZero",
		Variables:            []api.Variable{{Name: "approvers", Value: []string{}}},
	})
	if err != nil || !instance.Ended {
		t.Fatalf("零实例应直接结束: %v %+v", err, instance)
	}
}

// TestMultiInstance_InvalidCollection 验证非法集合。
// @param t 测试上下文
func TestMultiInstance_InvalidCollection(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, zeroMIXML)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "miZero",
		Variables:            []api.Variable{{Name: "approvers", Value: "not-a-slice"}},
	}); err == nil {
		t.Fatalf("非法集合应报错")
	}
}

const serviceMIXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <process id="miService" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="svc"/>
    <serviceTask id="svc" flowable:delegateExpression="${collect}">
      <multiInstanceLoopCharacteristics flowable:collection="${items}" flowable:elementVariable="item"/>
    </serviceTask>
    <sequenceFlow id="f2" sourceRef="svc" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// TestMultiInstance_ServiceTask 验证服务任务多实例。
// @param t 测试上下文
func TestMultiInstance_ServiceTask(t *testing.T) {
	engine := newTestEngine(t)
	count := 0
	engine.RegisterDelegate("collect", func(ctx context.Context, execution *Execution) error {
		count++
		if _, ok := execution.GetVariable("item"); !ok {
			return api.NewError("TEST", "缺少 item 变量", nil)
		}
		return nil
	})
	deployXML(t, engine, serviceMIXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "miService",
		Variables:            []api.Variable{{Name: "items", Value: []any{"a", "b", "c"}}},
	})
	if err != nil || !instance.Ended {
		t.Fatalf("服务多实例失败: %v %+v", err, instance)
	}
	if count != 3 {
		t.Fatalf("期望执行 3 次，实际 %d", count)
	}
}
