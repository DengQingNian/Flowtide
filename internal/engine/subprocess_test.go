package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
)

const subProcessXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <process id="sub" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="sp"/>
    <subProcess id="sp" name="Sub">
      <startEvent id="s2"/>
      <sequenceFlow id="sf1" sourceRef="s2" targetRef="inner"/>
      <userTask id="inner" name="Inner" flowable:assignee="${initiator}"/>
      <sequenceFlow id="sf2" sourceRef="inner" targetRef="e2"/>
      <endEvent id="e2"/>
    </subProcess>
    <sequenceFlow id="f2" sourceRef="sp" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// TestSubProcess_CompletesAndResumes 验证子流程完成后恢复父流程。
// @param t 测试上下文
func TestSubProcess_CompletesAndResumes(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, subProcessXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "sub",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "inner" {
		t.Fatalf("期望子流程任务，实际 %+v", tasks)
	}
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{UserID: "alice"}); err != nil {
		t.Fatalf("完成子流程任务失败: %v", err)
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("子流程完成后应结束主流程")
	}
}

const nestedSubProcessXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="nested" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="outer"/>
    <subProcess id="outer">
      <startEvent id="os"/>
      <sequenceFlow id="of1" sourceRef="os" targetRef="innerSp"/>
      <subProcess id="innerSp">
        <startEvent id="is"/>
        <sequenceFlow id="if1" sourceRef="is" targetRef="deep"/>
        <userTask id="deep" name="Deep"/>
        <sequenceFlow id="if2" sourceRef="deep" targetRef="ie"/>
        <endEvent id="ie"/>
      </subProcess>
      <sequenceFlow id="of2" sourceRef="innerSp" targetRef="oe"/>
      <endEvent id="oe"/>
    </subProcess>
    <sequenceFlow id="f2" sourceRef="outer" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// TestSubProcess_Nested 验证嵌套子流程。
// @param t 测试上下文
func TestSubProcess_Nested(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, nestedSubProcessXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "nested"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "deep" {
		t.Fatalf("期望最内层任务，实际 %+v", tasks)
	}
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("嵌套子流程完成后应结束")
	}
}

const subProcessParallelXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="subParallel" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="sp"/>
    <subProcess id="sp">
      <startEvent id="ss"/>
      <sequenceFlow id="sf1" sourceRef="ss" targetRef="fork"/>
      <parallelGateway id="fork"/>
      <sequenceFlow id="sa" sourceRef="fork" targetRef="a"/>
      <sequenceFlow id="sb" sourceRef="fork" targetRef="b"/>
      <userTask id="a" name="A"/>
      <userTask id="b" name="B"/>
      <sequenceFlow id="fa" sourceRef="a" targetRef="join"/>
      <sequenceFlow id="fb" sourceRef="b" targetRef="join"/>
      <parallelGateway id="join"/>
      <sequenceFlow id="sf2" sourceRef="join" targetRef="se"/>
      <endEvent id="se"/>
    </subProcess>
    <sequenceFlow id="f2" sourceRef="sp" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// TestSubProcess_ParallelInside 验证子流程内并行汇聚。
// @param t 测试上下文
func TestSubProcess_ParallelInside(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, subProcessParallelXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "subParallel"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 2 {
		t.Fatalf("期望 2 个任务，实际 %d", len(tasks))
	}
	for _, task := range tasks {
		if err := engine.Complete(context.Background(), task.ID, api.CompleteTaskRequest{}); err != nil {
			t.Fatalf("完成任务失败: %v", err)
		}
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("子流程并行汇聚后应结束")
	}
}
