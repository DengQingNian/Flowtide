package engine

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/flowable-go/flowable/api"
)

// TestMatchDMNInputEntry 验证输入条件的各类安全子集写法。
// @param t 测试上下文
func TestMatchDMNInputEntry(t *testing.T) {
	cases := []struct {
		name  string
		entry string
		value any
		want  bool
	}{
		{name: "任意短横线", entry: "-", value: 10, want: true},
		{name: "空条件按任意处理", entry: "", value: nil, want: true},
		{name: "双引号字符串相等", entry: "\"gold\"", value: "gold", want: true},
		{name: "双引号字符串不等", entry: "\"gold\"", value: "silver", want: false},
		{name: "单引号字符串相等", entry: "'silver'", value: "silver", want: true},
		{name: "布尔真", entry: "true", value: true, want: true},
		{name: "布尔假", entry: "false", value: true, want: false},
		{name: "整数相等", entry: "42", value: 42, want: true},
		{name: "浮点等价", entry: "42", value: 42.0, want: true},
		{name: "小于", entry: "< 100", value: 99, want: true},
		{name: "小于边界", entry: "< 100", value: 100, want: false},
		{name: "大于等于", entry: ">= 100", value: 100, want: true},
		{name: "小于等于", entry: "<= 100", value: 100, want: true},
		{name: "不等于", entry: "!= 100", value: 99, want: true},
		{name: "等于单符号", entry: "= 100", value: 100, want: true},
		{name: "连续等于", entry: "== 100", value: 100, want: true},
		{name: "闭区间下界", entry: "[1..10]", value: 1, want: true},
		{name: "闭区间上界", entry: "[1..10]", value: 10, want: true},
		{name: "闭区间外", entry: "[1..10]", value: 11, want: false},
		{name: "左开区间下界", entry: "(1..10]", value: 1, want: false},
		{name: "左开区间内部", entry: "(1..10]", value: 2, want: true},
		{name: "右开区间上界", entry: "[1..10)", value: 10, want: false},
		{name: "反向括号区间", entry: "]1..10[", value: 5, want: true},
		{name: "反向括号区间下界", entry: "]1..10[", value: 1, want: false},
		{name: "无下界区间", entry: "[..10]", value: 10, want: true},
		{name: "无上界区间", entry: "[10..]", value: 10, want: true},
		{name: "多值任一命中", entry: "\"gold\", 'silver'", value: "silver", want: true},
		{name: "多值均不命中", entry: "\"gold\", 'silver'", value: "bronze", want: false},
		{name: "字符串区间", entry: "[\"a\"..\"c\"]", value: "b", want: true},
		{name: "非数值比较不命中", entry: "> 10", value: "abc", want: false},
		{name: "空值只匹配任意", entry: "> 10", value: nil, want: false},
		{name: "不等比较空值", entry: "!= \"x\"", value: nil, want: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := matchDMNInputEntry(testCase.entry, testCase.value)
			if err != nil {
				t.Fatalf("求值输入条件失败: %v", err)
			}
			if got != testCase.want {
				t.Fatalf("条件 %q 对值 %v 期望 %v，实际 %v", testCase.entry, testCase.value, testCase.want, got)
			}
		})
	}
}

// TestMatchDMNInputEntryErrors 验证非法输入条件返回明确错误。
// @param t 测试上下文
func TestMatchDMNInputEntryErrors(t *testing.T) {
	cases := []string{
		"\"unterminated",
		"\"a\",",
		"foo",
		"> ",
		"> true",
		"[1..x]",
		"[true..false]",
		"[..]",
	}
	for _, entry := range cases {
		t.Run(entry, func(t *testing.T) {
			if _, err := matchDMNInputEntry(entry, 1); err == nil {
				t.Fatalf("非法条件 %q 应返回错误", entry)
			}
		})
	}
}

// TestSplitDMNAlternatives 验证多值条件的逗号拆分。
// @param t 测试上下文
func TestSplitDMNAlternatives(t *testing.T) {
	parts, err := splitDMNAlternatives("\"a,b\", 'c'")
	if err != nil {
		t.Fatalf("拆分失败: %v", err)
	}
	if len(parts) != 2 || strings.TrimSpace(parts[0]) != "\"a,b\"" || strings.TrimSpace(parts[1]) != "'c'" {
		t.Fatalf("拆分结果错误: %+v", parts)
	}
	if _, err := splitDMNAlternatives("\"a"); err == nil {
		t.Fatalf("未闭合引号应返回错误")
	}
	if _, err := splitDMNAlternatives("a,,b"); err == nil {
		t.Fatalf("空分支应返回错误")
	}
}

// TestParseDMNDecisions 验证求值模型的解析与 ID 回退。
// @param t 测试上下文
func TestParseDMNDecisions(t *testing.T) {
	const content = `<?xml version="1.0"?>
<definitions>
  <knowledgeModel>
    <decision id="nested" name="Nested">
      <decisionTable id="nestedTable" hitPolicy="ANY">
        <input id="nestedInput" label="等级">
          <inputExpression id="nestedExpr" typeRef="number" text="level * 2"/>
        </input>
        <output id="nestedOutput" name="结果" typeRef="boolean"/>
        <rule id="nestedRule">
          <inputEntry><text>&gt; 10</text></inputEntry>
          <outputEntry><text>true</text></outputEntry>
        </rule>
      </decisionTable>
    </decision>
  </knowledgeModel>
  <decision>
    <decisionTable id="tableOnly"/>
  </decision>
  <decision id="ignored" name="Ignored"/>
</definitions>`
	decisions, err := parseDMNDecisions("nested.dmn", []byte(content))
	if err != nil {
		t.Fatalf("解析 DMN 失败: %v", err)
	}
	if len(decisions) != 2 {
		t.Fatalf("决策数量错误: %+v", decisions)
	}
	nested := decisions[0]
	if nested.ID != "nested" || nested.Key != "nested" || nested.Name != "Nested" {
		t.Fatalf("嵌套决策字段错误: %+v", nested)
	}
	if nested.Table.ID != "nestedTable" || nested.Table.HitPolicy != "ANY" {
		t.Fatalf("嵌套决策表字段错误: %+v", nested.Table)
	}
	if len(nested.Table.Inputs) != 1 || nested.Table.Inputs[0].Expression != "level * 2" {
		t.Fatalf("输入表达式解析错误: %+v", nested.Table.Inputs)
	}
	if len(nested.Table.Outputs) != 1 || nested.Table.Outputs[0].Type != "boolean" {
		t.Fatalf("输出类型解析错误: %+v", nested.Table.Outputs)
	}
	if len(nested.Table.Rules) != 1 || len(nested.Table.Rules[0].Inputs) != 1 || len(nested.Table.Rules[0].Outputs) != 1 {
		t.Fatalf("规则解析错误: %+v", nested.Table.Rules)
	}
	// decision 缺少 id 时回退 decisionTable id，无决策表的 decision 被忽略
	if decisions[1].ID != "tableOnly" || decisions[1].Key != "tableOnly" {
		t.Fatalf("决策 ID 回退错误: %+v", decisions[1])
	}
}

// TestParseDMNDecisionsErrors 验证非法 DMN 资源返回 400。
// @param t 测试上下文
func TestParseDMNDecisionsErrors(t *testing.T) {
	cases := []struct {
		name string
		data string
	}{
		{name: "空内容", data: ""},
		{name: "错误根元素", data: "<notdefinitions/>"},
		{name: "DTD", data: "<!DOCTYPE definitions><definitions/>"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := parseDMNDecisions("bad.dmn", []byte(testCase.data))
			if !errors.Is(err, api.ErrBadRequest) || !strings.Contains(err.Error(), "ENGINE-1207") {
				t.Fatalf("非法 DMN 应返回 400 ENGINE-1207: %v", err)
			}
		})
	}
}

// TestEvalDMNOutputEntry 验证输出项的类型转换与表达式求值。
// @param t 测试上下文
func TestEvalDMNOutputEntry(t *testing.T) {
	ctx := context.Background()
	vars := map[string]any{"amount": 25, "name": "alice"}
	cases := []struct {
		name    string
		entry   string
		want    any
		wantSet bool
		wantErr bool
	}{
		{name: "跳过", entry: "-", wantSet: false},
		{name: "空输出", entry: "", wantSet: false},
		{name: "双引号字符串", entry: "\"high\"", want: "high", wantSet: true},
		{name: "单引号字符串", entry: "'low'", want: "low", wantSet: true},
		{name: "转义引号", entry: "\"a\\\"b\"", want: "a\"b", wantSet: true},
		{name: "布尔真", entry: "true", want: true, wantSet: true},
		{name: "布尔假", entry: "FALSE", want: false, wantSet: true},
		{name: "整数", entry: "42", want: 42, wantSet: true},
		{name: "浮点", entry: "1.5", want: 1.5, wantSet: true},
		{name: "表达式", entry: "${amount * 2}", want: 50.0, wantSet: true},
		{name: "井号表达式", entry: "#{name}", want: "alice", wantSet: true},
		{name: "裸字符串", entry: "plain", want: "plain", wantSet: true},
		{name: "非法引号", entry: "\"unterminated", wantErr: true},
		{name: "非法表达式", entry: "${amount +}", wantErr: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, set, err := evalDMNOutputEntry(ctx, testCase.entry, vars)
			if testCase.wantErr {
				if err == nil {
					t.Fatalf("输出项 %q 应返回错误", testCase.entry)
				}
				return
			}
			if err != nil {
				t.Fatalf("求值输出项失败: %v", err)
			}
			if set != testCase.wantSet {
				t.Fatalf("输出项 %q 是否产生值错误: %v", testCase.entry, set)
			}
			if set && !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("输出项 %q 期望 %#v，实际 %#v", testCase.entry, testCase.want, got)
			}
		})
	}
}

// TestDMNNumberAndCompareHelpers 验证数值解析与比较辅助函数。
// @param t 测试上下文
func TestDMNNumberAndCompareHelpers(t *testing.T) {
	if value, ok := parseDMNNumberValue("42"); !ok || value != 42 {
		t.Fatalf("整数解析错误: %v %v", value, ok)
	}
	if value, ok := parseDMNNumberValue("1.5"); !ok || value != 1.5 {
		t.Fatalf("浮点解析错误: %v %v", value, ok)
	}
	for _, text := range []string{"", "NaN", "Inf", "1.2.3", "abc"} {
		if _, ok := parseDMNNumberValue(text); ok {
			t.Fatalf("非法数字 %q 不应解析成功", text)
		}
	}
	if comparison, ok := dmnCompareValues(1, 2.0); !ok || comparison >= 0 {
		t.Fatalf("数值比较错误: %v %v", comparison, ok)
	}
	if _, ok := dmnCompareValues(true, false); ok {
		t.Fatalf("布尔值不应可比较")
	}
	if !dmnInputEqual(42, 42.0) || dmnInputEqual("42", 42) {
		t.Fatalf("输入相等判断错误")
	}
	if !dmnValuesEqual(1, 1.0) || dmnValuesEqual("1", 1) {
		t.Fatalf("输出值相等判断错误")
	}
}

// TestDMNOutputAndPolicyHelpers 验证结果变量、输出比较与策略规范化。
// @param t 测试上下文
func TestDMNOutputAndPolicyHelpers(t *testing.T) {
	values := []dmnOutputValue{{ID: "answer", Value: 42, Set: true}}
	if value, err := dmnResultVariableValue(values); err != nil || value != 42 {
		t.Fatalf("单输出结果变量错误: %v %v", value, err)
	}
	multi := []dmnOutputValue{
		{ID: "first", Value: "a", Set: true},
		{Name: "second", Value: "b", Set: true},
		{ID: "skipped", Set: false},
	}
	value, err := dmnResultVariableValue(multi)
	if err != nil {
		t.Fatalf("多输出结果变量错误: %v", err)
	}
	expected := map[string]any{"first": "a", "second": "b"}
	if !reflect.DeepEqual(value, expected) {
		t.Fatalf("多输出结果变量内容错误: %+v", value)
	}
	if _, err := dmnResultVariableValue([]dmnOutputValue{{Set: true}, {Set: true}}); err == nil {
		t.Fatalf("多输出缺少输出名应返回错误")
	}
	if !dmnOutputsEqual(values, []dmnOutputValue{{ID: "answer", Value: 42.0, Set: true}}) {
		t.Fatalf("数值输出比较应相等")
	}
	if dmnOutputsEqual(values, []dmnOutputValue{{ID: "answer", Value: 43, Set: true}}) {
		t.Fatalf("不同输出不应相等")
	}
	cases := map[string]string{
		"":              "FIRST",
		"unique":        "UNIQUE",
		"rule_order":    "RULE ORDER",
		" Rule  Order ": "RULE ORDER",
	}
	for raw, want := range cases {
		if got := normalizeDMNHitPolicy(raw); got != want {
			t.Fatalf("策略 %q 规范化期望 %q，实际 %q", raw, want, got)
		}
	}
	if name := dmnOutputName(dmnOutputModel{ID: "id", Name: "name"}); name != "id" {
		t.Fatalf("输出展示名应优先 id: %s", name)
	}
	if name := dmnOutputName(dmnOutputModel{Name: "name"}); name != "name" {
		t.Fatalf("输出展示名回退 name 错误: %s", name)
	}
	if err := dmnBadRequest("ENGINE-0000", "测试"); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("dmnBadRequest 应可识别为 400: %v", err)
	}
}
