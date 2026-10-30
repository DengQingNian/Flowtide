package flowable

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/flowable-go/flowable/api"
)

// newHistoricTaskQueryRepository 创建基于 sqlmock 的历史任务查询仓储。
// @param t 测试上下文
// @param dialect 数据库方言
// @return 仓储
// @return SQL 断言器
func newHistoricTaskQueryRepository(t *testing.T, dialect string) (*Repository, sqlmock.Sqlmock) {
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

// expectHistoricTaskQuery 断言一次历史任务计数查询和一次分页 ID 查询。
// @param mock SQL 断言器
// @param countQuery 计数 SQL
// @param countArgs 计数参数
// @param total 返回总数
// @param selectQuery ID 查询 SQL
// @param selectArgs ID 查询参数
// @param ids 返回的历史任务 ID
func expectHistoricTaskQuery(mock sqlmock.Sqlmock, countQuery string, countArgs []driver.Value, total int64, selectQuery string, selectArgs []driver.Value, ids ...string) {
	mock.ExpectQuery(countQuery).WithArgs(countArgs...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(total))
	idRows := sqlmock.NewRows([]string{"ID_"})
	for _, id := range ids {
		idRows.AddRow(id)
	}
	mock.ExpectQuery(selectQuery).WithArgs(selectArgs...).WillReturnRows(idRows)
}

// requireHistoricTaskQueryError 断言错误为指定错误码与中文消息的 api.Error，且不包含 SQL 文本。
// @param t 测试上下文
// @param err 错误
// @param code 期望错误码
func requireHistoricTaskQueryError(t *testing.T, err error, code string) {
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
	case "ROW-0050":
		message = "数据库不支持查询"
	case "ROW-0051":
		message = "统计历史任务数量失败"
	case "ROW-0052":
		message = "查询历史任务 ID 失败"
	case "ROW-0053":
		message = "读取历史任务 ID 失败"
	case "ROW-0054":
		message = "遍历历史任务 ID 失败"
	default:
		t.Fatalf("未知错误码: %s", code)
	}
	if engineErr.Message != message {
		t.Fatalf("错误消息不正确: %s", engineErr.Message)
	}
	if strings.Contains(engineErr.Message, "SELECT") || strings.Contains(engineErr.Message, "ACT_HI_TASKINST") {
		t.Fatalf("错误信息泄漏 SQL: %s", engineErr.Message)
	}
}

// TestQueryHistoricTaskInstanceIDs_NoFilter 验证无过滤条件时无 WHERE 且使用默认分页。
// @param t 测试上下文
func TestQueryHistoricTaskInstanceIDs_NoFilter(t *testing.T) {
	repository, mock := newHistoricTaskQueryRepository(t, "mysql")
	expectHistoricTaskQuery(mock,
		"SELECT COUNT(*) FROM ACT_HI_TASKINST T", nil, 2,
		"SELECT T.ID_ FROM ACT_HI_TASKINST T ORDER BY T.START_TIME_ ASC, T.ID_ ASC LIMIT ? OFFSET ?",
		[]driver.Value{int64(10), int64(0)}, "t1", "t2")
	ids, total, err := repository.QueryHistoricTaskInstanceIDs(context.Background(), HistoricTaskInstanceQuery{})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if total != 2 || len(ids) != 2 || ids[0] != "t1" || ids[1] != "t2" {
		t.Fatalf("结果错误: %v %d", ids, total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryHistoricTaskInstanceIDs_ColumnFilters 验证各等值列过滤条件的参数绑定。
// @param t 测试上下文
func TestQueryHistoricTaskInstanceIDs_ColumnFilters(t *testing.T) {
	cases := []struct {
		name  string                    // 用例名
		where string                    // WHERE 条件
		query HistoricTaskInstanceQuery // 查询条件
		args  []driver.Value            // 参数
	}{
		{"任务ID", "T.ID_ = ?", HistoricTaskInstanceQuery{ID: "t1"}, []driver.Value{"t1"}},
		{"任务名称", "T.NAME_ = ?", HistoricTaskInstanceQuery{Name: "审批"}, []driver.Value{"审批"}},
		{"处理人", "T.ASSIGNEE_ = ?", HistoricTaskInstanceQuery{Assignee: "张三"}, []driver.Value{"张三"}},
		{"任务定义Key", "T.TASK_DEF_KEY_ = ?", HistoricTaskInstanceQuery{TaskDefinitionKey: "tk1"}, []driver.Value{"tk1"}},
		{"流程实例ID", "T.PROC_INST_ID_ = ?", HistoricTaskInstanceQuery{ProcessInstanceID: "pi1"}, []driver.Value{"pi1"}},
		{"流程定义ID", "T.PROC_DEF_ID_ = ?", HistoricTaskInstanceQuery{ProcessDefinitionID: "pd1"}, []driver.Value{"pd1"}},
		{"租户", "T.TENANT_ID_ = ?", HistoricTaskInstanceQuery{TenantID: "acme"}, []driver.Value{"acme"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newHistoricTaskQueryRepository(t, "mysql")
			expectHistoricTaskQuery(mock,
				"SELECT COUNT(*) FROM ACT_HI_TASKINST T WHERE "+tc.where, tc.args, 1,
				"SELECT T.ID_ FROM ACT_HI_TASKINST T WHERE "+tc.where+" ORDER BY T.START_TIME_ ASC, T.ID_ ASC LIMIT ? OFFSET ?",
				append(append([]driver.Value{}, tc.args...), int64(10), int64(0)), "t1")
			ids, total, err := repository.QueryHistoricTaskInstanceIDs(context.Background(), tc.query)
			if err != nil || total != 1 || len(ids) != 1 || ids[0] != "t1" {
				t.Fatalf("查询错误: %v %v %d", ids, err, total)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricTaskInstanceIDs_ProcessDefinitionKey 验证流程定义 Key 的 JOIN 下推及参数顺序。
// @param t 测试上下文
func TestQueryHistoricTaskInstanceIDs_ProcessDefinitionKey(t *testing.T) {
	repository, mock := newHistoricTaskQueryRepository(t, "mysql")
	from := "ACT_HI_TASKINST T INNER JOIN ACT_RE_PROCDEF P ON P.ID_ = T.PROC_DEF_ID_ AND P.KEY_ = ?"
	where := " WHERE T.ID_ = ?"
	expectHistoricTaskQuery(mock,
		"SELECT COUNT(*) FROM "+from+where, []driver.Value{"demo", "t1"}, 1,
		"SELECT T.ID_ FROM "+from+where+" ORDER BY T.START_TIME_ ASC, T.ID_ ASC LIMIT ? OFFSET ?",
		[]driver.Value{"demo", "t1", int64(10), int64(0)}, "t1")
	ids, total, err := repository.QueryHistoricTaskInstanceIDs(context.Background(), HistoricTaskInstanceQuery{ProcessDefinitionKey: "demo", ID: "t1"})
	if err != nil || total != 1 || len(ids) != 1 || ids[0] != "t1" {
		t.Fatalf("查询错误: %v %v %d", ids, err, total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryHistoricTaskInstanceIDs_Finished 验证结束状态过滤的两种取值。
// @param t 测试上下文
func TestQueryHistoricTaskInstanceIDs_Finished(t *testing.T) {
	cases := []struct {
		name     string // 用例名
		finished bool   // 结束状态取值
		cond     string // 期望 END_TIME_ 条件
	}{
		{"已结束", true, "T.END_TIME_ IS NOT NULL"},
		{"未结束", false, "T.END_TIME_ IS NULL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newHistoricTaskQueryRepository(t, "mysql")
			where := " WHERE " + tc.cond
			expectHistoricTaskQuery(mock,
				"SELECT COUNT(*) FROM ACT_HI_TASKINST T"+where, nil, 1,
				"SELECT T.ID_ FROM ACT_HI_TASKINST T"+where+" ORDER BY T.START_TIME_ ASC, T.ID_ ASC LIMIT ? OFFSET ?",
				[]driver.Value{int64(10), int64(0)}, "t1")
			ids, total, err := repository.QueryHistoricTaskInstanceIDs(context.Background(), HistoricTaskInstanceQuery{Finished: ptrBool(tc.finished)})
			if err != nil || total != 1 || len(ids) != 1 || ids[0] != "t1" {
				t.Fatalf("查询错误: %v %v %d", ids, err, total)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricTaskInstanceIDs_TimeRange 验证开始与结束时间上下界的参数化。
// @param t 测试上下文
func TestQueryHistoricTaskInstanceIDs_TimeRange(t *testing.T) {
	after := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	before := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	cases := []struct {
		name  string                    // 用例名
		query HistoricTaskInstanceQuery // 查询条件
		op    string                    // 期望比较运算符
		arg   time.Time                 // 期望时间参数
	}{
		{"开始下界", HistoricTaskInstanceQuery{StartedAfter: &after}, "T.START_TIME_ >= ?", after},
		{"开始上界", HistoricTaskInstanceQuery{StartedBefore: &before}, "T.START_TIME_ <= ?", before},
		{"结束下界", HistoricTaskInstanceQuery{FinishedAfter: &after}, "T.END_TIME_ >= ?", after},
		{"结束上界", HistoricTaskInstanceQuery{FinishedBefore: &before}, "T.END_TIME_ <= ?", before},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newHistoricTaskQueryRepository(t, "mysql")
			where := " WHERE " + tc.op
			expectHistoricTaskQuery(mock,
				"SELECT COUNT(*) FROM ACT_HI_TASKINST T"+where, []driver.Value{tc.arg}, 1,
				"SELECT T.ID_ FROM ACT_HI_TASKINST T"+where+" ORDER BY T.START_TIME_ ASC, T.ID_ ASC LIMIT ? OFFSET ?",
				[]driver.Value{tc.arg, int64(10), int64(0)}, "t1")
			ids, total, err := repository.QueryHistoricTaskInstanceIDs(context.Background(), tc.query)
			if err != nil || total != 1 || len(ids) != 1 || ids[0] != "t1" {
				t.Fatalf("查询错误: %v %v %d", ids, err, total)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricTaskInstanceIDs_Pagination 验证分页参数规范化。
// @param t 测试上下文
func TestQueryHistoricTaskInstanceIDs_Pagination(t *testing.T) {
	cases := []struct {
		name   string // 用例名
		start  int    // 输入偏移
		size   int    // 输入每页数量
		limit  int64  // 期望 LIMIT
		offset int64  // 期望 OFFSET
	}{
		{"默认分页", 0, 0, 10, 0},
		{"负偏移归零", -5, 0, 10, 0},
		{"上限保护", 0, 2000, 1000, 0},
		{"显式分页", 2, 3, 3, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newHistoricTaskQueryRepository(t, "mysql")
			expectHistoricTaskQuery(mock,
				"SELECT COUNT(*) FROM ACT_HI_TASKINST T", nil, 0,
				"SELECT T.ID_ FROM ACT_HI_TASKINST T ORDER BY T.START_TIME_ ASC, T.ID_ ASC LIMIT ? OFFSET ?",
				[]driver.Value{tc.limit, tc.offset})
			ids, total, err := repository.QueryHistoricTaskInstanceIDs(context.Background(), HistoricTaskInstanceQuery{Start: tc.start, Size: tc.size})
			if err != nil || total != 0 || len(ids) != 0 {
				t.Fatalf("查询错误: %v %v %d", ids, err, total)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricTaskInstanceIDs_PostgresPlaceholders 验证 PostgreSQL 占位符编号。
// @param t 测试上下文
func TestQueryHistoricTaskInstanceIDs_PostgresPlaceholders(t *testing.T) {
	repository, mock := newHistoricTaskQueryRepository(t, "postgres")
	startedAfter := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	finishedBefore := time.Date(2026, 4, 5, 6, 7, 8, 0, time.UTC)
	from := "ACT_HI_TASKINST T INNER JOIN ACT_RE_PROCDEF P ON P.ID_ = T.PROC_DEF_ID_ AND P.KEY_ = $1"
	where := " WHERE T.ID_ = $2 AND T.END_TIME_ IS NOT NULL AND T.START_TIME_ >= $3 AND T.END_TIME_ <= $4"
	expectHistoricTaskQuery(mock,
		"SELECT COUNT(*) FROM "+from+where, []driver.Value{"demo", "t1", startedAfter, finishedBefore}, 1,
		"SELECT T.ID_ FROM "+from+where+" ORDER BY T.START_TIME_ ASC, T.ID_ ASC LIMIT $5 OFFSET $6",
		[]driver.Value{"demo", "t1", startedAfter, finishedBefore, int64(5), int64(10)}, "t1")
	ids, total, err := repository.QueryHistoricTaskInstanceIDs(context.Background(), HistoricTaskInstanceQuery{
		ProcessDefinitionKey: "demo",
		ID:                   "t1",
		Finished:             ptrBool(true),
		StartedAfter:         &startedAfter,
		FinishedBefore:       &finishedBefore,
		Start:                10,
		Size:                 5,
	})
	if err != nil || total != 1 || len(ids) != 1 || ids[0] != "t1" {
		t.Fatalf("查询错误: %v %v %d", ids, err, total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryHistoricTaskInstanceIDs_UnsupportedDB 验证数据库不支持查询时的错误。
// @param t 测试上下文
func TestQueryHistoricTaskInstanceIDs_UnsupportedDB(t *testing.T) {
	repository, err := NewRepository(&fakeDB{}, "mysql")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = repository.QueryHistoricTaskInstanceIDs(context.Background(), HistoricTaskInstanceQuery{})
	requireHistoricTaskQueryError(t, err, "ROW-0050")
}

// TestQueryHistoricTaskInstanceIDs_CountErrors 验证计数阶段错误。
// @param t 测试上下文
func TestQueryHistoricTaskInstanceIDs_CountErrors(t *testing.T) {
	countQuery := "SELECT COUNT(*) FROM ACT_HI_TASKINST T"
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
			repository, mock := newHistoricTaskQueryRepository(t, "mysql")
			tc.setup(mock, countQuery)
			_, _, err := repository.QueryHistoricTaskInstanceIDs(context.Background(), HistoricTaskInstanceQuery{})
			requireHistoricTaskQueryError(t, err, "ROW-0051")
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricTaskInstanceIDs_IDListErrors 验证 ID 列表阶段错误。
// @param t 测试上下文
func TestQueryHistoricTaskInstanceIDs_IDListErrors(t *testing.T) {
	selectQuery := "SELECT T.ID_ FROM ACT_HI_TASKINST T ORDER BY T.START_TIME_ ASC, T.ID_ ASC LIMIT ? OFFSET ?"
	cases := []struct {
		name  string                     // 用例名
		setup func(mock sqlmock.Sqlmock) // 断言设置
		code  string                     // 期望错误码
	}{
		{"查询失败", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).WillReturnError(errors.New("数据库不可用"))
		}, "ROW-0052"},
		{"扫描失败", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"ID_"}).AddRow(nil))
		}, "ROW-0053"},
		{"列数不匹配", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"ID_", "extra"}).AddRow("t1", "x"))
		}, "ROW-0053"},
		{"值类型非法", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"ID_"}).AddRow(true))
		}, "ROW-0053"},
		{"遍历失败", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"ID_"}).AddRow("t1").RowError(0, errors.New("遍历")))
		}, "ROW-0054"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newHistoricTaskQueryRepository(t, "mysql")
			mock.ExpectQuery("SELECT COUNT(*) FROM ACT_HI_TASKINST T").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
			tc.setup(mock)
			_, _, err := repository.QueryHistoricTaskInstanceIDs(context.Background(), HistoricTaskInstanceQuery{})
			requireHistoricTaskQueryError(t, err, tc.code)
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricTaskCountIterationError 验证历史任务计数查询在无行且遍历出错时返回错误。
// @param t 测试上下文
func TestQueryHistoricTaskCountIterationError(t *testing.T) {
	_, err := queryHistoricTaskCount(context.Background(), fixedQueryDB{rows: errRows{}}, "SELECT 1", nil)
	requireHistoricTaskQueryError(t, err, "ROW-0051")
}

// TestHistoricTaskQueryValueConversions 验证驱动值到历史任务计数与历史任务 ID 的转换。
// @param t 测试上下文
func TestHistoricTaskQueryValueConversions(t *testing.T) {
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
		got, err := historicTaskCountValue(tc.value)
		// 非法类型必须返回错误
		if tc.ok && (err != nil || got != tc.want) {
			t.Fatalf("计数转换错误: %v %v", got, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("非法计数应失败: %v", tc.value)
		}
		if !tc.ok {
			requireHistoricTaskQueryError(t, err, "ROW-0051")
		}
	}
	idCases := []struct {
		value any    // 驱动值
		want  string // 期望 ID
		ok    bool   // 是否成功
	}{
		{"t1", "t1", true},
		{[]byte("t2"), "t2", true},
		{true, "", false},
		{struct{}{}, "", false},
	}
	for _, tc := range idCases {
		got, err := historicTaskIDValue(tc.value)
		// 非法类型必须返回错误
		if tc.ok && (err != nil || got != tc.want) {
			t.Fatalf("ID 转换错误: %v %v", got, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("非法 ID 应失败: %v", tc.value)
		}
		if !tc.ok {
			requireHistoricTaskQueryError(t, err, "ROW-0053")
		}
	}
}
