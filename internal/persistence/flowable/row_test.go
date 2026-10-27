package flowable

import (
	"strings"
	"testing"
)

// TestTables 验证支持的表。
// @param t 测试上下文
func TestTables(t *testing.T) {
	tables := Tables()
	if len(tables) != 15 || tables[0] != "ACT_GE_BYTEARRAY" {
		t.Fatalf("表列表错误: %+v", tables)
	}
}

// TestNewRowAndSet 验证行与列校验。
// @param t 测试上下文
func TestNewRowAndSet(t *testing.T) {
	if _, err := NewRow("ACT_UNKNOWN"); err == nil {
		t.Fatalf("未知表应失败")
	}
	row, err := NewRow("ACT_GE_BYTEARRAY")
	if err != nil {
		t.Fatalf("创建行失败: %v", err)
	}
	if err := row.Set("ID_", "id"); err != nil {
		t.Fatalf("设置 ID 失败: %v", err)
	}
	if err := row.Set("UNKNOWN_", "x"); err == nil {
		t.Fatalf("未知列应失败")
	}
	if len(row.Columns()) != 6 {
		t.Fatalf("列数量错误: %+v", row.Columns())
	}
	columns := row.Columns()
	columns[0] = "X"
	if row.Columns()[0] != "ID_" {
		t.Fatalf("Columns 应返回副本")
	}
	if args := row.Args(); len(args) != 6 || args[0] != "id" {
		t.Fatalf("参数顺序错误: %+v", args)
	}
	if err := row.SetInsertColumns([]string{"ID_", "NAME_"}); err != nil {
		t.Fatalf("设置插入列失败: %v", err)
	}
	if args := row.Args(); len(args) != 2 || args[1] != nil {
		t.Fatalf("插入列参数错误: %+v", args)
	}
	if _, _, err := row.InsertSQL("mysql"); err != nil {
		t.Fatalf("生成部分插入失败: %v", err)
	}
	if err := row.SetInsertColumns([]string{"UNKNOWN_"}); err == nil {
		t.Fatalf("未知插入列应失败")
	}
	if err := row.SetInsertColumns(nil); err == nil {
		t.Fatalf("空插入列应失败")
	}
}

// TestInsertSQL 验证插入语句。
// @param t 测试上下文
func TestInsertSQL(t *testing.T) {
	row, _ := NewRow("ACT_RE_DEPLOYMENT")
	_ = row.Set("ID_", "d1")
	query, args, err := row.InsertSQL("mysql")
	if err != nil {
		t.Fatalf("生成 MySQL 插入失败: %v", err)
	}
	if !strings.Contains(query, "INSERT INTO ACT_RE_DEPLOYMENT") || strings.Count(query, "?") != 10 {
		t.Fatalf("MySQL 插入语句错误: %s", query)
	}
	if len(args) != 10 || args[0] != "d1" {
		t.Fatalf("参数错误: %+v", args)
	}
	query, args, err = row.InsertSQL("postgres")
	if err != nil || !strings.Contains(query, "$10") || len(args) != 10 {
		t.Fatalf("PostgreSQL 插入语句错误: %v %s", err, query)
	}
	if _, _, err := row.InsertSQL("oracle"); err == nil {
		t.Fatalf("非法方言应失败")
	}
}

// TestDeleteSQL 验证删除语句。
// @param t 测试上下文
func TestDeleteSQL(t *testing.T) {
	row, _ := NewRow("ACT_RE_DEPLOYMENT")
	if _, _, err := row.DeleteSQL("mysql"); err == nil {
		t.Fatalf("缺少 ID 应失败")
	}
	_ = row.Set("ID_", "d1")
	query, args, err := row.DeleteSQL("postgres")
	if err != nil || query != "DELETE FROM ACT_RE_DEPLOYMENT WHERE ID_ = $1" || args[0] != "d1" {
		t.Fatalf("删除语句错误: %v %s %+v", err, query, args)
	}
	if _, _, err := row.DeleteSQL("oracle"); err == nil {
		t.Fatalf("非法方言应失败")
	}
}

// TestUpdateSQL 验证乐观锁更新语句。
// @param t 测试上下文
func TestUpdateSQL(t *testing.T) {
	row, _ := NewRow("ACT_RU_JOB")
	_ = row.Set("ID_", "j1")
	_ = row.Set("REV_", 2)
	_ = row.Set("TYPE_", "timer")
	query, args, err := row.UpdateSQL("mysql", 2)
	if err != nil {
		t.Fatalf("生成更新语句失败: %v", err)
	}
	if !strings.Contains(query, "UPDATE ACT_RU_JOB") || !strings.Contains(query, "REV_ = ?") || !strings.Contains(query, "WHERE ID_ = ?") {
		t.Fatalf("更新语句错误: %s", query)
	}
	if len(args) != len(row.insertColumns())+1 {
		t.Fatalf("参数数量错误: %d", len(args))
	}
	if args[0] != int64(3) {
		t.Fatalf("REV_ 应递增: %+v", args[:1])
	}
	postgresQuery, _, err := row.UpdateSQL("postgres", 2)
	if err != nil || !strings.Contains(postgresQuery, "$") {
		t.Fatalf("PostgreSQL 更新语句错误: %v %s", err, postgresQuery)
	}
	missingID := Row{Table: "ACT_RU_JOB", Values: map[string]any{"REV_": 1}}
	if _, _, err := missingID.UpdateSQL("mysql", 1); err == nil {
		t.Fatalf("缺 ID 应失败")
	}
	missingRev, _ := NewRow("ACT_RU_JOB")
	_ = missingRev.Set("ID_", "j1")
	if _, _, err := missingRev.UpdateSQL("mysql", 1); err == nil {
		t.Fatalf("缺 REV 应失败")
	}
	if _, _, err := row.UpdateSQL("oracle", 1); err == nil {
		t.Fatalf("非法方言应失败")
	}
}
