package workflow

import (
	"context"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// TestParseWeightConfigs_JavaIDKeyCompat 验证权重配置兼容 Java 设计器写入的 "id" 键。
// 016 号审计：Go 此前只读 "identityId"，按 Java 格式（ProcessesUtil.getCountersignWeight
// 读 "id"）配置的权重会永远匹配失败并抛 WORKFLOW-2011。
// @param t 测试上下文
func TestParseWeightConfigs_JavaIDKeyCompat(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantID  string
		wantAgr float64
	}{
		{name: "Java id 键", raw: `[{"id":"alice","agreeWeight":2,"rejectWeight":1}]`, wantID: "alice", wantAgr: 2},
		{name: "Go identityId 键", raw: `[{"identityId":"bob","agreeWeight":3,"rejectWeight":1}]`, wantID: "bob", wantAgr: 3},
		{name: "两键并存以 id 为准", raw: `[{"id":"carol","identityId":"legacy","agreeWeight":5}]`, wantID: "carol", wantAgr: 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			configs := parseWeightConfigs(tc.raw)
			if len(configs) != 1 {
				t.Fatalf("应解析 1 条权重: %+v", configs)
			}
			if configs[0].IdentityID != tc.wantID {
				t.Fatalf("身份键期望 %q，实际 %q", tc.wantID, configs[0].IdentityID)
			}
			if configs[0].AgreeWeight == nil || *configs[0].AgreeWeight != tc.wantAgr {
				t.Fatalf("通过权重期望 %v，实际 %+v", tc.wantAgr, configs[0].AgreeWeight)
			}
		})
	}
}

// TestCompleteTaskVo_CountersignResultSet 验证办结请求区分"未传"与"显式 0"。
// 016 号审计：Java CompleteTaskVo 该字段为 Integer，null 时抛"未知会签任务执行结果"；
// Go 裸 int 无法区分，缺省会静默记为否决并扣 rejectWeight。
// @param t 测试上下文
func TestCompleteTaskVo_CountersignResultSet(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		wantSet bool
		want    int
	}{
		{name: "缺省未设置", payload: `{"taskId":"t1"}`, wantSet: false, want: 0},
		{name: "显式 null 视为未设置", payload: `{"taskId":"t1","countersignResult":null}`, wantSet: false, want: 0},
		{name: "显式 0 否决", payload: `{"taskId":"t1","countersignResult":0}`, wantSet: true, want: 0},
		{name: "显式 1 通过", payload: `{"taskId":"t1","countersignResult":1}`, wantSet: true, want: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var vo api.CompleteTaskVo
			if err := vo.UnmarshalJSON([]byte(tc.payload)); err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if vo.CountersignResultSet != tc.wantSet {
				t.Fatalf("是否显式设置期望 %v，实际 %v", tc.wantSet, vo.CountersignResultSet)
			}
			if vo.CountersignResult != tc.want {
				t.Fatalf("会签结论期望 %d，实际 %d", tc.want, vo.CountersignResult)
			}
		})
	}
}

// TestIsStepRuleLookupError 验证规则定位失败与存储故障被区别对待。
// 定位失败按"未启用比例退回"处理，存储故障继续上抛。
// @param t 测试上下文
func TestIsStepRuleLookupError(t *testing.T) {
	lookup := api.NewError(errCodeStepTaskMissing, "任务不存在", nil)
	if !isStepRuleLookupError(lookup) {
		t.Fatalf("任务不存在应判定为定位失败")
	}
	storage := api.NewError("WORKFLOW-2107", "规则库不可用", nil)
	if isStepRuleLookupError(storage) {
		t.Fatalf("存储故障不应判定为定位失败")
	}
	if isStepRuleLookupError(nil) {
		t.Fatalf("nil 不应判定为定位失败")
	}
}

// TestCountersignPercentResult_LookupMissDisabled 验证任务已不存在时按未启用返回。
// 016 号审计：规则端口接入后，普通实例调用 /process/complete/sign 一度退化为失败。
// @param t 测试上下文
func TestCountersignPercentResult_LookupMissDisabled(t *testing.T) {
	fx := newCountersignFixture(t)
	fx.rules.enabled = true
	fx.rules.rule = &PercentReturnRule{CountersignPercentReturn: true}
	fx.rules.err = api.NewError(errCodeStepTaskMissing, "任务不存在", nil)
	passed, err := fx.cs.CountersignPercentResult(context.Background(), "missing-task", 1)
	if err != nil {
		t.Fatalf("定位失败应返回未启用而非错误: %v", err)
	}
	if passed {
		t.Fatalf("未启用比例退回不应判定通过")
	}
}
