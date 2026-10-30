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

// newInstanceQueryRepository 创建基于 sqlmock 的流程实例查询仓储。
// @param t 测试上下文
// @param dialect 数据库方言
// @return 仓储
// @return SQL 断言器
func newInstanceQueryRepository(t *testing.T, dialect string) (*Repository, sqlmock.Sqlmock) {
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

// expectInstanceQuery 断言一次实例计数查询和一次分页 ID 查询。
// @param mock SQL 断言器
// @param countQuery 计数 SQL
// @param countArgs 计数参数
// @param total 返回总数
// @param selectQuery ID 查询 SQL
// @param selectArgs ID 查询参数
// @param ids 返回的实例 ID
func expectInstanceQuery(mock sqlmock.Sqlmock, countQuery string, countArgs []driver.Value, total int64, selectQuery string, selectArgs []driver.Value, ids ...string) {
	mock.ExpectQuery(countQuery).WithArgs(countArgs...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(total))
	idRows := sqlmock.NewRows([]string{"PROC_INST_ID_"})
	for _, id := range ids {
		idRows.AddRow(id)
	}
	mock.ExpectQuery(selectQuery).WithArgs(selectArgs...).WillReturnRows(idRows)
}

// requireInstanceQueryError 断言错误为指定错误码与中文消息的 api.Error，且不包含 SQL 文本。
// @param t 测试上下文
// @param err 错误
// @param code 期望错误码
func requireInstanceQueryError(t *testing.T, err error, code string) {
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
	case "ROW-0040":
		message = "数据库不支持查询"
	case "ROW-0041":
		message = "统计流程实例数量失败"
	case "ROW-0042":
		message = "查询流程实例 ID 失败"
	case "ROW-0043":
		message = "读取流程实例 ID 失败"
	case "ROW-0044":
		message = "遍历流程实例 ID 失败"
	default:
		t.Fatalf("未知错误码: %s", code)
	}
	if engineErr.Message != message {
		t.Fatalf("错误消息不正确: %s", engineErr.Message)
	}
	if strings.Contains(engineErr.Message, "SELECT") || strings.Contains(engineErr.Message, "ACT_RU_EXECUTION") {
		t.Fatalf("错误信息泄漏 SQL: %s", engineErr.Message)
	}
}

// ptrBool 返回布尔值指针。
// @param value 布尔值
// @return 布尔值指针
func ptrBool(value bool) *bool { return &value }

// TestQueryProcessInstanceIDs_NoFilter 验证无过滤条件时的实例根限定与默认分页。
// @param t 测试上下文
func TestQueryProcessInstanceIDs_NoFilter(t *testing.T) {
	repository, mock := newInstanceQueryRepository(t, "mysql")
	expectInstanceQuery(mock,
		"SELECT COUNT(*) FROM ACT_RU_EXECUTION E WHERE E.PARENT_ID_ IS NULL", nil, 2,
		"SELECT E.PROC_INST_ID_ FROM ACT_RU_EXECUTION E WHERE E.PARENT_ID_ IS NULL ORDER BY E.START_TIME_ ASC, E.PROC_INST_ID_ ASC LIMIT ? OFFSET ?",
		[]driver.Value{int64(10), int64(0)}, "i1", "i2")
	ids, total, err := repository.QueryProcessInstanceIDs(context.Background(), ProcessInstanceQuery{})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if total != 2 || len(ids) != 2 || ids[0] != "i1" || ids[1] != "i2" {
		t.Fatalf("结果错误: %v %d", ids, total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryProcessInstanceIDs_ColumnFilters 验证各直接列过滤条件的参数绑定。
// @param t 测试上下文
func TestQueryProcessInstanceIDs_ColumnFilters(t *testing.T) {
	cases := []struct {
		name  string               // 用例名
		where string               // WHERE 条件
		query ProcessInstanceQuery // 查询条件
		args  []driver.Value       // 参数
	}{
		{"实例ID", "E.ID_ = ?", ProcessInstanceQuery{ID: "i1"}, []driver.Value{"i1"}},
		{"流程定义ID", "E.PROC_DEF_ID_ = ?", ProcessInstanceQuery{ProcessDefinitionID: "pd1"}, []driver.Value{"pd1"}},
		{"业务键", "E.BUSINESS_KEY_ = ?", ProcessInstanceQuery{BusinessKey: "bk1"}, []driver.Value{"bk1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newInstanceQueryRepository(t, "mysql")
			expectInstanceQuery(mock,
				"SELECT COUNT(*) FROM ACT_RU_EXECUTION E WHERE E.PARENT_ID_ IS NULL AND "+tc.where, tc.args, 1,
				"SELECT E.PROC_INST_ID_ FROM ACT_RU_EXECUTION E WHERE E.PARENT_ID_ IS NULL AND "+tc.where+" ORDER BY E.START_TIME_ ASC, E.PROC_INST_ID_ ASC LIMIT ? OFFSET ?",
				append(append([]driver.Value{}, tc.args...), int64(10), int64(0)), "i1")
			ids, total, err := repository.QueryProcessInstanceIDs(context.Background(), tc.query)
			if err != nil || total != 1 || len(ids) != 1 || ids[0] != "i1" {
				t.Fatalf("查询错误: %v %v %d", ids, err, total)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryProcessInstanceIDs_Suspended 验证挂起状态过滤的两种取值。
// @param t 测试上下文
func TestQueryProcessInstanceIDs_Suspended(t *testing.T) {
	cases := []struct {
		name      string // 用例名
		suspended bool   // 挂起状态取值
		state     int64  // 期望 SUSPENSION_STATE_ 编码
	}{
		{"已挂起", true, 2},
		{"活动中", false, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newInstanceQueryRepository(t, "mysql")
			where := " WHERE E.PARENT_ID_ IS NULL AND E.SUSPENSION_STATE_ = ?"
			expectInstanceQuery(mock,
				"SELECT COUNT(*) FROM ACT_RU_EXECUTION E"+where, []driver.Value{tc.state}, 1,
				"SELECT E.PROC_INST_ID_ FROM ACT_RU_EXECUTION E"+where+" ORDER BY E.START_TIME_ ASC, E.PROC_INST_ID_ ASC LIMIT ? OFFSET ?",
				[]driver.Value{tc.state, int64(10), int64(0)}, "i1")
			ids, total, err := repository.QueryProcessInstanceIDs(context.Background(), ProcessInstanceQuery{Suspended: ptrBool(tc.suspended)})
			if err != nil || total != 1 || len(ids) != 1 || ids[0] != "i1" {
				t.Fatalf("查询错误: %v %v %d", ids, err, total)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryProcessInstanceIDs_ProcessDefinitionKey 验证流程定义 Key 的 JOIN 下推及参数顺序。
// @param t 测试上下文
func TestQueryProcessInstanceIDs_ProcessDefinitionKey(t *testing.T) {
	repository, mock := newInstanceQueryRepository(t, "mysql")
	from := "ACT_RU_EXECUTION E INNER JOIN ACT_RE_PROCDEF P ON P.ID_ = E.PROC_DEF_ID_ AND P.KEY_ = ?"
	where := " WHERE E.PARENT_ID_ IS NULL AND E.ID_ = ?"
	expectInstanceQuery(mock,
		"SELECT COUNT(*) FROM "+from+where, []driver.Value{"demo", "i1"}, 1,
		"SELECT E.PROC_INST_ID_ FROM "+from+where+" ORDER BY E.START_TIME_ ASC, E.PROC_INST_ID_ ASC LIMIT ? OFFSET ?",
		[]driver.Value{"demo", "i1", int64(10), int64(0)}, "i1")
	ids, total, err := repository.QueryProcessInstanceIDs(context.Background(), ProcessInstanceQuery{ProcessDefinitionKey: "demo", ID: "i1"})
	if err != nil || total != 1 || len(ids) != 1 || ids[0] != "i1" {
		t.Fatalf("查询错误: %v %v %d", ids, err, total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryProcessInstanceIDs_TimeRange 验证开始时间上下界的参数化。
// @param t 测试上下文
func TestQueryProcessInstanceIDs_TimeRange(t *testing.T) {
	after := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	before := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	cases := []struct {
		name  string               // 用例名
		query ProcessInstanceQuery // 查询条件
		op    string               // 期望比较运算符
		arg   time.Time            // 期望时间参数
	}{
		{"开始下界", ProcessInstanceQuery{StartedAfter: &after}, ">=", after},
		{"开始上界", ProcessInstanceQuery{StartedBefore: &before}, "<=", before},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newInstanceQueryRepository(t, "mysql")
			where := " WHERE E.PARENT_ID_ IS NULL AND E.START_TIME_ " + tc.op + " ?"
			expectInstanceQuery(mock,
				"SELECT COUNT(*) FROM ACT_RU_EXECUTION E"+where, []driver.Value{tc.arg}, 1,
				"SELECT E.PROC_INST_ID_ FROM ACT_RU_EXECUTION E"+where+" ORDER BY E.START_TIME_ ASC, E.PROC_INST_ID_ ASC LIMIT ? OFFSET ?",
				[]driver.Value{tc.arg, int64(10), int64(0)}, "i1")
			ids, total, err := repository.QueryProcessInstanceIDs(context.Background(), tc.query)
			if err != nil || total != 1 || len(ids) != 1 || ids[0] != "i1" {
				t.Fatalf("查询错误: %v %v %d", ids, err, total)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryProcessInstanceIDs_Pagination 验证分页参数规范化。
// @param t 测试上下文
func TestQueryProcessInstanceIDs_Pagination(t *testing.T) {
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
			repository, mock := newInstanceQueryRepository(t, "mysql")
			expectInstanceQuery(mock,
				"SELECT COUNT(*) FROM ACT_RU_EXECUTION E WHERE E.PARENT_ID_ IS NULL", nil, 0,
				"SELECT E.PROC_INST_ID_ FROM ACT_RU_EXECUTION E WHERE E.PARENT_ID_ IS NULL ORDER BY E.START_TIME_ ASC, E.PROC_INST_ID_ ASC LIMIT ? OFFSET ?",
				[]driver.Value{tc.limit, tc.offset})
			ids, total, err := repository.QueryProcessInstanceIDs(context.Background(), ProcessInstanceQuery{Start: tc.start, Size: tc.size})
			if err != nil || total != 0 || len(ids) != 0 {
				t.Fatalf("查询错误: %v %v %d", ids, err, total)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryProcessInstanceIDs_PostgresPlaceholders 验证 PostgreSQL 占位符编号。
// @param t 测试上下文
func TestQueryProcessInstanceIDs_PostgresPlaceholders(t *testing.T) {
	repository, mock := newInstanceQueryRepository(t, "postgres")
	startedAfter := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	from := "ACT_RU_EXECUTION E INNER JOIN ACT_RE_PROCDEF P ON P.ID_ = E.PROC_DEF_ID_ AND P.KEY_ = $1"
	where := " WHERE E.PARENT_ID_ IS NULL AND E.ID_ = $2 AND E.SUSPENSION_STATE_ = $3 AND E.START_TIME_ >= $4"
	expectInstanceQuery(mock,
		"SELECT COUNT(*) FROM "+from+where, []driver.Value{"demo", "i1", int64(2), startedAfter}, 1,
		"SELECT E.PROC_INST_ID_ FROM "+from+where+" ORDER BY E.START_TIME_ ASC, E.PROC_INST_ID_ ASC LIMIT $5 OFFSET $6",
		[]driver.Value{"demo", "i1", int64(2), startedAfter, int64(5), int64(10)}, "i1")
	ids, total, err := repository.QueryProcessInstanceIDs(context.Background(), ProcessInstanceQuery{
		ProcessDefinitionKey: "demo",
		ID:                   "i1",
		Suspended:            ptrBool(true),
		StartedAfter:         &startedAfter,
		Start:                10,
		Size:                 5,
	})
	if err != nil || total != 1 || len(ids) != 1 || ids[0] != "i1" {
		t.Fatalf("查询错误: %v %v %d", ids, err, total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryProcessInstanceIDs_UnsupportedDB 验证数据库不支持查询时的错误。
// @param t 测试上下文
func TestQueryProcessInstanceIDs_UnsupportedDB(t *testing.T) {
	repository, err := NewRepository(&fakeDB{}, "mysql")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = repository.QueryProcessInstanceIDs(context.Background(), ProcessInstanceQuery{})
	requireInstanceQueryError(t, err, "ROW-0040")
}

// TestQueryProcessInstanceIDs_CountErrors 验证计数阶段错误。
// @param t 测试上下文
func TestQueryProcessInstanceIDs_CountErrors(t *testing.T) {
	countQuery := "SELECT COUNT(*) FROM ACT_RU_EXECUTION E WHERE E.PARENT_ID_ IS NULL"
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
			repository, mock := newInstanceQueryRepository(t, "mysql")
			tc.setup(mock, countQuery)
			_, _, err := repository.QueryProcessInstanceIDs(context.Background(), ProcessInstanceQuery{})
			requireInstanceQueryError(t, err, "ROW-0041")
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryProcessInstanceIDs_IDListErrors 验证 ID 列表阶段错误。
// @param t 测试上下文
func TestQueryProcessInstanceIDs_IDListErrors(t *testing.T) {
	selectQuery := "SELECT E.PROC_INST_ID_ FROM ACT_RU_EXECUTION E WHERE E.PARENT_ID_ IS NULL ORDER BY E.START_TIME_ ASC, E.PROC_INST_ID_ ASC LIMIT ? OFFSET ?"
	cases := []struct {
		name  string                     // 用例名
		setup func(mock sqlmock.Sqlmock) // 断言设置
		code  string                     // 期望错误码
	}{
		{"查询失败", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).WillReturnError(errors.New("数据库不可用"))
		}, "ROW-0042"},
		{"扫描失败", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"PROC_INST_ID_"}).AddRow(nil))
		}, "ROW-0043"},
		{"列数不匹配", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"PROC_INST_ID_", "extra"}).AddRow("i1", "x"))
		}, "ROW-0043"},
		{"值类型非法", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"PROC_INST_ID_"}).AddRow(true))
		}, "ROW-0043"},
		{"遍历失败", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"PROC_INST_ID_"}).AddRow("i1").RowError(0, errors.New("遍历")))
		}, "ROW-0044"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newInstanceQueryRepository(t, "mysql")
			mock.ExpectQuery("SELECT COUNT(*) FROM ACT_RU_EXECUTION E WHERE E.PARENT_ID_ IS NULL").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
			tc.setup(mock)
			_, _, err := repository.QueryProcessInstanceIDs(context.Background(), ProcessInstanceQuery{})
			requireInstanceQueryError(t, err, tc.code)
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryInstanceCountIterationError 验证实例计数查询在无行且遍历出错时返回错误。
// @param t 测试上下文
func TestQueryInstanceCountIterationError(t *testing.T) {
	_, err := queryInstanceCount(context.Background(), fixedQueryDB{rows: errRows{}}, "SELECT 1", nil)
	requireInstanceQueryError(t, err, "ROW-0041")
}

// TestInstanceQueryValueConversions 验证驱动值到实例计数与实例 ID 的转换。
// @param t 测试上下文
func TestInstanceQueryValueConversions(t *testing.T) {
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
		got, err := instanceCountValue(tc.value)
		// 非法类型必须返回错误
		if tc.ok && (err != nil || got != tc.want) {
			t.Fatalf("计数转换错误: %v %v", got, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("非法计数应失败: %v", tc.value)
		}
		if !tc.ok {
			requireInstanceQueryError(t, err, "ROW-0041")
		}
	}
	idCases := []struct {
		value any    // 驱动值
		want  string // 期望 ID
		ok    bool   // 是否成功
	}{
		{"i1", "i1", true},
		{[]byte("i2"), "i2", true},
		{true, "", false},
		{struct{}{}, "", false},
	}
	for _, tc := range idCases {
		got, err := instanceIDValue(tc.value)
		// 非法类型必须返回错误
		if tc.ok && (err != nil || got != tc.want) {
			t.Fatalf("ID 转换错误: %v %v", got, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("非法 ID 应失败: %v", tc.value)
		}
		if !tc.ok {
			requireInstanceQueryError(t, err, "ROW-0043")
		}
	}
}
