package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/engine"
)

const restConditionalXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="restConditional" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="review"/>
    <userTask id="review" name="Review"/>
    <boundaryEvent id="amountReached" attachedToRef="review">
      <conditionalEventDefinition>
        <condition>${amount > 100}</condition>
      </conditionalEventDefinition>
    </boundaryEvent>
    <sequenceFlow id="f2" sourceRef="review" targetRef="normalEnd"/>
    <sequenceFlow id="f3" sourceRef="amountReached" targetRef="boundaryEnd"/>
    <endEvent id="normalEnd"/>
    <endEvent id="boundaryEnd"/>
  </process>
</definitions>`

// newConditionalRESTServer 创建条件事件测试服务。
// @param t 测试上下文
// @return 引擎
// @return REST 服务
func newConditionalRESTServer(t *testing.T) (*engine.Engine, *Server) {
	t.Helper()
	app, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	t.Cleanup(func() { _ = app.Close(context.Background()) })
	return app, New(app)
}

// deployConditionalXML 部署条件事件测试流程。
// @param t 测试上下文
// @param app 引擎
func deployConditionalXML(t *testing.T, app *engine.Engine) {
	t.Helper()
	if _, err := app.Deploy(context.Background(), api.DeploymentRequest{
		Name:      "conditional",
		Resources: map[string][]byte{"conditional.bpmn20.xml": []byte(restConditionalXML)},
	}); err != nil {
		t.Fatalf("部署失败: %v", err)
	}
}

// startConditional 启动条件事件测试实例。
// @param t 测试上下文
// @param app 引擎
// @param amount 金额变量
// @return 实例
func startConditional(t *testing.T, app *engine.Engine, amount int) api.ProcessInstance {
	t.Helper()
	instance, err := app.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "restConditional",
		Variables:            []api.Variable{{Name: "amount", Value: amount}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	return instance
}

// callEvaluateConditions 直接调用条件求值处理器。
// @param server REST 服务
// @param instanceID 流程实例 ID
// @return 响应记录器
func callEvaluateConditions(server *Server, instanceID string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/service/runtime/process-instances/"+instanceID+"/evaluate-conditions", nil)
	request.SetPathValue("id", instanceID)
	recorder := httptest.NewRecorder()
	server.handleEvaluateConditions(recorder, request)
	return recorder
}

// TestREST_EvaluateConditions 验证显式求值端点成功与错误语义。
// @param t 测试上下文
func TestREST_EvaluateConditions(t *testing.T) {
	app, server := newConditionalRESTServer(t)
	deployConditionalXML(t, app)

	running := startConditional(t, app, 50)
	if recorder := callEvaluateConditions(server, running.ID); recorder.Code != http.StatusNoContent {
		t.Fatalf("期望 204，实际 %d: %s", recorder.Code, recorder.Body.String())
	}

	if recorder := callEvaluateConditions(server, "missing"); recorder.Code != http.StatusNotFound {
		t.Fatalf("实例不存在期望 404，实际 %d: %s", recorder.Code, recorder.Body.String())
	}

	suspended := startConditional(t, app, 50)
	if err := app.SuspendProcessInstance(context.Background(), suspended.ID); err != nil {
		t.Fatalf("挂起失败: %v", err)
	}
	if recorder := callEvaluateConditions(server, suspended.ID); recorder.Code != http.StatusConflict {
		t.Fatalf("挂起实例期望 409，实际 %d: %s", recorder.Code, recorder.Body.String())
	}

	ended := startConditional(t, app, 200)
	if recorder := callEvaluateConditions(server, ended.ID); recorder.Code != http.StatusConflict {
		t.Fatalf("已结束实例期望 409，实际 %d: %s", recorder.Code, recorder.Body.String())
	}
}
