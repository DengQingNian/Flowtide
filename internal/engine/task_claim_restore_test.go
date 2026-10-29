package engine

import (
	"context"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
)

// TestTaskClaimMetadataRestore 验证签收时间不依赖历史级别，标准行恢复及对外复制均保持任务元数据。
// @param t 测试上下文
// @return 无
func TestTaskClaimMetadataRestore(t *testing.T) {
	ctx := context.Background()
	app, err := New(Config{HistoryLevel: HistoryNone})
	if err != nil {
		t.Fatal(err)
	}
	deployXML(t, app, simpleProcess)
	instance, err := app.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "simple"})
	if err != nil {
		t.Fatal(err)
	}
	page, err := app.ListTasks(ctx, api.TaskFilter{})
	if err != nil || len(page.Data) != 1 {
		t.Fatalf("读取任务: %v", err)
	}
	id := page.Data[0].ID
	if err := app.Claim(ctx, id, "alice"); err != nil {
		t.Fatal(err)
	}
	task, err := app.GetTask(ctx, id)
	if err != nil || task.ExecutionID == "" || task.ClaimTime == nil {
		t.Fatalf("签收元数据缺失: %+v %v", task, err)
	}
	want := *task.ClaimTime
	*task.ClaimTime = task.ClaimTime.Add(time.Hour)
	if err := app.SuspendProcessInstance(ctx, instance.ID); err != nil {
		t.Fatal(err)
	}
	rows, err := app.SnapshotRows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	restored := newTestEngine(t)
	if err := restored.RestoreRows(ctx, rows); err != nil {
		t.Fatal(err)
	}
	task, err = restored.GetTask(ctx, id)
	if err != nil || !task.Suspended || task.ClaimTime == nil || !task.ClaimTime.Equal(want) {
		t.Fatalf("标准行丢失任务元数据或暴露指针: %+v %v", task, err)
	}
}
