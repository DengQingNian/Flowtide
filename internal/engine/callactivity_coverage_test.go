package engine

import (
	"context"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// TestCallActivity_ErrorBranches 验证调用活动错误分支。
// @param t 测试上下文
func TestCallActivity_ErrorBranches(t *testing.T) {
	cases := []struct {
		name  string
		attrs string
		extra string
	}{
		{"缺少 calledElement", "", ""},
		{"入参缺少 target", `calledElement="child"`, `<extensionElements><flowable:in sourceExpression="${1}"/></extensionElements>`},
		{"入参表达式错误", `calledElement="child"`, `<extensionElements><flowable:in sourceExpression="${1 +}" target="x"/></extensionElements>`},
		{"业务键错误", `calledElement="child" flowable:businessKey="${1 +}"`, ""},
		{"实例名错误", `calledElement="child" flowable:processInstanceName="${1 +}"`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := newTestEngine(t)
			xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="child" isExecutable="true"><startEvent id="cs"/><sequenceFlow id="cf" sourceRef="cs" targetRef="ce"/><endEvent id="ce"/></process>
  <process id="parentErr" isExecutable="true"><startEvent id="ps"/><sequenceFlow id="pf" sourceRef="ps" targetRef="call"/>
    <callActivity id="call" ` + tc.attrs + `>` + tc.extra + `</callActivity>
    <sequenceFlow id="pf2" sourceRef="call" targetRef="pe"/><endEvent id="pe"/>
  </process>
</definitions>`
			deployXML(t, engine, xml)
			if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "parentErr"}); err == nil {
				t.Fatalf("期望调用活动失败")
			}
		})
	}
}

// TestCallActivity_SuspendedChild 验证挂起子流程。
// @param t 测试上下文
func TestCallActivity_SuspendedChild(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, callActivityXML)
	definition, _ := engine.GetProcessDefinitionByKey(context.Background(), "child")
	engine.state.Definitions[definition.ID].Info.Suspended = true
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "parent"}); err == nil {
		t.Fatalf("挂起子流程应拒绝调用")
	}
}

// TestCallActivity_ResumeParentBranches 验证恢复父执行空路径。
// @param t 测试上下文
func TestCallActivity_ResumeParentBranches(t *testing.T) {
	engine := newTestEngine(t)
	cc := &commandContext{engine: engine, ctx: context.Background(), now: time.Now()}
	engine.resumeParent(cc, &instanceState{ID: "child", SuperExecutionID: "missing"})
	engine.state.Executions["parent"] = &executionState{ID: "parent", ProcessInstanceID: "parentInstance", Ended: true}
	engine.state.Instances["parentInstance"] = &instanceState{ID: "parentInstance", RootExecutionID: "parent"}
	engine.resumeParent(cc, &instanceState{ID: "child", SuperExecutionID: "parent"})
}

// TestCallActivity_IOValue 验证参数求值空路径。
// @param t 测试上下文
func TestCallActivity_IOValue(t *testing.T) {
	engine := newTestEngine(t)
	execution := &executionState{ID: "e", Variables: map[string]any{"x": 1}}
	if value, err := engine.ioValue(execution, &model.IOParameter{}, nil); err != nil || value != nil {
		t.Fatalf("空参数应返回 nil: %v %v", value, err)
	}
	if _, err := engine.ioValue(execution, &model.IOParameter{SourceExpression: "${1 +}"}, nil); err == nil {
		t.Fatalf("非法表达式应报错")
	}
	if value, err := engine.ioValue(execution, &model.IOParameter{Source: "x"}, nil); err != nil || value != 1 {
		t.Fatalf("源变量读取失败: %v %v", value, err)
	}
}

// TestCallActivity_DepthAndTenant 验证调用深度与租户辅助函数。
// @param t 测试上下文
func TestCallActivity_DepthAndTenant(t *testing.T) {
	engine := newTestEngine(t)
	execution := &executionState{ID: "e", ProcessInstanceID: "missing"}
	if depth := engine.callDepth(execution); depth != 1 {
		t.Fatalf("缺失实例深度错误: %d", depth)
	}
	if tenant := parentTenant(engine, execution); tenant != "" {
		t.Fatalf("缺失实例租户应为空: %s", tenant)
	}
	engine.state.Instances["i"] = &instanceState{ID: "i", TenantID: "t1"}
	engine.state.Executions["e2"] = &executionState{ID: "e2", ProcessInstanceID: "i"}
	if tenant := parentTenant(engine, engine.state.Executions["e2"]); tenant != "t1" {
		t.Fatalf("租户读取错误: %s", tenant)
	}
}
