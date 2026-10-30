package engine

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/DengQingNian/Flowtide/api"
	flowablerow "github.com/DengQingNian/Flowtide/internal/persistence/flowable"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// 历史活动下推查询的 SQL 形状常量，与 internal/persistence/flowable 的白名单拼接结果逐字一致，
// 配合 QueryMatcherEqual 精确断言可以同时验证 SQL 文本与参数绑定顺序。
const (
	// historicActivityCountSQL 是无条件历史活动的计数 SQL，历史活动查询无条件时不含 WHERE 片段
	historicActivityCountSQL = "SELECT COUNT(*) FROM ACT_HI_ACTINST A"
	// historicActivitySelectSQL 是无条件历史活动的 ID 列表 SQL
	historicActivitySelectSQL = "SELECT A.ID_ FROM ACT_HI_ACTINST A ORDER BY A.START_TIME_ ASC, A.ID_ ASC LIMIT ? OFFSET ?"
	// historicActivityTenantCountSQL 是带租户条件的历史活动计数 SQL
	historicActivityTenantCountSQL = "SELECT COUNT(*) FROM ACT_HI_ACTINST A WHERE A.TENANT_ID_ = ?"
	// historicActivityTenantSelectSQL 是带租户条件的历史活动 ID 列表 SQL
	historicActivityTenantSelectSQL = "SELECT A.ID_ FROM ACT_HI_ACTINST A WHERE A.TENANT_ID_ = ? ORDER BY A.START_TIME_ ASC, A.ID_ ASC LIMIT ? OFFSET ?"
)

// stubExecOnlyDB 是只实现写入接口的假数据库，用于构造不支持查询的行仓储。
type stubExecOnlyDB struct{}

// ExecContext 实现行写入接口，测试中不会被调用。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 执行结果
// @return 执行错误
func (stubExecOnlyDB) ExecContext(ctx context.Context, query string, args ...any) (flowablerow.Result, error) {
	// 测试替身不执行任何语句，参数仅用于满足接口签名
	_, _, _ = ctx, query, args
	return nil, nil
}

// newNoQueryRowEngine 创建注入了不支持查询的行仓储的测试引擎。
// 历史活动与历史变量查询共用此辅助，用于验证 ROW-0055、ROW-0060 等错误语义。
// @param t 测试上下文
// @return 引擎
func newNoQueryRowEngine(t *testing.T) *Engine {
	t.Helper()
	repository, err := flowablerow.NewRepository(stubExecOnlyDB{}, "mysql")
	if err != nil {
		t.Fatalf("创建行仓储失败: %v", err)
	}
	engine := newTestEngine(t)
	engine.rowRepository = repository
	return engine
}

// historicActivityIDsOf 提取历史活动分页数据的 ID 列表，便于顺序断言。
// @param page 历史活动分页
// @return ID 列表
func historicActivityIDsOf(page api.Page[api.HistoricActivityInstance]) []string {
	ids := make([]string, 0, len(page.Data))
	for _, item := range page.Data {
		ids = append(ids, item.ID)
	}
	return ids
}

// assertHistoricActivityOrder 断言历史活动分页数据的 ID 顺序与期望一致。
// @param t 测试上下文
// @param page 历史活动分页
// @param want 期望的 ID 顺序
func assertHistoricActivityOrder(t *testing.T, page api.Page[api.HistoricActivityInstance], want ...string) {
	t.Helper()
	if len(page.Data) != len(want) {
		t.Fatalf("历史活动数量错误: 期望 %d 实际 %d (%v)", len(want), len(page.Data), historicActivityIDsOf(page))
	}
	for i, id := range want {
		if page.Data[i].ID != id {
			t.Fatalf("历史活动顺序错误: 期望 %v 实际 %v", want, historicActivityIDsOf(page))
		}
	}
}

// TestQueryHistoricActivityInstancesRowPushdown 验证历史活动下推查询遵循 SQL 顺序、总数并完整映射 DTO。
// SQL 返回 ID 顺序与内存开始时间顺序相反，结果必须按 SQL 顺序返回；
// 缺失 ID、归属实例缺失与租户不可见的历史活动被跳过，未结束与已结束活动都应返回。
// @param t 测试上下文
func TestQueryHistoricActivityInstancesRowPushdown(t *testing.T) {
	engine, mock := newTaskQueryEngine(t)
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	ended := base.Add(3 * time.Hour)
	// 默认租户历史实例保证历史活动通过归属校验
	engine.state.HistoricProcesses["p1"] = &api.HistoricProcessInstance{ID: "p1"}
	engine.state.HistoricProcesses["p2"] = &api.HistoricProcessInstance{ID: "p2"}
	engine.state.HistoricProcesses["p3"] = &api.HistoricProcessInstance{ID: "p3"}
	a1 := &api.HistoricActivityInstance{
		ID: "a1", ProcessDefinitionID: "def-1", ProcessInstanceID: "p1", ExecutionID: "e1",
		ActivityID: "act1", ActivityName: "活动A", ActivityType: "userTask", Assignee: "alice",
		StartTime: base.Add(2 * time.Hour), TransactionOrder: 7, DurationMillis: 100,
	}
	a2 := &api.HistoricActivityInstance{
		ID: "a2", ProcessDefinitionID: "def-2", ProcessInstanceID: "p2", ExecutionID: "e2",
		ActivityID: "act2", ActivityName: "活动B", ActivityType: "serviceTask", Assignee: "bob",
		StartTime: base, EndTime: &ended, DurationMillis: 200,
	}
	a3 := &api.HistoricActivityInstance{
		ID: "a3", ProcessDefinitionID: "def-1", ProcessInstanceID: "p3", ExecutionID: "e3",
		ActivityID: "act1", ActivityName: "活动C", ActivityType: "endEvent",
		StartTime: base.Add(time.Hour),
	}
	engine.state.HistoricActivities["a1"] = a1
	engine.state.HistoricActivities["a2"] = a2
	engine.state.HistoricActivities["a3"] = a3
	// 流程实例缺失导致归属校验失败，SQL 返回也不可见
	engine.state.HistoricActivities["ahidden"] = &api.HistoricActivityInstance{ID: "ahidden", ProcessInstanceID: "p-missing", StartTime: base}
	// 运行中实例属于其他租户，历史活动对空租户请求不可见
	engine.state.Instances["p-live"] = &instanceState{ID: "p-live", TenantID: "t2"}
	engine.state.HistoricActivities["aother"] = &api.HistoricActivityInstance{ID: "aother", ProcessInstanceID: "p-live", StartTime: base}
	mock.ExpectQuery(historicActivityCountSQL).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(6)))
	mock.ExpectQuery(historicActivitySelectSQL).
		WithArgs(int64(10), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"ID_"}).
			AddRow("a3").AddRow("a2").AddRow("不存在活动").AddRow("aother").AddRow("ahidden").AddRow("a1"))
	page, err := engine.QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{})
	if err != nil {
		t.Fatalf("下推查询失败: %v", err)
	}
	// 总数来自 SQL 计数，而非内存匹配数量
	if page.Total != 6 || page.Start != 0 || page.Size != 10 {
		t.Fatalf("分页信息错误: %+v", page)
	}
	// 缺失 ID、归属失败与租户不符的行被跳过，其余按 SQL 顺序返回
	assertHistoricActivityOrder(t, page, "a3", "a2", "a1")
	// 逐字段映射：DTO 必须与内存状态完全一致，不能因查询层未使用而丢失列
	want := []api.HistoricActivityInstance{*a3, *a2, *a1}
	for i := range want {
		if !reflect.DeepEqual(page.Data[i], want[i]) {
			t.Fatalf("历史活动字段映射错误: 第 %d 条 期望 %+v 实际 %+v", i, want[i], page.Data[i])
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryHistoricActivityInstancesRowError 验证历史活动 SQL 失败直接返回错误且不回退内存。
// 计数查询执行失败包装为 ROW-0056，行仓储不支持查询返回 ROW-0055；
// 内存中存在匹配数据时也必须报错，防止静默回退掩盖数据库问题。
// @param t 测试上下文
func TestQueryHistoricActivityInstancesRowError(t *testing.T) {
	t.Run("查询执行失败不回退内存", func(t *testing.T) {
		engine, mock := newTaskQueryEngine(t)
		engine.state.HistoricActivities["a1"] = &api.HistoricActivityInstance{ID: "a1"}
		// 仅设置计数期望并返回错误；若实现继续执行 SELECT 或回退内存，断言都会失败
		mock.ExpectQuery(historicActivityCountSQL).WillReturnError(errors.New("数据库不可用"))
		page, err := engine.QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{})
		assertRowErrorCode(t, err, "ROW-0056")
		if page.Total != 0 || len(page.Data) != 0 {
			t.Fatalf("失败查询必须返回空结果: %+v", page)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("行仓储不支持查询", func(t *testing.T) {
		engine := newNoQueryRowEngine(t)
		engine.state.HistoricActivities["a1"] = &api.HistoricActivityInstance{ID: "a1"}
		page, err := engine.QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{})
		// 行仓储不可查询属于 SQL 失败，同样不能回退内存
		assertRowErrorCode(t, err, "ROW-0055")
		if page.Total != 0 || len(page.Data) != 0 {
			t.Fatalf("失败查询必须返回空结果: %+v", page)
		}
	})
}

// TestQueryHistoricActivityInstancesTenantPushdown 验证租户生效时历史活动查询下推 SQL。
// 引擎绑定租户、请求携带租户或请求指定租户过滤时以 A.TENANT_ID_ 等值条件查询，
// 参数绑定必须精确匹配；回退内存会命中未设置的期望而失败。
// @param t 测试上下文
func TestQueryHistoricActivityInstancesTenantPushdown(t *testing.T) {
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string                             // 用例名
		bound  string                             // 引擎绑定的默认租户
		filter api.HistoricActivityInstanceFilter // 查询条件
		ctx    func() context.Context             // 请求上下文
	}{
		{
			name:   "绑定默认租户",
			bound:  "t1",
			filter: api.HistoricActivityInstanceFilter{},
			ctx:    func() context.Context { return context.Background() },
		},
		{
			name:   "请求租户上下文",
			bound:  "",
			filter: api.HistoricActivityInstanceFilter{},
			ctx:    func() context.Context { return tenant.With(context.Background(), "t1") },
		},
		{
			name:   "请求租户过滤",
			bound:  "",
			filter: api.HistoricActivityInstanceFilter{TenantID: "t1"},
			ctx:    func() context.Context { return tenant.With(context.Background(), "t1") },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine, mock := newTaskQueryEngine(t)
			engine.cfg.TenantID = tc.bound
			engine.state.HistoricProcesses["p1"] = &api.HistoricProcessInstance{ID: "p1", TenantID: "t1"}
			engine.state.HistoricActivities["a1"] = &api.HistoricActivityInstance{ID: "a1", ProcessInstanceID: "p1", TenantID: "t1", StartTime: base}
			// 计数与 ID 列表都以生效租户 t1 作为第一个绑定参数
			mock.ExpectQuery(historicActivityTenantCountSQL).
				WithArgs("t1").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
			mock.ExpectQuery(historicActivityTenantSelectSQL).
				WithArgs("t1", int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"ID_"}).AddRow("a1"))
			page, err := engine.QueryHistoricActivityInstances(tc.ctx(), tc.filter)
			if err != nil {
				t.Fatalf("下推查询失败: %v", err)
			}
			if page.Total != 1 || page.Start != 0 || page.Size != 10 {
				t.Fatalf("分页信息错误: %+v", page)
			}
			assertHistoricActivityOrder(t, page, "a1")
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricActivityInstancesMultiTenantTenantFilterPushdown 验证多租户历史数据下指定租户过滤仍下推 SQL。
// 生效租户非空时非默认租户历史数据不再触发回退，SQL 以 TENANT_ID_ 等值条件收敛，
// 默认租户历史活动不进入结果，总数以 SQL 计数为准。
// @param t 测试上下文
func TestQueryHistoricActivityInstancesMultiTenantTenantFilterPushdown(t *testing.T) {
	engine, mock := newTaskQueryEngine(t)
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	engine.state.HistoricProcesses["p1"] = &api.HistoricProcessInstance{ID: "p1"}
	engine.state.HistoricActivities["a1"] = &api.HistoricActivityInstance{ID: "a1", ProcessInstanceID: "p1", StartTime: base.Add(time.Hour)}
	engine.state.HistoricProcesses["p2"] = &api.HistoricProcessInstance{ID: "p2", TenantID: "t2"}
	engine.state.HistoricActivities["a2"] = &api.HistoricActivityInstance{ID: "a2", ProcessInstanceID: "p2", TenantID: "t2", StartTime: base}
	// 多租户历史数据与指定租户过滤并存时仍下推，条件参数为生效租户 t2
	mock.ExpectQuery(historicActivityTenantCountSQL).
		WithArgs("t2").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery(historicActivityTenantSelectSQL).
		WithArgs("t2", int64(10), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"ID_"}).AddRow("a2"))
	page, err := engine.QueryHistoricActivityInstances(tenant.With(context.Background(), "t2"), api.HistoricActivityInstanceFilter{TenantID: "t2"})
	if err != nil {
		t.Fatalf("下推查询失败: %v", err)
	}
	// 总数来自 SQL 计数；默认租户历史活动 a1 不在结果内
	if page.Total != 1 {
		t.Fatalf("总数错误: %+v", page)
	}
	assertHistoricActivityOrder(t, page, "a2")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryHistoricActivityInstancesFallbackMultiTenant 验证内存存在非默认租户历史数据时回退内存过滤。
// 历史活动自身记录非默认租户、历史实例记录非默认租户都触发回退；
// 历史实例记录非默认租户时其历史活动被归属校验隐藏。
// @param t 测试上下文
func TestQueryHistoricActivityInstancesFallbackMultiTenant(t *testing.T) {
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name string               // 用例名
		seed func(engine *Engine) // 非默认租户数据的注入方式
		want []string             // 期望的 ID 顺序
	}{
		{
			name: "历史活动记录非默认租户",
			seed: func(engine *Engine) {
				// 历史实例仍为默认租户，触发回退的只能是历史活动自身的租户
				engine.state.HistoricProcesses["p2"] = &api.HistoricProcessInstance{ID: "p2"}
				engine.state.HistoricActivities["a1"] = &api.HistoricActivityInstance{ID: "a1", StartTime: base.Add(time.Hour)}
				engine.state.HistoricActivities["a2"] = &api.HistoricActivityInstance{ID: "a2", ProcessInstanceID: "p2", TenantID: "t2", StartTime: base}
			},
			want: []string{"a2", "a1"},
		},
		{
			name: "历史实例记录非默认租户",
			seed: func(engine *Engine) {
				// 历史活动自身未记录租户，触发回退的是其归属的历史实例租户
				engine.state.HistoricProcesses["p2"] = &api.HistoricProcessInstance{ID: "p2", TenantID: "t2"}
				engine.state.HistoricActivities["a1"] = &api.HistoricActivityInstance{ID: "a1", StartTime: base.Add(time.Hour)}
				engine.state.HistoricActivities["a2"] = &api.HistoricActivityInstance{ID: "a2", ProcessInstanceID: "p2", StartTime: base}
			},
			want: []string{"a1"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine, mock := newTaskQueryEngine(t)
			tc.seed(engine)
			// 多租户历史状态必须回退内存，任何 SQL 下推都会命中未设置的期望而失败
			page, err := engine.QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{})
			if err != nil {
				t.Fatalf("内存查询失败: %v", err)
			}
			if page.Total != int64(len(tc.want)) || page.Start != 0 || page.Size != 10 {
				t.Fatalf("分页信息错误: %+v", page)
			}
			assertHistoricActivityOrder(t, page, tc.want...)
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricActivityInstancesNoRowRepositoryFallback 验证未配置行仓储时回退内存查询。
// 无行仓储只能走内存过滤，内存排序与总数必须完整返回。
// @param t 测试上下文
func TestQueryHistoricActivityInstancesNoRowRepositoryFallback(t *testing.T) {
	engine := newTestEngine(t)
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	engine.state.HistoricActivities["a1"] = &api.HistoricActivityInstance{ID: "a1", StartTime: base.Add(time.Hour)}
	engine.state.HistoricActivities["a2"] = &api.HistoricActivityInstance{ID: "a2", StartTime: base}
	page, err := engine.QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{})
	if err != nil {
		t.Fatalf("内存查询失败: %v", err)
	}
	if page.Total != 2 || page.Start != 0 || page.Size != 10 {
		t.Fatalf("分页信息错误: %+v", page)
	}
	assertHistoricActivityOrder(t, page, "a2", "a1")
}

// TestQueryHistoricActivityInstancesMemoryFilter 验证未配置行仓储时历史活动内存过滤语义。
// 覆盖等值条件、定义 Key 反查、Finished 布尔、开始与结束时间边界含等于、空结束时间不匹配任何区间、
// 排序并列规则、分页切片、偏移越界、超大偏移不溢出与分页参数规范化。
// @param t 测试上下文
func TestQueryHistoricActivityInstancesMemoryFilter(t *testing.T) {
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	end0 := base.Add(time.Hour)
	end1 := base.Add(2 * time.Hour)
	newEngine := func(t *testing.T) *Engine {
		t.Helper()
		engine := newTestEngine(t)
		// 未配置行仓储，查询只能走内存路径
		engine.state.HistoricProcesses["p1"] = &api.HistoricProcessInstance{ID: "p1", TenantID: ""}
		engine.state.HistoricProcesses["p2"] = &api.HistoricProcessInstance{ID: "p2", TenantID: ""}
		engine.state.HistoricProcesses["p3"] = &api.HistoricProcessInstance{ID: "p3", TenantID: ""}
		engine.state.HistoricActivities["a1"] = &api.HistoricActivityInstance{
			ID: "a1", ProcessInstanceID: "p1", ProcessDefinitionID: "def-1", ActivityID: "act1",
			ActivityName: "活动A", ActivityType: "userTask", Assignee: "alice", StartTime: base,
		}
		engine.state.HistoricActivities["a2"] = &api.HistoricActivityInstance{
			ID: "a2", ProcessInstanceID: "p2", ProcessDefinitionID: "def-2", ActivityID: "act2",
			ActivityName: "活动B", ActivityType: "serviceTask", Assignee: "bob", StartTime: base, EndTime: &end0,
		}
		engine.state.HistoricActivities["a3"] = &api.HistoricActivityInstance{
			ID: "a3", ProcessInstanceID: "p3", ProcessDefinitionID: "def-1", ActivityID: "act1",
			ActivityName: "活动C", ActivityType: "userTask", Assignee: "alice", StartTime: base.Add(time.Hour), EndTime: &end1,
		}
		// 定义缺失的历史活动用于验证 Key 条件下的不匹配语义
		engine.state.HistoricActivities["a4"] = &api.HistoricActivityInstance{ID: "a4", ProcessDefinitionID: "def-x", StartTime: base}
		// 非默认租户历史活动对空租户请求不可见
		engine.state.HistoricProcesses["p2t"] = &api.HistoricProcessInstance{ID: "p2t", TenantID: "t2"}
		engine.state.HistoricActivities["at2"] = &api.HistoricActivityInstance{ID: "at2", ProcessInstanceID: "p2t", TenantID: "t2", StartTime: base, EndTime: &end0}
		// 定义 Key 经流程定义反查，补齐定义状态
		engine.state.Definitions["def-1"] = &definitionState{Info: api.ProcessDefinition{ID: "def-1", Key: "keyA"}}
		engine.state.Definitions["def-2"] = &definitionState{Info: api.ProcessDefinition{ID: "def-2", Key: "keyB"}}
		return engine
	}
	t.Run("空条件返回全部可见历史活动", func(t *testing.T) {
		page, err := newEngine(t).QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Total != 4 || page.Start != 0 || page.Size != 10 {
			t.Fatalf("分页信息错误: %+v", page)
		}
		// 开始时间相同按 ID 升序并列排序
		assertHistoricActivityOrder(t, page, "a1", "a2", "a4", "a3")
		// 逐字段返回：默认租户可见活动的全部字段必须原样保留
		engine := newEngine(t)
		page, err = engine.QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{ID: "a2"})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if len(page.Data) != 1 || !reflect.DeepEqual(page.Data[0], *engine.state.HistoricActivities["a2"]) {
			t.Fatalf("历史活动字段错误: %+v", page.Data)
		}
	})
	t.Run("等值字段", func(t *testing.T) {
		cases := []struct {
			name   string                             // 用例名
			filter api.HistoricActivityInstanceFilter // 查询条件
			want   []string                           // 期望结果
		}{
			{name: "历史活动ID", filter: api.HistoricActivityInstanceFilter{ID: "a2"}, want: []string{"a2"}},
			{name: "流程实例ID", filter: api.HistoricActivityInstanceFilter{ProcessInstanceID: "p3"}, want: []string{"a3"}},
			{name: "流程定义ID", filter: api.HistoricActivityInstanceFilter{ProcessDefinitionID: "def-1"}, want: []string{"a1", "a3"}},
			{name: "活动ID", filter: api.HistoricActivityInstanceFilter{ActivityID: "act1"}, want: []string{"a1", "a3"}},
			{name: "活动名称", filter: api.HistoricActivityInstanceFilter{ActivityName: "活动A"}, want: []string{"a1"}},
			{name: "活动类型", filter: api.HistoricActivityInstanceFilter{ActivityType: "serviceTask"}, want: []string{"a2"}},
			{name: "处理人", filter: api.HistoricActivityInstanceFilter{Assignee: "alice"}, want: []string{"a1", "a3"}},
			{name: "不存在的处理人", filter: api.HistoricActivityInstanceFilter{Assignee: "carol"}, want: []string{}},
			// 非默认租户历史活动被归属校验隐藏，等值条件不会泄露其他租户数据
			{name: "其他租户", filter: api.HistoricActivityInstanceFilter{TenantID: "t2"}, want: []string{}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				page, err := newEngine(t).QueryHistoricActivityInstances(context.Background(), tc.filter)
				if err != nil {
					t.Fatalf("内存查询失败: %v", err)
				}
				assertHistoricActivityOrder(t, page, tc.want...)
			})
		}
	})
	t.Run("定义Key经流程定义反查", func(t *testing.T) {
		page, err := newEngine(t).QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{ProcessDefinitionKey: "keyA"})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		// keyA 命中 def-1 下的活动，def-x 缺失定义视为不匹配
		assertHistoricActivityOrder(t, page, "a1", "a3")
		page, err = newEngine(t).QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{ProcessDefinitionKey: "keyB"})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricActivityOrder(t, page, "a2")
		// 不存在的 Key 不匹配任何活动
		page, err = newEngine(t).QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{ProcessDefinitionKey: "keyZ"})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricActivityOrder(t, page)
	})
	t.Run("Finished过滤", func(t *testing.T) {
		finished := true
		page, err := newEngine(t).QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{Finished: &finished})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		// 结束状态以结束时间是否为空判断，租户不可见活动仍被隐藏
		assertHistoricActivityOrder(t, page, "a2", "a3")
		running := false
		page, err = newEngine(t).QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{Finished: &running})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricActivityOrder(t, page, "a1", "a4")
	})
	t.Run("开始时间边界含等于", func(t *testing.T) {
		// 下界含等于：开始时间等于边界仍匹配
		bound := base
		page, err := newEngine(t).QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{StartedAfter: &bound})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricActivityOrder(t, page, "a1", "a2", "a4", "a3")
		// 上界含等于：晚于边界的活动被排除
		page, err = newEngine(t).QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{StartedBefore: &bound})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricActivityOrder(t, page, "a1", "a2", "a4")
		// 下界超出 1 纳秒即不再含等于
		strict := base.Add(time.Nanosecond)
		page, err = newEngine(t).QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{StartedAfter: &strict})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricActivityOrder(t, page, "a3")
	})
	t.Run("结束时间边界含等于", func(t *testing.T) {
		// 下界含等于：结束时间等于边界仍匹配
		bound := end0
		page, err := newEngine(t).QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{FinishedAfter: &bound})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricActivityOrder(t, page, "a2", "a3")
		// 上界含等于：晚于边界的活动被排除
		page, err = newEngine(t).QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{FinishedBefore: &bound})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricActivityOrder(t, page, "a2")
		// 下界超出 1 纳秒即不再含等于
		strict := end0.Add(time.Nanosecond)
		page, err = newEngine(t).QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{FinishedAfter: &strict})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricActivityOrder(t, page, "a3")
	})
	t.Run("空结束时间不匹配结束区间", func(t *testing.T) {
		// a1、a4 结束时间为空，设置了任一结束区间即不匹配，即使区间语义上不冲突
		early := base.Add(-time.Hour)
		page, err := newEngine(t).QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{FinishedBefore: &early})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		// a2、a3 的结束时间晚于上界同样被排除，结果为空
		assertHistoricActivityOrder(t, page)
		// 区间即使晚于所有已结束活动，空结束时间的活动也不参与
		bound := end1
		page, err = newEngine(t).QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{FinishedAfter: &bound})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricActivityOrder(t, page, "a3")
	})
	t.Run("租户过滤", func(t *testing.T) {
		engine := newTestEngine(t)
		engine.cfg.TenantID = "t2"
		// 归属校验以历史实例租户优先，同租户历史活动可见
		engine.state.HistoricProcesses["p2"] = &api.HistoricProcessInstance{ID: "p2", TenantID: "t2"}
		engine.state.HistoricProcesses["p3"] = &api.HistoricProcessInstance{ID: "p3", TenantID: "t3"}
		engine.state.HistoricActivities["a"] = &api.HistoricActivityInstance{ID: "a", ProcessInstanceID: "p2", TenantID: "t2", StartTime: base}
		engine.state.HistoricActivities["b"] = &api.HistoricActivityInstance{ID: "b", ProcessInstanceID: "p3", TenantID: "t3", StartTime: base}
		// filter.TenantID 与请求租户一致时只返回该租户历史活动
		page, err := engine.QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{TenantID: "t2"})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricActivityOrder(t, page, "a")
		// filter.TenantID 与历史活动租户不一致时被等值条件排除
		page, err = engine.QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{TenantID: "t3"})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricActivityOrder(t, page)
	})
	t.Run("排序与分页切片", func(t *testing.T) {
		// 每页数量小于剩余条数时按 start+size 截断
		page, err := newEngine(t).QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{Start: 0, Size: 2})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Total != 4 || page.Start != 0 || page.Size != 2 {
			t.Fatalf("分页信息错误: %+v", page)
		}
		assertHistoricActivityOrder(t, page, "a1", "a2")
		// 偏移 1 起剩余条数不大于每页数量时返回到末尾
		page, err = newEngine(t).QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{Start: 1, Size: 3})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Total != 4 || page.Start != 1 || page.Size != 3 {
			t.Fatalf("分页信息错误: %+v", page)
		}
		assertHistoricActivityOrder(t, page, "a2", "a4", "a3")
	})
	t.Run("偏移越界返回空数据", func(t *testing.T) {
		page, err := newEngine(t).QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{Start: 9, Size: 3})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Total != 4 || page.Start != 9 || page.Size != 3 || len(page.Data) != 0 {
			t.Fatalf("越界分页信息错误: %+v", page)
		}
		// 极大每页数量与偏移相加不得溢出，偏移 1 起返回到末尾，数量按规范化封顶
		page, err = newEngine(t).QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{Start: 1, Size: math.MaxInt})
		if err != nil {
			t.Fatalf("极大分页查询失败: %v", err)
		}
		if page.Total != 4 || page.Size != 1000 || len(page.Data) != 3 {
			t.Fatalf("极大分页信息错误: %+v", page)
		}
		assertHistoricActivityOrder(t, page, "a2", "a4", "a3")
		// 超大偏移直接返回空数据，不与每页数量相加
		page, err = newEngine(t).QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{Start: math.MaxInt, Size: 1})
		if err != nil {
			t.Fatalf("超大偏移查询失败: %v", err)
		}
		if page.Total != 4 || page.Start != math.MaxInt || len(page.Data) != 0 {
			t.Fatalf("超大偏移分页信息错误: %+v", page)
		}
	})
	t.Run("分页参数规范化", func(t *testing.T) {
		// 负偏移归 0，非正每页数量取默认 10
		page, err := newEngine(t).QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{Start: -2, Size: -1})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Start != 0 || page.Size != 10 || page.Total != 4 {
			t.Fatalf("规范化分页信息错误: %+v", page)
		}
		assertHistoricActivityOrder(t, page, "a1", "a2", "a4", "a3")
		// 每页数量封顶 1000
		page, err = newEngine(t).QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{Start: 0, Size: 1500})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Size != 1000 {
			t.Fatalf("每页数量应封顶 1000: %+v", page)
		}
	})
}
