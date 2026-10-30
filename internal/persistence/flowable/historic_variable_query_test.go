package flowable

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/flowable-go/flowable/api"
)

// historicVariableFrom 是历史变量查询的 FROM 片段常量，避免各用例重复书写。
const historicVariableFrom = "ACT_HI_VARINST V"

// historicVariableOrderBy 是历史变量 ID 查询的固定排序片段。
const historicVariableOrderBy = " ORDER BY V.NAME_ ASC, V.ID_ ASC"

// newHistoricVariableQueryRepository 创建基于 sqlmock 的历史变量查询仓储。
// @param t 测试上下文
// @param dialect 数据库方言
// @return 仓储
// @return SQL 断言器
func newHistoricVariableQueryRepository(t *testing.T, dialect string) (*Repository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("创建 sqlmock 失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repository, err := NewRepository(SQLDB{DB: db}, dialect)
	if err != nil {
		t.Fatalf("创建仓储失败: %v", err)
	}
	return repository, mock
}

// expectHistoricVariableQuery 断言一次历史变量计数查询和一次分页 ID 查询。
// @param mock SQL 断言器
// @param countQuery 计数 SQL
// @param countArgs 计数参数
// @param total 返回总数
// @param selectQuery ID 查询 SQL
// @param selectArgs ID 查询参数
// @param ids 返回的历史变量 ID
func expectHistoricVariableQuery(mock sqlmock.Sqlmock, countQuery string, countArgs []driver.Value, total int64, selectQuery string, selectArgs []driver.Value, ids ...string) {
	mock.ExpectQuery(countQuery).WithArgs(countArgs...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(total))
	idRows := sqlmock.NewRows([]string{"ID_"})
	for _, id := range ids {
		idRows.AddRow(id)
	}
	mock.ExpectQuery(selectQuery).WithArgs(selectArgs...).WillReturnRows(idRows)
}

// requireHistoricVariableQueryError 断言错误为指定错误码与中文消息的 api.Error，且不包含 SQL 文本。
// @param t 测试上下文
// @param err 错误
// @param code 期望错误码
func requireHistoricVariableQueryError(t *testing.T, err error, code string) {
	t.Helper()
	var engineErr *api.Error
	if !errors.As(err, &engineErr) {
		t.Fatalf("错误类型不正确: %T %v", err, err)
	}
	if engineErr.Code != code {
		t.Fatalf("错误码不正确: %s %v", engineErr.Code, err)
	}
	var message string
	switch code {
	case "ROW-0060":
		message = "数据库不支持查询"
	case "ROW-0061":
		message = "统计历史变量数量失败"
	case "ROW-0062":
		message = "查询历史变量 ID 失败"
	case "ROW-0063":
		message = "读取历史变量 ID 失败"
	case "ROW-0064":
		message = "遍历历史变量 ID 失败"
	default:
		t.Fatalf("未知错误码: %s", code)
	}
	if engineErr.Message != message {
		t.Fatalf("错误消息不正确: %s", engineErr.Message)
	}
	if strings.Contains(engineErr.Message, "SELECT") || strings.Contains(engineErr.Message, "ACT_HI_VARINST") {
		t.Fatalf("错误信息泄漏 SQL: %s", engineErr.Message)
	}
}

// TestQueryHistoricVariableInstanceIDs_NoFilter 验证无过滤条件时无 WHERE 且使用默认分页。
// @param t 测试上下文
func TestQueryHistoricVariableInstanceIDs_NoFilter(t *testing.T) {
	repository, mock := newHistoricVariableQueryRepository(t, "mysql")
	expectHistoricVariableQuery(mock,
		"SELECT COUNT(*) FROM "+historicVariableFrom, nil, 2,
		"SELECT V.ID_ FROM "+historicVariableFrom+historicVariableOrderBy+" LIMIT ? OFFSET ?",
		[]driver.Value{int64(10), int64(0)}, "v1", "v2")
	ids, total, err := repository.QueryHistoricVariableInstanceIDs(context.Background(), HistoricVariableInstanceQuery{})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if total != 2 || len(ids) != 2 || ids[0] != "v1" || ids[1] != "v2" {
		t.Fatalf("结果错误: %v %d", ids, total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryHistoricVariableInstanceIDs_ColumnFilters 验证各等值列过滤条件的参数绑定。
// @param t 测试上下文
func TestQueryHistoricVariableInstanceIDs_ColumnFilters(t *testing.T) {
	cases := []struct {
		name  string                        // 用例名
		where string                        // WHERE 条件
		query HistoricVariableInstanceQuery // 查询条件
		args  []driver.Value                // 参数
	}{
		{"变量ID", "V.ID_ = ?", HistoricVariableInstanceQuery{ID: "v1"}, []driver.Value{"v1"}},
		{"变量名", "V.NAME_ = ?", HistoricVariableInstanceQuery{Name: "金额"}, []driver.Value{"金额"}},
		{"流程实例ID", "V.PROC_INST_ID_ = ?", HistoricVariableInstanceQuery{ProcessInstanceID: "pi1"}, []driver.Value{"pi1"}},
		{"执行ID", "V.EXECUTION_ID_ = ?", HistoricVariableInstanceQuery{ExecutionID: "ex1"}, []driver.Value{"ex1"}},
		{"任务ID", "V.TASK_ID_ = ?", HistoricVariableInstanceQuery{TaskID: "tk1"}, []driver.Value{"tk1"}},
		{"变量类型", "V.VAR_TYPE_ = ?", HistoricVariableInstanceQuery{VariableType: "string"}, []driver.Value{"string"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newHistoricVariableQueryRepository(t, "mysql")
			expectHistoricVariableQuery(mock,
				"SELECT COUNT(*) FROM "+historicVariableFrom+" WHERE "+tc.where, tc.args, 1,
				"SELECT V.ID_ FROM "+historicVariableFrom+" WHERE "+tc.where+historicVariableOrderBy+" LIMIT ? OFFSET ?",
				append(append([]driver.Value{}, tc.args...), int64(10), int64(0)), "v1")
			ids, total, err := repository.QueryHistoricVariableInstanceIDs(context.Background(), tc.query)
			if err != nil || total != 1 || len(ids) != 1 || ids[0] != "v1" {
				t.Fatalf("查询错误: %v %v %d", ids, err, total)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricVariableInstanceIDs_AllFilters 验证全部过滤条件同时生效时的拼接顺序与参数个数。
// @param t 测试上下文
func TestQueryHistoricVariableInstanceIDs_AllFilters(t *testing.T) {
	repository, mock := newHistoricVariableQueryRepository(t, "mysql")
	where := " WHERE V.ID_ = ? AND V.NAME_ = ? AND V.PROC_INST_ID_ = ? AND V.EXECUTION_ID_ = ?" +
		" AND V.TASK_ID_ = ? AND V.VAR_TYPE_ = ?"
	args := []driver.Value{"v1", "金额", "pi1", "ex1", "tk1", "string"}
	expectHistoricVariableQuery(mock,
		"SELECT COUNT(*) FROM "+historicVariableFrom+where, args, 1,
		"SELECT V.ID_ FROM "+historicVariableFrom+where+historicVariableOrderBy+" LIMIT ? OFFSET ?",
		append(append([]driver.Value{}, args...), int64(5), int64(2)), "v1")
	ids, total, err := repository.QueryHistoricVariableInstanceIDs(context.Background(), HistoricVariableInstanceQuery{
		ID:                "v1",
		Name:              "金额",
		ProcessInstanceID: "pi1",
		ExecutionID:       "ex1",
		TaskID:            "tk1",
		VariableType:      "string",
		Start:             2,
		Size:              5,
	})
	if err != nil || total != 1 || len(ids) != 1 || ids[0] != "v1" {
		t.Fatalf("查询错误: %v %v %d", ids, err, total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryHistoricVariableInstanceIDs_Pagination 验证分页参数规范化。
// @param t 测试上下文
func TestQueryHistoricVariableInstanceIDs_Pagination(t *testing.T) {
	cases := []struct {
		name   string // 用例名
		start  int    // 输入偏移
		size   int    // 输入每页数量
		limit  int64  // 期望 LIMIT
		offset int64  // 期望 OFFSET
	}{
		{"默认分页", 0, 0, 10, 0},
		{"负偏移归零", -5, 0, 10, 0},
		{"负数量取默认", 0, -1, 10, 0},
		{"上限保护", 0, 2000, 1000, 0},
		{"显式分页", 3, 7, 7, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newHistoricVariableQueryRepository(t, "mysql")
			expectHistoricVariableQuery(mock,
				"SELECT COUNT(*) FROM "+historicVariableFrom, nil, 0,
				"SELECT V.ID_ FROM "+historicVariableFrom+historicVariableOrderBy+" LIMIT ? OFFSET ?",
				[]driver.Value{tc.limit, tc.offset})
			ids, total, err := repository.QueryHistoricVariableInstanceIDs(context.Background(), HistoricVariableInstanceQuery{Start: tc.start, Size: tc.size})
			if err != nil || total != 0 || len(ids) != 0 {
				t.Fatalf("查询错误: %v %v %d", ids, err, total)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricVariableInstanceIDs_PostgresPlaceholders 验证 PostgreSQL 占位符编号。
// @param t 测试上下文
func TestQueryHistoricVariableInstanceIDs_PostgresPlaceholders(t *testing.T) {
	repository, mock := newHistoricVariableQueryRepository(t, "postgres")
	where := " WHERE V.ID_ = $1 AND V.NAME_ = $2 AND V.PROC_INST_ID_ = $3 AND V.VAR_TYPE_ = $4"
	expectHistoricVariableQuery(mock,
		"SELECT COUNT(*) FROM "+historicVariableFrom+where,
		[]driver.Value{"v1", "金额", "pi1", "string"}, 1,
		"SELECT V.ID_ FROM "+historicVariableFrom+where+historicVariableOrderBy+" LIMIT $5 OFFSET $6",
		[]driver.Value{"v1", "金额", "pi1", "string", int64(5), int64(10)}, "v1")
	ids, total, err := repository.QueryHistoricVariableInstanceIDs(context.Background(), HistoricVariableInstanceQuery{
		ID:                "v1",
		Name:              "金额",
		ProcessInstanceID: "pi1",
		VariableType:      "string",
		Start:             10,
		Size:              5,
	})
	if err != nil || total != 1 || len(ids) != 1 || ids[0] != "v1" {
		t.Fatalf("查询错误: %v %v %d", ids, err, total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryHistoricVariableInstanceIDs_UnsupportedDB 验证数据库不支持查询时的错误。
// @param t 测试上下文
func TestQueryHistoricVariableInstanceIDs_UnsupportedDB(t *testing.T) {
	repository, err := NewRepository(&fakeDB{}, "mysql")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = repository.QueryHistoricVariableInstanceIDs(context.Background(), HistoricVariableInstanceQuery{})
	requireHistoricVariableQueryError(t, err, "ROW-0060")
}

// TestQueryHistoricVariableInstanceIDs_CountErrors 验证计数阶段错误。
// @param t 测试上下文
func TestQueryHistoricVariableInstanceIDs_CountErrors(t *testing.T) {
	countQuery := "SELECT COUNT(*) FROM " + historicVariableFrom
	cases := []struct {
		name  string                                   // 用例名
		setup func(mock sqlmock.Sqlmock, query string) // 断言设置
	}{
		{"查询失败", func(mock sqlmock.Sqlmock, query string) {
			mock.ExpectQuery(query).WillReturnError(errors.New("数据库不可用"))
		}},
		{"空结果", func(mock sqlmock.Sqlmock, query string) {
			mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"count"}))
		}},
		{"扫描失败", func(mock sqlmock.Sqlmock, query string) {
			mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"count", "extra"}).AddRow(int64(1), int64(2)))
		}},
		{"值类型非法", func(mock sqlmock.Sqlmock, query string) {
			mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(true))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newHistoricVariableQueryRepository(t, "mysql")
			tc.setup(mock, countQuery)
			_, _, err := repository.QueryHistoricVariableInstanceIDs(context.Background(), HistoricVariableInstanceQuery{})
			requireHistoricVariableQueryError(t, err, "ROW-0061")
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricVariableInstanceIDs_IDListErrors 验证 ID 列表阶段错误。
// @param t 测试上下文
func TestQueryHistoricVariableInstanceIDs_IDListErrors(t *testing.T) {
	selectQuery := "SELECT V.ID_ FROM " + historicVariableFrom + historicVariableOrderBy + " LIMIT ? OFFSET ?"
	cases := []struct {
		name  string                     // 用例名
		setup func(mock sqlmock.Sqlmock) // 断言设置
		code  string                     // 期望错误码
	}{
		{"查询失败", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).WillReturnError(errors.New("数据库不可用"))
		}, "ROW-0062"},
		{"扫描失败", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"ID_", "extra"}).AddRow("v1", "x"))
		}, "ROW-0063"},
		{"值类型非法", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"ID_"}).AddRow(true))
		}, "ROW-0063"},
		{"遍历失败", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"ID_"}).AddRow("v1").RowError(0, errors.New("遍历")))
		}, "ROW-0064"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newHistoricVariableQueryRepository(t, "mysql")
			mock.ExpectQuery("SELECT COUNT(*) FROM " + historicVariableFrom).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
			tc.setup(mock)
			_, _, err := repository.QueryHistoricVariableInstanceIDs(context.Background(), HistoricVariableInstanceQuery{})
			requireHistoricVariableQueryError(t, err, tc.code)
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricVariableCountIterationError 验证历史变量计数查询在无行且遍历出错时返回错误。
// @param t 测试上下文
func TestQueryHistoricVariableCountIterationError(t *testing.T) {
	_, err := queryHistoricVariableCount(context.Background(), fixedQueryDB{rows: errRows{}}, "SELECT 1", nil)
	requireHistoricVariableQueryError(t, err, "ROW-0061")
}

// TestQueryHistoricVariableIDListIterationError 验证历史变量 ID 查询在结果集遍历出错时返回错误。
// @param t 测试上下文
func TestQueryHistoricVariableIDListIterationError(t *testing.T) {
	_, err := queryHistoricVariableIDList(context.Background(), fixedQueryDB{rows: errRows{}}, "SELECT 1", nil)
	requireHistoricVariableQueryError(t, err, "ROW-0064")
}

// TestHistoricVariableQueryValueConversions 验证驱动值到历史变量计数与历史变量 ID 的转换。
// @param t 测试上下文
func TestHistoricVariableQueryValueConversions(t *testing.T) {
	totalCases := []struct {
		value  any   // 驱动值
		want   int64 // 期望计数
		ok     bool  // 是否成功
		engine bool  // 失败时是否为带错误码的引擎错误（字符串解析失败为底层错误）
	}{
		{int64(4), 4, true, false},
		{8, 8, true, false},
		{"12", 12, true, false},
		{[]byte("13"), 13, true, false},
		{"abc", 0, false, false},
		{[]byte("y"), 0, false, false},
		{true, 0, false, true},
		{struct{}{}, 0, false, true},
	}
	for _, tc := range totalCases {
		got, err := historicVariableCountValue(tc.value)
		// 合法类型必须转换成功，非法类型必须返回错误
		if tc.ok && (err != nil || got != tc.want) {
			t.Fatalf("计数转换错误: %v %v", got, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("非法计数应失败: %v", tc.value)
		}
		if !tc.ok && tc.engine {
			requireHistoricVariableQueryError(t, err, "ROW-0061")
		}
		if !tc.ok && !tc.engine {
			var engineErr *api.Error
			if errors.As(err, &engineErr) {
				t.Fatalf("字符串解析失败不应为引擎错误: %v", err)
			}
		}
	}
	idCases := []struct {
		value any    // 驱动值
		want  string // 期望 ID
		ok    bool   // 是否成功
	}{
		{"v1", "v1", true},
		{[]byte("v2"), "v2", true},
		{42, "", false},
		{true, "", false},
		{struct{}{}, "", false},
	}
	for _, tc := range idCases {
		got, err := historicVariableIDValue(tc.value)
		// 合法类型必须转换成功，非法类型必须返回错误
		if tc.ok && (err != nil || got != tc.want) {
			t.Fatalf("ID 转换错误: %v %v", got, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("非法 ID 应失败: %v", tc.value)
		}
		if !tc.ok {
			requireHistoricVariableQueryError(t, err, "ROW-0063")
		}
	}
}
