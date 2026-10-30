package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// TestAttachmentContent 验证附件二进制内容。
// @param t 测试上下文
func TestAttachmentContent(t *testing.T) {
	engine := newTestEngine(t)
	ctx := context.Background()
	task, err := engine.CreateTask(ctx, api.CreateTaskRequest{Name: "Task"})
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	attachment, err := engine.AddAttachment(ctx, task.ID, api.Attachment{Name: "a.txt"})
	if err != nil {
		t.Fatalf("添加附件失败: %v", err)
	}
	if attachment.ContentURL != "" {
		t.Fatalf("无内容附件不应有 contentUrl: %+v", attachment)
	}
	if _, err := engine.GetAttachmentContent(ctx, attachment.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失内容应 404: %v", err)
	}
	if err := engine.SetAttachmentContent(ctx, attachment.ID, []byte("hello")); err != nil {
		t.Fatalf("设置附件内容失败: %v", err)
	}
	if err := engine.SetAttachmentContent(ctx, attachment.ID, nil); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("空内容应 400: %v", err)
	}
	loaded, err := engine.GetAttachment(ctx, attachment.ID)
	if err != nil || loaded.ContentURL == "" {
		t.Fatalf("附件 contentUrl 缺失: %v %+v", err, loaded)
	}
	data, err := engine.GetAttachmentContent(ctx, attachment.ID)
	if err != nil || string(data) != "hello" {
		t.Fatalf("附件内容错误: %v %s", err, data)
	}
	list, err := engine.ListAttachments(ctx, task.ID)
	if err != nil || len(list) != 1 || list[0].ContentURL == "" {
		t.Fatalf("附件列表 contentUrl 缺失: %v %+v", err, list)
	}
	if err := engine.SetAttachmentContent(ctx, "missing", []byte("x")); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失附件设置内容应 404: %v", err)
	}
	if _, err := engine.GetAttachmentContent(ctx, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失附件内容应 404: %v", err)
	}

	deployXML(t, engine, simpleProcess)
	instance, err := engine.StartProcessInstance(ctx, api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	processAttachment, err := engine.AddProcessAttachment(ctx, instance.ID, api.Attachment{Name: "p.txt"})
	if err != nil {
		t.Fatalf("添加流程附件失败: %v", err)
	}
	if err := engine.SetAttachmentContent(ctx, processAttachment.ID, []byte("process")); err != nil {
		t.Fatalf("设置流程附件内容失败: %v", err)
	}
	if data, err := engine.GetAttachmentContent(ctx, processAttachment.ID); err != nil || string(data) != "process" {
		t.Fatalf("流程附件内容错误: %v %s", err, data)
	}

	rows, err := engine.SnapshotRows(ctx)
	if err != nil {
		t.Fatalf("导出行失败: %v", err)
	}
	foundByte, foundContentID := false, false
	for _, row := range rows {
		if row.Table == "ACT_GE_BYTEARRAY" && row.Values["NAME_"] == "attachment/"+attachment.ID {
			foundByte = true
		}
		if row.Table == "ACT_HI_ATTACHMENT" && row.Values["ID_"] == attachment.ID && row.Values["CONTENT_ID_"] != "" {
			foundContentID = true
		}
	}
	if !foundByte || !foundContentID {
		t.Fatalf("附件内容行未导出: %v %v", foundByte, foundContentID)
	}
	restored := newTestEngine(t)
	if err := restored.RestoreRows(ctx, rows); err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if data, err := restored.GetAttachmentContent(ctx, attachment.ID); err != nil || string(data) != "hello" {
		t.Fatalf("恢复附件内容错误: %v %s", err, data)
	}
	cloned := cloneState(engine.state)
	if cloned.AttachmentContents[attachment.ID] == nil || string(cloned.AttachmentContents[attachment.ID].Data) != "hello" {
		t.Fatalf("克隆附件内容错误")
	}
	if engine.toSnapshot().AttachmentContents[attachment.ID] == nil {
		t.Fatalf("快照附件内容缺失")
	}
}
