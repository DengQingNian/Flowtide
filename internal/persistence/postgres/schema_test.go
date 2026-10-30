package postgres

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/DengQingNian/Flowtide/internal/persistence/schema"
)

// TestEnsureSchema 验证引擎（001）与 workflow（002）两段事务初始化、重复校验与损坏拒绝。
// @param t 测试上下文
// @return 无
func TestEnsureSchema(t *testing.T) {
	engineTables, _ := schema.Tables("postgres")
	engineDDL, _ := schema.Load("postgres")
	engineChecksum, _ := schema.Checksum("postgres")
	wfTables, _ := schema.FileTables("postgres", schema.WorkflowFile)
	wfDDL, _ := schema.LoadFile("postgres", schema.WorkflowFile)
	wfChecksum, _ := schema.ChecksumFile("postgres", schema.WorkflowFile)
	failure := errors.New("数据库故障")
	for _, scenario := range []string{
		"初始化", "已有库", "部分表", "未授权", "表查询失败", "版本错误", "标记缺失",
		"开始失败", "DDL失败", "标记失败", "提交失败",
		"workflow初始化", "workflow查询失败", "workflow开始失败", "workflowDDL失败", "workflow标记失败", "workflow提交失败",
		"workflow未授权", "workflow部分表", "workflow标记错误", "workflow标记缺失",
	} {
		t.Run(scenario, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			engineCount := 0
			switch scenario {
			case "已有库", "版本错误", "标记缺失", "workflow初始化", "workflow查询失败", "workflow开始失败", "workflowDDL失败", "workflow标记失败", "workflow提交失败", "workflow未授权", "workflow部分表", "workflow标记错误", "workflow标记缺失":
				engineCount = len(engineTables)
			case "部分表":
				engineCount = 1
			}
			wfCount := 0
			switch scenario {
			case "已有库", "workflow标记错误", "workflow标记缺失":
				wfCount = len(wfTables)
			case "workflow部分表":
				wfCount = 1
			}
			// 引擎表计数查询
			engineQuery := mock.ExpectQuery("SELECT COUNT")
			if scenario == "表查询失败" {
				engineQuery.WillReturnError(failure)
			} else {
				engineQuery.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(engineCount))
			}
			if engineCount == len(engineTables) {
				version := schema.Version
				if scenario == "版本错误" {
					version = "bad"
				}
				mock.ExpectQuery("SELECT VALUE_").WithArgs("schema.version").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(version))
				if scenario != "版本错误" {
					mark := mock.ExpectQuery("SELECT VALUE_").WithArgs(checksumKey)
					if scenario == "标记缺失" {
						mark.WillReturnError(failure)
					} else {
						mark.WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(engineChecksum))
					}
				}
			}
			// 引擎段事务初始化（空库且允许初始化时执行 001 脚本）
			switch scenario {
			case "初始化":
				expectScriptTx(mock, engineDDL, false, false, checksumKey, engineChecksum, false, false)
			case "开始失败":
				expectScriptTx(mock, engineDDL, true, false, checksumKey, engineChecksum, false, false)
			case "DDL失败":
				expectScriptTx(mock, engineDDL, false, true, checksumKey, engineChecksum, false, false)
			case "标记失败":
				expectScriptTx(mock, engineDDL, false, false, checksumKey, engineChecksum, true, false)
			case "提交失败":
				expectScriptTx(mock, engineDDL, false, false, checksumKey, engineChecksum, false, true)
			}
			// 引擎段就绪后才进入 workflow 段：计数、校验或补跑 002
			if engineReady(scenario, engineCount, len(engineTables)) {
				wfQuery := mock.ExpectQuery("SELECT COUNT")
				if scenario == "workflow查询失败" {
					wfQuery.WillReturnError(failure)
				} else {
					wfQuery.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(wfCount))
				}
				switch scenario {
				case "初始化", "workflow初始化":
					expectScriptTx(mock, wfDDL, false, false, workflowChecksumKey, wfChecksum, false, false)
				case "workflow开始失败":
					expectScriptTx(mock, wfDDL, true, false, workflowChecksumKey, wfChecksum, false, false)
				case "workflowDDL失败":
					expectScriptTx(mock, wfDDL, false, true, workflowChecksumKey, wfChecksum, false, false)
				case "workflow标记失败":
					expectScriptTx(mock, wfDDL, false, false, workflowChecksumKey, wfChecksum, true, false)
				case "workflow提交失败":
					expectScriptTx(mock, wfDDL, false, false, workflowChecksumKey, wfChecksum, false, true)
				}
				if wfCount == len(wfTables) {
					value := wfChecksum
					if scenario == "workflow标记错误" {
						value = "bad"
					}
					mark := mock.ExpectQuery("SELECT VALUE_").WithArgs(workflowChecksumKey)
					if scenario == "workflow标记缺失" {
						mark.WillReturnError(failure)
					} else {
						mark.WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(value))
					}
				}
			}
			initialize := scenario != "未授权" && scenario != "workflow未授权"
			err := EnsureSchema(context.Background(), db, initialize)
			wantSuccess := scenario == "初始化" || scenario == "已有库" || scenario == "workflow初始化"
			if (err == nil) != wantSuccess {
				t.Fatalf("Schema结果: %v", err)
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
	case "表查询失败", "部分表", "未授权", "版本错误", "标记缺失",
		"开始失败", "DDL失败", "标记失败", "提交失败":
		return false
	}
	// 空库初始化成功后引擎段即就绪；完整库直接就绪
	return engineCount == engineTotal || scenario == "初始化"
}

// expectScriptTx 为单段脚本的事务化执行设置期望（开始、逐条执行、标记、提交）。
// @param mock 数据库模拟
// @param ddl 脚本内容
// @param beginFail 开始事务是否失败
// @param ddlFail 首条 DDL 是否失败
// @param markerKey 完成标记名
// @param markerValue 完成标记值
// @param markerFail 完成标记写入是否失败
// @param commitFail 提交是否失败
func expectScriptTx(mock sqlmock.Sqlmock, ddl string, beginFail, ddlFail bool, markerKey, markerValue string, markerFail, commitFail bool) {
	begin := mock.ExpectBegin()
	if beginFail {
		begin.WillReturnError(failureError())
		return
	}
	for i, statement := range splitStatements(ddl) {
		exec := mock.ExpectExec(regexp.QuoteMeta(statement))
		// ddlFail 时首条语句失败并回滚
		if ddlFail && i == 0 {
			exec.WillReturnError(failureError())
			mock.ExpectRollback()
			return
		}
		exec.WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mark := mock.ExpectExec("INSERT INTO ACT_GE_PROPERTY").WithArgs(markerKey, markerValue)
	if markerFail {
		mark.WillReturnError(failureError())
		mock.ExpectRollback()
		return
	}
	mark.WillReturnResult(sqlmock.NewResult(0, 1))
	if commitFail {
		// 提交失败后 database/sql 不再向驱动转发回滚
		mock.ExpectCommit().WillReturnError(failureError())
		return
	}
	mock.ExpectCommit()
}

// failureError 返回测试用数据库错误。
// @return 错误
func failureError() error { return errors.New("数据库故障") }

// TestSplitStatements 验证内置脚本的注释和空语句处理。
// @param t 测试上下文
// @return 无
func TestSplitStatements(t *testing.T) {
	if got := splitStatements("-- 中文注释\nSELECT 1;\n;SELECT 2;\n"); len(got) != 2 {
		t.Fatalf("拆分错误: %v", got)
	}
}
