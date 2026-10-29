package api

import (
	"errors"
	"strings"
	"testing"
)

// TestVariableConflictError 验证公共调用方可同时获得中文说明、冲突类别及结构化变量位置。
// @param t 测试上下文
// @return 无
func TestVariableConflictError(t *testing.T) {
	conflict := &VariableConflictError{Name: "金额", ExecutionID: "execution"}
	err := NewError("ENGINE-1121", "变量已存在", conflict)
	if !errors.Is(err, ErrConflict) || !strings.Contains(err.Error(), "金额") {
		t.Fatal("变量冲突丢失类别或中文说明")
	}
	var typed *VariableConflictError
	if !errors.As(err, &typed) || typed != conflict || typed.ExecutionID != "execution" {
		t.Fatal("变量冲突位置不可读取")
	}
}
