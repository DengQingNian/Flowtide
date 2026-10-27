package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
)

// TestImportHistoricRows 验证历史行往返恢复。
// @param t 测试上下文
func TestImportHistoricRows(t *testing.T) {
	source := newTestEngine(t)
	deployXML(t, source, simpleProcess)
	instance, _ := source.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	tasks := listTasks(t, source)
	if err := source.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	rows, err := source.ExportHistoricRows(context.Background())
	if err != nil {
		t.Fatalf("导出历史失败: %v", err)
	}
	target := newTestEngine(t)
	if err := target.ImportHistoricRows(context.Background(), rows); err != nil {
		t.Fatalf("导入历史失败: %v", err)
	}
	process, err := target.GetHistoricProcessInstance(context.Background(), instance.ID)
	if err != nil || !process.Ended {
		t.Fatalf("历史流程恢复错误: %v %+v", err, process)
	}
	processes, _ := target.ListHistoricProcessInstances(context.Background())
	if len(processes) != 1 {
		t.Fatalf("历史流程列表错误: %+v", processes)
	}
	activities, _ := target.ListHistoricActivityInstances(context.Background(), instance.ID)
	if len(activities) == 0 {
		t.Fatalf("历史活动未恢复")
	}
	historicTasks, _ := target.ListHistoricTaskInstances(context.Background(), instance.ID)
	if len(historicTasks) != 1 || historicTasks[0].Assignee != "alice" {
		t.Fatalf("历史任务恢复错误: %+v", historicTasks)
	}
	variables, _ := target.ListHistoricVariableInstances(context.Background(), instance.ID)
	if len(variables) != 1 || variables[0].Value != "alice" {
		t.Fatalf("历史变量恢复错误: %+v", variables)
	}
}

// TestImportHistoricRows_Empty 验证空历史导入。
// @param t 测试上下文
func TestImportHistoricRows_Empty(t *testing.T) {
	engine := newTestEngine(t)
	if err := engine.ImportHistoricRows(context.Background(), nil); err != nil {
		t.Fatalf("空历史导入失败: %v", err)
	}
}

// TestImportHistoricRows_Errors 验证历史导入错误。
// @param t 测试上下文
func TestImportHistoricRows_Errors(t *testing.T) {
	engine := newTestEngine(t)
	if err := engine.ImportHistoricRows(context.Background(), []flowablerow.Row{{Table: "BAD"}}); err == nil {
		t.Fatalf("非法历史表应失败")
	}
	_ = engine.Close(context.Background())
	if err := engine.ImportHistoricRows(context.Background(), nil); err == nil {
		t.Fatalf("关闭后导入历史应失败")
	}
}
