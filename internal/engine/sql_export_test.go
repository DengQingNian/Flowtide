package engine

import (
	"context"
	"testing"
)

// TestExportRows 验证 ACT 行导出。
// @param t 测试上下文
func TestExportRows(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	rows, err := engine.ExportRows(context.Background())
	if err != nil || len(rows) != 3 {
		t.Fatalf("导出行数量错误: %v %d", err, len(rows))
	}
	if rows[0].Table != "ACT_GE_BYTEARRAY" || rows[1].Table != "ACT_RE_DEPLOYMENT" || rows[2].Table != "ACT_RE_PROCDEF" {
		t.Fatalf("导出行顺序错误: %+v", rows)
	}
	byteRow := rows[0]
	if byteRow.Values["NAME_"] != "test.bpmn20.xml" || byteRow.Values["DEPLOYMENT_ID_"] == "" {
		t.Fatalf("字节数组行错误: %+v", byteRow.Values)
	}
	if _, ok := byteRow.Values["BYTES_"].([]byte); !ok {
		t.Fatalf("字节数组内容类型错误: %T", byteRow.Values["BYTES_"])
	}
	procRow := rows[2]
	if procRow.Values["KEY_"] != "simple" || procRow.Values["VERSION_"] != 1 || procRow.Values["SUSPENSION_STATE_"] != 1 {
		t.Fatalf("流程定义行错误: %+v", procRow.Values)
	}
	if _, _, err := procRow.InsertSQL("postgres"); err != nil {
		t.Fatalf("生成 SQL 失败: %v", err)
	}
}

// TestExportRows_Empty 验证空状态导出。
// @param t 测试上下文
func TestExportRows_Empty(t *testing.T) {
	engine := newTestEngine(t)
	rows, err := engine.ExportRows(context.Background())
	if err != nil || len(rows) != 0 {
		t.Fatalf("空状态导出错误: %v %d", err, len(rows))
	}
}

// TestExportRows_Suspended 验证挂起状态。
// @param t 测试上下文
func TestExportRows_Suspended(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	definition, _ := engine.GetProcessDefinitionByKey(context.Background(), "simple")
	if err := engine.SuspendProcessDefinition(context.Background(), definition.ID, false); err != nil {
		t.Fatalf("挂起失败: %v", err)
	}
	rows, _ := engine.ExportRows(context.Background())
	for _, row := range rows {
		if row.Table == "ACT_RE_PROCDEF" && row.Values["SUSPENSION_STATE_"] != 2 {
			t.Fatalf("挂起状态错误: %+v", row.Values)
		}
	}
}

// TestExportRows_Closed 验证关闭引擎导出。
// @param t 测试上下文
func TestExportRows_Closed(t *testing.T) {
	engine := newTestEngine(t)
	_ = engine.Close(context.Background())
	if _, err := engine.ExportRows(context.Background()); err == nil {
		t.Fatalf("关闭后导出应报错")
	}
}
