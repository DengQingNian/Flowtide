package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
)

// TestImportRows 验证从 ACT 行恢复并启动流程。
// @param t 测试上下文
func TestImportRows(t *testing.T) {
	source := newTestEngine(t)
	deployXML(t, source, simpleProcess)
	rows, err := source.ExportRows(context.Background())
	if err != nil {
		t.Fatalf("导出行失败: %v", err)
	}
	target := newTestEngine(t)
	if err := target.ImportRows(context.Background(), rows); err != nil {
		t.Fatalf("导入行失败: %v", err)
	}
	definition, err := target.GetProcessDefinitionByKey(context.Background(), "simple")
	if err != nil || definition.ResourceName != "test.bpmn20.xml" {
		t.Fatalf("导入定义错误: %v %+v", err, definition)
	}
	instance, err := target.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil || instance.Ended {
		t.Fatalf("导入后启动失败: %v %+v", err, instance)
	}
	if len(listTasks(t, target)) != 1 {
		t.Fatalf("导入后应创建任务")
	}
	if err := target.ImportRows(context.Background(), rows); err != nil {
		t.Fatalf("重复导入失败: %v", err)
	}
}

// TestImportRows_Empty 验证空行导入。
// @param t 测试上下文
func TestImportRows_Empty(t *testing.T) {
	engine := newTestEngine(t)
	if err := engine.ImportRows(context.Background(), nil); err != nil {
		t.Fatalf("空行导入失败: %v", err)
	}
}

// TestImportRows_Errors 验证导入错误。
// @param t 测试上下文
func TestImportRows_Errors(t *testing.T) {
	engine := newTestEngine(t)
	if err := engine.ImportRows(context.Background(), []flowablerow.Row{{Table: "BAD"}}); err == nil {
		t.Fatalf("非法表应失败")
	}
	_ = engine.Close(context.Background())
	if err := engine.ImportRows(context.Background(), nil); err == nil {
		t.Fatalf("关闭后导入应失败")
	}
}
