package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// serviceXML 构造只含一个服务任务的流程。
// @param id 流程 ID
// @param attrs 服务任务属性
// @return BPMN XML
func serviceXML(id, attrs string) string {
	return `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="` + id + `" isExecutable="true"><startEvent id="start"/><sequenceFlow id="f1" sourceRef="start" targetRef="svc"/><serviceTask id="svc" ` + attrs + `/><sequenceFlow id="f2" sourceRef="svc" targetRef="end"/><endEvent id="end"/></process></definitions>`
}

// TestServiceTask_ExpressionResult 验证表达式结果变量。
// @param t 测试上下文
func TestServiceTask_ExpressionResult(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, serviceXML("exprResult", `flowable:expression="${1 + 1}" flowable:resultVariable="sum"`))
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "exprResult"})
	if err != nil || !instance.Ended {
		t.Fatalf("表达式服务任务失败: %v", err)
	}
	variables, _ := engine.GetVariables(context.Background(), instance.ID)
	if variables["sum"] != 2.0 {
		t.Fatalf("结果变量错误: %+v", variables)
	}
}

// TestServiceTask_Errors 验证服务任务错误分支。
// @param t 测试上下文
func TestServiceTask_Errors(t *testing.T) {
	cases := []struct {
		name  string
		attrs string
	}{
		{"未知委托表达式", `flowable:expression="${'missingDelegate'}"`},
		{"Java 类", `flowable:class="com.example.Handler"`},
		{"缺少实现", `name="NoImpl"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := newTestEngine(t)
			deployXML(t, engine, serviceXML("svcError", tc.attrs))
			if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "svcError"}); err == nil {
				t.Fatalf("期望服务任务失败")
			}
		})
	}
}

// TestServiceTask_DelegateError 验证委托返回错误。
// @param t 测试上下文
func TestServiceTask_DelegateError(t *testing.T) {
	engine := newTestEngine(t)
	engine.RegisterDelegate("fail", func(ctx context.Context, execution *Execution) error {
		return errors.New("委托失败")
	})
	deployXML(t, engine, serviceXML("delegateError", `flowable:delegateExpression="${fail}"`))
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "delegateError"}); err == nil {
		t.Fatalf("期望委托错误传播")
	}
}

// TestServiceTask_DelegateFromVariable 验证从变量解析委托名。
// @param t 测试上下文
func TestServiceTask_DelegateFromVariable(t *testing.T) {
	engine := newTestEngine(t)
	engine.RegisterDelegate("noop", func(ctx context.Context, execution *Execution) error { return nil })
	deployXML(t, engine, serviceXML("delegateVar", `flowable:delegateExpression="${delegateName}"`))
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "delegateVar",
		Variables:            []api.Variable{{Name: "delegateName", Value: "noop"}},
	})
	if err != nil || !instance.Ended {
		t.Fatalf("变量委托失败: %v", err)
	}
}

// TestServiceTask_BareDelegate 验证裸委托名。
// @param t 测试上下文
func TestServiceTask_BareDelegate(t *testing.T) {
	engine := newTestEngine(t)
	engine.RegisterDelegate("noop", func(ctx context.Context, execution *Execution) error { return nil })
	deployXML(t, engine, serviceXML("delegateBare", `flowable:delegateExpression="noop"`))
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "delegateBare"}); err != nil {
		t.Fatalf("裸委托失败: %v", err)
	}
}
