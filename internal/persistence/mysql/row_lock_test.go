package mysql

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// newLockedRowsMock 创建已授予独占锁的行数据库与 SQL 替身。
// @param t 测试上下文
// @return 行数据库
// @return SQL 替身
// @return 连接池
func newLockedRowsMock(t *testing.T) (*LockedRows, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT GET_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(1))
	locked, err := NewLockedRows(context.Background(), db, "测试库")
	if err != nil {
		t.Fatal(err)
	}
	return locked, mock, db
}

// TestLockedRowsOperations 验证读写与事务在持锁期间正常执行并校验独占权。
// @param t 测试上下文
// @return 无
func TestLockedRowsOperations(t *testing.T) {
	locked, mock, db := newLockedRowsMock(t)
	defer db.Close()
	ctx := context.Background()
	mock.ExpectQuery("SELECT IS_USED_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(true))
	if err := locked.CheckOwner(ctx); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT IS_USED_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(true))
	mock.ExpectExec("INSERT INTO ACT_RU_TASK").WithArgs("id").WillReturnResult(sqlmock.NewResult(0, 1))
	result, err := locked.ExecContext(ctx, "INSERT INTO ACT_RU_TASK (ID_) VALUES (?)", "id")
	if err != nil {
		t.Fatal(err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		t.Fatalf("影响行数=%d 错误=%v", affected, err)
	}
	mock.ExpectQuery("SELECT IS_USED_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(true))
	mock.ExpectQuery("SELECT ID_ FROM ACT_RU_TASK").WillReturnRows(sqlmock.NewRows([]string{"ID_"}).AddRow("id"))
	rows, err := locked.QueryContext(ctx, "SELECT ID_ FROM ACT_RU_TASK")
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatal("应有一行")
	}
	var id string
	if err := rows.Scan(&id); err != nil || id != "id" {
		t.Fatalf("读取=%s 错误=%v", id, err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT IS_USED_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(true))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE ACT_RU_TASK").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT IS_USED_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(true))
	mock.ExpectCommit()
	tx, err := locked.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE ACT_RU_TASK SET NAME_ = ?", "x"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT IS_USED_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(true))
	mock.ExpectBegin()
	mock.ExpectRollback()
	tx, err = locked.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT IS_USED_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(true))
	mock.ExpectBegin().WillReturnError(errors.New("开始事务失败"))
	if _, err := locked.BeginTx(ctx); err == nil {
		t.Fatal("开始事务失败应返回错误")
	}
	mock.ExpectQuery("SELECT RELEASE_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"released"}).AddRow(1))
	if err := locked.Release(); err != nil {
		t.Fatal(err)
	}
	if err := locked.Release(); err != nil {
		t.Fatal("重复释放应返回首次结果")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestLockedRowsRejectsLostLock 验证独占权丢失后拒绝读写并回滚提交。
// @param t 测试上下文
// @return 无
func TestLockedRowsRejectsLostLock(t *testing.T) {
	locked, mock, db := newLockedRowsMock(t)
	defer db.Close()
	ctx := context.Background()
	mock.ExpectQuery("SELECT IS_USED_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(false))
	if _, err := locked.ExecContext(ctx, "INSERT INTO ACT_RU_TASK (ID_) VALUES (?)", "id"); !errors.Is(err, ErrRowLockLost) {
		t.Fatalf("写入错误=%v", err)
	}
	mock.ExpectQuery("SELECT IS_USED_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(false))
	if _, err := locked.QueryContext(ctx, "SELECT ID_ FROM ACT_RU_TASK"); !errors.Is(err, ErrRowLockLost) {
		t.Fatalf("查询错误=%v", err)
	}
	mock.ExpectQuery("SELECT IS_USED_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(false))
	if _, err := locked.BeginTx(ctx); !errors.Is(err, ErrRowLockLost) {
		t.Fatalf("开始事务错误=%v", err)
	}
	mock.ExpectQuery("SELECT IS_USED_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(true))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT IS_USED_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(false))
	mock.ExpectRollback()
	tx, err := locked.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); !errors.Is(err, ErrRowLockLost) {
		t.Fatalf("提交错误=%v", err)
	}
	mock.ExpectQuery("SELECT RELEASE_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"released"}).AddRow(1))
	if err := locked.Release(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestLockedRowsAcquireErrors 验证空连接池、取消上下文及取锁失败均拒绝。
// @param t 测试上下文
// @return 无
func TestLockedRowsAcquireErrors(t *testing.T) {
	if _, err := NewLockedRows(context.Background(), nil, "测试库"); err == nil {
		t.Fatal("空连接池应失败")
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewLockedRows(canceled, db, "测试库"); err == nil {
		t.Fatal("取消连接应失败")
	}
	for _, setup := range []func(sqlmock.Sqlmock){
		func(m sqlmock.Sqlmock) {
			m.ExpectQuery("SELECT GET_LOCK").WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(0))
		},
		func(m sqlmock.Sqlmock) {
			m.ExpectQuery("SELECT GET_LOCK").WillReturnError(errors.New("查询失败"))
		},
	} {
		setup(mock)
		if _, err := NewLockedRows(context.Background(), db, "测试库"); err == nil {
			t.Fatal("取锁失败应拒绝")
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestLockedRowsCheckError 验证所有权查询失败时返回连接错误。
// @param t 测试上下文
// @return 无
func TestLockedRowsCheckError(t *testing.T) {
	locked, mock, db := newLockedRowsMock(t)
	defer db.Close()
	mock.ExpectQuery("SELECT IS_USED_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnError(errors.New("连接失败"))
	if err := locked.CheckOwner(context.Background()); err == nil {
		t.Fatal("查询失败应返回错误")
	}
	mock.ExpectQuery("SELECT RELEASE_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"released"}).AddRow(1))
	if err := locked.Release(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestLockedRowsReleaseErrors 验证释放失败或锁已不属于本连接时返回错误。
// @param t 测试上下文
// @return 无
func TestLockedRowsReleaseErrors(t *testing.T) {
	locked, mock, db := newLockedRowsMock(t)
	defer db.Close()
	mock.ExpectQuery("SELECT RELEASE_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnError(errors.New("释放失败"))
	if err := locked.Release(); err == nil {
		t.Fatal("释放失败应返回错误")
	}
	if err := locked.Release(); err == nil {
		t.Fatal("重复释放应返回首次错误")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	second, secondMock, secondDB := newLockedRowsMock(t)
	defer secondDB.Close()
	secondMock.ExpectQuery("SELECT RELEASE_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"released"}).AddRow(nil))
	if err := second.Release(); !errors.Is(err, ErrRowLockLost) {
		t.Fatalf("锁已丢失错误=%v", err)
	}
	if err := secondMock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestCheckNoSnapshot 验证快照标记检查的正常与拒绝路径。
// @param t 测试上下文
// @return 无
func TestCheckNoSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT COUNT").WithArgs(snapshotStateName).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	if err := CheckNoSnapshot(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT COUNT").WithArgs(snapshotStateName).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	if err := CheckNoSnapshot(context.Background(), db); err == nil {
		t.Fatal("存在快照时必须拒绝")
	}
	mock.ExpectQuery("SELECT COUNT").WithArgs(snapshotStateName).WillReturnError(errors.New("查询失败"))
	if err := CheckNoSnapshot(context.Background(), db); err == nil {
		t.Fatal("查询失败应返回错误")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
