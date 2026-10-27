package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
)

// TestTaskLogs 验证任务生命周期日志。
// @param t 测试上下文
func TestTaskLogs(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, identityLinkTaskXML)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "identityLinkTask"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	task := listTasks(t, engine)[0]
	if err := engine.Claim(context.Background(), task.ID, "alice"); err != nil {
		t.Fatalf("签收失败: %v", err)
	}
	if err := engine.Delegate(context.Background(), task.ID, "bob"); err != nil {
		t.Fatalf("委派失败: %v", err)
	}
	if err := engine.Complete(context.Background(), task.ID, api.CompleteTaskRequest{UserID: "bob"}); err != nil {
		t.Fatalf("完成失败: %v", err)
	}
	logs, err := engine.ListHistoricTaskLogs(context.Background(), task.ID)
	if err != nil || len(logs) != 4 {
		t.Fatalf("任务日志数量错误: %v %d", err, len(logs))
	}
	types := make(map[string]bool)
	for _, log := range logs {
		types[log.Type] = true
		if log.TaskID != task.ID {
			t.Fatalf("任务日志内容错误: %+v", log)
		}
	}
	for _, expected := range []string{"create", "claim", "delegate", "complete"} {
		if !types[expected] {
			t.Fatalf("缺少任务日志类型 %s: %+v", expected, types)
		}
	}
}
