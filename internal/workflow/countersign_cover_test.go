package workflow

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/DengQingNian/Flowtide/api"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
)

// fakeListenerControl 是监听器控制接口的测试桩（变量落内存映射）。
type fakeListenerControl struct {
	vars     map[string]any // 变量表
	deferred int            // 延迟动作计数
}

// SetVariable 记录实例级变量。
// @param name 变量名
// @param value 变量值
// @return 错误
func (f *fakeListenerControl) SetVariable(name string, value any) error {
	f.vars[name] = value
	return nil
}

// SetVariableLocal 记录局部变量。
// @param name 变量名
// @param value 变量值
// @return 错误
func (f *fakeListenerControl) SetVariableLocal(name string, value any) error {
	f.vars["local:"+name] = value
	return nil
}

// Defer 记录延迟动作。
// @param action 延迟动作
func (f *fakeListenerControl) Defer(_ func(ctx context.Context) error) {
	f.deferred++
}

// failingWeightStore 是返回查询错误的权重存储桩。
type failingWeightStore struct{}

// SaveByNode 空实现。
// @param ctx 操作上下文
// @param key 节点定位键
// @param configs 权重配置
// @return 错误
func (f *failingWeightStore) SaveByNode(_ context.Context, _ CountersignWeightKey, _ []CountersignWeight) error {
	return nil
}

// ListByNode 恒返回错误。
// @param ctx 操作上下文
// @param key 节点定位键
// @return 权重配置
// @return 错误
func (f *failingWeightStore) ListByNode(_ context.Context, _ CountersignWeightKey) ([]CountersignWeight, error) {
	return nil, assertError("权重库不可用")
}

// TestCountersign_ApplyCollectionConfigDirect 验证集合配置主体的直接分支（空节点 ID/预存覆盖）。
// @param t 测试上下文
func TestCountersign_ApplyCollectionConfigDirect(t *testing.T) {
	fx := newCountersignFixture(t)
	ctl := &fakeListenerControl{vars: map[string]any{}}
	// 空 activityID → 会签节点不存在
	if _, err := fx.cs.applyCollectionConfig(context.Background(), api.ListenerExecution{}, ctl); err == nil || !containsText(err, "会签节点不存在") {
		t.Fatalf("空节点 ID 应报错: %v", err)
	}
	// 预存记录覆盖模型执行者（applyCollectionConfig 内的 next_activity 覆盖分支）
	instance, _ := fx.startCountersign(t, "csCollection", "carol", nil)
	definition, err := fx.engine.RepositoryService().GetProcessDefinitionByKey(context.Background(), "csCollection")
	if err != nil {
		t.Fatalf("查询定义失败: %v", err)
	}
	if err := fx.next.SaveAgreeNext(context.Background(), NextActivitySaveRequest{
		ProcessInstanceID: instance.ID,
		ActivityID:        "csTask",
		IdentityIDs:       []string{"y1", "y2"},
		TenantID:          "dev",
		Creator:           "carol",
	}); err != nil {
		t.Fatalf("预存失败: %v", err)
	}
	execution := api.ListenerExecution{ProcessInstanceID: instance.ID, ActivityID: "csTask", ProcessDefinitionID: definition.ID}
	meta, err := fx.cs.applyCollectionConfig(context.Background(), execution, ctl)
	if err != nil {
		t.Fatalf("集合配置失败: %v", err)
	}
	if meta == nil || meta.ActivityID != "csTask" {
		t.Fatalf("应返回节点元数据: %+v", meta)
	}
	// executorList 应被预存记录覆盖（内存存储无稳定顺序，按集合比较）
	list, _ := ctl.vars[VarExecutorList].([]string)
	if len(list) != 2 || !sameStringSet(list, []string{"y1", "y2"}) {
		t.Fatalf("预存记录应覆盖模型执行者: %v", list)
	}
	if got := intOf(ctl.vars[VarNrOfInstances]); got != 2 {
		t.Fatalf("nrOfInstances 应为 2: %v", got)
	}
}

// TestCountersign_WarmupGhost 验证预热不存在定义的错误透传。
// @param t 测试上下文
func TestCountersign_WarmupGhost(t *testing.T) {
	fx := newCountersignFixture(t)
	if err := fx.cs.Warmup(context.Background(), "ghost-def"); err == nil {
		t.Fatal("未定义预热应报错")
	}
}

// TestCountersign_WeightListenerApplyError 验证权重监听器透传集合配置错误。
// @param t 测试上下文
func TestCountersign_WeightListenerApplyError(t *testing.T) {
	fx := newCountersignFixture(t)
	// 清空缓存使集合配置失败（未预热）
	fx.cs.mu.Lock()
	fx.cs.cache = make(map[string]map[string]*UserTaskMeta)
	fx.cs.mu.Unlock()
	_, err := fx.engine.RuntimeService().StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "csWeight",
		BusinessKey:          "biz-werr",
		TenantID:             "dev",
		UserID:               "carol",
		Variables:            []api.Variable{{Name: VarInitiator, Value: "carol"}},
	})
	if err == nil || !containsText(err, "未预热") {
		t.Fatalf("权重监听器应透传集合配置错误: %v", err)
	}
}

// TestCountersign_ListNodeWeightsStoreFallback 验证存储查询失败时回退模型 weightList。
// @param t 测试上下文
func TestCountersign_ListNodeWeightsStoreFallback(t *testing.T) {
	fx := newCountersignFixture(t)
	definition, err := fx.engine.RepositoryService().GetProcessDefinitionByKey(context.Background(), "csWeight")
	if err != nil {
		t.Fatalf("查询定义失败: %v", err)
	}
	// 注入失败存储
	fx.cs.weights = &failingWeightStore{}
	key := CountersignWeightKey{ProcessDefinitionKey: "csWeight", ActivityID: "wTask"}
	configs, err := fx.cs.ListNodeWeights(context.Background(), definition.ID, key)
	if err != nil || len(configs) != 2 {
		t.Fatalf("存储失败应回退模型: %v %v", configs, err)
	}
	// weightsFromConfigs 空身份跳过分支
	skip := weightsFromConfigs([]ExecutorConfig{{IdentityID: ""}, {IdentityID: "u1", AgreeWeight: 1}})
	if len(skip) != 1 || skip[0].IdentityID != "u1" {
		t.Fatalf("空身份应跳过: %+v", skip)
	}
}

// TestCountersign_PercentReturnRuleError 验证规则端口错误的透传。
// @param t 测试上下文
func TestCountersign_PercentReturnRuleError(t *testing.T) {
	fx := newCountersignFixture(t)
	fx.rules.rule = nil
	fx.rules.enabled = true
	fx.rules.err = assertError("规则库不可用")
	_, tasks := fx.startCountersign(t, "csCollection", "carol", nil)
	_, err := fx.cs.CountersignPercentResult(context.Background(), tasks[0].ID, 1)
	if err == nil || !containsText(err, "规则库不可用") {
		t.Fatalf("规则错误应透传: %v", err)
	}
}

// TestRowFloatOf 验证通用行值转浮点的全分支。
// @param t 测试上下文
func TestRowFloatOf(t *testing.T) {
	cases := []struct {
		value any
		want  float64
	}{
		{value: nil, want: 0},
		{value: 1.5, want: 1.5},
		{value: float32(2.5), want: 2.5},
		{value: 3, want: 3},
		{value: int32(4), want: 4},
		{value: int64(5), want: 5},
		{value: []byte("6.5"), want: 6.5},
		{value: []byte("bad"), want: 0},
		{value: "7.5", want: 7.5},
		{value: "bad", want: 0},
		{value: true, want: 0},
	}
	for _, c := range cases {
		if got := rowFloatOf(c.value); got != c.want {
			t.Fatalf("rowFloatOf(%#v) 应为 %v: %v", c.value, c.want, got)
		}
	}
}

// TestSQLCountersignWeightStoreErrors 验证权重 SQL 适配器的错误分支。
// @param t 测试上下文
func TestSQLCountersignWeightStoreErrors(t *testing.T) {
	adapter, mock := newWeightSQLAdapter(t)
	key := CountersignWeightKey{ProcessDefinitionKey: "k1", ActivityID: "a1"}
	// 查询旧行失败
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).WillReturnError(assertError("查询失败"))
	if err := adapter.SaveByNode(context.Background(), key, nil); err == nil {
		t.Fatal("查询失败应上抛")
	}
	// 删除旧行失败
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("old-1"))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM workflow_countersign_weight")).WillReturnError(assertError("删除失败"))
	if err := adapter.SaveByNode(context.Background(), key, nil); err == nil {
		t.Fatal("删除失败应上抛")
	}
	// 插入失败（空身份行被 continue 跳过后再插入报错）
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO workflow_countersign_weight")).WillReturnError(assertError("插入失败"))
	if err := adapter.SaveByNode(context.Background(), key, []CountersignWeight{{IdentityID: ""}, {IdentityID: "u1"}}); err == nil {
		t.Fatal("插入失败应上抛")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL 预期未满足: %v", err)
	}
	// 与用例无关的引用保持（pwf 仓储入口经适配器构造）
	_ = pwf.DialectMySQL
}
