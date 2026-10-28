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

// TestListTasksFallbackToMemory 验证存在租户作用域或租户过滤时保留内存路径。
// @param t 测试上下文
func TestListTasksFallbackToMemory(t *testing.T) {
	cases := []struct {
		name   string                 // 用例名
		tenant string                 // 绑定租户
		filter api.TaskFilter         // 查询条件
		setup  func(engine *Engine)   // 额外设置
		ctx    func() context.Context // 上下文
	}{
		{
			name: "租户过滤", tenant: "",
			filter: api.TaskFilter{TenantID: "t1", Size: 10},
			setup:  func(engine *Engine) { engine.state.Tasks["t1"] = &taskState{Task: api.Task{ID: "t1", TenantID: "t1"}} },
			ctx:    func() context.Context { return tenant.With(context.Background(), "t1") },
		},
		{
			name: "绑定租户上下文", tenant: "",
			filter: api.TaskFilter{Size: 10},
			setup:  func(engine *Engine) { engine.state.Tasks["t1"] = &taskState{Task: api.Task{ID: "t1", TenantID: "t1"}} },
			ctx:    func() context.Context { return tenant.With(context.Background(), "t1") },
		},
		{
			name: "默认租户", tenant: "t1",
			filter: api.TaskFilter{Size: 10},
			setup:  func(engine *Engine) { engine.state.Tasks["t1"] = &taskState{Task: api.Task{ID: "t1", TenantID: "t1"}} },
			ctx:    func() context.Context { return context.Background() },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine, mock := newTaskQueryEngine(t)
			engine.cfg.TenantID = tc.tenant
			tc.setup(engine)
			page, err := engine.ListTasks(tc.ctx(), tc.filter)
			if err != nil {
				t.Fatalf("内存查询失败: %v", err)
			}
			// 若发生 SQL 下推会命中未设置的 sqlmock 期望并报错
			if page.Total != 1 || len(page.Data) != 1 || page.Data[0].ID != "t1" {
				t.Fatalf("内存查询结果错误: %+v", page)
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
