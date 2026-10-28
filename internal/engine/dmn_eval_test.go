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

// TestParseDMNTableAggregationAndOutputValues 验证聚合器与 outputValues 的解析。
// @param t 测试上下文
func TestParseDMNTableAggregationAndOutputValues(t *testing.T) {
	const document = `<definitions>
  <decision id="decisionAggregation">
    <decisionTable id="tableAggregation" hitPolicy="COLLECT" aggregation="sum">
      <input id="amountInput"><inputExpression><text>amount</text></inputExpression></input>
      <output id="total" name="合计" typeRef="number"><outputValues><text>10, 20,30</text></outputValues></output>
      <output id="level" name="等级" typeRef="string"><outputValues><text>"high","low"</text></outputValues></output>
      <output id="empty" name="空" typeRef="string"><outputValues><text>  </text></outputValues></output>
      <rule><inputEntry><text>-</text></inputEntry><outputEntry><text>10</text></outputEntry></rule>
    </decisionTable>
  </decision>
</definitions>`
	decisions, err := parseDMNDecisions("aggregation.dmn", []byte(document))
	if err != nil {
		t.Fatalf("解析 DMN 失败: %v", err)
	}
	table := decisions[0].Table
	if table.Aggregation != dmnAggregationSum {
		t.Fatalf("聚合器解析错误: %q", table.Aggregation)
	}
	if !reflect.DeepEqual(table.Outputs[0].Values, []any{10, 20, 30}) {
		t.Fatalf("数字顺序值解析错误: %#v", table.Outputs[0].Values)
	}
	if !reflect.DeepEqual(table.Outputs[1].Values, []any{"high", "low"}) {
		t.Fatalf("字符串顺序值解析错误: %#v", table.Outputs[1].Values)
	}
	if table.Outputs[2].Values != nil {
		t.Fatalf("空顺序值应返回 nil: %#v", table.Outputs[2].Values)
	}

	// 兼容部分文档使用 aggregator 作为属性名
	const alias = `<definitions><decision id="d"><decisionTable id="t" hitPolicy="COLLECT" aggregator="max"/></decision></definitions>`
	aliasDecisions, aliasErr := parseDMNDecisions("alias.dmn", []byte(alias))
	if aliasErr != nil {
		t.Fatalf("解析 aggregator 别名失败: %v", aliasErr)
	}
	if aliasDecisions[0].Table.Aggregation != dmnAggregationMax {
		t.Fatalf("aggregator 别名解析错误: %q", aliasDecisions[0].Table.Aggregation)
	}
}

// TestParseDMNTableModelErrors 验证聚合器与顺序值非法时返回 400。
// @param t 测试上下文
func TestParseDMNTableModelErrors(t *testing.T) {
	cases := []struct {
		name     string
		document string
		code     string
	}{
		{
			name:     "非法聚合器",
			document: `<definitions><decision id="d"><decisionTable id="t" aggregation="AVG"/></decision></definitions>`,
			code:     dmnErrAggregation,
		},
		{
			name:     "数字顺序值非法",
			document: `<definitions><decision id="d"><decisionTable id="t"><output id="o" name="o" typeRef="number"><outputValues><text>10,abc</text></outputValues></output></decisionTable></decision></definitions>`,
			code:     dmnErrOutputOrder,
		},
		{
			name:     "布尔顺序值非法",
			document: `<definitions><decision id="d"><decisionTable id="t"><output id="o" name="o" typeRef="boolean"><outputValues><text>maybe</text></outputValues></output></decisionTable></decision></definitions>`,
			code:     dmnErrOutputOrder,
		},
		{
			name:     "顺序值引号未闭合",
			document: `<definitions><decision id="d"><decisionTable id="t"><output id="o" name="o" typeRef="string"><outputValues><text>"a,b</text></outputValues></output></decisionTable></decision></definitions>`,
			code:     dmnErrOutputOrder,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := parseDMNDecisions("bad.dmn", []byte(testCase.document))
			if !errors.Is(err, api.ErrBadRequest) || !strings.Contains(err.Error(), testCase.code) {
				t.Fatalf("期望 400 %s，实际 %v", testCase.code, err)
			}
		})
	}
}

// TestNormalizeDMNAggregation 验证聚合器规范化与非法取值。
// @param t 测试上下文
func TestNormalizeDMNAggregation(t *testing.T) {
	cases := map[string]string{
		"":      "",
		" sum ": dmnAggregationSum,
		"min":   dmnAggregationMin,
		"MAX":   dmnAggregationMax,
		"Count": dmnAggregationCount,
	}
	for raw, want := range cases {
		got, err := normalizeDMNAggregation(raw)
		if err != nil || got != want {
			t.Fatalf("聚合器 %q 期望 %q，实际 %q %v", raw, want, got, err)
		}
	}
	if _, err := normalizeDMNAggregation("AVG"); !errors.Is(err, api.ErrBadRequest) || !strings.Contains(err.Error(), dmnErrAggregation) {
		t.Fatalf("非法聚合器应返回 400 ENGINE-1921: %v", err)
	}
}

// TestDMNOrderHelpers 验证顺序值下标、可排序判断与多列比较。
// @param t 测试上下文
func TestDMNOrderHelpers(t *testing.T) {
	if index := dmnOrderIndex([]any{"high", "low"}, "low"); index != 1 {
		t.Fatalf("字符串顺序值下标错误: %d", index)
	}
	if index := dmnOrderIndex([]any{10, 20}, 20.0); index != 1 {
		t.Fatalf("数值顺序值下标应按数值比较: %d", index)
	}
	if index := dmnOrderIndex([]any{true}, true); index != 0 {
		t.Fatalf("布尔顺序值下标错误: %d", index)
	}
	if index := dmnOrderIndex([]any{"a"}, nil); index != -1 {
		t.Fatalf("nil 应视为不在顺序列表: %d", index)
	}
	if index := dmnOrderIndex([]any{"a"}, "z"); index != -1 {
		t.Fatalf("未命中顺序值应返回 -1: %d", index)
	}

	for _, value := range []any{nil, "a", true, 1, 1.5} {
		if !dmnOrderableValue(value) {
			t.Fatalf("值 %v 应可排序", value)
		}
	}
	for _, value := range []any{map[string]any{"a": 1}, []any{1}} {
		if dmnOrderableValue(value) {
			t.Fatalf("值 %v 不应可排序", value)
		}
	}

	table := &dmnTableModel{Outputs: []dmnOutputModel{
		{ID: "grade", Values: []any{"A", "B"}},
		{ID: "rank", Values: []any{1, 2}},
	}}
	left := []dmnOutputValue{{ID: "grade", Value: "A", Set: true}, {ID: "rank", Value: 2, Set: true}}
	right := []dmnOutputValue{{ID: "grade", Value: "B", Set: true}, {ID: "rank", Value: 1, Set: true}}
	if comparison := compareDMNOutputRows(table, left, right); comparison >= 0 {
		t.Fatalf("应先比较输出列 grade: %d", comparison)
	}
	same := []dmnOutputValue{{ID: "grade", Value: "A", Set: true}, {ID: "rank", Value: 2, Set: true}}
	if comparison := compareDMNOutputRows(table, left, same); comparison != 0 {
		t.Fatalf("相同顺序值应返回 0: %d", comparison)
	}

	// 未配置顺序值的输出列不参与比较
	ignored := &dmnTableModel{Outputs: []dmnOutputModel{{ID: "grade", Values: []any{"A"}}, {ID: "note"}}}
	leftIgnored := []dmnOutputValue{{Value: "A", Set: true}, {Value: "x", Set: true}}
	rightIgnored := []dmnOutputValue{{Value: "A", Set: true}, {Value: "y", Set: true}}
	if comparison := compareDMNOutputRows(ignored, leftIgnored, rightIgnored); comparison != 0 {
		t.Fatalf("未配置顺序值的输出列不应参与比较: %d", comparison)
	}
}

// TestSortDMNOutputOrder 验证 OUTPUT ORDER 的排序、稳定性与错误。
// @param t 测试上下文
func TestSortDMNOutputOrder(t *testing.T) {
	table := &dmnTableModel{HitPolicy: "OUTPUT ORDER", Outputs: []dmnOutputModel{
		{ID: "level", Values: []any{"high", "medium", "low"}},
	}}
	rows := [][]dmnOutputValue{
		{{ID: "level", Value: "low", Set: true}},
		{{ID: "level", Value: "high", Set: true}},
		{{ID: "level", Value: "medium", Set: true}},
		{{ID: "level", Value: "other", Set: true}},
	}
	sorted, err := sortDMNOutputOrder(table, rows)
	if err != nil {
		t.Fatalf("排序失败: %v", err)
	}
	want := []any{"other", "high", "medium", "low"}
	for index, row := range sorted {
		if row[0].Value != want[index] {
			t.Fatalf("排序结果错误: %#v", sorted)
		}
	}
	if rows[0][0].Value != "low" {
		t.Fatalf("排序不应修改原结果行: %#v", rows)
	}

	// 顺序值相同时保持文档顺序
	stable := &dmnTableModel{HitPolicy: "OUTPUT ORDER", Outputs: []dmnOutputModel{
		{ID: "grade", Values: []any{"A"}},
		{ID: "note"},
	}}
	stableRows := [][]dmnOutputValue{
		{{Value: "A", Set: true}, {Value: "n1", Set: true}},
		{{Value: "A", Set: true}, {Value: "n2", Set: true}},
		{{Value: "A", Set: true}, {Value: "n3", Set: true}},
	}
	stableSorted, err := sortDMNOutputOrder(stable, stableRows)
	if err != nil {
		t.Fatalf("稳定排序失败: %v", err)
	}
	for index, note := range []string{"n1", "n2", "n3"} {
		if stableSorted[index][1].Value != note {
			t.Fatalf("稳定排序结果错误: %#v", stableSorted)
		}
	}

	// 空值视为不在顺序列表中，排在最前
	nilTable := &dmnTableModel{HitPolicy: "PRIORITY", Outputs: []dmnOutputModel{{ID: "level", Values: []any{"b"}}}}
	nilRows := [][]dmnOutputValue{{{Value: "b", Set: true}}, {{Set: false}}}
	nilSorted, err := sortDMNOutputOrder(nilTable, nilRows)
	if err != nil || nilSorted[0][0].Set {
		t.Fatalf("空值应排在最前: %#v %v", nilSorted, err)
	}

	if _, err := sortDMNOutputOrder(&dmnTableModel{HitPolicy: "OUTPUT ORDER", Outputs: []dmnOutputModel{{ID: "level"}}}, rows); !errors.Is(err, api.ErrBadRequest) || !strings.Contains(err.Error(), dmnErrOutputOrder) {
		t.Fatalf("缺少顺序值应返回 400 ENGINE-1922: %v", err)
	}
	badTable := &dmnTableModel{HitPolicy: "OUTPUT ORDER", Outputs: []dmnOutputModel{{ID: "level", Values: []any{"a"}}}}
	if _, err := sortDMNOutputOrder(badTable, [][]dmnOutputValue{{{Value: map[string]any{"a": 1}, Set: true}}}); !errors.Is(err, api.ErrBadRequest) || !strings.Contains(err.Error(), dmnErrOutputOrder) {
		t.Fatalf("不可排序输出值应返回 400 ENGINE-1922: %v", err)
	}
}

// TestCollectDMNMatches 验证 COLLECT 的收集、聚合与错误。
// @param t 测试上下文
func TestCollectDMNMatches(t *testing.T) {
	outputs := []dmnOutputModel{{ID: "total", Name: "合计", Type: "number"}}
	rows := [][]dmnOutputValue{
		{{ID: "total", Value: 10, Set: true}},
		{{ID: "total", Value: 20, Set: true}},
	}
	collected, err := collectDMNMatches(&dmnTableModel{Outputs: outputs}, rows)
	if err != nil || len(collected) != 2 {
		t.Fatalf("COLLECT 无聚合器应返回全部匹配: %v %#v", err, collected)
	}
	for _, testCase := range []struct {
		aggregation string
		want        float64
	}{
		{dmnAggregationSum, 30},
		{dmnAggregationMin, 10},
		{dmnAggregationMax, 20},
		{dmnAggregationCount, 2},
	} {
		aggregated, aggregateErr := collectDMNMatches(&dmnTableModel{Aggregation: testCase.aggregation, Outputs: outputs}, rows)
		if aggregateErr != nil {
			t.Fatalf("聚合 %s 失败: %v", testCase.aggregation, aggregateErr)
		}
		if len(aggregated) != 1 || !aggregated[0][0].Set || aggregated[0][0].Value != testCase.want {
			t.Fatalf("聚合 %s 结果错误: %#v", testCase.aggregation, aggregated)
		}
		if aggregated[0][0].ID != "total" {
			t.Fatalf("聚合结果应保留输出列 id: %#v", aggregated[0][0])
		}
	}

	// 降序输入覆盖 MIN 的候选值更新分支
	reversed := [][]dmnOutputValue{
		{{ID: "total", Value: 20, Set: true}},
		{{ID: "total", Value: 10, Set: true}},
	}
	if aggregated, err := collectDMNMatches(&dmnTableModel{Aggregation: dmnAggregationMin, Outputs: outputs}, reversed); err != nil || aggregated[0][0].Value != 10.0 {
		t.Fatalf("MIN 降序输入聚合错误: %#v %v", aggregated, err)
	}
	if aggregated, err := collectDMNMatches(&dmnTableModel{Aggregation: dmnAggregationMax, Outputs: outputs}, reversed); err != nil || aggregated[0][0].Value != 20.0 {
		t.Fatalf("MAX 降序输入聚合错误: %#v %v", aggregated, err)
	}

	// 匹配行均未产生输出值时返回单条空结果
	emptyRows := [][]dmnOutputValue{{{ID: "total", Set: false}}}
	emptyAggregated, emptyErr := collectDMNMatches(&dmnTableModel{Aggregation: dmnAggregationSum, Outputs: outputs}, emptyRows)
	if emptyErr != nil || len(emptyAggregated) != 1 || emptyAggregated[0][0].Set {
		t.Fatalf("无输出值时应返回单条空结果: %#v %v", emptyAggregated, emptyErr)
	}

	// 四种聚合器都要求数值输出
	textRows := [][]dmnOutputValue{
		{{ID: "total", Value: "a", Set: true}},
		{{ID: "total", Value: "b", Set: true}},
	}
	for _, aggregation := range []string{dmnAggregationSum, dmnAggregationMin, dmnAggregationMax, dmnAggregationCount} {
		if _, err := collectDMNMatches(&dmnTableModel{Aggregation: aggregation, Outputs: outputs}, textRows); !errors.Is(err, api.ErrBadRequest) || !strings.Contains(err.Error(), dmnErrAggregation) {
			t.Fatalf("聚合器 %s 遇到非数值输出应返回 400 ENGINE-1921: %v", aggregation, err)
		}
	}

	// COUNT 仍需数值输出，返回有值行数
	countRows := [][]dmnOutputValue{
		{{ID: "total", Value: 10, Set: true}},
		{{ID: "total", Value: 20, Set: true}},
		{{ID: "total", Set: false}},
	}
	countAggregated, countErr := collectDMNMatches(&dmnTableModel{Aggregation: dmnAggregationCount, Outputs: outputs}, countRows)
	if countErr != nil || countAggregated[0][0].Value != 2.0 {
		t.Fatalf("COUNT 应统计有值的数字行数: %#v %v", countAggregated, countErr)
	}
}

// TestValidateDMNCollectionAggregation 验证 COLLECT 聚合的输出列约束。
// @param t 测试上下文
func TestValidateDMNCollectionAggregation(t *testing.T) {
	// 未配置聚合器时不限制输出列数与类型
	if err := validateDMNCollectionAggregation(&dmnTableModel{Outputs: []dmnOutputModel{{ID: "a"}, {ID: "b"}}}); err != nil {
		t.Fatalf("未配置聚合器不应校验输出列: %v", err)
	}
	// 多输出加聚合器违反 Flowable 约束
	multi := &dmnTableModel{Aggregation: dmnAggregationSum, Outputs: []dmnOutputModel{{ID: "a", Type: "number"}, {ID: "b", Type: "number"}}}
	if err := validateDMNCollectionAggregation(multi); !errors.Is(err, api.ErrBadRequest) || !strings.Contains(err.Error(), dmnErrAggregation) {
		t.Fatalf("多输出聚合应返回 400 ENGINE-1921: %v", err)
	}
	// 单输出但类型不是 number
	for _, outputType := range []string{"string", "", "boolean"} {
		table := &dmnTableModel{Aggregation: dmnAggregationCount, Outputs: []dmnOutputModel{{ID: "a", Type: outputType}}}
		if err := validateDMNCollectionAggregation(table); !errors.Is(err, api.ErrBadRequest) || !strings.Contains(err.Error(), dmnErrAggregation) {
			t.Fatalf("输出类型 %q 聚合应返回 400 ENGINE-1921: %v", outputType, err)
		}
	}
	// 单输出且类型为 number 时通过，类型比较不区分大小写
	valid := &dmnTableModel{Aggregation: dmnAggregationMax, Outputs: []dmnOutputModel{{ID: "a", Type: "Number"}}}
	if err := validateDMNCollectionAggregation(valid); err != nil {
		t.Fatalf("单输出 number 应通过校验: %v", err)
	}
}

// TestEvaluateDMNTableCollectValidation 验证 COLLECT 聚合校验先于输入求值执行。
// @param t 测试上下文
func TestEvaluateDMNTableCollectValidation(t *testing.T) {
	// 输入表达式与聚合约束同时非法时，聚合约束先返回
	table := &dmnTableModel{
		HitPolicy:   "COLLECT",
		Aggregation: dmnAggregationSum,
		Inputs:      []dmnInputModel{{Expression: "amount +"}},
		Outputs:     []dmnOutputModel{{ID: "a", Type: "number"}, {ID: "b", Type: "number"}},
		Rules:       []dmnRuleModel{{Inputs: []string{"-"}, Outputs: []string{"1", "2"}}},
	}
	_, err := (&Engine{}).evaluateDMNTable(context.Background(), table, map[string]any{"amount": 1})
	if !errors.Is(err, api.ErrBadRequest) || !strings.Contains(err.Error(), dmnErrAggregation) {
		t.Fatalf("COLLECT 聚合约束应先于输入求值返回 400 ENGINE-1921: %v", err)
	}
}

// TestDMNResultVariableRowsValue 验证多行结果变量的转换。
// @param t 测试上下文
func TestDMNResultVariableRowsValue(t *testing.T) {
	rows := [][]dmnOutputValue{
		{{ID: "result", Value: "a", Set: true}},
		{{ID: "result", Value: "b", Set: true}},
	}
	value, err := dmnResultVariableRowsValue(rows)
	if err != nil {
		t.Fatalf("转换多行结果失败: %v", err)
	}
	if !reflect.DeepEqual(value, []any{"a", "b"}) {
		t.Fatalf("多行单输出应返回切片: %#v", value)
	}

	multi := [][]dmnOutputValue{
		{{ID: "level", Value: "A", Set: true}, {ID: "note", Set: false}},
		{{ID: "level", Value: "B", Set: true}, {ID: "note", Value: "x", Set: true}},
	}
	value, err = dmnResultVariableRowsValue(multi)
	if err != nil {
		t.Fatalf("转换多行多输出失败: %v", err)
	}
	expected := []any{
		map[string]any{"level": "A"},
		map[string]any{"level": "B", "note": "x"},
	}
	if !reflect.DeepEqual(value, expected) {
		t.Fatalf("多行多输出转换错误: %#v", value)
	}

	single, err := dmnResultVariableRowsValue([][]dmnOutputValue{{{ID: "result", Value: 42, Set: true}}})
	if err != nil || single != 42 {
		t.Fatalf("单行结果应保持标量: %#v %v", single, err)
	}

	if _, err := dmnResultVariableRowsValue([][]dmnOutputValue{
		{{Set: true}, {Set: true}},
		{{Set: true}, {Set: true}},
	}); err == nil {
		t.Fatalf("多行多输出缺少名称应返回错误")
	}

	if _, err := dmnOutputVariableName(dmnOutputValue{}); err == nil {
		t.Fatalf("缺少 id 与 name 应返回错误")
	}
	if name, err := dmnOutputVariableName(dmnOutputValue{Name: "备注"}); err != nil || name != "备注" {
		t.Fatalf("变量名应回退 name: %q %v", name, err)
	}
}

// TestParseDMNOutputValue 验证单个顺序值的类型转换与错误。
// @param t 测试上下文
func TestParseDMNOutputValue(t *testing.T) {
	cases := []struct {
		name       string
		text       string
		outputType string
		want       any
		wantErr    bool
	}{
		{name: "数字类型", text: "10", outputType: "number", want: 10},
		{name: "数字类型带引号", text: `"10"`, outputType: "number", want: 10},
		{name: "数字类型非法", text: "abc", outputType: "number", wantErr: true},
		{name: "布尔类型真", text: "true", outputType: "boolean", want: true},
		{name: "布尔类型假", text: "false", outputType: "bool", want: false},
		{name: "布尔类型非法", text: "maybe", outputType: "boolean", wantErr: true},
		{name: "字符串类型", text: "high", outputType: "string", want: "high"},
		{name: "推断布尔", text: "true", want: true},
		{name: "推断布尔假", text: "false", want: false},
		{name: "推断数字", text: "1.5", want: 1.5},
		{name: "推断字符串", text: "plain", want: "plain"},
		{name: "引号未闭合", text: `"a`, outputType: "string", wantErr: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := parseDMNOutputValue(testCase.text, testCase.outputType)
			if testCase.wantErr {
				if err == nil {
					t.Fatalf("顺序值 %q 应返回错误", testCase.text)
				}
				return
			}
			if err != nil {
				t.Fatalf("解析顺序值失败: %v", err)
			}
			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("顺序值 %q 期望 %#v，实际 %#v", testCase.text, testCase.want, got)
			}
		})
	}
}

// TestEvaluateDMNTableRuleInputOverflow 验证规则输入条件多于输入列时返回 400。
// @param t 测试上下文
func TestEvaluateDMNTableRuleInputOverflow(t *testing.T) {
	table := &dmnTableModel{
		HitPolicy: "FIRST",
		Inputs:    []dmnInputModel{{Expression: "amount"}},
		Outputs:   []dmnOutputModel{{ID: "result"}},
		Rules:     []dmnRuleModel{{Inputs: []string{"-", "-"}, Outputs: []string{`"a"`}}},
	}
	_, err := (&Engine{}).evaluateDMNTable(context.Background(), table, map[string]any{"amount": 1})
	if !errors.Is(err, api.ErrBadRequest) || !strings.Contains(err.Error(), dmnErrInputEntry) {
		t.Fatalf("输入条件多于输入列应返回 400 ENGINE-1914: %v", err)
	}
}
