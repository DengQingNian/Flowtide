package migration

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestScanMySQL 验证只读扫描保留空表、模式版本和字节副本，并只统计未支持的表。
// @param t 测试上下文
// @return 无
func TestScanMySQL(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT table_name FROM information_schema.tables").WillReturnRows(sqlmock.NewRows([]string{"table_name"}).AddRow("ACT_GE_PROPERTY").AddRow("ACT_GE_BYTEARRAY").AddRow("ACT_RU_TASK").AddRow("ACT_UNKNOWN_JOB"))
	mock.ExpectQuery("SELECT \\* FROM `ACT_GE_PROPERTY`").WillReturnRows(sqlmock.NewRows([]string{"NAME_", "VALUE_"}).AddRow([]byte("schema.version"), []byte("8.1.0.3")))
	mock.ExpectQuery("SELECT \\* FROM `ACT_GE_BYTEARRAY`").WillReturnRows(sqlmock.NewRows([]string{"ID_", "BYTES_", "FULL_MSG_"}).AddRow([]byte("bytes"), []byte{1, 2}, []byte{3}))
	mock.ExpectQuery("SELECT \\* FROM `ACT_RU_TASK`").WillReturnRows(sqlmock.NewRows([]string{"ID_"}))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM `ACT_UNKNOWN_JOB`").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectRollback()
	got, err := Scan(context.Background(), db, "mysql")
	if err != nil {
		t.Fatal(err)
	}
	// 同时验证扫描结果和未知表计数，防止报告静默丢失数据。
	if got.Dialect != "mysql" || got.SchemaVersion != "8.1.0.3" || len(got.Rows) != 2 || len(got.Tables) != 4 {
		t.Fatalf("快照不完整: %+v", got)
	}
	if !reflect.DeepEqual(got.Rows[1].Values["BYTES_"], []byte{1, 2}) || got.Rows[1].Values["ID_"] != "bytes" {
		t.Fatalf("字节归一化错误: %#v", got.Rows[1].Values)
	}
	if got.Tables[2].Count != 0 || got.Tables[3].Count != 2 {
		t.Fatalf("表统计错误: %+v", got.Tables)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestScanPostgres 验证 PostgreSQL 表名称保留引用并规范化报告名称。
// @param t 测试上下文
// @return 无
func TestScanPostgres(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("table_schema = current_schema\\(\\)").WillReturnRows(sqlmock.NewRows([]string{"table_name"}).AddRow("act_ru_task"))
	mock.ExpectQuery(`SELECT \* FROM "act_ru_task"`).WillReturnRows(sqlmock.NewRows([]string{"id_", "tenant_id_"}).AddRow("task", nil))
	mock.ExpectRollback()
	got, err := Scan(context.Background(), db, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	// PostgreSQL 的实际小写名称应转换为分析器统一使用的大写键。
	if got.Tables[0].Name != "ACT_RU_TASK" || got.Rows[0].Values["ID_"] != "task" || got.Rows[0].Values["TENANT_ID_"] != nil {
		t.Fatalf("列名未统一: %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestScanFailures 验证每类扫描失败都会回滚并丢弃部分数据。
// @param t 测试上下文
// @return 无
func TestScanFailures(t *testing.T) {
	tests := []struct {
		name  string
		setup func(sqlmock.Sqlmock)
	}{
		{"开始事务", func(m sqlmock.Sqlmock) { m.ExpectBegin().WillReturnError(errors.New("失败")) }},
		{"发现查询", func(m sqlmock.Sqlmock) {
			m.ExpectBegin()
			m.ExpectQuery("SELECT table_name").WillReturnError(errors.New("失败"))
			m.ExpectRollback()
		}},
		{"发现扫描", func(m sqlmock.Sqlmock) {
			m.ExpectBegin()
			m.ExpectQuery("SELECT table_name").WillReturnRows(sqlmock.NewRows([]string{"table_name"}).AddRow(nil))
			m.ExpectRollback()
		}},
		{"发现遍历", func(m sqlmock.Sqlmock) {
			m.ExpectBegin()
			m.ExpectQuery("SELECT table_name").WillReturnRows(sqlmock.NewRows([]string{"table_name"}).AddRow("ACT_RU_TASK").RowError(0, errors.New("失败")))
			m.ExpectRollback()
		}},
		{"发现关闭", func(m sqlmock.Sqlmock) {
			m.ExpectBegin()
			m.ExpectQuery("SELECT table_name").WillReturnRows(sqlmock.NewRows([]string{"table_name"}).CloseError(errors.New("失败")))
			m.ExpectRollback()
		}},
		{"表名注入", func(m sqlmock.Sqlmock) {
			m.ExpectBegin()
			m.ExpectQuery("SELECT table_name").WillReturnRows(sqlmock.NewRows([]string{"table_name"}).AddRow("ACT_X;DELETE"))
			m.ExpectRollback()
		}},
		{"名称冲突", func(m sqlmock.Sqlmock) {
			m.ExpectBegin()
			m.ExpectQuery("SELECT table_name").WillReturnRows(sqlmock.NewRows([]string{"table_name"}).AddRow("ACT_RU_TASK").AddRow("act_ru_task"))
			m.ExpectRollback()
		}},
		{"读取查询", func(m sqlmock.Sqlmock) {
			expectSourceTable(m, "ACT_RU_TASK")
			m.ExpectQuery("SELECT \\* FROM").WillReturnError(errors.New("失败"))
			m.ExpectRollback()
		}},
		{"读取遍历", func(m sqlmock.Sqlmock) {
			expectSourceTable(m, "ACT_RU_TASK")
			m.ExpectQuery("SELECT \\* FROM").WillReturnRows(sqlmock.NewRows([]string{"ID_"}).AddRow("id").RowError(0, errors.New("失败")))
			m.ExpectRollback()
		}},
		{"读取关闭", func(m sqlmock.Sqlmock) {
			expectSourceTable(m, "ACT_RU_TASK")
			m.ExpectQuery("SELECT \\* FROM").WillReturnRows(sqlmock.NewRows([]string{"ID_"}).CloseError(errors.New("失败")))
			m.ExpectRollback()
		}},
		{"未知统计", func(m sqlmock.Sqlmock) {
			expectSourceTable(m, "ACT_X")
			m.ExpectQuery("SELECT COUNT").WillReturnError(errors.New("失败"))
			m.ExpectRollback()
		}},
		{"结束回滚", func(m sqlmock.Sqlmock) {
			m.ExpectBegin()
			m.ExpectQuery("SELECT table_name").WillReturnRows(sqlmock.NewRows([]string{"table_name"}))
			m.ExpectRollback().WillReturnError(errors.New("失败"))
		}},
	}
	// 每条失败路径独立建立事务，确保回滚且不返回之前成功读取的部分数据。
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			tt.setup(mock)
			got, err := Scan(context.Background(), db, "mysql")
			if err == nil || !reflect.DeepEqual(got, SourceSnapshot{}) {
				t.Fatalf("应失败且无部分快照: %+v %v", got, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// expectSourceTable 配置发现单张源表的共同查询。
// @param mock SQL 模拟器
// @param name 表名
// @return 无
func expectSourceTable(mock sqlmock.Sqlmock, name string) {
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT table_name").WillReturnRows(sqlmock.NewRows([]string{"table_name"}).AddRow(name))
}

// TestScanInvalidArguments 验证无数据库、不支持方言及已取消上下文均拒绝扫描。
// @param t 测试上下文
// @return 无
func TestScanInvalidArguments(t *testing.T) {
	// 参数与取消状态必须在读取源数据前阻断。
	if _, err := Scan(context.Background(), nil, "mysql"); err == nil {
		t.Fatal("空数据库应拒绝")
	}
	db, _, _ := sqlmock.New()
	defer db.Close()
	if _, err := Scan(context.Background(), db, "sqlite"); err == nil {
		t.Fatal("未知方言应拒绝")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Scan(ctx, db, "mysql"); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消错误应保留: %v", err)
	}
}

// TestScanJavaJobs 验证 Java 专属作业读取实体标识而非只有总量。
// @param t 测试上下文
// @return 无
func TestScanJavaJobs(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	expectSourceTable(mock, "ACT_RU_TIMER_JOB")
	mock.ExpectQuery("SELECT \\* FROM `ACT_RU_TIMER_JOB`").WillReturnRows(sqlmock.NewRows([]string{"ID_", "TENANT_ID_"}).AddRow("timer", "tenant"))
	mock.ExpectRollback()
	got, err := Scan(context.Background(), db, "mysql")
	if err != nil {
		t.Fatal(err)
	}
	// 专属作业保持实体标识，供分析器定位需要转换的行。
	if len(got.Rows) != 1 || got.Rows[0].Values["ID_"] != "timer" {
		t.Fatalf("缺少作业实体: %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
