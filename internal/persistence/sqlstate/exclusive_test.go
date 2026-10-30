package sqlstate

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestLockName 验证快照与行仓储共用的锁名规则稳定且不超过 MySQL 限制。
// @param t 测试上下文
// @return 无
func TestLockName(t *testing.T) {
	want := fmt.Sprintf("flowtide:%x", sha256.Sum256([]byte("开发库")))[:64]
	if got := LockName("开发库"); got != want {
		t.Fatalf("锁名=%s，期望=%s", got, want)
	}
	if len(LockName("开发库")) != 64 {
		t.Fatal("锁名长度必须为 64")
	}
	if LockName("库一") == LockName("库二") {
		t.Fatal("不同数据库必须得到不同锁名")
	}
}

// TestExclusiveMySQL 验证占用拒绝、所有权检查和关闭释放。
// @param t 测试上下文
// @return 无
func TestExclusiveMySQL(t *testing.T) {
	for _, occupied := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		granted := 1
		if occupied {
			granted = 0
		}
		mock.ExpectQuery("SELECT GET_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(granted))
		store, release, err := NewExclusiveMySQL(context.Background(), db, "开发库")
		if occupied {
			if err == nil {
				t.Fatal("其他进程已占用时必须拒绝")
			}
		} else {
			if err != nil {
				t.Fatal(err)
			}
			mock.ExpectQuery("SELECT IS_USED_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"owner"}).AddRow(true))
			if err := store.CheckOwner(context.Background()); err != nil {
				t.Fatal(err)
			}
			mock.ExpectQuery("SELECT IS_USED_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"owner"}).AddRow(false))
			if err := store.Save(context.Background(), []byte("无权写入")); err == nil {
				t.Fatal("丢失独占权后必须拒绝写入")
			}
			mock.ExpectExec("SELECT RELEASE_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
			if err := release(); err != nil {
				t.Fatal(err)
			}
			if err := release(); err != nil {
				t.Fatal(err)
			}
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		_ = db.Close()
	}
}

// TestExclusiveMySQLErrors 验证连接取消及锁查询故障会关闭已借连接。
// @param t 测试上下文
// @return 无
func TestExclusiveMySQLErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := NewExclusiveMySQL(canceled, db, "test"); err == nil {
		t.Fatal("取消连接应失败")
	}
	mock.ExpectQuery("SELECT GET_LOCK").WillReturnError(errors.New("查询失败"))
	if _, _, err := NewExclusiveMySQL(context.Background(), db, "test"); err == nil {
		t.Fatal("锁查询失败应拒绝")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
