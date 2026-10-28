package engine

import (
	"context"
	"strings"

	"github.com/flowable-go/flowable/api"
)

// isBuiltinProperty 判断是否为内置只读属性。
// @param name 属性名
// @return 是否内置
func isBuiltinProperty(name string) bool {
	switch name {
	case "common.schema.version", "schema.version", "schema.history", "next.dbid", "engine.version", "historyLevel", "tenantId":
		return true
	}
	return false
}

// CreateProperty 创建自定义引擎属性。
// @param ctx 上下文
// @param name 名称
// @param value 值
// @return 创建错误
func (e *Engine) CreateProperty(ctx context.Context, name, value string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, api.NewError("ENGINE-1801", "属性名不能为空", api.ErrBadRequest)
		}
		if isBuiltinProperty(name) {
			return nil, api.NewError("ENGINE-1802", "内置属性已存在", api.ErrConflict)
		}
		if _, exists := e.state.Properties[name]; exists {
			return nil, api.NewError("ENGINE-1802", "属性已存在", api.ErrConflict)
		}
		e.state.Properties[name] = value
		return nil, nil
	})
	return err
}

// UpdateProperty 更新自定义引擎属性。
// @param ctx 上下文
// @param name 名称
// @param value 值
// @return 更新错误
func (e *Engine) UpdateProperty(ctx context.Context, name, value string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		name = strings.TrimSpace(name)
		if isBuiltinProperty(name) {
			return nil, api.NewError("ENGINE-1803", "内置属性不可修改", api.ErrBadRequest)
		}
		if _, exists := e.state.Properties[name]; !exists {
			return nil, api.NewError("ENGINE-1804", "属性不存在", api.ErrNotFound)
		}
		e.state.Properties[name] = value
		return nil, nil
	})
	return err
}

// DeleteProperty 删除自定义引擎属性。
// @param ctx 上下文
// @param name 名称
// @return 删除错误
func (e *Engine) DeleteProperty(ctx context.Context, name string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		name = strings.TrimSpace(name)
		if isBuiltinProperty(name) {
			return nil, api.NewError("ENGINE-1803", "内置属性不可修改", api.ErrBadRequest)
		}
		if _, exists := e.state.Properties[name]; !exists {
			return nil, api.NewError("ENGINE-1804", "属性不存在", api.ErrNotFound)
		}
		delete(e.state.Properties, name)
		return nil, nil
	})
	return err
}
