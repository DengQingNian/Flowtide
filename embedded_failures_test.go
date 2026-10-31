package flowtide_test

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/DengQingNian/Flowtide"
	flowablerow "github.com/DengQingNian/Flowtide/internal/persistence/flowable"
	"github.com/DengQingNian/Flowtide/internal/persistence/schema"
	"github.com/DengQingNian/Flowtide/internal/persistence/sqlstate"
)

// expectEmbeddedSchema 为失败路径测试提供完整结构标记，确保不会在校验阶段触发任何 DDL。
// @param t 测试上下文
// @param mock SQL 预期
// @return 无
func expectEmbeddedSchema(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	tables, err := schema.Tables("mysql")
	if err != nil {
		t.Fatal(err)
	}
	checksum, err := schema.Checksum("mysql")
	if err != nil {
		t.Fatal(err)
	}
	workflowTables, err := schema.FileTables("mysql", schema.WorkflowFile)
	if err != nil {
		t.Fatal(err)
	}
	workflowChecksum, err := schema.ChecksumFile("mysql", schema.WorkflowFile)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(len(tables)))
	mock.ExpectQuery("SELECT VALUE_").WithArgs("schema.version").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(schema.Version))
	mock.ExpectQuery("SELECT VALUE_").WithArgs("flowtide.schema.checksum").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(checksum))
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(len(workflowTables)))
	mock.ExpectQuery("SELECT VALUE_").WithArgs("flowtide.workflow.schema.checksum").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(workflowChecksum))
}

// TestEmbeddedSQLCloseFailure 验证 SQL 嵌入装配成功后保存失败仍释放写者锁，错误可见且关闭幂等。
// @param t 测试上下文
// @return 无
func TestEmbeddedSQLCloseFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT DATABASE").WillReturnRows(sqlmock.NewRows([]string{"db"}).AddRow("db"))
	expectEmbeddedSchema(t, mock)
	mock.ExpectQuery("SELECT GET_LOCK").WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(1))
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	for _, name := range flowablerow.Tables() {
		mock.ExpectQuery("SELECT IS_USED_LOCK").WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(true))
		mock.ExpectQuery("SELECT .* FROM " + name).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	}
	app, err := flowtide.NewEmbedded(context.Background(), flowtide.EmbeddedConfig{DB: db, Database: "db"})
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT IS_USED_LOCK").WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(true))
	mock.ExpectBegin().WillReturnError(errors.New("保存事务不可用"))
	mock.ExpectQuery("SELECT RELEASE_LOCK").WillReturnRows(sqlmock.NewRows([]string{"released"}).AddRow(1))
	first := app.Close(context.Background())
	if first == nil || app.Close(context.Background()) != first {
		t.Fatal("关闭错误未保留或重复执行")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestEmbeddedSQLFailureCleanup 验证数据库身份、连接池、结构、写者锁、快照和恢复失败均拒绝启动。
// @param t 测试上下文
// @return 无
func TestEmbeddedSQLFailureCleanup(t *testing.T) {
	for _, stage := range []string{"database-query", "database-mismatch", "pool-size", "schema", "lock", "snapshot", "restore"} {
		t.Run(stage, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			failure := errors.New("模拟数据库故障")
			switch stage {
			case "database-query":
				mock.ExpectQuery("SELECT DATABASE").WillReturnError(failure)
			case "database-mismatch":
				mock.ExpectQuery("SELECT DATABASE").WillReturnRows(sqlmock.NewRows([]string{"database"}).AddRow("other"))
			default:
				mock.ExpectQuery("SELECT DATABASE").WillReturnRows(sqlmock.NewRows([]string{"database"}).AddRow("db"))
				if stage == "pool-size" {
					db.SetMaxOpenConns(1)
					break
				}
				if stage == "schema" {
					mock.ExpectQuery("SELECT COUNT").WillReturnError(failure)
					break
				}
				expectEmbeddedSchema(t, mock)
				if stage == "lock" {
					mock.ExpectQuery("SELECT GET_LOCK").WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(0))
					break
				}
				mock.ExpectQuery("SELECT GET_LOCK").WithArgs(sqlstate.LockName("db")).WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(1))
				if stage == "snapshot" {
					mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				} else {
					mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
					mock.ExpectQuery("SELECT IS_USED_LOCK").WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(true))
					mock.ExpectQuery("SELECT").WillReturnError(failure)
				}
				mock.ExpectQuery("SELECT RELEASE_LOCK").WillReturnRows(sqlmock.NewRows([]string{"released"}).AddRow(1))
			}
			app, err := flowtide.NewEmbedded(context.Background(), flowtide.EmbeddedConfig{DB: db, Database: "db"})
			if err == nil {
				_ = app.Close(context.Background())
				t.Fatal("故障后仍装配成功")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
