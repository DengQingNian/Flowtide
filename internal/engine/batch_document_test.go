package engine

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flowable-go/flowable/api"
)

// TestBatchDocumentsAndDelete 验证批次文档、分片文档与批次删除。
// @param t 测试上下文
func TestBatchDocumentsAndDelete(t *testing.T) {
	app := newTestEngine(t)
	deployXML(t, app, migrationV1)
	v1, _ := app.GetProcessDefinitionByKey(context.Background(), "migrate")
	deployXML(t, app, migrationV2)
	v2, _ := app.GetProcessDefinitionByKey(context.Background(), "migrate")
	instance, err := app.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionID: v1.ID})
	if err != nil {
		t.Fatalf("启动实例失败: %v", err)
	}
	batch, err := app.BatchMigrateProcessInstances(context.Background(), v2.ID,
		[]string{instance.ID}, map[string]string{"oldTask": "newTask"})
	if err != nil {
		t.Fatalf("批次迁移失败: %v", err)
	}
	if len(batch.Document) == 0 {
		t.Fatalf("批次文档未写入")
	}
	document, err := app.GetBatchDocument(context.Background(), batch.ID)
	if err != nil || !strings.Contains(string(document), batch.ID) || !strings.Contains(string(document), v2.ID) {
		t.Fatalf("批次文档错误: %v %s", err, document)
	}
	parts, err := app.ListBatchParts(context.Background(), batch.ID)
	if err != nil || len(parts) != 1 {
		t.Fatalf("分片列表错误: %v %+v", err, parts)
	}
	if len(parts[0].Document) == 0 {
		t.Fatalf("分片文档未写入")
	}
	partDocument, err := app.GetBatchPartDocument(context.Background(), parts[0].ID)
	if err != nil || !strings.Contains(string(partDocument), parts[0].ID) {
		t.Fatalf("分片文档错误: %v %s", err, partDocument)
	}
	// 模拟旧快照：文档缺失时按 Flowable 语义返回 404
	app.state.Batches[batch.ID].Info.Document = nil
	if _, err := app.GetBatchDocument(context.Background(), batch.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("无批次文档应 404: %v", err)
	}
	app.state.Batches[batch.ID].Parts[parts[0].ID].Document = nil
	if _, err := app.GetBatchPartDocument(context.Background(), parts[0].ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("无分片文档应 404: %v", err)
	}
	if err := app.DeleteBatch(context.Background(), batch.ID); err != nil {
		t.Fatalf("删除批次失败: %v", err)
	}
	if _, err := app.GetBatch(context.Background(), batch.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("删除后批次应 404: %v", err)
	}
	if _, err := app.ListBatchParts(context.Background(), batch.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("删除后分片应 404: %v", err)
	}
	if _, err := app.GetBatchDocument(context.Background(), batch.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("删除后批次文档应 404: %v", err)
	}
	if _, err := app.GetBatchPartDocument(context.Background(), parts[0].ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("删除后分片文档应 404: %v", err)
	}
	if err := app.DeleteBatch(context.Background(), batch.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("重复删除批次应 404: %v", err)
	}
	if _, err := app.GetBatchDocument(context.Background(), "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失批次文档应 404: %v", err)
	}
	if _, err := app.GetBatchPartDocument(context.Background(), "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失分片文档应 404: %v", err)
	}
}

// TestBatchDocumentCloneIsolation 验证读取批次和分片时文档字节不会被外部修改。
// @param t 测试上下文
func TestBatchDocumentCloneIsolation(t *testing.T) {
	app := newTestEngine(t)
	deployXML(t, app, migrationV1)
	v1, _ := app.GetProcessDefinitionByKey(context.Background(), "migrate")
	deployXML(t, app, migrationV2)
	v2, _ := app.GetProcessDefinitionByKey(context.Background(), "migrate")
	instance, err := app.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionID: v1.ID})
	if err != nil {
		t.Fatalf("启动实例失败: %v", err)
	}
	batch, err := app.BatchMigrateProcessInstances(context.Background(), v2.ID,
		[]string{instance.ID}, map[string]string{"oldTask": "newTask"})
	if err != nil {
		t.Fatalf("批次迁移失败: %v", err)
	}
	read, err := app.GetBatch(context.Background(), batch.ID)
	if err != nil {
		t.Fatalf("查询批次失败: %v", err)
	}
	// 修改返回值不应影响引擎内部快照
	read.Document[0] = 'x'
	document, err := app.GetBatchDocument(context.Background(), batch.ID)
	if err != nil {
		t.Fatalf("查询批次文档失败: %v", err)
	}
	if len(document) == 0 || document[0] == 'x' {
		t.Fatalf("批次文档被外部修改: %q", document)
	}
	parts, err := app.ListBatchParts(context.Background(), batch.ID)
	if err != nil || len(parts) != 1 {
		t.Fatalf("分片列表错误: %v %+v", err, parts)
	}
	parts[0].Document[0] = 'y'
	partDocument, err := app.GetBatchPartDocument(context.Background(), parts[0].ID)
	if err != nil {
		t.Fatalf("查询分片文档失败: %v", err)
	}
	if len(partDocument) == 0 || partDocument[0] == 'y' {
		t.Fatalf("分片文档被外部修改: %q", partDocument)
	}
}

// TestBatchDocumentsSnapshot 验证批次文档随快照恢复。
// @param t 测试上下文
func TestBatchDocumentsSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "engine.json")
	first, err := New(Config{HistoryLevel: HistoryAudit, StatePath: path})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	deployXML(t, first, migrationV1)
	v1, _ := first.GetProcessDefinitionByKey(context.Background(), "migrate")
	deployXML(t, first, migrationV2)
	v2, _ := first.GetProcessDefinitionByKey(context.Background(), "migrate")
	instance, err := first.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionID: v1.ID})
	if err != nil {
		t.Fatalf("启动实例失败: %v", err)
	}
	batch, err := first.BatchMigrateProcessInstances(context.Background(), v2.ID,
		[]string{instance.ID}, map[string]string{"oldTask": "newTask"})
	if err != nil {
		t.Fatalf("批次迁移失败: %v", err)
	}
	parts, _ := first.ListBatchParts(context.Background(), batch.ID)
	if len(parts) != 1 {
		t.Fatalf("分片数量错误: %+v", parts)
	}
	if err := first.Close(context.Background()); err != nil {
		t.Fatalf("关闭引擎失败: %v", err)
	}
	second, err := New(Config{HistoryLevel: HistoryAudit, StatePath: path})
	if err != nil {
		t.Fatalf("恢复引擎失败: %v", err)
	}
	defer func() { _ = second.Close(context.Background()) }()
	document, err := second.GetBatchDocument(context.Background(), batch.ID)
	if err != nil || !strings.Contains(string(document), batch.ID) {
		t.Fatalf("恢复批次文档错误: %v %s", err, document)
	}
	partDocument, err := second.GetBatchPartDocument(context.Background(), parts[0].ID)
	if err != nil || !strings.Contains(string(partDocument), parts[0].ID) {
		t.Fatalf("恢复分片文档错误: %v %s", err, partDocument)
	}
}
