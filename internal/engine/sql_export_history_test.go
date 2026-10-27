package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
)

// TestExportHistoricRows 验证历史行导出。
// @param t 测试上下文
func TestExportHistoricRows(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	tasks := listTasks(t, engine)
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	rows, err := engine.ExportHistoricRows(context.Background())
	if err != nil {
		t.Fatalf("导出历史失败: %v", err)
	}
	counts := map[string]int{}
	for _, row := range rows {
		counts[row.Table]++
		if row.Table == "ACT_HI_PROCINST" && row.Values["PROC_INST_ID_"] != instance.ID {
			t.Fatalf("历史流程行错误: %+v", row.Values)
		}
		if row.Table == "ACT_HI_TASKINST" && row.Values["ASSIGNEE_"] != "alice" {
			t.Fatalf("历史任务行错误: %+v", row.Values)
		}
		if row.Table == "ACT_HI_VARINST" && row.Values["NAME_"] != "initiator" {
			t.Fatalf("历史变量行错误: %+v", row.Values)
		}
		if _, _, err := row.InsertSQL("postgres"); err != nil {
			t.Fatalf("历史行 SQL 失败: %v", err)
		}
	}
	if counts["ACT_HI_PROCINST"] != 1 || counts["ACT_HI_TASKINST"] != 1 || counts["ACT_HI_VARINST"] != 1 || counts["ACT_HI_ACTINST"] < 3 {
		t.Fatalf("历史行数错误: %+v", counts)
	}
}

// TestExportHistoricRows_Empty 验证空历史。
// @param t 测试上下文
func TestExportHistoricRows_Empty(t *testing.T) {
	engine := newTestEngine(t)
	rows, err := engine.ExportHistoricRows(context.Background())
	if err != nil || len(rows) != 0 {
		t.Fatalf("空历史导出错误: %v %d", err, len(rows))
	}
}

// TestExportHistoricRows_Closed 验证关闭引擎导出历史。
// @param t 测试上下文
func TestExportHistoricRows_Closed(t *testing.T) {
	engine := newTestEngine(t)
	_ = engine.Close(context.Background())
	if _, err := engine.ExportHistoricRows(context.Background()); err == nil {
		t.Fatalf("关闭后导出历史应报错")
	}
}
