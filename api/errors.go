// Package api 定义流程引擎的公共类型与服务接口。
package api

import (
	"errors"
	"fmt"
)

// 公共错误哨兵，调用方可用 errors.Is 判断错误类别。
var (
	// ErrNotFound 表示资源不存在。
	ErrNotFound = errors.New("资源不存在")
	// ErrConflict 表示并发冲突或状态冲突。
	ErrConflict = errors.New("状态冲突")
	// ErrBadRequest 表示请求参数非法。
	ErrBadRequest = errors.New("请求参数非法")
	// ErrUnauthorized 表示未认证。
	ErrUnauthorized = errors.New("未认证")
	// ErrTooManyRequests 表示请求过多。
	ErrTooManyRequests = errors.New("请求过多")
	// ErrForbidden 表示无权限。
	ErrForbidden = errors.New("无权限")
	// ErrEngineClosed 表示引擎已关闭。
	ErrEngineClosed = errors.New("引擎已关闭")
)

// VariableConflictError 提供变量创建冲突的结构化位置，供调用方生成对应协议的错误响应。
type VariableConflictError struct {
	Name        string // 冲突的变量名称
	ExecutionID string // 请求操作的执行标识
}

// Error 返回变量冲突的中文说明。
// @return 错误文本
func (e *VariableConflictError) Error() string {
	return fmt.Sprintf("执行 %s 的变量 %s 已存在", e.ExecutionID, e.Name)
}

// Unwrap 保持公共冲突哨兵语义，便于跨协议判断错误类别。
// @return 冲突错误哨兵
func (e *VariableConflictError) Unwrap() error { return ErrConflict }

// Error 是引擎统一错误类型，携带稳定错误码与底层原因。
type Error struct {
	Code    string // 稳定错误码，如 ENGINE-0404
	Message string // 中文说明
	Cause   error  // 底层错误，可为空
}

// Error 返回错误的可读字符串。
// @return 错误字符串
func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Code + ": " + e.Message + ": " + e.Cause.Error()
	}
	return e.Code + ": " + e.Message
}

// Unwrap 返回底层错误，支持 errors.Is/As。
// @return 底层错误
func (e *Error) Unwrap() error { return e.Cause }

// NewError 创建引擎错误。
// @param code 稳定错误码
// @param message 中文说明
// @param cause 底层错误，可为空
// @return 引擎错误
func NewError(code, message string, cause error) *Error {
	return &Error{Code: code, Message: message, Cause: cause}
}
