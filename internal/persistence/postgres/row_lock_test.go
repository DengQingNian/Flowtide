package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestLockedRowsLifecycle 验证所有读写和事务在持锁连接执行，释放可重复调用。
// @param t 测试上下文
// @return 无
func TestLockedRowsLifecycle(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT pg_try_advisory_lock").WithArgs(rowLockKey).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	locked, err := NewLockedRows(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT EXISTS").WithArgs(rowLockKey).WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(true))
	mock.ExpectExec("INSERT").WillReturnResult(sqlmock.NewResult(0, 1))
	if _, err := locked.ExecContext(context.Background(), "INSERT"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT EXISTS").WithArgs(rowLockKey).WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(true))
	mock.ExpectQuery("SELECT ID_").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("id"))
	rows, err := locked.QueryContext(context.Background(), "SELECT ID_")
	if err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	mock.ExpectQuery("SELECT EXISTS").WithArgs(rowLockKey).WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(true))
	mock.ExpectBegin()
	tx, err := locked.BeginTx(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 1))
	if _, err := tx.ExecContext(context.Background(), "UPDATE"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT EXISTS").WithArgs(rowLockKey).WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(true))
	mock.ExpectCommit()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT pg_advisory_unlock").WithArgs(rowLockKey).WillReturnRows(sqlmock.NewRows([]string{"released"}).AddRow(true))
	mock.ExpectClose()
	if err := locked.Release(); err != nil {
		t.Fatal(err)
	}
	if err := locked.Release(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestLockedRowsLostOwnership 验证锁丢失后所有操作均失败，待提交事务必须回滚。
// @param t 测试上下文
// @return 无
func TestLockedRowsLostOwnership(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery("SELECT pg_try_advisory_lock").WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	locked, err := NewLockedRows(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"exec", "query", "begin"} {
		mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(false))
		var err error
		switch operation {
		case "exec":
			_, err = locked.ExecContext(context.Background(), "INSERT")
		case "query":
			_, err = locked.QueryContext(context.Background(), "SELECT")
		case "begin":
			_, err = locked.BeginTx(context.Background())
		}
		if !errors.Is(err, ErrRowLockLost) {
			t.Fatalf("%s 未拒绝丢锁: %v", operation, err)
		}
	}
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(true))
	mock.ExpectBegin()
	tx, err := locked.BeginTx(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(false))
	mock.ExpectRollback()
	if err := tx.Commit(); !errors.Is(err, ErrRowLockLost) {
		t.Fatal("丢锁事务未回滚")
	}
	mock.ExpectQuery("SELECT pg_advisory_unlock").WillReturnRows(sqlmock.NewRows([]string{"released"}).AddRow(false))
	if err := locked.Release(); !errors.Is(err, ErrRowLockLost) {
		t.Fatal("释放丢锁未返回错误")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestLockedRowsFailures 验证空池、取消、锁竞争及连接异常均保守失败。
// @param t 测试上下文
// @return 无
func TestLockedRowsFailures(t *testing.T) {
	if _, err := NewLockedRows(context.Background(), nil); err == nil {
		t.Fatal("空池未拒绝")
	}
	var empty *LockedRows
	if err := empty.Release(); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"取消", "锁竞争", "锁查询失败", "归属查询失败", "开始失败", "释放失败", "显式回滚"} {
		t.Run(scenario, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			ctx := context.Background()
			if scenario == "取消" {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				if _, err := NewLockedRows(cancelled, db); err == nil {
					t.Fatal("取消未失败")
				}
				return
			}
			acquire := mock.ExpectQuery("SELECT pg_try_advisory_lock")
			if scenario == "锁查询失败" {
				acquire.WillReturnError(errors.New("锁查询失败"))
			} else {
				acquire.WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(scenario != "锁竞争"))
			}
			locked, err := NewLockedRows(ctx, db)
			if scenario == "锁竞争" || scenario == "锁查询失败" {
				if err == nil {
					t.Fatal("取锁未失败")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			owner := mock.ExpectQuery("SELECT EXISTS")
			if scenario == "归属查询失败" {
				owner.WillReturnError(errors.New("归属查询失败"))
			} else {
				owner.WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(true))
			}
			if scenario == "开始失败" {
				mock.ExpectBegin().WillReturnError(errors.New("开始失败"))
				if _, err := locked.BeginTx(ctx); err == nil {
					t.Fatal("事务开始未失败")
				}
			} else if scenario == "显式回滚" {
				mock.ExpectBegin()
				tx, err := locked.BeginTx(ctx)
				if err != nil {
					t.Fatal(err)
				}
				mock.ExpectRollback()
				if err := tx.Rollback(); err != nil {
					t.Fatal(err)
				}
			} else if err := locked.CheckOwner(ctx); (err != nil) != (scenario == "归属查询失败") {
				t.Fatalf("归属错误: %v", err)
			}
			release := mock.ExpectQuery("SELECT pg_advisory_unlock")
			if scenario == "释放失败" {
				release.WillReturnError(errors.New("释放失败"))
			} else {
				release.WillReturnRows(sqlmock.NewRows([]string{"released"}).AddRow(true))
			}
			mock.ExpectClose()
			if err := locked.Release(); (err != nil) != (scenario == "释放失败") {
				t.Fatalf("释放错误: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
