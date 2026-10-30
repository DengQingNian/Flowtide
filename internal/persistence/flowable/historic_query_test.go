package flowable

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/DengQingNian/Flowtide/api"
)

// newHistoricQueryRepository 创建基于 sqlmock 的历史实例查询仓储。
// @param t 测试上下文
// @param dialect 数据库方言
// @return 仓储
// @return SQL 断言器
func newHistoricQueryRepository(t *testing.T, dialect string) (*Repository, sqlmock.Sqlmock) {
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

// expectHistoricQuery 断言一次历史实例计数查询和一次分页 ID 查询。
// @param mock SQL 断言器
// @param countQuery 计数 SQL
// @param countArgs 计数参数
// @param total 返回总数
// @param selectQuery ID 查询 SQL
// @param selectArgs ID 查询参数
// @param ids 返回的历史实例 ID
func expectHistoricQuery(mock sqlmock.Sqlmock, countQuery string, countArgs []driver.Value, total int64, selectQuery string, selectArgs []driver.Value, ids ...string) {
	mock.ExpectQuery(countQuery).WithArgs(countArgs...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(total))
	idRows := sqlmock.NewRows([]string{"PROC_INST_ID_"})
	for _, id := range ids {
		idRows.AddRow(id)
	}
	mock.ExpectQuery(selectQuery).WithArgs(selectArgs...).WillReturnRows(idRows)
}

// requireHistoricQueryError 断言错误为指定错误码与中文消息的 api.Error，且不包含 SQL 文本。
// @param t 测试上下文
// @param err 错误
// @param code 期望错误码
func requireHistoricQueryError(t *testing.T, err error, code string) {
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
	case "ROW-0045":
		message = "数据库不支持查询"
	case "ROW-0046":
		message = "统计历史实例数量失败"
	case "ROW-0047":
		message = "查询历史实例 ID 失败"
	case "ROW-0048":
		message = "读取历史实例 ID 失败"
	case "ROW-0049":
		message = "遍历历史实例 ID 失败"
	default:
		t.Fatalf("未知错误码: %s", code)
	}
	if engineErr.Message != message {
		t.Fatalf("错误消息不正确: %s", engineErr.Message)
	}
	if strings.Contains(engineErr.Message, "SELECT") || strings.Contains(engineErr.Message, "ACT_HI_PROCINST") {
		t.Fatalf("错误信息泄漏 SQL: %s", engineErr.Message)
	}
}

// TestQueryHistoricProcessInstanceIDs_NoFilter 验证无过滤条件时无 WHERE 且使用默认分页。
// @param t 测试上下文
func TestQueryHistoricProcessInstanceIDs_NoFilter(t *testing.T) {
	repository, mock := newHistoricQueryRepository(t, "mysql")
	expectHistoricQuery(mock,
		"SELECT COUNT(*) FROM ACT_HI_PROCINST H", nil, 2,
		"SELECT H.PROC_INST_ID_ FROM ACT_HI_PROCINST H ORDER BY H.START_TIME_ ASC, H.PROC_INST_ID_ ASC LIMIT ? OFFSET ?",
		[]driver.Value{int64(10), int64(0)}, "h1", "h2")
	ids, total, err := repository.QueryHistoricProcessInstanceIDs(context.Background(), HistoricProcessInstanceQuery{})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if total != 2 || len(ids) != 2 || ids[0] != "h1" || ids[1] != "h2" {
		t.Fatalf("结果错误: %v %d", ids, total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryHistoricProcessInstanceIDs_ColumnFilters 验证各直接列过滤条件的参数绑定。
// @param t 测试上下文
func TestQueryHistoricProcessInstanceIDs_ColumnFilters(t *testing.T) {
	cases := []struct {
		name  string                       // 用例名
		where string                       // WHERE 条件
		query HistoricProcessInstanceQuery // 查询条件
		args  []driver.Value               // 参数
	}{
		{"实例ID", "H.PROC_INST_ID_ = ?", HistoricProcessInstanceQuery{ID: "h1"}, []driver.Value{"h1"}},
		{"流程定义ID", "H.PROC_DEF_ID_ = ?", HistoricProcessInstanceQuery{ProcessDefinitionID: "pd1"}, []driver.Value{"pd1"}},
		{"业务键", "H.BUSINESS_KEY_ = ?", HistoricProcessInstanceQuery{BusinessKey: "bk1"}, []driver.Value{"bk1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newHistoricQueryRepository(t, "mysql")
			expectHistoricQuery(mock,
				"SELECT COUNT(*) FROM ACT_HI_PROCINST H WHERE "+tc.where, tc.args, 1,
				"SELECT H.PROC_INST_ID_ FROM ACT_HI_PROCINST H WHERE "+tc.where+" ORDER BY H.START_TIME_ ASC, H.PROC_INST_ID_ ASC LIMIT ? OFFSET ?",
				append(append([]driver.Value{}, tc.args...), int64(10), int64(0)), "h1")
			ids, total, err := repository.QueryHistoricProcessInstanceIDs(context.Background(), tc.query)
			if err != nil || total != 1 || len(ids) != 1 || ids[0] != "h1" {
				t.Fatalf("查询错误: %v %v %d", ids, err, total)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricProcessInstanceIDs_Finished 验证结束状态过滤的两种取值。
// @param t 测试上下文
func TestQueryHistoricProcessInstanceIDs_Finished(t *testing.T) {
	cases := []struct {
		name     string // 用例名
		finished bool   // 结束状态取值
		cond     string // 期望 END_TIME_ 条件
	}{
		{"已结束", true, "H.END_TIME_ IS NOT NULL"},
		{"未结束", false, "H.END_TIME_ IS NULL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newHistoricQueryRepository(t, "mysql")
			where := " WHERE " + tc.cond
			expectHistoricQuery(mock,
				"SELECT COUNT(*) FROM ACT_HI_PROCINST H"+where, nil, 1,
				"SELECT H.PROC_INST_ID_ FROM ACT_HI_PROCINST H"+where+" ORDER BY H.START_TIME_ ASC, H.PROC_INST_ID_ ASC LIMIT ? OFFSET ?",
				[]driver.Value{int64(10), int64(0)}, "h1")
			ids, total, err := repository.QueryHistoricProcessInstanceIDs(context.Background(), HistoricProcessInstanceQuery{Finished: ptrBool(tc.finished)})
			if err != nil || total != 1 || len(ids) != 1 || ids[0] != "h1" {
				t.Fatalf("查询错误: %v %v %d", ids, err, total)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricProcessInstanceIDs_ProcessDefinitionKey 验证流程定义 Key 的 JOIN 下推及参数顺序。
// @param t 测试上下文
func TestQueryHistoricProcessInstanceIDs_ProcessDefinitionKey(t *testing.T) {
	repository, mock := newHistoricQueryRepository(t, "mysql")
	from := "ACT_HI_PROCINST H INNER JOIN ACT_RE_PROCDEF P ON P.ID_ = H.PROC_DEF_ID_ AND P.KEY_ = ?"
	where := " WHERE H.PROC_INST_ID_ = ?"
	expectHistoricQuery(mock,
		"SELECT COUNT(*) FROM "+from+where, []driver.Value{"demo", "h1"}, 1,
		"SELECT H.PROC_INST_ID_ FROM "+from+where+" ORDER BY H.START_TIME_ ASC, H.PROC_INST_ID_ ASC LIMIT ? OFFSET ?",
		[]driver.Value{"demo", "h1", int64(10), int64(0)}, "h1")
	ids, total, err := repository.QueryHistoricProcessInstanceIDs(context.Background(), HistoricProcessInstanceQuery{ProcessDefinitionKey: "demo", ID: "h1"})
	if err != nil || total != 1 || len(ids) != 1 || ids[0] != "h1" {
		t.Fatalf("查询错误: %v %v %d", ids, err, total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryHistoricProcessInstanceIDs_TimeRange 验证开始与结束时间上下界的参数化。
// @param t 测试上下文
func TestQueryHistoricProcessInstanceIDs_TimeRange(t *testing.T) {
	after := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	before := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	cases := []struct {
		name  string                       // 用例名
		query HistoricProcessInstanceQuery // 查询条件
		op    string                       // 期望比较运算符
		arg   time.Time                    // 期望时间参数
	}{
		{"开始下界", HistoricProcessInstanceQuery{StartedAfter: &after}, "H.START_TIME_ >= ?", after},
		{"开始上界", HistoricProcessInstanceQuery{StartedBefore: &before}, "H.START_TIME_ <= ?", before},
		{"结束下界", HistoricProcessInstanceQuery{FinishedAfter: &after}, "H.END_TIME_ >= ?", after},
		{"结束上界", HistoricProcessInstanceQuery{FinishedBefore: &before}, "H.END_TIME_ <= ?", before},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newHistoricQueryRepository(t, "mysql")
			where := " WHERE " + tc.op
			expectHistoricQuery(mock,
				"SELECT COUNT(*) FROM ACT_HI_PROCINST H"+where, []driver.Value{tc.arg}, 1,
				"SELECT H.PROC_INST_ID_ FROM ACT_HI_PROCINST H"+where+" ORDER BY H.START_TIME_ ASC, H.PROC_INST_ID_ ASC LIMIT ? OFFSET ?",
				[]driver.Value{tc.arg, int64(10), int64(0)}, "h1")
			ids, total, err := repository.QueryHistoricProcessInstanceIDs(context.Background(), tc.query)
			if err != nil || total != 1 || len(ids) != 1 || ids[0] != "h1" {
				t.Fatalf("查询错误: %v %v %d", ids, err, total)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricProcessInstanceIDs_Pagination 验证分页参数规范化。
// @param t 测试上下文
func TestQueryHistoricProcessInstanceIDs_Pagination(t *testing.T) {
	cases := []struct {
		name   string // 用例名
		start  int    // 输入偏移
		size   int    // 输入每页数量
		limit  int64  // 期望 LIMIT
		offset int64  // 期望 OFFSET
	}{
		{"默认分页", -5, 0, 10, 0},
		{"上限保护", 0, 2000, 1000, 0},
		{"显式分页", 2, 3, 3, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newHistoricQueryRepository(t, "mysql")
			expectHistoricQuery(mock,
				"SELECT COUNT(*) FROM ACT_HI_PROCINST H", nil, 0,
				"SELECT H.PROC_INST_ID_ FROM ACT_HI_PROCINST H ORDER BY H.START_TIME_ ASC, H.PROC_INST_ID_ ASC LIMIT ? OFFSET ?",
				[]driver.Value{tc.limit, tc.offset})
			ids, total, err := repository.QueryHistoricProcessInstanceIDs(context.Background(), HistoricProcessInstanceQuery{Start: tc.start, Size: tc.size})
			if err != nil || total != 0 || len(ids) != 0 {
				t.Fatalf("查询错误: %v %v %d", ids, err, total)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricProcessInstanceIDs_PostgresPlaceholders 验证 PostgreSQL 占位符编号。
// @param t 测试上下文
func TestQueryHistoricProcessInstanceIDs_PostgresPlaceholders(t *testing.T) {
	repository, mock := newHistoricQueryRepository(t, "postgres")
	startedAfter := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	finishedBefore := time.Date(2026, 4, 5, 6, 7, 8, 0, time.UTC)
	from := "ACT_HI_PROCINST H INNER JOIN ACT_RE_PROCDEF P ON P.ID_ = H.PROC_DEF_ID_ AND P.KEY_ = $1"
	where := " WHERE H.PROC_INST_ID_ = $2 AND H.END_TIME_ IS NOT NULL AND H.START_TIME_ >= $3 AND H.END_TIME_ <= $4"
	expectHistoricQuery(mock,
		"SELECT COUNT(*) FROM "+from+where, []driver.Value{"demo", "h1", startedAfter, finishedBefore}, 1,
		"SELECT H.PROC_INST_ID_ FROM "+from+where+" ORDER BY H.START_TIME_ ASC, H.PROC_INST_ID_ ASC LIMIT $5 OFFSET $6",
		[]driver.Value{"demo", "h1", startedAfter, finishedBefore, int64(5), int64(10)}, "h1")
	ids, total, err := repository.QueryHistoricProcessInstanceIDs(context.Background(), HistoricProcessInstanceQuery{
		ProcessDefinitionKey: "demo",
		ID:                   "h1",
		Finished:             ptrBool(true),
		StartedAfter:         &startedAfter,
		FinishedBefore:       &finishedBefore,
		Start:                10,
		Size:                 5,
	})
	if err != nil || total != 1 || len(ids) != 1 || ids[0] != "h1" {
		t.Fatalf("查询错误: %v %v %d", ids, err, total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryHistoricProcessInstanceIDs_UnsupportedDB 验证数据库不支持查询时的错误。
// @param t 测试上下文
func TestQueryHistoricProcessInstanceIDs_UnsupportedDB(t *testing.T) {
	repository, err := NewRepository(&fakeDB{}, "mysql")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = repository.QueryHistoricProcessInstanceIDs(context.Background(), HistoricProcessInstanceQuery{})
	requireHistoricQueryError(t, err, "ROW-0045")
}

// TestQueryHistoricProcessInstanceIDs_CountErrors 验证计数阶段错误。
// @param t 测试上下文
func TestQueryHistoricProcessInstanceIDs_CountErrors(t *testing.T) {
	countQuery := "SELECT COUNT(*) FROM ACT_HI_PROCINST H"
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
			repository, mock := newHistoricQueryRepository(t, "mysql")
			tc.setup(mock, countQuery)
			_, _, err := repository.QueryHistoricProcessInstanceIDs(context.Background(), HistoricProcessInstanceQuery{})
			requireHistoricQueryError(t, err, "ROW-0046")
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricProcessInstanceIDs_IDListErrors 验证 ID 列表阶段错误。
// @param t 测试上下文
func TestQueryHistoricProcessInstanceIDs_IDListErrors(t *testing.T) {
	selectQuery := "SELECT H.PROC_INST_ID_ FROM ACT_HI_PROCINST H ORDER BY H.START_TIME_ ASC, H.PROC_INST_ID_ ASC LIMIT ? OFFSET ?"
	cases := []struct {
		name  string                     // 用例名
		setup func(mock sqlmock.Sqlmock) // 断言设置
		code  string                     // 期望错误码
	}{
		{"查询失败", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).WillReturnError(errors.New("数据库不可用"))
		}, "ROW-0047"},
		{"扫描失败", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"PROC_INST_ID_"}).AddRow(nil))
		}, "ROW-0048"},
		{"列数不匹配", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"PROC_INST_ID_", "extra"}).AddRow("h1", "x"))
		}, "ROW-0048"},
		{"值类型非法", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"PROC_INST_ID_"}).AddRow(true))
		}, "ROW-0048"},
		{"遍历失败", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"PROC_INST_ID_"}).AddRow("h1").RowError(0, errors.New("遍历")))
		}, "ROW-0049"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newHistoricQueryRepository(t, "mysql")
			mock.ExpectQuery("SELECT COUNT(*) FROM ACT_HI_PROCINST H").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
			tc.setup(mock)
			_, _, err := repository.QueryHistoricProcessInstanceIDs(context.Background(), HistoricProcessInstanceQuery{})
			requireHistoricQueryError(t, err, tc.code)
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricCountIterationError 验证历史实例计数查询在无行且遍历出错时返回错误。
// @param t 测试上下文
func TestQueryHistoricCountIterationError(t *testing.T) {
	_, err := queryHistoricProcessCount(context.Background(), fixedQueryDB{rows: errRows{}}, "SELECT 1", nil)
	requireHistoricQueryError(t, err, "ROW-0046")
}

// TestHistoricQueryValueConversions 验证驱动值到历史实例计数与历史实例 ID 的转换。
// @param t 测试上下文
func TestHistoricQueryValueConversions(t *testing.T) {
	totalCases := []struct {
		value any   // 驱动值
		want  int64 // 期望计数
		ok    bool  // 是否成功
	}{
		{int64(3), 3, true},
		{7, 7, true},
		{"9", 9, true},
		{[]byte("11"), 11, true},
		{true, 0, false},
		{struct{}{}, 0, false},
	}
	for _, tc := range totalCases {
		got, err := historicProcessCountValue(tc.value)
		// 非法类型必须返回错误
		if tc.ok && (err != nil || got != tc.want) {
			t.Fatalf("计数转换错误: %v %v", got, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("非法计数应失败: %v", tc.value)
		}
		if !tc.ok {
			requireHistoricQueryError(t, err, "ROW-0046")
		}
	}
	idCases := []struct {
		value any    // 驱动值
		want  string // 期望 ID
		ok    bool   // 是否成功
	}{
		{"h1", "h1", true},
		{[]byte("h2"), "h2", true},
		{true, "", false},
		{struct{}{}, "", false},
	}
	for _, tc := range idCases {
		got, err := historicProcessIDValue(tc.value)
		// 非法类型必须返回错误
		if tc.ok && (err != nil || got != tc.want) {
			t.Fatalf("ID 转换错误: %v %v", got, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("非法 ID 应失败: %v", tc.value)
		}
		if !tc.ok {
			requireHistoricQueryError(t, err, "ROW-0048")
		}
	}
}
