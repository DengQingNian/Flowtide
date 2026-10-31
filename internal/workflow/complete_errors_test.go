package workflow

import (
	"context"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/DengQingNian/Flowtide/api"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
)

// ─── 错误注入桩（仅注入本文件用例所需的端口方法） ─────────────────────

// ctaErrCommentTasks 是注入 AddComment 错误的任务评论能力桩。
type ctaErrCommentTasks struct {
	TaskCommentEngine // 内嵌真实引擎评论能力
	failAdd           bool
}

// AddComment 按开关返回错误。
// @param ctx 上下文
// @param taskID 任务 ID
// @param userID 作者
// @param message 内容
// @return 评论
// @return 错误
func (f *ctaErrCommentTasks) AddComment(ctx context.Context, taskID, userID, message string) (api.Comment, error) {
	// 错误注入点
	if f.failAdd {
		return api.Comment{}, context.DeadlineExceeded
	}
	return f.TaskCommentEngine.AddComment(ctx, taskID, userID, message)
}

// ctaErrNextStore 是注入 ListByTaskID/DeleteForResolve 错误的下一步存储桩。
type ctaErrNextStore struct {
	NextActivityStore
	failListByTask       bool
	failDeleteForResolve bool
}

// ListByTaskID 按开关返回错误。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 记录列表
// @return 错误
func (f *ctaErrNextStore) ListByTaskID(ctx context.Context, taskID string) ([]*NextActivity, error) {
	// 错误注入点
	if f.failListByTask {
		return nil, context.DeadlineExceeded
	}
	return f.NextActivityStore.ListByTaskID(ctx, taskID)
}

// DeleteForResolve 按开关返回错误。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param nextActivityID 节点 Id
// @param taskID 任务 Id
// @param identityID 执行人 Id
// @return 错误
func (f *ctaErrNextStore) DeleteForResolve(ctx context.Context, processInstanceID, nextActivityID, taskID, identityID string) error {
	// 错误注入点
	if f.failDeleteForResolve {
		return context.DeadlineExceeded
	}
	return f.NextActivityStore.DeleteForResolve(ctx, processInstanceID, nextActivityID, taskID, identityID)
}

// ctaErrMessageStore 是注入 DeleteByTaskID 错误的待办扩展存储桩。
type ctaErrMessageStore struct {
	MessageExtendStore
	failDelete bool
}

// DeleteByTaskID 按开关返回错误。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 错误
func (f *ctaErrMessageStore) DeleteByTaskID(ctx context.Context, taskID string) error {
	// 错误注入点
	if f.failDelete {
		return context.DeadlineExceeded
	}
	return f.MessageExtendStore.DeleteByTaskID(ctx, taskID)
}

// ctaErrHistoryStore 是注入 UpdateFinishTime/Save 错误的已办扩展存储桩。
type ctaErrHistoryStore struct {
	HistoryExtendStore
	failUpdateFinish bool
	failSave         bool
}

// UpdateFinishTime 按开关返回错误。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @param ownerID 处理人 Id
// @param finishTime 办结时间
// @return 错误
func (f *ctaErrHistoryStore) UpdateFinishTime(ctx context.Context, taskID, ownerID string, finishTime time.Time) error {
	// 错误注入点
	if f.failUpdateFinish {
		return context.DeadlineExceeded
	}
	return f.HistoryExtendStore.UpdateFinishTime(ctx, taskID, ownerID, finishTime)
}

// Save 按开关返回错误（已办镜像插入注入点）。
// @param ctx 操作上下文
// @param h 已办扩展
// @return 错误
func (f *ctaErrHistoryStore) Save(ctx context.Context, h *HistoryExtend) error {
	// 错误注入点
	if f.failSave {
		return context.DeadlineExceeded
	}
	return f.HistoryExtendStore.Save(ctx, h)
}

// ctaErrOperationStore 是注入 Save 错误的操作记录存储桩。
type ctaErrOperationStore struct {
	OperationRecordStore
	failSave bool
}

// Save 按开关返回错误。
// @param ctx 操作上下文
// @param r 操作记录
// @return 错误
func (f *ctaErrOperationStore) Save(ctx context.Context, r *OperationRecord) error {
	// 错误注入点
	if f.failSave {
		return context.DeadlineExceeded
	}
	return f.OperationRecordStore.Save(ctx, r)
}

// ctaErrCTAStore 是注入 Save 错误的直送记录存储桩。
type ctaErrCTAStore struct {
	CompleteToActivityStore
	failSave bool
}

// Save 按开关返回错误。
// @param ctx 操作上下文
// @param record 直送记录
// @return 错误
func (f *ctaErrCTAStore) Save(ctx context.Context, record *CompleteToActivity) error {
	// 错误注入点
	if f.failSave {
		return context.DeadlineExceeded
	}
	return f.CompleteToActivityStore.Save(ctx, record)
}

// withCompleteService 基于当前环境构造指定存储端口的办结服务变体。
// @param f 测试环境
// @param next 下一步存储端口
// @param messages 待办存储端口
// @param histories 已办存储端口
// @param operations 操作记录存储端口
// @param comments 评论能力
// @param completeTo 直送记录端口
// @return 办结服务变体
func withCompleteService(f *completeFixture, next NextActivityStore, messages MessageExtendStore, histories HistoryExtendStore, operations OperationRecordStore, comments TaskCommentEngine, completeTo CompleteToActivityStore) *CompleteService {
	return NewCompleteService(
		f.engine, f.meta,
		NewNextActivityService(next, f.fakeClock),
		NewCommentService(comments, f.engine.HistoryService(), f.engine.IdentityService()),
		NewOperationRecordService(operations, f.engine.HistoryService(), engineInstanceQuery{runtime: f.engine.RuntimeService(), history: f.engine.HistoryService()}, f.engine.RepositoryService(), f.engine.IdentityService(), f.fakeClock),
		NewMessageExtendService(messages, f.fakeClock),
		NewHistoryExtendService(histories, f.fakeClock),
		f.documents, completeTo, f.engine.IdentityService(), f.fakeClock,
	)
}

// agreeHandleRequest 构造"预存下一步 handle"的请求（错误用例辅助）。
// @return 预存请求
func agreeHandleRequest() *api.NextActivityRequest {
	return &api.NextActivityRequest{TaskID: "current", NextActivityList: []*api.NextActivityVo{
		{ActivityID: "handle", ExecutorIDs: []string{"alice"}},
	}}
}

// TestComplete_StoreErrorPropagation 验证办结链路各存储端口的错误传播（每个子用例独立环境）。
// @param t 测试上下文
func TestComplete_StoreErrorPropagation(t *testing.T) {
	cases := []struct {
		name string // 用例名
		want string // 期望错误片段
		run  func(t *testing.T, fx *completeFixture) error
	}{
		{"预存下一步失败", "注入错误", func(t *testing.T, fx *completeFixture) error {
			instance, apply := fx.start(t, "flowComplete", "e1", "alice")
			fx.seedFirstTodo(t, instance, apply)
			service := withCompleteService(fx, &failingNextStore{NextActivityStore: fx.stores.Next, failLatestVersion: true},
				fx.stores.Message, fx.stores.History, fx.stores.Operation, fx.engine.TaskService(), fx.completeTo)
			_, err := service.Complete(ctxB, &api.CompleteTaskVo{
				BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice", Message: "x"},
				Request:       agreeHandleRequest(),
			})
			return err
		}},
		{"意见写入失败", "context deadline exceeded", func(t *testing.T, fx *completeFixture) error {
			instance, apply := fx.start(t, "flowComplete", "e2", "alice")
			fx.seedFirstTodo(t, instance, apply)
			service := withCompleteService(fx, fx.stores.Next, fx.stores.Message, fx.stores.History, fx.stores.Operation,
				&ctaErrCommentTasks{TaskCommentEngine: fx.engine.TaskService(), failAdd: true}, fx.completeTo)
			_, err := service.Complete(ctxB, &api.CompleteTaskVo{
				BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice", Message: "x"},
				Request:       agreeHandleRequest(),
			})
			return err
		}},
		{"操作记录失败", "context deadline exceeded", func(t *testing.T, fx *completeFixture) error {
			instance, apply := fx.start(t, "flowComplete", "e3", "alice")
			fx.seedFirstTodo(t, instance, apply)
			service := withCompleteService(fx, fx.stores.Next, fx.stores.Message, fx.stores.History,
				&ctaErrOperationStore{OperationRecordStore: fx.stores.Operation, failSave: true}, fx.engine.TaskService(), fx.completeTo)
			_, err := service.Complete(ctxB, &api.CompleteTaskVo{
				BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice", Message: "x"},
				Request:       agreeHandleRequest(),
			})
			return err
		}},
		{"待办删除失败", "context deadline exceeded", func(t *testing.T, fx *completeFixture) error {
			instance, apply := fx.start(t, "flowComplete", "e4", "alice")
			fx.seedFirstTodo(t, instance, apply)
			service := withCompleteService(fx, fx.stores.Next,
				&ctaErrMessageStore{MessageExtendStore: fx.stores.Message, failDelete: true},
				fx.stores.History, fx.stores.Operation, fx.engine.TaskService(), fx.completeTo)
			_, err := service.Complete(ctxB, &api.CompleteTaskVo{
				BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice", Message: "x"},
				Request:       agreeHandleRequest(),
			})
			return err
		}},
		{"已办完成时间失败", "context deadline exceeded", func(t *testing.T, fx *completeFixture) error {
			instance, apply := fx.start(t, "flowComplete", "e5", "alice")
			fx.seedFirstTodo(t, instance, apply)
			service := withCompleteService(fx, fx.stores.Next, fx.stores.Message,
				&ctaErrHistoryStore{HistoryExtendStore: fx.stores.History, failUpdateFinish: true},
				fx.stores.Operation, fx.engine.TaskService(), fx.completeTo)
			_, err := service.Complete(ctxB, &api.CompleteTaskVo{
				BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice", Message: "x"},
				Request:       agreeHandleRequest(),
			})
			return err
		}},
		{"预存记录查询失败", "context deadline exceeded", func(t *testing.T, fx *completeFixture) error {
			instance, apply := fx.start(t, "flowComplete", "e6", "alice")
			fx.seedFirstTodo(t, instance, apply)
			service := withCompleteService(fx,
				&ctaErrNextStore{NextActivityStore: fx.stores.Next, failListByTask: true},
				fx.stores.Message, fx.stores.History, fx.stores.Operation, fx.engine.TaskService(), fx.completeTo)
			_, err := service.Complete(ctxB, &api.CompleteTaskVo{
				BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice", Message: "x"},
				Request:       agreeHandleRequest(),
			})
			return err
		}},
		{"预存记录删除失败", "context deadline exceeded", func(t *testing.T, fx *completeFixture) error {
			instance, apply := fx.start(t, "flowComplete", "e7", "alice")
			fx.seedFirstTodo(t, instance, apply)
			service := withCompleteService(fx,
				&ctaErrNextStore{NextActivityStore: fx.stores.Next, failDeleteForResolve: true},
				fx.stores.Message, fx.stores.History, fx.stores.Operation, fx.engine.TaskService(), fx.completeTo)
			_, err := service.Complete(ctxB, &api.CompleteTaskVo{
				BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice", Message: "x"},
				Request:       agreeHandleRequest(),
			})
			return err
		}},
		{"新待办查询失败", "注入错误", func(t *testing.T, fx *completeFixture) error {
			instance, apply := fx.start(t, "flowComplete", "e8", "alice")
			fx.seedFirstTodo(t, instance, apply)
			service := withCompleteService(fx, fx.stores.Next,
				&failingMessageStore{MessageExtendStore: fx.stores.Message, failGet: true},
				fx.stores.History, fx.stores.Operation, fx.engine.TaskService(), fx.completeTo)
			_, err := service.Complete(ctxB, &api.CompleteTaskVo{
				BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice", Message: "x"},
				Request:       agreeHandleRequest(),
			})
			return err
		}},
		{"新待办保存失败", "注入错误", func(t *testing.T, fx *completeFixture) error {
			instance, apply := fx.start(t, "flowComplete", "e9", "alice")
			fx.seedFirstTodo(t, instance, apply)
			service := withCompleteService(fx, fx.stores.Next,
				&failingMessageStore{MessageExtendStore: fx.stores.Message, failSave: true},
				fx.stores.History, fx.stores.Operation, fx.engine.TaskService(), fx.completeTo)
			_, err := service.Complete(ctxB, &api.CompleteTaskVo{
				BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice", Message: "x"},
				Request:       agreeHandleRequest(),
			})
			return err
		}},
		{"已办镜像失败", "context deadline exceeded", func(t *testing.T, fx *completeFixture) error {
			instance, apply := fx.start(t, "flowComplete", "e10", "alice")
			fx.seedFirstTodo(t, instance, apply)
			service := withCompleteService(fx, fx.stores.Next, fx.stores.Message,
				&ctaErrHistoryStore{HistoryExtendStore: fx.stores.History, failSave: true},
				fx.stores.Operation, fx.engine.TaskService(), fx.completeTo)
			_, err := service.Complete(ctxB, &api.CompleteTaskVo{
				BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice", Message: "x"},
				Request:       agreeHandleRequest(),
			})
			return err
		}},
		{"直送记录失败", "context deadline exceeded", func(t *testing.T, fx *completeFixture) error {
			instance, apply := fx.start(t, "flowFreeSelect", "e11", "alice")
			fx.seedFirstTodo(t, instance, apply)
			service := withCompleteService(fx, fx.stores.Next, fx.stores.Message, fx.stores.History, fx.stores.Operation,
				fx.engine.TaskService(), &ctaErrCTAStore{CompleteToActivityStore: fx.completeTo, failSave: true})
			_, err := service.Complete(ctxB, &api.CompleteTaskVo{
				BaseProcessVo:    api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice", Message: "x"},
				TargetActivityID: "approve",
				Request:          agreeHandleRequest(),
			})
			return err
		}},
	}
	for _, tc := range cases {
		fx := newCompleteFixture(t)
		if err := tc.run(t, fx); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: 期望错误含 %q，实际 %v", tc.name, tc.want, err)
		}
	}
}

// TestComplete_SkipEndAndNilRequestItems 验证预存请求中空条目与 end 条目跳过落库。
// @param t 测试上下文
func TestComplete_SkipEndAndNilRequestItems(t *testing.T) {
	fx := newCompleteFixture(t)
	instance, apply := fx.start(t, "flowComplete", "e-skip", "alice")
	fx.seedFirstTodo(t, instance, apply)
	result, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice", Message: "x", Type: CommentTypeSP},
		Request: &api.NextActivityRequest{TaskID: apply.ID, NextActivityList: []*api.NextActivityVo{
			{ActivityID: "handle", ExecutorIDs: []string{"alice"}},
			nil,
			{ActivityID: StepCodeEnd},
		}},
	})
	if err != nil {
		t.Fatalf("办结失败: %v", err)
	}
	// 空条目与 end 跳过，handle 落库 1 条
	unused, _ := fx.next.ListUnusedByInstanceAndActivity(ctxB, instance.ID, "handle")
	if len(unused) != 1 {
		t.Fatalf("handle 预存应为 1 条，实际 %d", len(unused))
	}
	if len(result.NextTaskIDs) != 1 {
		t.Fatalf("新待办应为 1 条: %v", result.NextTaskIDs)
	}
}

// TestComplete_NoMigrationWhenTargetMatches 验证实际推进节点已命中目标时不迁移、不落直送记录。
// @param t 测试上下文
func TestComplete_NoMigrationWhenTargetMatches(t *testing.T) {
	fx := newCompleteFixture(t)
	instance, apply := fx.start(t, "flowComplete", "e-match", "alice")
	fx.seedFirstTodo(t, instance, apply)
	_, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo:    api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice", Message: "x"},
		TargetActivityID: "handle",
		Request:          agreeHandleRequest(),
	})
	if err != nil {
		t.Fatalf("办结失败: %v", err)
	}
	// 实际节点即目标：无直送记录
	records, _ := fx.completeTo.ListByTask(ctxB, apply.ID)
	if len(records) != 0 {
		t.Fatalf("不应产生直送记录: %+v", records)
	}
}

// TestComplete_VariablesPassedThrough 验证请求变量原样并入引擎办结变量。
// @param t 测试上下文
func TestComplete_VariablesPassedThrough(t *testing.T) {
	fx := newCompleteFixture(t)
	instance, apply := fx.start(t, "flowComplete", "e-var", "alice")
	fx.seedFirstTodo(t, instance, apply)
	if _, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice", Message: "x"},
		Variables:     map[string]any{"customVar": "v1"},
		Request:       agreeHandleRequest(),
	}); err != nil {
		t.Fatalf("办结失败: %v", err)
	}
	value, err := fx.engine.RuntimeService().GetVariable(ctxB, instance.ID, "customVar")
	if err != nil || value != "v1" {
		t.Fatalf("自定义变量应传入引擎: %v（%v）", value, err)
	}
}

// TestComplete_ParentNotPendingResolveError 验证父任务未处于委派状态时归还报错。
// @param t 测试上下文
func TestComplete_ParentNotPendingResolveError(t *testing.T) {
	fx := newCompleteFixture(t)
	instance, apply := fx.start(t, "flowComplete", "e-pnp", "alice")
	fx.seedFirstTodo(t, instance, apply)
	// 推进到 handle（未委派，非 PENDING）
	if _, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice", Message: "x"},
		Request:       agreeHandleRequest(),
	}); err != nil {
		t.Fatalf("首次办结失败: %v", err)
	}
	parent := fx.firstTaskOf(t, instance.ID)
	sub, err := fx.engine.TaskService().CreateTask(ctxB, api.CreateTaskRequest{Name: "加签", Assignee: "helper", ParentTaskID: parent.ID, TenantID: "dev"})
	if err != nil {
		t.Fatalf("创建子任务失败: %v", err)
	}
	_, err = fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: sub.ID, UserCode: "helper", Message: "x"},
	})
	if err == nil || !strings.Contains(err.Error(), "任务未处于委派状态") {
		t.Fatalf("期望委派状态错误，实际 %v", err)
	}
}

// TestComplete_WeightValueParsing 验证权重值的多类型解析。
// @param t 测试上下文
func TestComplete_WeightValueParsing(t *testing.T) {
	cases := []struct {
		value any     // 输入值
		want  float64 // 期望浮点
	}{
		{2.5, 2.5},
		{float32(1.5), 1.5},
		{3, 3},
		{int32(4), 4},
		{int64(5), 5},
		{"6.5", 6.5},
		{nil, 0},
		{true, 0},
	}
	for _, tc := range cases {
		if got := weightToFloatOf(tc.value); got != tc.want {
			t.Fatalf("%v: 期望 %v，实际 %v", tc.value, tc.want, got)
		}
	}
	// 字符串解析：非法串回退 0
	if got := parseWeightString("abc"); got != 0 {
		t.Fatalf("非法串应回退 0，实际 %v", got)
	}
	if got := parseWeightString("  7.5  "); got != 7.5 {
		t.Fatalf("空白串解析错误: %v", got)
	}
	if got := parseWeightString(""); got != 0 {
		t.Fatalf("空串应回退 0，实际 %v", got)
	}
}

// TestComplete_AssignTailErrorPropagation 验证委派/转办/归还收尾链错误传播与未知用户容错。
// @param t 测试上下文
func TestComplete_AssignTailErrorPropagation(t *testing.T) {
	// 意见失败
	fx := newCompleteFixture(t)
	instance, apply := fx.start(t, "flowComplete", "e-wp", "alice")
	fx.seedFirstTodo(t, instance, apply)
	commentFail := withCompleteService(fx, fx.stores.Next, fx.stores.Message, fx.stores.History, fx.stores.Operation,
		&ctaErrCommentTasks{TaskCommentEngine: fx.engine.TaskService(), failAdd: true}, fx.completeTo)
	if err := commentFail.DelegateTask(ctxB, &DelegateTaskRequest{TaskID: apply.ID, Owner: "alice", DelegateID: "bob", Message: "m"}); err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("委派意见失败应传播: %v", err)
	}

	// 操作记录失败
	opFail := withCompleteService(fx, fx.stores.Next, fx.stores.Message, fx.stores.History,
		&ctaErrOperationStore{OperationRecordStore: fx.stores.Operation, failSave: true}, fx.engine.TaskService(), fx.completeTo)
	if err := opFail.TransferTask(ctxB, &TransferTaskRequest{TaskID: apply.ID, Owner: "alice", Receiver: "carl", Message: "m"}); err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("转办操作记录失败应传播: %v", err)
	}

	// 归还操作记录失败
	if err := opFail.ResolveTask(ctxB, &ResolveTaskRequest{TaskID: apply.ID, Message: "m"}); err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("归还操作记录失败应传播: %v", err)
	}

	// 委派预存删除失败
	nextFail := withCompleteService(fx,
		&ctaErrNextStore{NextActivityStore: fx.stores.Next, failDeleteForResolve: true},
		fx.stores.Message, fx.stores.History, fx.stores.Operation, fx.engine.TaskService(), fx.completeTo)
	if err := nextFail.DelegateTask(ctxB, &DelegateTaskRequest{TaskID: apply.ID, Owner: "alice", DelegateID: "bob", Message: "m"}); err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("委派预存删除失败应传播: %v", err)
	}

	// 未知用户容错（展示名留空，不阻断）
	fx2 := newCompleteFixture(t)
	instance2, apply2 := fx2.start(t, "flowComplete", "e-ghost", "alice")
	fx2.seedFirstTodo(t, instance2, apply2)
	if err := fx2.complete.DelegateTask(ctxB, &DelegateTaskRequest{TaskID: apply2.ID, Owner: "alice", DelegateID: "ghost", Message: "m"}); err != nil {
		t.Fatalf("未知受托人不应报错: %v", err)
	}
	todo, found, _ := fx2.messages.GetByTaskID(ctxB, apply2.ID)
	if !found || todo.OwnerID != "ghost" || todo.Consigner != "" {
		t.Fatalf("未知受托人待办应更新且姓名留空: %+v", todo)
	}
}

// TestComplete_DeleteTransferSkipsSelfLoop 验证委派/转办清理跳过自环预存记录。
// @param t 测试上下文
func TestComplete_DeleteTransferSkipsSelfLoop(t *testing.T) {
	fx := newCompleteFixture(t)
	instance, apply := fx.start(t, "flowComplete", "e-loop", "alice")
	fx.seedFirstTodo(t, instance, apply)
	// 自环记录：preTaskId = 当前任务（另用独立身份，避免同身份整批删除的存储语义覆盖）
	if err := fx.next.InsertDual(ctxB, &NextActivity{
		BaseColumns:    BaseColumns{TenantID: instance.TenantID},
		NextActivityID: "apply", IdentityID: "carl", SendIdentityID: "alice",
		ProcessInstanceID: instance.ID, Used: 1, TaskID: apply.ID, PreTaskID: apply.ID, Version: 9,
	}, "alice"); err != nil {
		t.Fatalf("播种自环记录失败: %v", err)
	}
	if err := fx.complete.DelegateTask(ctxB, &DelegateTaskRequest{TaskID: apply.ID, Owner: "alice", DelegateID: "bob", Message: "m"}); err != nil {
		t.Fatalf("委派失败: %v", err)
	}
	// 自环记录保留（pre_task_id=taskId 的记录不作为清理触发点）
	remaining, _ := fx.next.ListByTaskID(ctxB, apply.ID)
	selfLoop := false
	for _, record := range remaining {
		if record.PreTaskID == apply.ID && record.IdentityID == "carl" {
			selfLoop = true
		}
	}
	if !selfLoop {
		t.Fatalf("自环记录应保留: %+v", remaining)
	}
	// 发起侧消费记录已被清理（pre_task_id != 当前任务）
	for _, record := range remaining {
		if record.IdentityID == "alice" && record.PreTaskID != apply.ID {
			t.Fatalf("非自环记录应被清理: %+v", record)
		}
	}
}

// TestQuery_ErrorPropagationAndFallbacks 验证查询门面错误传播与缺省分支。
// @param t 测试上下文
func TestQuery_ErrorPropagationAndFallbacks(t *testing.T) {
	fx := newCompleteFixture(t)
	seedQueryRows(t, fx)

	// Start 负值归 0
	page, err := fx.query.TodoListByIdentityID(ctxB, "alice", PageQuery{Start: -5, Size: 1})
	if err != nil || page.Start != 0 || len(page.Data) != 1 {
		t.Fatalf("负偏移应归 0: %+v（%v）", page, err)
	}
	// 待办查询存储失败
	failMsg := NewQueryService(fx.engine,
		NewMessageExtendService(&failingMessageStore{MessageExtendStore: fx.stores.Message, failList: true}, fx.fakeClock),
		fx.histories, fx.operations, fx.documents)
	if _, err := failMsg.TodoListByIdentityID(ctxB, "alice", PageQuery{}); err == nil {
		t.Fatalf("待办存储失败应传播")
	}
	// 已办查询存储失败
	failHist := NewQueryService(fx.engine, fx.messages,
		NewHistoryExtendService(&failingHistoryStore{HistoryExtendStore: fx.stores.History, failListOwner: true}, fx.fakeClock),
		fx.operations, fx.documents)
	if _, err := failHist.DoneListByIdentityID(ctxB, "alice", PageQuery{}); err == nil {
		t.Fatalf("已办存储失败应传播")
	}
	// 无 document 实例：状态按引擎结束标记回退
	_, bareTask := fx.start(t, "flowComplete", "e-bare", "carl")
	_ = bareTask
	bareList, err := fx.query.InstanceListByUser(ctxB, "carl", PageQuery{})
	if err != nil || bareList.Total != 1 || bareList.Data[0].Status != InstanceStatusRunning {
		t.Fatalf("无 document 实例应回退运行中: %+v（%v）", bareList, err)
	}
}

// TestQuery_DoneJoinEngineHistory 验证已办列表联查引擎历史任务补办结时间。
// @param t 测试上下文
func TestQuery_DoneJoinEngineHistory(t *testing.T) {
	fx := newCompleteFixture(t)
	instance, apply := fx.start(t, "flowComplete", "e-done", "alice")
	fx.seedFirstTodo(t, instance, apply)
	// 办结（产生引擎历史任务）
	if _, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice", Message: "x"},
		Request:       agreeHandleRequest(),
	}); err != nil {
		t.Fatalf("办结失败: %v", err)
	}
	// 已办行（空租户）挂到已完成任务上
	finish := completeFixtureTime.Add(time.Hour)
	if err := fx.stores.History.Save(ctxB, &HistoryExtend{
		MessageExtend: MessageExtend{
			BaseColumns: BaseColumns{ID: "qh-done", TenantID: "", CreateTime: completeFixtureTime},
			TaskID:      apply.ID, ProcessInstanceID: instance.ID, OwnerID: "alice",
		},
		TaskFinishTime: &finish,
	}); err != nil {
		t.Fatalf("播种已办失败: %v", err)
	}
	done, err := fx.query.DoneListByIdentityID(ctxB, "alice", PageQuery{Size: 10})
	if err != nil || done.Total < 1 {
		t.Fatalf("已办查询失败: total=%d（%v）", done.Total, err)
	}
	// 定位本用例播种的 qh-done 行（空租户不过滤语义下与其他播种行同列）
	var target *DoneTaskView
	for _, view := range done.Data {
		if view.TaskID == apply.ID {
			target = view
			break
		}
	}
	if target == nil || !target.EngineExists || target.EngineFinishTime == nil {
		t.Fatalf("引擎历史任务联查失败: %+v", target)
	}
}

// TestComplete_CoverageExtras 验证构造缺省、权重配置解析边界、变量现值累计与其他节点的预存记录保留。
// @param t 测试上下文
func TestComplete_CoverageExtras(t *testing.T) {
	fx := newCompleteFixture(t)

	// nil 时钟与 nil 身份查询的构造（回退真实时钟/留空姓名）
	bare := NewCompleteService(fx.engine, fx.meta, fx.next, fx.comments, fx.operations, fx.messages, fx.histories, fx.documents, fx.completeTo, nil, nil)
	if bare == nil {
		t.Fatalf("缺省构造不应为空")
	}
	// 权重配置解析边界：非 JSON 与非法 JSON 均视为未配置
	if cfgs := parseWeightConfigs(""); cfgs != nil {
		t.Fatalf("空串应返回 nil: %+v", cfgs)
	}
	if cfgs := parseWeightConfigs("plain"); cfgs != nil {
		t.Fatalf("非 JSON 应返回 nil: %+v", cfgs)
	}
	if cfgs := parseWeightConfigs("[bad"); cfgs != nil {
		t.Fatalf("非法 JSON 应返回 nil: %+v", cfgs)
	}

	// 变量现值累计：预置 totalWeight=10，alice 通过 +2.5 → 12.5
	instance, wtask := fx.start(t, "flowWeight", "e-w0", "alice")
	if err := fx.engine.RuntimeService().SetVariable(ctxB, instance.ID, VarTotalWeight, 10); err != nil {
		t.Fatalf("预置权重失败: %v", err)
	}
	if _, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo:        api.BaseProcessVo{TaskID: wtask.ID, UserCode: "alice", Message: "通过"},
		CountersignResult:    1,
		CountersignResultSet: true,
	}); err != nil {
		t.Fatalf("权重办结失败: %v", err)
	}
	if value, _ := fx.engine.RuntimeService().GetVariable(ctxB, instance.ID, VarTotalWeight); value != 12.5 {
		t.Fatalf("期望 totalWeight=12.5，实际 %v", value)
	}

	// 其他节点的预存记录在删当前节点时保留（continue 分支）
	instance2, apply := fx.start(t, "flowComplete", "e-other", "alice")
	fx.seedFirstTodo(t, instance2, apply)
	if err := fx.next.InsertDual(ctxB, &NextActivity{
		BaseColumns:    BaseColumns{TenantID: instance2.TenantID},
		NextActivityID: "otherNode", IdentityID: "alice", SendIdentityID: "alice",
		ProcessInstanceID: instance2.ID, Used: 1, TaskID: apply.ID, Version: 9,
	}, "alice"); err != nil {
		t.Fatalf("播种他节点记录失败: %v", err)
	}
	if _, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice", Message: "x"},
		Request:       agreeHandleRequest(),
	}); err != nil {
		t.Fatalf("办结失败: %v", err)
	}
	kept, _ := fx.next.ListByTaskID(ctxB, apply.ID)
	foundOther := false
	for _, record := range kept {
		if record.NextActivityID == "otherNode" {
			foundOther = true
		}
	}
	if !foundOther {
		t.Fatalf("其他节点预存记录应保留: %+v", kept)
	}
}

// TestComplete_TodoOwnerGroupFilled 验证新待办的处理人组信息经身份库补全。
// @param t 测试上下文
func TestComplete_TodoOwnerGroupFilled(t *testing.T) {
	fx := newCompleteFixture(t)
	// 创建组并建立 alice 的成员关系
	if _, err := fx.engine.IdentityService().CreateGroup(ctxB, api.Group{ID: "grp-1", Name: "研发部", TenantID: "dev"}); err != nil {
		t.Fatalf("创建组失败: %v", err)
	}
	if err := fx.engine.IdentityService().AddMembership(ctxB, "alice", "grp-1"); err != nil {
		t.Fatalf("建立成员关系失败: %v", err)
	}
	instance, apply := fx.start(t, "flowComplete", "e-grp", "alice")
	fx.seedFirstTodo(t, instance, apply)
	if _, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice", Message: "x"},
		Request:       agreeHandleRequest(),
	}); err != nil {
		t.Fatalf("办结失败: %v", err)
	}
	handle := fx.firstTaskOf(t, instance.ID)
	todo, found, _ := fx.messages.GetByTaskID(ctxB, handle.ID)
	if !found || todo.OwnerGroup != "研发部" || todo.OwnerGroupID != "grp-1" || todo.OwnerGroupPaths != "研发部" {
		t.Fatalf("待办组信息应补全: %+v", todo)
	}
}

// TestComplete_TransferResolveValidationExtras 验证转办/归还的补充校验与错误传播。
// @param t 测试上下文
func TestComplete_TransferResolveValidationExtras(t *testing.T) {
	fx := newCompleteFixture(t)
	instance, apply := fx.start(t, "flowComplete", "e-tre", "alice")
	fx.seedFirstTodo(t, instance, apply)

	// 转办必填校验：taskId / owner / message 缺失与任务不存在
	if err := fx.complete.TransferTask(ctxB, &TransferTaskRequest{Owner: "alice", Receiver: "carl", Message: "m"}); err == nil || !strings.Contains(err.Error(), "参数 taskId 为空") {
		t.Fatalf("期望转办 taskId 校验错误，实际 %v", err)
	}
	if err := fx.complete.TransferTask(ctxB, &TransferTaskRequest{TaskID: apply.ID, Receiver: "carl", Message: "m"}); err == nil || !strings.Contains(err.Error(), "参数 owner 为空") {
		t.Fatalf("期望转办 owner 校验错误，实际 %v", err)
	}
	if err := fx.complete.TransferTask(ctxB, &TransferTaskRequest{TaskID: apply.ID, Owner: "alice", Receiver: "carl"}); err == nil || !strings.Contains(err.Error(), "参数 message 为空") {
		t.Fatalf("期望转办 message 校验错误，实际 %v", err)
	}
	if err := fx.complete.TransferTask(ctxB, &TransferTaskRequest{TaskID: "ghost", Owner: "alice", Receiver: "carl", Message: "m"}); err == nil || !strings.Contains(err.Error(), "任务不存在") {
		t.Fatalf("期望转办任务不存在错误，实际 %v", err)
	}
	// 归还：任务不存在
	if err := fx.complete.ResolveTask(ctxB, &ResolveTaskRequest{TaskID: "ghost", Message: "m"}); err == nil || !strings.Contains(err.Error(), "任务不存在") {
		t.Fatalf("期望归还任务不存在错误，实际 %v", err)
	}
	// 归还：任务非委派状态（引擎 Resolve 拒绝）
	if err := fx.complete.ResolveTask(ctxB, &ResolveTaskRequest{TaskID: apply.ID, Message: "m"}); err == nil || !strings.Contains(err.Error(), "任务未处于委派状态") {
		t.Fatalf("期望委派状态错误，实际 %v", err)
	}
	// 归还：意见写入失败传播
	commentFail := withCompleteService(fx, fx.stores.Next, fx.stores.Message, fx.stores.History, fx.stores.Operation,
		&ctaErrCommentTasks{TaskCommentEngine: fx.engine.TaskService(), failAdd: true}, fx.completeTo)
	if err := fx.complete.DelegateTask(ctxB, &DelegateTaskRequest{TaskID: apply.ID, Owner: "alice", DelegateID: "bob", Message: "m"}); err != nil {
		t.Fatalf("委派失败: %v", err)
	}
	if err := commentFail.ResolveTask(ctxB, &ResolveTaskRequest{TaskID: apply.ID, Message: "m"}); err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("归还意见失败应传播: %v", err)
	}
}

// TestComplete_DelegationStoreErrorBranches 验证委派分支内预存写入/清理失败的传播。
// @param t 测试上下文
func TestComplete_DelegationStoreErrorBranches(t *testing.T) {
	// SaveResolveNext 失败（版本推算注入错误）
	fx := newCompleteFixture(t)
	instance, apply := fx.start(t, "flowComplete", "e-ds1", "alice")
	fx.seedFirstTodo(t, instance, apply)
	if err := fx.engine.TaskService().Delegate(ctxB, apply.ID, "bob"); err != nil {
		t.Fatalf("委派失败: %v", err)
	}
	if err := fx.next.InsertDual(ctxB, &NextActivity{
		BaseColumns:    BaseColumns{TenantID: instance.TenantID},
		NextActivityID: "apply", IdentityID: "bob", SendIdentityID: "bob",
		ProcessInstanceID: instance.ID, Used: 1, TaskID: apply.ID, Version: 1,
	}, "bob"); err != nil {
		t.Fatalf("播种委派预存失败: %v", err)
	}
	service := withCompleteService(fx, &failingNextStore{NextActivityStore: fx.stores.Next, failLatestVersion: true},
		fx.stores.Message, fx.stores.History, fx.stores.Operation, fx.engine.TaskService(), fx.completeTo)
	if _, err := service.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, UserCode: "bob", Message: "x"},
	}); err == nil || !strings.Contains(err.Error(), "注入错误") {
		t.Fatalf("还原预存失败应传播: %v", err)
	}

	// DeleteForResolve 失败（清理注入错误）
	fx2 := newCompleteFixture(t)
	instance2, apply2 := fx2.start(t, "flowComplete", "e-ds2", "alice")
	fx2.seedFirstTodo(t, instance2, apply2)
	if err := fx2.engine.TaskService().Delegate(ctxB, apply2.ID, "bob"); err != nil {
		t.Fatalf("委派失败: %v", err)
	}
	if err := fx2.next.InsertDual(ctxB, &NextActivity{
		BaseColumns:    BaseColumns{TenantID: instance2.TenantID},
		NextActivityID: "apply", IdentityID: "bob", SendIdentityID: "bob",
		ProcessInstanceID: instance2.ID, Used: 1, TaskID: apply2.ID, Version: 1,
	}, "bob"); err != nil {
		t.Fatalf("播种委派预存失败: %v", err)
	}
	service2 := withCompleteService(fx2,
		&ctaErrNextStore{NextActivityStore: fx2.stores.Next, failDeleteForResolve: true},
		fx2.stores.Message, fx2.stores.History, fx2.stores.Operation, fx2.engine.TaskService(), fx2.completeTo)
	if _, err := service2.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: apply2.ID, UserCode: "bob", Message: "x"},
	}); err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("委派清理失败应传播: %v", err)
	}
}

// TestSQLCompleteToActivityStoreWithMock 验证直送记录 SQL 适配（sqlmock）。
// @param t 测试上下文
func TestSQLCompleteToActivityStoreWithMock(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock 构造失败: %v", err)
	}
	defer func() { _ = db.Close() }()
	store, err := pwf.NewStore(db, pwf.DialectMySQL)
	if err != nil {
		t.Fatalf("仓储入口构造失败: %v", err)
	}
	adapter := NewSQLCompleteToActivityStore(store)

	// Save：断言 INSERT 语句
	mock.ExpectExec("INSERT INTO workflow_complete_to_activity").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := adapter.Save(ctxB, &CompleteToActivity{
		BaseColumns:       BaseColumns{ID: "cta-1", TenantID: "dev"},
		SourceActivityID:  "apply",
		TargetActivityID:  "approve",
		TaskID:            "t-1",
		ProcessInstanceID: "pi-1",
	}); err != nil {
		t.Fatalf("SQL Save 失败: %v", err)
	}

	// ListByProcessInstance：断言 SELECT 与行转换
	rows := sqlmock.NewRows([]string{"id", "flow_id", "source_flow_element_id", "target_flow_element_id", "task_id", "process_instance_id",
		"creator", "creator_name", "create_time", "updater", "update_time", "tenant_id"}).
		AddRow("cta-1", "", "apply", "approve", "t-1", "pi-1", "u1", "张三", "2026-10-31 08:00:00", "u1", "2026-10-31 08:00:00", "dev")
	mock.ExpectQuery("SELECT (.+) FROM workflow_complete_to_activity").WithArgs("pi-1").WillReturnRows(rows)
	list, err := adapter.ListByProcessInstance(ctxB, "pi-1")
	if err != nil || len(list) != 1 || list[0].TargetActivityID != "approve" {
		t.Fatalf("SQL 查询失败: %v %+v", err, list)
	}

	// ListByTask
	rows2 := sqlmock.NewRows([]string{"id", "flow_id", "source_flow_element_id", "target_flow_element_id", "task_id", "process_instance_id",
		"creator", "creator_name", "create_time", "updater", "update_time", "tenant_id"})
	mock.ExpectQuery("SELECT (.+) FROM workflow_complete_to_activity").WithArgs("t-9").WillReturnRows(rows2)
	empty, err := adapter.ListByTask(ctxB, "t-9")
	if err != nil || len(empty) != 0 {
		t.Fatalf("SQL 按任务查询失败: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL 期望未满足: %v", err)
	}
}
