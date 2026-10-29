package engine

import "context"

// actorKey 保存请求的操作人标识。
type actorKey struct{}

// WithActor 将可信操作人写入上下文；REST 认证中间件使用。
// @param ctx 调用方上下文
// @param userID 认证后的操作人
// @return 携带操作人的上下文
func WithActor(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, actorKey{}, userID)
}

// actorFromContext 读取操作人，未设置时为空。
// @param ctx 调用方上下文
// @return 操作人
func actorFromContext(ctx context.Context) string {
	value, _ := ctx.Value(actorKey{}).(string)
	return value
}
