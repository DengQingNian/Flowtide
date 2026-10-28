package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/tenant"
)

// TestExecutionVariableOperations 验证执行变量读写与清空。
// @param t 测试上下文
func TestExecutionVariableOperations(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	ctx := context.Background()
	instance, err := engine.StartProcessInstance(ctx, api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	executions, err := engine.ListExecutions(ctx, instance.ID)
	if err != nil || len(executions) == 0 {
		t.Fatalf("执行查询失败: %v %+v", err, executions)
	}
	executionID := executions[0].ID
	if err := engine.SetExecutionVariable(ctx, executionID, "count", 3); err != nil {
		t.Fatalf("设置执行变量失败: %v", err)
	}
	value, err := engine.GetExecutionVariable(ctx, executionID, "count")
	if err != nil || value != 3 {
		t.Fatalf("执行变量查询失败: %v %v", err, value)
	}
	if _, err := engine.GetExecutionVariable(ctx, executionID, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失变量应 404: %v", err)
	}
	if err := engine.DeleteExecutionVariable(ctx, executionID, "count"); err != nil {
		t.Fatalf("删除执行变量失败: %v", err)
	}
	if err := engine.DeleteExecutionVariable(ctx, executionID, "count"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("重复删除应 404: %v", err)
	}
	if err := engine.SetExecutionVariable(ctx, executionID, "flag", true); err != nil {
		t.Fatalf("设置变量失败: %v", err)
	}
	if err := engine.DeleteAllExecutionVariables(ctx, executionID); err != nil {
		t.Fatalf("清空执行变量失败: %v", err)
	}
	values, err := engine.GetExecutionVariables(ctx, executionID)
	if err != nil || len(values) != 0 {
		t.Fatalf("清空后变量应为空: %v %+v", err, values)
	}
	if err := engine.DeleteAllInstanceVariables(ctx, instance.ID); err != nil {
		t.Fatalf("清空实例变量失败: %v", err)
	}
	instanceValues, err := engine.GetVariables(ctx, instance.ID)
	if err != nil || len(instanceValues) != 0 {
		t.Fatalf("清空后实例变量应为空: %v %+v", err, instanceValues)
	}
	if err := engine.DeleteAllExecutionVariables(ctx, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失执行应 404: %v", err)
	}
	if err := engine.DeleteAllInstanceVariables(ctx, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失实例应 404: %v", err)
	}
}

// TestExecutionVariableTenant 验证执行变量租户隔离。
// @param t 测试上下文
func TestExecutionVariableTenant(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	ctx := context.Background()
	instance, err := engine.StartProcessInstance(ctx, api.StartRequest{
		ProcessDefinitionKey: "simple",
		TenantID:             "t1",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	executions, _ := engine.ListExecutions(ctx, instance.ID)
	executionID := executions[0].ID
	foreign := tenant.With(ctx, "t2")
	if _, err := engine.GetExecutionVariable(foreign, executionID, "initiator"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户读取应 404: %v", err)
	}
	if err := engine.SetExecutionVariable(foreign, executionID, "x", 1); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户写入应 404: %v", err)
	}
	if err := engine.DeleteAllExecutionVariables(foreign, executionID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户清空应 404: %v", err)
	}
	if err := engine.DeleteAllInstanceVariables(foreign, instance.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户清空实例应 404: %v", err)
	}
}

// TestHistoricVariableLookups 验证历史变量与明细查询。
// @param t 测试上下文
func TestHistoricVariableLookups(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	ctx := context.Background()
	instance, err := engine.StartProcessInstance(ctx, api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	variable, err := engine.GetHistoricVariableByName(ctx, instance.ID, "initiator")
	if err != nil || variable.Value != "alice" {
		t.Fatalf("历史变量查询失败: %v %+v", err, variable)
	}
	if _, err := engine.GetHistoricVariableByName(ctx, instance.ID, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失历史变量应 404: %v", err)
	}
	if _, err := engine.GetHistoricVariableByName(ctx, "missing", "initiator"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失实例应 404: %v", err)
	}
	details, err := engine.ListHistoricDetails(ctx, instance.ID)
	if err != nil || len(details) == 0 {
		t.Fatalf("历史明细为空: %v %+v", err, details)
	}
	detail, err := engine.GetHistoricDetail(ctx, details[0].ID)
	if err != nil || detail.Name != details[0].Name {
		t.Fatalf("历史明细查询失败: %v %+v", err, detail)
	}
	if _, err := engine.GetHistoricDetail(ctx, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失明细应 404: %v", err)
	}

	task, err := engine.CreateTask(ctx, api.CreateTaskRequest{Name: "Task"})
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	if err := engine.SetTaskVariable(ctx, task.ID, "payload", "body"); err != nil {
		t.Fatalf("设置任务变量失败: %v", err)
	}
	taskVariable, err := engine.GetHistoricTaskVariableByName(ctx, task.ID, "payload")
	if err != nil || taskVariable.Value != "body" {
		t.Fatalf("历史任务变量查询失败: %v %+v", err, taskVariable)
	}
	if _, err := engine.GetHistoricTaskVariableByName(ctx, task.ID, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失历史任务变量应 404: %v", err)
	}
	if _, err := engine.GetHistoricTaskVariableByName(ctx, "missing", "payload"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失历史任务应 404: %v", err)
	}
	if err := engine.DeleteAllTaskVariables(ctx, task.ID); err != nil {
		t.Fatalf("清空任务变量失败: %v", err)
	}
	if values, err := engine.GetTaskVariables(ctx, task.ID); err != nil || len(values) != 0 {
		t.Fatalf("清空后任务变量应为空: %v %+v", err, values)
	}
	if err := engine.DeleteAllTaskVariables(ctx, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失任务清空应 404: %v", err)
	}
}
