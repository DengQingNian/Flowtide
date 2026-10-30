package engine

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/tenant"
)

// 历史任务下推查询的 SQL 形状常量，与 internal/persistence/flowable 的白名单拼接结果逐字一致，
// 配合 QueryMatcherEqual 精确断言可以同时验证 SQL 文本与参数绑定顺序。
const (
	// historicTaskCountSQL 是无条件历史任务的计数 SQL，历史任务查询无条件时不含 WHERE 片段
	historicTaskCountSQL = "SELECT COUNT(*) FROM ACT_HI_TASKINST T"
	// historicTaskSelectSQL 是无条件历史任务的 ID 列表 SQL
	historicTaskSelectSQL = "SELECT T.ID_ FROM ACT_HI_TASKINST T ORDER BY T.START_TIME_ ASC, T.ID_ ASC LIMIT ? OFFSET ?"
	// historicTaskTenantCountSQL 是带租户条件的历史任务计数 SQL
	historicTaskTenantCountSQL = "SELECT COUNT(*) FROM ACT_HI_TASKINST T WHERE T.TENANT_ID_ = ?"
	// historicTaskTenantSelectSQL 是带租户条件的历史任务 ID 列表 SQL
	historicTaskTenantSelectSQL = "SELECT T.ID_ FROM ACT_HI_TASKINST T WHERE T.TENANT_ID_ = ? ORDER BY T.START_TIME_ ASC, T.ID_ ASC LIMIT ? OFFSET ?"
)

// historicTaskIDsOf 提取历史任务分页数据的 ID 列表，便于顺序断言。
// @param page 历史任务分页
// @return ID 列表
func historicTaskIDsOf(page api.Page[api.HistoricTaskInstance]) []string {
	ids := make([]string, 0, len(page.Data))
	for _, item := range page.Data {
		ids = append(ids, item.ID)
	}
	return ids
}

// assertHistoricTaskOrder 断言历史任务分页数据的 ID 顺序与期望一致。
// @param t 测试上下文
// @param page 历史任务分页
// @param want 期望的 ID 顺序
func assertHistoricTaskOrder(t *testing.T, page api.Page[api.HistoricTaskInstance], want ...string) {
	t.Helper()
	if len(page.Data) != len(want) {
		t.Fatalf("历史任务数量错误: 期望 %d 实际 %d (%v)", len(want), len(page.Data), historicTaskIDsOf(page))
	}
	for i, id := range want {
		if page.Data[i].ID != id {
			t.Fatalf("历史任务顺序错误: 期望 %v 实际 %v", want, historicTaskIDsOf(page))
		}
	}
}

// TestQueryHistoricTaskInstancesRowPushdown 验证历史任务下推查询遵循 SQL 顺序与总数。
// SQL 返回 ID 顺序与内存开始时间顺序相反，结果必须按 SQL 顺序返回；
// 缺失 ID 与租户不可见的历史任务被跳过，未结束任务与已结束任务都应返回。
// @param t 测试上下文
func TestQueryHistoricTaskInstancesRowPushdown(t *testing.T) {
	engine, mock := newTaskQueryEngine(t)
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	ended := base.Add(3 * time.Hour)
	// 内存开始时间与 SQL 顺序相反，t2 已结束但历史查询不应跳过
	engine.state.HistoricTasks["t1"] = &api.HistoricTaskInstance{ID: "t1", StartTime: base.Add(2 * time.Hour)}
	engine.state.HistoricTasks["t2"] = &api.HistoricTaskInstance{ID: "t2", StartTime: base.Add(time.Hour), EndTime: &ended}
	engine.state.HistoricTasks["t3"] = &api.HistoricTaskInstance{ID: "t3", StartTime: base}
	// 流程实例缺失导致归属校验失败，SQL 返回也不可见
	engine.state.HistoricTasks["thidden"] = &api.HistoricTaskInstance{ID: "thidden", ProcessInstanceID: "p-missing", StartTime: base}
	mock.ExpectQuery(historicTaskCountSQL).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(5)))
	mock.ExpectQuery(historicTaskSelectSQL).
		WithArgs(int64(10), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"ID_"}).
			AddRow("t3").AddRow("t2").AddRow("不存在任务").AddRow("thidden").AddRow("t1"))
	page, err := engine.QueryHistoricTaskInstances(context.Background(), api.HistoricTaskInstanceFilter{})
	if err != nil {
		t.Fatalf("下推查询失败: %v", err)
	}
	// 总数来自 SQL 计数，而非内存匹配数量
	if page.Total != 5 || page.Start != 0 || page.Size != 10 {
		t.Fatalf("分页信息错误: %+v", page)
	}
	// 缺失 ID 与归属校验失败的行被跳过，其余按 SQL 顺序返回
	assertHistoricTaskOrder(t, page, "t3", "t2", "t1")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryHistoricTaskInstancesRowError 验证历史任务 SQL 失败直接返回 ROW-0051 且不回退内存。
// 内存中存在匹配数据时也必须报错，防止静默回退掩盖数据库问题。
// @param t 测试上下文
func TestQueryHistoricTaskInstancesRowError(t *testing.T) {
	engine, mock := newTaskQueryEngine(t)
	engine.state.HistoricTasks["t1"] = &api.HistoricTaskInstance{ID: "t1"}
	// 仅设置计数期望并返回错误；若实现继续执行 SELECT 或回退内存，断言都会失败
	mock.ExpectQuery(historicTaskCountSQL).WillReturnError(errors.New("数据库不可用"))
	page, err := engine.QueryHistoricTaskInstances(context.Background(), api.HistoricTaskInstanceFilter{})
	assertRowErrorCode(t, err, "ROW-0051")
	if page.Total != 0 || len(page.Data) != 0 {
		t.Fatalf("失败查询必须返回空结果: %+v", page)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryHistoricTaskInstancesTenantPushdown 验证租户生效时历史任务查询下推 SQL。
// 引擎绑定租户、请求携带租户或请求指定租户过滤时以 T.TENANT_ID_ 等值条件查询，
// 参数绑定必须精确匹配；回退内存会命中未设置的期望而失败。
// @param t 测试上下文
func TestQueryHistoricTaskInstancesTenantPushdown(t *testing.T) {
	cases := []struct {
		name   string                         // 用例名
		bound  string                         // 引擎绑定的默认租户
		filter api.HistoricTaskInstanceFilter // 查询条件
		ctx    func() context.Context         // 请求上下文
	}{
		{
			name:   "绑定默认租户",
			bound:  "t1",
			filter: api.HistoricTaskInstanceFilter{},
			ctx:    func() context.Context { return context.Background() },
		},
		{
			name:   "请求租户上下文",
			bound:  "",
			filter: api.HistoricTaskInstanceFilter{},
			ctx:    func() context.Context { return tenant.With(context.Background(), "t1") },
		},
		{
			name:   "请求租户过滤",
			bound:  "",
			filter: api.HistoricTaskInstanceFilter{TenantID: "t1"},
			ctx:    func() context.Context { return tenant.With(context.Background(), "t1") },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine, mock := newTaskQueryEngine(t)
			engine.cfg.TenantID = tc.bound
			engine.state.HistoricTasks["t1"] = &api.HistoricTaskInstance{ID: "t1", TenantID: "t1", StartTime: time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)}
			// 计数与 ID 列表都以生效租户 t1 作为第一个绑定参数
			mock.ExpectQuery(historicTaskTenantCountSQL).
				WithArgs("t1").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
			mock.ExpectQuery(historicTaskTenantSelectSQL).
				WithArgs("t1", int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"ID_"}).AddRow("t1"))
			page, err := engine.QueryHistoricTaskInstances(tc.ctx(), tc.filter)
			if err != nil {
				t.Fatalf("下推查询失败: %v", err)
			}
			if page.Total != 1 || page.Start != 0 || page.Size != 10 {
				t.Fatalf("分页信息错误: %+v", page)
			}
			assertHistoricTaskOrder(t, page, "t1")
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricTaskInstancesMultiTenantTenantFilterPushdown 验证多租户历史数据下指定租户过滤仍下推 SQL。
// 生效租户非空时非默认租户历史数据不再触发回退，SQL 以 TENANT_ID_ 等值条件收敛，
// 默认租户历史任务不进入结果，总数以 SQL 计数为准。
// @param t 测试上下文
func TestQueryHistoricTaskInstancesMultiTenantTenantFilterPushdown(t *testing.T) {
	engine, mock := newTaskQueryEngine(t)
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	engine.state.HistoricTasks["t1"] = &api.HistoricTaskInstance{ID: "t1", StartTime: base.Add(time.Hour)}
	engine.state.HistoricTasks["t2"] = &api.HistoricTaskInstance{ID: "t2", TenantID: "t2", StartTime: base}
	// 多租户历史数据与指定租户过滤并存时仍下推，条件参数为生效租户 t2
	mock.ExpectQuery(historicTaskTenantCountSQL).
		WithArgs("t2").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery(historicTaskTenantSelectSQL).
		WithArgs("t2", int64(10), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"ID_"}).AddRow("t2"))
	page, err := engine.QueryHistoricTaskInstances(tenant.With(context.Background(), "t2"), api.HistoricTaskInstanceFilter{TenantID: "t2"})
	if err != nil {
		t.Fatalf("下推查询失败: %v", err)
	}
	// 总数来自 SQL 计数；默认租户历史任务 t1 不在结果内
	if page.Total != 1 {
		t.Fatalf("总数错误: %+v", page)
	}
	assertHistoricTaskOrder(t, page, "t2")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryHistoricTaskInstancesFallbackMultiTenant 验证内存存在非默认租户历史任务时回退内存过滤。
// 任务 DTO 与历史实例任一记录非默认租户都触发回退，非默认租户历史任务被归属校验隐藏。
// @param t 测试上下文
func TestQueryHistoricTaskInstancesFallbackMultiTenant(t *testing.T) {
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name string               // 用例名
		seed func(engine *Engine) // 非默认租户数据的注入方式
	}{
		{
			name: "历史任务记录非默认租户",
			seed: func(engine *Engine) {
				engine.state.HistoricTasks["t1"] = &api.HistoricTaskInstance{ID: "t1", StartTime: base.Add(time.Hour)}
				engine.state.HistoricTasks["t2"] = &api.HistoricTaskInstance{ID: "t2", TenantID: "t2", StartTime: base}
			},
		},
		{
			name: "历史实例记录非默认租户",
			seed: func(engine *Engine) {
				engine.state.HistoricTasks["t1"] = &api.HistoricTaskInstance{ID: "t1", StartTime: base.Add(time.Hour)}
				engine.state.HistoricTasks["t2"] = &api.HistoricTaskInstance{ID: "t2", StartTime: base, ProcessInstanceID: "p2"}
				engine.state.HistoricProcesses["p2"] = &api.HistoricProcessInstance{ID: "p2", TenantID: "t2"}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine, mock := newTaskQueryEngine(t)
			tc.seed(engine)
			// 多租户历史状态必须回退内存，任何 SQL 下推都会命中未设置的期望而失败
			page, err := engine.QueryHistoricTaskInstances(context.Background(), api.HistoricTaskInstanceFilter{})
			if err != nil {
				t.Fatalf("内存查询失败: %v", err)
			}
			if page.Total != 1 || page.Start != 0 || page.Size != 10 {
				t.Fatalf("分页信息错误: %+v", page)
			}
			assertHistoricTaskOrder(t, page, "t1")
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricTaskInstancesMemoryFilter 验证未配置行仓储时历史任务内存过滤语义。
// 覆盖等值条件、定义 Key 反查、Finished 布尔、开始与结束时间边界含等于、空结束时间不匹配任何区间、
// 排序并列规则、分页切片、偏移越界与分页参数规范化。
// @param t 测试上下文
func TestQueryHistoricTaskInstancesMemoryFilter(t *testing.T) {
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	end0 := base.Add(time.Hour)
	end1 := base.Add(2 * time.Hour)
	newEngine := func(t *testing.T) *Engine {
		t.Helper()
		engine := newTestEngine(t)
		// 未配置行仓储，查询只能走内存路径
		engine.state.HistoricTasks["t1"] = &api.HistoricTaskInstance{ID: "t1", Name: "任务A", Assignee: "alice", TaskDefinitionKey: "userTask1", ProcessInstanceID: "p1", ProcessDefinitionID: "def-1", StartTime: base}
		engine.state.HistoricTasks["t2"] = &api.HistoricTaskInstance{ID: "t2", Name: "任务B", Assignee: "bob", TaskDefinitionKey: "userTask2", ProcessInstanceID: "p2", ProcessDefinitionID: "def-2", StartTime: base, EndTime: &end0}
		engine.state.HistoricTasks["t3"] = &api.HistoricTaskInstance{ID: "t3", Name: "任务C", Assignee: "alice", TaskDefinitionKey: "userTask1", ProcessInstanceID: "p3", ProcessDefinitionID: "def-1", StartTime: base.Add(time.Hour), EndTime: &end1}
		// 定义缺失的历史任务用于验证 Key 条件下的不匹配语义
		engine.state.HistoricTasks["t4"] = &api.HistoricTaskInstance{ID: "t4", Name: "任务D", ProcessDefinitionID: "def-x", StartTime: base}
		// 非默认租户历史任务对空租户请求不可见
		engine.state.HistoricTasks["tt2"] = &api.HistoricTaskInstance{ID: "tt2", TenantID: "t2", StartTime: base, EndTime: &end0}
		// 归属校验以历史实例租户优先，补齐流程实例历史
		engine.state.HistoricProcesses["p1"] = &api.HistoricProcessInstance{ID: "p1"}
		engine.state.HistoricProcesses["p2"] = &api.HistoricProcessInstance{ID: "p2"}
		engine.state.HistoricProcesses["p3"] = &api.HistoricProcessInstance{ID: "p3"}
		// 定义 Key 经流程定义反查，补齐定义状态
		engine.state.Definitions["def-1"] = &definitionState{Info: api.ProcessDefinition{ID: "def-1", Key: "keyA"}}
		engine.state.Definitions["def-2"] = &definitionState{Info: api.ProcessDefinition{ID: "def-2", Key: "keyB"}}
		return engine
	}
	t.Run("空条件返回全部可见历史任务", func(t *testing.T) {
		page, err := newEngine(t).QueryHistoricTaskInstances(context.Background(), api.HistoricTaskInstanceFilter{})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Total != 4 || page.Start != 0 || page.Size != 10 {
			t.Fatalf("分页信息错误: %+v", page)
		}
		// 开始时间相同按 ID 升序并列排序
		assertHistoricTaskOrder(t, page, "t1", "t2", "t4", "t3")
	})
	t.Run("等值字段", func(t *testing.T) {
		cases := []struct {
			name   string                         // 用例名
			filter api.HistoricTaskInstanceFilter // 查询条件
			want   []string                       // 期望结果
		}{
			{name: "任务ID", filter: api.HistoricTaskInstanceFilter{ID: "t2"}, want: []string{"t2"}},
			{name: "名称", filter: api.HistoricTaskInstanceFilter{Name: "任务A"}, want: []string{"t1"}},
			{name: "处理人", filter: api.HistoricTaskInstanceFilter{Assignee: "alice"}, want: []string{"t1", "t3"}},
			{name: "任务定义Key", filter: api.HistoricTaskInstanceFilter{TaskDefinitionKey: "userTask2"}, want: []string{"t2"}},
			{name: "流程实例ID", filter: api.HistoricTaskInstanceFilter{ProcessInstanceID: "p3"}, want: []string{"t3"}},
			{name: "定义ID", filter: api.HistoricTaskInstanceFilter{ProcessDefinitionID: "def-1"}, want: []string{"t1", "t3"}},
			{name: "不存在的处理人", filter: api.HistoricTaskInstanceFilter{Assignee: "carol"}, want: []string{}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				page, err := newEngine(t).QueryHistoricTaskInstances(context.Background(), tc.filter)
				if err != nil {
					t.Fatalf("内存查询失败: %v", err)
				}
				assertHistoricTaskOrder(t, page, tc.want...)
			})
		}
	})
	t.Run("定义Key经流程定义反查", func(t *testing.T) {
		page, err := newEngine(t).QueryHistoricTaskInstances(context.Background(), api.HistoricTaskInstanceFilter{ProcessDefinitionKey: "keyA"})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		// keyA 命中 def-1 下的任务，def-x 缺失定义视为不匹配
		assertHistoricTaskOrder(t, page, "t1", "t3")
		page, err = newEngine(t).QueryHistoricTaskInstances(context.Background(), api.HistoricTaskInstanceFilter{ProcessDefinitionKey: "keyB"})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricTaskOrder(t, page, "t2")
		// 不存在的 Key 不匹配任何任务
		page, err = newEngine(t).QueryHistoricTaskInstances(context.Background(), api.HistoricTaskInstanceFilter{ProcessDefinitionKey: "keyZ"})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricTaskOrder(t, page)
	})
	t.Run("Finished过滤", func(t *testing.T) {
		finished := true
		page, err := newEngine(t).QueryHistoricTaskInstances(context.Background(), api.HistoricTaskInstanceFilter{Finished: &finished})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		// 结束状态以结束时间是否为空判断，租户不可见任务仍被隐藏
		assertHistoricTaskOrder(t, page, "t2", "t3")
		running := false
		page, err = newEngine(t).QueryHistoricTaskInstances(context.Background(), api.HistoricTaskInstanceFilter{Finished: &running})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricTaskOrder(t, page, "t1", "t4")
	})
	t.Run("开始时间边界含等于", func(t *testing.T) {
		// 下界含等于：开始时间等于边界仍匹配
		bound := base
		page, err := newEngine(t).QueryHistoricTaskInstances(context.Background(), api.HistoricTaskInstanceFilter{StartedAfter: &bound})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricTaskOrder(t, page, "t1", "t2", "t4", "t3")
		// 上界含等于：晚于边界的任务被排除
		page, err = newEngine(t).QueryHistoricTaskInstances(context.Background(), api.HistoricTaskInstanceFilter{StartedBefore: &bound})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricTaskOrder(t, page, "t1", "t2", "t4")
		// 下界超出 1 纳秒即不再含等于
		strict := base.Add(time.Nanosecond)
		page, err = newEngine(t).QueryHistoricTaskInstances(context.Background(), api.HistoricTaskInstanceFilter{StartedAfter: &strict})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricTaskOrder(t, page, "t3")
	})
	t.Run("结束时间边界含等于", func(t *testing.T) {
		// 下界含等于：结束时间等于边界仍匹配
		bound := end0
		page, err := newEngine(t).QueryHistoricTaskInstances(context.Background(), api.HistoricTaskInstanceFilter{FinishedAfter: &bound})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricTaskOrder(t, page, "t2", "t3")
		// 上界含等于：晚于边界的任务被排除
		page, err = newEngine(t).QueryHistoricTaskInstances(context.Background(), api.HistoricTaskInstanceFilter{FinishedBefore: &bound})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricTaskOrder(t, page, "t2")
		// 下界超出 1 纳秒即不再含等于
		strict := end0.Add(time.Nanosecond)
		page, err = newEngine(t).QueryHistoricTaskInstances(context.Background(), api.HistoricTaskInstanceFilter{FinishedAfter: &strict})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricTaskOrder(t, page, "t3")
	})
	t.Run("空结束时间不匹配结束区间", func(t *testing.T) {
		// t1、t4 结束时间为空，设置了任一结束区间即不匹配，即使区间语义上不冲突
		early := base.Add(-time.Hour)
		page, err := newEngine(t).QueryHistoricTaskInstances(context.Background(), api.HistoricTaskInstanceFilter{FinishedBefore: &early})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		// t2、t3 的结束时间晚于上界同样被排除，结果为空
		assertHistoricTaskOrder(t, page)
		// 区间即使晚于所有已结束任务，空结束时间的任务也不参与
		bound := end1
		page, err = newEngine(t).QueryHistoricTaskInstances(context.Background(), api.HistoricTaskInstanceFilter{FinishedAfter: &bound})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricTaskOrder(t, page, "t3")
	})
	t.Run("租户过滤", func(t *testing.T) {
		engine := newTestEngine(t)
		engine.cfg.TenantID = "t2"
		// 独立历史任务的归属校验退化为任务自身租户
		engine.state.HistoricTasks["a"] = &api.HistoricTaskInstance{ID: "a", TenantID: "t2", StartTime: base}
		engine.state.HistoricTasks["b"] = &api.HistoricTaskInstance{ID: "b", TenantID: "t3", StartTime: base}
		// filter.TenantID 与请求租户一致时只返回该租户历史任务
		page, err := engine.QueryHistoricTaskInstances(context.Background(), api.HistoricTaskInstanceFilter{TenantID: "t2"})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricTaskOrder(t, page, "a")
		// filter.TenantID 与请求租户不一致时历史任务被归属校验隐藏
		page, err = engine.QueryHistoricTaskInstances(context.Background(), api.HistoricTaskInstanceFilter{TenantID: "t3"})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		assertHistoricTaskOrder(t, page)
	})
	t.Run("排序与分页切片", func(t *testing.T) {
		// 每页数量小于剩余条数时按 start+size 截断
		page, err := newEngine(t).QueryHistoricTaskInstances(context.Background(), api.HistoricTaskInstanceFilter{Start: 0, Size: 2})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Total != 4 || page.Start != 0 || page.Size != 2 {
			t.Fatalf("分页信息错误: %+v", page)
		}
		assertHistoricTaskOrder(t, page, "t1", "t2")
		// 偏移 1 起剩余条数不大于每页数量时返回到末尾
		page, err = newEngine(t).QueryHistoricTaskInstances(context.Background(), api.HistoricTaskInstanceFilter{Start: 1, Size: 3})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Total != 4 || page.Start != 1 || page.Size != 3 {
			t.Fatalf("分页信息错误: %+v", page)
		}
		assertHistoricTaskOrder(t, page, "t2", "t4", "t3")
	})
	t.Run("偏移越界返回空数据", func(t *testing.T) {
		page, err := newEngine(t).QueryHistoricTaskInstances(context.Background(), api.HistoricTaskInstanceFilter{Start: 9, Size: 3})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Total != 4 || page.Start != 9 || page.Size != 3 || len(page.Data) != 0 {
			t.Fatalf("越界分页信息错误: %+v", page)
		}
		// 极大每页数量与偏移相加不得溢出，偏移 1 起返回到末尾，数量按规范化封顶
		page, err = newEngine(t).QueryHistoricTaskInstances(context.Background(), api.HistoricTaskInstanceFilter{Start: 1, Size: math.MaxInt})
		if err != nil {
			t.Fatalf("极大分页查询失败: %v", err)
		}
		if page.Total != 4 || page.Size != 1000 || len(page.Data) != 3 {
			t.Fatalf("极大分页信息错误: %+v", page)
		}
		assertHistoricTaskOrder(t, page, "t2", "t4", "t3")
	})
	t.Run("分页参数规范化", func(t *testing.T) {
		// 负偏移归 0，非正每页数量取默认 10
		page, err := newEngine(t).QueryHistoricTaskInstances(context.Background(), api.HistoricTaskInstanceFilter{Start: -2, Size: -1})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Start != 0 || page.Size != 10 || page.Total != 4 {
			t.Fatalf("规范化分页信息错误: %+v", page)
		}
		assertHistoricTaskOrder(t, page, "t1", "t2", "t4", "t3")
		// 每页数量封顶 1000
		page, err = newEngine(t).QueryHistoricTaskInstances(context.Background(), api.HistoricTaskInstanceFilter{Start: 0, Size: 1500})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Size != 1000 {
			t.Fatalf("每页数量应封顶 1000: %+v", page)
		}
	})
}

// TestMatchesHistoricTaskInstanceFilterSemantics 验证历史任务内存匹配的租户与结束状态语义。
// 空租户条件不过滤，非空租户要求完全一致；Finished 与 SQL END_TIME_ 判空语义一致。
// @param t 测试上下文
func TestMatchesHistoricTaskInstanceFilterSemantics(t *testing.T) {
	engine := newTestEngine(t)
	end := time.Date(2026, 10, 28, 1, 0, 0, 0, time.UTC)
	historic := api.HistoricTaskInstance{ID: "t1", TenantID: "t1", EndTime: &end}
	if !engine.matchesHistoricTaskInstanceFilter(historic, api.HistoricTaskInstanceFilter{}) {
		t.Fatal("空条件不应过滤历史任务")
	}
	if !engine.matchesHistoricTaskInstanceFilter(historic, api.HistoricTaskInstanceFilter{TenantID: "t1"}) {
		t.Fatal("租户一致应匹配")
	}
	if engine.matchesHistoricTaskInstanceFilter(historic, api.HistoricTaskInstanceFilter{TenantID: "t2"}) {
		t.Fatal("租户不一致不应匹配")
	}
	// 已结束任务匹配 Finished 为真、不匹配为假
	finished := true
	if !engine.matchesHistoricTaskInstanceFilter(historic, api.HistoricTaskInstanceFilter{Finished: &finished}) {
		t.Fatal("已结束任务应匹配 Finished 为真")
	}
	running := false
	if engine.matchesHistoricTaskInstanceFilter(historic, api.HistoricTaskInstanceFilter{Finished: &running}) {
		t.Fatal("已结束任务不应匹配 Finished 为假")
	}
	// 未结束任务以结束时间为空表达
	open := api.HistoricTaskInstance{ID: "t2", TenantID: "t1"}
	if !engine.matchesHistoricTaskInstanceFilter(open, api.HistoricTaskInstanceFilter{Finished: &running}) {
		t.Fatal("未结束任务应匹配 Finished 为假")
	}
	if engine.matchesHistoricTaskInstanceFilter(open, api.HistoricTaskInstanceFilter{Finished: &finished}) {
		t.Fatal("未结束任务不应匹配 Finished 为真")
	}
}
