package engine

import (
	"context"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

const compensationXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <process id="compensation" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="doWork"/>
    <serviceTask id="doWork" flowable:delegateExpression="${doWork}"/>
    <boundaryEvent id="comp" attachedToRef="doWork"><compensateEventDefinition/></boundaryEvent>
    <serviceTask id="undoWork" isForCompensation="true" flowable:delegateExpression="${undoWork}"/>
    <association id="assoc" sourceRef="comp" targetRef="undoWork"/>
    <sequenceFlow id="f2" sourceRef="doWork" targetRef="throwComp"/>
    <intermediateThrowEvent id="throwComp"><compensateEventDefinition/></intermediateThrowEvent>
    <sequenceFlow id="f3" sourceRef="throwComp" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestCompensation_ServiceHandler 验证补偿登记与逆序执行。
// @param t 测试上下文
func TestCompensation_ServiceHandler(t *testing.T) {
	engine := newTestEngine(t)
	worked := false
	undone := false
	engine.RegisterDelegate("doWork", func(ctx context.Context, execution *Execution) error {
		worked = true
		return nil
	})
	engine.RegisterDelegate("undoWork", func(ctx context.Context, execution *Execution) error {
		undone = true
		return nil
	})
	deployXML(t, engine, compensationXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "compensation"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if !worked || !undone {
		t.Fatalf("期望工作与补偿都执行: worked=%v undone=%v", worked, undone)
	}
	if !instance.Ended {
		t.Fatalf("补偿后流程应结束")
	}
	if len(engine.state.Compensations[instance.ID]) != 0 {
		t.Fatalf("补偿登记应清空")
	}
}

// TestCompensation_UserHandlerUnsupported 验证不支持的补偿处理器。
// @param t 测试上下文
func TestCompensation_UserHandlerUnsupported(t *testing.T) {
	engine := newTestEngine(t)
	engine.RegisterDelegate("doWork", func(ctx context.Context, execution *Execution) error { return nil })
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="compUser" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="doWork"/><serviceTask id="doWork" flowable:delegateExpression="${doWork}"/><boundaryEvent id="comp" attachedToRef="doWork"><compensateEventDefinition/></boundaryEvent><userTask id="undo" isForCompensation="true"/><association id="assoc" sourceRef="comp" targetRef="undo"/><sequenceFlow id="f2" sourceRef="doWork" targetRef="throwComp"/><intermediateThrowEvent id="throwComp"><compensateEventDefinition/></intermediateThrowEvent><sequenceFlow id="f3" sourceRef="throwComp" targetRef="e"/><endEvent id="e"/></process></definitions>`
	deployXML(t, engine, xml)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "compUser"}); err == nil {
		t.Fatalf("用户任务补偿处理器应报错")
	}
}
