// Package tenant 提供请求租户上下文。
package tenant

import "context"

type contextKey struct{}

// With 将租户写入上下文。
// @param ctx 上下文
// @param tenant 租户
// @return 新上下文
func With(ctx context.Context, tenant string) context.Context {
	if tenant == "" {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, tenant)
}

// From 从上下文读取租户。
// @param ctx 上下文
// @return 租户
func From(ctx context.Context) string {
	value, _ := ctx.Value(contextKey{}).(string)
	return value
}
