// schedulers_test.go 验证调度器：每日时刻计算、自动办结端到端（发起→超时→自动完成→下一待办）、
// 退回型跳过、超时提醒、催办扫描联动、僵尸检测差集、开关关闭不触发与 Start/Stop 循环。
package workflow

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
)

// fakeBackExecutor 是自动办结退回执行器桩（记录调用）。
type fakeBackExecutor struct {
	calls []*api.BackTaskVo // 退回调用记录
	fail  bool              // 是否失败
}

// BackToStepTask 记录退回调用。
// @param ctx 操作上下文
// @param vo 退回请求
// @return 错误
func (f *fakeBackExecutor) BackToStepTask(ctx context.Context, vo *api.BackTaskVo) error {
	if f.fail {
		return errors.New("back failed")
	}
	f.calls = append(f.calls, vo)
	return nil
}

// newSchedulerFixture 构造调度器测试环境（复用办结语料与全套域服务）。
// @param t 测试上下文
// @return 办结测试环境
// @return 调度器
// @return 规则桩
// @return 送达器桩
// @return 退回执行器桩
func newSchedulerFixture(t *testing.T) (*completeFixture, *Scheduler, *fakeRuleProvider, *fakeNotifier, *fakeBackExecutor) {
	t.Helper()
	fx := newCompleteFixture(t)
	todos := NewMemTodoSource(fx.stores.Message)
	rules := &fakeRuleProvider{limits: map[string]float64{}, autoRule: map[string]AutoCompleteRule{}}
	notifier := &fakeNotifier{}
	back := &fakeBackExecutor{}
	exceptions := NewExceptionService(NewMemTaskExceptionStore(fx.fakeClock), fx.fakeClock)
	remind := NewRemindService(NewMemRemindStore(fx.fakeClock), NewMemRemindTargetStore(fx.fakeClock), todos, rules, nil, nil, nil, nil, fx.fakeClock)
	scheduler := NewScheduler(SchedulerConfig{
		Engine:     fx.engine,
		Options:    Options{NoHandleAutoComplete: true, TaskTimeoutRemind: true, SendWorkflowRemind: true, Clock: fx.fakeClock},
		TodoSource: todos,
		Rules:      rules,
		Complete:   fx.complete,
		Next:       fx.next,
		Back:       back,
		Remind:     remind,
		Notifier:   notifier,
		Exceptions: exceptions,
	})
	return fx, scheduler, rules, notifier, back
}

// TestNextDailyRun 验证每日时刻的下次触发计算。
// @param t 测试上下文
func TestNextDailyRun(t *testing.T) {
	// 07:00 → 当日 08:00
	now := time.Date(2026, 10, 31, 7, 0, 0, 0, time.UTC)
	next := nextDailyRun(now, 8)
	if !next.Equal(time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("08:00 前应排当日: %v", next)
	}
	// 09:00 → 次日 08:00
	now = time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	next = nextDailyRun(now, 8)
	if !next.Equal(time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("08:00 后应排次日: %v", next)
	}
	// 恰好 08:00 → 次日 08:00（避免 0 间隔忙转）
	now = time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC)
	next = nextDailyRun(now, 8)
	if !next.Equal(time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("恰等应排次日: %v", next)
	}
}

// TestAutoCompleteTickDisabled 验证开关关闭不触发。
// @param t 测试上下文
func TestAutoCompleteTickDisabled(t *testing.T) {
	fx, scheduler, _, _, _ := newSchedulerFixture(t)
	instance, task := fx.start(t, "flowComplete", "biz-ac-off", "alice")
	fx.seedFirstTodo(t, instance, task)
	_ = task
	scheduler.opts.NoHandleAutoComplete = false
	// 开关关闭：tick 直接返回且无副作用
	if err := scheduler.AutoCompleteTick(context.Background()); err != nil {
		t.Fatalf("关闭态 tick 不应报错: %v", err)
	}
}

// firstInstanceOf 按业务键取首个实例（测试辅助）。
// @param t 测试上下文
// @param businessKey 业务键
// @return 流程实例
func (f *completeFixture) firstInstanceOf(t *testing.T, businessKey string) api.ProcessInstance {
	t.Helper()
	page, err := f.engine.RuntimeService().QueryProcessInstances(context.Background(), api.ProcessInstanceFilter{BusinessKey: businessKey, Size: 1})
	if err != nil || len(page.Data) == 0 {
		t.Fatalf("查询实例失败: %v", err)
	}
	return page.Data[0]
}

// TestAutoCompleteTickAgreeEndToEnd 验证自动办结端到端：发起→预存下一步→超时→tick→下一待办生成。
// @param t 测试上下文
func TestAutoCompleteTickAgreeEndToEnd(t *testing.T) {
	fx, scheduler, rules, _, _ := newSchedulerFixture(t)
	ctx := context.Background()
	instance, task := fx.start(t, "flowComplete", "biz-ac-agree", "alice")
	fx.seedFirstTodo(t, instance, task)
	// 预存下一步（handle → bob），模拟"同意方向已预存"
	if err := fx.next.SaveAgreeNext(ctx, NextActivitySaveRequest{
		ProcessInstanceID: instance.ID,
		ActivityID:        "handle",
		IdentityIDs:       []string{"bob"},
		PreTaskID:         task.ID,
		SendIdentityID:    "alice",
		TenantID:          instance.TenantID,
		Creator:           "alice",
	}); err != nil {
		t.Fatalf("预存失败: %v", err)
	}
	// 注册执行人重写钩子（新任务 assignee 按预存记录改写为 bob，
	// 与生产装配 Service.RegisterExecutorRewriteHook 等价）
	hook := NewExecutorRewriteHook(NewModelMetaService(fx.engine), fx.next)
	hook.Register(fx.engine)
	if err := hook.WarmupAll(ctx); err != nil {
		t.Fatalf("预热失败: %v", err)
	}
	// 步骤规则：apply 节点 30 分钟自动办结（同意）
	rules.autoRule["apply"] = AutoCompleteRule{AutoComplete: 1, TimeLimit: float64Ptr(30), TimeUnit: "分钟"}
	// 未到期（10 分钟）：不触发
	fx.fakeClock.Set(completeFixtureTime.Add(10 * time.Minute))
	if err := scheduler.AutoCompleteTick(ctx); err != nil {
		t.Fatalf("未到期 tick 失败: %v", err)
	}
	if _, found, _ := fx.messages.GetByTaskID(ctx, task.ID); !found {
		t.Fatal("未到期不应办结")
	}
	// 到期（31 分钟）：自动办结并生成 handle 待办
	fx.fakeClock.Set(completeFixtureTime.Add(31 * time.Minute))
	if err := scheduler.AutoCompleteTick(ctx); err != nil {
		t.Fatalf("到期 tick 失败: %v", err)
	}
	// 原待办已删除
	if _, found, _ := fx.messages.GetByTaskID(ctx, task.ID); found {
		t.Error("办结后原待办应删除")
	}
	// 新待办（handle → bob）
	page, err := fx.engine.TaskService().ListTasks(ctx, api.TaskFilter{ProcessInstanceID: instance.ID, Size: 10})
	if err != nil || len(page.Data) != 1 || page.Data[0].TaskDefinitionKey != "handle" {
		t.Fatalf("下一任务未生成: %v, %d", err, len(page.Data))
	}
	nextTodo, found, err := fx.messages.GetByTaskID(ctx, page.Data[0].ID)
	if err != nil || !found {
		t.Fatalf("下一待办未生成: %v, %v", found, err)
	}
	if nextTodo.OwnerID != "bob" {
		t.Errorf("下一待办处理人错误: %s", nextTodo.OwnerID)
	}
	// 自动办结留痕（操作记录"同意（自动办结）"type=SP）
	ops, err := fx.stores.Operation.ListByTask(ctx, task.ID)
	if err != nil || len(ops) == 0 {
		t.Fatalf("操作记录缺失: %d, %v", len(ops), err)
	}
	lastOp := ops[len(ops)-1]
	if lastOp.Type != CommentTypeSP || !strings.Contains(lastOp.Message, AutoCompleteAgreeMessage) {
		t.Errorf("自动办结留痕错误: type=%s msg=%s", lastOp.Type, lastOp.Message)
	}
}

// float64Ptr 返回浮点指针（测试辅助）。
// @param v 浮点值
// @return 指针
func float64Ptr(v float64) *float64 { return &v }

// TestAutoCompleteTickSkips 验证自动办结的跳过分支：无预存/未配置规则/退回未接线/退回执行。
// @param t 测试上下文
func TestAutoCompleteTickSkips(t *testing.T) {
	fx, scheduler, rules, _, back := newSchedulerFixture(t)
	ctx := context.Background()
	// 场景 1：无预存记录（同意型跳过，P5 推演缺口）
	instance, task := fx.start(t, "flowComplete", "biz-ac-skip1", "alice")
	fx.seedFirstTodo(t, instance, task)
	rules.autoRule["apply"] = AutoCompleteRule{AutoComplete: 1, TimeLimit: float64Ptr(30), TimeUnit: "分钟"}
	fx.fakeClock.Set(completeFixtureTime.Add(31 * time.Minute))
	if err := scheduler.AutoCompleteTick(ctx); err != nil {
		t.Fatalf("无预存 tick 失败: %v", err)
	}
	if _, found, _ := fx.messages.GetByTaskID(ctx, task.ID); !found {
		t.Error("无预存不应办结")
	}
	// 场景 2：规则未配置（AutoComplete=0）
	rules.autoRule["apply"] = AutoCompleteRule{}
	if err := scheduler.AutoCompleteTick(ctx); err != nil {
		t.Fatalf("未配置规则 tick 失败: %v", err)
	}
	// 场景 3：退回型但 allowReturn=0 → 跳过
	rules.autoRule["apply"] = AutoCompleteRule{AutoComplete: 2, TimeLimit: float64Ptr(30), TimeUnit: "分钟", AllowReturn: 0}
	if err := scheduler.AutoCompleteTick(ctx); err != nil {
		t.Fatalf("禁止退回 tick 失败: %v", err)
	}
	if len(back.calls) != 0 {
		t.Errorf("禁止退回不应调用退回: %d", len(back.calls))
	}
	// 场景 4：退回型允许退回但无退回执行器（nil 跳过）
	scheduler.back = nil
	rules.autoRule["apply"] = AutoCompleteRule{AutoComplete: 2, TimeLimit: float64Ptr(30), TimeUnit: "分钟", AllowReturn: 1}
	if err := scheduler.AutoCompleteTick(ctx); err != nil {
		t.Fatalf("无退回执行器 tick 失败: %v", err)
	}
	// 场景 5：退回型允许退回 + 执行器 + 退回目标预存 → 调用退回
	scheduler.back = back
	// 预存退回目标（apply → bob 的未消耗记录，即退回到 apply 节点给 bob 处理）
	if err := fx.next.SaveAgreeNext(ctx, NextActivitySaveRequest{
		ProcessInstanceID: instance.ID,
		ActivityID:        "apply",
		IdentityIDs:       []string{"bob"},
		PreTaskID:         task.ID,
		SendIdentityID:    "alice",
		TenantID:          instance.TenantID,
		Creator:           "alice",
	}); err != nil {
		t.Fatalf("预存失败: %v", err)
	}
	rules.autoRule["apply"] = AutoCompleteRule{AutoComplete: 2, TimeLimit: float64Ptr(30), TimeUnit: "分钟", AllowReturn: 1, BackStepCode: "apply"}
	if err := scheduler.AutoCompleteTick(ctx); err != nil {
		t.Fatalf("退回 tick 失败: %v", err)
	}
	if len(back.calls) != 1 {
		t.Fatalf("退回应被调用: %d", len(back.calls))
	}
	if back.calls[0].TargetActivityID != "apply" || back.calls[0].Message != AutoCompleteReturnMessage {
		t.Errorf("退回参数错误: target=%s msg=%s", back.calls[0].TargetActivityID, back.calls[0].Message)
	}
	// 场景 6：退回目标无预存 → 跳过（不调用）
	rules.autoRule["apply"] = AutoCompleteRule{AutoComplete: 2, TimeLimit: float64Ptr(30), TimeUnit: "分钟", AllowReturn: 1, BackStepCode: "no-such-node"}
	if err := scheduler.AutoCompleteTick(ctx); err != nil {
		t.Fatalf("无目标预存 tick 失败: %v", err)
	}
	if len(back.calls) != 1 {
		t.Errorf("无目标预存不应再次调用: %d", len(back.calls))
	}
}

// TestTimeoutRemindTick 验证超时提醒（开关/超时判定/未超时跳过）。
// @param t 测试上下文
func TestTimeoutRemindTick(t *testing.T) {
	fx, scheduler, rules, notifier, _ := newSchedulerFixture(t)
	ctx := context.Background()
	// 开关关闭不触发
	scheduler.opts.TaskTimeoutRemind = false
	if err := scheduler.TimeoutRemindTick(ctx); err != nil {
		t.Fatalf("关闭态 tick 失败: %v", err)
	}
	if len(notifier.notifications) != 0 {
		t.Errorf("关闭态不应提醒: %d", len(notifier.notifications))
	}
	scheduler.opts.TaskTimeoutRemind = true
	// 无时限配置：不提醒
	instance, task := fx.start(t, "flowComplete", "biz-timeout-1", "alice")
	fx.seedFirstTodo(t, instance, task)
	fx.fakeClock.Set(completeFixtureTime.Add(24 * time.Hour))
	if err := scheduler.TimeoutRemindTick(ctx); err != nil {
		t.Fatalf("无时限 tick 失败: %v", err)
	}
	if len(notifier.notifications) != 0 {
		t.Errorf("无时限不应提醒: %d", len(notifier.notifications))
	}
	// 有时限且超时：提醒
	rules.limits["apply"] = 60 // 60 分钟
	fx.fakeClock.Set(completeFixtureTime.Add(2 * time.Hour))
	if err := scheduler.TimeoutRemindTick(ctx); err != nil {
		t.Fatalf("超时 tick 失败: %v", err)
	}
	if len(notifier.notifications) != 1 {
		t.Fatalf("超时应提醒: %d", len(notifier.notifications))
	}
	if notifier.notifications[0].Kind != RemindKindTimeout || !strings.Contains(notifier.notifications[0].Content, "已超时") {
		t.Errorf("提醒内容错误: %+v", notifier.notifications[0])
	}
	_ = task
	// 未超时：不提醒（新实例新待办）
	instance2, task2 := fx.start(t, "flowComplete", "biz-timeout-2", "alice")
	fx.seedFirstTodo(t, instance2, task2)
	fx.fakeClock.Set(completeFixtureTime.Add(30 * time.Minute))
	if err := scheduler.TimeoutRemindTick(ctx); err != nil {
		t.Fatalf("未超时 tick 失败: %v", err)
	}
	if len(notifier.notifications) != 1 {
		t.Errorf("未超时不应提醒: %d", len(notifier.notifications))
	}
}

// TestRemindScanTick 验证催办扫描联动（开关/到期命中→送达→标记）。
// @param t 测试上下文
func TestRemindScanTick(t *testing.T) {
	fx, scheduler, rules, notifier, _ := newSchedulerFixture(t)
	ctx := context.Background()
	// 开关关闭不触发
	scheduler.opts.SendWorkflowRemind = false
	if err := scheduler.RemindScanTick(ctx); err != nil {
		t.Fatalf("关闭态 tick 失败: %v", err)
	}
	scheduler.opts.SendWorkflowRemind = true
	// 播种待办与催办配置（-1111 当前处理人，预警提前 5 分钟，时限 10 分钟）
	instance, task := fx.start(t, "flowComplete", "biz-remind-1", "alice")
	fx.seedFirstTodo(t, instance, task)
	rules.limits["apply"] = 10
	reminds := NewMemRemindStore(fx.fakeClock)
	targets := NewMemRemindTargetStore(fx.fakeClock)
	if _, err := NewRemindService(reminds, targets, scheduler.todoSource, rules, nil, nil, nil, nil, fx.fakeClock).Add(ctx, RemindSaveRequest{
		Remind: &Remind{PID: "model-1", StepIDs: "apply", TenantID: "dev"},
		Targets: []*RemindTarget{{ExecutorID: RemindExecutorCurrentHandler, Type: RemindTypeAhead, Ahead: 5, TimeUnit: "分钟"}},
	}); err != nil {
		t.Fatalf("催办配置失败: %v", err)
	}
	// 调度器的催办服务使用独立存储，替换为该服务后重跑
	scheduler.remind = NewRemindService(reminds, targets, scheduler.todoSource, rules, nil, nil, nil, nil, fx.fakeClock)
	// 未到期（接收+5min 到期线 08:05；当前 08:02）
	fx.fakeClock.Set(completeFixtureTime.Add(2 * time.Minute))
	if err := scheduler.RemindScanTick(ctx); err != nil {
		t.Fatalf("未到期 tick 失败: %v", err)
	}
	if len(notifier.notifications) != 0 {
		t.Errorf("未到期不应提醒: %d", len(notifier.notifications))
	}
	// 到期（08:06）
	fx.fakeClock.Set(completeFixtureTime.Add(6 * time.Minute))
	if err := scheduler.RemindScanTick(ctx); err != nil {
		t.Fatalf("到期 tick 失败: %v", err)
	}
	if len(notifier.notifications) != 1 {
		t.Fatalf("到期应送达: %d", len(notifier.notifications))
	}
	if notifier.notifications[0].ReceiverID != "alice" {
		t.Errorf("接收人错误: %s", notifier.notifications[0].ReceiverID)
	}
	// 标记已发送：再次扫描不重复发送（未启用重试）
	if err := scheduler.RemindScanTick(ctx); err != nil {
		t.Fatalf("重复扫描失败: %v", err)
	}
	if len(notifier.notifications) != 1 {
		t.Errorf("已发送不应重复: %d", len(notifier.notifications))
	}
}

// TestZombieScanTick 验证僵尸流程检测差集。
// @param t 测试上下文
func TestZombieScanTick(t *testing.T) {
	fx, scheduler, _, _, _ := newSchedulerFixture(t)
	ctx := context.Background()
	// 无实例：tick 直接返回
	if err := scheduler.ZombieScanTick(ctx); err != nil {
		t.Fatalf("空实例 tick 失败: %v", err)
	}
	// 场景 1：有待办的实例不记异常
	instance, task := fx.start(t, "flowComplete", "biz-zombie-1", "alice")
	fx.seedFirstTodo(t, instance, task)
	if err := scheduler.ZombieScanTick(ctx); err != nil {
		t.Fatalf("有待办 tick 失败: %v", err)
	}
	// 场景 2：无待办的实例（种子数据删除待办模拟监听失败）
	instance2, task2 := fx.start(t, "flowComplete", "biz-zombie-2", "alice")
	fx.seedFirstTodo(t, instance2, task2)
	if err := fx.messages.DeleteByTaskID(ctx, task2.ID); err != nil {
		t.Fatalf("删除待办失败: %v", err)
	}
	if err := scheduler.ZombieScanTick(ctx); err != nil {
		t.Fatalf("无待办 tick 失败: %v", err)
	}
	// 异常记录：仅僵尸实例一条
	list, err := scheduler.exceptions.ListUnhandled(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("异常记录错误: %d, %v", len(list), err)
	}
	if list[0].ProcessInstanceID != instance2.ID || list[0].ExceptionType != ExceptionTypeNotTaskMessage {
		t.Errorf("僵尸记录错误: instance=%s type=%s", list[0].ProcessInstanceID, list[0].ExceptionType)
	}
	if list[0].Message != "流程未生成待办" || list[0].ProcessDefinitionKey != "flowComplete" {
		t.Errorf("僵尸记录内容错误: msg=%s key=%s", list[0].Message, list[0].ProcessDefinitionKey)
	}
	// 场景 3：重复扫描不重复落库
	if err := scheduler.ZombieScanTick(ctx); err != nil {
		t.Fatalf("重复扫描失败: %v", err)
	}
	if list, _ := scheduler.exceptions.ListUnhandled(ctx); len(list) != 1 {
		t.Errorf("重复扫描不应重复落库: %d", len(list))
	}
	// 场景 4：异常服务缺失不报错
	scheduler.exceptions = nil
	if err := scheduler.ZombieScanTick(ctx); err != nil {
		t.Fatalf("异常服务缺失 tick 失败: %v", err)
	}
}

// TestSchedulerTriggerFunctions 验证注入触发函数与 Start/Stop 循环。
// @param t 测试上下文
func TestSchedulerTriggerFunctions(t *testing.T) {
	fx, scheduler, _, _, _ := newSchedulerFixture(t)
	var autoCompleteCalls, timeoutCalls, remindCalls, zombieCalls atomic.Int64
	scheduler.AutoCompleteFn = func(ctx context.Context) error { autoCompleteCalls.Add(1); return nil }
	scheduler.TimeoutRemindFn = func(ctx context.Context) error { timeoutCalls.Add(1); return nil }
	scheduler.RemindScanFn = func(ctx context.Context) error { remindCalls.Add(1); return nil }
	scheduler.ZombieScanFn = func(ctx context.Context) error { zombieCalls.Add(1); return nil }
	// 缩短周期（直改字段，同包测试）
	scheduler.autoCompleteInterval = 20 * time.Millisecond
	scheduler.remindInterval = 20 * time.Millisecond
	scheduler.zombieInterval = 20 * time.Millisecond
	// Start → 等待触发 → Stop
	scheduler.Start(context.Background())
	time.Sleep(120 * time.Millisecond)
	scheduler.Stop()
	// Stop 幂等
	scheduler.Stop()
	if autoCompleteCalls.Load() == 0 || timeoutCalls.Load() != 0 || remindCalls.Load() == 0 || zombieCalls.Load() == 0 {
		t.Errorf("触发计数错误: ac=%d timeout=%d remind=%d zombie=%d",
			autoCompleteCalls.Load(), timeoutCalls.Load(), remindCalls.Load(), zombieCalls.Load())
	}
	// 超时提醒未到 8 点不触发（假时钟 08:00 整排次日，真实计时器在测试窗口内不触发）
	_ = fx
	// 重复 Start 幂等（已 Stop 后允许再启动一轮）
	scheduler.Start(context.Background())
	time.Sleep(40 * time.Millisecond)
	scheduler.Stop()
}

// TestSchedulerTickDependenciesMissing 验证依赖缺失时各 tick 不报错。
// @param t 测试上下文
func TestSchedulerTickDependenciesMissing(t *testing.T) {
	scheduler := NewScheduler(SchedulerConfig{
		Options: Options{NoHandleAutoComplete: true, TaskTimeoutRemind: true, SendWorkflowRemind: true, Clock: clock.NewFake(completeFixtureTime)},
	})
	ctx := context.Background()
	if err := scheduler.AutoCompleteTick(ctx); err != nil {
		t.Errorf("缺数据源自动办结 tick 不应报错: %v", err)
	}
	if err := scheduler.TimeoutRemindTick(ctx); err != nil {
		t.Errorf("缺数据源超时提醒 tick 不应报错: %v", err)
	}
	if err := scheduler.RemindScanTick(ctx); err != nil {
		t.Errorf("缺催办服务 tick 不应报错: %v", err)
	}
	if err := scheduler.ZombieScanTick(ctx); err != nil {
		t.Errorf("缺引擎/异常服务 tick 不应报错: %v", err)
	}
	// 注入规则桩后 overdue 分支：时限未配置/接收时间缺失均不触发
	scheduler.todoSource = NewMemTodoSource(NewMemMessageStore(clock.NewFake(completeFixtureTime)))
	scheduler.AutoCompleteTick(ctx)
	scheduler.TimeoutRemindTick(ctx)
}
