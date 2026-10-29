package main

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestNewPostgresRowStorage 验证标准服务接入独占行仓储并拒绝快照混用。
// @param t 测试上下文
// @return 无
func TestNewPostgresRowStorage(t *testing.T) {
	for _, scenario := range []string{"正常", "存在快照", "查询失败", "无计数行"} {
		t.Run(scenario, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			mock.ExpectQuery("SELECT pg_try_advisory_lock").WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
			mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(true))
			query := mock.ExpectQuery("SELECT COUNT").WithArgs("flowable-go-state")
			if scenario == "查询失败" {
				query.WillReturnError(errors.New("查询失败"))
			} else if scenario == "无计数行" {
				query.WillReturnRows(sqlmock.NewRows([]string{"count"}))
			} else {
				count := 0
				if scenario == "存在快照" {
					count = 1
				}
				query.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
			}
			mock.ExpectQuery("SELECT pg_advisory_unlock").WillReturnRows(sqlmock.NewRows([]string{"released"}).AddRow(true))
			mock.ExpectClose()
			setup, err := newPostgresRowStorage(context.Background(), db)
			if scenario == "正常" {
				if err != nil || setup.Repository == nil || !setup.LoadRows || setup.Store != nil {
					t.Fatalf("PG 服务配置: %+v %v", setup, err)
				}
				if err := setup.Close(); err != nil {
					t.Fatal(err)
				}
				if err := setup.Close(); err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("不安全存储未拒绝")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
