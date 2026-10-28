package mysql

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/flowable-go/flowable/internal/persistence/schema"
)

// TestEnsureSchema 验证初始化、重复执行、版本冲突与数据库错误。
// @param t 测试上下文
// @return 无
func TestEnsureSchema(t *testing.T) {
	tables, _ := schema.Tables("mysql")
	ddl, _ := schema.Load("mysql")
	checksum, _ := schema.Checksum("mysql")
	failure := errors.New("数据库故障")
	// 逐一模拟迁移阶段与启动校验阶段的数据库响应，断言错误不会被吞掉。
	for _, scenario := range []string{"空库未授权", "初始化", "DDL失败", "标记失败", "已有完整库", "版本冲突", "缺少版本", "校验和冲突", "缺少标记", "部分表", "查询失败"} {
		t.Run(scenario, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			count := 0
			if scenario == "已有完整库" || scenario == "版本冲突" || scenario == "缺少版本" || scenario == "校验和冲突" || scenario == "缺少标记" {
				count = len(tables)
			}
			if scenario == "部分表" {
				count = 1
			}
			query := mock.ExpectQuery("SELECT COUNT.*information_schema.tables")
			if scenario == "查询失败" {
				query.WillReturnError(failure)
			} else {
				query.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
			}
			if scenario == "初始化" || scenario == "DDL失败" || scenario == "标记失败" {
				for i, statement := range splitStatements(ddl) {
					expected := mock.ExpectExec(regexp.QuoteMeta(statement))
					if scenario == "DDL失败" && i == 0 {
						expected.WillReturnError(failure)
						break
					}
					expected.WillReturnResult(sqlmock.NewResult(0, 1))
				}
				if scenario != "DDL失败" {
					expected := mock.ExpectExec("INSERT INTO ACT_GE_PROPERTY").WithArgs("flowable-go.schema.checksum", checksum)
					if scenario == "标记失败" {
						expected.WillReturnError(failure)
					} else {
						expected.WillReturnResult(sqlmock.NewResult(0, 1))
					}
				}
			}
			if count == len(tables) {
				version := schema.Version
				if scenario == "版本冲突" {
					version = "old"
				}
				query := mock.ExpectQuery("SELECT VALUE_ FROM ACT_GE_PROPERTY").WithArgs("schema.version")
				if scenario == "缺少版本" {
					query.WillReturnError(failure)
				} else {
					query.WillReturnRows(sqlmock.NewRows([]string{"VALUE_"}).AddRow(version))
				}
				if scenario != "版本冲突" && scenario != "缺少版本" {
					value := checksum
					if scenario == "校验和冲突" {
						value = "old"
					}
					query := mock.ExpectQuery("SELECT VALUE_ FROM ACT_GE_PROPERTY").WithArgs("flowable-go.schema.checksum")
					if scenario == "缺少标记" {
						query.WillReturnRows(sqlmock.NewRows([]string{"VALUE_"}))
					} else {
						query.WillReturnRows(sqlmock.NewRows([]string{"VALUE_"}).AddRow(value))
					}
				}
			}
			err = EnsureSchema(context.Background(), db, scenario != "空库未授权")
			wantSuccess := scenario == "初始化" || scenario == "已有完整库"
			if (err == nil) != wantSuccess {
				t.Fatalf("结果错误: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestSplitStatements 验证内置脚本的注释和空语句处理。
// @param t 测试上下文
// @return 无
func TestSplitStatements(t *testing.T) {
	if got := splitStatements("-- 中文注释\nSELECT 1;\n;SELECT 2;\n"); len(got) != 2 {
		t.Fatalf("拆分错误: %v", got)
	}
}
