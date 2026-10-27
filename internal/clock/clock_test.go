package clock

import (
	"testing"
	"time"
)

// TestFakeClock 验证假时钟推进。
// @param t 测试上下文
func TestFakeClock(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fake := NewFake(start)
	if !fake.Now().Equal(start) {
		t.Fatalf("初始时间错误")
	}
	fake.Advance(time.Hour)
	if !fake.Now().Equal(start.Add(time.Hour)) {
		t.Fatalf("推进后时间错误")
	}
	next := start.Add(2 * time.Hour)
	fake.Set(next)
	if !fake.Now().Equal(next) {
		t.Fatalf("设置后时间错误")
	}
}

// TestRealClock 验证真实时钟。
// @param t 测试上下文
func TestRealClock(t *testing.T) {
	now := (Real{}).Now()
	if now.IsZero() {
		t.Fatalf("真实时钟返回零值")
	}
}
