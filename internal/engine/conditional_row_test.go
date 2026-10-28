package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
)

// TestConditionalSubscriptionRowRoundTrip 验证条件订阅行导出恢复。
// @param t 测试上下文
func TestConditionalSubscriptionRowRoundTrip(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, conditionalCatchXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "conditionalCatch",
		Variables:            []api.Variable{{Name: "ready", Value: false}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	rows, err := engine.SnapshotRows(context.Background())
	if err != nil {
		t.Fatalf("导出行失败: %v", err)
	}
	found := false
	for _, row := range rows {
		if row.Table == "ACT_RU_EVENT_SUBSCR" && row.Values["EVENT_TYPE_"] == "condition" {
			found = true
			if row.Values["CONFIGURATION_"] != "${ready == true}" {
				t.Fatalf("条件未写入 CONFIGURATION_: %+v", row.Values)
			}
		}
	}
	if !found {
		t.Fatalf("未导出条件订阅行")
	}
	restored := newTestEngine(t)
	if err := restored.RestoreRows(context.Background(), rows); err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if err := restored.SetVariable(context.Background(), instance.ID, "ready", true); err != nil {
		t.Fatalf("设置变量失败: %v", err)
	}
	got, _ := restored.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("恢复后条件应触发并结束")
	}
}
