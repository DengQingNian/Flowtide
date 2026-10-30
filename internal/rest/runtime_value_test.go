package rest

import (
	"context"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/engine"
)

// TestRuntimeTypedVariablesAtomicity 验证启动及触发先解析整组类型，非法变量不能产生实例或推进执行。
// @param t 测试上下文
// @return 无
func TestRuntimeTypedVariablesAtomicity(t *testing.T) {
	ctx := context.Background()
	app, err := engine.New(engine.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Deploy(ctx, api.DeploymentRequest{Resources: map[string][]byte{"receive.bpmn20.xml": []byte(restTriggerXML)}}); err != nil {
		t.Fatal(err)
	}
	handler := New(app, WithBasicAuth(map[string]string{"starter": "reference-secret"})).Handler()
	for _, variables := range []string{
		`[{"name":"amount","type":"long","value":9007199254740993},{"name":"date","type":"date","value":"invalid"}]`,
		`[{"name":"","value":1}]`, `"invalid"`, `{"amount":{"unclosed":true}`,
	} {
		response := runtimeJavaRequest(handler, "starter", "POST", "/service/runtime/process-instances", `{"processDefinitionKey":"restTrigger","variables":`+variables+`}`)
		if response.Code != 400 {
			t.Fatalf("非法变量未拒绝: %d %s", response.Code, response.Body.String())
		}
		instances, err := app.ListProcessInstances(ctx)
		if err != nil || len(instances) != 0 {
			t.Fatalf("失败请求仍创建实例: %+v %v", instances, err)
		}
	}
	// 保留对象形式扩展时也必须保证长整数精度，不能使用默认浮点解码。
	response := runtimeJavaRequest(handler, "starter", "POST", "/service/runtime/process-instances", `{"processDefinitionKey":"restTrigger","variables":{"amount":9007199254740993}}`)
	if response.Code != 201 {
		t.Fatalf("启动失败: %d %s", response.Code, response.Body.String())
	}
	instances, _ := app.ListProcessInstances(ctx)
	if len(instances) != 1 {
		t.Fatal("启动实例数错误")
	}
	instance := instances[0]
	value, err := app.GetVariable(ctx, instance.ID, "amount")
	if err != nil || value != int64(9007199254740993) {
		t.Fatalf("启动变量精度丢失: %T %v %v", value, value, err)
	}
	child := runtimeJavaChild(t, app, instance.ID)
	response = runtimeJavaRequest(handler, "starter", "PUT", "/service/runtime/executions/"+child, `{"action":"trigger","variables":[{"name":"amount","type":"long","value":2}],"transientVariables":[{"name":"date","type":"date","value":"invalid"}]}`)
	if response.Code != 400 {
		t.Fatalf("非法瞬态变量未拒绝: %d", response.Code)
	}
	value, _ = app.GetVariable(ctx, instance.ID, "amount")
	execution, _ := app.GetExecution(ctx, child)
	if value != int64(9007199254740993) || execution.ActivityID != "recv" {
		t.Fatalf("失败触发改变了变量或活动: %v %+v", value, execution)
	}
	response = runtimeJavaRequest(handler, "starter", "PUT", "/service/runtime/executions/"+child, `{"action":"trigger","variables":[{"name":"amount","type":"long","value":9007199254740995}]}`)
	if response.Code != 200 {
		t.Fatalf("类型化触发失败: %d %s", response.Code, response.Body.String())
	}
	value, _ = app.GetVariable(ctx, instance.ID, "amount")
	if value != int64(9007199254740995) {
		t.Fatalf("触发变量精度丢失: %T %v", value, value)
	}
}
