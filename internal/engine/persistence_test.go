package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// TestPersistence_RoundTrip 验证状态落盘与恢复。
// @param t 测试上下文
func TestPersistence_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "engine.json")
	first, err := New(Config{HistoryLevel: HistoryAudit, StatePath: path})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	deployXML(t, first, simpleProcess)
	instance, err := first.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if err := first.Close(context.Background()); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("状态文件未生成: %v", err)
	}

	second, err := New(Config{HistoryLevel: HistoryAudit, StatePath: path})
	if err != nil {
		t.Fatalf("恢复引擎失败: %v", err)
	}
	defer func() { _ = second.Close(context.Background()) }()
	definitions, err := second.ListProcessDefinitions(context.Background())
	if err != nil || len(definitions) != 1 {
		t.Fatalf("恢复定义失败: %v %d", err, len(definitions))
	}
	restored, err := second.GetProcessInstance(context.Background(), instance.ID)
	if err != nil || restored.Ended {
		t.Fatalf("恢复实例失败: %v %+v", err, restored)
	}
	tasks, err := second.ListTasks(context.Background(), api.TaskFilter{})
	if err != nil || len(tasks.Data) != 1 {
		t.Fatalf("恢复任务失败: %v %+v", err, tasks)
	}
	if err := second.Complete(context.Background(), tasks.Data[0].ID, api.CompleteTaskRequest{UserID: "alice"}); err != nil {
		t.Fatalf("恢复后完成任务失败: %v", err)
	}
	if err := second.Close(context.Background()); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}

	third, err := New(Config{HistoryLevel: HistoryAudit, StatePath: path})
	if err != nil {
		t.Fatalf("再次恢复失败: %v", err)
	}
	defer func() { _ = third.Close(context.Background()) }()
	ended, err := third.GetProcessInstance(context.Background(), instance.ID)
	if err != nil || !ended.Ended {
		t.Fatalf("完成状态未持久化: %v %+v", err, ended)
	}
	historic, err := third.ListHistoricProcessInstances(context.Background())
	if err != nil || len(historic) != 1 || !historic[0].Ended {
		t.Fatalf("历史未持久化: %v %+v", err, historic)
	}
}

// TestPersistence_Corrupt 验证损坏状态文件。
// @param t 测试上下文
func TestPersistence_Corrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{bad"), 0o600); err != nil {
		t.Fatalf("写入损坏文件失败: %v", err)
	}
	if _, err := New(Config{StatePath: path}); err == nil {
		t.Fatalf("损坏状态文件应报错")
	}
}

// TestPersistence_NoPath 验证内存模式。
// @param t 测试上下文
func TestPersistence_NoPath(t *testing.T) {
	app, err := New(Config{})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	deployXML(t, app, simpleProcess)
	if _, err := app.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	}); err != nil {
		t.Fatalf("内存模式启动失败: %v", err)
	}
}
