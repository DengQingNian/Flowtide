// p4_extra_test.go 补充 P4 域服务的边界覆盖：失败存储注入、服务缺省装配、
// 调度循环直跑、规则异常路径与对象展开的异常分支。
package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
)

// failingRemindStore 是恒失败的催办配置存储桩。
type failingRemindStore struct {
	err error // 注入的失败错误
}

// Save 恒返回注入错误。
// @param ctx 操作上下文
// @param remind 催办配置
// @return 注入错误
func (f *failingRemindStore) Save(ctx context.Context, remind *Remind) error { return f.err }

// Get 恒返回注入错误。
// @param ctx 操作上下文
// @param id 配置 Id
// @return 空配置
// @return false
// @return 注入错误
func (f *failingRemindStore) Get(ctx context.Context, id string) (*Remind, bool, error) {
	return nil, false, f.err
}

// ListByModel 恒返回注入错误。
// @param ctx 操作上下文
// @param modelID 模型 Id
// @return nil
// @return 注入错误
func (f *failingRemindStore) ListByModel(ctx context.Context, modelID string) ([]*Remind, error) {
	return nil, f.err
}

// ListActive 恒返回注入错误。
// @param ctx 操作上下文
// @return nil
// @return 注入错误
func (f *failingRemindStore) ListActive(ctx context.Context) ([]*Remind, error) { return nil, f.err }

// ListNotSent 恒返回注入错误。
// @param ctx 操作上下文
// @return nil
// @return 注入错误
func (f *failingRemindStore) ListNotSent(ctx context.Context) ([]*Remind, error) { return nil, f.err }

// MarkSent 恒返回注入错误。
// @param ctx 操作上下文
// @param id 配置 Id
// @return 注入错误
func (f *failingRemindStore) MarkSent(ctx context.Context, id string) error { return f.err }

// MarkDeleted 恒返回注入错误。
// @param ctx 操作上下文
// @param id 配置 Id
// @return 注入错误
func (f *failingRemindStore) MarkDeleted(ctx context.Context, id string) error { return f.err }

// MarkDeletedByIDs 恒返回注入错误。
// @param ctx 操作上下文
// @param ids 配置 Id 列表
// @return 注入错误
func (f *failingRemindStore) MarkDeletedByIDs(ctx context.Context, ids []string) error {
	return f.err
}

// failingRemindTargetStore 是恒失败的催办对象存储桩。
type failingRemindTargetStore struct {
	err error // 注入的失败错误
}

// Save 恒返回注入错误。
// @param ctx 操作上下文
// @param target 催办对象
// @return 注入错误
func (f *failingRemindTargetStore) Save(ctx context.Context, target *RemindTarget) error {
	return f.err
}

// ListByRemind 恒返回注入错误。
// @param ctx 操作上下文
// @param remindID 配置 Id
// @return nil
// @return 注入错误
func (f *failingRemindTargetStore) ListByRemind(ctx context.Context, remindID string) ([]*RemindTarget, error) {
	return nil, f.err
}

// DeleteByRemind 恒返回注入错误。
// @param ctx 操作上下文
// @param remindID 配置 Id
// @return 注入错误
func (f *failingRemindTargetStore) DeleteByRemind(ctx context.Context, remindID string) error {
	return f.err
}

// MarkSent 恒返回注入错误。
// @param ctx 操作上下文
// @param id 对象 Id
// @return 注入错误
func (f *failingRemindTargetStore) MarkSent(ctx context.Context, id string) error { return f.err }

// UpdateRetryCount 恒返回注入错误。
// @param ctx 操作上下文
// @param id 对象 Id
// @param count 新计数
// @return 注入错误
func (f *failingRemindTargetStore) UpdateRetryCount(ctx context.Context, id string, count int) error {
	return f.err
}

// erroringRuleProvider 是规则读取失败的规则提供方桩。
type erroringRuleProvider struct {
	limitErr    error // RemindLimitOf 注入错误
	autoErr     error // AutoCompleteRuleOf 注入错误
	zeroMembers bool  // 成员查询返回空集
}

// RemindLimitOf 返回注入错误或空配置。
// @param ctx 操作上下文
// @param stepCode 步骤代号
// @return 时限指针
// @return 单位
// @return 错误
func (e *erroringRuleProvider) RemindLimitOf(ctx context.Context, stepCode string) (*float64, string, error) {
	return nil, "", e.limitErr
}

// AutoCompleteRuleOf 返回注入错误或零值规则。
// @param ctx 操作上下文
// @param stepCode 步骤代号
// @return 规则
// @return 错误
func (e *erroringRuleProvider) AutoCompleteRuleOf(ctx context.Context, stepCode string) (AutoCompleteRule, error) {
	return AutoCompleteRule{}, e.autoErr
}

// erroringStepResolver 是步骤解析失败的解析桩。
type erroringStepResolver struct{}

// ResolveStepID 恒返回错误。
// @param ctx 操作上下文
// @param stepCode 步骤代号
// @return 空串
// @return 错误
func (erroringStepResolver) ResolveStepID(ctx context.Context, stepCode string) (string, error) {
	return "", errors.New("resolve failed")
}

// erroringMembers 是成员查询失败的成员桩。
type erroringMembers struct {
	err    error      // 注入错误
	groups map[string][]api.User // 组成员
}

// ListMembers 返回注入错误或成员。
// @param ctx 操作上下文
// @param groupID 组 Id
// @return 成员列表
// @return 错误
func (e *erroringMembers) ListMembers(ctx context.Context, groupID string) ([]api.User, error) {
	if e.err != nil {
		return nil, e.err
	}
	return e.groups[groupID], nil
}

// erroringVariables 是变量读取失败的变量桩。
type erroringVariables struct {
	err   error         // 注入错误
	value any           // 注入返回值
}

// GetVariable 返回注入错误或值。
// @param ctx 操作上下文
// @param processInstanceID 实例 Id
// @param name 变量名
// @return 变量值
// @return 错误
func (e *erroringVariables) GetVariable(ctx context.Context, processInstanceID, name string) (any, error) {
	if e.err != nil {
		return nil, e.err
	}
	return e.value, nil
}

// TestRemindServiceFailingStores 验证存储失败路径全部上抛。
// @param t 测试上下文
func TestRemindServiceFailingStores(t *testing.T) {
	storeErr := errors.New("db down")
	svc := NewRemindService(
		&failingRemindStore{err: storeErr},
		&failingRemindTargetStore{err: storeErr},
		&failingTodoSource{err: storeErr},
		NopScheduleRuleProvider{}, NopChannelProvider{}, nil, nil, nil, nil)
	ctx := context.Background()
	// Add：主表保存失败
	if _, err := svc.Add(ctx, RemindSaveRequest{Remind: &Remind{PID: "model-1"}}); err == nil {
		t.Error("主表保存失败应上抛")
	}
	// Add：对象保存失败（主表内存实现）
	memTargets := NewMemRemindTargetStore(nil)
	okSvc := NewRemindService(NewMemRemindStore(nil), memTargets, nil, NopScheduleRuleProvider{}, NopChannelProvider{}, nil, nil, nil, nil)
	if _, err := okSvc.Add(ctx, RemindSaveRequest{
		Remind:  &Remind{PID: "model-1"},
		Targets: []*RemindTarget{{ExecutorID: "-1111", PID: "blocked"}},
	}); err == nil {
		t.Error("对象保存失败应上抛")
	}
	// Update：Get 失败 / 保存失败
	if _, err := svc.Update(ctx, RemindSaveRequest{Remind: &Remind{ID: "r-1", PID: "model-1"}}); err == nil {
		t.Error("Get 失败应上抛")
	}
	memSvc := NewRemindService(NewMemRemindStore(nil), NewMemRemindTargetStore(nil), nil, NopScheduleRuleProvider{}, NopChannelProvider{}, nil, nil, nil, nil)
	if _, err := memSvc.Update(ctx, RemindSaveRequest{Remind: &Remind{ID: "r-1", PID: "model-1"}}); err == nil {
		t.Error("更新不存在应报错")
	}
	// Get/List/Remove/批量删除失败
	if _, _, err := svc.Get(ctx, "r-1"); err == nil {
		t.Error("Get 失败应上抛")
	}
	if _, err := svc.List(ctx, "model-1", PageQuery{}); err == nil {
		t.Error("List 失败应上抛")
	}
	if err := svc.Remove(ctx, "r-1"); err == nil {
		t.Error("Remove 失败应上抛")
	}
	if err := svc.RemoveBatch(ctx, []string{"r-1"}); err == nil {
		t.Error("RemoveBatch 失败应上抛")
	}
	// ScanDue/SendNow 失败
	if _, err := svc.ScanDue(ctx, time.Now()); err == nil {
		t.Error("ScanDue 配置查询失败应上抛")
	}
	if _, err := svc.SendNow(ctx, "task-1"); err == nil {
		t.Error("SendNow 配置查询失败应上抛")
	}
}

// failingTodoSource 是恒失败的待办数据源桩。
type failingTodoSource struct {
	err error // 注入的失败错误
}

// ListAll 恒返回注入错误。
// @param ctx 操作上下文
// @return nil
// @return 注入错误
func (f *failingTodoSource) ListAll(ctx context.Context) ([]*MessageExtend, error) {
	return nil, f.err
}

// GetByTaskID 恒返回注入错误。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 空待办
// @return false
// @return 注入错误
func (f *failingTodoSource) GetByTaskID(ctx context.Context, taskID string) (*MessageExtend, bool, error) {
	return nil, false, f.err
}

// TestRemindServiceScanAndSendErrors 验证待办数据源失败路径。
// @param t 测试上下文
func TestRemindServiceScanAndSendErrors(t *testing.T) {
	storeErr := errors.New("todo down")
	memReminds := NewMemRemindStore(nil)
	memTargets := NewMemRemindTargetStore(nil)
	svc := NewRemindService(memReminds, memTargets, &failingTodoSource{err: storeErr}, NopScheduleRuleProvider{}, NopChannelProvider{}, nil, nil, nil, nil)
	ctx := context.Background()
	if _, err := svc.ScanDue(ctx, time.Now()); err == nil {
		t.Error("ScanDue 待办查询失败应上抛")
	}
	if _, err := svc.SendNow(ctx, "task-1"); err == nil {
		t.Error("SendNow 待办查询失败应上抛")
	}
	// 渠道读取失败上抛（配置/对象/待办就绪）
	if err := memReminds.Save(ctx, &Remind{ID: "r-1", PID: "model-1", StepIDs: "step-1"}); err != nil {
		t.Fatalf("配置保存失败: %v", err)
	}
	if err := memTargets.Save(ctx, &RemindTarget{ID: "tg-1", PID: "r-1", Enable: 1, Type: RemindTypeAhead, ExecutorID: "-1111"}); err != nil {
		t.Fatalf("对象保存失败: %v", err)
	}
	// 待办失败先于渠道校验；再以正常待办源触发渠道失败
	todo := &MessageExtend{BaseColumns: BaseColumns{ID: "msg-1", TenantID: "dev"}, TaskID: "task-1", TaskStepCode: "step-1", OwnerID: "alice", Title: "标题"}
	memTodos := NewMemTodoSource(NewMemMessageStore(nil))
	if err := memTodos.store.Save(ctx, todo); err != nil {
		t.Fatalf("待办保存失败: %v", err)
	}
	channelSvc := NewRemindService(memReminds, memTargets, memTodos, NopScheduleRuleProvider{}, &failingChannelSource{err: storeErr}, nil, nil, nil, nil)
	if _, err := channelSvc.ScanDue(ctx, time.Now()); err == nil {
		t.Error("渠道读取失败应上抛")
	}
	if _, err := channelSvc.SendNow(ctx, "task-1"); err == nil {
		t.Error("渠道读取失败应上抛（手动催办）")
	}
}

// failingChannelSource 是渠道读取失败的渠道桩。
type failingChannelSource struct {
	err error // 注入错误
}

// ChannelsOf 恒返回注入错误。
// @param ctx 操作上下文
// @param todo 待办扩展
// @return nil
// @return 注入错误
func (f *failingChannelSource) ChannelsOf(ctx context.Context, todo *MessageExtend) ([]string, error) {
	return nil, f.err
}

// TestRemindServiceDefaultsAndClones 验证缺省装配与克隆边界。
// @param t 测试上下文
func TestRemindServiceDefaultsAndClones(t *testing.T) {
	ctx := context.Background()
	// 缺省装配：nil 规则/渠道/时钟
	svc := NewRemindService(NewMemRemindStore(nil), NewMemRemindTargetStore(nil), nil, nil, nil, nil, nil, nil, nil)
	// 缺省规则提供方可用（Nop）
	if _, _, err := svc.rules.RemindLimitOf(ctx, "step-1"); err != nil {
		t.Errorf("缺省规则提供方不应报错: %v", err)
	}
	// cloneRemind 带对象（MemRemindStore.Get 的克隆循环）
	store := NewMemRemindStore(nil)
	remind := &Remind{ID: "r-1", PID: "model-1", Targets: []*RemindTarget{{ID: "tg-1", PID: "r-1"}}}
	if err := store.Save(ctx, remind); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	got, found, _ := store.Get(ctx, "r-1")
	if !found || len(got.Targets) != 1 || got.Targets[0].ID != "tg-1" {
		t.Fatalf("克隆对象缺失: %v %d", found, len(got.Targets))
	}
	// 原对象变更不影响克隆
	remind.Targets[0].ID = "tg-2"
	if got.Targets[0].ID != "tg-1" {
		t.Error("克隆应为深拷贝")
	}
	// sortReminds 多行排序（乱序创建时间）
	store2 := NewMemRemindStore(nil)
	base := time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC)
	if err := store2.Save(ctx, &Remind{ID: "r-late", PID: "m", CreateTime: base.Add(2 * time.Hour)}); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if err := store2.Save(ctx, &Remind{ID: "r-early", PID: "m", CreateTime: base}); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if err := store2.Save(ctx, &Remind{ID: "r-mid", PID: "m", CreateTime: base.Add(time.Hour)}); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	list, _ := store2.ListActive(ctx)
	if len(list) != 3 || list[0].ID != "r-early" || list[2].ID != "r-late" {
		t.Errorf("排序错误: %v", listIDs(list))
	}
	// sortRemindTargets 多行排序
	targets := NewMemRemindTargetStore(nil)
	if err := targets.Save(ctx, &RemindTarget{ID: "t-late", PID: "r-1", CreateTime: base.Add(2 * time.Hour)}); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if err := targets.Save(ctx, &RemindTarget{ID: "t-early", PID: "r-1", CreateTime: base}); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	tlist, _ := targets.ListByRemind(ctx, "r-1")
	if len(tlist) != 2 || tlist[0].ID != "t-early" {
		t.Errorf("对象排序错误: %v, %v", tlist[0].ID, tlist[1].ID)
	}
}

// listIDs 提取配置 Id 列表（测试辅助）。
// @param rows 配置列表
// @return Id 列表
func listIDs(rows []*Remind) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids
}

// TestRemindServiceExpansionErrors 验证对象展开的异常分支。
// @param t 测试上下文
func TestRemindServiceExpansionErrors(t *testing.T) {
	ctx := context.Background()
	// 成员查询失败 / 空组
	members := &erroringMembers{err: errors.New("role down"), groups: map[string][]api.User{"empty": {}}}
	svc := NewRemindService(nil, nil, nil, nil, nil, nil, members, nil, nil)
	if got := svc.expandRoleMembers(ctx, "role-x"); got != nil {
		t.Errorf("成员查询失败应返回 nil: %v", got)
	}
	if got := svc.expandRoleMembers(ctx, "empty"); got != nil {
		t.Errorf("空组应返回 nil: %v", got)
	}
	// ${initiator}：变量读取失败 / 非字符串值
	vars := &erroringVariables{err: errors.New("var down")}
	svc2 := NewRemindService(nil, nil, nil, nil, nil, nil, nil, vars, nil)
	instance := &RemindInstanceContext{Todo: &MessageExtend{OwnerID: "carl"}}
	if got := svc2.initiatorOf(ctx, instance.Todo); got != "" {
		t.Errorf("变量读取失败应返回空: %q", got)
	}
	vars2 := &erroringVariables{value: 42}
	svc3 := NewRemindService(nil, nil, nil, nil, nil, nil, nil, vars2, nil)
	if got := svc3.initiatorOf(ctx, &MessageExtend{ProcessInstanceID: "pi-1"}); got != "42" {
		t.Errorf("非字符串发起人应格式化: %q", got)
	}
	// 白名单解析失败：跳过
	resolverSvc := NewRemindService(nil, nil, nil, nil, nil, erroringStepResolver{}, nil, nil, nil)
	remind := &Remind{StepIDs: "step-1"}
	if ok := resolverSvc.inWhitelist(ctx, remind, &MessageExtend{TaskStepCode: "step-1"}); ok {
		t.Error("解析失败不应命中白名单")
	}
	// 送达时 initiator 缺失：-1111 对象展开为当前待办人
	notifier := &fakeNotifier{}
	deliverSvc := NewRemindService(nil, nil, nil, nil, nil, nil, nil, &erroringVariables{err: errors.New("down")}, nil)
	hit := &RemindHit{
		Target: &RemindTarget{ID: "tg-1", Enable: 1, ExecutorID: RemindExecutorCurrentHandler},
		Todo:   &MessageExtend{OwnerID: "carl", Title: "标题", TaskReceiptTime: ptrTime(completeFixtureTime)},
		Kind:   RemindKindAhead,
	}
	delivered, err := deliverSvc.Deliver(ctx, hit, notifier)
	if err != nil || delivered != 1 {
		t.Fatalf("送达错误: %d, %v", delivered, err)
	}
	if notifier.notifications[0].ReceiverID != "carl" {
		t.Errorf("接收人错误: %s", notifier.notifications[0].ReceiverID)
	}
}

// ptrTime 返回时间指针（测试辅助）。
// @param t 时间值
// @return 指针
func ptrTime(t time.Time) *time.Time { return &t }

// TestSchedulerDefaultsAndLoop 验证缺省装配与调度循环直跑。
// @param t 测试上下文
func TestSchedulerDefaultsAndLoop(t *testing.T) {
	// 缺省时钟（nil Clock → Real）
	scheduler := NewScheduler(SchedulerConfig{
		Options:    Options{NoHandleAutoComplete: true, TaskTimeoutRemind: true, SendWorkflowRemind: true},
		TodoSource: &failingTodoSource{err: errors.New("down")},
		Rules:      &erroringRuleProvider{autoErr: errors.New("rule down"), limitErr: errors.New("rule down")},
	})
	if scheduler.opts.Clock == nil {
		t.Fatal("缺省时钟未兜底")
	}
	ctx := context.Background()
	// 数据源失败上抛（自动办结/超时提醒）
	if err := scheduler.AutoCompleteTick(ctx); err == nil {
		t.Error("待办源失败应上抛（自动办结）")
	}
	if err := scheduler.TimeoutRemindTick(ctx); err == nil {
		t.Error("待办源失败应上抛（超时提醒）")
	}
	// 缺省送达器/规则空实现可用
	if err := scheduler.notifier.Notify(ctx, &RemindNotification{ReceiverID: "alice"}); err != nil {
		t.Errorf("缺省送达器不应报错: %v", err)
	}
	// 调度循环直跑：无注入触发函数 + 微周期，覆盖 run* 非注入分支与 ticker 分支
	plain := NewScheduler(SchedulerConfig{
		Options:    Options{NoHandleAutoComplete: true, SendWorkflowRemind: true, TaskTimeoutRemind: true, Clock: clock.NewFake(completeFixtureTime)},
		TodoSource: NewMemTodoSource(NewMemMessageStore(clock.NewFake(completeFixtureTime))),
		Rules:      NopScheduleRuleProvider{},
		Remind:     NewRemindService(NewMemRemindStore(nil), NewMemRemindTargetStore(nil), nil, nil, nil, nil, nil, nil, nil),
	})
	plain.autoCompleteInterval = 5 * time.Millisecond
	plain.remindInterval = 5 * time.Millisecond
	plain.zombieInterval = 5 * time.Millisecond
	plain.Start(ctx)
	time.Sleep(60 * time.Millisecond)
	plain.Stop()
	// 循环内各 tick 均为空转（无数据），不报错即通过
}

// TestSchedulerAutoCompleteErrorPaths 验证自动办结的规则异常与缺项分支。
// @param t 测试上下文
func TestSchedulerAutoCompleteErrorPaths(t *testing.T) {
	fake := clock.NewFake(completeFixtureTime)
	todoStore := NewMemMessageStore(fake)
	todos := NewMemTodoSource(todoStore)
	ctx := context.Background()
	// 待办（接收时间 08:00）
	receipt := completeFixtureTime
	todo := &MessageExtend{
		BaseColumns:     BaseColumns{ID: "msg-1", TenantID: "dev", CreateTime: receipt},
		TaskID:          "task-1",
		TaskStepCode:    "step-1",
		OwnerID:         "alice",
		TaskReceiptTime: &receipt,
	}
	if err := todoStore.Save(ctx, todo); err != nil {
		t.Fatalf("待办保存失败: %v", err)
	}
	// 规则读取失败 → 跳过
	errRules := &erroringRuleProvider{autoErr: errors.New("rule down")}
	scheduler := NewScheduler(SchedulerConfig{
		Options:    Options{NoHandleAutoComplete: true, Clock: fake},
		TodoSource: todos,
		Rules:      errRules,
	})
	fake.Set(completeFixtureTime.Add(2 * time.Hour))
	if err := scheduler.AutoCompleteTick(ctx); err != nil {
		t.Fatalf("规则失败 tick 不应报错: %v", err)
	}
	// 规则 AutoComplete=1 但无 TimeLimit → 逾期判定 false → 跳过
	rules := &fakeRuleProvider{limits: map[string]float64{}, autoRule: map[string]AutoCompleteRule{}}
	rules.autoRule["step-1"] = AutoCompleteRule{AutoComplete: 1}
	scheduler.rules = rules
	if err := scheduler.AutoCompleteTick(ctx); err != nil {
		t.Fatalf("无时限 tick 不应报错: %v", err)
	}
	// complete 缺失 → 同意分支直接返回
	rules.autoRule["step-1"] = AutoCompleteRule{AutoComplete: 1, TimeLimit: float64Ptr(10), TimeUnit: "分钟"}
	if err := scheduler.AutoCompleteTick(ctx); err != nil {
		t.Fatalf("缺 complete tick 不应报错: %v", err)
	}
	// 退回分支：BackStepCode 缺失 → 跳过（back 已注入）
	back := &fakeBackExecutor{}
	rules.autoRule["step-1"] = AutoCompleteRule{AutoComplete: 2, TimeLimit: float64Ptr(10), TimeUnit: "分钟", AllowReturn: 1}
	scheduler.back = back
	if err := scheduler.AutoCompleteTick(ctx); err != nil {
		t.Fatalf("缺退回目标 tick 不应报错: %v", err)
	}
	if len(back.calls) != 0 {
		t.Errorf("缺退回目标不应调用退回: %d", len(back.calls))
	}
	// 催办扫描：送达失败 → continue（不阻断）
	memReminds := NewMemRemindStore(fake)
	memTargets := NewMemRemindTargetStore(fake)
	if err := memReminds.Save(ctx, &Remind{ID: "r-1", PID: "m", StepIDs: "step-1"}); err != nil {
		t.Fatalf("配置保存失败: %v", err)
	}
	if err := memTargets.Save(ctx, &RemindTarget{ID: "tg-1", PID: "r-1", Enable: 1, Type: RemindTypeAhead, Ahead: 5, TimeUnit: "分钟", ExecutorID: "alice"}); err != nil {
		t.Fatalf("对象保存失败: %v", err)
	}
	remindSvc := NewRemindService(memReminds, memTargets, todos, NopScheduleRuleProvider{}, NopChannelProvider{}, nil, nil, nil, fake)
	scheduler2 := NewScheduler(SchedulerConfig{
		Options:    Options{SendWorkflowRemind: true, Clock: fake},
		TodoSource: todos,
		Remind:     remindSvc,
		Notifier:   &fakeNotifier{fail: true},
	})
	if err := scheduler2.RemindScanTick(ctx); err != nil {
		t.Fatalf("送达失败 tick 不应报错: %v", err)
	}
}

// TestNextActivityRequestOfMerge 验证预存请求聚合（同节点合并与已消耗跳过）。
// @param t 测试上下文
func TestNextActivityRequestOfMerge(t *testing.T) {
	records := []*NextActivity{
		{NextActivityID: "handle", IdentityID: "bob", Used: 0},
		{NextActivityID: "handle", IdentityID: "carl", Used: 0},
		{NextActivityID: "handle", IdentityID: "ghost", Used: 1},
		{NextActivityID: "review", IdentityID: "helper", Used: 0},
	}
	request := nextActivityRequestOf(records)
	if len(request.NextActivityList) != 2 {
		t.Fatalf("节点数错误: %d", len(request.NextActivityList))
	}
	// 同节点合并 + 已消耗跳过
	ids := request.NextActivityList[0].ExecutorIDs
	if len(ids) != 2 || ids[0] != "bob" || ids[1] != "carl" {
		t.Errorf("合并结果错误: %v", ids)
	}
	if request.NextActivityList[1].ActivityID != "review" {
		t.Errorf("第二节点错误: %s", request.NextActivityList[1].ActivityID)
	}
}
