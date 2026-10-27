package idgen

import (
	"strings"
	"testing"
	"time"
)

// TestNewID 验证 UUIDv7 格式与唯一性。
// @param t 测试上下文
func TestNewID(t *testing.T) {
	generator := NewUUIDv7()
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := generator.NewID()
		if len(id) != 36 {
			t.Fatalf("长度错误: %s", id)
		}
		if strings.Count(id, "-") != 4 {
			t.Fatalf("格式错误: %s", id)
		}
		if id[14] != '7' {
			t.Fatalf("版本位错误: %s", id)
		}
		if seen[id] {
			t.Fatalf("重复 ID: %s", id)
		}
		seen[id] = true
	}
}

// TestNewIDAt 验证指定时间生成。
// @param t 测试上下文
func TestNewIDAt(t *testing.T) {
	generator := NewUUIDv7()
	id := generator.NewIDAt(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	if id == "" {
		t.Fatalf("ID 为空")
	}
}
