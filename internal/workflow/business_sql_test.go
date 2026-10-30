// business_sql_test.go 验证业务系统注册 SQL 适配器（组合既有仓储 + 裸 SQL 补齐方法）。
package workflow

import (
	"context"
	"database/sql/driver"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/DengQingNian/Flowtide/api"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
)

// bizSelectColumns 是查询列清单文本（与表定义顺序一致）。
const bizSelectColumns = "SELECT id, system_symbol, system_name, task_call_addr, execution_call_addr, secret, creator, creator_name, create_time, updater, update_time, tenant_id FROM workflow_business_system"

// newBizSQLStore 创建 SQL 适配器与其 sqlmock。
// @param t 测试上下文
// @return 适配器
// @return 数据库模拟
func newBizSQLStore(t *testing.T) (*SQLBusinessSystemStore, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("创建 sqlmock 失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := pwf.NewStore(db, pwf.DialectMySQL)
	if err != nil {
		t.Fatalf("创建 workflow 仓储入口失败: %v", err)
	}
	adapter, err := NewSQLBusinessSystemStore(db, pwf.DialectMySQL, store)
	if err != nil {
		t.Fatalf("创建 SQL 适配器失败: %v", err)
	}
	return adapter, mock
}

// bizFullRow 返回一行完整注册数据（列顺序与 bizSelectColumns 一致）。
// @param symbol 系统代号
// @return 行值
func bizFullRow(symbol string) []driver.Value {
	return []driver.Value{"id-1", symbol, "系统 " + symbol, "http://" + symbol + "/task", "http://" + symbol + "/exec",
		"secret-1", "admin", "管理员", time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC),
		"editor", time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC), symbol}
}

// TestNewSQLBusinessSystemStoreValidation 验证构造入参校验。
// @param t 测试上下文
func TestNewSQLBusinessSystemStoreValidation(t *testing.T) {
	if _, err := NewSQLBusinessSystemStore(nil, pwf.DialectMySQL, nil); err == nil {
		t.Error("空入参应报错")
	}
}

// TestSQLBusinessSystemStoreSaveInsert 验证 Save 新增路径与插入列值映射。
// @param t 测试上下文
func TestSQLBusinessSystemStoreSaveInsert(t *testing.T) {
	adapter, mock := newBizSQLStore(t)
	ctx := context.Background()
	when := time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC)
	// 按代号查询未命中 → 走 Insert（12 列全量）
	mock.ExpectQuery(regexp.QuoteMeta(bizSelectColumns + " WHERE system_symbol = ? LIMIT 1")).
		WithArgs("oa").WillReturnRows(sqlmock.NewRows(businessSystemColumns))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO workflow_business_system (id, system_symbol, system_name, task_call_addr, execution_call_addr, secret, creator, creator_name, create_time, updater, update_time, tenant_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")).
		WithArgs("id-new", "oa", "OA 系统", "http://oa/task", "http://oa/exec", "s1", "admin", "管理员", when, "admin", when, "oa").
		WillReturnResult(sqlmock.NewResult(1, 1))
	err := adapter.Save(ctx, &BusinessSystem{
		ID: "id-new", SystemSymbol: "oa", SystemName: "OA 系统",
		TaskCallAddr: "http://oa/task", ExecutionCallAddr: "http://oa/exec",
		Secret: "s1", Creator: "admin", CreatorName: "管理员", Updater: "admin",
		CreateTime: when, UpdateTime: when, TenantID: "oa",
	})
	if err != nil {
		t.Fatalf("新增保存失败: %v", err)
	}
	// 新增缺 ID → 报错（先查后插，查询未命中）
	mock.ExpectQuery(regexp.QuoteMeta(bizSelectColumns + " WHERE system_symbol = ? LIMIT 1")).
		WithArgs("crm").WillReturnRows(sqlmock.NewRows(businessSystemColumns))
	if err := adapter.Save(ctx, &BusinessSystem{SystemSymbol: "crm", UpdateTime: when}); err == nil {
		t.Error("新增缺 ID 应报错")
	}
	// 入参校验
	if err := adapter.Save(ctx, nil); err == nil {
		t.Error("nil 入参应报错")
	}
	if err := adapter.Save(ctx, &BusinessSystem{UpdateTime: when}); err == nil {
		t.Error("空代号应报错")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestSQLBusinessSystemStoreSaveUpdate 验证 Save 更新路径（保留原 ID 按主键覆盖）。
// @param t 测试上下文
func TestSQLBusinessSystemStoreSaveUpdate(t *testing.T) {
	adapter, mock := newBizSQLStore(t)
	ctx := context.Background()
	when := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	existing := sqlmock.NewRows(businessSystemColumns).AddRow("id-1", "oa", "旧名", "http://old/task", "", "s0", "admin", "管理员", when.Add(-time.Hour), "admin", when.Add(-time.Hour), "oa")
	mock.ExpectQuery(regexp.QuoteMeta(bizSelectColumns + " WHERE system_symbol = ? LIMIT 1")).WithArgs("oa").WillReturnRows(existing)
	// SET 列按表定义顺序：system_symbol..secret, creator, creator_name, updater, update_time, tenant_id
	mock.ExpectExec(regexp.QuoteMeta("UPDATE workflow_business_system SET system_symbol = ?, system_name = ?, task_call_addr = ?, execution_call_addr = ?, secret = ?, creator = ?, creator_name = ?, updater = ?, update_time = ?, tenant_id = ? WHERE id = ?")).
		WithArgs("oa", "新名", "http://old/task", "http://new/exec", "s0", "admin", "管理员", "editor", when, "oa", "id-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	err := adapter.Save(ctx, &BusinessSystem{
		ID: "ignored", SystemSymbol: "oa", SystemName: "新名", TaskCallAddr: "http://old/task",
		ExecutionCallAddr: "http://new/exec", Secret: "s0", Creator: "admin", CreatorName: "管理员",
		Updater: "editor", UpdateTime: when, TenantID: "oa",
	})
	if err != nil {
		t.Fatalf("更新保存失败: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestSQLBusinessSystemStoreGetBySymbol 验证按代号查询的行映射与错误路径。
// @param t 测试上下文
func TestSQLBusinessSystemStoreGetBySymbol(t *testing.T) {
	adapter, mock := newBizSQLStore(t)
	ctx := context.Background()
	// 命中并校验全部字段映射
	mock.ExpectQuery(regexp.QuoteMeta(bizSelectColumns + " WHERE system_symbol = ? LIMIT 1")).WithArgs("oa").
		WillReturnRows(sqlmock.NewRows(businessSystemColumns).AddRow(bizFullRow("oa")...))
	system, found, err := adapter.GetBySymbol(ctx, "oa")
	if err != nil || !found {
		t.Fatalf("按代号查询失败: %v", err)
	}
	when := time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	if system.ID != "id-1" || system.SystemSymbol != "oa" || system.SystemName != "系统 oa" ||
		system.TaskCallAddr != "http://oa/task" || system.ExecutionCallAddr != "http://oa/exec" ||
		system.Secret != "secret-1" || system.Creator != "admin" || system.CreatorName != "管理员" ||
		system.Updater != "editor" || system.TenantID != "oa" ||
		!system.CreateTime.Equal(when) || !system.UpdateTime.Equal(updated) {
		t.Errorf("行映射不符: %+v", system)
	}
	// 查无
	mock.ExpectQuery(regexp.QuoteMeta(bizSelectColumns + " WHERE system_symbol = ? LIMIT 1")).WithArgs("nope").
		WillReturnRows(sqlmock.NewRows(businessSystemColumns))
	if _, found, err := adapter.GetBySymbol(ctx, "nope"); err != nil || found {
		t.Error("查无应返回 false")
	}
	// 查询报错
	mock.ExpectQuery(regexp.QuoteMeta(bizSelectColumns + " WHERE system_symbol = ? LIMIT 1")).WithArgs("bad").
		WillReturnError(errors.New("db down"))
	if _, _, err := adapter.GetBySymbol(ctx, "bad"); err == nil {
		t.Error("查询报错应上抛")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestSQLBusinessSystemStoreGetByTenant 验证裸 SQL 补齐的按租户查询。
// @param t 测试上下文
func TestSQLBusinessSystemStoreGetByTenant(t *testing.T) {
	adapter, mock := newBizSQLStore(t)
	ctx := context.Background()
	// 命中
	mock.ExpectQuery(regexp.QuoteMeta(bizSelectColumns + " WHERE tenant_id = ? ORDER BY system_symbol LIMIT 1")).WithArgs("oa").
		WillReturnRows(sqlmock.NewRows(businessSystemColumns).AddRow(bizFullRow("oa")...))
	system, found, err := adapter.GetByTenant(ctx, "oa")
	if err != nil || !found || system.SystemSymbol != "oa" {
		t.Fatalf("按租户查询失败: %v", err)
	}
	// 查无
	mock.ExpectQuery(regexp.QuoteMeta(bizSelectColumns + " WHERE tenant_id = ? ORDER BY system_symbol LIMIT 1")).WithArgs("nope").
		WillReturnRows(sqlmock.NewRows(businessSystemColumns))
	if _, found, _ := adapter.GetByTenant(ctx, "nope"); found {
		t.Error("查无租户不应命中")
	}
	// 查询报错
	mock.ExpectQuery(regexp.QuoteMeta(bizSelectColumns + " WHERE tenant_id = ? ORDER BY system_symbol LIMIT 1")).WithArgs("bad").
		WillReturnError(errors.New("db down"))
	if _, _, err := adapter.GetByTenant(ctx, "bad"); err == nil {
		t.Error("查询报错应上抛")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestSQLBusinessSystemStoreList 验证列表与行映射。
// @param t 测试上下文
func TestSQLBusinessSystemStoreList(t *testing.T) {
	adapter, mock := newBizSQLStore(t)
	ctx := context.Background()
	rows := sqlmock.NewRows(businessSystemColumns)
	rows.AddRow(bizFullRow("crm")...)
	rows.AddRow(bizFullRow("oa")...)
	mock.ExpectQuery(regexp.QuoteMeta(bizSelectColumns + " WHERE 1 = 1 ORDER BY system_symbol")).WillReturnRows(rows)
	all, err := adapter.List(ctx)
	if err != nil {
		t.Fatalf("列表失败: %v", err)
	}
	if len(all) != 2 || all[0].SystemSymbol != "crm" || all[1].SystemSymbol != "oa" {
		t.Errorf("列表映射不符: %+v", all)
	}
	// 报错路径
	mock.ExpectQuery(regexp.QuoteMeta(bizSelectColumns + " WHERE 1 = 1 ORDER BY system_symbol")).WillReturnError(errors.New("db down"))
	if _, err := adapter.List(ctx); err == nil {
		t.Error("列表报错应上抛")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestSQLBusinessSystemStoreDelete 验证按代号删除（先查 ID 再按主键删除）。
// @param t 测试上下文
func TestSQLBusinessSystemStoreDelete(t *testing.T) {
	adapter, mock := newBizSQLStore(t)
	ctx := context.Background()
	// 命中删除
	mock.ExpectQuery(regexp.QuoteMeta(bizSelectColumns + " WHERE system_symbol = ? LIMIT 1")).WithArgs("oa").
		WillReturnRows(sqlmock.NewRows(businessSystemColumns).AddRow(bizFullRow("oa")...))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM workflow_business_system WHERE id = ?")).WithArgs("id-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := adapter.Delete(ctx, "oa"); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	// 查无不删除
	mock.ExpectQuery(regexp.QuoteMeta(bizSelectColumns + " WHERE system_symbol = ? LIMIT 1")).WithArgs("nope").
		WillReturnRows(sqlmock.NewRows(businessSystemColumns))
	if err := adapter.Delete(ctx, "nope"); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("删除不存在应报 not found，实际 %v", err)
	}
	// 删除执行报错
	mock.ExpectQuery(regexp.QuoteMeta(bizSelectColumns + " WHERE system_symbol = ? LIMIT 1")).WithArgs("bad").
		WillReturnRows(sqlmock.NewRows(businessSystemColumns).AddRow(bizFullRow("bad")...))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM workflow_business_system WHERE id = ?")).WithArgs("id-1").
		WillReturnError(errors.New("db down"))
	if err := adapter.Delete(ctx, "bad"); err == nil {
		t.Error("删除报错应上抛")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestSQLBusinessSystemStoreScanError 验证结果行扫描失败路径。
// @param t 测试上下文
func TestSQLBusinessSystemStoreScanError(t *testing.T) {
	adapter, mock := newBizSQLStore(t)
	ctx := context.Background()
	// 返回列数与扫描目标不一致 → Scan 报错
	mock.ExpectQuery(regexp.QuoteMeta(bizSelectColumns + " WHERE tenant_id = ? ORDER BY system_symbol LIMIT 1")).WithArgs("oa").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("id-1"))
	if _, _, err := adapter.GetByTenant(ctx, "oa"); err == nil {
		t.Error("扫描失败应上抛")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}
