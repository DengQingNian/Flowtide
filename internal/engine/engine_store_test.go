package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
	statestore "github.com/flowable-go/flowable/internal/persistence/state"
)

// TestEngine_MemoryStore 验证可插拔状态存储恢复。
// @param t 测试上下文
func TestEngine_MemoryStore(t *testing.T) {
	store := statestore.NewMemory()
	engine, err := New(Config{StateStore: store})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	deployXML(t, engine, simpleProcess)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if err := engine.SetVariable(context.Background(), instance.ID, "amount", 10); err != nil {
		t.Fatalf("设置变量失败: %v", err)
	}
	restored, err := New(Config{StateStore: store})
	if err != nil {
		t.Fatalf("恢复引擎失败: %v", err)
	}
	tasks, _ := restored.ListTasks(context.Background(), api.TaskFilter{})
	if tasks.Total != 1 {
		t.Fatalf("恢复任务错误: %+v", tasks)
	}
	values, _ := restored.GetVariables(context.Background(), instance.ID)
	if values["amount"] != float64(10) {
		t.Fatalf("恢复变量错误: %+v", values)
	}
}

// TestEngine_StoreLoadError 验证存储读取错误。
// @param t 测试上下文
func TestEngine_StoreLoadError(t *testing.T) {
	if _, err := New(Config{StateStore: failingStore{}}); err == nil {
		t.Fatalf("存储读取错误应返回")
	}
}

// failingStore 是读取失败的存储。
type failingStore struct{}

// Load 返回错误。
// @param ctx 上下文
// @return 数据
// @return 错误
func (failingStore) Load(ctx context.Context) ([]byte, error) {
	return nil, context.Canceled
}

// Save 返回错误。
// @param ctx 上下文
// @param data 数据
// @return 错误
func (failingStore) Save(ctx context.Context, data []byte) error { return nil }
