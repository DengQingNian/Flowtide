package engine

import (
	"context"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// TestHistoricDetails 验证变量明细记录。
// @param t 测试上下文
func TestHistoricDetails(t *testing.T) {
	engine := newTestEngine(t)
	engine.cfg.HistoryLevel = HistoryFull
	deployXML(t, engine, simpleProcess)
	instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err := engine.SetVariable(context.Background(), instance.ID, "initiator", "bob"); err != nil {
		t.Fatalf("更新变量失败: %v", err)
	}
	task := listTasks(t, engine)[0]
	if err := engine.SetTaskVariable(context.Background(), task.ID, "local", "v"); err != nil {
		t.Fatalf("设置任务变量失败: %v", err)
	}
	details, err := engine.ListHistoricDetails(context.Background(), instance.ID)
	if err != nil || len(details) != 3 {
		t.Fatalf("明细数量错误: %v %d", err, len(details))
	}
	if details[0].Name != "initiator" || details[0].Value != "alice" {
		t.Fatalf("明细内容错误: %+v", details[0])
	}
	if details[2].TaskID != task.ID {
		t.Fatalf("任务明细错误: %+v", details[2])
	}
}
