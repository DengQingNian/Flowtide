package api

import (
	"errors"
	"testing"
)

// TestError 验证错误链与消息。
// @param t 测试上下文
func TestError(t *testing.T) {
	cause := errors.New("底层错误")
	err := NewError("ENGINE-0001", "测试错误", cause)
	if err.Error() != "ENGINE-0001: 测试错误: 底层错误" {
		t.Fatalf("错误消息不符: %s", err.Error())
	}
	if !errors.Is(err, cause) {
		t.Fatalf("期望 errors.Is 匹配底层错误")
	}
	if err.Unwrap() != cause {
		t.Fatalf("Unwrap 返回错误")
	}
	noCause := NewError("E", "消息", nil)
	if noCause.Error() != "E: 消息" {
		t.Fatalf("无底层错误消息不符: %s", noCause.Error())
	}
}
