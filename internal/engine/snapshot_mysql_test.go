package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
	statestore "github.com/flowable-go/flowable/internal/persistence/state"
)

// TestSnapshotModelAndInstanceLinks 验证 SQL 所复用的快照完整保留模型及实例身份链接。
// @param t 测试上下文
// @return 无
func TestSnapshotModelAndInstanceLinks(t *testing.T) {
	ctx := context.Background()
	store := statestore.NewMemory()
	first, err := New(Config{StateStore: store})
	if err != nil {
		t.Fatal(err)
	}
	model, err := first.CreateModel(ctx, api.CreateModelRequest{Name: "开发模型", Key: "mysql-model"})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.SetModelSource(ctx, model.ID, []byte("中文模型内容")); err != nil {
		t.Fatal(err)
	}
	if err := first.SetModelSourceExtra(ctx, model.ID, []byte("扩展模型内容")); err != nil {
		t.Fatal(err)
	}
	deployXML(t, first, simpleProcess)
	instance, err := first.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "simple", UserID: "alice", Variables: []api.Variable{{Name: "initiator", Value: "alice"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(ctx); err != nil {
		t.Fatal(err)
	}
	// 使用新引擎读取同一快照，避免把旧内存对象误当作恢复成功。
	restored, err := New(Config{StateStore: store})
	if err != nil {
		t.Fatal(err)
	}
	if model, err := restored.GetModel(ctx, model.ID); err != nil || model.Name != "开发模型" {
		t.Fatalf("模型恢复失败: %v", err)
	}
	if source, err := restored.GetModelSource(ctx, model.ID); err != nil || string(source) != "中文模型内容" {
		t.Fatalf("模型内容恢复失败: %v", err)
	}
	if source, err := restored.GetModelSourceExtra(ctx, model.ID); err != nil || string(source) != "扩展模型内容" {
		t.Fatalf("模型扩展内容恢复失败: %v", err)
	}
	links, err := restored.ListInstanceIdentityLinks(ctx, instance.ID)
	if err != nil || len(links) != 1 || links[0].UserID != "alice" {
		t.Fatalf("身份链接恢复失败: %v", err)
	}
}
