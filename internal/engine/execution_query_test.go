package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/internal/tenant"

	"github.com/flowable-go/flowable/api"
)

// TestExecutionQueries 验证执行查询。
// @param t 测试上下文
func TestExecutionQueries(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	executions, err := engine.ListExecutions(context.Background(), instance.ID)
	if err != nil || len(executions) == 0 {
		t.Fatalf("执行列表错误: %v %d", err, len(executions))
	}
	execution, err := engine.GetExecution(context.Background(), executions[0].ID)
	if err != nil || execution.ProcessInstanceID != instance.ID {
		t.Fatalf("执行查询失败: %v %+v", err, execution)
	}
	if _, err := engine.GetExecution(context.Background(), "missing"); err == nil {
		t.Fatalf("缺失执行应报错")
	}
	if all, err := engine.ListExecutions(context.Background(), ""); err != nil || len(all) == 0 {
		t.Fatalf("全部执行查询失败: %v", err)
	}
	historic, err := engine.GetHistoricProcessInstance(context.Background(), instance.ID)
	if err != nil || historic.ID != instance.ID {
		t.Fatalf("历史流程查询失败: %v", err)
	}
	if _, err := engine.GetHistoricProcessInstance(context.Background(), "missing"); err == nil {
		t.Fatalf("缺失历史流程应报错")
	}
	tasks := listTasks(t, engine)
	if _, err := engine.GetHistoricTaskInstance(context.Background(), tasks[0].ID); err != nil {
		t.Fatalf("历史任务查询失败: %v", err)
	}
	if _, err := engine.GetHistoricTaskInstance(context.Background(), "missing"); err == nil {
		t.Fatalf("缺失历史任务应报错")
	}
}

// waitingActivityExecutionID 返回测试实例唯一的等待活动执行，禁止把流程根当作活动。
// @param t 测试上下文
// @param app 引擎
// @param instanceID 实例标识
// @return 等待活动执行标识
func waitingActivityExecutionID(t *testing.T, app *Engine, instanceID string) string {
	t.Helper()
	executions, err := app.ListExecutions(context.Background(), instanceID)
	if err != nil {
		t.Fatal(err)
	}
	id := ""
	for _, execution := range executions {
		if execution.Ended || !execution.Waiting || execution.ActivityID == "" {
			continue
		}
		if id != "" {
			t.Fatal("测试实例存在多个等待活动")
		}
		id = execution.ID
	}
	if id == "" {
		t.Fatal("测试实例没有等待活动")
	}
	return id
}

// TestExecutionVariables 验证执行变量与租户。
// @param t 测试上下文
func TestExecutionVariables(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	executions, _ := engine.ListExecutions(context.Background(), instance.ID)
	values, err := engine.GetExecutionVariables(context.Background(), executions[0].ID)
	if err != nil || values["initiator"] != "alice" {
		t.Fatalf("执行变量错误: %v %+v", err, values)
	}
	if _, err := engine.GetExecutionVariables(context.Background(), "missing"); err == nil {
		t.Fatalf("缺失执行应失败")
	}
	other := tenant.With(context.Background(), "t2")
	if _, err := engine.GetExecutionVariables(other, executions[0].ID); err == nil {
		t.Fatalf("跨租户执行变量应失败")
	}
}
