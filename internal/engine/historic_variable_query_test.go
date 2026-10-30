package engine

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/tenant"
)

// 历史变量下推查询的 SQL 形状常量，与 internal/persistence/flowable 的白名单拼接结果逐字一致，
// 配合 QueryMatcherEqual 精确断言可以同时验证 SQL 文本与参数绑定顺序。
const (
	// historicVariableCountSQL 是无条件历史变量的计数 SQL，历史变量查询无条件时不含 WHERE 片段
	historicVariableCountSQL = "SELECT COUNT(*) FROM ACT_HI_VARINST V"
	// historicVariableSelectSQL 是无条件历史变量的 ID 列表 SQL
	historicVariableSelectSQL = "SELECT V.ID_ FROM ACT_HI_VARINST V ORDER BY V.NAME_ ASC, V.ID_ ASC LIMIT ? OFFSET ?"
)

// historicVariableIDsOf 提取历史变量分页数据的 ID 列表，便于顺序断言。
// @param page 历史变量分页
// @return ID 列表
func historicVariableIDsOf(page api.Page[api.HistoricVariableInstance]) []string {
	ids := make([]string, 0, len(page.Data))
	for _, item := range page.Data {
		ids = append(ids, item.ID)
	}
	return ids
}

// assertHistoricVariableOrder 断言历史变量分页数据的 ID 顺序与期望一致。
// @param t 测试上下文
// @param page 历史变量分页
// @param want 期望的 ID 顺序
func assertHistoricVariableOrder(t *testing.T, page api.Page[api.HistoricVariableInstance], want ...string) {
	t.Helper()
	if len(page.Data) != len(want) {
		t.Fatalf("历史变量数量错误: 期望 %d 实际 %d (%v)", len(want), len(page.Data), historicVariableIDsOf(page))
	}
	for i, id := range want {
		if page.Data[i].ID != id {
			t.Fatalf("历史变量顺序错误: 期望 %v 实际 %v", want, historicVariableIDsOf(page))
		}
	}
}

// TestQueryHistoricVariableInstancesRowPushdown 验证历史变量下推查询遵循 SQL 顺序、总数并完整映射 DTO。
// SQL 返回 ID 顺序与内存变量名顺序相反，结果必须按 SQL 顺序返回；
// 缺失 ID 与归属任务缺失的历史变量被跳过。
// @param t 测试上下文
func TestQueryHistoricVariableInstancesRowPushdown(t *testing.T) {
	engine, mock := newTaskQueryEngine(t)
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	// 默认租户历史实例保证历史变量通过归属校验
	engine.state.HistoricProcesses["p1"] = &api.HistoricProcessInstance{ID: "p1"}
	engine.state.HistoricProcesses["p2"] = &api.HistoricProcessInstance{ID: "p2"}
	engine.state.HistoricProcesses["p3"] = &api.HistoricProcessInstance{ID: "p3"}
	v1 := &api.HistoricVariableInstance{
		ID: "v1", Name: "zeta", Type: "string", ProcessInstanceID: "p1", ExecutionID: "e1",
		Value: "值", CreateTime: base, LastUpdatedTime: base.Add(time.Hour),
	}
	v2 := &api.HistoricVariableInstance{
		ID: "v2", Name: "alpha", Type: "long", ProcessInstanceID: "p2", ExecutionID: "e2",
		Value: int64(7), CreateTime: base, LastUpdatedTime: base,
	}
	v3 := &api.HistoricVariableInstance{
		ID: "v3", Name: "alpha", Type: "boolean", ProcessInstanceID: "p3", ExecutionID: "e3",
		Value: true, CreateTime: base, LastUpdatedTime: base,
	}
	engine.state.HistoricVariables["v1"] = v1
	engine.state.HistoricVariables["v2"] = v2
	engine.state.HistoricVariables["v3"] = v3
	// 归属任务缺失的历史变量无法通过归属校验，必须被跳过（非默认租户任务会触发整体内存回退，见下个用例）
	engine.state.HistoricVariables["vother"] = &api.HistoricVariableInstance{ID: "vother", Name: "beta", TaskID: "t-missing"}
	mock.ExpectQuery(historicVariableCountSQL).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(5)))
	mock.ExpectQuery(historicVariableSelectSQL).
		WithArgs(int64(10), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"ID_"}).
			AddRow("v2").AddRow("v3").AddRow("不存在变量").AddRow("vother").AddRow("v1"))
	page, err := engine.QueryHistoricVariableInstances(context.Background(), api.HistoricVariableInstanceFilter{})
	if err != nil {
		t.Fatalf("下推查询失败: %v", err)
	}
	// 总数来自 SQL 计数，而非内存匹配数量
	if page.Total != 5 || page.Start != 0 || page.Size != 10 {
		t.Fatalf("分页信息错误: %+v", page)
	}
	// 缺失 ID 与归属失败的行被跳过，其余按 SQL 顺序返回
	assertHistoricVariableOrder(t, page, "v2", "v3", "v1")
	// 逐字段映射：DTO 必须与内存状态完全一致，不能因查询层未使用而丢失列
	want := []api.HistoricVariableInstance{*v2, *v3, *v1}
	for i := range want {
		if !reflect.DeepEqual(page.Data[i], want[i]) {
			t.Fatalf("历史变量字段映射错误: 第 %d 条 期望 %+v 实际 %+v", i, want[i], page.Data[i])
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryHistoricVariableInstancesCompositeKey 验证历史变量以归属复合键建索引时仍能按 SQL 主键装配 DTO。
// 生产写入路径的 map 键为“执行/任务 + 变量名”，与历史主键不同；
// 下推必须按 DTO 的 ID 字段查找，不能把 map 键当作历史主键。
// @param t 测试上下文
func TestQueryHistoricVariableInstancesCompositeKey(t *testing.T) {
	engine, mock := newTaskQueryEngine(t)
	engine.state.HistoricProcesses["p1"] = &api.HistoricProcessInstance{ID: "p1"}
	// 与 recordHistoricVariable 一致的归属复合键，历史主键在 DTO 内
	engine.state.HistoricVariables["e1|amount"] = &api.HistoricVariableInstance{
		ID: "v-uuid", Name: "amount", Type: "long", ProcessInstanceID: "p1", ExecutionID: "e1", Value: int64(100),
	}
	mock.ExpectQuery(historicVariableCountSQL).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery(historicVariableSelectSQL).
		WithArgs(int64(10), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"ID_"}).AddRow("v-uuid"))
	page, err := engine.QueryHistoricVariableInstances(context.Background(), api.HistoricVariableInstanceFilter{})
	if err != nil {
		t.Fatalf("下推查询失败: %v", err)
	}
	if page.Total != 1 || len(page.Data) != 1 || page.Data[0].ID != "v-uuid" {
		t.Fatalf("复合键索引下必须按历史主键装配 DTO: %+v", page)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryHistoricVariableInstancesTenantTaskFallback 验证存在非默认租户独立历史任务时回退内存。
// 历史变量表没有租户列，独立历史任务的变量无法在 SQL 内收敛租户；
// 若仍下推，SQL 计数会把他租户变量计入总数并挤占分页，因此必须整体回退内存并隐藏他租户变量。
// @param t 测试上下文
func TestQueryHistoricVariableInstancesTenantTaskFallback(t *testing.T) {
	engine, _ := newTaskQueryEngine(t)
	engine.state.HistoricProcesses["p1"] = &api.HistoricProcessInstance{ID: "p1"}
	engine.state.HistoricVariables["v1"] = &api.HistoricVariableInstance{ID: "v1", Name: "alpha", ProcessInstanceID: "p1"}
	// 非默认租户的独立历史任务及其任务变量
	engine.state.HistoricTasks["t-other"] = &api.HistoricTaskInstance{ID: "t-other", TenantID: "t2"}
	engine.state.HistoricVariables["vother"] = &api.HistoricVariableInstance{ID: "vother", Name: "beta", TaskID: "t-other"}
	// 不设置任何 SQL 期望：一旦仍下推，sqlmock 会因无期望而报错
	page, err := engine.QueryHistoricVariableInstances(context.Background(), api.HistoricVariableInstanceFilter{})
	if err != nil {
		t.Fatalf("内存回退查询失败: %v", err)
	}
	if page.Total != 1 || len(page.Data) != 1 || page.Data[0].ID != "v1" {
		t.Fatalf("非默认租户任务变量必须被隐藏: %+v", page)
	}
}

// TestQueryHistoricVariableInstancesRowError 验证历史变量 SQL 失败直接返回错误且不回退内存。
// 计数查询执行失败包装为 ROW-0061，行仓储不支持查询返回 ROW-0060；
// 内存中存在匹配数据时也必须报错，防止静默回退掩盖数据库问题。
// @param t 测试上下文
func TestQueryHistoricVariableInstancesRowError(t *testing.T) {
	t.Run("查询执行失败不回退内存", func(t *testing.T) {
		engine, mock := newTaskQueryEngine(t)
		engine.state.HistoricVariables["v1"] = &api.HistoricVariableInstance{ID: "v1", Name: "a"}
		// 仅设置计数期望并返回错误；若实现继续执行 SELECT 或回退内存，断言都会失败
		mock.ExpectQuery(historicVariableCountSQL).WillReturnError(errors.New("数据库不可用"))
		page, err := engine.QueryHistoricVariableInstances(context.Background(), api.HistoricVariableInstanceFilter{})
		assertRowErrorCode(t, err, "ROW-0061")
		if page.Total != 0 || len(page.Data) != 0 {
			t.Fatalf("失败查询必须返回空结果: %+v", page)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("行仓储不支持查询", func(t *testing.T) {
		engine := newNoQueryRowEngine(t)
		engine.state.HistoricVariables["v1"] = &api.HistoricVariableInstance{ID: "v1", Name: "a"}
		page, err := engine.QueryHistoricVariableInstances(context.Background(), api.HistoricVariableInstanceFilter{})
		// 行仓储不可查询属于 SQL 失败，同样不能回退内存
		assertRowErrorCode(t, err, "ROW-0060")
		if page.Total != 0 || len(page.Data) != 0 {
			t.Fatalf("失败查询必须返回空结果: %+v", page)
		}
	})
}

// TestQueryHistoricVariableInstancesTenantFallback 验证租户生效时历史变量查询回退内存。
// ACT_HI_VARINST 无租户列，引擎绑定租户或请求携带租户都无法在 SQL 内收敛，
// 任何 SQL 下推都会命中未设置的期望而失败；其他租户变量被归属校验隐藏。
// @param t 测试上下文
func TestQueryHistoricVariableInstancesTenantFallback(t *testing.T) {
	cases := []struct {
		name  string                 // 用例名
		bound string                 // 引擎绑定的默认租户
		ctx   func() context.Context // 请求上下文
	}{
		{
			name:  "绑定默认租户",
			bound: "t1",
			ctx:   func() context.Context { return context.Background() },
		},
		{
			name:  "请求租户上下文",
			bound: "",
			ctx:   func() context.Context { return tenant.With(context.Background(), "t1") },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine, mock := newTaskQueryEngine(t)
			engine.cfg.TenantID = tc.bound
			// 同租户历史实例下的变量可见，默认租户变量在本租户视图下隐藏
			engine.state.HistoricProcesses["p1"] = &api.HistoricProcessInstance{ID: "p1", TenantID: "t1"}
			engine.state.HistoricProcesses["p0"] = &api.HistoricProcessInstance{ID: "p0"}
			engine.state.HistoricVariables["v1"] = &api.HistoricVariableInstance{ID: "v1", Name: "a", ProcessInstanceID: "p1"}
			engine.state.HistoricVariables["v0"] = &api.HistoricVariableInstance{ID: "v0", Name: "b", ProcessInstanceID: "p0"}
			page, err := engine.QueryHistoricVariableInstances(tc.ctx(), api.HistoricVariableInstanceFilter{})
			if err != nil {
				t.Fatalf("内存查询失败: %v", err)
			}
			if page.Total != 1 || page.Start != 0 || page.Size != 10 {
				t.Fatalf("分页信息错误: %+v", page)
			}
			assertHistoricVariableOrder(t, page, "v1")
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricVariableInstancesFallbackNonDefaultTenant 验证内存存在非默认租户历史实例时回退内存。
// 历史变量表没有租户列，默认租户视图下其他租户历史实例的变量必须回退内存并由归属校验隐藏。
// @param t 测试上下文
func TestQueryHistoricVariableInstancesFallbackNonDefaultTenant(t *testing.T) {
	engine, mock := newTaskQueryEngine(t)
	engine.state.HistoricProcesses["p2"] = &api.HistoricProcessInstance{ID: "p2", TenantID: "t2"}
	engine.state.HistoricVariables["v1"] = &api.HistoricVariableInstance{ID: "v1", Name: "a"}
	engine.state.HistoricVariables["v2"] = &api.HistoricVariableInstance{ID: "v2", Name: "b", ProcessInstanceID: "p2"}
	// 非默认租户历史实例必须回退内存，任何 SQL 下推都会命中未设置的期望而失败
	page, err := engine.QueryHistoricVariableInstances(context.Background(), api.HistoricVariableInstanceFilter{})
	if err != nil {
		t.Fatalf("内存查询失败: %v", err)
	}
	if page.Total != 1 || page.Start != 0 || page.Size != 10 {
		t.Fatalf("分页信息错误: %+v", page)
	}
	assertHistoricVariableOrder(t, page, "v1")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryHistoricVariableInstancesNoRowRepositoryFallback 验证未配置行仓储时回退内存查询。
// 无行仓储只能走内存过滤，变量名升序排序与总数必须完整返回。
// @param t 测试上下文
func TestQueryHistoricVariableInstancesNoRowRepositoryFallback(t *testing.T) {
	engine := newTestEngine(t)
	engine.state.HistoricVariables["v1"] = &api.HistoricVariableInstance{ID: "v1", Name: "b"}
	engine.state.HistoricVariables["v2"] = &api.HistoricVariableInstance{ID: "v2", Name: "a"}
	page, err := engine.QueryHistoricVariableInstances(context.Background(), api.HistoricVariableInstanceFilter{})
	if err != nil {
		t.Fatalf("内存查询失败: %v", err)
	}
	if page.Total != 2 || page.Start != 0 || page.Size != 10 {
		t.Fatalf("分页信息错误: %+v", page)
	}
	assertHistoricVariableOrder(t, page, "v2", "v1")
}

// TestQueryHistoricVariableInstancesMemoryFilter 验证历史变量内存过滤语义。
// 覆盖等值条件（ID、名称、流程实例、执行、任务、变量类型）、变量名与 ID 排序、
// 分页切片、偏移越界、超大偏移不溢出与分页参数规范化。
// @param t 测试上下文
func TestQueryHistoricVariableInstancesMemoryFilter(t *testing.T) {
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	newEngine := func(t *testing.T) *Engine {
		t.Helper()
		engine := newTestEngine(t)
		// 未配置行仓储，查询只能走内存路径
		engine.state.HistoricProcesses["p1"] = &api.HistoricProcessInstance{ID: "p1"}
		engine.state.HistoricProcesses["p2"] = &api.HistoricProcessInstance{ID: "p2"}
		engine.state.HistoricProcesses["p3"] = &api.HistoricProcessInstance{ID: "p3"}
		engine.state.HistoricVariables["v1"] = &api.HistoricVariableInstance{
			ID: "v1", Name: "alpha", Type: "string", ProcessInstanceID: "p1", ExecutionID: "e1",
			Value: "甲", CreateTime: base, LastUpdatedTime: base.Add(time.Hour),
		}
		engine.state.HistoricVariables["v2"] = &api.HistoricVariableInstance{
			ID: "v2", Name: "beta", Type: "long", ProcessInstanceID: "p2", ExecutionID: "e2",
			Value: int64(7), CreateTime: base, LastUpdatedTime: base,
		}
		engine.state.HistoricVariables["v3"] = &api.HistoricVariableInstance{
			ID: "v3", Name: "alpha", Type: "string", ProcessInstanceID: "p3", ExecutionID: "e3",
			Value: int64(9), CreateTime: base, LastUpdatedTime: base,
		}
		// 独立任务变量用于验证任务条件；非默认租户任务变量对空租户请求不可见
		engine.state.HistoricTasks["t1"] = &api.HistoricTaskInstance{ID: "t1"}
		engine.state.HistoricTasks["t2"] = &api.HistoricTaskInstance{ID: "t2", TenantID: "t2"}
		engine.state.HistoricVariables["v4"] = &api.HistoricVariableInstance{ID: "v4", Name: "gamma", TaskID: "t1"}
		engine.state.HistoricVariables["vhidden"] = &api.HistoricVariableInstance{ID: "vhidden", Name: "delta", TaskID: "t2"}
		return engine
	}
	t.Run("空条件返回全部可见历史变量", func(t *testing.T) {
		engine := newEngine(t)
		page, err := engine.QueryHistoricVariableInstances(context.Background(), api.HistoricVariableInstanceFilter{})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Total != 4 || page.Start != 0 || page.Size != 10 {
			t.Fatalf("分页信息错误: %+v", page)
		}
		// 变量名升序，同名按 ID 升序；非默认租户任务变量被隐藏
		assertHistoricVariableOrder(t, page, "v1", "v3", "v2", "v4")
		// 逐字段返回：可见变量的全部字段必须原样保留
		for i, id := range []string{"v1", "v3", "v2", "v4"} {
			if !reflect.DeepEqual(page.Data[i], *engine.state.HistoricVariables[id]) {
				t.Fatalf("历史变量字段错误: %s 期望 %+v 实际 %+v", id, *engine.state.HistoricVariables[id], page.Data[i])
			}
		}
	})
	t.Run("等值字段", func(t *testing.T) {
		cases := []struct {
			name   string                             // 用例名
			filter api.HistoricVariableInstanceFilter // 查询条件
			want   []string                           // 期望结果
		}{
			{name: "变量ID", filter: api.HistoricVariableInstanceFilter{ID: "v2"}, want: []string{"v2"}},
			{name: "变量名", filter: api.HistoricVariableInstanceFilter{Name: "alpha"}, want: []string{"v1", "v3"}},
			{name: "流程实例ID", filter: api.HistoricVariableInstanceFilter{ProcessInstanceID: "p3"}, want: []string{"v3"}},
			{name: "执行ID", filter: api.HistoricVariableInstanceFilter{ExecutionID: "e1"}, want: []string{"v1"}},
			{name: "任务ID", filter: api.HistoricVariableInstanceFilter{TaskID: "t1"}, want: []string{"v4"}},
			{name: "变量类型", filter: api.HistoricVariableInstanceFilter{VariableType: "long"}, want: []string{"v2"}},
			{name: "不存在的变量名", filter: api.HistoricVariableInstanceFilter{Name: "omega"}, want: []string{}},
			// 非默认租户任务变量即使名称匹配也不可见
			{name: "其他租户任务变量", filter: api.HistoricVariableInstanceFilter{Name: "delta"}, want: []string{}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				page, err := newEngine(t).QueryHistoricVariableInstances(context.Background(), tc.filter)
				if err != nil {
					t.Fatalf("内存查询失败: %v", err)
				}
				assertHistoricVariableOrder(t, page, tc.want...)
			})
		}
	})
	t.Run("排序与分页切片", func(t *testing.T) {
		// 每页数量小于剩余条数时按 start+size 截断
		page, err := newEngine(t).QueryHistoricVariableInstances(context.Background(), api.HistoricVariableInstanceFilter{Start: 0, Size: 2})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Total != 4 || page.Start != 0 || page.Size != 2 {
			t.Fatalf("分页信息错误: %+v", page)
		}
		assertHistoricVariableOrder(t, page, "v1", "v3")
		// 偏移 1 起继续按变量名升序返回
		page, err = newEngine(t).QueryHistoricVariableInstances(context.Background(), api.HistoricVariableInstanceFilter{Start: 1, Size: 2})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Total != 4 || page.Start != 1 || page.Size != 2 {
			t.Fatalf("分页信息错误: %+v", page)
		}
		assertHistoricVariableOrder(t, page, "v3", "v2")
	})
	t.Run("偏移越界返回空数据", func(t *testing.T) {
		page, err := newEngine(t).QueryHistoricVariableInstances(context.Background(), api.HistoricVariableInstanceFilter{Start: 9, Size: 2})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Total != 4 || page.Start != 9 || page.Size != 2 || len(page.Data) != 0 {
			t.Fatalf("越界分页信息错误: %+v", page)
		}
		// 极大每页数量与偏移相加不得溢出，偏移 1 起返回到末尾，数量按规范化封顶
		page, err = newEngine(t).QueryHistoricVariableInstances(context.Background(), api.HistoricVariableInstanceFilter{Start: 1, Size: math.MaxInt})
		if err != nil {
			t.Fatalf("极大分页查询失败: %v", err)
		}
		if page.Total != 4 || page.Size != 1000 || len(page.Data) != 3 {
			t.Fatalf("极大分页信息错误: %+v", page)
		}
		assertHistoricVariableOrder(t, page, "v3", "v2", "v4")
		// 超大偏移直接返回空数据，不与每页数量相加
		page, err = newEngine(t).QueryHistoricVariableInstances(context.Background(), api.HistoricVariableInstanceFilter{Start: math.MaxInt, Size: 1})
		if err != nil {
			t.Fatalf("超大偏移查询失败: %v", err)
		}
		if page.Total != 4 || page.Start != math.MaxInt || len(page.Data) != 0 {
			t.Fatalf("超大偏移分页信息错误: %+v", page)
		}
	})
	t.Run("分页参数规范化", func(t *testing.T) {
		// 负偏移归 0，非正每页数量取默认 10
		page, err := newEngine(t).QueryHistoricVariableInstances(context.Background(), api.HistoricVariableInstanceFilter{Start: -2, Size: -1})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Start != 0 || page.Size != 10 || page.Total != 4 {
			t.Fatalf("规范化分页信息错误: %+v", page)
		}
		assertHistoricVariableOrder(t, page, "v1", "v3", "v2", "v4")
		// 每页数量封顶 1000
		page, err = newEngine(t).QueryHistoricVariableInstances(context.Background(), api.HistoricVariableInstanceFilter{Start: 0, Size: 1500})
		if err != nil {
			t.Fatalf("内存查询失败: %v", err)
		}
		if page.Size != 1000 {
			t.Fatalf("每页数量应封顶 1000: %+v", page)
		}
	})
}
