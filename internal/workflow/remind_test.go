// remind_test.go 验证催办域服务：对象展开（-1111/${initiator}/角色/字面量）、预警/延时到期
// 边界（恰等/越线）、步骤白名单过滤、重试计数、手动催办与 SQL 适配。
package workflow

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
)

// fakeRuleProvider 是步骤规则提供方桩（可注入催办时限与自动办结规则）。
type fakeRuleProvider struct {
	limits   map[string]float64         // 步骤代号 → 催办时限（分钟）
	autoRule map[string]AutoCompleteRule // 步骤代号 → 自动办结规则
}

// RemindLimitOf 返回步骤催办时限（分钟，单位固定"分钟"）。
// @param ctx 操作上下文
// @param stepCode 步骤代号
// @return 时限指针，未配置返回 nil
// @return 单位
// @return 错误
func (f *fakeRuleProvider) RemindLimitOf(ctx context.Context, stepCode string) (*float64, string, error) {
	if limit, ok := f.limits[stepCode]; ok {
		value := limit
		return &value, "分钟", nil
	}
	return nil, "", nil
}

// AutoCompleteRuleOf 返回步骤自动办结规则。
// @param ctx 操作上下文
// @param stepCode 步骤代号
// @return 规则
// @return 错误
func (f *fakeRuleProvider) AutoCompleteRuleOf(ctx context.Context, stepCode string) (AutoCompleteRule, error) {
	return f.autoRule[stepCode], nil
}

// fakeChannelProvider 是渠道来源桩（返回固定渠道）。
type fakeChannelProvider struct {
	channels []string // 固定渠道
}

// ChannelsOf 返回固定渠道。
// @param ctx 操作上下文
// @param todo 待办扩展
// @return 渠道列表
// @return 错误
func (f *fakeChannelProvider) ChannelsOf(ctx context.Context, todo *MessageExtend) ([]string, error) {
	return f.channels, nil
}

// fakeMembers 是角色成员查询桩。
type fakeMembers struct {
	groups map[string][]api.User // 组 Id → 成员
}

// ListMembers 返回组成员。
// @param ctx 操作上下文
// @param groupID 组 Id
// @return 成员列表
// @return 错误
func (f *fakeMembers) ListMembers(ctx context.Context, groupID string) ([]api.User, error) {
	return f.groups[groupID], nil
}

// fakeVariables 是流程变量读取桩。
type fakeVariables struct {
	vars map[string]map[string]any // 实例 Id → 变量
}

// GetVariable 返回实例变量。
// @param ctx 操作上下文
// @param processInstanceID 实例 Id
// @param name 变量名
// @return 变量值
// @return 错误
func (f *fakeVariables) GetVariable(ctx context.Context, processInstanceID, name string) (any, error) {
	instance, ok := f.vars[processInstanceID]
	if !ok {
		return nil, api.NewError("FAKE-101", "实例不存在", api.ErrNotFound)
	}
	value, ok := instance[name]
	if !ok {
		return nil, api.NewError("FAKE-102", "变量不存在", api.ErrNotFound)
	}
	return value, nil
}

// fakeNotifier 是送达器桩（记录通知并可控失败）。
type fakeNotifier struct {
	notifications []*RemindNotification // 已送达通知
	fail          bool                  // 是否失败
}

// Notify 记录通知。
// @param ctx 操作上下文
// @param notification 通知报文
// @return 错误
func (f *fakeNotifier) Notify(ctx context.Context, notification *RemindNotification) error {
	if f.fail {
		return api.NewError("FAKE-103", "送达失败", nil)
	}
	f.notifications = append(f.notifications, notification)
	return nil
}

// newRemindFixture 构造催办域服务测试环境。
// @param t 测试上下文
// @return 域服务
// @return 配置存储
// @return 对象存储
// @return 待办数据源
// @return 规则桩
// @return 送达器桩
// @return 假时钟
func newRemindFixture(t *testing.T) (*RemindService, *MemRemindStore, *MemRemindTargetStore, *MemTodoSource, *fakeRuleProvider, *fakeNotifier, *clock.Fake) {
	t.Helper()
	fake := clock.NewFake(completeFixtureTime)
	reminds := NewMemRemindStore(fake)
	targets := NewMemRemindTargetStore(fake)
	todos := NewMemTodoSource(NewMemMessageStore(fake))
	rules := &fakeRuleProvider{limits: map[string]float64{}, autoRule: map[string]AutoCompleteRule{}}
	notifier := &fakeNotifier{}
	svc := NewRemindService(reminds, targets, todos, rules, &fakeChannelProvider{channels: []string{"email"}}, nil, &fakeMembers{groups: map[string][]api.User{
		"role-1": {{ID: "alice"}, {ID: "bob"}},
	}}, &fakeVariables{vars: map[string]map[string]any{"pi-1": {VarInitiator: "alice"}}}, fake)
	return svc, reminds, targets, todos, rules, notifier, fake
}

// seedTodo 播种一条待办（接收时间为假时钟基准）。
// @param t 测试上下文
// @param todos 待办数据源
// @param store 待办内存存储
// @param taskID 任务 Id
// @param stepCode 步骤代号
// @param owner 处理人
// @return 待办扩展
func seedTodo(t *testing.T, store *MemMessageStore, taskID, stepCode, owner string) *MessageExtend {
	t.Helper()
	receipt := completeFixtureTime
	todo := &MessageExtend{
		BaseColumns:       BaseColumns{ID: "msg-" + taskID, TenantID: "dev", CreateTime: receipt, UpdateTime: receipt},
		TaskID:            taskID,
		ProcessInstanceID: "pi-1",
		Title:             "标题-" + taskID,
		TaskStepCode:      stepCode,
		TaskStepName:      "步骤-" + stepCode,
		FlowCategoryName:  "办结测试流程",
		OwnerID:           owner,
		TaskReceiptTime:   &receipt,
	}
	if err := store.Save(context.Background(), todo); err != nil {
		t.Fatalf("播种待办失败: %v", err)
	}
	return todo
}

// TestDurationMillis 验证时长毫秒换算（对位 Java getDuration）。
// @param t 测试上下文
func TestDurationMillis(t *testing.T) {
	if got := durationMillis(1, "天"); got != 24*60*60*1000 {
		t.Errorf("天换算错误: %d", got)
	}
	if got := durationMillis(2, "DAY"); got != 2*24*60*60*1000 {
		t.Errorf("day 换算错误: %d", got)
	}
	if got := durationMillis(1, "小时"); got != 60*60*1000 {
		t.Errorf("小时换算错误: %d", got)
	}
	if got := durationMillis(2, "Hour"); got != 2*60*60*1000 {
		t.Errorf("hour 换算错误: %d", got)
	}
	if got := durationMillis(3, "分钟"); got != 3*60*1000 {
		t.Errorf("分钟换算错误: %d", got)
	}
	if got := durationMillis(3, "Minute"); got != 3*60*1000 {
		t.Errorf("minute 换算错误: %d", got)
	}
	// 未知单位按 0（Java null → 0 静默语义）
	if got := durationMillis(3, "周"); got != 0 {
		t.Errorf("未知单位应为 0: %d", got)
	}
}

// TestWhitelistContains 验证白名单精确分段匹配（修复 Java 子串误命中）。
// @param t 测试上下文
func TestWhitelistContains(t *testing.T) {
	if !whitelistContains("s1,s2,s3", "s2") {
		t.Error("s2 应在白名单")
	}
	// Java contains 子串误命中场景：id=11 不应误命中 110
	if whitelistContains("110", "11") {
		t.Error("11 不应误命中 110（精确匹配）")
	}
	if whitelistContains("", "s1") {
		t.Error("空白名单不命中")
	}
	if whitelistContains("s1,s2", "") {
		t.Error("空目标不命中")
	}
}

// TestRemindServiceAddDefaults 验证 Add 的默认对象生成与对象保存。
// @param t 测试上下文
func TestRemindServiceAddDefaults(t *testing.T) {
	svc, reminds, targets, _, _, _, _ := newRemindFixture(t)
	ctx := context.Background()
	// 空 pid 报错
	if _, err := svc.Add(ctx, RemindSaveRequest{Remind: &Remind{}}); err == nil {
		t.Error("缺模型 Id 应报错")
	}
	// 空请求
	if _, err := svc.Add(ctx, RemindSaveRequest{}); err == nil {
		t.Error("空配置应报错")
	}
	// 默认 -1111 预警行
	remind, err := svc.Add(ctx, RemindSaveRequest{Remind: &Remind{PID: "model-1", StepIDs: "step-1", TenantID: "dev"}})
	if err != nil {
		t.Fatalf("新增失败: %v", err)
	}
	if remind.ID == "" || remind.Deleted != 0 {
		t.Errorf("配置主键/删除标记错误: %q %d", remind.ID, remind.Deleted)
	}
	list, err := targets.ListByRemind(ctx, remind.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("默认对象缺失: %d, %v", len(list), err)
	}
	if list[0].ExecutorID != RemindExecutorCurrentHandler || list[0].Type != RemindTypeAhead || list[0].Enable != 1 {
		t.Errorf("默认对象应为 -1111 预警启用行: %+v", list[0])
	}
	// 显式对象列表
	remind2, err := svc.Add(ctx, RemindSaveRequest{
		Remind: &Remind{PID: "model-1", StepIDs: "step-1", EnablePre: 1, EnablePost: 1, TenantID: "dev"},
		Targets: []*RemindTarget{
			{ExecutorID: RemindExecutorInitiator, Name: "发起人", Type: RemindTypeAhead, Ahead: 10, TimeUnit: "分钟"},
			{ExecutorID: "role-1", Name: "审批角色", TargetType: RemindTargetTypeRole, Type: RemindTypeDelay, Delay: 5, TimeUnit: "小时"},
		},
	})
	if err != nil {
		t.Fatalf("新增失败: %v", err)
	}
	list2, _ := targets.ListByRemind(ctx, remind2.ID)
	if len(list2) != 2 {
		t.Fatalf("显式对象应为 2 条: %d", len(list2))
	}
	// 配置查询（级联）
	got, found, err := svc.Get(ctx, remind2.ID)
	if err != nil || !found || len(got.Targets) != 2 {
		t.Fatalf("配置查询错误: %v %v %d", found, err, len(got.Targets))
	}
	// 按模型分页查询
	page, err := svc.List(ctx, "model-1", PageQuery{Start: 0, Size: 1})
	if err != nil || page.Total != 2 || len(page.Data) != 1 {
		t.Fatalf("分页查询错误: %+v, %v", page, err)
	}
	// 空页（越界）
	page2, err := svc.List(ctx, "model-1", PageQuery{Start: 9, Size: 5})
	if err != nil || len(page2.Data) != 0 || page2.Total != 2 {
		t.Fatalf("越界分页错误: %+v, %v", page2, err)
	}
	page3, err := svc.List(ctx, "model-1", PageQuery{Start: 0, Size: 0})
	if err != nil || len(page3.Data) != 0 {
		t.Fatalf("零分页错误: %+v, %v", page3, err)
	}
	_ = reminds
}

// TestRemindServiceUpdateAndRemove 验证配置更新与逻辑删除/批量删除。
// @param t 测试上下文
func TestRemindServiceUpdateAndRemove(t *testing.T) {
	svc, reminds, targets, _, _, _, _ := newRemindFixture(t)
	ctx := context.Background()
	remind, err := svc.Add(ctx, RemindSaveRequest{Remind: &Remind{PID: "model-1", StepIDs: "step-1", TenantID: "dev"}})
	if err != nil {
		t.Fatalf("新增失败: %v", err)
	}
	// 更新不存在
	if _, err := svc.Update(ctx, RemindSaveRequest{Remind: &Remind{ID: "no-such", PID: "model-1"}}); err == nil {
		t.Error("更新不存在应报错")
	}
	// 更新空 Id
	if _, err := svc.Update(ctx, RemindSaveRequest{Remind: &Remind{PID: "model-1"}}); err == nil {
		t.Error("更新空 Id 应报错")
	}
	// 更新主表与对象重建
	updated, err := svc.Update(ctx, RemindSaveRequest{
		Remind:  &Remind{ID: remind.ID, PID: "model-1", StepIDs: "step-2", Updater: "alice"},
		Targets: []*RemindTarget{{ExecutorID: "carl", Type: RemindTypeDelay, Delay: 1, TimeUnit: "天"}},
	})
	if err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if updated.StepIDs != "step-2" {
		t.Errorf("白名单未更新: %q", updated.StepIDs)
	}
	list, _ := targets.ListByRemind(ctx, remind.ID)
	if len(list) != 1 || list[0].ExecutorID != "carl" {
		t.Fatalf("对象重建错误: %+v", list)
	}
	// 允许清空白名单（ReplaceStepIDs）
	cleared, err := svc.Update(ctx, RemindSaveRequest{Remind: &Remind{ID: remind.ID, PID: "model-1"}, ReplaceStepIDs: true})
	if err != nil {
		t.Fatalf("清空失败: %v", err)
	}
	if cleared.StepIDs != "" {
		t.Errorf("白名单应清空: %q", cleared.StepIDs)
	}
	// 逻辑删除与批量删除
	if err := svc.Remove(ctx, remind.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	got, found, _ := reminds.Get(ctx, remind.ID)
	if !found || got.Deleted != 1 {
		t.Errorf("逻辑删除未生效: %v %d", found, got.Deleted)
	}
	if err := svc.RemoveBatch(ctx, []string{"no-such-1", "no-such-2"}); err != nil {
		t.Errorf("批量删除应幂等: %v", err)
	}
}

// TestExpandTargets 验证对象展开（-1111/${initiator}/角色/字面量）。
// @param t 测试上下文
func TestExpandTargets(t *testing.T) {
	svc, _, _, _, _, _, _ := newRemindFixture(t)
	ctx := context.Background()
	instance := &RemindInstanceContext{
		Todo:      &MessageExtend{OwnerID: "carl"},
		Initiator: "alice",
	}
	// -1111 → 当前待办人
	if got := svc.ExpandTargets(ctx, &RemindTarget{Enable: 1, ExecutorID: RemindExecutorCurrentHandler}, instance); len(got) != 1 || got[0] != "carl" {
		t.Errorf("-1111 应展开当前待办人: %v", got)
	}
	// ${initiator} → 发起人
	if got := svc.ExpandTargets(ctx, &RemindTarget{Enable: 1, ExecutorID: RemindExecutorInitiator}, instance); len(got) != 1 || got[0] != "alice" {
		t.Errorf("${initiator} 应展开发起人: %v", got)
	}
	// 角色对象 → 组成员
	if got := svc.ExpandTargets(ctx, &RemindTarget{Enable: 1, ExecutorID: "role-1", TargetType: RemindTargetTypeRole}, instance); len(got) != 2 {
		t.Errorf("角色应展开组成员: %v", got)
	}
	// 字面用户
	if got := svc.ExpandTargets(ctx, &RemindTarget{Enable: 1, ExecutorID: "bob"}, instance); len(got) != 1 || got[0] != "bob" {
		t.Errorf("字面用户应原样返回: %v", got)
	}
	// 禁用对象不展开
	if got := svc.ExpandTargets(ctx, &RemindTarget{Enable: 0, ExecutorID: "bob"}, instance); got != nil {
		t.Errorf("禁用对象不展开: %v", got)
	}
	// 空上下文的 -1111 不展开
	if got := svc.ExpandTargets(ctx, &RemindTarget{Enable: 1, ExecutorID: RemindExecutorCurrentHandler}, &RemindInstanceContext{}); got != nil {
		t.Errorf("缺待办上下文不展开: %v", got)
	}
	// 缺发起人上下文不展开
	if got := svc.ExpandTargets(ctx, &RemindTarget{Enable: 1, ExecutorID: RemindExecutorInitiator}, &RemindInstanceContext{}); got != nil {
		t.Errorf("缺发起人上下文不展开: %v", got)
	}
	// 未知角色展开为空
	if got := svc.ExpandTargets(ctx, &RemindTarget{Enable: 1, ExecutorID: "role-x", TargetType: RemindTargetTypeRole}, instance); got != nil {
		t.Errorf("未知角色展开为空: %v", got)
	}
}

// TestScanDueAheadBoundaries 验证预警型边界（按处理时限：恰等/越线/未到）。
// @param t 测试上下文
func TestScanDueAheadBoundaries(t *testing.T) {
	svc, reminds, targets, todos, rules, _, fake := newRemindFixture(t)
	ctx := context.Background()
	store := todos.store
	rules.limits["step-1"] = 60 // 处理时限 60 分钟
	_ = seedTodo(t, store, "task-1", "step-1", "carl")
	// 预警型：提前量 10 分钟；接收=08:00，到期=09:00；08:50 恰等命中
	remind, err := svc.Add(ctx, RemindSaveRequest{
		Remind: &Remind{PID: "model-1", StepIDs: "step-1", EnablePre: 1, TenantID: "dev"},
		Targets: []*RemindTarget{{ExecutorID: RemindExecutorCurrentHandler, Type: RemindTypeAhead, Ahead: 10, TimeUnit: "分钟"}},
	})
	if err != nil {
		t.Fatalf("配置失败: %v", err)
	}
	_ = remind
	// 未到（08:30 < 08:50）
	fake.Set(completeFixtureTime.Add(30 * time.Minute))
	hits, err := svc.ScanDue(ctx, fake.Now())
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("未到不应命中: %d", len(hits))
	}
	// 恰等（08:50 = 09:00 - 10min）
	fake.Set(completeFixtureTime.Add(50 * time.Minute))
	hits, err = svc.ScanDue(ctx, fake.Now())
	if err != nil || len(hits) != 1 {
		t.Fatalf("恰等应命中: %d, %v", len(hits), err)
	}
	if hits[0].Kind != RemindKindAhead || hits[0].Content == "" {
		t.Errorf("命中内容错误: kind=%q content=%q", hits[0].Kind, hits[0].Content)
	}
	if len(hits[0].Channels) != 1 || hits[0].Channels[0] != "email" {
		t.Errorf("渠道注入错误: %v", hits[0].Channels)
	}
	// 越线（09:30 > 09:00）
	fake.Set(completeFixtureTime.Add(90 * time.Minute))
	hits, _ = svc.ScanDue(ctx, fake.Now())
	if len(hits) != 1 {
		t.Errorf("越线应命中: %d", len(hits))
	}
	// 无时限配置的步骤跳过
	rules.limits = map[string]float64{}
	fake.Set(completeFixtureTime.Add(2 * time.Hour))
	hits, _ = svc.ScanDue(ctx, fake.Now())
	if len(hits) != 0 {
		t.Errorf("无时限不应命中: %d", len(hits))
	}
	_ = reminds
	_ = targets
}

// TestScanDueAheadByPlanTime 验证预警型按计划完成时间判定。
// @param t 测试上下文
func TestScanDueAheadByPlanTime(t *testing.T) {
	svc, _, _, todos, rules, _, fake := newRemindFixture(t)
	ctx := context.Background()
	store := todos.store
	// 不配置步骤时限（计划完成时间判定不依赖步骤规则）
	_ = rules
	todo := seedTodo(t, store, "task-plan", "step-1", "carl")
	plan := completeFixtureTime.Add(30 * time.Minute)
	// seedTodo 存的是克隆行，计划完成时间需重新落库
	todo.PlanFinishTime = &plan
	if err := store.Save(context.Background(), todo); err != nil {
		t.Fatalf("更新计划时间失败: %v", err)
	}
	remind, err := svc.Add(ctx, RemindSaveRequest{
		Remind: &Remind{PID: "model-1", StepIDs: "step-1", TenantID: "dev"},
		Targets: []*RemindTarget{{ExecutorID: RemindExecutorCurrentHandler, Type: RemindTypeAhead, Ahead: 10, TimeUnit: "分钟", ByPlanCompleteTime: 1}},
	})
	if err != nil {
		t.Fatalf("配置失败: %v", err)
	}
	// 未到（计划 08:30，提前 10min → 08:20 才到；当前 08:10）
	fake.Set(completeFixtureTime.Add(10 * time.Minute))
	if hits, _ := svc.ScanDue(ctx, fake.Now()); len(hits) != 0 {
		t.Errorf("计划时间未到不应命中: %d", len(hits))
	}
	// 恰等（08:20 = 08:30 - 10min）
	fake.Set(completeFixtureTime.Add(20 * time.Minute))
	hits, _ := svc.ScanDue(ctx, fake.Now())
	if len(hits) != 1 {
		t.Errorf("计划时间恰等应命中: %d", len(hits))
	}
	// 无计划完成时间不命中（同步落库）
	todo.PlanFinishTime = nil
	if err := store.Save(context.Background(), todo); err != nil {
		t.Fatalf("清空计划时间失败: %v", err)
	}
	hits, _ = svc.ScanDue(ctx, fake.Now())
	if len(hits) != 0 {
		t.Errorf("无计划时间不应命中: %d", len(hits))
	}
	_ = remind
}

// TestScanDueDelayBoundaries 验证延时型三分支（计划时间/处理时限/任务接收时间）。
// @param t 测试上下文
func TestScanDueDelayBoundaries(t *testing.T) {
	svc, _, _, todos, rules, _, fake := newRemindFixture(t)
	ctx := context.Background()
	store := todos.store
	rules.limits["step-1"] = 30 // 处理时限 30 分钟
	todo := seedTodo(t, store, "task-delay", "step-1", "carl")
	// 延时型：延迟量 10 分钟
	remind, err := svc.Add(ctx, RemindSaveRequest{
		Remind: &Remind{PID: "model-1", StepIDs: "step-1", EnablePost: 1, TenantID: "dev"},
		Targets: []*RemindTarget{{ExecutorID: RemindExecutorCurrentHandler, Type: RemindTypeDelay, Delay: 10, TimeUnit: "分钟"}},
	})
	if err != nil {
		t.Fatalf("配置失败: %v", err)
	}
	_ = remind
	// 分支 1（无时限配置）：接收 08:00 + 10min = 08:10 恰等
	rules.limits = map[string]float64{}
	fake.Set(completeFixtureTime.Add(10 * time.Minute))
	hits, _ := svc.ScanDue(ctx, fake.Now())
	if len(hits) != 1 {
		t.Fatalf("接收时间恰等应命中: %d", len(hits))
	}
	if hits[0].Kind != RemindKindDelay {
		t.Errorf("延时类别错误: %q", hits[0].Kind)
	}
	// 分支 2（有时限配置）：due=08:30 + 10min = 08:40
	rules.limits["step-1"] = 30
	fake.Set(completeFixtureTime.Add(40 * time.Minute))
	hits, _ = svc.ScanDue(ctx, fake.Now())
	if len(hits) != 1 {
		t.Fatalf("处理时限恰等应命中: %d", len(hits))
	}
	// 未到（08:20 < 08:40，且接收时间分支 08:20 < 08:10+? 已过 → 不对：08:20 > 08:10 会命中接收分支）
	// 注意：Java 分支顺序为先 due 后接收，due 未过但接收已过时按 Java 语义走接收分支命中。
	// 这里验证 Java 同款行为：08:20 时 due 分支未中、接收分支已过 → 命中。
	fake.Set(completeFixtureTime.Add(20 * time.Minute))
	hits, _ = svc.ScanDue(ctx, fake.Now())
	if len(hits) != 1 {
		t.Fatalf("回退接收时间分支应命中: %d", len(hits))
	}
	// 分支 3：按计划完成时间
	plan := completeFixtureTime.Add(5 * time.Minute)
	// seedTodo 存的是克隆行，计划完成时间需重新落库
	todo.PlanFinishTime = &plan
	if err := store.Save(context.Background(), todo); err != nil {
		t.Fatalf("更新计划时间失败: %v", err)
	}
	remindTarget, err := svc.Add(ctx, RemindSaveRequest{
		Remind: &Remind{PID: "model-1", StepIDs: "step-1", TenantID: "dev"},
		Targets: []*RemindTarget{{ExecutorID: RemindExecutorCurrentHandler, Type: RemindTypeDelay, Delay: 10, TimeUnit: "分钟", ByPlanCompleteTime: 1}},
	})
	if err != nil {
		t.Fatalf("配置失败: %v", err)
	}
	// 计划 08:05 + 10min = 08:15；当前 08:16 命中
	fake.Set(completeFixtureTime.Add(16 * time.Minute))
	hits, _ = svc.ScanDue(ctx, fake.Now())
	if len(hits) < 1 {
		t.Fatalf("计划时间延时分支应命中: %d", len(hits))
	}
	// 无计划完成时间回退其他分支（同步落库）
	todo.PlanFinishTime = nil
	if err := store.Save(context.Background(), todo); err != nil {
		t.Fatalf("清空计划时间失败: %v", err)
	}
	hits, _ = svc.ScanDue(ctx, fake.Now())
	if len(hits) < 1 {
		t.Fatalf("回退分支应命中: %d", len(hits))
	}
	_ = remindTarget
}

// TestScanDueWhitelistAndRetry 验证白名单过滤与重试门槛。
// @param t 测试上下文
func TestScanDueWhitelistAndRetry(t *testing.T) {
	svc, reminds, targets, todos, rules, _, fake := newRemindFixture(t)
	ctx := context.Background()
	store := todos.store
	rules.limits["step-1"] = 10
	seedTodo(t, store, "task-1", "step-1", "carl")
	// 白名单为空：跳过（Java 同款）
	remind, err := svc.Add(ctx, RemindSaveRequest{
		Remind:  &Remind{PID: "model-1", StepIDs: "", TenantID: "dev"},
		Targets: []*RemindTarget{{ExecutorID: RemindExecutorCurrentHandler, Type: RemindTypeAhead, Ahead: 5, TimeUnit: "分钟"}},
	})
	if err != nil {
		t.Fatalf("配置失败: %v", err)
	}
	fake.Set(completeFixtureTime.Add(6 * time.Minute))
	if hits, _ := svc.ScanDue(ctx, fake.Now()); len(hits) != 0 {
		t.Errorf("空白名单应跳过: %d", len(hits))
	}
	// 白名单不含该步骤：跳过
	if _, err := svc.Update(ctx, RemindSaveRequest{Remind: &Remind{ID: remind.ID, PID: "model-1"}, ReplaceStepIDs: true,
		Targets: []*RemindTarget{{ExecutorID: RemindExecutorCurrentHandler, Type: RemindTypeAhead, Ahead: 5, TimeUnit: "分钟"}}}); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	// 更新后白名单为空仍跳过；改为不相关步骤
	if err := reminds.Save(ctx, &Remind{ID: remind.ID, PID: "model-1", StepIDs: "step-other", TenantID: "dev"}); err != nil {
		t.Fatalf("直改失败: %v", err)
	}
	if hits, _ := svc.ScanDue(ctx, fake.Now()); len(hits) != 0 {
		t.Errorf("白名单不含该步骤应跳过: %d", len(hits))
	}
	// 已发送对象：未启用重试不再发
	if err := targets.MarkSent(ctx, listFirstTargetID(t, targets, remind.ID)); err != nil {
		t.Fatalf("标记发送失败: %v", err)
	}
	if err := reminds.Save(ctx, &Remind{ID: remind.ID, PID: "model-1", StepIDs: "step-1", TenantID: "dev"}); err != nil {
		t.Fatalf("直改失败: %v", err)
	}
	if hits, _ := svc.ScanDue(ctx, fake.Now()); len(hits) != 0 {
		t.Errorf("已发送未启用重试不应命中: %d", len(hits))
	}
	// 启用重试但次数用尽不再发
	target := listFirstTarget(t, targets, remind.ID)
	target.Retry = 1
	target.RetryTimes = 2
	target.RetryCount = 2
	if err := targets.Save(ctx, target); err != nil {
		t.Fatalf("更新对象失败: %v", err)
	}
	if hits, _ := svc.ScanDue(ctx, fake.Now()); len(hits) != 0 {
		t.Errorf("重试次数用尽不应命中: %d", len(hits))
	}
	// 启用重试且未用尽：命中
	target.RetryCount = 1
	if err := targets.Save(ctx, target); err != nil {
		t.Fatalf("更新对象失败: %v", err)
	}
	hits, _ := svc.ScanDue(ctx, fake.Now())
	if len(hits) != 1 {
		t.Fatalf("重试未用尽应命中: %d", len(hits))
	}
}

// listFirstTargetID 返回配置首个对象的 Id（测试辅助）。
// @param t 测试上下文
// @param targets 对象存储
// @param remindID 配置 Id
// @return 对象 Id
func listFirstTargetID(t *testing.T, targets *MemRemindTargetStore, remindID string) string {
	t.Helper()
	list, err := targets.ListByRemind(context.Background(), remindID)
	if err != nil || len(list) == 0 {
		t.Fatalf("对象缺失: %v", err)
	}
	return list[0].ID
}

// listFirstTarget 返回配置首个对象（测试辅助）。
// @param t 测试上下文
// @param targets 对象存储
// @param remindID 配置 Id
// @return 对象
func listFirstTarget(t *testing.T, targets *MemRemindTargetStore, remindID string) *RemindTarget {
	t.Helper()
	list, err := targets.ListByRemind(context.Background(), remindID)
	if err != nil || len(list) == 0 {
		t.Fatalf("对象缺失: %v", err)
	}
	return list[0]
}

// TestDeliver 验证送达链路：展开/去重/通知/标记。
// @param t 测试上下文
func TestDeliver(t *testing.T) {
	svc, reminds, targets, _, _, notifier, _ := newRemindFixture(t)
	ctx := context.Background()
	remind, err := svc.Add(ctx, RemindSaveRequest{
		Remind: &Remind{PID: "model-1", StepIDs: "step-1", TenantID: "dev"},
		Targets: []*RemindTarget{{ExecutorID: "role-1", TargetType: RemindTargetTypeRole, Type: RemindTypeAhead, Ahead: 5, TimeUnit: "分钟", Retry: 1, RetryTimes: 2}},
	})
	if err != nil {
		t.Fatalf("配置失败: %v", err)
	}
	receipt := completeFixtureTime
	todo := &MessageExtend{
		BaseColumns:       BaseColumns{TenantID: "dev", CreateTime: receipt},
		TaskID:            "task-d1",
		ProcessInstanceID: "pi-1",
		Title:             "标题",
		TaskStepCode:      "step-1",
		FlowCategoryName:  "办结测试流程",
		OwnerID:           "carl",
		TaskReceiptTime:   &receipt,
	}
	// 白名单无待办在名单内（ListAll 无该步骤待办）→ 直接构造命中
	targetList, _ := targets.ListByRemind(ctx, remind.ID)
	hit := &RemindHit{
		Remind:   remind,
		Target:   targetList[0],
		Todo:     todo,
		Kind:     RemindKindAhead,
		Content:  remindContent(todo, RemindKindAhead),
		Channels: []string{"email"},
	}
	// 送达：role-1 展开 alice/bob，去重后各通知一次
	delivered, err := svc.Deliver(ctx, hit, notifier)
	if err != nil || delivered != 2 {
		t.Fatalf("送达错误: %d, %v", delivered, err)
	}
	if len(notifier.notifications) != 2 {
		t.Fatalf("通知数错误: %d", len(notifier.notifications))
	}
	if notifier.notifications[0].ReceiverID != "alice" || notifier.notifications[1].ReceiverID != "bob" {
		t.Errorf("接收人错误: %s, %s", notifier.notifications[0].ReceiverID, notifier.notifications[1].ReceiverID)
	}
	if notifier.notifications[0].Kind != RemindKindAhead || notifier.notifications[0].Subject != "标题" {
		t.Errorf("通知内容错误: %+v", notifier.notifications[0])
	}
	// 标记：sent=1 且重试计数 +1
	marked, _ := targets.ListByRemind(ctx, remind.ID)
	if marked[0].Sent != 1 || marked[0].RetryCount != 1 {
		t.Errorf("标记错误: sent=%d count=%d", marked[0].Sent, marked[0].RetryCount)
	}
	// 主表 sent 标记由调度层维护，此处验证对象标记即可
	_ = reminds
	// 送达器失败：返回错误且计数不增加
	notifier.fail = true
	before := marked[0].RetryCount
	if _, err := svc.Deliver(ctx, hit, notifier); err == nil {
		t.Error("送达失败应上抛")
	}
	marked, _ = targets.ListByRemind(ctx, remind.ID)
	if marked[0].RetryCount != before {
		t.Errorf("失败不应累计计数: %d", marked[0].RetryCount)
	}
	notifier.fail = false
	// 重试上限：再送一次后计数达上限，扫描不再命中
	if _, err := svc.Deliver(ctx, hit, notifier); err != nil {
		t.Fatalf("重试送达失败: %v", err)
	}
	marked, _ = targets.ListByRemind(ctx, remind.ID)
	if marked[0].RetryCount != 2 {
		t.Fatalf("重试计数应达上限: %d", marked[0].RetryCount)
	}
	// 空命中/空送达器
	if delivered, err := svc.Deliver(ctx, nil, notifier); err != nil || delivered != 0 {
		t.Errorf("空命中应零送达: %d, %v", delivered, err)
	}
	if delivered, err := svc.Deliver(ctx, hit, nil); err != nil || delivered != 0 {
		t.Errorf("空送达器应零送达: %d, %v", delivered, err)
	}
}

// TestSendNow 验证手动催办。
// @param t 测试上下文
func TestSendNow(t *testing.T) {
	svc, _, _, todos, _, _, _ := newRemindFixture(t)
	ctx := context.Background()
	// 空任务 Id
	if _, err := svc.SendNow(ctx, ""); err == nil {
		t.Error("空任务 Id 应报错")
	}
	// 待办不存在
	if _, err := svc.SendNow(ctx, "no-such"); err == nil {
		t.Error("待办不存在应报错")
	}
	store := todos.store
	todo := seedTodo(t, store, "task-1", "step-1", "carl")
	// 无配置：零命中
	if hits, err := svc.SendNow(ctx, todo.TaskID); err != nil || len(hits) != 0 {
		t.Fatalf("无配置应零命中: %d, %v", len(hits), err)
	}
	// 配置白名单包含该步骤：命中（手动催办不受 sent/retry 限制）
	if _, err := svc.Add(ctx, RemindSaveRequest{
		Remind: &Remind{PID: "model-1", StepIDs: "step-1", TenantID: "dev", Sent: 1},
		Targets: []*RemindTarget{{ExecutorID: RemindExecutorCurrentHandler, Type: RemindTypeAhead, Ahead: 5, TimeUnit: "分钟"}},
	}); err != nil {
		t.Fatalf("配置失败: %v", err)
	}
	hits, err := svc.SendNow(ctx, todo.TaskID)
	if err != nil || len(hits) != 1 {
		t.Fatalf("手动催办应命中: %d, %v", len(hits), err)
	}
	if hits[0].Kind != RemindKindManual || hits[0].Todo.TaskID != todo.TaskID {
		t.Errorf("命中内容错误: kind=%q todo=%s", hits[0].Kind, hits[0].Todo.TaskID)
	}
}

// TestLogNotifier 验证日志送达器（默认实现不外发）。
// @param t 测试上下文
func TestLogNotifier(t *testing.T) {
	notifier := LogNotifier{}
	if err := notifier.Notify(context.Background(), &RemindNotification{ReceiverID: "alice", Kind: RemindKindAhead, Content: "内容"}); err != nil {
		t.Errorf("日志送达失败: %v", err)
	}
	// 空报文不崩溃
	if err := notifier.Notify(context.Background(), nil); err != nil {
		t.Errorf("空报文应容忍: %v", err)
	}
}

// TestMemRemindStoresEdge 验证内存存储边界。
// @param t 测试上下文
func TestMemRemindStoresEdge(t *testing.T) {
	ctx := context.Background()
	reminds := NewMemRemindStore(nil)
	// 空配置/缺 pid
	if err := reminds.Save(ctx, nil); err == nil {
		t.Error("空配置应报错")
	}
	if err := reminds.Save(ctx, &Remind{}); err == nil {
		t.Error("缺 pid 应报错")
	}
	targets := NewMemRemindTargetStore(nil)
	if err := targets.Save(ctx, nil); err == nil {
		t.Error("空对象应报错")
	}
	if err := targets.Save(ctx, &RemindTarget{}); err == nil {
		t.Error("缺 pid 应报错")
	}
	// 删除对象的先删后增
	if err := targets.Save(ctx, &RemindTarget{ID: "tg-1", PID: "r-1", Enable: 1}); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if err := targets.DeleteByRemind(ctx, "r-1"); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if list, _ := targets.ListByRemind(ctx, "r-1"); len(list) != 0 {
		t.Errorf("删除后应为空: %d", len(list))
	}
	// 停用对象不回查
	if err := targets.Save(ctx, &RemindTarget{ID: "tg-2", PID: "r-1", Enable: 0}); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if list, _ := targets.ListByRemind(ctx, "r-1"); len(list) != 0 {
		t.Errorf("停用对象不应回查: %d", len(list))
	}
	// 标记与计数
	if err := targets.MarkSent(ctx, "tg-2"); err != nil {
		t.Fatalf("标记失败: %v", err)
	}
	if err := targets.UpdateRetryCount(ctx, "tg-2", 3); err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	// 未知 Id 静默
	if err := targets.MarkSent(ctx, "no-such"); err != nil {
		t.Errorf("未知 Id 标记应静默: %v", err)
	}
	if err := targets.UpdateRetryCount(ctx, "no-such", 1); err != nil {
		t.Errorf("未知 Id 计数应静默: %v", err)
	}
}

// ─── SQL 适配（sqlmock） ─────────────────────────────────────────────

// newRemindSQLStores 创建催办配置/对象 SQL 适配器与其 sqlmock。
// @param t 测试上下文
// @return 配置适配器
// @return 对象适配器
// @return 数据库模拟
func newRemindSQLStores(t *testing.T) (*SQLRemindStore, *SQLRemindTargetStore, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("创建 sqlmock 失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := pwf.NewStore(db, pwf.DialectMySQL)
	if err != nil {
		t.Fatalf("创建 workflow 仓储入口失败: %v", err)
	}
	return NewSQLRemindStore(store, nil), NewSQLRemindTargetStore(store), mock
}

// TestSQLRemindStoreSaveAndGet 验证催办配置 SQL 适配（upsert/级联/标记/删除）。
// @param t 测试上下文
func TestSQLRemindStoreSaveAndGet(t *testing.T) {
	reminds, targets, mock := newRemindSQLStores(t)
	ctx := context.Background()
	stamped := time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC)
	remind := &Remind{ID: "r-1", PID: "model-1", StepIDs: "step-1", EnablePre: 1, TenantID: "dev", CreateTime: stamped, UpdateTime: stamped}
	// 新增（UPDATE 0 行 → INSERT）
	mock.ExpectExec("UPDATE workflow_remind SET").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO workflow_remind").WillReturnResult(sqlmock.NewResult(1, 1))
	if err := reminds.Save(ctx, remind); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	// 更新（UPDATE 1 行）
	mock.ExpectExec("UPDATE workflow_remind SET").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := reminds.Save(ctx, remind); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	// 空入参
	if err := reminds.Save(ctx, &Remind{ID: "r-2"}); err == nil {
		t.Error("缺 pid 应报错")
	}
	// 级联回读
	mock.ExpectQuery("SELECT (.+) FROM workflow_remind WHERE").WithArgs("r-1").WillReturnRows(
		sqlmock.NewRows([]string{"id", "pid", "step_ids", "enable_pre", "enable_post", "sent", "deleted", "creator", "creator_name", "create_time", "updater", "update_time", "tenant_id"}).
			AddRow("r-1", "model-1", "step-1", 1, 0, 0, 0, "alice", "张三", stamped, "", stamped, "dev"))
	mock.ExpectQuery("SELECT (.+) FROM workflow_remind_target WHERE").WithArgs("r-1").WillReturnRows(
		sqlmock.NewRows([]string{"id", "pid", "executor_id", "enable", "name", "target_type", "type", "retry", "retry_duration", "retry_times", "retry_count", "retry_time_unit", "ahead", "delay", "by_plan_complete_time", "removable", "sent", "deleted", "creator", "creator_name", "create_time", "updater", "update_time", "tenant_id"}).
			AddRow("tg-1", "r-1", "-1111", 1, "当前处理人", 1, 1, 0, 0, 0, 0, "", 5, 0, 0, 0, 0, 0, "alice", "张三", stamped, "", stamped, "dev"))
	got, found, err := reminds.Get(ctx, "r-1")
	if err != nil || !found {
		t.Fatalf("级联回读失败: %v %v", found, err)
	}
	if len(got.Targets) != 1 {
		t.Fatalf("级联对象缺失: %d", len(got.Targets))
	}
	if got.Targets[0].Ahead != 5 {
		t.Errorf("对象浮点列回读错误: %v", got.Targets[0].Ahead)
	}
	// 查无
	mock.ExpectQuery("SELECT (.+) FROM workflow_remind WHERE").WithArgs("r-x").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if _, found, _ := reminds.Get(ctx, "r-x"); found {
		t.Error("查无应返回不存在")
	}
	// ListByModel
	mock.ExpectQuery("SELECT (.+) FROM workflow_remind WHERE").WithArgs("model-1").WillReturnRows(
		sqlmock.NewRows([]string{"id", "pid", "step_ids", "enable_pre", "enable_post", "sent", "deleted"}).
			AddRow("r-1", "model-1", "step-1", 1, 0, 0, 0))
	if list, err := reminds.ListByModel(ctx, "model-1"); err != nil || len(list) != 1 {
		t.Fatalf("按模型查询错误: %d, %v", len(list), err)
	}
	// SQL 适配的全量扫描接口当前为空集占位（主控接线说明见报告）
	if list, err := reminds.ListActive(ctx); err != nil || len(list) != 0 {
		t.Errorf("ListActive 占位应返回空集: %d, %v", len(list), err)
	}
	if list, err := reminds.ListNotSent(ctx); err != nil || len(list) != 0 {
		t.Errorf("ListNotSent 占位应返回空集: %d, %v", len(list), err)
	}
	// MarkSent
	mock.ExpectExec("UPDATE workflow_remind SET").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := reminds.MarkSent(ctx, "r-1"); err != nil {
		t.Fatalf("标记发送失败: %v", err)
	}
	// 逻辑删除（单/批）
	mock.ExpectExec("UPDATE workflow_remind SET").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := reminds.MarkDeleted(ctx, "r-1"); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	mock.ExpectExec("UPDATE workflow_remind SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE workflow_remind SET").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := reminds.MarkDeletedByIDs(ctx, []string{"r-1", "r-2"}); err != nil {
		t.Fatalf("批量删除失败: %v", err)
	}
	// 对象适配：新增/更新/查询/删除/标记/计数
	target := &RemindTarget{ID: "tg-1", PID: "r-1", ExecutorID: "-1111", Enable: 1, Type: 1, Ahead: 5, TimeUnit: "分钟", CreateTime: stamped, TenantID: "dev"}
	mock.ExpectExec("UPDATE workflow_remind_target SET").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO workflow_remind_target").WillReturnResult(sqlmock.NewResult(1, 1))
	if err := targets.Save(ctx, target); err != nil {
		t.Fatalf("对象保存失败: %v", err)
	}
	mock.ExpectExec("UPDATE workflow_remind_target SET").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := targets.Save(ctx, target); err != nil {
		t.Fatalf("对象更新失败: %v", err)
	}
	if err := targets.Save(ctx, &RemindTarget{ID: "tg-2"}); err == nil {
		t.Error("缺 pid 应报错")
	}
	mock.ExpectQuery("SELECT (.+) FROM workflow_remind_target WHERE").WithArgs("r-1").WillReturnRows(
		sqlmock.NewRows([]string{"id", "pid", "executor_id", "enable", "name", "target_type", "type", "retry", "retry_duration", "retry_times", "retry_count", "retry_time_unit", "ahead", "delay", "by_plan_complete_time", "removable", "sent", "deleted", "creator", "creator_name", "create_time", "updater", "update_time", "tenant_id"}).
			AddRow("tg-1", "r-1", "-1111", 1, "当前处理人", 1, 1, 1, 0, 2, 1, "分钟", 5, 0, 0, 0, 1, 0, "alice", "张三", stamped, "", stamped, "dev"))
	if list, err := targets.ListByRemind(ctx, "r-1"); err != nil || len(list) != 1 || list[0].RetryTimes != 2 {
		t.Fatalf("对象查询错误: %d, %v", len(list), err)
	}
	// 停用对象被内存过滤
	mock.ExpectQuery("SELECT (.+) FROM workflow_remind_target WHERE").WithArgs("r-1").WillReturnRows(
		sqlmock.NewRows([]string{"id", "pid", "executor_id", "enable", "type", "ahead", "deleted"}).
			AddRow("tg-2", "r-1", "-1111", 0, 1, 5, 0))
	if list, _ := targets.ListByRemind(ctx, "r-1"); len(list) != 0 {
		t.Errorf("停用对象应被过滤: %d", len(list))
	}
	mock.ExpectExec("DELETE FROM workflow_remind_target").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := targets.DeleteByRemind(ctx, "r-1"); err != nil {
		t.Fatalf("对象删除失败: %v", err)
	}
	mock.ExpectExec("UPDATE workflow_remind_target SET").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := targets.MarkSent(ctx, "tg-1"); err != nil {
		t.Fatalf("对象标记失败: %v", err)
	}
	mock.ExpectExec("UPDATE workflow_remind_target SET").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := targets.UpdateRetryCount(ctx, "tg-1", 2); err != nil {
		t.Fatalf("对象计数失败: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("未满足的期望: %v", err)
	}
}

// TestRemindContentFormat 验证催办内容格式（Java sendToUser 同款）。
// @param t 测试上下文
func TestRemindContentFormat(t *testing.T) {
	receipt := completeFixtureTime
	todo := &MessageExtend{Title: "标题", FlowCategoryName: "测试流程", TaskReceiptTime: &receipt}
	content := remindContent(todo, RemindKindAhead)
	if !strings.Contains(content, "【预警提醒】标题") || !strings.Contains(content, "流程：测试流程") {
		t.Errorf("内容格式错误: %q", content)
	}
	if !strings.Contains(content, "2026-10-31 08:00:00") {
		t.Errorf("接收时间格式错误: %q", content)
	}
	// 无接收时间留空（内容以时间前缀结尾）
	todo.TaskReceiptTime = nil
	content = remindContent(todo, RemindKindAhead)
	if !strings.HasSuffix(content, "时间：") {
		t.Errorf("无接收时间应留空: %q", content)
	}
}
