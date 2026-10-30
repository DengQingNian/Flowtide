package engine

import (
	"context"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// TestBackUserTaskDeletesHistoricActivities 验证退回命令内联 G7 历史清理（P2-B 增补）：
// 退回后"目标及其后继节点"的历史活动行被删除（Java generalBack L2594-2604 对位），
// 目标节点保留新落位行，历史任务行与 Java 一致保留（不删 ACT_HI_TASKINST 等价存储），
// 退回后重走全流程历史与实例状态自洽。
// @param t 测试上下文
func TestBackUserTaskDeletesHistoricActivities(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, backLinearXML)
	instance, err := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backLinear"})
	if err != nil {
		t.Fatal(err)
	}
	// 记录 A 节点首轮任务（用于断言历史任务保留）
	taskAFirst := taskOfKey(t, eng, "A")
	completeByKey(t, eng, fake, "A", "alice")
	completeByKey(t, eng, fake, "B", "bob")
	// 记录 C 任务（退回时被迁移清场取消，应带 back 删除原因）
	taskC := taskOfKey(t, eng, "C")
	// 前置：A/B 各有 1 行历史活动
	if rows := historicActivityCount(t, eng, instance.ID, "A"); rows != 1 {
		t.Fatalf("退回前 A 应有 1 行历史活动，实际 %d", rows)
	}
	if rows := historicActivityCount(t, eng, instance.ID, "B"); rows != 1 {
		t.Fatalf("退回前 B 应有 1 行历史活动，实际 %d", rows)
	}
	// 从 C 退回到 A
	if landing, err := eng.BackUserTaskToActivity(context.Background(), taskOfKey(t, eng, "C").ID, "A"); err != nil || landing != "A" {
		t.Fatalf("退回失败: %v %s", err, landing)
	}
	// G7：后继 B/C 的历史活动行已删除
	if rows := historicActivityCount(t, eng, instance.ID, "B"); rows != 0 {
		t.Fatalf("退回后 B 的历史活动行应被删除，实际 %d", rows)
	}
	if rows := historicActivityCount(t, eng, instance.ID, "C"); rows != 0 {
		t.Fatalf("退回后 C 的历史活动行应被删除，实际 %d", rows)
	}
	// 目标 A：旧行已删、新落位行已建，恰 1 行
	if rows := historicActivityCount(t, eng, instance.ID, "A"); rows != 1 {
		t.Fatalf("退回后 A 应恰有 1 行历史活动（新落位），实际 %d", rows)
	}
	// 历史任务保留（Java generalBack 不删历史任务行）：
	// 首轮 A 任务为正常办结历史；被退回取消的 C 任务带 back 删除原因
	hisA, err := eng.GetHistoricTaskInstance(context.Background(), taskAFirst.ID)
	if err != nil {
		t.Fatalf("退回后首轮任务历史应保留: %v", err)
	}
	if hisA.EndTime == nil {
		t.Fatalf("首轮 A 任务历史应已办结: %+v", hisA)
	}
	hisC, err := eng.GetHistoricTaskInstance(context.Background(), taskC.ID)
	if err != nil {
		t.Fatalf("退回后 C 任务历史应保留: %v", err)
	}
	if hisC.EndTime == nil || hisC.DeleteReason != "back" {
		t.Fatalf("C 任务历史应带 back 删除原因: %+v", hisC)
	}
	// 退回后重走全流程：历史行随推进重建，实例正常结束
	completeByKey(t, eng, fake, "A", "alice")
	completeByKey(t, eng, fake, "B", "bob")
	completeByKey(t, eng, fake, "C", "carl")
	if rows := historicActivityCount(t, eng, instance.ID, "B"); rows != 1 {
		t.Fatalf("重走后 B 应有 1 行历史活动，实际 %d", rows)
	}
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("退回后重走应正常结束")
	}
}
