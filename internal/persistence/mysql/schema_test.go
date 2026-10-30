package mysql

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/DengQingNian/Flowtide/internal/persistence/schema"
)

// TestEnsureSchema 验证引擎（001）与 workflow（002）两段初始化、重复执行、版本冲突与数据库错误。
// @param t 测试上下文
// @return 无
func TestEnsureSchema(t *testing.T) {
	engineTables, _ := schema.Tables("mysql")
	engineDDL, _ := schema.Load("mysql")
	engineChecksum, _ := schema.Checksum("mysql")
	wfTables, _ := schema.FileTables("mysql", schema.WorkflowFile)
	wfDDL, _ := schema.LoadFile("mysql", schema.WorkflowFile)
	wfChecksum, _ := schema.ChecksumFile("mysql", schema.WorkflowFile)
	failure := errors.New("数据库故障")
	// 逐一模拟迁移阶段与启动校验阶段的数据库响应，断言错误不会被吞掉。
	for _, scenario := range []string{
		"空库未授权", "初始化", "引擎DDL失败", "引擎标记失败", "引擎部分表", "查询失败",
		"版本冲突", "缺少版本", "校验和冲突", "缺少标记",
		"已有完整库", "workflow初始化", "workflowDDL失败", "workflow标记失败",
		"workflow部分表", "workflow未初始化", "workflow标记冲突", "workflow缺少标记", "workflow查询失败",
	} {
		t.Run(scenario, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			engineCount := 0
			switch scenario {
			case "已有完整库", "版本冲突", "缺少版本", "校验和冲突", "缺少标记", "workflow部分表", "workflow未初始化", "workflow初始化", "workflowDDL失败", "workflow标记失败", "workflow标记冲突", "workflow缺少标记", "workflow查询失败":
				engineCount = len(engineTables)
			case "引擎部分表":
				engineCount = 1
			}
			wfCount := 0
			switch scenario {
			case "已有完整库", "workflow标记冲突", "workflow缺少标记":
				wfCount = len(wfTables)
			case "workflow部分表":
				wfCount = 1
			}
			// 引擎表计数查询
			engineQuery := mock.ExpectQuery("SELECT COUNT.*information_schema.tables")
			if scenario == "查询失败" {
				engineQuery.WillReturnError(failure)
			} else {
				engineQuery.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(engineCount))
			}
			// 引擎初始化（空库且允许初始化时执行 001 脚本）
			switch scenario {
			case "初始化":
				expectScriptExec(mock, engineDDL, -1, checksumKey, engineChecksum, false)
			case "引擎DDL失败":
				expectScriptExec(mock, engineDDL, 0, checksumKey, engineChecksum, false)
			case "引擎标记失败":
				expectScriptExec(mock, engineDDL, -1, checksumKey, engineChecksum, true)
			}
			// 引擎完整库时校验版本与引擎标记
			if engineCount == len(engineTables) {
				version := schema.Version
				if scenario == "版本冲突" {
					version = "old"
				}
				versionQuery := mock.ExpectQuery("SELECT VALUE_ FROM ACT_GE_PROPERTY").WithArgs("schema.version")
				if scenario == "缺少版本" {
					versionQuery.WillReturnError(failure)
				} else {
					versionQuery.WillReturnRows(sqlmock.NewRows([]string{"VALUE_"}).AddRow(version))
				}
				if scenario != "版本冲突" && scenario != "缺少版本" {
					value := engineChecksum
					if scenario == "校验和冲突" {
						value = "old"
					}
					markQuery := mock.ExpectQuery("SELECT VALUE_ FROM ACT_GE_PROPERTY").WithArgs(checksumKey)
					if scenario == "缺少标记" {
						markQuery.WillReturnError(failure)
					} else {
						markQuery.WillReturnRows(sqlmock.NewRows([]string{"VALUE_"}).AddRow(value))
					}
				}
			}
			// 引擎部分就绪后才进入 workflow 段：计数、校验或补跑 002
			if engineReady(scenario, engineCount, len(engineTables)) {
				wfQuery := mock.ExpectQuery("SELECT COUNT.*information_schema.tables")
				if scenario == "workflow查询失败" {
					wfQuery.WillReturnError(failure)
				} else {
					wfQuery.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(wfCount))
				}
				switch scenario {
				case "初始化", "workflow初始化":
					expectScriptExec(mock, wfDDL, -1, workflowChecksumKey, wfChecksum, false)
				case "workflowDDL失败":
					expectScriptExec(mock, wfDDL, 0, workflowChecksumKey, wfChecksum, false)
				case "workflow标记失败":
					expectScriptExec(mock, wfDDL, -1, workflowChecksumKey, wfChecksum, true)
				}
				if wfCount == len(wfTables) {
					value := wfChecksum
					if scenario == "workflow标记冲突" {
						value = "old"
					}
					markQuery := mock.ExpectQuery("SELECT VALUE_ FROM ACT_GE_PROPERTY").WithArgs(workflowChecksumKey)
					if scenario == "workflow缺少标记" {
						markQuery.WillReturnError(failure)
					} else {
						markQuery.WillReturnRows(sqlmock.NewRows([]string{"VALUE_"}).AddRow(value))
					}
				}
			}
			initialize := scenario != "空库未授权" && scenario != "workflow未初始化"
			err = EnsureSchema(context.Background(), db, initialize)
			wantSuccess := scenario == "初始化" || scenario == "已有完整库" || scenario == "workflow初始化"
			if (err == nil) != wantSuccess {
				t.Fatalf("结果错误: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// engineReady 判断引擎段是否已通过校验，workflow 段期望只在引擎就绪后设置。
// @param scenario 场景名
// @param engineCount 引擎表计数
// @param engineTotal 引擎表总数
// @return 引擎段是否就绪
func engineReady(scenario string, engineCount, engineTotal int) bool {
	switch scenario {
	case "查询失败", "引擎DDL失败", "引擎标记失败", "引擎部分表", "空库未授权",
		"版本冲突", "缺少版本", "校验和冲突", "缺少标记":
		return false
	}
	// 空库初始化成功后引擎段即就绪；完整库直接就绪
	return engineCount == engineTotal || scenario == "初始化"
}

// expectScriptExec 为脚本逐条执行设置期望，可在首条语句或完成标记处注入失败。
// @param mock 数据库模拟
// @param ddl 脚本内容
// @param failAt 首条语句失败标志（-1 表示不失败）
// @param markerKey 完成标记名
// @param markerValue 完成标记值
// @param markerFail 完成标记写入是否失败
func expectScriptExec(mock sqlmock.Sqlmock, ddl string, failAt int, markerKey, markerValue string, markerFail bool) {
	for i, statement := range splitStatements(ddl) {
		expected := mock.ExpectExec(regexp.QuoteMeta(statement))
		// failAt=0 表示第一条语句失败，模拟 DDL 中断
		if i == failAt {
			expected.WillReturnError(errors.New("DDL失败"))
			return
		}
		expected.WillReturnResult(sqlmock.NewResult(0, 1))
	}
	expected := mock.ExpectExec("INSERT INTO ACT_GE_PROPERTY").WithArgs(markerKey, markerValue)
	if markerFail {
		expected.WillReturnError(errors.New("标记失败"))
	} else {
		expected.WillReturnResult(sqlmock.NewResult(0, 1))
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
