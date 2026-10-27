package flowable

import (
	"testing"
	"time"
)

// TestScalarHelpers 验证列读取辅助函数。
// @param t 测试上下文
func TestScalarHelpers(t *testing.T) {
	row := Row{Table: "ACT_RE_PROCDEF", Values: map[string]any{
		"nil":      nil,
		"int":      1,
		"int64":    int64(2),
		"float":    3.0,
		"string":   "x",
		"bytes":    []byte("b"),
		"time":     time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC),
		"timeText": "2026-10-28T00:00:00Z",
		"bad":      "bad",
	}}
	if value, err := stringValue(row, "nil"); err != nil || value != "" {
		t.Fatalf("nil 字符串错误: %v %v", value, err)
	}
	if _, err := stringValue(row, "int"); err == nil {
		t.Fatalf("非字符串应失败")
	}
	if _, err := stringValue(row, "missing"); err == nil {
		t.Fatalf("缺失列应失败")
	}
	if value, err := bytesValue(row, "nil"); err != nil || value != nil {
		t.Fatalf("nil 字节错误: %v %v", value, err)
	}
	if _, err := bytesValue(row, "missing"); err == nil {
		t.Fatalf("缺失字节列应失败")
	}
	if _, err := bytesValue(row, "string"); err == nil {
		t.Fatalf("非字节列应失败")
	}
	if value, _ := intValue(row, "int"); value != 1 {
		t.Fatalf("int 转换错误")
	}
	if value, _ := intValue(row, "int64"); value != 2 {
		t.Fatalf("int64 转换错误")
	}
	if value, _ := intValue(row, "float"); value != 3 {
		t.Fatalf("float 转换错误")
	}
	if _, err := intValue(row, "missing"); err == nil {
		t.Fatalf("缺失整数列应失败")
	}
	if _, err := intValue(row, "bad"); err == nil {
		t.Fatalf("非法整数应失败")
	}
	if value, err := timeValue(row, "time"); err != nil || value.IsZero() {
		t.Fatalf("时间转换错误: %v", err)
	}
	if value, err := timeValue(row, "timeText"); err != nil || value.IsZero() {
		t.Fatalf("时间文本转换错误: %v", err)
	}
	if _, err := timeValue(row, "missing"); err == nil {
		t.Fatalf("缺失时间列应失败")
	}
	if _, err := timeValue(row, "bad"); err == nil {
		t.Fatalf("非法时间应失败")
	}
	if _, err := timeValue(row, "int"); err == nil {
		t.Fatalf("非时间类型应失败")
	}
}

// TestDecodeDefinitionErrors 验证定义行错误。
// @param t 测试上下文
func TestDecodeDefinitionErrors(t *testing.T) {
	cases := []func(Row){
		func(row Row) { delete(row.Values, "KEY_") },
		func(row Row) { row.Values["VERSION_"] = "bad" },
		func(row Row) { row.Values["SUSPENSION_STATE_"] = "bad" },
	}
	for _, mutate := range cases {
		row := procRow(t, "p1", "d1", "x")
		mutate(row)
		if _, err := decodeDefinition(row); err == nil {
			t.Fatalf("定义行应解析失败")
		}
	}
	deployment := deploymentRow(t, "d1")
	delete(deployment.Values, "NAME_")
	if _, err := decodeDeployment(deployment); err == nil {
		t.Fatalf("部署行应解析失败")
	}
}
