package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/tenant"
)

// TestEventSubscriptionQueries 验证事件订阅查询与租户隔离。
// @param t 测试上下文
func TestEventSubscriptionQueries(t *testing.T) {
	engine := newTestEngine(t)
	engine.cfg.TenantID = "t1"
	deployXML(t, engine, messageCatchXML)
	ctx := context.Background()
	instance, err := engine.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "msgCatch", TenantID: "t1"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	subscriptions, err := engine.ListEventSubscriptions(ctx, instance.ID)
	if err != nil || len(subscriptions) != 1 {
		t.Fatalf("订阅查询失败: %v %+v", err, subscriptions)
	}
	subscription := subscriptions[0]
	if subscription.EventType != "message" || subscription.EventName != "approvedMessage" ||
		subscription.ActivityID != "wait" || subscription.ProcessInstanceID != instance.ID || subscription.URL == "" {
		t.Fatalf("订阅字段错误: %+v", subscription)
	}
	loaded, err := engine.GetEventSubscription(ctx, subscription.ID)
	if err != nil || loaded.ID != subscription.ID {
		t.Fatalf("单条订阅查询失败: %v %+v", err, loaded)
	}
	if empty, err := engine.ListEventSubscriptions(ctx, "missing"); err != nil || len(empty) != 0 {
		t.Fatalf("不存在实例应返回空: %v %+v", err, empty)
	}
	if scoped, err := engine.ListEventSubscriptions(tenant.With(ctx, "t2"), ""); err != nil || len(scoped) != 0 {
		t.Fatalf("跨租户列表应过滤: %v %+v", err, scoped)
	}
	if scoped, err := engine.ListEventSubscriptions(tenant.With(ctx, "t1"), ""); err != nil || len(scoped) != 1 {
		t.Fatalf("同租户列表应可见: %v %+v", err, scoped)
	}
	if _, err := engine.GetEventSubscription(tenant.With(ctx, "t2"), subscription.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户查询应 404: %v", err)
	}
	if _, err := engine.GetEventSubscription(ctx, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("不存在订阅应 404: %v", err)
	}
}

// TestGetCommentAndAttachment 验证单条评论与附件查询。
// @param t 测试上下文
func TestGetCommentAndAttachment(t *testing.T) {
	engine := newTestEngine(t)
	engine.cfg.TenantID = "t1"
	ctx := context.Background()
	task, err := engine.CreateTask(ctx, api.CreateTaskRequest{Name: "Task", TenantID: "t1"})
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	comment, err := engine.AddComment(ctx, task.ID, "alice", "hello")
	if err != nil {
		t.Fatalf("添加评论失败: %v", err)
	}
	loaded, err := engine.GetComment(ctx, comment.ID)
	if err != nil || loaded.Message != "hello" || loaded.TaskID != task.ID {
		t.Fatalf("评论查询失败: %v %+v", err, loaded)
	}
	if _, err := engine.GetComment(tenant.With(ctx, "t2"), comment.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户评论应 404: %v", err)
	}
	if _, err := engine.GetComment(ctx, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("不存在评论应 404: %v", err)
	}

	attachment, err := engine.AddAttachment(ctx, task.ID, api.Attachment{Name: "a.txt", URL: "http://x"})
	if err != nil {
		t.Fatalf("添加附件失败: %v", err)
	}
	loadedAttachment, err := engine.GetAttachment(ctx, attachment.ID)
	if err != nil || loadedAttachment.Name != "a.txt" || loadedAttachment.TaskID != task.ID {
		t.Fatalf("附件查询失败: %v %+v", err, loadedAttachment)
	}
	if _, err := engine.GetAttachment(tenant.With(ctx, "t2"), attachment.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户附件应 404: %v", err)
	}
	if _, err := engine.GetAttachment(ctx, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("不存在附件应 404: %v", err)
	}

	deployXML(t, engine, simpleProcess)
	instance, err := engine.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "simple", Variables: []api.Variable{{Name: "initiator", Value: "alice"}}})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	processComment, err := engine.AddProcessComment(ctx, instance.ID, "alice", "proc")
	if err != nil {
		t.Fatalf("添加流程评论失败: %v", err)
	}
	if loaded, err := engine.GetComment(ctx, processComment.ID); err != nil || loaded.ProcessInstanceID != instance.ID {
		t.Fatalf("流程评论查询失败: %v %+v", err, loaded)
	}
	processAttachment, err := engine.AddProcessAttachment(ctx, instance.ID, api.Attachment{Name: "p.txt"})
	if err != nil {
		t.Fatalf("添加流程附件失败: %v", err)
	}
	if loaded, err := engine.GetAttachment(ctx, processAttachment.ID); err != nil || loaded.ProcessInstanceID != instance.ID {
		t.Fatalf("流程附件查询失败: %v %+v", err, loaded)
	}
}
