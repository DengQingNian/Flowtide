package engine

import (
	"context"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
)

// TestImportRuntimeRows 验证运行时行往返恢复。
// @param t 测试上下文
func TestImportRuntimeRows(t *testing.T) {
	source := newTestEngine(t)
	deployXML(t, source, simpleProcess)
	instance, err := source.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	task := listTasks(t, source)[0]
	if err := source.SetTaskVariable(context.Background(), task.ID, "local", "v"); err != nil {
		t.Fatalf("设置任务变量失败: %v", err)
	}
	repositoryRows, _ := source.ExportRows(context.Background())
	runtimeRows, err := source.ExportRuntimeRows(context.Background())
	if err != nil {
		t.Fatalf("导出运行时行失败: %v", err)
	}
	target := newTestEngine(t)
	if err := target.ImportRows(context.Background(), repositoryRows); err != nil {
		t.Fatalf("导入仓库失败: %v", err)
	}
	if err := target.ImportRuntimeRows(context.Background(), runtimeRows); err != nil {
		t.Fatalf("导入运行时失败: %v", err)
	}
	tasks, _ := target.ListTasks(context.Background(), api.TaskFilter{})
	if tasks.Total != 1 || tasks.Data[0].ID != task.ID || tasks.Data[0].Assignee != "alice" {
		t.Fatalf("恢复任务错误: %+v", tasks)
	}
	values, _ := target.GetTaskVariables(context.Background(), task.ID)
	if values["initiator"] != "alice" || values["local"] != "v" {
		t.Fatalf("恢复变量错误: %+v", values)
	}
	if err := target.Complete(context.Background(), task.ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatalf("恢复后完成任务失败: %v", err)
	}
	got, _ := target.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("恢复后流程应结束")
	}
}

// TestImportRuntimeRows_Errors 验证运行时导入错误。
// @param t 测试上下文
func TestImportRuntimeRows_Errors(t *testing.T) {
	engine := newTestEngine(t)
	if err := engine.ImportRuntimeRows(context.Background(), []flowablerow.Row{{Table: "BAD"}}); err == nil {
		t.Fatalf("非法表应失败")
	}
	execution := runtimeExecutionRow(t, "e1", "i1", "missing")
	if err := engine.ImportRuntimeRows(context.Background(), []flowablerow.Row{execution}); err == nil {
		t.Fatalf("缺失定义应失败")
	}
	_ = engine.Close(context.Background())
	if err := engine.ImportRuntimeRows(context.Background(), nil); err == nil {
		t.Fatalf("关闭后导入应失败")
	}
}

// TestRuntimeVariableValue 验证变量类型转换。
// @param t 测试上下文
func TestRuntimeVariableValue(t *testing.T) {
	text := "x"
	long := int64(2)
	double := 1.5
	if value, _ := runtimeVariableValue(flowablerow.RuntimeVariable{Type: "string", Text: &text}); value != "x" {
		t.Fatalf("字符串转换错误")
	}
	if value, _ := runtimeVariableValue(flowablerow.RuntimeVariable{Type: "boolean", Long: &long}); value != true {
		t.Fatalf("布尔转换错误")
	}
	if value, _ := runtimeVariableValue(flowablerow.RuntimeVariable{Type: "long", Long: &long}); value != int64(2) {
		t.Fatalf("整数转换错误")
	}
	if value, _ := runtimeVariableValue(flowablerow.RuntimeVariable{Type: "double", Double: &double}); value != 1.5 {
		t.Fatalf("浮点转换错误")
	}
	if value, _ := runtimeVariableValue(flowablerow.RuntimeVariable{Type: "null"}); value != nil {
		t.Fatalf("空值转换错误")
	}
	jsonText := `{"a":1}`
	if value, _ := runtimeVariableValue(flowablerow.RuntimeVariable{Type: "json", Text: &jsonText}); value == nil {
		t.Fatalf("JSON 转换错误")
	}
	bad := "{"
	if _, err := runtimeVariableValue(flowablerow.RuntimeVariable{Type: "json", Text: &bad}); err == nil {
		t.Fatalf("非法 JSON 应失败")
	}
	if value, _ := runtimeVariableValue(flowablerow.RuntimeVariable{Type: "unknown"}); value != nil {
		t.Fatalf("未知类型应返回 nil")
	}
}

// runtimeExecutionRow 构造执行行。
// @param t 测试上下文
// @param executionID 执行 ID
// @param instanceID 实例 ID
// @param definitionID 定义 ID
// @return 行
func runtimeExecutionRow(t *testing.T, executionID, instanceID, definitionID string) flowablerow.Row {
	t.Helper()
	row, _ := flowablerow.NewRow("ACT_RU_EXECUTION")
	_ = row.SetInsertColumns([]string{"ID_", "PROC_INST_ID_", "PROC_DEF_ID_", "PARENT_ID_", "ACT_ID_", "BUSINESS_KEY_", "TENANT_ID_", "START_TIME_", "IS_ACTIVE_", "IS_CONCURRENT_", "IS_SCOPE_", "IS_EVENT_SCOPE_", "IS_MI_ROOT_", "SUSPENSION_STATE_"})
	_ = row.Set("ID_", executionID)
	_ = row.Set("PROC_INST_ID_", instanceID)
	_ = row.Set("PROC_DEF_ID_", definitionID)
	_ = row.Set("PARENT_ID_", "")
	_ = row.Set("ACT_ID_", "task")
	_ = row.Set("BUSINESS_KEY_", "")
	_ = row.Set("TENANT_ID_", "")
	_ = row.Set("START_TIME_", time.Now())
	_ = row.Set("IS_ACTIVE_", true)
	_ = row.Set("IS_CONCURRENT_", false)
	_ = row.Set("IS_SCOPE_", true)
	_ = row.Set("IS_EVENT_SCOPE_", false)
	_ = row.Set("IS_MI_ROOT_", false)
	_ = row.Set("SUSPENSION_STATE_", 1)
	return *row
}
