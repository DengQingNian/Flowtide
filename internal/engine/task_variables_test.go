package engine

import (
	"context"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// TestTaskVariables 验证任务变量读写。
// @param t 测试上下文
func TestTaskVariables(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	task := listTasks(t, engine)[0]
	if err := engine.SetTaskVariable(context.Background(), task.ID, "local", "v"); err != nil {
		t.Fatalf("设置任务变量失败: %v", err)
	}
	values, err := engine.GetTaskVariables(context.Background(), task.ID)
	if err != nil || values["local"] != "v" || values["initiator"] != "alice" {
		t.Fatalf("任务变量错误: %v %+v", err, values)
	}
	if err := engine.DeleteTaskVariable(context.Background(), task.ID, "local"); err != nil {
		t.Fatalf("删除任务变量失败: %v", err)
	}
	values, _ = engine.GetTaskVariables(context.Background(), task.ID)
	if _, exists := values["local"]; exists {
		t.Fatalf("任务变量应删除")
	}
}

// TestTaskVariables_Errors 验证任务变量错误。
// @param t 测试上下文
func TestTaskVariables_Errors(t *testing.T) {
	engine := newTestEngine(t)
	if _, err := engine.GetTaskVariables(context.Background(), "missing"); err == nil {
		t.Fatalf("缺失任务变量应报错")
	}
	if err := engine.SetTaskVariable(context.Background(), "missing", "x", 1); err == nil {
		t.Fatalf("缺失任务设置应报错")
	}
	if err := engine.DeleteTaskVariable(context.Background(), "missing", "x"); err == nil {
		t.Fatalf("缺失任务删除应报错")
	}
	deployXML(t, engine, simpleProcess)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	task := listTasks(t, engine)[0]
	if err := engine.SetTaskVariable(context.Background(), task.ID, "", 1); err == nil {
		t.Fatalf("空变量名应报错")
	}
	_ = engine.Close(context.Background())
	if _, err := engine.GetTaskVariables(context.Background(), task.ID); err == nil {
		t.Fatalf("关闭后任务变量应报错")
	}
}

// TestGetSingleVariable 验证单个变量查询。
// @param t 测试上下文
func TestGetSingleVariable(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	value, err := engine.GetVariable(context.Background(), instance.ID, "initiator")
	if err != nil || value != "alice" {
		t.Fatalf("实例变量查询失败: %v %v", err, value)
	}
	if _, err := engine.GetVariable(context.Background(), instance.ID, "missing"); err == nil {
		t.Fatalf("缺失变量应失败")
	}
	if _, err := engine.GetVariable(context.Background(), "missing", "x"); err == nil {
		t.Fatalf("缺失实例应失败")
	}
	task := listTasks(t, engine)[0]
	if err := engine.SetTaskVariable(context.Background(), task.ID, "local", "v"); err != nil {
		t.Fatalf("设置任务变量失败: %v", err)
	}
	taskValue, err := engine.GetTaskVariable(context.Background(), task.ID, "local")
	if err != nil || taskValue != "v" {
		t.Fatalf("任务变量查询失败: %v %v", err, taskValue)
	}
	if taskValue, err := engine.GetTaskVariable(context.Background(), task.ID, "initiator"); err != nil || taskValue != "alice" {
		t.Fatalf("任务流程变量查询失败: %v %v", err, taskValue)
	}
	if _, err := engine.GetTaskVariable(context.Background(), task.ID, "missing"); err == nil {
		t.Fatalf("缺失任务变量应失败")
	}
	if _, err := engine.GetTaskVariable(context.Background(), "missing", "x"); err == nil {
		t.Fatalf("缺失任务应失败")
	}
}
