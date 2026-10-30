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

// historicActivityFrom 是历史活动查询的 FROM 片段常量，避免各用例重复书写。
const historicActivityFrom = "ACT_HI_ACTINST A"

// historicActivityOrderBy 是历史活动 ID 查询的固定排序片段。
const historicActivityOrderBy = " ORDER BY A.START_TIME_ ASC, A.ID_ ASC"

// newHistoricActivityQueryRepository 创建基于 sqlmock 的历史活动查询仓储。
// @param t 测试上下文
// @param dialect 数据库方言
// @return 仓储
// @return SQL 断言器
func newHistoricActivityQueryRepository(t *testing.T, dialect string) (*Repository, sqlmock.Sqlmock) {
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

// expectHistoricActivityQuery 断言一次历史活动计数查询和一次分页 ID 查询。
// @param mock SQL 断言器
// @param countQuery 计数 SQL
// @param countArgs 计数参数
// @param total 返回总数
// @param selectQuery ID 查询 SQL
// @param selectArgs ID 查询参数
// @param ids 返回的历史活动 ID
func expectHistoricActivityQuery(mock sqlmock.Sqlmock, countQuery string, countArgs []driver.Value, total int64, selectQuery string, selectArgs []driver.Value, ids ...string) {
	mock.ExpectQuery(countQuery).WithArgs(countArgs...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(total))
	idRows := sqlmock.NewRows([]string{"ID_"})
	for _, id := range ids {
		idRows.AddRow(id)
	}
	mock.ExpectQuery(selectQuery).WithArgs(selectArgs...).WillReturnRows(idRows)
}

// requireHistoricActivityQueryError 断言错误为指定错误码与中文消息的 api.Error，且不包含 SQL 文本。
// @param t 测试上下文
// @param err 错误
// @param code 期望错误码
func requireHistoricActivityQueryError(t *testing.T, err error, code string) {
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
	case "ROW-0055":
		message = "数据库不支持查询"
	case "ROW-0056":
		message = "统计历史活动数量失败"
	case "ROW-0057":
		message = "查询历史活动 ID 失败"
	case "ROW-0058":
		message = "读取历史活动 ID 失败"
	case "ROW-0059":
		message = "遍历历史活动 ID 失败"
	default:
		t.Fatalf("未知错误码: %s", code)
	}
	if engineErr.Message != message {
		t.Fatalf("错误消息不正确: %s", engineErr.Message)
	}
	if strings.Contains(engineErr.Message, "SELECT") || strings.Contains(engineErr.Message, "ACT_HI_ACTINST") {
		t.Fatalf("错误信息泄漏 SQL: %s", engineErr.Message)
	}
}

// TestQueryHistoricActivityInstanceIDs_NoFilter 验证无过滤条件时无 WHERE 且使用默认分页。
// @param t 测试上下文
func TestQueryHistoricActivityInstanceIDs_NoFilter(t *testing.T) {
	repository, mock := newHistoricActivityQueryRepository(t, "mysql")
	expectHistoricActivityQuery(mock,
		"SELECT COUNT(*) FROM "+historicActivityFrom, nil, 2,
		"SELECT A.ID_ FROM "+historicActivityFrom+historicActivityOrderBy+" LIMIT ? OFFSET ?",
		[]driver.Value{int64(10), int64(0)}, "a1", "a2")
	ids, total, err := repository.QueryHistoricActivityInstanceIDs(context.Background(), HistoricActivityInstanceQuery{})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if total != 2 || len(ids) != 2 || ids[0] != "a1" || ids[1] != "a2" {
		t.Fatalf("结果错误: %v %d", ids, total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryHistoricActivityInstanceIDs_ColumnFilters 验证各等值列过滤条件的参数绑定。
// @param t 测试上下文
func TestQueryHistoricActivityInstanceIDs_ColumnFilters(t *testing.T) {
	cases := []struct {
		name  string                        // 用例名
		where string                        // WHERE 条件
		query HistoricActivityInstanceQuery // 查询条件
		args  []driver.Value                // 参数
	}{
		{"活动ID", "A.ID_ = ?", HistoricActivityInstanceQuery{ID: "a1"}, []driver.Value{"a1"}},
		{"流程实例ID", "A.PROC_INST_ID_ = ?", HistoricActivityInstanceQuery{ProcessInstanceID: "pi1"}, []driver.Value{"pi1"}},
		{"流程定义ID", "A.PROC_DEF_ID_ = ?", HistoricActivityInstanceQuery{ProcessDefinitionID: "pd1"}, []driver.Value{"pd1"}},
		{"活动定义ID", "A.ACT_ID_ = ?", HistoricActivityInstanceQuery{ActivityID: "act1"}, []driver.Value{"act1"}},
		{"活动名称", "A.ACT_NAME_ = ?", HistoricActivityInstanceQuery{ActivityName: "审批"}, []driver.Value{"审批"}},
		{"活动类型", "A.ACT_TYPE_ = ?", HistoricActivityInstanceQuery{ActivityType: "userTask"}, []driver.Value{"userTask"}},
		{"处理人", "A.ASSIGNEE_ = ?", HistoricActivityInstanceQuery{Assignee: "张三"}, []driver.Value{"张三"}},
		{"租户", "A.TENANT_ID_ = ?", HistoricActivityInstanceQuery{TenantID: "acme"}, []driver.Value{"acme"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newHistoricActivityQueryRepository(t, "mysql")
			expectHistoricActivityQuery(mock,
				"SELECT COUNT(*) FROM "+historicActivityFrom+" WHERE "+tc.where, tc.args, 1,
				"SELECT A.ID_ FROM "+historicActivityFrom+" WHERE "+tc.where+historicActivityOrderBy+" LIMIT ? OFFSET ?",
				append(append([]driver.Value{}, tc.args...), int64(10), int64(0)), "a1")
			ids, total, err := repository.QueryHistoricActivityInstanceIDs(context.Background(), tc.query)
			if err != nil || total != 1 || len(ids) != 1 || ids[0] != "a1" {
				t.Fatalf("查询错误: %v %v %d", ids, err, total)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricActivityInstanceIDs_ProcessDefinitionKey 验证流程定义 Key 的 JOIN 下推及参数顺序。
// @param t 测试上下文
func TestQueryHistoricActivityInstanceIDs_ProcessDefinitionKey(t *testing.T) {
	repository, mock := newHistoricActivityQueryRepository(t, "mysql")
	from := historicActivityFrom + " INNER JOIN ACT_RE_PROCDEF P ON P.ID_ = A.PROC_DEF_ID_ AND P.KEY_ = ?"
	where := " WHERE A.ID_ = ?"
	expectHistoricActivityQuery(mock,
		"SELECT COUNT(*) FROM "+from+where, []driver.Value{"demo", "a1"}, 1,
		"SELECT A.ID_ FROM "+from+where+historicActivityOrderBy+" LIMIT ? OFFSET ?",
		[]driver.Value{"demo", "a1", int64(10), int64(0)}, "a1")
	ids, total, err := repository.QueryHistoricActivityInstanceIDs(context.Background(), HistoricActivityInstanceQuery{ProcessDefinitionKey: "demo", ID: "a1"})
	if err != nil || total != 1 || len(ids) != 1 || ids[0] != "a1" {
		t.Fatalf("查询错误: %v %v %d", ids, err, total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryHistoricActivityInstanceIDs_Finished 验证结束状态过滤的两种取值。
// @param t 测试上下文
func TestQueryHistoricActivityInstanceIDs_Finished(t *testing.T) {
	cases := []struct {
		name     string // 用例名
		finished bool   // 结束状态取值
		cond     string // 期望 END_TIME_ 条件
	}{
		{"已结束", true, "A.END_TIME_ IS NOT NULL"},
		{"未结束", false, "A.END_TIME_ IS NULL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newHistoricActivityQueryRepository(t, "mysql")
			where := " WHERE " + tc.cond
			expectHistoricActivityQuery(mock,
				"SELECT COUNT(*) FROM "+historicActivityFrom+where, nil, 1,
				"SELECT A.ID_ FROM "+historicActivityFrom+where+historicActivityOrderBy+" LIMIT ? OFFSET ?",
				[]driver.Value{int64(10), int64(0)}, "a1")
			ids, total, err := repository.QueryHistoricActivityInstanceIDs(context.Background(), HistoricActivityInstanceQuery{Finished: ptrBool(tc.finished)})
			if err != nil || total != 1 || len(ids) != 1 || ids[0] != "a1" {
				t.Fatalf("查询错误: %v %v %d", ids, err, total)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricActivityInstanceIDs_TimeRange 验证开始与结束时间四类边界的参数化。
// @param t 测试上下文
func TestQueryHistoricActivityInstanceIDs_TimeRange(t *testing.T) {
	after := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	before := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	cases := []struct {
		name  string                        // 用例名
		query HistoricActivityInstanceQuery // 查询条件
		op    string                        // 期望比较运算符
		arg   time.Time                     // 期望时间参数
	}{
		{"开始下界", HistoricActivityInstanceQuery{StartedAfter: &after}, "A.START_TIME_ >= ?", after},
		{"开始上界", HistoricActivityInstanceQuery{StartedBefore: &before}, "A.START_TIME_ <= ?", before},
		{"结束下界", HistoricActivityInstanceQuery{FinishedAfter: &after}, "A.END_TIME_ >= ?", after},
		{"结束上界", HistoricActivityInstanceQuery{FinishedBefore: &before}, "A.END_TIME_ <= ?", before},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newHistoricActivityQueryRepository(t, "mysql")
			where := " WHERE " + tc.op
			expectHistoricActivityQuery(mock,
				"SELECT COUNT(*) FROM "+historicActivityFrom+where, []driver.Value{tc.arg}, 1,
				"SELECT A.ID_ FROM "+historicActivityFrom+where+historicActivityOrderBy+" LIMIT ? OFFSET ?",
				[]driver.Value{tc.arg, int64(10), int64(0)}, "a1")
			ids, total, err := repository.QueryHistoricActivityInstanceIDs(context.Background(), tc.query)
			if err != nil || total != 1 || len(ids) != 1 || ids[0] != "a1" {
				t.Fatalf("查询错误: %v %v %d", ids, err, total)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricActivityInstanceIDs_AllFilters 验证全部过滤条件同时生效时的拼接顺序与参数个数。
// @param t 测试上下文
func TestQueryHistoricActivityInstanceIDs_AllFilters(t *testing.T) {
	startedAfter := time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)
	startedBefore := time.Date(2026, 5, 7, 7, 8, 9, 0, time.UTC)
	finishedAfter := time.Date(2026, 5, 8, 7, 8, 9, 0, time.UTC)
	finishedBefore := time.Date(2026, 5, 9, 7, 8, 9, 0, time.UTC)
	repository, mock := newHistoricActivityQueryRepository(t, "mysql")
	from := historicActivityFrom + " INNER JOIN ACT_RE_PROCDEF P ON P.ID_ = A.PROC_DEF_ID_ AND P.KEY_ = ?"
	where := " WHERE A.ID_ = ? AND A.PROC_INST_ID_ = ? AND A.PROC_DEF_ID_ = ? AND A.ACT_ID_ = ?" +
		" AND A.ACT_NAME_ = ? AND A.ACT_TYPE_ = ? AND A.ASSIGNEE_ = ? AND A.TENANT_ID_ = ?" +
		" AND A.END_TIME_ IS NOT NULL AND A.START_TIME_ >= ? AND A.START_TIME_ <= ?" +
		" AND A.END_TIME_ >= ? AND A.END_TIME_ <= ?"
	args := []driver.Value{"demo", "a1", "pi1", "pd1", "act1", "审批", "userTask", "张三", "acme",
		startedAfter, startedBefore, finishedAfter, finishedBefore}
	expectHistoricActivityQuery(mock,
		"SELECT COUNT(*) FROM "+from+where, args, 1,
		"SELECT A.ID_ FROM "+from+where+historicActivityOrderBy+" LIMIT ? OFFSET ?",
		append(append([]driver.Value{}, args...), int64(5), int64(2)), "a1")
	ids, total, err := repository.QueryHistoricActivityInstanceIDs(context.Background(), HistoricActivityInstanceQuery{
		ID:                   "a1",
		ProcessInstanceID:    "pi1",
		ProcessDefinitionID:  "pd1",
		ProcessDefinitionKey: "demo",
		ActivityID:           "act1",
		ActivityName:         "审批",
		ActivityType:         "userTask",
		Assignee:             "张三",
		TenantID:             "acme",
		StartedAfter:         &startedAfter,
		StartedBefore:        &startedBefore,
		FinishedAfter:        &finishedAfter,
		FinishedBefore:       &finishedBefore,
		Finished:             ptrBool(true),
		Start:                2,
		Size:                 5,
	})
	if err != nil || total != 1 || len(ids) != 1 || ids[0] != "a1" {
		t.Fatalf("查询错误: %v %v %d", ids, err, total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryHistoricActivityInstanceIDs_Pagination 验证分页参数规范化。
// @param t 测试上下文
func TestQueryHistoricActivityInstanceIDs_Pagination(t *testing.T) {
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
		{"显式分页", 2, 3, 3, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newHistoricActivityQueryRepository(t, "mysql")
			expectHistoricActivityQuery(mock,
				"SELECT COUNT(*) FROM "+historicActivityFrom, nil, 0,
				"SELECT A.ID_ FROM "+historicActivityFrom+historicActivityOrderBy+" LIMIT ? OFFSET ?",
				[]driver.Value{tc.limit, tc.offset})
			ids, total, err := repository.QueryHistoricActivityInstanceIDs(context.Background(), HistoricActivityInstanceQuery{Start: tc.start, Size: tc.size})
			if err != nil || total != 0 || len(ids) != 0 {
				t.Fatalf("查询错误: %v %v %d", ids, err, total)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricActivityInstanceIDs_PostgresPlaceholders 验证 PostgreSQL 占位符编号。
// @param t 测试上下文
func TestQueryHistoricActivityInstanceIDs_PostgresPlaceholders(t *testing.T) {
	repository, mock := newHistoricActivityQueryRepository(t, "postgres")
	startedAfter := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	finishedBefore := time.Date(2026, 4, 5, 6, 7, 8, 0, time.UTC)
	from := historicActivityFrom + " INNER JOIN ACT_RE_PROCDEF P ON P.ID_ = A.PROC_DEF_ID_ AND P.KEY_ = $1"
	where := " WHERE A.ID_ = $2 AND A.END_TIME_ IS NOT NULL AND A.START_TIME_ >= $3 AND A.END_TIME_ <= $4"
	expectHistoricActivityQuery(mock,
		"SELECT COUNT(*) FROM "+from+where,
		[]driver.Value{"demo", "a1", startedAfter, finishedBefore}, 1,
		"SELECT A.ID_ FROM "+from+where+historicActivityOrderBy+" LIMIT $5 OFFSET $6",
		[]driver.Value{"demo", "a1", startedAfter, finishedBefore, int64(5), int64(10)}, "a1")
	ids, total, err := repository.QueryHistoricActivityInstanceIDs(context.Background(), HistoricActivityInstanceQuery{
		ProcessDefinitionKey: "demo",
		ID:                   "a1",
		Finished:             ptrBool(true),
		StartedAfter:         &startedAfter,
		FinishedBefore:       &finishedBefore,
		Start:                10,
		Size:                 5,
	})
	if err != nil || total != 1 || len(ids) != 1 || ids[0] != "a1" {
		t.Fatalf("查询错误: %v %v %d", ids, err, total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryHistoricActivityInstanceIDs_UnsupportedDB 验证数据库不支持查询时的错误。
// @param t 测试上下文
func TestQueryHistoricActivityInstanceIDs_UnsupportedDB(t *testing.T) {
	repository, err := NewRepository(&fakeDB{}, "mysql")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = repository.QueryHistoricActivityInstanceIDs(context.Background(), HistoricActivityInstanceQuery{})
	requireHistoricActivityQueryError(t, err, "ROW-0055")
}

// TestQueryHistoricActivityInstanceIDs_CountErrors 验证计数阶段错误。
// @param t 测试上下文
func TestQueryHistoricActivityInstanceIDs_CountErrors(t *testing.T) {
	countQuery := "SELECT COUNT(*) FROM " + historicActivityFrom
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
			repository, mock := newHistoricActivityQueryRepository(t, "mysql")
			tc.setup(mock, countQuery)
			_, _, err := repository.QueryHistoricActivityInstanceIDs(context.Background(), HistoricActivityInstanceQuery{})
			requireHistoricActivityQueryError(t, err, "ROW-0056")
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricActivityInstanceIDs_IDListErrors 验证 ID 列表阶段错误。
// @param t 测试上下文
func TestQueryHistoricActivityInstanceIDs_IDListErrors(t *testing.T) {
	selectQuery := "SELECT A.ID_ FROM " + historicActivityFrom + historicActivityOrderBy + " LIMIT ? OFFSET ?"
	cases := []struct {
		name  string                     // 用例名
		setup func(mock sqlmock.Sqlmock) // 断言设置
		code  string                     // 期望错误码
	}{
		{"查询失败", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).WillReturnError(errors.New("数据库不可用"))
		}, "ROW-0057"},
		{"扫描失败", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"ID_", "extra"}).AddRow("a1", "x"))
		}, "ROW-0058"},
		{"值类型非法", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"ID_"}).AddRow(true))
		}, "ROW-0058"},
		{"遍历失败", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"ID_"}).AddRow("a1").RowError(0, errors.New("遍历")))
		}, "ROW-0059"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newHistoricActivityQueryRepository(t, "mysql")
			mock.ExpectQuery("SELECT COUNT(*) FROM " + historicActivityFrom).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
			tc.setup(mock)
			_, _, err := repository.QueryHistoricActivityInstanceIDs(context.Background(), HistoricActivityInstanceQuery{})
			requireHistoricActivityQueryError(t, err, tc.code)
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryHistoricActivityCountIterationError 验证历史活动计数查询在无行且遍历出错时返回错误。
// @param t 测试上下文
func TestQueryHistoricActivityCountIterationError(t *testing.T) {
	_, err := queryHistoricActivityCount(context.Background(), fixedQueryDB{rows: errRows{}}, "SELECT 1", nil)
	requireHistoricActivityQueryError(t, err, "ROW-0056")
}

// TestQueryHistoricActivityIDListIterationError 验证历史活动 ID 查询在结果集遍历出错时返回错误。
// @param t 测试上下文
func TestQueryHistoricActivityIDListIterationError(t *testing.T) {
	_, err := queryHistoricActivityIDList(context.Background(), fixedQueryDB{rows: errRows{}}, "SELECT 1", nil)
	requireHistoricActivityQueryError(t, err, "ROW-0059")
}

// TestHistoricActivityQueryValueConversions 验证驱动值到历史活动计数与历史活动 ID 的转换。
// @param t 测试上下文
func TestHistoricActivityQueryValueConversions(t *testing.T) {
	totalCases := []struct {
		value  any   // 驱动值
		want   int64 // 期望计数
		ok     bool  // 是否成功
		engine bool  // 失败时是否为带错误码的引擎错误（字符串解析失败为底层错误）
	}{
		{int64(3), 3, true, false},
		{7, 7, true, false},
		{"9", 9, true, false},
		{[]byte("11"), 11, true, false},
		{"abc", 0, false, false},
		{[]byte("x"), 0, false, false},
		{true, 0, false, true},
		{struct{}{}, 0, false, true},
	}
	for _, tc := range totalCases {
		got, err := historicActivityCountValue(tc.value)
		// 合法类型必须转换成功，非法类型必须返回错误
		if tc.ok && (err != nil || got != tc.want) {
			t.Fatalf("计数转换错误: %v %v", got, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("非法计数应失败: %v", tc.value)
		}
		if !tc.ok && tc.engine {
			requireHistoricActivityQueryError(t, err, "ROW-0056")
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
		{"a1", "a1", true},
		{[]byte("a2"), "a2", true},
		{42, "", false},
		{true, "", false},
		{struct{}{}, "", false},
	}
	for _, tc := range idCases {
		got, err := historicActivityIDValue(tc.value)
		// 合法类型必须转换成功，非法类型必须返回错误
		if tc.ok && (err != nil || got != tc.want) {
			t.Fatalf("ID 转换错误: %v %v", got, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("非法 ID 应失败: %v", tc.value)
		}
		if !tc.ok {
			requireHistoricActivityQueryError(t, err, "ROW-0058")
		}
	}
}
