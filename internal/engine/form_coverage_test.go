package engine

import (
	"context"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
)

// TestConvertFormValue 验证表单类型转换。
// @param t 测试上下文
func TestConvertFormValue(t *testing.T) {
	if value, err := convertFormValue(1, "long"); err != nil || value != 1 {
		t.Fatalf("int 转 long 失败: %v %v", value, err)
	}
	if value, err := convertFormValue(float64(2), "long"); err != nil || value != int64(2) {
		t.Fatalf("float 转 long 失败: %v %v", value, err)
	}
	if value, err := convertFormValue("3", "long"); err != nil || value != int64(3) {
		t.Fatalf("字符串转 long 失败: %v %v", value, err)
	}
	if value, err := convertFormValue(1, "double"); err != nil || value != 1.0 {
		t.Fatalf("int 转 double 失败: %v %v", value, err)
	}
	if value, err := convertFormValue("2.5", "double"); err != nil || value != 2.5 {
		t.Fatalf("字符串转 double 失败: %v %v", value, err)
	}
	if value, err := convertFormValue("true", "boolean"); err != nil || value != true {
		t.Fatalf("字符串转 boolean 失败: %v %v", value, err)
	}
	if _, err := convertFormValue(time.Now(), "date"); err != nil {
		t.Fatalf("时间转 date 失败: %v", err)
	}
	if _, err := convertFormValue("bad", "date"); err == nil {
		t.Fatalf("非法日期应报错")
	}
	if _, err := convertFormValue(struct{}{}, "long"); err == nil {
		t.Fatalf("非法 long 应报错")
	}
	if value, err := convertFormValue("x", "unknown"); err != nil || value != "x" {
		t.Fatalf("未知类型应原样返回")
	}
}

// TestForm_Closed 验证关闭引擎的表单错误。
// @param t 测试上下文
func TestForm_Closed(t *testing.T) {
	engine := newTestEngine(t)
	_ = engine.Close(context.Background())
	if _, err := engine.GetStartFormData(context.Background(), "d"); err == nil {
		t.Fatalf("关闭后启动表单应报错")
	}
	if _, err := engine.GetTaskFormData(context.Background(), "t"); err == nil {
		t.Fatalf("关闭后任务表单应报错")
	}
	if err := engine.SubmitTaskFormData(context.Background(), "t", nil); err == nil {
		t.Fatalf("关闭后提交表单应报错")
	}
}

// TestForm_DirectErrors 验证表单直接错误分支。
// @param t 测试上下文
func TestForm_DirectErrors(t *testing.T) {
	engine := newTestEngine(t)
	engine.state.Tasks["t"] = &taskState{Task: api.Task{ID: "t", ProcessDefinitionID: "missing"}, ActivityID: "x"}
	if _, err := engine.GetTaskFormData(context.Background(), "t"); err == nil {
		t.Fatalf("缺失定义任务表单应报错")
	}
	if err := engine.SubmitTaskFormData(context.Background(), "t", nil); err == nil {
		t.Fatalf("缺失定义提交应报错")
	}
	if err := engine.SubmitTaskFormData(context.Background(), "t", map[string]any{"x": 1}); err == nil {
		t.Fatalf("缺失定义提交应报错")
	}
	if toFormProperties(nil, nil) == nil {
		t.Fatalf("空字段应返回空切片而非 nil")
	}
	if value, err := convertFormValue(true, "boolean"); err != nil || value != true {
		t.Fatalf("bool 转换失败: %v %v", value, err)
	}
	if value, err := convertFormValue("2026-10-28T00:00:00Z", "date"); err != nil || value == nil {
		t.Fatalf("日期字符串转换失败: %v %v", value, err)
	}
	if value, err := convertFormValue(int64(1), "long"); err != nil || value != int64(1) {
		t.Fatalf("int64 转换失败: %v %v", value, err)
	}
	if value, err := convertFormValue(float64(1), "double"); err != nil || value != 1.0 {
		t.Fatalf("double 转换失败: %v %v", value, err)
	}
}
