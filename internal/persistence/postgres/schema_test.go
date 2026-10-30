package postgres

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/DengQingNian/Flowtide/internal/persistence/schema"
)

// TestEnsureSchema 验证 PostgreSQL 全量初始化原子提交、重复校验与损坏拒绝。
// @param t 测试上下文
// @return 无
func TestEnsureSchema(t *testing.T) {
	tables, _ := schema.Tables("postgres")
	checksum, _ := schema.Checksum("postgres")
	for _, scenario := range []string{"初始化", "已有库", "部分表", "未授权", "表查询失败", "版本错误", "标记缺失", "开始失败", "DDL失败", "标记失败", "提交失败"} {
		t.Run(scenario, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			count := 0
			if scenario == "已有库" || scenario == "版本错误" || scenario == "标记缺失" {
				count = len(tables)
			}
			if scenario == "部分表" {
				count = 1
			}
			query := mock.ExpectQuery("SELECT COUNT")
			if scenario == "表查询失败" {
				query.WillReturnError(errors.New("查询失败"))
			} else {
				query.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
			}
			if count == len(tables) {
				version := schema.Version
				if scenario == "版本错误" {
					version = "bad"
				}
				mock.ExpectQuery("SELECT VALUE_").WithArgs("schema.version").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(version))
				if scenario != "版本错误" {
					mark := mock.ExpectQuery("SELECT VALUE_").WithArgs(checksumKey)
					if scenario == "标记缺失" {
						mark.WillReturnError(errors.New("缺失"))
					} else {
						mark.WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(checksum))
					}
				}
			}
			initialize := scenario != "未授权"
			if count == 0 && initialize && scenario != "表查询失败" {
				begin := mock.ExpectBegin()
				if scenario == "开始失败" {
					begin.WillReturnError(errors.New("开始失败"))
				} else {
					ddl, _ := schema.Load("postgres")
					for i, statement := range splitStatements(ddl) {
						exec := mock.ExpectExec(regexp.QuoteMeta(statement))
						if scenario == "DDL失败" && i == 0 {
							exec.WillReturnError(errors.New("DDL失败"))
							break
						}
						exec.WillReturnResult(sqlmock.NewResult(0, 0))
					}
					if scenario != "DDL失败" {
						mark := mock.ExpectExec("INSERT INTO ACT_GE_PROPERTY").WithArgs(checksumKey, checksum)
						if scenario == "标记失败" {
							mark.WillReturnError(errors.New("标记失败"))
						} else {
							mark.WillReturnResult(sqlmock.NewResult(0, 1))
						}
					}
					if scenario == "DDL失败" || scenario == "标记失败" {
						mock.ExpectRollback()
					} else if scenario == "提交失败" {
						mock.ExpectCommit().WillReturnError(errors.New("提交失败"))
					} else {
						mock.ExpectCommit()
					}
				}
			}
			if err := EnsureSchema(context.Background(), db, initialize); (err == nil) != (scenario == "初始化" || scenario == "已有库") {
				t.Fatalf("Schema结果: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
