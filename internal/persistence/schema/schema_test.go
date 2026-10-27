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
