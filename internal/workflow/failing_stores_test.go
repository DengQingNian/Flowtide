package workflow

import (
	"context"
	"errors"
	"testing"
)

// failingNextStore 是带错误注入的下一步存储桩（覆盖服务层错误传播路径）。
type failingNextStore struct {
	NextActivityStore
	failLatestVersion bool
	failInsertDual    bool
	failMarkUsed      bool
}

// LatestVersion 按开关返回错误。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param nextActivityID 节点 ID
// @return 最大版本号
// @return 错误
func (f *failingNextStore) LatestVersion(ctx context.Context, processInstanceID, nextActivityID string) (int, error) {
	// 错误注入点
	if f.failLatestVersion {
		return 0, errors.New("注入错误")
	}
	return f.NextActivityStore.LatestVersion(ctx, processInstanceID, nextActivityID)
}

// InsertDual 按开关返回错误。
// @param ctx 操作上下文
// @param act 下一步记录
// @return 错误
func (f *failingNextStore) InsertDual(ctx context.Context, act *NextActivity) error {
	// 错误注入点
	if f.failInsertDual {
		return errors.New("注入错误")
	}
	return f.NextActivityStore.InsertDual(ctx, act)
}

// MarkUsedAndWriteBack 按开关返回错误。
// @param ctx 操作上下文
// @param ids 记录 Id 集合
// @param taskID 任务 Id
// @return 错误
func (f *failingNextStore) MarkUsedAndWriteBack(ctx context.Context, ids []string, taskID string) error {
	// 错误注入点
	if f.failMarkUsed {
		return errors.New("注入错误")
	}
	return f.NextActivityStore.MarkUsedAndWriteBack(ctx, ids, taskID)
}

// failingMessageStore 是带错误注入的待办扩展存储桩。
type failingMessageStore struct {
	MessageExtendStore
	failGet    bool
	failSave   bool
	failList   bool
}

// GetByTaskID 按开关返回错误。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 待办扩展
// @return 是否存在
// @return 错误
func (f *failingMessageStore) GetByTaskID(ctx context.Context, taskID string) (*MessageExtend, bool, error) {
	// 错误注入点
	if f.failGet {
		return nil, false, errors.New("注入错误")
	}
	return f.MessageExtendStore.GetByTaskID(ctx, taskID)
}

// Save 按开关返回错误。
// @param ctx 操作上下文
// @param msg 待办扩展
// @return 错误
func (f *failingMessageStore) Save(ctx context.Context, msg *MessageExtend) error {
	// 错误注入点
	if f.failSave {
		return errors.New("注入错误")
	}
	return f.MessageExtendStore.Save(ctx, msg)
}

// ListByOwner 按开关返回错误。
// @param ctx 操作上下文
// @param ownerID 处理人 Id
// @param tenantID 租户
// @param page 分页参数
// @return 当前页
// @return 总数
// @return 错误
func (f *failingMessageStore) ListByOwner(ctx context.Context, ownerID, tenantID string, page PageQuery) ([]*MessageExtend, int64, error) {
	// 错误注入点
	if f.failList {
		return nil, 0, errors.New("注入错误")
	}
	return f.MessageExtendStore.ListByOwner(ctx, ownerID, tenantID, page)
}

// failingHistoryStore 是带错误注入的已办扩展存储桩。
type failingHistoryStore struct {
	HistoryExtendStore
	failListOwner bool
}

// ListByOwner 按开关返回错误。
// @param ctx 操作上下文
// @param ownerID 处理人 Id
// @param tenantID 租户
// @param page 分页参数
// @return 当前页
// @return 总数
// @return 错误
func (f *failingHistoryStore) ListByOwner(ctx context.Context, ownerID, tenantID string, page PageQuery) ([]*HistoryExtend, int64, error) {
	// 错误注入点
	if f.failListOwner {
		return nil, 0, errors.New("注入错误")
	}
	return f.HistoryExtendStore.ListByOwner(ctx, ownerID, tenantID, page)
}

// TestNextActivityErrorPaths 验证下一步服务错误传播。
// @param t 测试上下文
func TestNextActivityErrorPaths(t *testing.T) {
	inner := NewMemNextStore(nil)
	failing := &failingNextStore{NextActivityStore: inner, failLatestVersion: true}
	service := NewNextActivityService(failing, nil)
	ctx := context.Background()
	// InsertDual 版本推算错误
	if err := service.InsertDual(ctx, &NextActivity{ProcessInstanceID: "pi", NextActivityID: "a", IdentityID: "u"}, "op"); err == nil {
		t.Fatal("版本推算错误应透传")
	}
	// SaveFirstTaskUser 版本推算错误
	if err := service.SaveFirstTaskUser(ctx, FirstStepRequest{ProcessInstanceID: "pi", ActivityID: "a", IdentityIDs: []string{"u"}, TenantID: "sys"}); err == nil {
		t.Fatal("版本推算错误应透传")
	}
	// SaveAgreeNext / SaveBackToStepNext / SaveResolveNext 版本推算错误
	if err := service.SaveAgreeNext(ctx, NextActivitySaveRequest{ProcessInstanceID: "pi", ActivityID: "a", IdentityIDs: []string{"u"}, TenantID: "sys"}); err == nil {
		t.Fatal("版本推算错误应透传")
	}
	if err := service.SaveBackToStepNext(ctx, NextActivitySaveRequest{ProcessInstanceID: "pi", ActivityID: "a", IdentityIDs: []string{"u"}, TenantID: "sys"}); err == nil {
		t.Fatal("版本推算错误应透传")
	}
	// SaveResolveNext：先造来源记录（找到来源后才走到版本推算）
	resolveSource := &NextActivity{ProcessInstanceID: "pi", NextActivityID: "a", IdentityID: "x", SendIdentityID: "s", TaskID: "t", Used: 1, Version: 1, TenantID: "sys"}
	if err := inner.InsertDual(ctx, resolveSource); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveResolveNext(ctx, ResolveNextSaveRequest{ProcessInstanceID: "pi", ActivityID: "a", IdentityIDs: []string{"u"}, TaskID: "t", SendIdentityID: "s"}); err == nil {
		t.Fatal("版本推算错误应透传")
	}
	// InsertDual 错误（各保存入口）
	failing.failLatestVersion = false
	failing.failInsertDual = true
	if err := service.InsertDual(ctx, &NextActivity{ProcessInstanceID: "pi", NextActivityID: "a", IdentityID: "u"}, "op"); err == nil {
		t.Fatal("双写错误应透传")
	}
	if err := service.SaveFirstTaskUser(ctx, FirstStepRequest{ProcessInstanceID: "pi", ActivityID: "a", IdentityIDs: []string{"u"}, TenantID: "sys"}); err == nil {
		t.Fatal("双写错误应透传")
	}
	if err := service.SaveAgreeNext(ctx, NextActivitySaveRequest{ProcessInstanceID: "pi", ActivityID: "a", IdentityIDs: []string{"u"}, TenantID: "sys"}); err == nil {
		t.Fatal("双写错误应透传")
	}
	// SaveResolveNext：来源存在但双写失败
	source := &NextActivity{ProcessInstanceID: "pi", NextActivityID: "a", IdentityID: "x", SendIdentityID: "s", TaskID: "t", Used: 1, Version: 1, TenantID: "sys"}
	if err := inner.InsertDual(ctx, source); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveResolveNext(ctx, ResolveNextSaveRequest{ProcessInstanceID: "pi", ActivityID: "a", IdentityIDs: []string{"u"}, TaskID: "t", SendIdentityID: "s"}); err == nil {
		t.Fatal("双写错误应透传")
	}
	// MarkUsedAndWriteBack 错误
	failing.failInsertDual = false
	failing.failMarkUsed = true
	if err := service.MarkUsedAndWriteBack(ctx, []string{"x"}, "t1"); err == nil {
		t.Fatal("消耗错误应透传")
	}
}

// TestMessageErrorPaths 验证待办服务错误传播。
// @param t 测试上下文
func TestMessageErrorPaths(t *testing.T) {
	inner := NewMemMessageStore(nil)
	failing := &failingMessageStore{MessageExtendStore: inner, failGet: true}
	service := NewMessageExtendService(failing, nil)
	ctx := context.Background()
	// CreateForTask 幂等查询错误
	if _, err := service.CreateForTask(ctx, sampleMessage("t1", "u1")); err == nil {
		t.Fatal("幂等查询错误应透传")
	}
	// Save 错误
	failing.failGet = false
	failing.failSave = true
	if _, err := service.CreateForTask(ctx, sampleMessage("t1", "u1")); err == nil {
		t.Fatal("落库错误应透传")
	}
	// ListByOwner 错误
	failing.failSave = false
	failing.failList = true
	if _, err := service.ListByOwner(ctx, "u1", "sys", PageQuery{}); err == nil {
		t.Fatal("分页查询错误应透传")
	}
}

// TestHistoryErrorPaths 验证已办服务错误传播。
// @param t 测试上下文
func TestHistoryErrorPaths(t *testing.T) {
	failing := &failingHistoryStore{HistoryExtendStore: NewMemHistoryStore(nil), failListOwner: true}
	service := NewHistoryExtendService(failing, nil)
	ctx := context.Background()
	// ListByOwner 错误
	if _, err := service.ListByOwner(ctx, "u1", "sys", PageQuery{}); err == nil {
		t.Fatal("分页查询错误应透传")
	}
}
