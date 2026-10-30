package engine

import (
	"context"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// TestComments 验证任务评论。
// @param t 测试上下文
func TestComments(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	task := listTasks(t, engine)[0]
	comment, err := engine.AddComment(context.Background(), task.ID, "alice", "已审核")
	if err != nil || comment.Message != "已审核" {
		t.Fatalf("添加评论失败: %v", err)
	}
	if _, err := engine.AddComment(context.Background(), task.ID, "bob", "第二条"); err != nil {
		t.Fatalf("添加第二条评论失败: %v", err)
	}
	comments, err := engine.ListComments(context.Background(), task.ID)
	if err != nil || len(comments) != 2 || comments[0].UserID != "alice" {
		t.Fatalf("评论列表错误: %v %+v", err, comments)
	}
	if all, err := engine.ListComments(context.Background(), ""); err != nil || len(all) != 2 {
		t.Fatalf("全部评论查询错误: %v %+v", err, all)
	}
	if err := engine.DeleteComment(context.Background(), comment.ID); err != nil {
		t.Fatalf("删除评论失败: %v", err)
	}
	if comments, _ := engine.ListComments(context.Background(), task.ID); len(comments) != 1 {
		t.Fatalf("应剩余 1 条评论")
	}
}

// TestAttachments 验证任务附件。
// @param t 测试上下文
func TestAttachments(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	task := listTasks(t, engine)[0]
	attachment, err := engine.AddAttachment(context.Background(), task.ID, api.Attachment{Name: "doc.pdf", URL: "http://example.com/doc"})
	if err != nil || attachment.Name != "doc.pdf" {
		t.Fatalf("添加附件失败: %v", err)
	}
	if _, err := engine.AddAttachment(context.Background(), task.ID, api.Attachment{Name: "doc2.pdf"}); err != nil {
		t.Fatalf("添加第二个附件失败: %v", err)
	}
	attachments, err := engine.ListAttachments(context.Background(), task.ID)
	if err != nil || len(attachments) != 2 {
		t.Fatalf("附件列表错误: %v %+v", err, attachments)
	}
	if all, err := engine.ListAttachments(context.Background(), ""); err != nil || len(all) != 2 {
		t.Fatalf("全部附件查询错误: %v %+v", err, all)
	}
}

// TestTaskExtra_Errors 验证评论附件错误。
// @param t 测试上下文
func TestTaskExtra_Errors(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	task := listTasks(t, engine)[0]
	if _, err := engine.AddComment(context.Background(), task.ID, "alice", ""); err == nil {
		t.Fatalf("空评论应报错")
	}
	if _, err := engine.AddComment(context.Background(), "missing", "alice", "x"); err == nil {
		t.Fatalf("缺失任务评论应报错")
	}
	if _, err := engine.AddAttachment(context.Background(), task.ID, api.Attachment{}); err == nil {
		t.Fatalf("空附件名应报错")
	}
	if _, err := engine.AddAttachment(context.Background(), "missing", api.Attachment{Name: "x"}); err == nil {
		t.Fatalf("缺失任务附件应报错")
	}
	if err := engine.DeleteComment(context.Background(), "missing"); err == nil {
		t.Fatalf("删除缺失评论应报错")
	}
}

// TestTaskExtra_Closed 验证关闭引擎的评论附件错误。
// @param t 测试上下文
func TestTaskExtra_Closed(t *testing.T) {
	engine := newTestEngine(t)
	_ = engine.Close(context.Background())
	if _, err := engine.AddComment(context.Background(), "t", "u", "m"); err == nil {
		t.Fatalf("关闭后添加评论应报错")
	}
	if _, err := engine.ListComments(context.Background(), "t"); err == nil {
		t.Fatalf("关闭后评论列表应报错")
	}
	if err := engine.DeleteComment(context.Background(), "c"); err == nil {
		t.Fatalf("关闭后删除评论应报错")
	}
	if _, err := engine.AddAttachment(context.Background(), "t", api.Attachment{Name: "x"}); err == nil {
		t.Fatalf("关闭后添加附件应报错")
	}
	if _, err := engine.ListAttachments(context.Background(), "t"); err == nil {
		t.Fatalf("关闭后附件列表应报错")
	}
}

// TestProcessComments 验证流程实例评论。
// @param t 测试上下文
func TestProcessComments(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	comment, err := engine.AddProcessComment(context.Background(), instance.ID, "alice", "proc")
	if err != nil || comment.ProcessInstanceID != instance.ID {
		t.Fatalf("添加流程评论失败: %v", err)
	}
	task := listTasks(t, engine)[0]
	if _, err := engine.AddComment(context.Background(), task.ID, "alice", "task"); err != nil {
		t.Fatalf("添加任务评论失败: %v", err)
	}
	comments, err := engine.ListProcessComments(context.Background(), instance.ID)
	if err != nil || len(comments) != 1 || comments[0].Message != "proc" {
		t.Fatalf("流程评论列表错误: %v %+v", err, comments)
	}
	if _, err := engine.AddProcessComment(context.Background(), "missing", "u", "m"); err == nil {
		t.Fatalf("缺失实例评论应失败")
	}
	if _, err := engine.AddProcessComment(context.Background(), instance.ID, "u", ""); err == nil {
		t.Fatalf("空评论应失败")
	}
}

// TestProcessAttachments 验证流程实例附件。
// @param t 测试上下文
func TestProcessAttachments(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	attachment, err := engine.AddProcessAttachment(context.Background(), instance.ID, api.Attachment{Name: "proc"})
	if err != nil || attachment.ProcessInstanceID != instance.ID {
		t.Fatalf("添加流程附件失败: %v", err)
	}
	task := listTasks(t, engine)[0]
	if _, err := engine.AddAttachment(context.Background(), task.ID, api.Attachment{Name: "task"}); err != nil {
		t.Fatalf("添加任务附件失败: %v", err)
	}
	attachments, err := engine.ListProcessAttachments(context.Background(), instance.ID)
	if err != nil || len(attachments) != 1 || attachments[0].Name != "proc" {
		t.Fatalf("流程附件列表错误: %v %+v", err, attachments)
	}
	if _, err := engine.AddProcessAttachment(context.Background(), "missing", api.Attachment{Name: "x"}); err == nil {
		t.Fatalf("缺失实例附件应失败")
	}
	if _, err := engine.AddProcessAttachment(context.Background(), instance.ID, api.Attachment{}); err == nil {
		t.Fatalf("空附件名应失败")
	}
}

// TestDeleteAttachment 验证删除附件。
// @param t 测试上下文
func TestDeleteAttachment(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	attachment, err := engine.AddProcessAttachment(context.Background(), instance.ID, api.Attachment{Name: "x"})
	if err != nil {
		t.Fatalf("添加附件失败: %v", err)
	}
	if err := engine.DeleteAttachment(context.Background(), attachment.ID); err != nil {
		t.Fatalf("删除附件失败: %v", err)
	}
	if attachments, _ := engine.ListProcessAttachments(context.Background(), instance.ID); len(attachments) != 0 {
		t.Fatalf("附件应删除")
	}
	if err := engine.DeleteAttachment(context.Background(), "missing"); err == nil {
		t.Fatalf("缺失附件删除应失败")
	}
}
