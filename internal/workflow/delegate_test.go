// delegate_test.go 验证规则型委托域服务：时间重叠三段式边界、传递禁止、命中优先级矩阵、
// 待办改派钩子、待办调整多 owner 与 SQL 适配。
package workflow

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/DengQingNian/Flowtide/internal/clock"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
)

// TestTimeOverlaps 验证时间重叠三段式边界（相交/相触/包含/分离）。
// @param t 测试上下文
func TestTimeOverlaps(t *testing.T) {
	day := func(d int, hour int) time.Time { return time.Date(2026, 10, d, hour, 0, 0, 0, time.UTC) }
	cases := []struct {
		name         string    // 用例名
		existingFrom time.Time // 既有生效
		existingTo   time.Time // 既有失效
		newFrom      time.Time // 新生效
		newTo        time.Time // 新失效
		want         bool      // 期望重叠
	}{
		{"新区间左端落入既有区间", day(1, 0), day(10, 0), day(5, 0), day(20, 0), true},
		{"新区间右端落入既有区间", day(5, 0), day(20, 0), day(1, 0), day(10, 0), true},
		{"既有区间被新区间包含", day(5, 0), day(10, 0), day(1, 0), day(20, 0), true},
		{"端点相触（恰好相等）", day(1, 0), day(10, 0), day(10, 0), day(20, 0), true},
		{"完全分离", day(1, 0), day(5, 0), day(6, 0), day(10, 0), false},
		{"既有区间包含新区间", day(1, 0), day(20, 0), day(5, 0), day(10, 0), true},
		{"零值时间不判定", time.Time{}, day(5, 0), day(1, 0), day(10, 0), false},
	}
	for _, tc := range cases {
		if got := timeOverlaps(tc.existingFrom, tc.existingTo, tc.newFrom, tc.newTo); got != tc.want {
			t.Errorf("%s: 期望 %v 实得 %v", tc.name, tc.want, got)
		}
	}
}

// TestWithinValidWindow 验证命中窗口的按天宽化（Java mapper 语义）。
// @param t 测试上下文
func TestWithinValidWindow(t *testing.T) {
	// 当日 09:00 生效、当日 18:00 失效（仅存时刻）
	from := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 31, 18, 0, 0, 0, time.UTC)
	// 同日 00:30 命中（宽化到当日零点起）
	if !withinValidWindow(from, to, time.Date(2026, 10, 31, 0, 30, 0, 0, time.UTC)) {
		t.Error("同日零点后应命中（按天宽化）")
	}
	// 同日 23:59:59 命中
	if !withinValidWindow(from, to, time.Date(2026, 10, 31, 23, 59, 59, 0, time.UTC)) {
		t.Error("同日末秒应命中")
	}
	// 次日不命中
	if withinValidWindow(from, to, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) {
		t.Error("次日不应命中")
	}
	// 零值窗口不命中
	if withinValidWindow(time.Time{}, to, time.Now()) {
		t.Error("零值生效时间不应命中")
	}
}

// fakeCategoryLookup 是分类代号解析桩（可注入固定映射）。
type fakeCategoryLookup struct {
	models map[string]string // 代号 → 模型 Id
}

// ResolveModelID 解析分类代号。
// @param ctx 操作上下文
// @param tenantID 租户
// @param flowCategoryCode 分类代号
// @return 模型 Id
// @return 错误
func (f *fakeCategoryLookup) ResolveModelID(ctx context.Context, tenantID, flowCategoryCode string) (string, error) {
	return f.models[flowCategoryCode], nil
}

// newDelegateFixture 构造委托域服务测试环境（复用办结测试语料与内存引擎）。
// @param t 测试上下文
// @return 办结测试环境
// @return 委托域服务
// @return 委托规则内存存储
// @return 委托镜像内存存储
func newDelegateFixture(t *testing.T) (*completeFixture, *DelegateRuleService, *MemTaskDelegateStore, *MemDelegateTaskMirrorStore) {
	t.Helper()
	fx := newCompleteFixture(t)
	store := NewMemTaskDelegateStore(fx.fakeClock)
	mirrors := NewMemDelegateTaskMirrorStore(fx.fakeClock)
	categories := &fakeCategoryLookup{models: map[string]string{"flowComplete": "model-1"}}
	svc := NewDelegateRuleService(store, mirrors, fx.messages, fx.next, fx.operations, fx.engine, fx.engine.IdentityService(), categories, fx.fakeClock)
	return fx, svc, store, mirrors
}

// baseRule 构造一条合法委托规则模板（单项目委托 alice→bob，覆盖假时钟 2026-10-31 的生效窗口）。
// @return 委托规则
func baseRule() *DelegateRule {
	return &DelegateRule{
		ConsignType: ConsignTypeSingleProject,
		OwnerID:     "alice",
		SourceCode:  "dev",
		VaildFrom:   time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		VaildTo:     time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC),
		Consigners:  []DelegateConsigner{{ConsignerID: "bob", ConsignerName: "李四"}},
		ConsignObjs: []DelegateConsignObj{{OwnerType: ConsignTypeSingleProject, OwnerID: "proj-1", OwnerName: "项目一"}},
	}
}

// TestBuildCondition 验证委托条件串生成（修正 Java :161/:163 笔误的语义）。
// @param t 测试上下文
func TestBuildCondition(t *testing.T) {
	_, svc, _, _ := newDelegateFixture(t)
	// 全部委托无条件串
	all := baseRule()
	all.ConsignType = ConsignTypeAll
	svc.buildCondition(all)
	if all.Condition != "" {
		t.Errorf("全部委托应无条件串: %q", all.Condition)
	}
	// 单项目
	project := baseRule()
	svc.buildCondition(project)
	if project.Condition != "@ProjId='proj-1'" {
		t.Errorf("单项目条件串错误: %q", project.Condition)
	}
	// 单流程分类（使用 @FlowCategory 前缀，修正 Java 笔误）
	category := baseRule()
	category.ConsignType = ConsignTypeSingleFlowCategory
	category.ConsignObjs = []DelegateConsignObj{{OwnerID: "model-1"}}
	svc.buildCondition(category)
	if category.Condition != "@FlowCategory='model-1'" {
		t.Errorf("单分类条件串错误: %q", category.Condition)
	}
	// 多项目逗号串
	multi := baseRule()
	multi.ConsignType = ConsignTypeMultiProject
	multi.ConsignObjs = []DelegateConsignObj{{OwnerID: "proj-1"}, {OwnerID: "proj-2"}}
	svc.buildCondition(multi)
	if multi.Condition != "@ProjId='proj-1,proj-2'" {
		t.Errorf("多项目条件串错误: %q", multi.Condition)
	}
}

// TestDelegateRuleAddValidation 验证新增校验链（对位 Java add:58 各分支）。
// @param t 测试上下文
func TestDelegateRuleAddValidation(t *testing.T) {
	_, svc, _, _ := newDelegateFixture(t)
	ctx := context.Background()
	// 空规则
	if err := svc.Add(ctx, nil); err == nil {
		t.Error("空规则应报错")
	}
	// 委托人缺失
	missingOwner := baseRule()
	missingOwner.OwnerID = ""
	if err := svc.Add(ctx, missingOwner); err == nil || !strings.Contains(err.Error(), "委托人不能为空") {
		t.Errorf("委托人缺失应报错: %v", err)
	}
	// 生效/失效时间缺失
	noFrom := baseRule()
	noFrom.VaildFrom = time.Time{}
	if err := svc.Add(ctx, noFrom); err == nil || !strings.Contains(err.Error(), "生效时间") {
		t.Errorf("生效时间缺失应报错: %v", err)
	}
	noTo := baseRule()
	noTo.VaildTo = time.Time{}
	if err := svc.Add(ctx, noTo); err == nil || !strings.Contains(err.Error(), "失效时间") {
		t.Errorf("失效时间缺失应报错: %v", err)
	}
	// 失效早于生效
	inverted := baseRule()
	inverted.VaildTo = inverted.VaildFrom.Add(-time.Hour)
	if err := svc.Add(ctx, inverted); err == nil || !strings.Contains(err.Error(), "失效时间不能早于") {
		t.Errorf("时间倒置应报错: %v", err)
	}
	// 被委托人缺失
	noConsigner := baseRule()
	noConsigner.Consigners = nil
	if err := svc.Add(ctx, noConsigner); err == nil || !strings.Contains(err.Error(), "被委托人不能为空") {
		t.Errorf("被委托人缺失应报错: %v", err)
	}
	// 被委托人 Id 缺失
	blankID := baseRule()
	blankID.Consigners = []DelegateConsigner{{ConsignerID: ""}}
	if err := svc.Add(ctx, blankID); err == nil || !strings.Contains(err.Error(), "被委托人ID不能为空") {
		t.Errorf("被委托人 Id 缺失应报错: %v", err)
	}
	// 被委托人包含委托人本人
	self := baseRule()
	self.Consigners = []DelegateConsigner{{ConsignerID: "alice"}}
	if err := svc.Add(ctx, self); err == nil || !strings.Contains(err.Error(), "不能包含委托人") {
		t.Errorf("委托人本人应报错: %v", err)
	}
	// 范围委托缺对象（项目/分类两种文案）
	noObj := baseRule()
	noObj.ConsignObjs = nil
	if err := svc.Add(ctx, noObj); err == nil || !strings.Contains(err.Error(), "项目名称") {
		t.Errorf("项目对象缺失应报错: %v", err)
	}
	catNoObj := baseRule()
	catNoObj.ConsignType = ConsignTypeSingleFlowCategory
	catNoObj.ConsignObjs = nil
	if err := svc.Add(ctx, catNoObj); err == nil || !strings.Contains(err.Error(), "流程名称") {
		t.Errorf("分类对象缺失应报错: %v", err)
	}
}

// TestDelegateRuleAddOverlap 验证时间重叠三段式校验（同 owner 同对象，既有区间 10-01~11-30）。
// @param t 测试上下文
func TestDelegateRuleAddOverlap(t *testing.T) {
	_, svc, _, _ := newDelegateFixture(t)
	ctx := context.Background()
	// 既有规则：10-01 ~ 11-30（proj-1，alice→bob）
	if err := svc.Add(ctx, baseRule()); err != nil {
		t.Fatalf("既有规则保存失败: %v", err)
	}
	cases := []struct {
		name    string              // 用例名
		mutate  func(*DelegateRule) // 规则变形
		wantErr bool                // 期望报错
	}{
		{"完全相同区间", func(r *DelegateRule) {}, true},
		{"区间相交（局部重叠）", func(r *DelegateRule) {
			r.VaildFrom = time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
			r.VaildTo = time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
		}, true},
		{"端点相触", func(r *DelegateRule) {
			r.VaildFrom = time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)
			r.VaildTo = time.Date(2026, 12, 10, 0, 0, 0, 0, time.UTC)
		}, true},
		{"包含既有区间", func(r *DelegateRule) {
			r.VaildFrom = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
			r.VaildTo = time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
		}, true},
		{"分离区间不冲突", func(r *DelegateRule) {
			r.VaildFrom = time.Date(2026, 12, 11, 0, 0, 0, 0, time.UTC)
			r.VaildTo = time.Date(2026, 12, 20, 0, 0, 0, 0, time.UTC)
		}, false},
		{"不同对象不冲突", func(r *DelegateRule) {
			r.ConsignObjs = []DelegateConsignObj{{OwnerID: "proj-2"}}
			r.VaildFrom = time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
			r.VaildTo = time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
		}, false},
	}
	for _, tc := range cases {
		candidate := baseRule()
		tc.mutate(candidate)
		err := svc.Add(ctx, candidate)
		if tc.wantErr && err == nil {
			t.Errorf("%s: 应报重叠错误", tc.name)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("%s: 不应报错: %v", tc.name, err)
		}
	}
	// 停用规则不参与冲突：先停用既有规则，再提交重叠区间应允许
	rules, _ := svc.List(ctx, "alice", "dev")
	// 首条即最初保存的既有规则（按创建时间升序）
	if len(rules) > 0 {
		if err := svc.Disable(ctx, rules[0].ID); err != nil {
			t.Fatalf("停用失败: %v", err)
		}
	}
	candidate := baseRule()
	candidate.VaildFrom = time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	candidate.VaildTo = time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	if err := svc.Add(ctx, candidate); err != nil {
		t.Errorf("停用规则不应阻断重叠提交: %v", err)
	}
	// 恢复启用最初那条
	rules, _ = svc.List(ctx, "alice", "dev")
	if len(rules) > 0 {
		_ = svc.Enable(ctx, rules[0].ID)
	}
	// 全部委托之间冲突
	allExisting := baseRule()
	allExisting.ConsignType = ConsignTypeAll
	allExisting.ConsignObjs = nil
	if err := svc.Add(ctx, allExisting); err != nil {
		t.Fatalf("全部委托保存失败: %v", err)
	}
	allNew := baseRule()
	allNew.ConsignType = ConsignTypeAll
	allNew.ConsignObjs = nil
	if err := svc.Add(ctx, allNew); err == nil || !strings.Contains(err.Error(), "重叠") {
		t.Errorf("全部委托之间应冲突: %v", err)
	}
}

// TestDelegateRulePassChain 验证禁止委托传递（委托人已是他人被委托人）。
// @param t 测试上下文
func TestDelegateRulePassChain(t *testing.T) {
	_, svc, _, _ := newDelegateFixture(t)
	ctx := context.Background()
	// bob 已被 alice 委托（10-01 ~ 11-30，proj-1）
	if err := svc.Add(ctx, baseRule()); err != nil {
		t.Fatalf("既有规则保存失败: %v", err)
	}
	// carl 想把任务委托给 alice（alice 未被任何规则委托，允许）
	carlToAlice := baseRule()
	carlToAlice.OwnerID = "carl"
	carlToAlice.Consigners = []DelegateConsigner{{ConsignerID: "alice"}}
	carlToAlice.VaildFrom = time.Date(2026, 12, 11, 0, 0, 0, 0, time.UTC)
	carlToAlice.VaildTo = time.Date(2026, 12, 20, 0, 0, 0, 0, time.UTC)
	if err := svc.Add(ctx, carlToAlice); err != nil {
		t.Fatalf("carl→alice 应允许: %v", err)
	}
	// bob 已被 alice 委托（时间重叠窗口内），bob 再委托出去 → 禁止传递
	bobToCarl := baseRule()
	bobToCarl.OwnerID = "bob"
	bobToCarl.Consigners = []DelegateConsigner{{ConsignerID: "carl"}}
	bobToCarl.VaildFrom = time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	bobToCarl.VaildTo = time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	if err := svc.Add(ctx, bobToCarl); err == nil || !strings.Contains(err.Error(), "不能传递委托") {
		t.Errorf("传递链应被禁止: %v", err)
	}
	// 时间不重叠时不禁止（bob 的既有被委托窗口 10-01~11-30 与 12 月无交集）
	bobToCarlLater := baseRule()
	bobToCarlLater.OwnerID = "bob"
	bobToCarlLater.Consigners = []DelegateConsigner{{ConsignerID: "carl"}}
	bobToCarlLater.VaildFrom = time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	bobToCarlLater.VaildTo = time.Date(2026, 12, 10, 0, 0, 0, 0, time.UTC)
	if err := svc.Add(ctx, bobToCarlLater); err != nil {
		t.Errorf("时间分离的传递应允许: %v", err)
	}
}

// TestDelegateRuleAddUpdateLifecycle 验证新增成功、更新、启停、退休、删除生命周期。
// @param t 测试上下文
func TestDelegateRuleAddUpdateLifecycle(t *testing.T) {
	_, svc, store, _ := newDelegateFixture(t)
	ctx := context.Background()
	// 新增成功：enable=1 且条件串生成
	rule := baseRule()
	if err := svc.Add(ctx, rule); err != nil {
		t.Fatalf("新增失败: %v", err)
	}
	if rule.Enable != DelegateEnabled || rule.Condition != "@ProjId='proj-1'" || rule.ID == "" {
		t.Errorf("新增结果错误: enable=%d condition=%q id=%q", rule.Enable, rule.Condition, rule.ID)
	}
	// 回读级联
	got, found, err := store.Get(ctx, rule.ID)
	if err != nil || !found || len(got.Consigners) != 1 || len(got.ConsignObjs) != 1 {
		t.Fatalf("回读失败: %v %v %+v", found, err, got)
	}
	if got.Consigners[0].DelegateID != rule.ID || got.ConsignObjs[0].DelegateID != rule.ID {
		t.Error("子表外键未回填")
	}
	// 更新不存在
	if err := svc.Update(ctx, &DelegateRule{ID: "no-such"}); err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Errorf("更新不存在应报错: %v", err)
	}
	// 更新空 Id
	if err := svc.Update(ctx, &DelegateRule{}); err == nil {
		t.Error("更新空 Id 应报错")
	}
	// 更新成功（改时间与被委托人；排除自身后无其他规则，重叠校验通过）
	if err := svc.Update(ctx, &DelegateRule{
		ID:          rule.ID,
		ConsignType: ConsignTypeSingleProject,
		OwnerID:     "alice",
		SourceCode:  "dev",
		VaildFrom:   time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC),
		VaildTo:     time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC),
		Consigners:  []DelegateConsigner{{ConsignerID: "carl", ConsignerName: "王五"}},
		ConsignObjs: []DelegateConsignObj{{OwnerID: "proj-1"}},
	}); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	updated, _, _ := store.Get(ctx, rule.ID)
	if updated.Consigners[0].ConsignerID != "carl" || !updated.VaildFrom.Equal(time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("更新未生效: %+v", updated)
	}
	// 停用 → 列表仍含（含停用）→ 启用恢复
	if err := svc.Disable(ctx, rule.ID); err != nil {
		t.Fatalf("停用失败: %v", err)
	}
	if list, _ := svc.List(ctx, "alice", "dev"); len(list) != 1 {
		t.Errorf("列表应含停用规则: %d", len(list))
	}
	if err := svc.Enable(ctx, rule.ID); err != nil {
		t.Fatalf("启用失败: %v", err)
	}
	// 退休：enable=0 + retire=1
	if err := svc.Retire(ctx, rule.ID); err != nil {
		t.Fatalf("退休失败: %v", err)
	}
	retired, _, _ := store.Get(ctx, rule.ID)
	if retired.Enable != DelegateDisabled || retired.Retire != DelegateRetired {
		t.Errorf("退休标记错误: enable=%d retire=%d", retired.Enable, retired.Retire)
	}
	// 退休规则不参与时间重叠：alice 再提交同窗口规则应允许
	after := baseRule()
	if err := svc.Add(ctx, after); err != nil {
		t.Errorf("退休规则不应阻断新规则: %v", err)
	}
	// 删除（级联）
	if err := svc.Remove(ctx, after.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if _, found, _ := store.Get(ctx, after.ID); found {
		t.Error("删除后不应存在")
	}
	// 列表租户过滤
	if list, _ := svc.List(ctx, "alice", "other"); len(list) != 0 {
		t.Errorf("异租户列表应为空: %d", len(list))
	}
	// 审计时间来自注入时钟
	if rule.CreateTime.Before(completeFixtureTime) {
		t.Errorf("审计时间应来自注入时钟: %v", rule.CreateTime)
	}
}

// TestHitConsignerPriority 验证命中优先级矩阵：单项目 > 单分类 > 全部。
// @param t 测试上下文
func TestHitConsignerPriority(t *testing.T) {
	_, svc, _, _ := newDelegateFixture(t)
	ctx := context.Background()
	// 空委托人/租户未命中
	if got, _ := svc.HitConsigner(ctx, "", "dev", "proj-1", "flowComplete"); got != "" {
		t.Errorf("空委托人应未命中: %q", got)
	}
	if got, _ := svc.HitConsigner(ctx, "alice", "", "proj-1", "flowComplete"); got != "" {
		t.Errorf("空租户应未命中: %q", got)
	}
	// 全部委托（10-01 ~ 11-30，假时钟 10-31 在窗口内）
	all := baseRule()
	all.ConsignType = ConsignTypeAll
	all.ConsignObjs = nil
	all.Consigners = []DelegateConsigner{{ConsignerID: "helper"}}
	if err := svc.Add(ctx, all); err != nil {
		t.Fatalf("全部委托保存失败: %v", err)
	}
	if got, _ := svc.HitConsigner(ctx, "alice", "dev", "proj-1", "flowComplete"); got != "helper" {
		t.Errorf("仅全部委托时应命中 helper: %q", got)
	}
	// 增加单分类委托（优先级高于全部）
	category := baseRule()
	category.ConsignType = ConsignTypeSingleFlowCategory
	category.ConsignObjs = []DelegateConsignObj{{OwnerID: "model-1"}}
	if err := svc.Add(ctx, category); err != nil {
		t.Fatalf("分类委托保存失败: %v", err)
	}
	if got, _ := svc.HitConsigner(ctx, "alice", "dev", "proj-1", "flowComplete"); got != "bob" {
		t.Errorf("单分类应优先于全部: %q", got)
	}
	// 无项目参数时分类仍命中
	if got, _ := svc.HitConsigner(ctx, "alice", "dev", "", "flowComplete"); got != "bob" {
		t.Errorf("无项目时分类应命中: %q", got)
	}
	// 增加单项目委托（优先级最高）
	project := baseRule()
	project.ConsignType = ConsignTypeSingleProject
	project.ConsignObjs = []DelegateConsignObj{{OwnerID: "proj-1"}}
	if err := svc.Add(ctx, project); err != nil {
		t.Fatalf("项目委托保存失败: %v", err)
	}
	if got, _ := svc.HitConsigner(ctx, "alice", "dev", "proj-1", "flowComplete"); got != "bob" {
		t.Errorf("单项目应优先: %q", got)
	}
	// 项目不匹配时回退分类
	if got, _ := svc.HitConsigner(ctx, "alice", "dev", "proj-other", "flowComplete"); got != "bob" {
		t.Errorf("项目不匹配应回退分类: %q", got)
	}
	// 分类也不匹配时回退全部
	if got, _ := svc.HitConsigner(ctx, "alice", "dev", "proj-other", "otherFlow"); got != "helper" {
		t.Errorf("分类不匹配应回退全部: %q", got)
	}
	// 多项目委托命中
	multi := baseRule()
	multi.ConsignType = ConsignTypeMultiProject
	multi.ConsignObjs = []DelegateConsignObj{{OwnerID: "proj-a"}, {OwnerID: "proj-b"}}
	if err := svc.Add(ctx, multi); err != nil {
		t.Fatalf("多项目委托保存失败: %v", err)
	}
	if got, _ := svc.HitConsigner(ctx, "alice", "dev", "proj-b", "otherFlow"); got != "bob" {
		t.Errorf("多项目应命中: %q", got)
	}
	// 未生效窗口不命中（carl 的 12 月规则在 10-31 不生效）
	future := baseRule()
	future.OwnerID = "carl"
	future.VaildFrom = time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	future.VaildTo = time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	if err := svc.Add(ctx, future); err != nil {
		t.Fatalf("未来规则保存失败: %v", err)
	}
	if got, _ := svc.HitConsigner(ctx, "carl", "dev", "proj-1", "flowComplete"); got != "" {
		t.Errorf("未生效窗口不应命中: %q", got)
	}
	// 停用规则不命中
	disabled := baseRule()
	disabled.OwnerID = "carl"
	disabled.VaildFrom = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	disabled.VaildTo = time.Date(2026, 10, 31, 23, 0, 0, 0, time.UTC)
	if err := svc.Add(ctx, disabled); err != nil {
		t.Fatalf("停用规则保存失败: %v", err)
	}
	if got, _ := svc.HitConsigner(ctx, "carl", "dev", "proj-1", "flowComplete"); got != "bob" {
		t.Errorf("窗口内启用规则应命中: %q", got)
	}
	if err := svc.Disable(ctx, disabled.ID); err != nil {
		t.Fatalf("停用失败: %v", err)
	}
	if got, _ := svc.HitConsigner(ctx, "carl", "dev", "proj-1", "flowComplete"); got != "" {
		t.Errorf("停用规则不应命中: %q", got)
	}
}

// TestHitConsignerCategoryResolver 验证分类代号解析端口的注入与回退。
// @param t 测试上下文
func TestHitConsignerCategoryResolver(t *testing.T) {
	fx := newCompleteFixture(t)
	store := NewMemTaskDelegateStore(fx.fakeClock)
	// 解析端口返回空 → 回退代号直匹配
	emptyResolver := &fakeCategoryLookup{models: map[string]string{}}
	svc := NewDelegateRuleService(store, nil, nil, nil, nil, nil, nil, emptyResolver, fx.fakeClock)
	ctx := context.Background()
	rule := baseRule()
	rule.ConsignType = ConsignTypeSingleFlowCategory
	rule.ConsignObjs = []DelegateConsignObj{{OwnerID: "flowComplete"}}
	if err := svc.Add(ctx, rule); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if got, _ := svc.HitConsigner(ctx, "alice", "dev", "", "flowComplete"); got != "bob" {
		t.Errorf("解析为空应回退代号直匹配: %q", got)
	}
	// 解析端口返回模型 Id → 按模型 Id 命中
	if err := svc.Remove(ctx, rule.ID); err != nil {
		t.Fatalf("移除失败: %v", err)
	}
	resolver := &fakeCategoryLookup{models: map[string]string{"flowComplete": "model-9"}}
	svc2 := NewDelegateRuleService(store, nil, nil, nil, nil, nil, nil, resolver, fx.fakeClock)
	rule2 := baseRule()
	rule2.ConsignType = ConsignTypeSingleFlowCategory
	rule2.ConsignObjs = []DelegateConsignObj{{OwnerID: "model-9"}}
	if err := svc2.Add(ctx, rule2); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if got, _ := svc2.HitConsigner(ctx, "alice", "dev", "", "flowComplete"); got != "bob" {
		t.Errorf("模型 Id 应命中: %q", got)
	}
	// 无匹配分类不命中
	if got, _ := svc2.HitConsigner(ctx, "alice", "dev", "", "otherFlow"); got != "" {
		t.Errorf("无匹配分类不应命中: %q", got)
	}
}

// TestApplyOnTodoCreate 验证待办生成改派钩子。
// @param t 测试上下文
func TestApplyOnTodoCreate(t *testing.T) {
	fx, svc, _, mirrors := newDelegateFixture(t)
	ctx := context.Background()
	// 委托规则：alice → bob（全部委托，窗口覆盖假时钟）
	rule := baseRule()
	rule.ConsignType = ConsignTypeAll
	rule.ConsignObjs = nil
	if err := svc.Add(ctx, rule); err != nil {
		t.Fatalf("规则保存失败: %v", err)
	}
	// 空待办/缺处理人原样返回
	if msg, err := svc.ApplyOnTodoCreate(ctx, nil); err != nil || msg != nil {
		t.Errorf("空待办应原样返回: %v, %v", msg, err)
	}
	blank := &MessageExtend{TaskID: "t-x"}
	if msg, err := svc.ApplyOnTodoCreate(ctx, blank); err != nil || msg.OwnerID != "" {
		t.Errorf("缺处理人应原样返回: %v, %v", msg, err)
	}
	// 发起流程并播种待办（alice 的 apply 任务）
	instance, task := fx.start(t, "flowComplete", "biz-delegate-1", "alice")
	fx.seedFirstTodo(t, instance, task)
	todo, found, err := fx.messages.GetByTaskID(ctx, task.ID)
	if err != nil || !found {
		t.Fatalf("待办未播种: %v %v", found, err)
	}
	// 钩子改派：owner → bob，consigner → bob（受托人），镜像落库
	updated, err := svc.ApplyOnTodoCreate(ctx, todo)
	if err != nil {
		t.Fatalf("改派失败: %v", err)
	}
	if updated.OwnerID != "bob" || updated.ConsignerID != "bob" || updated.OwnerName != "李四" {
		t.Errorf("改派结果错误: owner=%s consigner=%s name=%q", updated.OwnerID, updated.ConsignerID, updated.OwnerName)
	}
	// 待办行已重建为 bob
	rebuilt, found, err := fx.messages.GetByTaskID(ctx, task.ID)
	if err != nil || !found || rebuilt.OwnerID != "bob" {
		t.Fatalf("待办未重建: %v %v", found, err)
	}
	// 委托镜像：consigner=委托人 alice，entrust=bob
	list, err := mirrors.ListByTaskID(ctx, task.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("镜像未落库: %d, %v", len(list), err)
	}
	if list[0].ConsignerID != "alice" || list[0].EntrustID != "bob" {
		t.Errorf("镜像字段错误: consigner=%s entrust=%s", list[0].ConsignerID, list[0].EntrustID)
	}
	if got, _ := mirrors.ListByEntrust(ctx, "bob"); len(got) != 1 {
		t.Errorf("按被委托人查询镜像错误: %d", len(got))
	}
	// 未命中委托的待办原样返回
	instance2, task2 := fx.start(t, "flowComplete", "biz-delegate-2", "carl")
	fx.seedFirstTodo(t, instance2, task2)
	todo2, _, _ := fx.messages.GetByTaskID(ctx, task2.ID)
	if msg, err := svc.ApplyOnTodoCreate(ctx, todo2); err != nil || msg.OwnerID != "carl" {
		t.Errorf("未命中应原样返回: %v, %v", msg.OwnerID, err)
	}
}

// TestApplyOnTodoCreateWithoutMirror 验证镜像存储缺失时改派仍生效。
// @param t 测试上下文
func TestApplyOnTodoCreateWithoutMirror(t *testing.T) {
	fx := newCompleteFixture(t)
	store := NewMemTaskDelegateStore(fx.fakeClock)
	svc := NewDelegateRuleService(store, nil, nil, nil, nil, nil, nil, nil, fx.fakeClock)
	ctx := context.Background()
	rule := baseRule()
	rule.ConsignType = ConsignTypeAll
	rule.ConsignObjs = nil
	if err := svc.Add(ctx, rule); err != nil {
		t.Fatalf("规则保存失败: %v", err)
	}
	// 无待办服务/镜像：命中后仅返回改派视图，不触碰存储
	msg := &MessageExtend{TaskID: "t-1", OwnerID: "alice", TenantID: "dev", FlowCategoryCode: "flowComplete"}
	updated, err := svc.ApplyOnTodoCreate(ctx, msg)
	if err != nil {
		t.Fatalf("改派失败: %v", err)
	}
	if updated.OwnerID != "bob" || updated.OwnerName != "" {
		t.Errorf("改派视图错误: owner=%s name=%q", updated.OwnerID, updated.OwnerName)
	}
}

// TestAdjustTaskOwner 验证待办调整（多 owner 复制/清理/assignee/镜像/WP 记录）。
// @param t 测试上下文
func TestAdjustTaskOwner(t *testing.T) {
	fx, svc, _, mirrors := newDelegateFixture(t)
	ctx := context.Background()
	// 发起流程：alice 的 apply 任务
	instance, task := fx.start(t, "flowComplete", "biz-adjust-1", "alice")
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
	// 校验链
	keep := true
	if err := svc.AdjustTaskOwner(ctx, AdjustOwnerRequest{CurrentOwnerID: "alice", UserCode: "alice", OwnerIDs: []string{"carl"}, KeepCurrentTask: &keep}); err == nil || !strings.Contains(err.Error(), "taskId") {
		t.Errorf("taskId 缺失应报错: %v", err)
	}
	if err := svc.AdjustTaskOwner(ctx, AdjustOwnerRequest{TaskID: task.ID, UserCode: "alice", OwnerIDs: []string{"carl"}, KeepCurrentTask: &keep}); err == nil || !strings.Contains(err.Error(), "currentOwnerId") {
		t.Errorf("currentOwnerId 缺失应报错: %v", err)
	}
	if err := svc.AdjustTaskOwner(ctx, AdjustOwnerRequest{TaskID: task.ID, CurrentOwnerID: "alice", OwnerIDs: []string{"carl"}, KeepCurrentTask: &keep}); err == nil || !strings.Contains(err.Error(), "userCode") {
		t.Errorf("userCode 缺失应报错: %v", err)
	}
	if err := svc.AdjustTaskOwner(ctx, AdjustOwnerRequest{TaskID: task.ID, CurrentOwnerID: "alice", UserCode: "alice", KeepCurrentTask: &keep}); err == nil || !strings.Contains(err.Error(), "ownerIds") {
		t.Errorf("ownerIds 缺失应报错: %v", err)
	}
	if err := svc.AdjustTaskOwner(ctx, AdjustOwnerRequest{TaskID: task.ID, CurrentOwnerID: "alice", UserCode: "alice", OwnerIDs: []string{"alice"}, KeepCurrentTask: &keep}); err == nil || !strings.Contains(err.Error(), "不能委托任务给当前办理人") {
		t.Errorf("委托给本人应报错: %v", err)
	}
	if err := svc.AdjustTaskOwner(ctx, AdjustOwnerRequest{TaskID: "no-such", CurrentOwnerID: "alice", UserCode: "alice", OwnerIDs: []string{"carl"}, KeepCurrentTask: &keep}); err == nil || !strings.Contains(err.Error(), "任务不存在") {
		t.Errorf("任务不存在应报错: %v", err)
	}
	// 处理人不存在（alice 存在，改用未知人触发）
	if err := svc.AdjustTaskOwner(ctx, AdjustOwnerRequest{TaskID: task.ID, CurrentOwnerID: "ghost", UserCode: "alice", OwnerIDs: []string{"carl"}, KeepCurrentTask: &keep}); err == nil || !strings.Contains(err.Error(), "任务处理人不存在") {
		t.Errorf("处理人不存在应报错: %v", err)
	}
	// 正常调整：alice → carl + helper（多人），不保留原待办
	if err := svc.AdjustTaskOwner(ctx, AdjustOwnerRequest{
		TaskID:          task.ID,
		CurrentOwnerID:  "alice",
		UserCode:        "alice",
		OwnerIDs:        []string{"carl", "helper"},
		KeepCurrentTask: nil, // nil 视为不保留
	}); err != nil {
		t.Fatalf("调整失败: %v", err)
	}
	// 预存记录：handle 节点新增 carl/helper 两条（used=0），原 alice 记录被删
	records, err := fx.next.ListByTaskID(ctx, task.ID)
	if err != nil {
		t.Fatalf("查询预存失败: %v", err)
	}
	owners := make(map[string]bool, 2)
	for _, record := range records {
		owners[record.IdentityID] = true
	}
	if !owners["carl"] || !owners["helper"] || owners["alice"] {
		t.Errorf("预存记录复制/清理错误: %v", owners)
	}
	// 引擎任务 assignee 改写为首个新 owner
	updatedTask, err := fx.engine.TaskService().GetTask(ctx, task.ID)
	if err != nil || updatedTask.Assignee != "carl" {
		t.Fatalf("assignee 未改写: %v, %q", err, updatedTask.Assignee)
	}
	// 待办改派给首个新 owner，consigner 记录原处理人
	todo, found, _ := fx.messages.GetByTaskID(ctx, task.ID)
	if !found || todo.OwnerID != "carl" || todo.ConsignerID != "alice" {
		t.Errorf("待办改派错误: found=%v owner=%s consigner=%s", found, todo.OwnerID, todo.ConsignerID)
	}
	// 委托镜像逐 owner 落库
	list, _ := mirrors.ListByTaskID(ctx, task.ID)
	if len(list) != 2 {
		t.Fatalf("镜像应为 2 条: %d", len(list))
	}
	if list[0].EntrustID != "carl" || list[1].EntrustID != "helper" {
		t.Errorf("镜像顺序/内容错误: %s, %s", list[0].EntrustID, list[1].EntrustID)
	}
	// WP 操作记录
	ops, err := fx.stores.Operation.ListByTask(ctx, task.ID)
	if err != nil || len(ops) == 0 {
		t.Fatalf("操作记录缺失: %d, %v", len(ops), err)
	}
	lastOp := ops[len(ops)-1]
	if lastOp.Type != CommentTypeWP || !strings.Contains(lastOp.Message, "委托任务") {
		t.Errorf("WP 记录错误: type=%s msg=%s", lastOp.Type, lastOp.Message)
	}
	// 实例级镜像删除
	if err := mirrors.DeleteByProcessInstance(ctx, instance.ID); err != nil {
		t.Fatalf("删除镜像失败: %v", err)
	}
	if list, _ := mirrors.ListByTaskID(ctx, task.ID); len(list) != 0 {
		t.Errorf("删除后镜像应为空: %d", len(list))
	}
}

// TestAdjustTaskOwnerKeepCurrent 验证保留原待办模式（keepCurrentTask=true）。
// @param t 测试上下文
func TestAdjustTaskOwnerKeepCurrent(t *testing.T) {
	fx, svc, _, _ := newDelegateFixture(t)
	ctx := context.Background()
	instance, task := fx.start(t, "flowComplete", "biz-adjust-2", "alice")
	fx.seedFirstTodo(t, instance, task)
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
	keep := true
	if err := svc.AdjustTaskOwner(ctx, AdjustOwnerRequest{
		TaskID: task.ID, CurrentOwnerID: "alice", UserCode: "alice",
		OwnerIDs: []string{"carl"}, KeepCurrentTask: &keep,
	}); err != nil {
		t.Fatalf("调整失败: %v", err)
	}
	// 预存记录保留 alice 并新增 carl
	records, _ := fx.next.ListByTaskID(ctx, task.ID)
	owners := make(map[string]bool, 2)
	for _, record := range records {
		owners[record.IdentityID] = true
	}
	if !owners["alice"] || !owners["carl"] {
		t.Errorf("保留模式下预存记录错误: %v", owners)
	}
	// 原待办保留给 alice
	todo, found, _ := fx.messages.GetByTaskID(ctx, task.ID)
	if !found || todo.OwnerID != "alice" {
		t.Errorf("保留模式下待办应保留: found=%v owner=%s", found, todo.OwnerID)
	}
	// 无预存记录时复制跳过（不报错）
	instance2, task2 := fx.start(t, "flowComplete", "biz-adjust-3", "alice")
	fx.seedFirstTodo(t, instance2, task2)
	if err := svc.AdjustTaskOwner(ctx, AdjustOwnerRequest{
		TaskID: task2.ID, CurrentOwnerID: "alice", UserCode: "alice",
		OwnerIDs: []string{"carl"}, KeepCurrentTask: &keep,
	}); err != nil {
		t.Fatalf("无预存调整失败: %v", err)
	}
}

// TestMemTaskDelegateStoreEdge 验证内存存储边界（空入参/未知 Id 更新/镜像逗号串）。
// @param t 测试上下文
func TestMemTaskDelegateStoreEdge(t *testing.T) {
	store := NewMemTaskDelegateStore(nil)
	ctx := context.Background()
	// 空规则/缺委托人
	if err := store.Save(ctx, nil); err == nil {
		t.Error("空规则应报错")
	}
	if err := store.Save(ctx, &DelegateRule{SourceCode: "dev"}); err == nil {
		t.Error("缺委托人应报错")
	}
	// 子表 Id 缺失自动生成
	rule := &DelegateRule{
		BaseColumns: BaseColumns{TenantID: "dev"},
		OwnerID:     "alice", SourceCode: "dev", Enable: DelegateEnabled,
		Consigners:  []DelegateConsigner{{ConsignerID: "bob"}},
		ConsignObjs: []DelegateConsignObj{{OwnerID: "proj-1"}},
	}
	if err := store.Save(ctx, rule); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if rule.Consigners[0].ID == "" || rule.ConsignObjs[0].ID == "" {
		t.Error("子表 Id 应自动生成")
	}
	// ListByConsigner 反查
	if list, _ := store.ListByConsigner(ctx, "bob", "dev"); len(list) != 1 {
		t.Errorf("被委托人反查应命中: %d", len(list))
	}
	if list, _ := store.ListByConsigner(ctx, "ghost", "dev"); len(list) != 0 {
		t.Errorf("未知被委托人不应命中: %d", len(list))
	}
	// 停用规则不参与反查
	if err := store.UpdateState(ctx, rule.ID, DelegateDisabled, DelegateDisabled); err != nil {
		t.Fatalf("停用失败: %v", err)
	}
	if list, _ := store.ListByConsigner(ctx, "bob", "dev"); len(list) != 0 {
		t.Errorf("停用规则不应反查命中: %d", len(list))
	}
	// 未知 Id 更新静默
	if err := store.UpdateState(ctx, "no-such", 1, 0); err != nil {
		t.Errorf("未知 Id 更新应静默: %v", err)
	}
	_ = NewMemTaskDelegateStore(clock.NewFake(time.Now()))
	// 镜像存储边界
	mirror := NewMemDelegateTaskMirrorStore(nil)
	if err := mirror.Save(ctx, nil); err == nil {
		t.Error("空镜像应报错")
	}
	if err := mirror.Save(ctx, &DelegateTaskMirror{TaskID: "t-1"}); err == nil {
		t.Error("缺被委托人应报错")
	}
	// 逗号串包含判定
	if !mirrorContainsID("a, b ,c", "b") {
		t.Error("逗号串应包含 b")
	}
	if mirrorContainsID("a,b", "") {
		t.Error("空 Id 不应命中")
	}
	if mirrorContainsID("a,b", "c") {
		t.Error("c 不应在串内")
	}
}

// ─── SQL 适配（sqlmock） ─────────────────────────────────────────────

// newDelegateSQLStores 创建委托三表 + 镜像 SQL 适配器与其 sqlmock。
// @param t 测试上下文
// @return 规则适配器
// @return 镜像适配器
// @return 数据库模拟
func newDelegateSQLStores(t *testing.T) (*SQLTaskDelegateStore, *SQLDelegateTaskMirrorStore, sqlmock.Sqlmock) {
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
	return NewSQLTaskDelegateStore(store), NewSQLDelegateTaskMirrorStore(store), mock
}

// delegateMainColumns 是委托主表查询列清单文本（与表定义顺序一致）。
const delegateMainColumns = "id, consign_type, owner_id, owner_name, source, source_code, source_name, type, condition, vaild_from, vaild_to, enable, retire, remark, creator, creator_name, create_time, updater, update_time, tenant_id"

// TestSQLTaskDelegateStoreSaveAndGet 验证主表 upsert 与级联回读。
// @param t 测试上下文
func TestSQLTaskDelegateStoreSaveAndGet(t *testing.T) {
	rules, mirrors, mock := newDelegateSQLStores(t)
	ctx := context.Background()
	stamped := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	rule := &DelegateRule{
		BaseColumns: BaseColumns{ID: "d-1", TenantID: "dev", CreateTime: stamped, UpdateTime: stamped},
		ConsignType: ConsignTypeSingleProject, OwnerID: "alice", SourceCode: "dev",
		VaildFrom: stamped, VaildTo: stamped.AddDate(0, 0, 30), Enable: 1,
		Consigners:  []DelegateConsigner{{ID: "dc-1", ConsignerID: "bob"}},
		ConsignObjs: []DelegateConsignObj{{ID: "do-1", OwnerID: "proj-1"}},
	}
	// 新增路径：UPDATE 0 行 → 插入主表 → 重建子表
	mock.ExpectExec("UPDATE workflow_task_delegate SET").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO workflow_task_delegate").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("DELETE FROM workflow_task_consigners").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO workflow_task_consigners").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("DELETE FROM workflow_task_consign_obj").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO workflow_task_consign_obj").WillReturnResult(sqlmock.NewResult(1, 1))
	if err := rules.Save(ctx, rule); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	// 更新路径：UPDATE 1 行 → 直接重建子表
	mock.ExpectExec("UPDATE workflow_task_delegate SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM workflow_task_consigners").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO workflow_task_consigners").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("DELETE FROM workflow_task_consign_obj").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO workflow_task_consign_obj").WillReturnResult(sqlmock.NewResult(1, 1))
	if err := rules.Save(ctx, rule); err != nil {
		t.Fatalf("更新保存失败: %v", err)
	}
	// 空入参
	if err := rules.Save(ctx, &DelegateRule{OwnerID: "alice"}); err == nil {
		t.Error("缺来源代号应报错")
	}
	// 级联回读
	mock.ExpectQuery("SELECT (.+) FROM workflow_task_delegate WHERE").WithArgs("d-1").WillReturnRows(
		sqlmock.NewRows(strings.Split(delegateMainColumns, ", ")).
			AddRow("d-1", ConsignTypeSingleProject, "alice", "张三", "", "dev", "OA", "", "", stamped, stamped.AddDate(0, 0, 30), 1, 0, "", "", "", stamped, "", stamped, "dev"))
	mock.ExpectQuery("SELECT (.+) FROM workflow_task_consigners WHERE").WithArgs("d-1").WillReturnRows(
		sqlmock.NewRows([]string{"id", "delegate_id", "consigner_id", "consigner_name"}).AddRow("dc-1", "d-1", "bob", "李四"))
	mock.ExpectQuery("SELECT (.+) FROM workflow_task_consign_obj WHERE").WithArgs("d-1").WillReturnRows(
		sqlmock.NewRows([]string{"id", "delegate_id", "owner_type", "owner_id", "owner_name"}).AddRow("do-1", "d-1", ConsignTypeSingleProject, "proj-1", "项目一"))
	got, found, err := rules.Get(ctx, "d-1")
	if err != nil || !found || len(got.Consigners) != 1 || len(got.ConsignObjs) != 1 {
		t.Fatalf("级联回读失败: %v %v %+v", found, err, got)
	}
	// 查无
	mock.ExpectQuery("SELECT (.+) FROM workflow_task_delegate WHERE").WithArgs("d-x").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if _, found, _ := rules.Get(ctx, "d-x"); found {
		t.Error("查无应返回不存在")
	}
	// 删除级联
	mock.ExpectExec("DELETE FROM workflow_task_consigners").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM workflow_task_consign_obj").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM workflow_task_delegate").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := rules.Delete(ctx, "d-1"); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	// UpdateState
	mock.ExpectExec("UPDATE workflow_task_delegate SET").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := rules.UpdateState(ctx, "d-1", 0, 1); err != nil {
		t.Fatalf("更新状态失败: %v", err)
	}
	// 镜像落库与查询
	mirror := &DelegateTaskMirror{
		MessageExtend: MessageExtend{TaskID: "task-1", ProcessInstanceID: "pi-1"},
		EntrustID:     "bob", EntrustName: "李四",
	}
	mirror.BaseColumns = BaseColumns{ID: "m-1", TenantID: "dev"}
	mock.ExpectExec("INSERT INTO workflow_delegate_task").WillReturnResult(sqlmock.NewResult(1, 1))
	if err := mirrors.Save(ctx, mirror); err != nil {
		t.Fatalf("镜像保存失败: %v", err)
	}
	if err := mirrors.Save(ctx, &DelegateTaskMirror{TaskID: "task-1"}); err == nil {
		t.Error("缺被委托人应报错")
	}
	mirrorRow := sqlmock.NewRows([]string{"id", "task_id", "process_instance_id", "entrust_id", "entrust_name", "owner_id"}).
		AddRow("m-1", "task-1", "pi-1", "bob", "李四", "bob")
	mock.ExpectQuery("SELECT (.+) FROM workflow_delegate_task WHERE").WithArgs("task-1").WillReturnRows(mirrorRow)
	if list, err := mirrors.ListByTaskID(ctx, "task-1"); err != nil || len(list) != 1 || list[0].EntrustID != "bob" {
		t.Fatalf("镜像回读错误: %d, %v", len(list), err)
	}
	// sqlmock 行不可复用，按被委托人查询使用独立行对象
	entrustRow := sqlmock.NewRows([]string{"id", "task_id", "process_instance_id", "entrust_id", "entrust_name", "owner_id"}).
		AddRow("m-1", "task-1", "pi-1", "bob", "李四", "bob")
	mock.ExpectQuery("SELECT (.+) FROM workflow_delegate_task WHERE").WithArgs("bob").WillReturnRows(entrustRow)
	if list, err := mirrors.ListByEntrust(ctx, "bob"); err != nil || len(list) != 1 {
		t.Fatalf("按被委托人回读错误: %d, %v", len(list), err)
	}
	mock.ExpectExec("DELETE FROM workflow_delegate_task").WillReturnResult(sqlmock.NewResult(0, 2))
	if err := mirrors.DeleteByProcessInstance(ctx, "pi-1"); err != nil {
		t.Fatalf("镜像删除失败: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("未满足的期望: %v", err)
	}
}

// TestSQLTaskDelegateStoreListByOwner 验证按 owner 的启用查询、租户过滤与被委托人反查。
// @param t 测试上下文
func TestSQLTaskDelegateStoreListByOwner(t *testing.T) {
	rules, _, mock := newDelegateSQLStores(t)
	ctx := context.Background()
	stamped := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	// ListByOwner（仓储仅回启用规则；两条分属不同租户）
	mock.ExpectQuery("SELECT (.+) FROM workflow_task_delegate").WithArgs("alice").WillReturnRows(
		sqlmock.NewRows([]string{"id", "consign_type", "owner_id", "source_code", "vaild_from", "vaild_to", "enable", "retire"}).
			AddRow("d-1", ConsignTypeAll, "alice", "dev", stamped, stamped.AddDate(0, 0, 30), 1, 0).
			AddRow("d-2", ConsignTypeAll, "alice", "other", stamped, stamped.AddDate(0, 0, 30), 1, 0))
	// 级联装载（仅 dev 租户的一条需要装载子表）
	mock.ExpectQuery("SELECT (.+) FROM workflow_task_consigners WHERE").WithArgs("d-1").WillReturnRows(
		sqlmock.NewRows([]string{"id", "delegate_id", "consigner_id", "consigner_name"}).AddRow("dc-1", "d-1", "bob", "李四"))
	mock.ExpectQuery("SELECT (.+) FROM workflow_task_consign_obj WHERE").WithArgs("d-1").WillReturnRows(
		sqlmock.NewRows([]string{"id", "delegate_id", "owner_type", "owner_id", "owner_name"}))
	list, err := rules.ListByOwner(ctx, "alice", "dev")
	if err != nil || len(list) != 1 || list[0].ID != "d-1" {
		t.Fatalf("租户过滤错误: %d, %v", len(list), err)
	}
	// ListByConsigner：按来源代号查询启用规则后逐规则核验被委托人
	mock.ExpectQuery("SELECT (.+) FROM workflow_task_delegate").WithArgs("dev").WillReturnRows(
		sqlmock.NewRows([]string{"id", "consign_type", "owner_id", "source_code", "vaild_from", "vaild_to", "enable", "retire"}).
			AddRow("d-1", ConsignTypeAll, "alice", "dev", stamped, stamped.AddDate(0, 0, 30), 1, 0))
	mock.ExpectQuery("SELECT (.+) FROM workflow_task_consigners WHERE").WithArgs("d-1").WillReturnRows(
		sqlmock.NewRows([]string{"id", "delegate_id", "consigner_id", "consigner_name"}).AddRow("dc-1", "d-1", "bob", "李四"))
	consigned, err := rules.ListByConsigner(ctx, "bob", "dev")
	if err != nil || len(consigned) != 1 || consigned[0].ID != "d-1" {
		t.Fatalf("被委托人反查错误: %d, %v", len(consigned), err)
	}
	// 空租户反查直接空集
	if list, err := rules.ListByConsigner(ctx, "bob", ""); err != nil || len(list) != 0 {
		t.Errorf("空租户反查应返回空集: %d, %v", len(list), err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("未满足的期望: %v", err)
	}
}
