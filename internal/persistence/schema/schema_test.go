package schema

import (
	"strings"
	"testing"
)

// TestLoad 验证两种方言的 Schema 加载。
// @param t 测试上下文
func TestLoad(t *testing.T) {
	for _, dialect := range Dialects() {
		content, err := Load(dialect)
		if err != nil || len(content) == 0 {
			t.Fatalf("加载 %s 失败: %v", dialect, err)
		}
		if !strings.Contains(content, "ACT_RE_PROCDEF") {
			t.Fatalf("%s 缺少 ACT_RE_PROCDEF", dialect)
		}
	}
}

// TestTables 验证表名解析。
// @param t 测试上下文
func TestTables(t *testing.T) {
	for _, dialect := range Dialects() {
		tables, err := Tables(dialect)
		if err != nil {
			t.Fatalf("解析 %s 表名失败: %v", dialect, err)
		}
		if len(tables) != 41 {
			t.Fatalf("%s 表数量错误: %d", dialect, len(tables))
		}
		if tables[0] != "ACT_EVT_LOG" {
			t.Fatalf("%s 表名未排序: %s", dialect, tables[0])
		}
	}
}

// TestChecksum 验证校验和。
// @param t 测试上下文
func TestChecksum(t *testing.T) {
	first, err := Checksum("mysql")
	if err != nil || len(first) != 64 {
		t.Fatalf("校验和错误: %v %s", err, first)
	}
	second, _ := Checksum("mysql")
	if first != second {
		t.Fatalf("校验和应稳定")
	}
}

// TestTableColumns 验证表列解析。
// @param t 测试上下文
func TestTableColumns(t *testing.T) {
	for _, dialect := range Dialects() {
		columns, types, err := TableColumns(dialect, "act_ru_task")
		if err != nil {
			t.Fatalf("解析 %s 列失败: %v", dialect, err)
		}
		if len(columns) != len(types) || len(columns) == 0 {
			t.Fatalf("%s 列数量错误: %d/%d", dialect, len(columns), len(types))
		}
		if columns[0] != "ID_" || types[0] == "" {
			t.Fatalf("%s 首列解析错误: %s %s", dialect, columns[0], types[0])
		}
		if columns[len(columns)-1] == "PRIMARY" {
			t.Fatalf("%s 不应包含约束声明", dialect)
		}
	}
}

// TestTableColumnsMissing 验证未知表返回未找到。
// @param t 测试上下文
func TestTableColumnsMissing(t *testing.T) {
	if _, _, err := TableColumns("postgres", "ACT_UNKNOWN"); err == nil {
		t.Fatalf("未知表应返回错误")
	}
	if _, _, err := TableColumns("oracle", "ACT_RU_TASK"); err == nil {
		t.Fatalf("不支持方言应返回错误")
	}
}

// TestSplitColumnDefinitions 验证列定义拆分。
// @param t 测试上下文
func TestSplitColumnDefinitions(t *testing.T) {
	parts := splitColumnDefinitions("A numeric(10,2), B varchar(32), primary key (A)")
	if len(parts) != 3 {
		t.Fatalf("拆分数量错误: %d", len(parts))
	}
	if !isConstraint("Primary") || isConstraint("A") {
		t.Fatalf("约束判断错误")
	}
}

// TestErrors 验证方言错误。
// @param t 测试上下文
func TestErrors(t *testing.T) {
	if _, err := Load("oracle"); err == nil {
		t.Fatalf("不支持方言应报错")
	}
	if _, err := Tables("oracle"); err == nil {
		t.Fatalf("不支持方言表名应报错")
	}
	if _, err := Checksum("oracle"); err == nil {
		t.Fatalf("不支持方言校验和应报错")
	}
	if Version != "8.1.0.3" {
		t.Fatalf("Schema 版本错误")
	}
}
