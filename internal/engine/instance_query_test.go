package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/tenant"
)

// 实例与历史实例下推查询的 SQL 形状常量，与 internal/persistence/flowable 的白名单拼接结果逐字一致，
// 配合 QueryMatcherEqual 精确断言可以同时验证 SQL 文本与参数绑定顺序。
const (
	// instanceCountSQL 是无条件运行中实例的计数 SQL
	instanceCountSQL = "SELECT COUNT(*) FROM ACT_RU_EXECUTION E WHERE E.PARENT_ID_ IS NULL"
	// instanceSelectSQL 是无条件运行中实例的 ID 列表 SQL
	instanceSelectSQL = "SELECT E.PROC_INST_ID_ FROM ACT_RU_EXECUTION E WHERE E.PARENT_ID_ IS NULL ORDER BY E.START_TIME_ ASC, E.PROC_INST_ID_ ASC LIMIT ? OFFSET ?"
	// instanceSuspendedCountSQL 是带挂起条件的运行中实例计数 SQL
	instanceSuspendedCountSQL = "SELECT COUNT(*) FROM ACT_RU_EXECUTION E WHERE E.PARENT_ID_ IS NULL AND E.SUSPENSION_STATE_ = ?"
	// instanceSuspendedSelectSQL 是带挂起条件的运行中实例 ID 列表 SQL
	instanceSuspendedSelectSQL = "SELECT E.PROC_INST_ID_ FROM ACT_RU_EXECUTION E WHERE E.PARENT_ID_ IS NULL AND E.SUSPENSION_STATE_ = ? ORDER BY E.START_TIME_ ASC, E.PROC_INST_ID_ ASC LIMIT ? OFFSET ?"
	// historicCountSQL 是无条件历史实例的计数 SQL，历史查询无条件时不含 WHERE 片段
	historicCountSQL = "SELECT COUNT(*) FROM ACT_HI_PROCINST H"
	// historicSelectSQL 是无条件历史实例的 ID 列表 SQL
	historicSelectSQL = "SELECT H.PROC_INST_ID_ FROM ACT_HI_PROCINST H ORDER BY H.START_TIME_ ASC, H.PROC_INST_ID_ ASC LIMIT ? OFFSET ?"
	// instanceTenantCountSQL 是带租户条件的运行中实例计数 SQL
	instanceTenantCountSQL = "SELECT COUNT(*) FROM ACT_RU_EXECUTION E WHERE E.PARENT_ID_ IS NULL AND E.TENANT_ID_ = ?"
	// instanceTenantSelectSQL 是带租户条件的运行中实例 ID 列表 SQL
	instanceTenantSelectSQL = "SELECT E.PROC_INST_ID_ FROM ACT_RU_EXECUTION E WHERE E.PARENT_ID_ IS NULL AND E.TENANT_ID_ = ? ORDER BY E.START_TIME_ ASC, E.PROC_INST_ID_ ASC LIMIT ? OFFSET ?"
	// historicTenantCountSQL 是带租户条件的历史实例计数 SQL
	historicTenantCountSQL = "SELECT COUNT(*) FROM ACT_HI_PROCINST H WHERE H.TENANT_ID_ = ?"
	// historicTenantSelectSQL 是带租户条件的历史实例 ID 列表 SQL
	historicTenantSelectSQL = "SELECT H.PROC_INST_ID_ FROM ACT_HI_PROCINST H WHERE H.TENANT_ID_ = ? ORDER BY H.START_TIME_ ASC, H.PROC_INST_ID_ ASC LIMIT ? OFFSET ?"
)

// instanceIDsOf 提取运行中实例分页数据的 ID 列表，便于顺序断言。
// @param page 实例分页
// @return ID 列表
func instanceIDsOf(page api.Page[api.ProcessInstance]) []string {
	ids := make([]string, 0, len(page.Data))
	for _, item := range page.Data {
		ids = append(ids, item.ID)
	}
	return ids
}

// historicIDsOf 提取历史实例分页数据的 ID 列表，便于顺序断言。
// @param page 历史实例分页
// @return ID 列表
func historicIDsOf(page api.Page[api.HistoricProcessInstance]) []string {
	ids := make([]string, 0, len(page.Data))
	for _, item := range page.Data {
		ids = append(ids, item.ID)
	}
	return ids
}

// assertInstanceOrder 断言运行中实例分页数据的 ID 顺序与期望一致。
// @param t 测试上下文
// @param page 实例分页
// @param want 期望的 ID 顺序
func assertInstanceOrder(t *testing.T, page api.Page[api.ProcessInstance], want ...string) {
	t.Helper()
	if len(page.Data) != len(want) {
		t.Fatalf("实例数量错误: 期望 %d 实际 %d (%v)", len(want), len(page.Data), instanceIDsOf(page))
	}
	for i, id := range want {
		if page.Data[i].ID != id {
			t.Fatalf("实例顺序错误: 期望 %v 实际 %v", want, instanceIDsOf(page))
		}
	}
}

// assertHistoricOrder 断言历史实例分页数据的 ID 顺序与期望一致。
// @param t 测试上下文
// @param page 历史实例分页
// @param want 期望的 ID 顺序
func assertHistoricOrder(t *testing.T, page api.Page[api.HistoricProcessInstance], want ...string) {
	t.Helper()
	if len(page.Data) != len(want) {
		t.Fatalf("历史实例数量错误: 期望 %d 实际 %d (%v)", len(want), len(page.Data), historicIDsOf(page))
	}
	for i, id := range want {
		if page.Data[i].ID != id {
			t.Fatalf("历史实例顺序错误: 期望 %v 实际 %v", want, historicIDsOf(page))
		}
	}
}

// assertRowErrorCode 断言错误非空且错误码与期望一致，用于验证 SQL 失败语义。
// @param t 测试上下文
// @param err 待检错误
// @param code 期望的错误码
func assertRowErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatal("SQL 失败必须返回错误")
	}
	var engineErr *api.Error
	// 错误必须是可解包的 api.Error 且错误码精确匹配，防止静默回退掩盖数据库问题
	if !errors.As(err, &engineErr) || engineErr.Code != code {
		t.Fatalf("错误码不正确: %v", err)
	}
}

// TestQueryProcessInstancesRowPushdown 验证运行中实例下推查询遵循 SQL 顺序与总数。
// SQL 返回 ID 顺序与内存创建时间顺序相反，结果必须按 SQL 顺序返回；
// 缺失 ID 与已结束实例被跳过；下推前置条件已排除非默认租户数据，
// 此处验证空租户请求对默认租户实例可见（allowTenant 放行），租户不可见语义在回退路径覆盖。
// @param t 测试上下文
func TestQueryProcessInstancesRowPushdown(t *testing.T) {
	engine, mock := newTaskQueryEngine(t)
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	// 内存开始时间与 SQL 顺序相反，验证结果遵循 SQL 已排序的 ID 顺序
	engine.state.Instances["i1"] = &instanceState{ID: "i1", StartTime: base.Add(2 * time.Hour)}
	engine.state.Instances["i2"] = &instanceState{ID: "i2", StartTime: base.Add(time.Hour)}
	engine.state.Instances["i3"] = &instanceState{ID: "i3", StartTime: base}
	engine.state.Instances["iend"] = &instanceState{ID: "iend", StartTime: base.Add(3 * time.Hour), Ended: true}
	mock.ExpectQuery(instanceCountSQL).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(5)))
	mock.ExpectQuery(instanceSelectSQL).
		WithArgs(int64(10), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"PROC_INST_ID_"}).
			AddRow("i3").AddRow("iend").AddRow("不存在实例").AddRow("i2").AddRow("i1"))
	page, err := engine.QueryProcessInstances(context.Background(), api.ProcessInstanceFilter{})
	if err != nil {
		t.Fatalf("下推查询失败: %v", err)
	}
	// 总数来自 SQL 计数，而非内存匹配数量
	if page.Total != 5 || page.Start != 0 || page.Size != 10 {
		t.Fatalf("分页信息错误: %+v", page)
	}
	// 已结束与缺失的实例被跳过，其余按 SQL 顺序返回
	assertInstanceOrder(t, page, "i3", "i2", "i1")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryProcessInstancesSuspendedPushdown 验证挂起条件下推时的参数绑定顺序。
// SUSPENSION_STATE_ 编码为 1 活动、2 挂起，参数顺序为挂起状态在前、LIMIT/OFFSET 在后。
// @param t 测试上下文
func TestQueryProcessInstancesSuspendedPushdown(t *testing.T) {
	cases := []struct {
		name      string // 用例名
		suspended bool   // 挂起条件
		state     int64  // SUSPENSION_STATE_ 期望绑定值
		returnID  string // SQL 返回的实例 ID
	}{
		{name: "挂起为真", suspended: true, state: 2, returnID: "i2"},
		{name: "挂起为假", suspended: false, state: 1, returnID: "i1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine, mock := newTaskQueryEngine(t)
			base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
			engine.state.Instances["i1"] = &instanceState{ID: "i1", StartTime: base}
			engine.state.Instances["i2"] = &instanceState{ID: "i2", StartTime: base.Add(time.Hour)}
			mock.ExpectQuery(instanceSuspendedCountSQL).
				WithArgs(tc.state).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
			mock.ExpectQuery(instanceSuspendedSelectSQL).
				// 绑定顺序：挂起状态、每页数量、偏移量
				WithArgs(tc.state, int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"PROC_INST_ID_"}).AddRow(tc.returnID))
			page, err := engine.QueryProcessInstances(context.Background(), api.ProcessInstanceFilter{Suspended: &tc.suspended})
			if err != nil {
				t.Fatalf("挂起下推查询失败: %v", err)
			}
			if page.Total != 1 || page.Size != 10 {
				t.Fatalf("分页信息错误: %+v", page)
			}
			assertInstanceOrder(t, page, tc.returnID)
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryProcessInstancesTenantPushdown 验证租户生效时运行中实例查询下推 SQL。
// 引擎绑定租户、请求携带租户或请求指定租户过滤时以 E.TENANT_ID_ 等值条件查询，
// 参数绑定必须精确匹配；回退内存会命中未设置的期望而失败。
// @param t 测试上下文
func TestQueryProcessInstancesTenantPushdown(t *testing.T) {
	cases := []struct {
		name   string                    // 用例名
		bound  string                    // 引擎绑定的默认租户
		filter api.ProcessInstanceFilter // 查询条件
		ctx    func() context.Context    // 请求上下文
	}{
		{
			name:   "绑定默认租户",
			bound:  "t1",
			filter: api.ProcessInstanceFilter{},
			ctx:    func() context.Context { return context.Background() },
		},
		{
			name:   "请求租户上下文",
			bound:  "",
			filter: api.ProcessInstanceFilter{},
			ctx:    func() context.Context { return tenant.With(context.Background(), "t1") },
		},
		{
			name:   "请求租户过滤",
			bound:  "",
			filter: api.ProcessInstanceFilter{TenantID: "t1"},
			ctx:    func() context.Context { return tenant.With(context.Background(), "t1") },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine, mock := newTaskQueryEngine(t)
			engine.cfg.TenantID = tc.bound
			engine.state.Instances["i1"] = &instanceState{ID: "i1", TenantID: "t1", StartTime: time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)}
			// 计数与 ID 列表都以生效租户 t1 作为第一个绑定参数
			mock.ExpectQuery(instanceTenantCountSQL).
				WithArgs("t1").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
			mock.ExpectQuery(instanceTenantSelectSQL).
				WithArgs("t1", int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"PROC_INST_ID_"}).AddRow("i1"))
			page, err := engine.QueryProcessInstances(tc.ctx(), tc.filter)
			if err != nil {
				t.Fatalf("下推查询失败: %v", err)
			}
			if page.Total != 1 || page.Start != 0 || page.Size != 10 {
				t.Fatalf("分页信息错误: %+v", page)
			}
			assertInstanceOrder(t, page, "i1")
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryProcessInstancesMultiTenantTenantFilterPushdown 验证多租户数据下指定租户过滤仍下推 SQL。
// 生效租户非空时非默认租户数据不再触发回退，SQL 以 TENANT_ID_ 等值条件收敛，
// 默认租户实例不进入结果，总数以 SQL 计数为准。
// @param t 测试上下文
func TestQueryProcessInstancesMultiTenantTenantFilterPushdown(t *testing.T) {
	engine, mock := newTaskQueryEngine(t)
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	engine.state.Instances["i1"] = &instanceState{ID: "i1", StartTime: base.Add(time.Hour)}
	engine.state.Instances["i2"] = &instanceState{ID: "i2", TenantID: "t2", StartTime: base}
	// 多租户数据与指定租户过滤并存时仍下推，条件参数为生效租户 t2
	mock.ExpectQuery(instanceTenantCountSQL).
		WithArgs("t2").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery(instanceTenantSelectSQL).
		WithArgs("t2", int64(10), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"PROC_INST_ID_"}).AddRow("i2"))
	page, err := engine.QueryProcessInstances(tenant.With(context.Background(), "t2"), api.ProcessInstanceFilter{TenantID: "t2"})
	if err != nil {
		t.Fatalf("下推查询失败: %v", err)
	}
	// 总数来自 SQL 计数；默认租户实例 i1 不在结果内
	if page.Total != 1 {
		t.Fatalf("总数错误: %+v", page)
	}
	assertInstanceOrder(t, page, "i2")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryProcessInstancesFallbackMultiTenant 验证内存存在非默认租户实例时回退内存过滤。
// 非默认租户实例会使 SQL 无租户条件下的总数与分页失真，必须回退内存并隐藏该实例。
// @param t 测试上下文
func TestQueryProcessInstancesFallbackMultiTenant(t *testing.T) {
	engine, mock := newTaskQueryEngine(t)
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	engine.state.Instances["i1"] = &instanceState{ID: "i1", StartTime: base.Add(time.Hour)}
	engine.state.Instances["i2"] = &instanceState{ID: "i2", TenantID: "t2", StartTime: base}
	// 多租户状态必须回退内存，任何 SQL 下推都会命中未设置的期望而失败
	page, err := engine.QueryProcessInstances(context.Background(), api.ProcessInstanceFilter{})
	if err != nil {
		t.Fatalf("内存查询失败: %v", err)
	}
	if page.Total != 1 || page.Start != 0 || page.Size != 10 {
		t.Fatalf("分页信息错误: %+v", page)
	}
	// 非默认租户实例对空租户请求不可见
	assertInstanceOrder(t, page, "i1")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryProcessInstancesRowError 验证运行中实例 SQL 失败直接返回 ROW-0041 且不回退内存。
// 内存中存在匹配数据时也必须报错，防止静默回退掩盖数据库问题。
// @param t 测试上下文
func TestQueryProcessInstancesRowError(t *testing.T) {
	engine, mock := newTaskQueryEngine(t)
	engine.state.Instances["i1"] = &instanceState{ID: "i1"}
	// 仅设置计数期望并返回错误；若实现继续执行 SELECT 或回退内存，断言都会失败
	mock.ExpectQuery(instanceCountSQL).WillReturnError(errors.New("数据库不可用"))
	page, err := engine.QueryProcessInstances(context.Background(), api.ProcessInstanceFilter{})
	assertRowErrorCode(t, err, "ROW-0041")
	if page.Total != 0 || len(page.Data) != 0 {
		t.Fatalf("失败查询必须返回空结果: %+v", page)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryProcessInstancesMemoryFilter 验证未配置行仓储时逐字段内存过滤语义。
// 覆盖等值条件、挂起布尔、时间边界含等于、排序并列规则、分页切片、偏移越界与分页参数规范化。
// @param t 测试上下文
func TestQueryProcessInstancesMemoryFilter(t *testing.T) {
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	later := base.Add(time.Hour)
	newEngine := func(t *testing.T) *Engine {
		t.Helper()
		engine := newTestEngine(t)
		// 未配置行仓储，查询只能走内存路径
		engine.state.Instances["i1"] = &instanceState{ID: "i1", ProcessDefinitionID: "def-1", ProcessDefinitionKey: "keyA", BusinessKey: "biz1", StartTime: base}
		engine.state.Instances["i2"] = &instanceState{ID: "i2", ProcessDefinitionID: "def-2", ProcessDefinitionKey: "keyB", BusinessKey: "biz2", StartTime: base, Suspended: true}
		engine.state.Instances["i4"] = &instanceState{ID: "i4", ProcessDefinitionID: "def-1", ProcessDefinitionKey: "keyA", StartTime: later}
		// 已结束实例不进入运行中列表
		engine.state.Instances["iend"] = &instanceState{ID: "iend", BusinessKey: "biz3", StartTime: base, Ended: true}
		// 非默认租户实例对空租户请求不可见
		engine.state.Instances["it2"] = &instanceState{ID: "it2", TenantID: "t2", StartTime: base}
		return engine
	}
	t.Run("空条件返回未结束的默认租户实例", func(t *testing.T) {
		page, err := newEngine(t).QueryProcessInstances(context.Background(), api.ProcessInstanceFilter{})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Total != 3 || page.Start != 0 || page.Size != 10 {
			t.Fatalf("分页信息错误: %+v", page)
		}
		// 开始时间相同按 ID 升序并列排序
		assertInstanceOrder(t, page, "i1", "i2", "i4")
	})
	t.Run("ID等值", func(t *testing.T) {
		page, err := newEngine(t).QueryProcessInstances(context.Background(), api.ProcessInstanceFilter{ID: "i2"})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertInstanceOrder(t, page, "i2")
	})
	t.Run("定义ID等值", func(t *testing.T) {
		page, err := newEngine(t).QueryProcessInstances(context.Background(), api.ProcessInstanceFilter{ProcessDefinitionID: "def-2"})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertInstanceOrder(t, page, "i2")
		// 不存在的定义 ID 返回空结果
		page, err = newEngine(t).QueryProcessInstances(context.Background(), api.ProcessInstanceFilter{ProcessDefinitionID: "def-3"})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertInstanceOrder(t, page)
	})
	t.Run("定义Key等值", func(t *testing.T) {
		page, err := newEngine(t).QueryProcessInstances(context.Background(), api.ProcessInstanceFilter{ProcessDefinitionKey: "keyA"})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertInstanceOrder(t, page, "i1", "i4")
	})
	t.Run("业务键等值", func(t *testing.T) {
		page, err := newEngine(t).QueryProcessInstances(context.Background(), api.ProcessInstanceFilter{BusinessKey: "biz2"})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertInstanceOrder(t, page, "i2")
	})
	t.Run("挂起过滤", func(t *testing.T) {
		suspended := true
		page, err := newEngine(t).QueryProcessInstances(context.Background(), api.ProcessInstanceFilter{Suspended: &suspended})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertInstanceOrder(t, page, "i2")
		active := false
		page, err = newEngine(t).QueryProcessInstances(context.Background(), api.ProcessInstanceFilter{Suspended: &active})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertInstanceOrder(t, page, "i1", "i4")
	})
	t.Run("租户过滤", func(t *testing.T) {
		engine := newTestEngine(t)
		engine.cfg.TenantID = "t2"
		engine.state.Instances["a"] = &instanceState{ID: "a", TenantID: "t2", StartTime: base}
		engine.state.Instances["b"] = &instanceState{ID: "b", TenantID: "t3", StartTime: base}
		// filter.TenantID 与请求租户一致时只返回该租户实例
		page, err := engine.QueryProcessInstances(context.Background(), api.ProcessInstanceFilter{TenantID: "t2"})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertInstanceOrder(t, page, "a")
		// filter.TenantID 与请求租户不一致时实例被 allowTenant 隐藏
		page, err = engine.QueryProcessInstances(context.Background(), api.ProcessInstanceFilter{TenantID: "t3"})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertInstanceOrder(t, page)
	})
	t.Run("时间边界含等于", func(t *testing.T) {
		// 下界含等于：开始时间等于边界仍匹配
		bound := base
		page, err := newEngine(t).QueryProcessInstances(context.Background(), api.ProcessInstanceFilter{StartedAfter: &bound})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertInstanceOrder(t, page, "i1", "i2", "i4")
		// 上界含等于：晚于边界的实例被排除
		page, err = newEngine(t).QueryProcessInstances(context.Background(), api.ProcessInstanceFilter{StartedBefore: &bound})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertInstanceOrder(t, page, "i1", "i2")
		// 上下界相同等价于精确匹配该时刻
		page, err = newEngine(t).QueryProcessInstances(context.Background(), api.ProcessInstanceFilter{StartedAfter: &bound, StartedBefore: &bound})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertInstanceOrder(t, page, "i1", "i2")
		// 下界超出 1 纳秒即不再含等于
		strict := base.Add(time.Nanosecond)
		page, err = newEngine(t).QueryProcessInstances(context.Background(), api.ProcessInstanceFilter{StartedAfter: &strict})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertInstanceOrder(t, page, "i4")
	})
	t.Run("排序与分页切片", func(t *testing.T) {
		page, err := newEngine(t).QueryProcessInstances(context.Background(), api.ProcessInstanceFilter{Start: 1, Size: 1})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Total != 3 || page.Start != 1 || page.Size != 1 {
			t.Fatalf("分页信息错误: %+v", page)
		}
		assertInstanceOrder(t, page, "i2")
	})
	t.Run("偏移越界返回空数据", func(t *testing.T) {
		page, err := newEngine(t).QueryProcessInstances(context.Background(), api.ProcessInstanceFilter{Start: 5, Size: 2})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Total != 3 || page.Start != 5 || page.Size != 2 || len(page.Data) != 0 {
			t.Fatalf("越界分页信息错误: %+v", page)
		}
	})
	t.Run("分页参数规范化", func(t *testing.T) {
		// 负偏移归 0，非正每页数量取默认 10
		page, err := newEngine(t).QueryProcessInstances(context.Background(), api.ProcessInstanceFilter{Start: -1, Size: -5})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Start != 0 || page.Size != 10 || page.Total != 3 {
			t.Fatalf("规范化分页信息错误: %+v", page)
		}
		assertInstanceOrder(t, page, "i1", "i2", "i4")
		// 每页数量封顶 1000
		page, err = newEngine(t).QueryProcessInstances(context.Background(), api.ProcessInstanceFilter{Start: 0, Size: 2000})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Size != 1000 {
			t.Fatalf("每页数量应封顶 1000: %+v", page)
		}
	})
}

// TestMatchesProcessInstanceFilterTenant 验证实例内存匹配的租户等值语义。
// 空租户条件不过滤，非空租户要求完全一致，与 SQL TENANT_ID_ 等值语义一致。
// @param t 测试上下文
func TestMatchesProcessInstanceFilterTenant(t *testing.T) {
	instance := api.ProcessInstance{ID: "i1", TenantID: "t1"}
	if !matchesProcessInstanceFilter(instance, api.ProcessInstanceFilter{}) {
		t.Fatal("空租户条件不应过滤实例")
	}
	if !matchesProcessInstanceFilter(instance, api.ProcessInstanceFilter{TenantID: "t1"}) {
		t.Fatal("租户一致应匹配")
	}
	if matchesProcessInstanceFilter(instance, api.ProcessInstanceFilter{TenantID: "t2"}) {
		t.Fatal("租户不一致不应匹配")
	}
}

// TestQueryHistoricProcessInstancesRowPushdown 验证历史实例下推查询遵循 SQL 顺序与总数。
// 历史路径只跳过缺失 ID 与租户不可见数据，已结束实例仍应返回，与运行中实例语义不同。
// @param t 测试上下文
func TestQueryHistoricProcessInstancesRowPushdown(t *testing.T) {
	engine, mock := newTaskQueryEngine(t)
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	ended := base.Add(3 * time.Hour)
	// 内存开始时间与 SQL 顺序相反，h2 已结束但历史查询不应跳过
	engine.state.HistoricProcesses["h1"] = &api.HistoricProcessInstance{ID: "h1", StartTime: base.Add(2 * time.Hour)}
	engine.state.HistoricProcesses["h2"] = &api.HistoricProcessInstance{ID: "h2", StartTime: base.Add(time.Hour), Ended: true, EndTime: &ended}
	engine.state.HistoricProcesses["h3"] = &api.HistoricProcessInstance{ID: "h3", StartTime: base}
	mock.ExpectQuery(historicCountSQL).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(7)))
	mock.ExpectQuery(historicSelectSQL).
		WithArgs(int64(10), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"PROC_INST_ID_"}).
			AddRow("h3").AddRow("h2").AddRow("不存在实例").AddRow("h1"))
	page, err := engine.QueryHistoricProcessInstances(context.Background(), api.HistoricProcessInstanceFilter{})
	if err != nil {
		t.Fatalf("下推查询失败: %v", err)
	}
	// 总数来自 SQL 计数，而非内存匹配数量
	if page.Total != 7 || page.Start != 0 || page.Size != 10 {
		t.Fatalf("分页信息错误: %+v", page)
	}
	// 缺失 ID 被跳过，已结束实例保留，其余按 SQL 顺序返回
	assertHistoricOrder(t, page, "h3", "h2", "h1")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryHistoricProcessInstancesRowError 验证历史实例 SQL 失败直接返回 ROW-0046 且不回退内存。
// 内存中存在匹配数据时也必须报错，防止静默回退掩盖数据库问题。
// @param t 测试上下文
func TestQueryHistoricProcessInstancesRowError(t *testing.T) {
	engine, mock := newTaskQueryEngine(t)
	engine.state.HistoricProcesses["h1"] = &api.HistoricProcessInstance{ID: "h1"}
	// 仅设置计数期望并返回错误；若实现继续执行 SELECT 或回退内存，断言都会失败
	mock.ExpectQuery(historicCountSQL).WillReturnError(errors.New("数据库不可用"))
	page, err := engine.QueryHistoricProcessInstances(context.Background(), api.HistoricProcessInstanceFilter{})
	assertRowErrorCode(t, err, "ROW-0046")
	if page.Total != 0 || len(page.Data) != 0 {
		t.Fatalf("失败查询必须返回空结果: %+v", page)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryHistoricProcessInstancesFallbackMultiTenant 验证内存存在非默认租户历史实例时回退内存过滤。
// hasNonDefaultTenantHistory 命中后必须走内存路径，非默认租户历史实例被 allowTenant 隐藏。
// @param t 测试上下文
func TestQueryHistoricProcessInstancesFallbackMultiTenant(t *testing.T) {
	engine, mock := newTaskQueryEngine(t)
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	engine.state.HistoricProcesses["h1"] = &api.HistoricProcessInstance{ID: "h1", StartTime: base.Add(time.Hour)}
	engine.state.HistoricProcesses["h2"] = &api.HistoricProcessInstance{ID: "h2", TenantID: "t2", StartTime: base}
	// 多租户历史状态必须回退内存，任何 SQL 下推都会命中未设置的期望而失败
	page, err := engine.QueryHistoricProcessInstances(context.Background(), api.HistoricProcessInstanceFilter{})
	if err != nil {
		t.Fatalf("内存查询失败: %v", err)
	}
	if page.Total != 1 || page.Start != 0 || page.Size != 10 {
		t.Fatalf("分页信息错误: %+v", page)
	}
	assertHistoricOrder(t, page, "h1")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryHistoricProcessInstancesTenantPushdown 验证租户生效时历史实例查询下推 SQL。
// 引擎绑定租户、请求携带租户或请求指定租户过滤时以 H.TENANT_ID_ 等值条件查询，
// 参数绑定必须精确匹配；回退内存会命中未设置的期望而失败。
// @param t 测试上下文
func TestQueryHistoricProcessInstancesTenantPushdown(t *testing.T) {
	cases := []struct {
		name   string                            // 用例名
		bound  string                            // 引擎绑定的默认租户
		filter api.HistoricProcessInstanceFilter // 查询条件
		ctx    func() context.Context            // 请求上下文
	}{
		{
			name:   "绑定默认租户",
			bound:  "t1",
			filter: api.HistoricProcessInstanceFilter{},
			ctx:    func() context.Context { return context.Background() },
		},
		{
			name:   "请求租户上下文",
			bound:  "",
			filter: api.HistoricProcessInstanceFilter{},
			ctx:    func() context.Context { return tenant.With(context.Background(), "t1") },
		},
		{
			name:   "请求租户过滤",
			bound:  "",
			filter: api.HistoricProcessInstanceFilter{TenantID: "t1"},
			ctx:    func() context.Context { return tenant.With(context.Background(), "t1") },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine, mock := newTaskQueryEngine(t)
			engine.cfg.TenantID = tc.bound
			engine.state.HistoricProcesses["h1"] = &api.HistoricProcessInstance{ID: "h1", TenantID: "t1", StartTime: time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)}
			// 计数与 ID 列表都以生效租户 t1 作为第一个绑定参数
			mock.ExpectQuery(historicTenantCountSQL).
				WithArgs("t1").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
			mock.ExpectQuery(historicTenantSelectSQL).
				WithArgs("t1", int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"PROC_INST_ID_"}).AddRow("h1"))
			page, err := engine.QueryHistoricProcessInstances(tc.ctx(), tc.filter)
			if err != nil {
				t.Fatalf("下推查询失败: %v", err)
			}
			if page.Total != 1 || page.Start != 0 || page.Size != 10 {
				t.Fatalf("分页信息错误: %+v", page)
			}
			assertHistoricOrder(t, page, "h1")
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricProcessInstancesMultiTenantTenantFilterPushdown 验证多租户历史数据下指定租户过滤仍下推 SQL。
// 生效租户非空时非默认租户历史数据不再触发回退，SQL 以 TENANT_ID_ 等值条件收敛，
// 默认租户历史实例不进入结果，总数以 SQL 计数为准。
// @param t 测试上下文
func TestQueryHistoricProcessInstancesMultiTenantTenantFilterPushdown(t *testing.T) {
	engine, mock := newTaskQueryEngine(t)
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	engine.state.HistoricProcesses["h1"] = &api.HistoricProcessInstance{ID: "h1", StartTime: base.Add(time.Hour)}
	engine.state.HistoricProcesses["h2"] = &api.HistoricProcessInstance{ID: "h2", TenantID: "t2", StartTime: base}
	// 多租户历史数据与指定租户过滤并存时仍下推，条件参数为生效租户 t2
	mock.ExpectQuery(historicTenantCountSQL).
		WithArgs("t2").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery(historicTenantSelectSQL).
		WithArgs("t2", int64(10), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"PROC_INST_ID_"}).AddRow("h2"))
	page, err := engine.QueryHistoricProcessInstances(tenant.With(context.Background(), "t2"), api.HistoricProcessInstanceFilter{TenantID: "t2"})
	if err != nil {
		t.Fatalf("下推查询失败: %v", err)
	}
	// 总数来自 SQL 计数；默认租户历史实例 h1 不在结果内
	if page.Total != 1 {
		t.Fatalf("总数错误: %+v", page)
	}
	assertHistoricOrder(t, page, "h2")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryHistoricProcessInstancesMemoryFilter 验证未配置行仓储时历史实例内存过滤语义。
// 覆盖等值条件、Finished 布尔、结束时间区间含等于、空结束时间不匹配任何区间、排序与分页规范化。
// @param t 测试上下文
func TestQueryHistoricProcessInstancesMemoryFilter(t *testing.T) {
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	end0 := base.Add(time.Hour)
	end1 := base.Add(2 * time.Hour)
	newEngine := func(t *testing.T) *Engine {
		t.Helper()
		engine := newTestEngine(t)
		// 未配置行仓储，查询只能走内存路径
		engine.state.HistoricProcesses["h1"] = &api.HistoricProcessInstance{ID: "h1", ProcessDefinitionID: "def-1", ProcessDefinitionKey: "keyA", BusinessKey: "biz1", StartTime: base}
		engine.state.HistoricProcesses["h2"] = &api.HistoricProcessInstance{ID: "h2", ProcessDefinitionID: "def-2", ProcessDefinitionKey: "keyB", BusinessKey: "biz2", StartTime: base, EndTime: &end0, Ended: true}
		engine.state.HistoricProcesses["h3"] = &api.HistoricProcessInstance{ID: "h3", ProcessDefinitionID: "def-1", StartTime: base.Add(time.Hour), EndTime: &end1, Ended: true}
		// 非默认租户历史实例对空租户请求不可见
		engine.state.HistoricProcesses["ht2"] = &api.HistoricProcessInstance{ID: "ht2", TenantID: "t2", StartTime: base, EndTime: &end0, Ended: true}
		return engine
	}
	t.Run("空条件返回全部可见历史", func(t *testing.T) {
		page, err := newEngine(t).QueryHistoricProcessInstances(context.Background(), api.HistoricProcessInstanceFilter{})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Total != 3 || page.Start != 0 || page.Size != 10 {
			t.Fatalf("分页信息错误: %+v", page)
		}
		// 开始时间相同按 ID 升序并列排序
		assertHistoricOrder(t, page, "h1", "h2", "h3")
	})
	t.Run("等值字段", func(t *testing.T) {
		page, err := newEngine(t).QueryHistoricProcessInstances(context.Background(), api.HistoricProcessInstanceFilter{ID: "h2"})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricOrder(t, page, "h2")
		page, err = newEngine(t).QueryHistoricProcessInstances(context.Background(), api.HistoricProcessInstanceFilter{ProcessDefinitionID: "def-1"})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricOrder(t, page, "h1", "h3")
		page, err = newEngine(t).QueryHistoricProcessInstances(context.Background(), api.HistoricProcessInstanceFilter{ProcessDefinitionKey: "keyB"})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricOrder(t, page, "h2")
		page, err = newEngine(t).QueryHistoricProcessInstances(context.Background(), api.HistoricProcessInstanceFilter{BusinessKey: "biz2"})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricOrder(t, page, "h2")
	})
	t.Run("Finished过滤", func(t *testing.T) {
		finished := true
		page, err := newEngine(t).QueryHistoricProcessInstances(context.Background(), api.HistoricProcessInstanceFilter{Finished: &finished})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricOrder(t, page, "h2", "h3")
		running := false
		page, err = newEngine(t).QueryHistoricProcessInstances(context.Background(), api.HistoricProcessInstanceFilter{Finished: &running})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricOrder(t, page, "h1")
	})
	t.Run("租户过滤", func(t *testing.T) {
		engine := newTestEngine(t)
		engine.cfg.TenantID = "t2"
		engine.state.HistoricProcesses["a"] = &api.HistoricProcessInstance{ID: "a", TenantID: "t2", StartTime: base}
		engine.state.HistoricProcesses["b"] = &api.HistoricProcessInstance{ID: "b", TenantID: "t3", StartTime: base}
		// filter.TenantID 与请求租户一致时只返回该租户历史实例
		page, err := engine.QueryHistoricProcessInstances(context.Background(), api.HistoricProcessInstanceFilter{TenantID: "t2"})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricOrder(t, page, "a")
		// filter.TenantID 与请求租户不一致时历史实例被 allowTenant 隐藏
		page, err = engine.QueryHistoricProcessInstances(context.Background(), api.HistoricProcessInstanceFilter{TenantID: "t3"})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricOrder(t, page)
	})
	t.Run("结束时间边界含等于", func(t *testing.T) {
		// 下界含等于：结束时间等于边界仍匹配
		bound := end0
		page, err := newEngine(t).QueryHistoricProcessInstances(context.Background(), api.HistoricProcessInstanceFilter{FinishedAfter: &bound})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricOrder(t, page, "h2", "h3")
		// 上界含等于：晚于边界的实例被排除
		page, err = newEngine(t).QueryHistoricProcessInstances(context.Background(), api.HistoricProcessInstanceFilter{FinishedBefore: &bound})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricOrder(t, page, "h2")
		// 下界超出 1 纳秒即不再含等于
		strict := end0.Add(time.Nanosecond)
		page, err = newEngine(t).QueryHistoricProcessInstances(context.Background(), api.HistoricProcessInstanceFilter{FinishedAfter: &strict})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricOrder(t, page, "h3")
	})
	t.Run("空结束时间不匹配结束区间", func(t *testing.T) {
		// h1 结束时间为空，设置了任一结束区间即不匹配，即使区间语义上不冲突
		early := base.Add(-time.Hour)
		page, err := newEngine(t).QueryHistoricProcessInstances(context.Background(), api.HistoricProcessInstanceFilter{FinishedBefore: &early})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		// h2、h3 的结束时间晚于上界同样被排除，结果为空
		assertHistoricOrder(t, page)
		// 区间即使覆盖所有已结束实例，空结束时间的 h1 也不参与
		bound := end1
		page, err = newEngine(t).QueryHistoricProcessInstances(context.Background(), api.HistoricProcessInstanceFilter{FinishedAfter: &bound})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricOrder(t, page, "h3")
	})
	t.Run("开始时间边界含等于", func(t *testing.T) {
		// 下界含等于：开始时间等于边界仍匹配
		bound := base
		page, err := newEngine(t).QueryHistoricProcessInstances(context.Background(), api.HistoricProcessInstanceFilter{StartedAfter: &bound})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricOrder(t, page, "h1", "h2", "h3")
		// 上界含等于：晚于边界的实例被排除
		page, err = newEngine(t).QueryHistoricProcessInstances(context.Background(), api.HistoricProcessInstanceFilter{StartedBefore: &bound})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricOrder(t, page, "h1", "h2")
		// 下界超出 1 纳秒即不再含等于
		strict := base.Add(time.Nanosecond)
		page, err = newEngine(t).QueryHistoricProcessInstances(context.Background(), api.HistoricProcessInstanceFilter{StartedAfter: &strict})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricOrder(t, page, "h3")
	})
	t.Run("排序与分页切片", func(t *testing.T) {
		// 每页数量小于剩余条数时按 start+size 截断
		page, err := newEngine(t).QueryHistoricProcessInstances(context.Background(), api.HistoricProcessInstanceFilter{Start: 0, Size: 2})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Total != 3 || page.Start != 0 || page.Size != 2 {
			t.Fatalf("分页信息错误: %+v", page)
		}
		assertHistoricOrder(t, page, "h1", "h2")
		// 偏移 1 起剩余条数不大于每页数量时返回到末尾
		page, err = newEngine(t).QueryHistoricProcessInstances(context.Background(), api.HistoricProcessInstanceFilter{Start: 1, Size: 2})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Total != 3 || page.Start != 1 || page.Size != 2 {
			t.Fatalf("分页信息错误: %+v", page)
		}
		assertHistoricOrder(t, page, "h2", "h3")
	})
	t.Run("偏移越界返回空数据", func(t *testing.T) {
		page, err := newEngine(t).QueryHistoricProcessInstances(context.Background(), api.HistoricProcessInstanceFilter{Start: 9, Size: 3})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Total != 3 || page.Start != 9 || page.Size != 3 || len(page.Data) != 0 {
			t.Fatalf("越界分页信息错误: %+v", page)
		}
	})
	t.Run("分页参数规范化", func(t *testing.T) {
		// 负偏移归 0，非正每页数量取默认 10
		page, err := newEngine(t).QueryHistoricProcessInstances(context.Background(), api.HistoricProcessInstanceFilter{Start: -2, Size: -1})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Start != 0 || page.Size != 10 || page.Total != 3 {
			t.Fatalf("规范化分页信息错误: %+v", page)
		}
		assertHistoricOrder(t, page, "h1", "h2", "h3")
		// 每页数量封顶 1000
		page, err = newEngine(t).QueryHistoricProcessInstances(context.Background(), api.HistoricProcessInstanceFilter{Start: 0, Size: 1500})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Size != 1000 {
			t.Fatalf("每页数量应封顶 1000: %+v", page)
		}
	})
}

// TestMatchesHistoricProcessInstanceFilterTenant 验证历史实例内存匹配的租户等值语义。
// 空租户条件不过滤，非空租户要求完全一致，与 SQL TENANT_ID_ 等值语义一致。
// @param t 测试上下文
func TestMatchesHistoricProcessInstanceFilterTenant(t *testing.T) {
	historic := api.HistoricProcessInstance{ID: "h1", TenantID: "t1"}
	if !matchesHistoricProcessInstanceFilter(historic, api.HistoricProcessInstanceFilter{}) {
		t.Fatal("空租户条件不应过滤历史实例")
	}
	if !matchesHistoricProcessInstanceFilter(historic, api.HistoricProcessInstanceFilter{TenantID: "t1"}) {
		t.Fatal("租户一致应匹配")
	}
	if matchesHistoricProcessInstanceFilter(historic, api.HistoricProcessInstanceFilter{TenantID: "t2"}) {
		t.Fatal("租户不一致不应匹配")
	}
}
