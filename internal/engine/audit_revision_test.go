package engine

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/flowable-go/flowable/api"
	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
)

// expectAuditModelUpdate 要求更新使用数据库当前修订号，并模拟提交成功或失败。
// @param mock SQL 断言器
// @param revision 数据库当前版本
// @param commitError 提交错误
// @return 无
func expectAuditModelUpdate(mock sqlmock.Sqlmock, revision int64, commitError error) {
	mock.ExpectBegin()
	args := make([]driver.Value, 0, 14)
	// 模型有十二个可修改列，主键和旧修订号必须位于末尾。
	for i := 0; i < 12; i++ {
		args = append(args, sqlmock.AnyArg())
	}
	args = append(args, sqlmock.AnyArg(), revision)
	update := mock.ExpectExec("UPDATE ACT_RE_MODEL SET").WithArgs(args...).WillReturnResult(sqlmock.NewResult(0, 1))
	_ = update
	if commitError != nil {
		mock.ExpectCommit().WillReturnError(commitError)
	} else {
		mock.ExpectCommit()
	}
}

// TestAuditConsecutiveRowUpdates 验证连续更新、提交失败和恢复后保留真实修订号。
// @param t 测试上下文
// @return 无
func TestAuditConsecutiveRowUpdates(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repository, _ := flowablerow.NewRepository(flowablerow.SQLDB{DB: db}, "postgres")
	app, err := New(Config{RowRepository: repository})
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO ACT_RE_MODEL").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	model, err := app.CreateModel(context.Background(), api.CreateModelRequest{Name: "初始", Key: "model"})
	if err != nil {
		t.Fatal(err)
	}
	for revision := int64(1); revision <= 3; revision++ {
		name := model.Name + string(rune('一'+revision))
		expectAuditModelUpdate(mock, revision, nil)
		if _, err := app.UpdateModel(context.Background(), model.ID, api.ModelUpdate{Name: &name}); err != nil {
			t.Fatalf("连续更新第 %d 次失败: %v", revision, err)
		}
	}
	name := "提交失败"
	expectAuditModelUpdate(mock, 4, errors.New("提交失败"))
	if _, err := app.UpdateModel(context.Background(), model.ID, api.ModelUpdate{Name: &name}); err == nil {
		t.Fatal("提交失败应报错")
	}
	expectAuditModelUpdate(mock, 4, nil)
	name = "重试成功"
	if _, err := app.UpdateModel(context.Background(), model.ID, api.ModelUpdate{Name: &name}); err != nil {
		t.Fatal(err)
	}
	rows, err := app.SnapshotRows(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for i := range rows {
		if rows[i].Table == "ACT_RE_MODEL" {
			rows[i].Values["REV_"] = int64(9)
		}
	}
	restored := newTestEngine(t)
	if err := restored.RestoreRows(context.Background(), rows); err != nil {
		t.Fatal(err)
	}
	restored.rowRepository = repository
	restored.lastRows = rows
	expectAuditModelUpdate(mock, 9, nil)
	name = "恢复后更新"
	if _, err := restored.UpdateModel(context.Background(), model.ID, api.ModelUpdate{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
