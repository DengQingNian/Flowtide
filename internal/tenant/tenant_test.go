package tenant

import (
	"context"
	"testing"
)

// TestWithFrom 验证租户上下文。
// @param t 测试上下文
func TestWithFrom(t *testing.T) {
	ctx := context.Background()
	if From(ctx) != "" {
		t.Fatalf("空上下文租户应为空")
	}
	if From(With(With(ctx, "t1"), "")) != "" {
		t.Fatal("空租户必须能覆盖原租户")
	}
	ctx = With(ctx, "t1")
	if From(ctx) != "t1" {
		t.Fatalf("租户读取错误")
	}
	if From(context.Background()) != "" {
		t.Fatalf("其他上下文不应受影响")
	}
}

// TestBound 验证默认租户也有明确作用域标记。
// @param t 测试上下文
// @return 无
func TestBound(t *testing.T) {
	if Bound(context.Background()) || !Bound(With(context.Background(), "")) || !Bound(With(context.Background(), "t1")) {
		t.Fatal("租户作用域标记错误")
	}
}
