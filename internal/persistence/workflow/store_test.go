package workflow

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// fixedTime 固定测试时钟，便于断言 create_time/update_time 自动填充。
var fixedTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// newTestStore 创建基于 sqlmock 的仓储入口。
// @param t 测试上下文
// @param dialect 方言
// @return 仓储入口
// @return 数据库模拟
// @return 连接池
func newTestStore(t *testing.T, dialect string) (*Store, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewStore(db, dialect, WithClock(ClockFunc(func() time.Time { return fixedTime })))
	if err != nil {
		t.Fatal(err)
	}
	return store, mock, db
}

// newRows 构造结果集。
// @param columns 列名
// @param rows 行值
// @return 结果集
func newRows(columns []string, rows [][]driver.Value) *sqlmock.Rows {
	result := sqlmock.NewRows(columns)
	for _, row := range rows {
		result.AddRow(row...)
	}
	return result
}

// TestNewStore 验证仓储入口的参数校验与配置。
// @param t 测试上下文
func TestNewStore(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := NewStore(nil, DialectMySQL); err == nil {
		t.Fatal("空连接池应报错")
	}
	if _, err := NewStore(db, "oracle"); err == nil {
		t.Fatal("不支持方言应报错")
	}
	// 方言大小写与首尾空白应被归一化
	store, err := NewStore(db, " Postgres ")
	if err != nil {
		t.Fatal(err)
	}
	if store.Dialect() != DialectPostgres {
		t.Fatalf("方言归一化失败: %s", store.Dialect())
	}
	if !store.Now().After(fixedTime.Add(-time.Minute)) {
		t.Fatal("默认时钟应返回系统时间")
	}
	// 显式注入固定时钟并忽略空配置
	if _, err := NewStore(db, DialectMySQL, WithClock(nil), WithClock(ClockFunc(func() time.Time { return fixedTime }))); err != nil {
		t.Fatal(err)
	}
}

// TestPlaceholders 验证方言占位符与保留字转义。
// @param t 测试上下文
func TestPlaceholders(t *testing.T) {
	mysql, _, _ := newTestStore(t, DialectMySQL)
	pg, _, _ := newTestStore(t, DialectPostgres)
	if mysql.ph(3) != "?" || mysql.phList(2) != "?, ?" {
		t.Fatal("MySQL 占位符错误")
	}
	if pg.ph(1) != "$1" || pg.ph(3) != "$3" || pg.phList(3) != "$1, $2, $3" {
		t.Fatal("PostgreSQL 占位符错误")
	}
	// condition 列需要按方言转义，普通列保持原样
	if mysql.quoteColumn("condition") != "`condition`" || pg.quoteColumn("condition") != `"condition"` {
		t.Fatal("condition 转义错误")
	}
	if mysql.quoteColumn("task_id") != "task_id" || pg.quoteColumn("task_id") != "task_id" {
		t.Fatal("普通列不应转义")
	}
	// MySQL 相对占位符无需平移；PostgreSQL 整体后移
	if mysql.shiftPlaceholders("id = ? AND used = ?", 3) != "id = ? AND used = ?" {
		t.Fatal("MySQL 平移应无变化")
	}
	if got := pg.shiftPlaceholders("id = $1 AND used = $2", 3); got != "id = $4 AND used = $5" {
		t.Fatalf("PostgreSQL 占位符平移错误: %s", got)
	}
	if got := pg.shiftPlaceholders("id = $1", 0); got != "id = $1" {
		t.Fatalf("零平移应保持原样: %s", got)
	}
}

// TestBuildInClause 验证 IN 子句生成。
// @param t 测试上下文
func TestBuildInClause(t *testing.T) {
	mysql, _, _ := newTestStore(t, DialectMySQL)
	pg, _, _ := newTestStore(t, DialectPostgres)
	clause, args := mysql.buildInClause("task_step_code", 1, []string{"a", "b"})
	if clause != "task_step_code IN (?, ?)" || len(args) != 2 || args[0] != "a" {
		t.Fatalf("MySQL IN 子句错误: %s %v", clause, args)
	}
	clause, args = pg.buildInClause("task_step_code", 3, []string{"a", "b"})
	if clause != "task_step_code IN ($3, $4)" || len(args) != 2 {
		t.Fatalf("PostgreSQL IN 子句错误: %s %v", clause, args)
	}
}

// TestNormalizeValue 验证驱动值归一化。
// @param t 测试上下文
func TestNormalizeValue(t *testing.T) {
	if got := normalizeValue([]byte("文本")); got != "文本" {
		t.Fatalf("字节值应转为字符串: %v", got)
	}
	if got := normalizeValue(int64(3)); got != int64(3) {
		t.Fatalf("整数值应保持原样: %v", got)
	}
}

// TestInsert 验证插入 SQL、参数顺序与公共时间列自动填充。
// @param t 测试上下文
func TestInsert(t *testing.T) {
	store, mock, _ := newTestStore(t, DialectMySQL)
	repo := NewBusinessSystemRepository(store)
	// 参数按表定义顺序输出：id, secret, creator + 自动填充的两个时间列
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO workflow_business_system (id, secret, creator, create_time, update_time) VALUES (?, ?, ?, ?, ?)")).
		WithArgs("sys-1", "secret-1", "张三", fixedTime, fixedTime).
		WillReturnResult(sqlmock.NewResult(1, 1))
	row := Row{"id": "sys-1", "creator": "张三", "secret": "secret-1"}
	if err := repo.Insert(context.Background(), row); err != nil {
		t.Fatal(err)
	}
	// 插入行自带时间列时尊重原值
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO workflow_business_system (id, create_time, update_time) VALUES (?, ?, ?)")).
		WithArgs("sys-2", fixedTime, fixedTime).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := repo.Insert(context.Background(), Row{"id": "sys-2", "create_time": fixedTime, "update_time": fixedTime}); err != nil {
		t.Fatal(err)
	}
	// 错误分支：空行、未知列、缺失 id、执行失败
	if err := repo.Insert(context.Background(), Row{}); err == nil {
		t.Fatal("空行应报错")
	}
	if err := repo.Insert(context.Background(), Row{"id": "x", "不存在的列": 1}); err == nil {
		t.Fatal("未知列应报错")
	}
	if err := repo.Insert(context.Background(), Row{"creator": "x"}); err == nil {
		t.Fatal("缺失 id 应报错")
	}
	mock.ExpectExec("INSERT INTO workflow_business_system").WillReturnError(errors.New("数据库故障"))
	if err := repo.Insert(context.Background(), Row{"id": "sys-3"}); err == nil {
		t.Fatal("执行失败应报错")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestInsertPostgres 验证 PostgreSQL 方言的序号占位符。
// @param t 测试上下文
func TestInsertPostgres(t *testing.T) {
	store, mock, _ := newTestStore(t, DialectPostgres)
	repo := NewBusinessSystemRepository(store)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO workflow_business_system (id, system_symbol, create_time, update_time) VALUES ($1, $2, $3, $4)")).
		WithArgs("sys-1", "oa", fixedTime, fixedTime).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := repo.Insert(context.Background(), Row{"id": "sys-1", "system_symbol": "oa"}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestUpdateByPK 验证按主键更新：SET 列、update_time 填充、主键不可改与错误分支。
// @param t 测试上下文
func TestUpdateByPK(t *testing.T) {
	store, mock, _ := newTestStore(t, DialectMySQL)
	repo := NewTaskDelegateRepository(store)
	// SET 列按表定义顺序：enable, retire, update_time；id 不参与更新
	mock.ExpectExec(regexp.QuoteMeta("UPDATE workflow_task_delegate SET enable = ?, retire = ?, update_time = ? WHERE id = ?")).
		WithArgs(0, 1, fixedTime, "d1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	affected, err := repo.UpdateByPK(context.Background(), "d1", Row{"id": "d1", "retire": 1, "enable": 0})
	if err != nil || affected != 1 {
		t.Fatalf("更新失败: %v %d", err, affected)
	}
	// 错误分支：空主键、空变更、未知列、执行失败
	if _, err := repo.UpdateByPK(context.Background(), "", Row{"retire": 1}); err == nil {
		t.Fatal("空主键应报错")
	}
	if _, err := repo.UpdateByPK(context.Background(), "d1", Row{}); err == nil {
		t.Fatal("空变更应报错")
	}
	if _, err := repo.UpdateByPK(context.Background(), "d1", Row{"未知列": 1}); err == nil {
		t.Fatal("未知列应报错")
	}
	mock.ExpectExec("UPDATE workflow_task_delegate").WillReturnError(errors.New("数据库故障"))
	if _, err := repo.UpdateByPK(context.Background(), "d1", Row{"retire": 1}); err == nil {
		t.Fatal("执行失败应报错")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestUpdatePostgresPlaceholders 验证条件更新时 PostgreSQL 占位符整体平移。
// @param t 测试上下文
func TestUpdatePostgresPlaceholders(t *testing.T) {
	store, mock, _ := newTestStore(t, DialectPostgres)
	repo := NewNextActivityRepository(store)
	// SET 占用 $1..$3，WHERE 相对占位符平移为 $4
	mock.ExpectExec(regexp.QuoteMeta("UPDATE workflow_next_activity SET used = $1, task_id = $2, update_time = $3 WHERE id = $4 AND used = 0")).
		WithArgs(1, "t1", fixedTime, "n1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if _, err := repo.MarkUsed(context.Background(), "n1", "t1"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestDeleteByPK 验证按主键删除与错误分支。
// @param t 测试上下文
func TestDeleteByPK(t *testing.T) {
	store, mock, _ := newTestStore(t, DialectMySQL)
	repo := NewOperationRecordRepository(store)
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM workflow_operation_record WHERE id = ?")).
		WithArgs("r1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	affected, err := repo.DeleteByPK(context.Background(), "r1")
	if err != nil || affected != 1 {
		t.Fatalf("删除失败: %v %d", err, affected)
	}
	if _, err := repo.DeleteByPK(context.Background(), ""); err == nil {
		t.Fatal("空主键应报错")
	}
	mock.ExpectExec("DELETE FROM workflow_operation_record").WillReturnError(errors.New("数据库故障"))
	if _, err := repo.DeleteByPK(context.Background(), "r1"); err == nil {
		t.Fatal("执行失败应报错")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestGetByPK 验证按主键查询：命中、未命中、驱动值转换与错误分支。
// @param t 测试上下文
func TestGetByPK(t *testing.T) {
	store, mock, _ := newTestStore(t, DialectMySQL)
	repo := NewModelRepository(store)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, resource_id, model_name, model_code, tips, target, roles, menu_icon, remark, sort, parent_id, enable, return_sort, is_leaf, process_definition_key, visible, model_id, last_deploy_time, creator, creator_name, create_time, updater, update_time, tenant_id FROM workflow_model WHERE id = ?")).
		WithArgs("m1").
		WillReturnRows(newRows([]string{"id", "model_name", "sort"}, [][]driver.Value{{[]byte("m1"), "请假模型", int64(3)}}))
	row, found, err := repo.GetByPK(context.Background(), "m1")
	if err != nil || !found {
		t.Fatalf("查询失败: %v %v", err, found)
	}
	// 驱动返回的 []byte 应统一转为字符串
	if row["id"] != "m1" || row["model_name"] != "请假模型" || row["sort"] != int64(3) {
		t.Fatalf("行数据错误: %v", row)
	}
	// 未命中返回 (nil, false, nil)
	mock.ExpectQuery("SELECT .* FROM workflow_model").WithArgs("m2").WillReturnRows(newRows([]string{"id"}, nil))
	if _, found, err := repo.GetByPK(context.Background(), "m2"); err != nil || found {
		t.Fatalf("未命中应返回 false: %v %v", err, found)
	}
	// 错误分支：空主键、查询失败
	if _, _, err := repo.GetByPK(context.Background(), ""); err == nil {
		t.Fatal("空主键应报错")
	}
	mock.ExpectQuery("SELECT .* FROM workflow_model").WillReturnError(errors.New("数据库故障"))
	if _, _, err := repo.GetByPK(context.Background(), "m1"); err == nil {
		t.Fatal("查询失败应报错")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestWithTx 验证工作单元的提交、回滚、嵌套复用与错误分支。
// @param t 测试上下文
func TestWithTx(t *testing.T) {
	store, mock, _ := newTestStore(t, DialectMySQL)
	repo := NewBusinessSystemRepository(store)
	ctx := context.Background()
	insertSQL := regexp.QuoteMeta("INSERT INTO workflow_business_system (id, create_time, update_time) VALUES (?, ?, ?)")
	expectInsert := func() {
		mock.ExpectExec(insertSQL).WithArgs("sys-1", fixedTime, fixedTime).WillReturnResult(sqlmock.NewResult(1, 1))
	}
	// 提交路径：事务内写入随事务一起提交
	mock.ExpectBegin()
	expectInsert()
	mock.ExpectCommit()
	if err := store.WithTx(ctx, func(tx context.Context) error {
		return repo.Insert(tx, Row{"id": "sys-1"})
	}); err != nil {
		t.Fatal(err)
	}
	// 回滚路径：fn 返回错误时整体回滚
	mock.ExpectBegin()
	expectInsert()
	mock.ExpectRollback()
	if err := store.WithTx(ctx, func(tx context.Context) error {
		if err := repo.Insert(tx, Row{"id": "sys-1"}); err != nil {
			return err
		}
		return errors.New("业务失败")
	}); err == nil {
		t.Fatal("fn 错误应返回")
	}
	// 嵌套调用复用同一事务，不重复 Begin
	mock.ExpectBegin()
	expectInsert()
	mock.ExpectCommit()
	if err := store.WithTx(ctx, func(tx context.Context) error {
		return store.WithTx(tx, func(inner context.Context) error {
			return repo.Insert(inner, Row{"id": "sys-1"})
		})
	}); err != nil {
		t.Fatal(err)
	}
	// panic 时回滚并向上传播
	mock.ExpectBegin()
	mock.ExpectRollback()
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("panic 应向上传播")
			}
		}()
		_ = store.WithTx(ctx, func(tx context.Context) error { panic("意外错误") })
	}()
	// 错误分支：空函数、开启失败、提交失败
	if err := store.WithTx(ctx, nil); err == nil {
		t.Fatal("空事务函数应报错")
	}
	mock.ExpectBegin().WillReturnError(errors.New("开启失败"))
	if err := store.WithTx(ctx, func(tx context.Context) error { return nil }); err == nil {
		t.Fatal("开启失败应报错")
	}
	mock.ExpectBegin()
	mock.ExpectCommit().WillReturnError(errors.New("提交失败"))
	if err := store.WithTx(ctx, func(tx context.Context) error { return nil }); err == nil {
		t.Fatal("提交失败应报错")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestQueryList 验证多行查询与错误分支。
// @param t 测试上下文
func TestQueryList(t *testing.T) {
	store, mock, _ := newTestStore(t, DialectMySQL)
	repo := NewModelRepository(store)
	ctx := context.Background()
	mock.ExpectQuery("SELECT .* FROM workflow_model WHERE parent_id = \\? ORDER BY sort").
		WithArgs("root").
		WillReturnRows(newRows([]string{"id", "model_name"}, [][]driver.Value{
			{[]byte("m1"), "模型一"},
			{[]byte("m2"), "模型二"},
		}))
	rows, err := repo.ListByParent(ctx, "root")
	if err != nil || len(rows) != 2 {
		t.Fatalf("查询失败: %v %d", err, len(rows))
	}
	if rows[1]["model_name"] != "模型二" {
		t.Fatalf("行顺序错误: %v", rows[1])
	}
	// 空结果返回空切片
	mock.ExpectQuery("SELECT .* FROM workflow_model").WithArgs("root").WillReturnRows(newRows([]string{"id"}, nil))
	if rows, err := repo.ListByParent(ctx, "root"); err != nil || len(rows) != 0 {
		t.Fatalf("空结果错误: %v %d", err, len(rows))
	}
	// 查询失败
	mock.ExpectQuery("SELECT .* FROM workflow_model").WillReturnError(errors.New("数据库故障"))
	if _, err := repo.ListByParent(ctx, "root"); err == nil {
		t.Fatal("查询失败应报错")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
