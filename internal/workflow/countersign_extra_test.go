package workflow

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/DengQingNian/Flowtide/api"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
)

// fakeGroupMembers 是组成员查询的测试桩（记录组 ID，可注入错误）。
type fakeGroupMembers struct {
	users map[string][]string // 组 ID → 用户 ID 列表
	err   map[string]error    // 组 ID → 注入错误
}

// ListMembers 返回组成员。
// @param ctx 操作上下文
// @param groupID 组 ID
// @return 用户列表
// @return 错误
// @param _ 接口占位参数（实现不使用）
func (f *fakeGroupMembers) ListMembers(_ context.Context, groupID string) ([]api.User, error) {
	// 注入错误优先
	if err, ok := f.err[groupID]; ok {
		return nil, err
	}
	ids := f.users[groupID]
	out := make([]api.User, 0, len(ids))
	for _, id := range ids {
		out = append(out, api.User{ID: id})
	}
	return out, nil
}

// TestCountersign_ExecutorExpansion 验证模型执行者 org/role 经组成员查询展开与降级。
// @param t 测试上下文
func TestCountersign_ExecutorExpansion(t *testing.T) {
	fx := newCountersignFixture(t)
	members := &fakeGroupMembers{
		users: map[string][]string{"orgA": {"u1", "u2"}, "roleB": {"u3"}},
		err:   map[string]error{"roleBad": assertError("身份库不可用")},
	}
	svc := NewCountersignService(fx.engine, fx.meta, WithCountersignMembers(members))
	// org/role 展开 + person 直用（保持配置顺序）
	meta := &UserTaskMeta{ExecutorListRaw: `[{"executorType":"org","orgId":"orgA"},{"executorType":"role","roleId":"roleB"},{"identityId":"u9"}]`}
	ids := svc.modelExecutorIDs(context.Background(), meta)
	if len(ids) != 4 || ids[0] != "u1" || ids[1] != "u2" || ids[2] != "u3" || ids[3] != "u9" {
		t.Fatalf("org/role 应展开: %v", ids)
	}
	// 条目无类型时回退节点级 executorType
	nodeTyped := &UserTaskMeta{ExecutorType: "role", ExecutorListRaw: `[{"roleId":"roleB"}]`}
	ids = svc.modelExecutorIDs(context.Background(), nodeTyped)
	if len(ids) != 1 || ids[0] != "u3" {
		t.Fatalf("节点级类型应回退生效: %v", ids)
	}
	// 成员查询错误：保留组 ID 原值兜底
	broken := &UserTaskMeta{ExecutorListRaw: `[{"executorType":"role","roleId":"roleBad"}]`}
	ids = svc.modelExecutorIDs(context.Background(), broken)
	if len(ids) != 1 || ids[0] != "roleBad" {
		t.Fatalf("展开失败应保留组 ID: %v", ids)
	}
	// 组 ID 为空：跳过
	empty := &UserTaskMeta{ExecutorListRaw: `[{"executorType":"org"}]`}
	if ids = svc.modelExecutorIDs(context.Background(), empty); len(ids) != 0 {
		t.Fatalf("空组 ID 应跳过: %v", ids)
	}
	// 未注入成员查询：保留组 ID 原值（Java 原样透传兜底）
	bare := NewCountersignService(fx.engine, fx.meta)
	raw := &UserTaskMeta{ExecutorListRaw: `[{"executorType":"org","orgId":"orgA"}]`}
	ids = bare.modelExecutorIDs(context.Background(), raw)
	if len(ids) != 1 || ids[0] != "orgA" {
		t.Fatalf("未注入展开应保留组 ID: %v", ids)
	}
	// 逗号分隔退化形态（person 直用）
	comma := &UserTaskMeta{ExecutorListRaw: "u1, u2,u3"}
	ids = bare.modelExecutorIDs(context.Background(), comma)
	if len(ids) != 3 || ids[0] != "u1" {
		t.Fatalf("逗号形态应解析: %v", ids)
	}
}

// TestCountersign_TypeHelpers 验证变量类型转换工具的全分支。
// @param t 测试上下文
func TestCountersign_TypeHelpers(t *testing.T) {
	// intOf 全类型分支
	cases := []struct {
		value any
		want  int
	}{
		{value: 3, want: 3},
		{value: int32(4), want: 4},
		{value: int64(5), want: 5},
		{value: float32(6.7), want: 6},
		{value: 7.9, want: 7},
		{value: "8", want: 8},
		{value: "bad", want: 0},
		{value: nil, want: 0},
		{value: true, want: 0},
	}
	for _, c := range cases {
		if got := intOf(c.value); got != c.want {
			t.Fatalf("intOf(%#v) 应为 %d: %d", c.value, c.want, got)
		}
	}
	// toStringSlice 分支
	if got := toStringSlice([]string{"a", "b"}); len(got) != 2 || got[0] != "a" {
		t.Fatalf("[]string 应直通: %v", got)
	}
	if got := toStringSlice([]any{"a", 1, " c "}); len(got) != 3 || got[1] != "1" || got[2] != "c" {
		t.Fatalf("[]any 应字符串化: %v", got)
	}
	if got := toStringSlice("not-a-slice"); got != nil {
		t.Fatalf("非切片应为 nil: %v", got)
	}
	if got := toStringSlice(nil); got != nil {
		t.Fatalf("nil 应为 nil: %v", got)
	}
	// customExecutorIDs：变量缺失 / 空列表
	helper := NewCountersignService(nil, nil)
	if got := helper.customExecutorIDs(api.ListenerExecution{}); got != nil {
		t.Fatalf("缺失变量应为 nil: %v", got)
	}
	if got := helper.customExecutorIDs(api.ListenerExecution{Variables: map[string]any{VarCustomExecutorList: []string{}}}); got != nil {
		t.Fatalf("空列表应为 nil: %v", got)
	}
	// randomSort 非布尔：不打乱
	if got := helper.customExecutorIDs(api.ListenerExecution{Variables: map[string]any{
		VarCustomExecutorList: []string{"a", "b"},
		VarRandomSort:         "yes",
	}}); len(got) != 2 || got[0] != "a" {
		t.Fatalf("randomSort 非布尔应保持原序: %v", got)
	}
}

// TestCountersign_DefaultShuffle 验证缺省随机排序为全量置换。
// @param t 测试上下文
func TestCountersign_DefaultShuffle(t *testing.T) {
	fx := newCountersignFixture(t)
	// 覆盖缺省 shuffleFn（rand.Shuffle 闭包）：逆序前先恢复缺省实现
	fx.cs.shuffleFn = nil
	svc := NewCountersignService(fx.engine, fx.meta)
	values := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	svc.shuffleFn(values)
	// 置换语义：多重集合不变
	counts := map[string]int{}
	for _, value := range values {
		counts[value]++
	}
	if len(counts) != 8 {
		t.Fatalf("随机排序应保持元素全集: %v", values)
	}
}

// newWeightSQLAdapter 创建权重 SQL 适配器与其 sqlmock。
// @param t 测试上下文
// @return 适配器
// @return 数据库模拟
func newWeightSQLAdapter(t *testing.T) (*SQLCountersignWeightStore, sqlmock.Sqlmock) {
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
	return NewSQLCountersignWeightStore(store), mock
}

// TestSQLCountersignWeightStore 验证权重 SQL 适配器（先删后插 + 行映射）。
// @param t 测试上下文
func TestSQLCountersignWeightStore(t *testing.T) {
	adapter, mock := newWeightSQLAdapter(t)
	key := CountersignWeightKey{ProcessDefinitionKey: "k1", ActivityID: "a1", TenantID: "dev"}
	// 先删：查旧行 → 逐主键删除
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "activity_id", "process_definition_key", "agree_weight", "reject_weight", "identity_id"}).
			AddRow("old-1", "a1", "k1", 2.5, 1, "alice").
			AddRow("old-2", "a1", "k1", 1, nil, "bob"))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM workflow_countersign_weight")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM workflow_countersign_weight")).WillReturnResult(sqlmock.NewResult(0, 1))
	// 后插：两条新配置
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO workflow_countersign_weight")).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO workflow_countersign_weight")).WillReturnResult(sqlmock.NewResult(2, 1))
	if err := adapter.SaveByNode(context.Background(), key, []CountersignWeight{
		{IdentityID: "alice", AgreeWeight: 2.5, RejectWeight: 1},
		{IdentityID: "carl", AgreeWeight: 3},
	}); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	// 查询映射（数值字符串/NULL 形态）
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "activity_id", "process_definition_key", "agree_weight", "reject_weight", "identity_id"}).
			AddRow("id-1", "a1", "k1", "2.5", nil, "alice").
			AddRow("id-2", "a1", "k1", 3, []byte("4"), "carl"))
	configs, err := adapter.ListByNode(context.Background(), key)
	if err != nil || len(configs) != 2 {
		t.Fatalf("查询失败: %v %v", configs, err)
	}
	if configs[0].AgreeWeight != 2.5 || configs[0].RejectWeight != 0 {
		t.Fatalf("行映射错误: %+v", configs[0])
	}
	if configs[1].AgreeWeight != 3 || configs[1].RejectWeight != 4 {
		t.Fatalf("行映射错误: %+v", configs[1])
	}
	// 定位键不完整
	if err := adapter.SaveByNode(context.Background(), CountersignWeightKey{ActivityID: "a"}, nil); err == nil || !containsText(err, "缺少定义 Key") {
		t.Fatalf("定位键缺失应报错: %v", err)
	}
	// 查询错误透传
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).WillReturnError(assertError("数据库不可用"))
	if _, err := adapter.ListByNode(context.Background(), key); err == nil {
		t.Fatalf("查询错误应上抛")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL 预期未满足: %v", err)
	}
}

// containsText 判断错误信息是否包含文案。
// @param err 错误
// @param text 期望文案
// @return 是否包含
func containsText(err error, text string) bool {
	return err != nil && regexp.MustCompile(regexp.QuoteMeta(text)).MatchString(err.Error())
}
