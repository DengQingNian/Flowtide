package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestParseStorageMode 验证存储类型选择及状态文件互斥规则。
// @param t 测试上下文
// @return 无
func TestParseStorageMode(t *testing.T) {
	cases := []struct {
		name      string // 场景
		storage   string // 存储类型
		statePath string // 状态文件
		rowMode   bool   // 期望行模式
		wantErr   bool   // 是否期望错误
	}{
		{"默认存储", "", "state.json", false, false},
		{"快照模式", "mysql", "", false, false},
		{"快照与状态文件互斥", "mysql", "state.json", false, true},
		{"行仓储模式", "mysql-rows", "", true, false},
		{"行仓储与状态文件互斥", "mysql-rows", "state.json", false, true},
		{"PG行仓储", "postgres-rows", "", true, false},
		{"PG行仓储与文件互斥", "postgres-rows", "state.json", false, true},
		{"非法模式", "postgres", "", false, true},
	}
	for _, scenario := range cases {
		t.Run(scenario.name, func(t *testing.T) {
			rowMode, err := parseStorageMode(scenario.storage, scenario.statePath)
			if (err != nil) != scenario.wantErr || rowMode != scenario.rowMode {
				t.Fatalf("行模式=%v 错误=%v", rowMode, err)
			}
		})
	}
}

// TestNewStorageDefault 验证未选择数据库时保留原有内存或文件模式。
// @param t 测试上下文
// @return 无
func TestNewStorageDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	setup, err := newStorage(context.Background(), "", "state.json", path)
	if err != nil {
		t.Fatal(err)
	}
	if setup.Store != nil || setup.Repository != nil || setup.LoadRows {
		t.Fatal("默认模式不应创建数据库存储")
	}
	if err := setup.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestNewStorageRejectsInvalidConfig 验证非法模式及互斥参数在连接前失败。
// @param t 测试上下文
// @return 无
func TestNewStorageRejectsInvalidConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	for _, scenario := range []struct{ storage, statePath string }{
		{"unknown", ""},
		{"mysql", "state.json"},
		{"mysql-rows", "state.json"},
	} {
		if _, err := newStorage(context.Background(), scenario.storage, scenario.statePath, path); err == nil {
			t.Fatalf("配置 %+v 应失败", scenario)
		}
	}
}

// TestNewStorageConnectionError 验证两种 MySQL 模式在连接失败时返回错误。
// @param t 测试上下文
// @return 无
func TestNewStorageConnectionError(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	for key, value := range map[string]string{
		"db_mysql_host":    "127.0.0.1",
		"db_mysql_port":    "1",
		"db_mysql_user":    "test",
		"db_mysql_pwd":     "",
		"db_mysql_db_name": "test",
	} {
		t.Setenv(key, value)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, storage := range []string{"mysql", "mysql-rows"} {
		if _, err := newStorage(ctx, storage, "", path); err == nil {
			t.Fatalf("%s 连接取消应失败", storage)
		}
	}
}

// TestNewRowStorage 验证行仓储初始化、快照拒绝与取锁失败路径。
// @param t 测试上下文
// @return 无
func TestNewRowStorage(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT GET_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(1))
	mock.ExpectQuery("SELECT COUNT").WithArgs("flowable-go-state").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	setup, err := newRowStorage(ctx, db, "测试库")
	if err != nil {
		t.Fatal(err)
	}
	if setup.Repository == nil || !setup.LoadRows || setup.Store != nil {
		t.Fatalf("行仓储初始化结果错误: %+v", setup)
	}
	mock.ExpectQuery("SELECT RELEASE_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"released"}).AddRow(1))
	mock.ExpectClose() // 持锁连接
	mock.ExpectClose() // 查询快照标记使用的连接池连接
	if err := setup.Close(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	snapshotDB, snapshotMock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	snapshotMock.ExpectQuery("SELECT GET_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(1))
	snapshotMock.ExpectQuery("SELECT COUNT").WithArgs("flowable-go-state").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	snapshotMock.ExpectQuery("SELECT RELEASE_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"released"}).AddRow(1))
	snapshotMock.ExpectClose() // 持锁连接
	snapshotMock.ExpectClose() // 查询快照标记使用的连接池连接
	if _, err := newRowStorage(ctx, snapshotDB, "测试库"); err == nil {
		t.Fatal("存在快照数据时必须拒绝")
	}
	if err := snapshotMock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	lockDB, lockMock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	lockMock.ExpectQuery("SELECT GET_LOCK").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(0))
	lockMock.ExpectClose()
	if _, err := newRowStorage(ctx, lockDB, "测试库"); err == nil {
		t.Fatal("取锁失败必须拒绝")
	}
	if err := lockMock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestStorageSetupClose 验证资源释放只执行一次并保留首次错误。
// @param t 测试上下文
// @return 无
func TestStorageSetupClose(t *testing.T) {
	var calls int
	setup := &storageSetup{closeFn: func() error {
		calls++
		return nil
	}}
	if err := setup.Close(); err != nil {
		t.Fatal(err)
	}
	if err := setup.Close(); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("释放次数=%d", calls)
	}
	want := errors.New("释放失败")
	failing := &storageSetup{closeFn: func() error { return want }}
	if err := failing.Close(); !errors.Is(err, want) {
		t.Fatalf("首次错误=%v", err)
	}
	if err := failing.Close(); !errors.Is(err, want) {
		t.Fatalf("重复调用应返回首次错误: %v", err)
	}
	if err := (&storageSetup{}).Close(); err != nil {
		t.Fatal(err)
	}
}
