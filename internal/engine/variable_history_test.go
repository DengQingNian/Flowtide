package engine

import (
	"context"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
)

// TestHistoricVariables 验证历史变量记录与更新。
// @param t 测试上下文
func TestHistoricVariables(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if err := engine.SetVariable(context.Background(), instance.ID, "amount", 10); err != nil {
		t.Fatalf("设置变量失败: %v", err)
	}
	if err := engine.SetVariable(context.Background(), instance.ID, "amount", 20); err != nil {
		t.Fatalf("更新变量失败: %v", err)
	}
	items, err := engine.ListHistoricVariableInstances(context.Background(), instance.ID)
	if err != nil {
		t.Fatalf("查询历史变量失败: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("期望 2 个历史变量，实际 %d: %+v", len(items), items)
	}
	for _, item := range items {
		switch item.Name {
		case "initiator":
			if item.Type != "string" || item.Value != "alice" {
				t.Fatalf("字符串变量记录错误: %+v", item)
			}
		case "amount":
			if item.Type != "long" || item.Value != 20 {
				t.Fatalf("数值变量记录错误: %+v", item)
			}
		}
	}
}

// TestHistoricVariables_None 验证关闭历史不记录变量。
// @param t 测试上下文
func TestHistoricVariables_None(t *testing.T) {
	engine, _ := New(Config{HistoryLevel: HistoryNone})
	deployXML(t, engine, simpleProcess)
	instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	items, _ := engine.ListHistoricVariableInstances(context.Background(), instance.ID)
	if len(items) != 0 {
		t.Fatalf("关闭历史不应记录变量")
	}
}

// TestVariableTypeName 验证变量类型名。
// @param t 测试上下文
func TestVariableTypeName(t *testing.T) {
	cases := []struct {
		value any
		want  string
	}{
		{nil, "null"}, {"x", "string"}, {true, "boolean"}, {1, "long"}, {1.5, "double"},
		{[]byte("x"), "binary"}, {map[string]any{}, "json"},
		{time.UnixMilli(1700000000123), "date"},
	}
	for _, tc := range cases {
		if got := variableTypeName(tc.value); got != tc.want {
			t.Fatalf("%T 期望 %s，实际 %s", tc.value, tc.want, got)
		}
	}
}

// TestDeleteVariable 验证删除实例变量。
// @param t 测试上下文
func TestDeleteVariable(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err := engine.DeleteVariable(context.Background(), instance.ID, "initiator"); err != nil {
		t.Fatalf("删除变量失败: %v", err)
	}
	values, _ := engine.GetVariables(context.Background(), instance.ID)
	if _, exists := values["initiator"]; exists {
		t.Fatalf("变量应删除")
	}
	if err := engine.DeleteVariable(context.Background(), "missing", "x"); err == nil {
		t.Fatalf("缺失实例删除变量应报错")
	}
}
