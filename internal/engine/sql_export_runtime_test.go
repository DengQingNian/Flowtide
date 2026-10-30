package engine

import (
	"context"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
)

// TestExportRuntimeRows 验证运行时行导出。
// @param t 测试上下文
func TestExportRuntimeRows(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	task := listTasks(t, engine)[0]
	if err := engine.SetTaskVariable(context.Background(), task.ID, "local", 5); err != nil {
		t.Fatalf("设置任务变量失败: %v", err)
	}
	rows, err := engine.ExportRuntimeRows(context.Background())
	if err != nil {
		t.Fatalf("导出行失败: %v", err)
	}
	tableCount := map[string]int{}
	for _, row := range rows {
		tableCount[row.Table]++
		if row.Table == "ACT_RU_TASK" {
			if row.Values["ID_"] != task.ID || row.Values["NAME_"] != "Approve" || row.Values["ASSIGNEE_"] != "alice" {
				t.Fatalf("任务行错误: %+v", row.Values)
			}
		}
		if row.Table == "ACT_RU_EXECUTION" && row.Values["PROC_INST_ID_"] != instance.ID {
			t.Fatalf("执行行错误: %+v", row.Values)
		}
		if row.Table == "ACT_RU_VARIABLE" && row.Values["NAME_"] == "initiator" && row.Values["TEXT_"] != "alice" {
			t.Fatalf("变量行错误: %+v", row.Values)
		}
	}
	if tableCount["ACT_RU_EXECUTION"] == 0 || tableCount["ACT_RU_TASK"] != 1 || tableCount["ACT_RU_VARIABLE"] < 2 {
		t.Fatalf("表行数错误: %+v", tableCount)
	}
	for _, row := range rows {
		if _, _, err := row.InsertSQL("postgres"); err != nil {
			t.Fatalf("生成插入 SQL 失败: %v", err)
		}
	}
}

// TestExportRuntimeRows_Empty 验证无运行时行。
// @param t 测试上下文
func TestExportRuntimeRows_Empty(t *testing.T) {
	engine := newTestEngine(t)
	rows, err := engine.ExportRuntimeRows(context.Background())
	if err != nil || len(rows) != 0 {
		t.Fatalf("空运行时导出错误: %v %d", err, len(rows))
	}
}

// TestExportRuntimeRows_Closed 验证关闭引擎导出。
// @param t 测试上下文
func TestExportRuntimeRows_Closed(t *testing.T) {
	engine := newTestEngine(t)
	_ = engine.Close(context.Background())
	if _, err := engine.ExportRuntimeRows(context.Background()); err == nil {
		t.Fatalf("关闭后导出应报错")
	}
}

// TestVariableColumns 验证变量列映射。
// @param t 测试上下文
func TestVariableColumns(t *testing.T) {
	cases := []struct {
		value any
		kind  string
	}{
		{"x", "string"}, {true, "boolean"}, {int(1), "long"}, {int64(2), "long"},
		{1.5, "double"}, {nil, "null"}, {[]string{"a"}, "json"},
	}
	for _, tc := range cases {
		kind, _, _, _ := variableColumns(tc.value)
		if kind != tc.kind {
			t.Fatalf("%T 期望 %s，实际 %s", tc.value, tc.kind, kind)
		}
	}
	if variableID("e", "t", "n") == variableID("e", "t2", "n") {
		t.Fatalf("变量 ID 应区分任务")
	}
}

// TestExportRuntimeRows_Suspended 验证挂起状态与任务字段。
// @param t 测试上下文
func TestExportRuntimeRows_Suspended(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	task := listTasks(t, engine)[0]
	if err := engine.SetTaskVariable(context.Background(), task.ID, "flag", true); err != nil {
		t.Fatalf("设置变量失败: %v", err)
	}
	if err := engine.SetTaskVariable(context.Background(), task.ID, "score", 1.5); err != nil {
		t.Fatalf("设置变量失败: %v", err)
	}
	if err := engine.SetTaskVariable(context.Background(), task.ID, "tags", []string{"a"}); err != nil {
		t.Fatalf("设置变量失败: %v", err)
	}
	due := time.Now().Add(time.Hour)
	engine.state.Tasks[task.ID].Task.DueDate = &due
	if err := engine.SuspendProcessInstance(context.Background(), instance.ID); err != nil {
		t.Fatalf("挂起失败: %v", err)
	}
	rows, err := engine.ExportRuntimeRows(context.Background())
	if err != nil {
		t.Fatalf("导出失败: %v", err)
	}
	seenTypes := map[string]bool{}
	for _, row := range rows {
		switch row.Table {
		case "ACT_RU_TASK":
			if row.Values["SUSPENSION_STATE_"] != 2 || row.Values["DUE_DATE_"] == nil {
				t.Fatalf("任务挂起/到期字段错误: %+v", row.Values)
			}
		case "ACT_RU_EXECUTION":
			if row.Values["SUSPENSION_STATE_"] != 2 {
				t.Fatalf("执行挂起字段错误: %+v", row.Values)
			}
		case "ACT_RU_VARIABLE":
			seenTypes[row.Values["TYPE_"].(string)] = true
		}
	}
	if !seenTypes["boolean"] || !seenTypes["double"] || !seenTypes["json"] {
		t.Fatalf("变量类型未覆盖: %+v", seenTypes)
	}
}
