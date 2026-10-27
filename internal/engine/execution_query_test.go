package engine

import (
	"context"
	"testing"

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
