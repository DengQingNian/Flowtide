package flowable

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestSQLDBAdapter 验证真实 SQL 接口适配以及事务内写入失败回滚。
// @param t 测试上下文
// @return 无
func TestSQLDBAdapter(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	adapter := SQLDB{DB: db}
	mock.ExpectExec("INSERT").WillReturnResult(sqlmock.NewResult(0, 1))
	if _, err := adapter.ExecContext(context.Background(), "INSERT"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"ID_"}).AddRow("id"))
	rows, err := adapter.QueryContext(context.Background(), "SELECT")
	if err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	repository, _ := NewRepository(adapter, "postgres")
	row := auditRow(t, "ACT_GE_PROPERTY", map[string]any{"NAME_": "property", "VALUE_": "a", "REV_": 1})
	mock.ExpectBegin()
	mock.ExpectExec("INSERT").WillReturnError(errors.New("写入失败"))
	mock.ExpectRollback()
	if err := repository.Apply(context.Background(), []Operation{{Kind: OperationInsert, Row: row}}); err == nil {
		t.Fatal("事务写入失败应报错")
	}
	mock.ExpectBegin().WillReturnError(errors.New("事务创建失败"))
	if _, err := adapter.BeginTx(context.Background()); err == nil {
		t.Fatal("开始事务错误应保留")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
