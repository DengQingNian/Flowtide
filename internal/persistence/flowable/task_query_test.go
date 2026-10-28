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

// newTaskQueryRepository 创建基于 sqlmock 的任务查询仓储。
// @param t 测试上下文
// @param dialect 数据库方言
// @return 仓储
// @return SQL 断言器
func newTaskQueryRepository(t *testing.T, dialect string) (*Repository, sqlmock.Sqlmock) {
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

// expectTaskQuery 断言一次计数查询和一次分页 ID 查询。
// @param mock SQL 断言器
// @param countQuery 计数 SQL
// @param countArgs 计数参数
// @param total 返回总数
// @param selectQuery ID 查询 SQL
// @param selectArgs ID 查询参数
// @param ids 返回的任务 ID
func expectTaskQuery(mock sqlmock.Sqlmock, countQuery string, countArgs []driver.Value, total int64, selectQuery string, selectArgs []driver.Value, ids ...string) {
	mock.ExpectQuery(countQuery).WithArgs(countArgs...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(total))
	idRows := sqlmock.NewRows([]string{"ID_"})
	for _, id := range ids {
		idRows.AddRow(id)
	}
	mock.ExpectQuery(selectQuery).WithArgs(selectArgs...).WillReturnRows(idRows)
}

// requireTaskQueryError 断言错误为指定错误码的中文 api.Error，且不包含 SQL 文本。
// @param t 测试上下文
// @param err 错误
// @param code 期望错误码
func requireTaskQueryError(t *testing.T, err error, code string) {
	t.Helper()
	var engineErr *api.Error
	if !errors.As(err, &engineErr) {
		t.Fatalf("错误类型不正确: %T %v", err, err)
	}
	if engineErr.Code != code {
		t.Fatalf("错误码不正确: %s %v", engineErr.Code, err)
	}
	if strings.Contains(engineErr.Message, "SELECT") || strings.Contains(engineErr.Message, "ACT_RU_TASK") {
		t.Fatalf("错误信息泄漏 SQL: %s", engineErr.Message)
	}
}

// TestQueryTaskIDs_NoFilter 验证无过滤条件时的计数与分页。
// @param t 测试上下文
func TestQueryTaskIDs_NoFilter(t *testing.T) {
	repository, mock := newTaskQueryRepository(t, "mysql")
	expectTaskQuery(mock,
		"SELECT COUNT(*) FROM ACT_RU_TASK T", nil, 2,
		"SELECT T.ID_ FROM ACT_RU_TASK T ORDER BY T.CREATE_TIME_ ASC, T.ID_ ASC LIMIT ? OFFSET ?",
		[]driver.Value{int64(10), int64(0)}, "t1", "t2")
	ids, total, err := repository.QueryTaskIDs(context.Background(), TaskQuery{})
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

// TestQueryTaskIDs_ColumnFilters 验证各直接列过滤条件。
// @param t 测试上下文
func TestQueryTaskIDs_ColumnFilters(t *testing.T) {
	cases := []struct {
		name  string         // 用例名
		where string         // WHERE 条件
		query TaskQuery      // 查询条件
		args  []driver.Value // 参数
	}{
		{"处理人", "T.ASSIGNEE_ = ?", TaskQuery{Assignee: "alice"}, []driver.Value{"alice"}},
		{"所有者", "T.OWNER_ = ?", TaskQuery{Owner: "bob"}, []driver.Value{"bob"}},
		{"委派状态", "T.DELEGATION_ = ?", TaskQuery{DelegationState: "PENDING"}, []driver.Value{"PENDING"}},
		{"流程实例", "T.PROC_INST_ID_ = ?", TaskQuery{ProcessInstanceID: "i1"}, []driver.Value{"i1"}},
		{"父任务", "T.PARENT_TASK_ID_ = ?", TaskQuery{ParentTaskID: "p1"}, []driver.Value{"p1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newTaskQueryRepository(t, "mysql")
			expectTaskQuery(mock,
				"SELECT COUNT(*) FROM ACT_RU_TASK T WHERE "+tc.where, tc.args, 1,
				"SELECT T.ID_ FROM ACT_RU_TASK T WHERE "+tc.where+" ORDER BY T.CREATE_TIME_ ASC, T.ID_ ASC LIMIT ? OFFSET ?",
				append(append([]driver.Value{}, tc.args...), int64(10), int64(0)), "t1")
			ids, total, err := repository.QueryTaskIDs(context.Background(), tc.query)
			if err != nil || total != 1 || len(ids) != 1 || ids[0] != "t1" {
				t.Fatalf("查询错误: %v %v %d", ids, err, total)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryTaskIDs_ProcessDefinitionKey 验证流程定义 Key 的 JOIN 下推。
// @param t 测试上下文
func TestQueryTaskIDs_ProcessDefinitionKey(t *testing.T) {
	repository, mock := newTaskQueryRepository(t, "mysql")
	from := "ACT_RU_TASK T INNER JOIN ACT_RE_PROCDEF P ON P.ID_ = T.PROC_DEF_ID_ AND P.KEY_ = ?"
	expectTaskQuery(mock,
		"SELECT COUNT(*) FROM "+from, []driver.Value{"demo"}, 1,
		"SELECT T.ID_ FROM "+from+" ORDER BY T.CREATE_TIME_ ASC, T.ID_ ASC LIMIT ? OFFSET ?",
		[]driver.Value{"demo", int64(10), int64(0)}, "t1")
	ids, total, err := repository.QueryTaskIDs(context.Background(), TaskQuery{ProcessDefinitionKey: "demo"})
	if err != nil || total != 1 || len(ids) != 1 {
		t.Fatalf("查询错误: %v %v %d", ids, err, total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryTaskIDs_CandidateUser 验证候选人用户、候选组及空候选组语义。
// @param t 测试上下文
func TestQueryTaskIDs_CandidateUser(t *testing.T) {
	userCondition := "(T.ASSIGNEE_ IS NULL OR T.ASSIGNEE_ = '') AND EXISTS (SELECT 1 FROM ACT_RU_IDENTITYLINK L WHERE L.TASK_ID_ = T.ID_ AND L.TYPE_ = 'candidate' AND (L.USER_ID_ = ?))"
	groupCondition := "(T.ASSIGNEE_ IS NULL OR T.ASSIGNEE_ = '') AND EXISTS (SELECT 1 FROM ACT_RU_IDENTITYLINK L WHERE L.TASK_ID_ = T.ID_ AND L.TYPE_ = 'candidate' AND (L.USER_ID_ = ? OR L.GROUP_ID_ IN (?, ?)))"
	cases := []struct {
		name  string         // 用例名
		query TaskQuery      // 查询条件
		where string         // 条件
		args  []driver.Value // 参数
	}{
		{"仅用户候选", TaskQuery{CandidateUser: "alice"}, userCondition, []driver.Value{"alice"}},
		{"用户与候选组", TaskQuery{CandidateUser: "alice", CandidateGroups: []string{"g1", "g2"}}, groupCondition, []driver.Value{"alice", "g1", "g2"}},
		{"空候选组只匹配用户", TaskQuery{CandidateUser: "alice", CandidateGroups: []string{"", ""}}, userCondition, []driver.Value{"alice"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newTaskQueryRepository(t, "mysql")
			expectTaskQuery(mock,
				"SELECT COUNT(*) FROM ACT_RU_TASK T WHERE "+tc.where, tc.args, 1,
				"SELECT T.ID_ FROM ACT_RU_TASK T WHERE "+tc.where+" ORDER BY T.CREATE_TIME_ ASC, T.ID_ ASC LIMIT ? OFFSET ?",
				append(append([]driver.Value{}, tc.args...), int64(10), int64(0)), "t1")
			ids, total, err := repository.QueryTaskIDs(context.Background(), tc.query)
			if err != nil || total != 1 || len(ids) != 1 {
				t.Fatalf("查询错误: %v %v %d", ids, err, total)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryTaskIDs_Pagination 验证分页参数规范化。
// @param t 测试上下文
func TestQueryTaskIDs_Pagination(t *testing.T) {
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
			repository, mock := newTaskQueryRepository(t, "mysql")
			expectTaskQuery(mock,
				"SELECT COUNT(*) FROM ACT_RU_TASK T", nil, 0,
				"SELECT T.ID_ FROM ACT_RU_TASK T ORDER BY T.CREATE_TIME_ ASC, T.ID_ ASC LIMIT ? OFFSET ?",
				[]driver.Value{tc.limit, tc.offset})
			ids, total, err := repository.QueryTaskIDs(context.Background(), TaskQuery{Start: tc.start, Size: tc.size})
			if err != nil || total != 0 || len(ids) != 0 {
				t.Fatalf("查询错误: %v %v %d", ids, err, total)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryTaskIDs_PostgresPlaceholders 验证 PostgreSQL 占位符编号。
// @param t 测试上下文
func TestQueryTaskIDs_PostgresPlaceholders(t *testing.T) {
	repository, mock := newTaskQueryRepository(t, "postgres")
	from := "ACT_RU_TASK T INNER JOIN ACT_RE_PROCDEF P ON P.ID_ = T.PROC_DEF_ID_ AND P.KEY_ = $1"
	where := " WHERE T.ASSIGNEE_ = $2 AND (T.ASSIGNEE_ IS NULL OR T.ASSIGNEE_ = '') AND EXISTS (SELECT 1 FROM ACT_RU_IDENTITYLINK L WHERE L.TASK_ID_ = T.ID_ AND L.TYPE_ = 'candidate' AND (L.USER_ID_ = $3 OR L.GROUP_ID_ IN ($4)))"
	expectTaskQuery(mock,
		"SELECT COUNT(*) FROM "+from+where, []driver.Value{"demo", "alice", "alice", "g1"}, 1,
		"SELECT T.ID_ FROM "+from+where+" ORDER BY T.CREATE_TIME_ ASC, T.ID_ ASC LIMIT $5 OFFSET $6",
		[]driver.Value{"demo", "alice", "alice", "g1", int64(5), int64(10)}, "t1")
	ids, total, err := repository.QueryTaskIDs(context.Background(), TaskQuery{
		ProcessDefinitionKey: "demo",
		Assignee:             "alice",
		CandidateUser:        "alice",
		CandidateGroups:      []string{"g1"},
		Start:                10,
		Size:                 5,
	})
	if err != nil || total != 1 || len(ids) != 1 {
		t.Fatalf("查询错误: %v %v %d", ids, err, total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryTaskIDs_TenantNotPushedDown 验证租户过滤被明确拒绝。
// @param t 测试上下文
func TestQueryTaskIDs_TenantNotPushedDown(t *testing.T) {
	repository, mock := newTaskQueryRepository(t, "mysql")
	_, _, err := repository.QueryTaskIDs(context.Background(), TaskQuery{TenantID: "t1"})
	requireTaskQueryError(t, err, "ROW-0035")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryTaskIDs_UnsupportedDB 验证数据库不支持查询时的错误。
// @param t 测试上下文
func TestQueryTaskIDs_UnsupportedDB(t *testing.T) {
	repository, err := NewRepository(&fakeDB{}, "mysql")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = repository.QueryTaskIDs(context.Background(), TaskQuery{})
	requireTaskQueryError(t, err, "ROW-0017")
}

// TestQueryTaskIDs_CountErrors 验证计数阶段错误。
// @param t 测试上下文
func TestQueryTaskIDs_CountErrors(t *testing.T) {
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
			mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow("bad"))
		}},
		{"列数不匹配", func(mock sqlmock.Sqlmock, query string) {
			mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"count", "extra"}).AddRow(int64(1), int64(2)))
		}},
		{"遍历失败", func(mock sqlmock.Sqlmock, query string) {
			mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"count"}).RowError(0, errors.New("遍历")))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newTaskQueryRepository(t, "mysql")
			tc.setup(mock, "SELECT COUNT(*) FROM ACT_RU_TASK T")
			_, _, err := repository.QueryTaskIDs(context.Background(), TaskQuery{})
			requireTaskQueryError(t, err, "ROW-0036")
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestQueryTaskIDs_IDListErrors 验证 ID 列表阶段错误。
// @param t 测试上下文
func TestQueryTaskIDs_IDListErrors(t *testing.T) {
	selectQuery := "SELECT T.ID_ FROM ACT_RU_TASK T ORDER BY T.CREATE_TIME_ ASC, T.ID_ ASC LIMIT ? OFFSET ?"
	cases := []struct {
		name  string                     // 用例名
		setup func(mock sqlmock.Sqlmock) // 断言设置
		code  string                     // 期望错误码
	}{
		{"查询失败", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).WillReturnError(errors.New("数据库不可用"))
		}, "ROW-0037"},
		{"扫描失败", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"ID_"}).AddRow(nil))
		}, "ROW-0038"},
		{"列数不匹配", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"ID_", "extra"}).AddRow("t1", "x"))
		}, "ROW-0038"},
		{"遍历失败", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(selectQuery).WithArgs(int64(10), int64(0)).
				WillReturnRows(sqlmock.NewRows([]string{"ID_"}).AddRow("t1").RowError(0, errors.New("遍历")))
		}, "ROW-0039"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository, mock := newTaskQueryRepository(t, "mysql")
			mock.ExpectQuery("SELECT COUNT(*) FROM ACT_RU_TASK T").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
			tc.setup(mock)
			_, _, err := repository.QueryTaskIDs(context.Background(), TaskQuery{})
			requireTaskQueryError(t, err, tc.code)
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestTaskQueryValueConversions 验证驱动值到计数与任务 ID 的转换。
// @param t 测试上下文
func TestTaskQueryValueConversions(t *testing.T) {
	totalCases := []struct {
		value any   // 驱动值
		want  int64 // 期望计数
		ok    bool  // 是否成功
	}{
		{int64(3), 3, true},
		{7, 7, true},
		{"9", 9, true},
		{[]byte("11"), 11, true},
		{struct{}{}, 0, false},
	}
	for _, tc := range totalCases {
		got, err := taskCountValue(tc.value)
		// 非法类型必须返回错误
		if tc.ok && (err != nil || got != tc.want) {
			t.Fatalf("计数转换错误: %v %v", got, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("非法计数应失败: %v", tc.value)
		}
	}
	idCases := []struct {
		value any    // 驱动值
		want  string // 期望 ID
		ok    bool   // 是否成功
	}{
		{"t1", "t1", true},
		{[]byte("t2"), "t2", true},
		{struct{}{}, "", false},
	}
	for _, tc := range idCases {
		got, err := taskIDValue(tc.value)
		// 非法类型必须返回错误
		if tc.ok && (err != nil || got != tc.want) {
			t.Fatalf("ID 转换错误: %v %v", got, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("非法 ID 应失败: %v", tc.value)
		}
	}
}

// errRows 是固定返回遍历错误的测试结果集。
type errRows struct{}

// Next 前进到下一行。
// @return 是否有下一行
func (r errRows) Next() bool { return false }

// Scan 扫描当前行。
// @param dest 目标
// @return 错误
func (r errRows) Scan(dest ...any) error { return nil }

// Close 关闭结果集。
// @return 错误
func (r errRows) Close() error { return nil }

// Err 返回遍历错误。
// @return 遍历错误
func (r errRows) Err() error { return errors.New("遍历失败") }

// fixedQueryDB 是固定返回一个结果集的测试数据库。
type fixedQueryDB struct {
	rows Rows // 结果集
}

// ExecContext 执行 SQL。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 结果
// @return 错误
func (d fixedQueryDB) ExecContext(ctx context.Context, query string, args ...any) (Result, error) {
	return fakeResult{}, nil
}

// QueryContext 执行查询。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 结果集
// @return 错误
func (d fixedQueryDB) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	return d.rows, nil
}

// TestQueryTaskCountIterationError 验证计数查询在无行且遍历出错时返回错误。
// @param t 测试上下文
func TestQueryTaskCountIterationError(t *testing.T) {
	_, err := queryTaskCount(context.Background(), fixedQueryDB{rows: errRows{}}, "SELECT 1", nil)
	requireTaskQueryError(t, err, "ROW-0036")
}
