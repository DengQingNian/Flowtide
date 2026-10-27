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
	if With(ctx, "") != ctx {
		t.Fatalf("空租户应返回原上下文")
	}
	ctx = With(ctx, "t1")
	if From(ctx) != "t1" {
		t.Fatalf("租户读取错误")
	}
	if From(context.Background()) != "" {
		t.Fatalf("其他上下文不应受影响")
	}
}
