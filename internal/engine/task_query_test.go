package engine

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/flowable-go/flowable/api"
	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
	"github.com/flowable-go/flowable/internal/tenant"
)

// newTaskQueryEngine 创建注入了 sqlmock 行仓储的测试引擎。
// @param t 测试上下文
// @return 引擎
// @return SQL 断言器
func newTaskQueryEngine(t *testing.T) (*Engine, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("创建 sqlmock 失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repository, err := flowablerow.NewRepository(flowablerow.SQLDB{DB: db}, "mysql")
	if err != nil {
		t.Fatalf("创建行仓储失败: %v", err)
	}
	engine := newTestEngine(t)
	engine.rowRepository = repository
	return engine, mock
}

// TestCandidateGroupsForUser 验证候选组从成员关系推导的语义。
// @param t 测试上下文
func TestCandidateGroupsForUser(t *testing.T) {
	if groups := candidateGroupsForUser(nil, ""); groups != nil {
		t.Fatalf("空用户不应产生候选组: %v", groups)
	}
	memberships := map[string]bool{
		"alice|g2": true,
		"alice|g1": true,
		"bob|g9":   true,
		"alice|":   true,
	}
	if groups := candidateGroupsForUser(memberships, "alice"); !reflect.DeepEqual(groups, []string{"g1", "g2"}) {
		t.Fatalf("候选组错误: %v", groups)
	}
	if groups := candidateGroupsForUser(memberships, "carol"); len(groups) != 0 {
		t.Fatalf("无成员关系应返回空组: %v", groups)
	}
}

// TestMapTaskIDs 验证 SQL ID 到内存任务的顺序映射。
// @param t 测试上下文
func TestMapTaskIDs(t *testing.T) {
	tasks := map[string]*taskState{
		"t1": {Task: api.Task{ID: "t1"}},
		"t3": {Task: api.Task{ID: "t3"}},
	}
	mapped := mapTaskIDs(tasks, []string{"t3", "t2", "t1"})
	if len(mapped) != 2 || mapped[0].Task.ID != "t3" || mapped[1].Task.ID != "t1" {
		t.Fatalf("映射结果错误: %+v", mapped)
	}
	if empty := mapTaskIDs(tasks, nil); len(empty) != 0 {
		t.Fatalf("空 ID 应返回空列表: %+v", empty)
	}
}

// TestListTasksRowQueryPushdown 验证下推查询使用 SQL 顺序、总数并复用租户校验。
// @param t 测试上下文
func TestListTasksRowQueryPushdown(t *testing.T) {
	engine, mock := newTaskQueryEngine(t)
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	engine.state.Instances["i1"] = &instanceState{ID: "i1"}
	engine.state.Instances["i2"] = &instanceState{ID: "i2"}
	// 内存创建时间与 SQL 顺序相反，验证结果遵循 SQL 已排序的 ID 顺序
	engine.state.Tasks["t1"] = &taskState{Task: api.Task{ID: "t1", ProcessInstanceID: "i1", CreateTime: base.Add(2 * time.Hour)}}
	engine.state.Tasks["t2"] = &taskState{Task: api.Task{ID: "t2", ProcessInstanceID: "i1", CreateTime: base}}
	engine.state.Tasks["t3"] = &taskState{Task: api.Task{ID: "t3", ProcessInstanceID: "i2", CreateTime: base.Add(time.Hour)}}
	mock.ExpectQuery("SELECT COUNT(*) FROM ACT_RU_TASK T").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(4)))
	mock.ExpectQuery("SELECT T.ID_ FROM ACT_RU_TASK T ORDER BY T.CREATE_TIME_ ASC, T.ID_ ASC LIMIT ? OFFSET ?").
		WithArgs(int64(10), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"ID_"}).AddRow("t2").AddRow("t1").AddRow("t3").AddRow("已删除任务"))
	page, err := engine.ListTasks(context.Background(), api.TaskFilter{Size: 10})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if page.Total != 4 || page.Start != 0 || page.Size != 10 {
		t.Fatalf("分页信息错误: %+v", page)
	}
	// 结果顺序遵循 SQL 已排序的 ID，不在内存中的 ID 被跳过
	if len(page.Data) != 3 || page.Data[0].ID != "t2" || page.Data[1].ID != "t1" || page.Data[2].ID != "t3" {
		t.Fatalf("任务顺序错误: %+v", page.Data)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestListTasksRowQueryCandidateGroups 验证候选组参数按成员关系下推。
// @param t 测试上下文
func TestListTasksRowQueryCandidateGroups(t *testing.T) {
	engine, mock := newTaskQueryEngine(t)
	engine.state.Memberships["alice|g2"] = true
	engine.state.Memberships["alice|g1"] = true
	engine.state.Memberships["bob|g9"] = true
	engine.state.Tasks["t1"] = &taskState{Task: api.Task{ID: "t1"}}
	mock.ExpectQuery("SELECT COUNT(*) FROM ACT_RU_TASK T WHERE (T.ASSIGNEE_ IS NULL OR T.ASSIGNEE_ = '') AND EXISTS (SELECT 1 FROM ACT_RU_IDENTITYLINK L WHERE L.TASK_ID_ = T.ID_ AND L.TYPE_ = 'candidate' AND (L.USER_ID_ = ? OR L.GROUP_ID_ IN (?, ?)))").
		WithArgs("alice", "g1", "g2").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery("SELECT T.ID_ FROM ACT_RU_TASK T WHERE (T.ASSIGNEE_ IS NULL OR T.ASSIGNEE_ = '') AND EXISTS (SELECT 1 FROM ACT_RU_IDENTITYLINK L WHERE L.TASK_ID_ = T.ID_ AND L.TYPE_ = 'candidate' AND (L.USER_ID_ = ? OR L.GROUP_ID_ IN (?, ?))) ORDER BY T.CREATE_TIME_ ASC, T.ID_ ASC LIMIT ? OFFSET ?").
		WithArgs("alice", "g1", "g2", int64(5), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"ID_"}).AddRow("t1"))
	page, err := engine.ListTasks(context.Background(), api.TaskFilter{CandidateUser: "alice", Size: 5})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if page.Total != 1 || len(page.Data) != 1 || page.Data[0].ID != "t1" {
		t.Fatalf("候选组查询错误: %+v", page)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestListTasksRowQueryError 验证 SQL 失败直接返回错误且不回退内存。
// @param t 测试上下文
func TestListTasksRowQueryError(t *testing.T) {
	engine, mock := newTaskQueryEngine(t)
	engine.state.Tasks["t1"] = &taskState{Task: api.Task{ID: "t1"}}
	mock.ExpectQuery("SELECT COUNT(*) FROM ACT_RU_TASK T").WillReturnError(errors.New("数据库不可用"))
	page, err := engine.ListTasks(context.Background(), api.TaskFilter{})
	if err == nil {
		t.Fatalf("SQL 失败必须返回错误: %+v", page)
	}
	var engineErr *api.Error
	if !errors.As(err, &engineErr) || engineErr.Code != "ROW-0036" {
		t.Fatalf("错误码不正确: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestListTasksTenantPushdown 验证绑定租户、请求租户与租户过滤时任务查询下推 SQL。
// 生效租户（请求过滤优先，其次上下文/默认租户）非空时以 TENANT_ID_ 等值条件查询，
// 计数与 ID 列表 SQL 的租户参数绑定必须精确匹配；回退内存会命中未设置的期望而失败。
// @param t 测试上下文
func TestListTasksTenantPushdown(t *testing.T) {
	cases := []struct {
		name   string                 // 用例名
		tenant string                 // 引擎绑定的默认租户
		filter api.TaskFilter         // 查询条件
		ctx    func() context.Context // 上下文
	}{
		{
			name:   "租户过滤",
			tenant: "",
			filter: api.TaskFilter{TenantID: "t1", Size: 10},
			ctx:    func() context.Context { return tenant.With(context.Background(), "t1") },
		},
		{
			name:   "请求租户上下文",
			tenant: "",
			filter: api.TaskFilter{Size: 10},
			ctx:    func() context.Context { return tenant.With(context.Background(), "t1") },
		},
		{
			name:   "默认租户",
			tenant: "t1",
			filter: api.TaskFilter{Size: 10},
			ctx:    func() context.Context { return context.Background() },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine, mock := newTaskQueryEngine(t)
			engine.cfg.TenantID = tc.tenant
			engine.state.Tasks["t1"] = &taskState{Task: api.Task{ID: "t1", TenantID: "t1"}}
			// 计数与 ID 列表都以生效租户 t1 作为第一个绑定参数
			mock.ExpectQuery("SELECT COUNT(*) FROM ACT_RU_TASK T WHERE T.TENANT_ID_ = ?").
				WithArgs("t1").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
			mock.ExpectQuery("SELECT T.ID_ FROM ACT_RU_TASK T WHERE T.TENANT_ID_ = ? ORDER BY T.CREATE_TIME_ ASC, T.ID_ ASC LIMIT ? OFFSET ?").
				WithArgs("t1", int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"ID_"}).AddRow("t1"))
			page, err := engine.ListTasks(tc.ctx(), tc.filter)
			if err != nil {
				t.Fatalf("下推查询失败: %v", err)
			}
			if page.Total != 1 || len(page.Data) != 1 || page.Data[0].ID != "t1" {
				t.Fatalf("下推查询结果错误: %+v", page)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestHasNonDefaultTenantData 验证非默认租户任务与实例的识别。
// @param t 测试上下文
func TestHasNonDefaultTenantData(t *testing.T) {
	engine := newTestEngine(t)
	if engine.hasNonDefaultTenantData() {
		t.Fatal("空状态不应存在非默认租户数据")
	}
	engine.state.Instances["i1"] = &instanceState{ID: "i1"}
	engine.state.Tasks["t1"] = &taskState{Task: api.Task{ID: "t1", ProcessInstanceID: "i1"}}
	if engine.hasNonDefaultTenantData() {
		t.Fatal("默认租户任务不应触发回退")
	}
	engine.state.Tasks["t2"] = &taskState{Task: api.Task{ID: "t2", TenantID: "t2"}}
	if !engine.hasNonDefaultTenantData() {
		t.Fatal("任务自身非默认租户应触发回退")
	}
	engine = newTestEngine(t)
	engine.state.Instances["i2"] = &instanceState{ID: "i2", TenantID: "t2"}
	if !engine.hasNonDefaultTenantData() {
		t.Fatal("实例非默认租户应触发回退")
	}
}

// TestListTasksMultiTenantFallback 验证存在非默认租户数据时不发起 SQL，结果与内存一致。
// @param t 测试上下文
func TestListTasksMultiTenantFallback(t *testing.T) {
	engine, mock := newTaskQueryEngine(t)
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	engine.state.Instances["i1"] = &instanceState{ID: "i1"}
	engine.state.Instances["i2"] = &instanceState{ID: "i2", TenantID: "t2"}
	engine.state.Tasks["t1"] = &taskState{Task: api.Task{ID: "t1", ProcessInstanceID: "i1", CreateTime: base.Add(2 * time.Hour)}}
	engine.state.Tasks["t2"] = &taskState{Task: api.Task{ID: "t2", ProcessInstanceID: "i1", CreateTime: base}}
	engine.state.Tasks["t3"] = &taskState{Task: api.Task{ID: "t3", ProcessInstanceID: "i2", CreateTime: base.Add(time.Hour)}}
	// 多租户状态必须回退内存，任何 SQL 下推都会命中未设置的期望而失败
	page, err := engine.ListTasks(context.Background(), api.TaskFilter{Size: 10})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if page.Total != 2 || len(page.Data) != 2 || page.Data[0].ID != "t2" || page.Data[1].ID != "t1" {
		t.Fatalf("内存过滤结果错误: %+v", page)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestListTasksMultiTenantTenantFilterPushdown 验证多租户数据下指定租户过滤仍下推 SQL。
// 生效租户非空时非默认租户数据不再触发回退，SQL 以 TENANT_ID_ 等值条件收敛结果，
// 默认租户任务不进入结果，总数以 SQL 计数为准。
// @param t 测试上下文
func TestListTasksMultiTenantTenantFilterPushdown(t *testing.T) {
	engine, mock := newTaskQueryEngine(t)
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	engine.state.Instances["i1"] = &instanceState{ID: "i1"}
	engine.state.Instances["i2"] = &instanceState{ID: "i2", TenantID: "t2"}
	engine.state.Tasks["t1"] = &taskState{Task: api.Task{ID: "t1", ProcessInstanceID: "i1", CreateTime: base}}
	engine.state.Tasks["t2"] = &taskState{Task: api.Task{ID: "t2", ProcessInstanceID: "i2", CreateTime: base.Add(time.Hour)}}
	// 多租户数据与指定租户过滤并存时仍下推，条件参数为生效租户 t2
	mock.ExpectQuery("SELECT COUNT(*) FROM ACT_RU_TASK T WHERE T.TENANT_ID_ = ?").
		WithArgs("t2").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery("SELECT T.ID_ FROM ACT_RU_TASK T WHERE T.TENANT_ID_ = ? ORDER BY T.CREATE_TIME_ ASC, T.ID_ ASC LIMIT ? OFFSET ?").
		WithArgs("t2", int64(10), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"ID_"}).AddRow("t2"))
	page, err := engine.ListTasks(tenant.With(context.Background(), "t2"), api.TaskFilter{TenantID: "t2"})
	if err != nil {
		t.Fatalf("下推查询失败: %v", err)
	}
	// 总数来自 SQL 计数；默认租户任务 t1 不在结果内
	if page.Total != 1 || len(page.Data) != 1 || page.Data[0].ID != "t2" {
		t.Fatalf("租户下推结果错误: %+v", page)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestListTasksMemoryTenantFilter 验证内存回退路径的租户过滤语义。
// 未配置行仓储时走内存过滤，filter.TenantID 以任务自身或所属实例租户等值匹配，
// 并与 allowTenant 的请求租户校验叠加生效。
// @param t 测试上下文
func TestListTasksMemoryTenantFilter(t *testing.T) {
	engine := newTestEngine(t)
	engine.cfg.TenantID = "t2"
	engine.state.Instances["i2"] = &instanceState{ID: "i2", TenantID: "t2"}
	engine.state.Instances["i3"] = &instanceState{ID: "i3", TenantID: "t3"}
	engine.state.Tasks["t2"] = &taskState{Task: api.Task{ID: "t2", ProcessInstanceID: "i2"}}
	engine.state.Tasks["t3"] = &taskState{Task: api.Task{ID: "t3", ProcessInstanceID: "i3"}}
	engine.state.Tasks["solo"] = &taskState{Task: api.Task{ID: "solo", TenantID: "t2"}}
	page, err := engine.ListTasks(context.Background(), api.TaskFilter{TenantID: "t2"})
	if err != nil {
		t.Fatalf("内存查询失败: %v", err)
	}
	// 租户一致时任务自身与所属实例租户两种来源都命中
	if page.Total != 2 || len(page.Data) != 2 || page.Data[0].ID != "solo" || page.Data[1].ID != "t2" {
		t.Fatalf("租户过滤结果错误: %+v", page)
	}
	// filter.TenantID 与请求租户不一致时结果为空
	page, err = engine.ListTasks(context.Background(), api.TaskFilter{TenantID: "t3"})
	if err != nil {
		t.Fatalf("内存查询失败: %v", err)
	}
	if page.Total != 0 || len(page.Data) != 0 {
		t.Fatalf("跨租户过滤应为空: %+v", page)
	}
}
