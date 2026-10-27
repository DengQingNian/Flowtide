package flowable

import (
	"testing"
	"time"
)

// runtimeExecution 构造执行行。
// @param id 执行 ID
// @return 行
func runtimeExecution(t *testing.T, id string) Row {
	t.Helper()
	row, _ := NewRow("ACT_RU_EXECUTION")
	_ = row.Set("ID_", id)
	_ = row.Set("PROC_INST_ID_", "i1")
	_ = row.Set("PROC_DEF_ID_", "d1")
	_ = row.Set("PARENT_ID_", "")
	_ = row.Set("ACT_ID_", "task")
	_ = row.Set("BUSINESS_KEY_", "bk")
	_ = row.Set("TENANT_ID_", "t1")
	_ = row.Set("START_TIME_", time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	_ = row.Set("IS_ACTIVE_", true)
	_ = row.Set("IS_CONCURRENT_", false)
	_ = row.Set("IS_SCOPE_", true)
	_ = row.Set("IS_EVENT_SCOPE_", false)
	_ = row.Set("IS_MI_ROOT_", false)
	_ = row.Set("SUSPENSION_STATE_", 1)
	return *row
}

// runtimeTask 构造任务行。
// @param id 任务 ID
// @return 行
func runtimeTask(t *testing.T, id string) Row {
	t.Helper()
	row, _ := NewRow("ACT_RU_TASK")
	_ = row.Set("ID_", id)
	_ = row.Set("EXECUTION_ID_", "e1")
	_ = row.Set("PROC_INST_ID_", "i1")
	_ = row.Set("PROC_DEF_ID_", "d1")
	_ = row.Set("NAME_", "Task")
	_ = row.Set("DESCRIPTION_", "Desc")
	_ = row.Set("TASK_DEF_KEY_", "task")
	_ = row.Set("OWNER_", "owner")
	_ = row.Set("ASSIGNEE_", "alice")
	_ = row.Set("CATEGORY_", "cat")
	_ = row.Set("FORM_KEY_", "form")
	_ = row.Set("TENANT_ID_", "t1")
	_ = row.Set("PRIORITY_", 50)
	_ = row.Set("CREATE_TIME_", time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	_ = row.Set("DUE_DATE_", time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC))
	_ = row.Set("SUSPENSION_STATE_", 1)
	return *row
}

// runtimeVariable 构造变量行。
// @param name 变量名
// @param kind 类型
// @return 行
func runtimeVariable(t *testing.T, name, kind string) Row {
	t.Helper()
	row, _ := NewRow("ACT_RU_VARIABLE")
	_ = row.Set("ID_", "v-"+name)
	_ = row.Set("TYPE_", kind)
	_ = row.Set("NAME_", name)
	_ = row.Set("EXECUTION_ID_", "e1")
	_ = row.Set("PROC_INST_ID_", "i1")
	_ = row.Set("TASK_ID_", "")
	_ = row.Set("LONG_", int64(1))
	_ = row.Set("DOUBLE_", 1.5)
	_ = row.Set("TEXT_", "text")
	return *row
}

// TestDecodeRuntimeRows 验证运行时行恢复。
// @param t 测试上下文
func TestDecodeRuntimeRows(t *testing.T) {
	rows := []Row{runtimeExecution(t, "e1"), runtimeTask(t, "t1"), runtimeVariable(t, "v1", "string")}
	snapshot, err := DecodeRuntimeRows(rows)
	if err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if len(snapshot.Executions) != 1 || snapshot.Executions[0].BusinessKey != "bk" {
		t.Fatalf("执行恢复错误: %+v", snapshot.Executions)
	}
	if len(snapshot.Tasks) != 1 || snapshot.Tasks[0].Assignee != "alice" || snapshot.Tasks[0].DueDate == nil {
		t.Fatalf("任务恢复错误: %+v", snapshot.Tasks)
	}
	if len(snapshot.Variables) != 1 || snapshot.Variables[0].Text == nil || *snapshot.Variables[0].Text != "text" {
		t.Fatalf("变量恢复错误: %+v", snapshot.Variables)
	}
}

// TestDecodeRuntimeRows_Errors 验证运行时行错误。
// @param t 测试上下文
func TestDecodeRuntimeRows_Errors(t *testing.T) {
	mutations := []func() Row{
		func() Row { return Row{Table: "BAD"} },
		func() Row { row := runtimeExecution(t, "e1"); delete(row.Values, "ID_"); return row },
		func() Row { row := runtimeExecution(t, "e1"); row.Values["IS_ACTIVE_"] = "x"; return row },
		func() Row { row := runtimeExecution(t, "e1"); row.Values["SUSPENSION_STATE_"] = "x"; return row },
		func() Row { row := runtimeExecution(t, "e1"); row.Values["START_TIME_"] = "bad"; return row },
		func() Row { row := runtimeTask(t, "t1"); delete(row.Values, "ID_"); return row },
		func() Row { row := runtimeTask(t, "t1"); row.Values["PRIORITY_"] = "x"; return row },
		func() Row { row := runtimeTask(t, "t1"); row.Values["DUE_DATE_"] = 1; return row },
		func() Row { row := runtimeVariable(t, "v1", "string"); delete(row.Values, "ID_"); return row },
		func() Row { row := runtimeVariable(t, "v1", "string"); row.Values["LONG_"] = "x"; return row },
		func() Row { row := runtimeVariable(t, "v1", "string"); row.Values["DOUBLE_"] = "x"; return row },
		func() Row { row := runtimeVariable(t, "v1", "string"); row.Values["TEXT_"] = 1; return row },
	}
	for _, mutate := range mutations {
		if _, err := DecodeRuntimeRows([]Row{mutate()}); err == nil {
			t.Fatalf("期望解析失败: %+v", mutate())
		}
	}
	row := runtimeVariable(t, "v1", "string")
	row.Values["DUE_DATE_"] = "2026-10-29T00:00:00Z"
	if _, err := optionalTime(row, "DUE_DATE_"); err != nil {
		t.Fatalf("时间字符串解析失败: %v", err)
	}
}

// TestRuntimeHelpers 验证运行时列辅助函数。
// @param t 测试上下文
func TestRuntimeHelpers(t *testing.T) {
	row := Row{Table: "ACT_RU_EXECUTION", Values: map[string]any{
		"int": 1, "int64": int64(2), "float": 3.0, "bool": true, "bad": "x",
	}}
	for _, value := range []any{1, int64(1), 1.0} {
		row.Values["v"] = value
		if ok, err := boolValue(row, "v"); err != nil || !ok {
			t.Fatalf("布尔转换错误: %v", err)
		}
	}
	if _, err := boolValue(row, "bad"); err == nil {
		t.Fatalf("非法布尔应失败")
	}
	if _, err := boolValue(row, "missing"); err == nil {
		t.Fatalf("缺失布尔应失败")
	}
	if value, err := optionalInt(row, "int"); err != nil || *value != 1 {
		t.Fatalf("optionalInt int 错误: %v", err)
	}
	if value, err := optionalInt(row, "float"); err != nil || *value != 3 {
		t.Fatalf("optionalInt float 错误: %v", err)
	}
	if _, err := optionalInt(row, "bad"); err == nil {
		t.Fatalf("optionalInt 非法应失败")
	}
	if value, err := optionalFloat(row, "int64"); err != nil || *value != 2 {
		t.Fatalf("optionalFloat int64 错误: %v", err)
	}
	if value, err := optionalFloat(row, "int"); err != nil || *value != 1 {
		t.Fatalf("optionalFloat int 错误: %v", err)
	}
	if _, err := optionalFloat(row, "bad"); err == nil {
		t.Fatalf("optionalFloat 非法应失败")
	}
}

// TestRuntimeDecodeMissingColumns 验证缺列错误。
// @param t 测试上下文
func TestRuntimeDecodeMissingColumns(t *testing.T) {
	executionColumns := []string{"ID_", "PROC_INST_ID_", "PROC_DEF_ID_", "PARENT_ID_", "ACT_ID_", "BUSINESS_KEY_", "TENANT_ID_", "START_TIME_", "IS_ACTIVE_", "IS_CONCURRENT_", "IS_SCOPE_", "IS_EVENT_SCOPE_", "IS_MI_ROOT_", "SUSPENSION_STATE_"}
	for _, column := range executionColumns {
		row := runtimeExecution(t, "e1")
		delete(row.Values, column)
		if _, err := decodeExecution(row); err == nil {
			t.Fatalf("执行缺列 %s 应失败", column)
		}
	}
	taskColumns := []string{"ID_", "EXECUTION_ID_", "PROC_INST_ID_", "PROC_DEF_ID_", "NAME_", "DESCRIPTION_", "TASK_DEF_KEY_", "OWNER_", "ASSIGNEE_", "CATEGORY_", "FORM_KEY_", "TENANT_ID_", "PRIORITY_", "CREATE_TIME_", "SUSPENSION_STATE_"}
	for _, column := range taskColumns {
		row := runtimeTask(t, "t1")
		delete(row.Values, column)
		if _, err := decodeTask(row); err == nil {
			t.Fatalf("任务缺列 %s 应失败", column)
		}
	}
	variableColumns := []string{"ID_", "TYPE_", "NAME_", "EXECUTION_ID_", "PROC_INST_ID_", "TASK_ID_"}
	for _, column := range variableColumns {
		row := runtimeVariable(t, "v1", "string")
		delete(row.Values, column)
		if _, err := decodeVariable(row); err == nil {
			t.Fatalf("变量缺列 %s 应失败", column)
		}
	}
}
