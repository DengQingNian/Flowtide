package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
	flowablerow "github.com/DengQingNian/Flowtide/internal/persistence/flowable"
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
	if _, err := runtimeVariableValue(flowablerow.RuntimeVariable{Type: "unknown"}); err == nil {
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

// TestImportRuntimeRows_Subscription 验证订阅恢复。
// @param t 测试上下文
func TestImportRuntimeRows_Subscription(t *testing.T) {
	source := newTestEngine(t)
	deployXML(t, source, receiveTaskXML)
	if _, err := source.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "receiveTask"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	repositoryRows, _ := source.ExportRows(context.Background())
	runtimeRows, _ := source.ExportRuntimeRows(context.Background())
	target := newTestEngine(t)
	_ = target.ImportRows(context.Background(), repositoryRows)
	if err := target.ImportRuntimeRows(context.Background(), runtimeRows); err != nil {
		t.Fatalf("导入订阅失败: %v", err)
	}
	triggered, err := target.MessageEventReceived(context.Background(), "receiveMessage", nil)
	if err != nil || triggered != 1 {
		t.Fatalf("恢复后消息触发失败: %v %d", err, triggered)
	}
	if len(listTasks(t, target)) != 1 {
		t.Fatalf("恢复后应创建任务")
	}
}

// TestImportRuntimeRows_Job 验证作业恢复。
// @param t 测试上下文
func TestImportRuntimeRows_Job(t *testing.T) {
	source := newTestEngine(t)
	source.RegisterDelegate("noop", func(ctx context.Context, execution *Execution) error { return nil })
	deployXML(t, source, asyncServiceXML("asyncJob", `flowable:asyncBefore="true"`))
	instance, err := source.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "asyncJob"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	repositoryRows, _ := source.ExportRows(context.Background())
	runtimeRows, _ := source.ExportRuntimeRows(context.Background())
	target := newTestEngine(t)
	target.RegisterDelegate("noop", func(ctx context.Context, execution *Execution) error { return nil })
	_ = target.ImportRows(context.Background(), repositoryRows)
	if err := target.ImportRuntimeRows(context.Background(), runtimeRows); err != nil {
		t.Fatalf("导入作业失败: %v", err)
	}
	if executed, err := target.RunDueJobs(context.Background()); err != nil || executed != 1 {
		t.Fatalf("恢复后作业执行失败: %v %d", err, executed)
	}
	got, _ := target.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("恢复作业后流程应结束")
	}
}

// TestImportRuntimeRows_DefinitionErrors 验证缺失定义/执行。
// @param t 测试上下文
func TestImportRuntimeRows_DefinitionErrors(t *testing.T) {
	source := newTestEngine(t)
	deployXML(t, source, receiveTaskXML)
	_, _ = source.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "receiveTask"})
	repositoryRows, _ := source.ExportRows(context.Background())
	runtimeRows, _ := source.ExportRuntimeRows(context.Background())
	target := newTestEngine(t)
	_ = target.ImportRows(context.Background(), repositoryRows)
	for id := range target.state.Definitions {
		delete(target.state.Definitions, id)
	}
	if err := target.ImportRuntimeRows(context.Background(), runtimeRows); err == nil {
		t.Fatalf("缺失定义应失败")
	}
	target2 := newTestEngine(t)
	_ = target2.ImportRows(context.Background(), repositoryRows)
	withoutExecution := make([]flowablerow.Row, 0)
	for _, row := range runtimeRows {
		if row.Table != "ACT_RU_EXECUTION" {
			withoutExecution = append(withoutExecution, row)
		}
	}
	if err := target2.ImportRuntimeRows(context.Background(), withoutExecution); err == nil {
		t.Fatalf("缺失执行应失败")
	}
}

// TestRuntimeVariableValue_Default 验证未知变量类型始终阻断。
// @param t 测试上下文
func TestRuntimeVariableValue_Default(t *testing.T) {
	text := "x"
	long := int64(1)
	double := 1.5
	if _, err := runtimeVariableValue(flowablerow.RuntimeVariable{Type: "unknown", Text: &text}); err == nil {
		t.Fatalf("未知类型文本回退错误")
	}
	if _, err := runtimeVariableValue(flowablerow.RuntimeVariable{Type: "unknown", Long: &long}); err == nil {
		t.Fatalf("未知类型整数回退错误")
	}
	if _, err := runtimeVariableValue(flowablerow.RuntimeVariable{Type: "unknown", Double: &double}); err == nil {
		t.Fatalf("未知类型浮点回退错误")
	}
	if _, err := runtimeVariableValue(flowablerow.RuntimeVariable{Type: "unknown"}); err == nil {
		t.Fatalf("未知类型空值回退错误")
	}
}

// TestImportRuntimeRows_JobDefinitionError 验证作业缺失定义。
// @param t 测试上下文
func TestImportRuntimeRows_JobDefinitionError(t *testing.T) {
	source := newTestEngine(t)
	source.RegisterDelegate("noop", func(ctx context.Context, execution *Execution) error { return nil })
	deployXML(t, source, asyncServiceXML("asyncMissing", `flowable:asyncBefore="true"`))
	_, _ = source.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "asyncMissing"})
	repositoryRows, _ := source.ExportRows(context.Background())
	runtimeRows, _ := source.ExportRuntimeRows(context.Background())
	target := newTestEngine(t)
	_ = target.ImportRows(context.Background(), repositoryRows)
	for id := range target.state.Definitions {
		delete(target.state.Definitions, id)
	}
	if err := target.ImportRuntimeRows(context.Background(), runtimeRows); err == nil {
		t.Fatalf("作业缺失定义应失败")
	}
}

// TestImportRuntimeRows_TaskExecutionError 验证任务缺失执行。
// @param t 测试上下文
func TestImportRuntimeRows_TaskExecutionError(t *testing.T) {
	source := newTestEngine(t)
	deployXML(t, source, simpleProcess)
	_, _ = source.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	repositoryRows, _ := source.ExportRows(context.Background())
	runtimeRows, _ := source.ExportRuntimeRows(context.Background())
	target := newTestEngine(t)
	_ = target.ImportRows(context.Background(), repositoryRows)
	withoutExecution := make([]flowablerow.Row, 0)
	for _, row := range runtimeRows {
		if row.Table != "ACT_RU_EXECUTION" {
			withoutExecution = append(withoutExecution, row)
		}
	}
	if err := target.ImportRuntimeRows(context.Background(), withoutExecution); err == nil {
		t.Fatalf("任务缺失执行应失败")
	}
}

// TestImportRuntimeRows_VariableExecutionError 验证变量缺失执行。
// @param t 测试上下文
func TestImportRuntimeRows_VariableExecutionError(t *testing.T) {
	source := newTestEngine(t)
	deployXML(t, source, simpleProcess)
	_, _ = source.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	repositoryRows, _ := source.ExportRows(context.Background())
	runtimeRows, _ := source.ExportRuntimeRows(context.Background())
	target := newTestEngine(t)
	_ = target.ImportRows(context.Background(), repositoryRows)
	onlyVariables := make([]flowablerow.Row, 0)
	for _, row := range runtimeRows {
		if row.Table == "ACT_RU_VARIABLE" {
			onlyVariables = append(onlyVariables, row)
		}
	}
	if err := target.ImportRuntimeRows(context.Background(), onlyVariables); err == nil {
		t.Fatalf("变量缺失执行应失败")
	}
}

// TestImportRuntimeRows_JobInstanceError 验证作业缺失实例。
// @param t 测试上下文
func TestImportRuntimeRows_JobInstanceError(t *testing.T) {
	source := newTestEngine(t)
	source.RegisterDelegate("noop", func(ctx context.Context, execution *Execution) error { return nil })
	deployXML(t, source, asyncServiceXML("asyncInstance", `flowable:asyncBefore="true"`))
	_, _ = source.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "asyncInstance"})
	repositoryRows, _ := source.ExportRows(context.Background())
	runtimeRows, _ := source.ExportRuntimeRows(context.Background())
	target := newTestEngine(t)
	_ = target.ImportRows(context.Background(), repositoryRows)
	withoutExecution := make([]flowablerow.Row, 0)
	for _, row := range runtimeRows {
		if row.Table != "ACT_RU_EXECUTION" {
			withoutExecution = append(withoutExecution, row)
		}
	}
	if err := target.ImportRuntimeRows(context.Background(), withoutExecution); err == nil {
		t.Fatalf("作业缺失实例应失败")
	}
}

// TestImportRuntimeRows_DeadLetter 验证死信恢复。
// @param t 测试上下文
func TestImportRuntimeRows_DeadLetter(t *testing.T) {
	fake := clock.NewFake(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	fail := true
	source, _ := New(Config{Clock: fake})
	source.RegisterDelegate("noop", func(ctx context.Context, execution *Execution) error {
		if fail {
			return errors.New("fail")
		}
		return nil
	})
	deployXML(t, source, asyncServiceXML("deadLetter", `flowable:asyncBefore="true"`))
	instance, _ := source.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "deadLetter"})
	for i := 0; i < 3; i++ {
		_, _ = source.RunDueJobs(context.Background())
		fake.Advance(10 * time.Second)
	}
	repositoryRows, _ := source.ExportRows(context.Background())
	runtimeRows, _ := source.ExportRuntimeRows(context.Background())
	count := 0
	for _, row := range runtimeRows {
		if row.Table == "ACT_RU_DEADLETTER_JOB" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("期望 1 条死信行，实际 %d", count)
	}
	target, _ := New(Config{})
	target.RegisterDelegate("noop", func(ctx context.Context, execution *Execution) error { return nil })
	_ = target.ImportRows(context.Background(), repositoryRows)
	if err := target.ImportRuntimeRows(context.Background(), runtimeRows); err != nil {
		t.Fatalf("导入死信失败: %v", err)
	}
	dead, _ := target.ListDeadLetterJobs(context.Background())
	if len(dead) != 1 {
		t.Fatalf("恢复死信错误: %+v", dead)
	}
	fail = false
	if err := target.MoveDeadLetterJobToExecutable(context.Background(), dead[0].ID, 1); err != nil {
		t.Fatalf("恢复死信失败: %v", err)
	}
	if executed, err := target.RunDueJobs(context.Background()); err != nil || executed != 1 {
		t.Fatalf("恢复后执行失败: %v %d", err, executed)
	}
	got, _ := target.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("死信恢复后流程应结束")
	}
}
