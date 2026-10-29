package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
	statestore "github.com/flowable-go/flowable/internal/persistence/state"
)

// TestMigrationSequenceFlowHistory 验证 Java 迁移续跑模式补记顺序流且普通实例保持原行为。
// @param t 测试上下文
// @return 无
func TestMigrationSequenceFlowHistory(t *testing.T) {
	store := statestore.NewMemory()
	source, err := New(Config{HistoryLevel: HistoryAudit, StateStore: store, RecordSequenceFlowHistory: true})
	if err != nil {
		t.Fatal(err)
	}
	deployXML(t, source, simpleProcess)
	instance, err := source.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "simple"})
	if err != nil {
		t.Fatal(err)
	}
	if count := sequenceFlowActivities(t, source, instance.ID); count != 1 {
		t.Fatalf("启动后顺序流历史数量错误：%d", count)
	}
	// 重启必须从状态恢复迁移标记，继续执行也不能退回普通历史模式。
	restored, err := New(Config{StateStore: store})
	if err != nil {
		t.Fatal(err)
	}
	if !restored.state.RecordSequenceFlowHistory {
		t.Fatal("迁移顺序流标记未随状态恢复")
	}
	tasks := listTasks(t, restored)
	if err := restored.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatal(err)
	}
	if count := sequenceFlowActivities(t, restored, instance.ID); count != 2 {
		t.Fatalf("完成后顺序流历史数量错误：%d", count)
	}
	plain := newTestEngine(t)
	deployXML(t, plain, simpleProcess)
	plainInstance, err := plain.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "simple"})
	if err != nil {
		t.Fatal(err)
	}
	// 原版语义：顺序流历史随普通执行记录，迁移标记仅兼容旧状态读取。
	if count := sequenceFlowActivities(t, plain, plainInstance.ID); count != 1 {
		t.Fatalf("普通实例应记录顺序流：%d", count)
	}
}

// TestMigrationSequenceFlowHistoryRows 验证迁移顺序流标记随行仓储恢复，不能只在 JSON 快照保留。
// @param t 测试上下文
// @return 无
func TestMigrationSequenceFlowHistoryRows(t *testing.T) {
	source, err := New(Config{HistoryLevel: HistoryAudit, RecordSequenceFlowHistory: true})
	if err != nil {
		t.Fatal(err)
	}
	deployXML(t, source, simpleProcess)
	instance, err := source.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "simple"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := source.SnapshotRows(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	target := newTestEngine(t)
	if err := target.RestoreRows(context.Background(), rows); err != nil {
		t.Fatal(err)
	}
	if !target.state.RecordSequenceFlowHistory {
		t.Fatal("行恢复丢失 Java 迁移顺序流标记")
	}
	task := listTasks(t, target)[0]
	if err := target.Complete(context.Background(), task.ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatal(err)
	}
	if count := sequenceFlowActivities(t, target, instance.ID); count != 2 {
		t.Fatalf("行恢复后顺序流历史数量=%d", count)
	}
}

// sequenceFlowActivities 统计实例中类型为 sequenceFlow 的历史活动。
// @param t 测试上下文
// @param workflow 引擎
// @param instanceID 流程实例 ID
// @return 数量
func sequenceFlowActivities(t *testing.T, workflow *Engine, instanceID string) int {
	t.Helper()
	activities, err := workflow.ListHistoricActivityInstances(context.Background(), instanceID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, activity := range activities {
		if activity.ActivityType == "sequenceFlow" {
			count++
		}
	}
	return count
}
