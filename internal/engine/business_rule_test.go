package engine

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// businessRuleBPMN 构造只包含开始、业务规则任务与结束事件的 BPMN。
// @param processID 流程 Key
// @param attributes businessRuleTask 上的决策属性
// @return BPMN XML
func businessRuleBPMN(processID, attributes string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="__PID__" name="Business Rule" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="flow1" sourceRef="start" targetRef="rule"/>
    <businessRuleTask id="rule" name="Rule" __ATTRS__/>
    <sequenceFlow id="flow2" sourceRef="rule" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`
}

// dmnTableDocument 构造包含单个决策与决策表的 DMN 文档。
// @param decisionID 决策 ID
// @param tableID 决策表 ID
// @param hitPolicy 命中策略，空串表示默认
// @param inputs 输入列 XML
// @param outputs 输出列 XML
// @param rules 规则 XML
// @return DMN XML
func dmnTableDocument(decisionID, tableID, hitPolicy, inputs, outputs, rules string) string {
	return dmnTableDocumentWithAggregation(decisionID, tableID, hitPolicy, "", inputs, outputs, rules)
}

// dmnTableDocumentWithAggregation 构造带聚合器属性的决策表 DMN 文档。
// @param decisionID 决策 ID
// @param tableID 决策表 ID
// @param hitPolicy 命中策略，空串表示默认
// @param aggregation 聚合器，空串表示不做聚合
// @param inputs 输入列 XML
// @param outputs 输出列 XML
// @param rules 规则 XML
// @return DMN XML
func dmnTableDocumentWithAggregation(decisionID, tableID, hitPolicy, aggregation, inputs, outputs, rules string) string {
	document := `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20191111/MODEL/">
  <decision id="__DID__" name="决策">
    <decisionTable id="__TID__" hitPolicy="__HIT__" aggregation="__AGG__">
      __INPUTS__
      __OUTPUTS__
      __RULES__
    </decisionTable>
  </decision>
</definitions>`
	document = strings.ReplaceAll(document, "__DID__", decisionID)
	document = strings.ReplaceAll(document, "__TID__", tableID)
	document = strings.ReplaceAll(document, "__HIT__", hitPolicy)
	document = strings.ReplaceAll(document, "__AGG__", aggregation)
	document = strings.ReplaceAll(document, "__INPUTS__", inputs)
	document = strings.ReplaceAll(document, "__OUTPUTS__", outputs)
	document = strings.ReplaceAll(document, "__RULES__", rules)
	return document
}

// dmnInputColumn 构造输入列。
// @param id 输入 ID
// @param expression 输入表达式
// @return 输入列 XML
func dmnInputColumn(id, expression string) string {
	column := `<input id="__ID__"><inputExpression><text>__EXPR__</text></inputExpression></input>`
	column = strings.ReplaceAll(column, "__ID__", id)
	column = strings.ReplaceAll(column, "__EXPR__", expression)
	return column
}

// dmnOutputColumn 构造输出列。
// @param id 输出 ID
// @param name 输出名称
// @return 输出列 XML
func dmnOutputColumn(id, name string) string {
	column := `<output id="__ID__" name="__NAME__" typeRef="string"/>`
	column = strings.ReplaceAll(column, "__ID__", id)
	column = strings.ReplaceAll(column, "__NAME__", name)
	return column
}

// dmnOutputColumnWithValues 构造带 outputValues 顺序值列表的输出列。
// @param id 输出 ID
// @param name 输出名称
// @param typeName 输出类型
// @param values 顺序值文本，多个值用逗号分隔
// @return 输出列 XML
func dmnOutputColumnWithValues(id, name, typeName, values string) string {
	column := `<output id="__ID__" name="__NAME__" typeRef="__TYPE__"><outputValues><text>__VALUES__</text></outputValues></output>`
	column = strings.ReplaceAll(column, "__ID__", id)
	column = strings.ReplaceAll(column, "__NAME__", name)
	column = strings.ReplaceAll(column, "__TYPE__", typeName)
	column = strings.ReplaceAll(column, "__VALUES__", values)
	return column
}

// dmnRuleXML 构造规则，输入与输出按文档顺序拼接。
// @param inputs 输入条件原文，调用方负责 XML 转义
// @param outputs 输出项原文
// @return 规则 XML
func dmnRuleXML(inputs, outputs []string) string {
	var builder strings.Builder
	builder.WriteString("<rule>")
	for _, input := range inputs {
		builder.WriteString("<inputEntry><text>")
		builder.WriteString(input)
		builder.WriteString("</text></inputEntry>")
	}
	for _, output := range outputs {
		builder.WriteString("<outputEntry><text>")
		builder.WriteString(output)
		builder.WriteString("</text></outputEntry>")
	}
	builder.WriteString("</rule>")
	return builder.String()
}

// deployBusinessRuleBundle 以 ZIP 部署 BPMN 与 DMN，并返回流程定义。
// @param t 测试上下文
// @param engine 引擎
// @param processID 流程 Key
// @param attributes 业务规则任务属性
// @param dmn DMN 内容，空串表示不部署 DMN
// @return 流程定义
func deployBusinessRuleBundle(t *testing.T, engine *Engine, processID, attributes, dmn string) api.ProcessDefinition {
	t.Helper()
	bpmn := businessRuleBPMN(processID, attributes)
	bpmn = strings.ReplaceAll(bpmn, "__PID__", processID)
	bpmn = strings.ReplaceAll(bpmn, "__ATTRS__", attributes)
	entries := map[string][]byte{"processes/rule.bpmn20.xml": []byte(bpmn)}
	if dmn != "" {
		entries["decisions/rule.dmn"] = []byte(dmn)
	}
	if _, err := engine.Deploy(context.Background(), api.DeploymentRequest{
		Name:      "business-rule-bundle",
		Resources: map[string][]byte{"bundle.zip": makeZip(t, entries)},
	}); err != nil {
		t.Fatalf("部署业务规则资源失败: %v", err)
	}
	definition, err := engine.GetProcessDefinitionByKey(context.Background(), processID)
	if err != nil {
		t.Fatalf("查询流程定义失败: %v", err)
	}
	return definition
}

// startBusinessRule 启动业务规则流程。
// @param t 测试上下文
// @param engine 引擎
// @param processID 流程 Key
// @param variables 启动变量
// @return 流程实例
// @return 启动错误
func startBusinessRule(t *testing.T, engine *Engine, processID string, variables ...api.Variable) (api.ProcessInstance, error) {
	t.Helper()
	return engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: processID,
		Variables:            variables,
	})
}

// requireBusinessRuleError 校验业务规则任务返回指定 400 错误码。
// @param t 测试上下文
// @param err 错误
// @param code 期望错误码
func requireBusinessRuleError(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("期望业务规则任务失败并返回 %s", code)
	}
	if !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("业务规则任务错误应映射为 400: %v", err)
	}
	if !strings.Contains(err.Error(), code) {
		t.Fatalf("业务规则任务错误码期望 %s，实际 %v", code, err)
	}
}

// TestBusinessRuleTask_ResultVariable 验证结果变量、默认 FIRST 与任意条件。
// @param t 测试上下文
func TestBusinessRuleTask_ResultVariable(t *testing.T) {
	engine := newTestEngine(t)
	dmn := dmnTableDocument("decisionGrade", "tableGrade", "", dmnInputColumn("amountInput", "amount"), dmnOutputColumn("gradeOutput", "grade"),
		dmnRuleXML([]string{"> 100"}, []string{"\"high\""})+dmnRuleXML([]string{"-"}, []string{"\"low\""}))
	deployBusinessRuleBundle(t, engine, "gradeProcess", `flowable:decisionRef="decisionGrade" flowable:resultVariable="grade"`, dmn)

	instance, err := startBusinessRule(t, engine, "gradeProcess", api.Variable{Name: "amount", Value: 150})
	if err != nil {
		t.Fatalf("启动流程失败: %v", err)
	}
	if !instance.Ended {
		t.Fatalf("业务规则任务后流程应结束")
	}
	if instance.Variables["grade"] != "high" {
		t.Fatalf("结果变量错误: %+v", instance.Variables)
	}

	second, err := startBusinessRule(t, engine, "gradeProcess", api.Variable{Name: "amount", Value: 50})
	if err != nil {
		t.Fatalf("第二次启动流程失败: %v", err)
	}
	if second.Variables["grade"] != "low" {
		t.Fatalf("任意条件规则结果错误: %+v", second.Variables)
	}
}

// TestBusinessRuleTask_UnprefixedAttributesAndTableID 验证无前缀属性与决策表 ID 引用。
// @param t 测试上下文
func TestBusinessRuleTask_UnprefixedAttributesAndTableID(t *testing.T) {
	engine := newTestEngine(t)
	dmn := dmnTableDocument("decisionPlain", "tablePlain", "FIRST", dmnInputColumn("amountInput", "amount"), dmnOutputColumn("verdict", "verdict"),
		dmnRuleXML([]string{">= 0"}, []string{"\"ok\""}))
	deployBusinessRuleBundle(t, engine, "plainProcess", `decisionRef="tablePlain" resultVariable="verdict"`, dmn)

	instance, err := startBusinessRule(t, engine, "plainProcess", api.Variable{Name: "amount", Value: 1})
	if err != nil {
		t.Fatalf("启动流程失败: %v", err)
	}
	if instance.Variables["verdict"] != "ok" {
		t.Fatalf("无前缀属性或决策表 ID 引用错误: %+v", instance.Variables)
	}
}

// TestBusinessRuleTask_MultiOutput 验证无结果变量时按输出 id/name 写入变量。
// @param t 测试上下文
func TestBusinessRuleTask_MultiOutput(t *testing.T) {
	engine := newTestEngine(t)
	outputs := dmnOutputColumn("level", "等级") + dmnOutputColumn("", "note")
	dmn := dmnTableDocument("decisionLevel", "tableLevel", "FIRST", dmnInputColumn("scoreInput", "score"), outputs,
		dmnRuleXML([]string{"[0..10]"}, []string{"\"A\"", "\"ok\""})+
			dmnRuleXML([]string{"(10..20]"}, []string{"\"B\"", "-"}))
	deployBusinessRuleBundle(t, engine, "levelProcess", `flowable:decisionRef="decisionLevel"`, dmn)

	first, err := startBusinessRule(t, engine, "levelProcess",
		api.Variable{Name: "score", Value: 5}, api.Variable{Name: "note", Value: "keep"})
	if err != nil {
		t.Fatalf("启动流程失败: %v", err)
	}
	if first.Variables["level"] != "A" || first.Variables["note"] != "ok" {
		t.Fatalf("多输出变量错误: %+v", first.Variables)
	}

	second, err := startBusinessRule(t, engine, "levelProcess",
		api.Variable{Name: "score", Value: 15}, api.Variable{Name: "note", Value: "keep"})
	if err != nil {
		t.Fatalf("第二次启动流程失败: %v", err)
	}
	if second.Variables["level"] != "B" {
		t.Fatalf("第二个区间结果错误: %+v", second.Variables)
	}
	// "-" 输出跳过，不覆盖已有变量
	if second.Variables["note"] != "keep" {
		t.Fatalf("跳过输出不应覆盖变量: %+v", second.Variables)
	}
}

// TestBusinessRuleTask_MultiOutputResultVariable 验证多输出写入结果变量时使用映射。
// @param t 测试上下文
func TestBusinessRuleTask_MultiOutputResultVariable(t *testing.T) {
	engine := newTestEngine(t)
	outputs := dmnOutputColumn("level", "等级") + dmnOutputColumn("note", "备注")
	dmn := dmnTableDocument("decisionMap", "tableMap", "FIRST", dmnInputColumn("amountInput", "amount"), outputs,
		dmnRuleXML([]string{"> 0"}, []string{"\"A\"", "\"ok\""}))
	deployBusinessRuleBundle(t, engine, "mapProcess", `flowable:decisionRef="decisionMap" flowable:resultVariable="decision"`, dmn)

	instance, err := startBusinessRule(t, engine, "mapProcess", api.Variable{Name: "amount", Value: 1})
	if err != nil {
		t.Fatalf("启动流程失败: %v", err)
	}
	expected := map[string]any{"level": "A", "note": "ok"}
	if !reflect.DeepEqual(instance.Variables["decision"], expected) {
		t.Fatalf("多输出结果变量错误: %#v", instance.Variables["decision"])
	}
}

// TestBusinessRuleTask_HitPolicies 验证 FIRST、UNIQUE、ANY、RULE ORDER 与不支持的策略。
// @param t 测试上下文
func TestBusinessRuleTask_HitPolicies(t *testing.T) {
	t.Run("FIRST 默认策略", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocument("decisionFirst", "tableFirst", "", dmnInputColumn("amountInput", "amount"), dmnOutputColumn("result", "result"),
			dmnRuleXML([]string{"-"}, []string{"\"first\""})+dmnRuleXML([]string{"-"}, []string{"\"second\""}))
		deployBusinessRuleBundle(t, engine, "firstProcess", `flowable:decisionRef="decisionFirst" flowable:resultVariable="result"`, dmn)
		instance, err := startBusinessRule(t, engine, "firstProcess", api.Variable{Name: "amount", Value: 1})
		if err != nil {
			t.Fatalf("启动流程失败: %v", err)
		}
		if instance.Variables["result"] != "first" {
			t.Fatalf("FIRST 策略应返回首条匹配: %+v", instance.Variables)
		}
	})

	t.Run("RULE ORDER 返回全部匹配", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocument("decisionOrder", "tableOrder", "RULE ORDER", dmnInputColumn("amountInput", "amount"), dmnOutputColumn("result", "result"),
			dmnRuleXML([]string{"-"}, []string{"\"first\""})+dmnRuleXML([]string{"-"}, []string{"\"second\""}))
		deployBusinessRuleBundle(t, engine, "orderProcess", `flowable:decisionRef="decisionOrder" flowable:resultVariable="result"`, dmn)
		instance, err := startBusinessRule(t, engine, "orderProcess", api.Variable{Name: "amount", Value: 1})
		if err != nil {
			t.Fatalf("启动流程失败: %v", err)
		}
		// RULE ORDER 按文档顺序返回全部匹配结果
		expected := []any{"first", "second"}
		if !reflect.DeepEqual(instance.Variables["result"], expected) {
			t.Fatalf("RULE ORDER 策略应按文档顺序返回全部匹配: %#v", instance.Variables["result"])
		}
	})

	t.Run("UNIQUE 单条命中", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocument("decisionUnique", "tableUnique", "UNIQUE", dmnInputColumn("amountInput", "amount"), dmnOutputColumn("result", "result"),
			dmnRuleXML([]string{"> 100"}, []string{"\"high\""})+dmnRuleXML([]string{"[0..100]"}, []string{"\"low\""}))
		deployBusinessRuleBundle(t, engine, "uniqueProcess", `flowable:decisionRef="decisionUnique" flowable:resultVariable="result"`, dmn)
		instance, err := startBusinessRule(t, engine, "uniqueProcess", api.Variable{Name: "amount", Value: 150})
		if err != nil {
			t.Fatalf("启动流程失败: %v", err)
		}
		if instance.Variables["result"] != "high" {
			t.Fatalf("UNIQUE 单条命中结果错误: %+v", instance.Variables)
		}
	})

	t.Run("UNIQUE 多条命中冲突", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocument("decisionUniqueConflict", "tableUniqueConflict", "UNIQUE", dmnInputColumn("amountInput", "amount"), dmnOutputColumn("result", "result"),
			dmnRuleXML([]string{"-"}, []string{"\"a\""})+dmnRuleXML([]string{"-"}, []string{"\"a\""}))
		deployBusinessRuleBundle(t, engine, "uniqueConflictProcess", `flowable:decisionRef="decisionUniqueConflict" flowable:resultVariable="result"`, dmn)
		_, err := startBusinessRule(t, engine, "uniqueConflictProcess", api.Variable{Name: "amount", Value: 1})
		requireBusinessRuleError(t, err, "ENGINE-1916")
		instances, listErr := engine.ListProcessInstances(context.Background())
		if listErr != nil || len(instances) != 0 {
			t.Fatalf("求值失败应回滚实例: %v %+v", listErr, instances)
		}
	})

	t.Run("ANY 输出一致", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocument("decisionAny", "tableAny", "ANY", dmnInputColumn("amountInput", "amount"), dmnOutputColumn("result", "result"),
			dmnRuleXML([]string{"-"}, []string{"\"same\""})+dmnRuleXML([]string{"-"}, []string{"\"same\""}))
		deployBusinessRuleBundle(t, engine, "anyProcess", `flowable:decisionRef="decisionAny" flowable:resultVariable="result"`, dmn)
		instance, err := startBusinessRule(t, engine, "anyProcess", api.Variable{Name: "amount", Value: 1})
		if err != nil {
			t.Fatalf("启动流程失败: %v", err)
		}
		if instance.Variables["result"] != "same" {
			t.Fatalf("ANY 一致输出错误: %+v", instance.Variables)
		}
	})

	t.Run("ANY 输出不一致", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocument("decisionAnyConflict", "tableAnyConflict", "ANY", dmnInputColumn("amountInput", "amount"), dmnOutputColumn("result", "result"),
			dmnRuleXML([]string{"-"}, []string{"\"a\""})+dmnRuleXML([]string{"-"}, []string{"\"b\""}))
		deployBusinessRuleBundle(t, engine, "anyConflictProcess", `flowable:decisionRef="decisionAnyConflict" flowable:resultVariable="result"`, dmn)
		_, err := startBusinessRule(t, engine, "anyConflictProcess", api.Variable{Name: "amount", Value: 1})
		requireBusinessRuleError(t, err, "ENGINE-1917")
	})

	t.Run("ANY 无匹配", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocument("decisionAnyEmpty", "tableAnyEmpty", "ANY", dmnInputColumn("amountInput", "amount"), dmnOutputColumn("result", "result"),
			dmnRuleXML([]string{"> 100"}, []string{"\"high\""}))
		deployBusinessRuleBundle(t, engine, "anyEmptyProcess", `flowable:decisionRef="decisionAnyEmpty" flowable:resultVariable="result"`, dmn)
		_, err := startBusinessRule(t, engine, "anyEmptyProcess", api.Variable{Name: "amount", Value: 1})
		requireBusinessRuleError(t, err, "ENGINE-1915")
	})

	t.Run("不支持 UNORDERED", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocument("decisionUnordered", "tableUnordered", "UNORDERED", dmnInputColumn("amountInput", "amount"), dmnOutputColumn("result", "result"),
			dmnRuleXML([]string{"-"}, []string{"\"a\""}))
		deployBusinessRuleBundle(t, engine, "unorderedProcess", `flowable:decisionRef="decisionUnordered" flowable:resultVariable="result"`, dmn)
		_, err := startBusinessRule(t, engine, "unorderedProcess", api.Variable{Name: "amount", Value: 1})
		requireBusinessRuleError(t, err, "ENGINE-1918")
	})
}

// TestBusinessRuleTask_InputConditions 验证字符串、布尔、区间与比较条件。
// @param t 测试上下文
func TestBusinessRuleTask_InputConditions(t *testing.T) {
	engine := newTestEngine(t)
	inputs := dmnInputColumn("vipInput", "vip") + dmnInputColumn("typeInput", "type") + dmnInputColumn("amountInput", "amount")
	outputs := dmnOutputColumn("decision", "decision")
	rules := dmnRuleXML([]string{"true", "\"gold\"", "-"}, []string{"\"vip-gold\""}) +
		dmnRuleXML([]string{"-", "\"gold\", 'silver'", "[0..100]"}, []string{"\"premium\""}) +
		dmnRuleXML([]string{"-", "-", "(100..200]"}, []string{"\"standard\""}) +
		dmnRuleXML([]string{"-", "-", "> 200"}, []string{"\"high\""}) +
		dmnRuleXML([]string{"-", "-", "-"}, []string{"\"basic\""})
	dmn := dmnTableDocument("decisionConditions", "tableConditions", "FIRST", inputs, outputs, rules)
	deployBusinessRuleBundle(t, engine, "conditionsProcess", `flowable:decisionRef="decisionConditions" flowable:resultVariable="decision"`, dmn)

	cases := []struct {
		name     string
		vip      bool
		kind     string
		amount   any
		expected string
	}{
		{name: "布尔与字符串", vip: true, kind: "gold", amount: 1, expected: "vip-gold"},
		{name: "单引号多值", vip: false, kind: "silver", amount: 50, expected: "premium"},
		{name: "闭区间内部", vip: false, kind: "bronze", amount: 150, expected: "standard"},
		{name: "大于比较", vip: false, kind: "bronze", amount: 250, expected: "high"},
		{name: "任意兜底", vip: false, kind: "bronze", amount: 0, expected: "basic"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			instance, err := startBusinessRule(t, engine, "conditionsProcess",
				api.Variable{Name: "vip", Value: testCase.vip},
				api.Variable{Name: "type", Value: testCase.kind},
				api.Variable{Name: "amount", Value: testCase.amount})
			if err != nil {
				t.Fatalf("启动流程失败: %v", err)
			}
			if instance.Variables["decision"] != testCase.expected {
				t.Fatalf("决策结果期望 %s，实际 %+v", testCase.expected, instance.Variables)
			}
		})
	}

	// 非数值输入与数字比较不命中，落到兜底规则
	instance, err := startBusinessRule(t, engine, "conditionsProcess",
		api.Variable{Name: "vip", Value: false},
		api.Variable{Name: "type", Value: "other"},
		api.Variable{Name: "amount", Value: "abc"})
	if err != nil {
		t.Fatalf("非数值输入启动失败: %v", err)
	}
	if instance.Variables["decision"] != "basic" {
		t.Fatalf("非数值输入应落到兜底规则: %+v", instance.Variables)
	}
}

// TestBusinessRuleTask_OutputConversions 验证输出字符串、数字、布尔、跳过与表达式。
// @param t 测试上下文
func TestBusinessRuleTask_OutputConversions(t *testing.T) {
	engine := newTestEngine(t)
	outputs := dmnOutputColumn("text", "文本") + dmnOutputColumn("number", "数字") +
		dmnOutputColumn("flag", "布尔") + dmnOutputColumn("skipped", "跳过") + dmnOutputColumn("dynamic", "动态")
	dmn := dmnTableDocument("decisionOutputs", "tableOutputs", "FIRST", dmnInputColumn("amountInput", "amount"), outputs,
		dmnRuleXML([]string{"-"}, []string{"\"hello\"", "42", "true", "-", "${amount * 2}"}))
	deployBusinessRuleBundle(t, engine, "outputsProcess", `flowable:decisionRef="decisionOutputs"`, dmn)

	instance, err := startBusinessRule(t, engine, "outputsProcess",
		api.Variable{Name: "amount", Value: 7}, api.Variable{Name: "skipped", Value: "keep"})
	if err != nil {
		t.Fatalf("启动流程失败: %v", err)
	}
	if instance.Variables["text"] != "hello" {
		t.Fatalf("字符串输出错误: %+v", instance.Variables)
	}
	if instance.Variables["number"] != 42 {
		t.Fatalf("数字输出错误: %+v", instance.Variables)
	}
	if instance.Variables["flag"] != true {
		t.Fatalf("布尔输出错误: %+v", instance.Variables)
	}
	if instance.Variables["dynamic"] != 14.0 {
		t.Fatalf("表达式输出错误: %+v", instance.Variables)
	}
	if instance.Variables["skipped"] != "keep" {
		t.Fatalf("跳过输出不应覆盖已有变量: %+v", instance.Variables)
	}
}

// TestBusinessRuleTask_Errors 验证缺失决策、非法表结构与表达式错误。
// @param t 测试上下文
func TestBusinessRuleTask_Errors(t *testing.T) {
	t.Run("缺少 decisionRef", func(t *testing.T) {
		engine := newTestEngine(t)
		deployBusinessRuleBundle(t, engine, "missingRefProcess", `flowable:resultVariable="result"`, "")
		_, err := startBusinessRule(t, engine, "missingRefProcess")
		requireBusinessRuleError(t, err, "ENGINE-1910")
	})

	t.Run("部署内无 DMN 资源", func(t *testing.T) {
		engine := newTestEngine(t)
		deployBusinessRuleBundle(t, engine, "noDMNProcess", `flowable:decisionRef="missing"`, "")
		_, err := startBusinessRule(t, engine, "noDMNProcess")
		requireBusinessRuleError(t, err, "ENGINE-1911")
	})

	t.Run("决策不存在", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocument("otherDecision", "otherTable", "FIRST", dmnInputColumn("amountInput", "amount"), dmnOutputColumn("result", "result"),
			dmnRuleXML([]string{"-"}, []string{"\"x\""}))
		deployBusinessRuleBundle(t, engine, "unknownDecisionProcess", `flowable:decisionRef="missing"`, dmn)
		_, err := startBusinessRule(t, engine, "unknownDecisionProcess")
		requireBusinessRuleError(t, err, "ENGINE-1911")
	})

	t.Run("决策没有决策表", func(t *testing.T) {
		engine := newTestEngine(t)
		const dmn = `<definitions><decision id="noTable"/></definitions>`
		deployBusinessRuleBundle(t, engine, "noTableProcess", `flowable:decisionRef="noTable"`, dmn)
		_, err := startBusinessRule(t, engine, "noTableProcess")
		requireBusinessRuleError(t, err, "ENGINE-1911")
	})

	t.Run("决策表没有输出列", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocument("decisionNoOutput", "tableNoOutput", "FIRST", dmnInputColumn("amountInput", "amount"), "",
			dmnRuleXML([]string{"-"}, []string{}))
		deployBusinessRuleBundle(t, engine, "noOutputProcess", `flowable:decisionRef="decisionNoOutput"`, dmn)
		_, err := startBusinessRule(t, engine, "noOutputProcess")
		requireBusinessRuleError(t, err, "ENGINE-1912")
	})

	t.Run("部署不存在", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocument("decisionGone", "tableGone", "FIRST", dmnInputColumn("amountInput", "amount"), dmnOutputColumn("result", "result"),
			dmnRuleXML([]string{"-"}, []string{"\"x\""}))
		definition := deployBusinessRuleBundle(t, engine, "goneProcess", `flowable:decisionRef="decisionGone"`, dmn)
		delete(engine.state.Deployments, definition.DeploymentID)
		_, err := startBusinessRule(t, engine, "goneProcess")
		requireBusinessRuleError(t, err, "ENGINE-1911")
	})

	t.Run("输入表达式语法错误", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocument("decisionBadInput", "tableBadInput", "FIRST", dmnInputColumn("amountInput", "amount +"), dmnOutputColumn("result", "result"),
			dmnRuleXML([]string{"-"}, []string{"\"x\""}))
		deployBusinessRuleBundle(t, engine, "badInputProcess", `flowable:decisionRef="decisionBadInput"`, dmn)
		_, err := startBusinessRule(t, engine, "badInputProcess")
		requireBusinessRuleError(t, err, "ENGINE-1913")
	})

	t.Run("输入条件非法", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocument("decisionBadEntry", "tableBadEntry", "FIRST", dmnInputColumn("amountInput", "amount"), dmnOutputColumn("result", "result"),
			dmnRuleXML([]string{"\"unterminated"}, []string{"\"x\""}))
		deployBusinessRuleBundle(t, engine, "badEntryProcess", `flowable:decisionRef="decisionBadEntry"`, dmn)
		_, err := startBusinessRule(t, engine, "badEntryProcess")
		requireBusinessRuleError(t, err, "ENGINE-1914")
	})

	t.Run("输出表达式非法", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocument("decisionBadOutput", "tableBadOutput", "FIRST", dmnInputColumn("amountInput", "amount"), dmnOutputColumn("result", "result"),
			dmnRuleXML([]string{"-"}, []string{"${amount +}"}))
		deployBusinessRuleBundle(t, engine, "badOutputProcess", `flowable:decisionRef="decisionBadOutput"`, dmn)
		_, err := startBusinessRule(t, engine, "badOutputProcess")
		requireBusinessRuleError(t, err, "ENGINE-1919")
	})

	t.Run("输出列缺少名称", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocument("decisionNoName", "tableNoName", "FIRST", dmnInputColumn("amountInput", "amount"), `<output/>`,
			dmnRuleXML([]string{"-"}, []string{"\"x\""}))
		deployBusinessRuleBundle(t, engine, "noNameProcess", `flowable:decisionRef="decisionNoName"`, dmn)
		_, err := startBusinessRule(t, engine, "noNameProcess")
		requireBusinessRuleError(t, err, "ENGINE-1920")
	})

	t.Run("无匹配规则", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocument("decisionNoMatch", "tableNoMatch", "FIRST", dmnInputColumn("amountInput", "amount"), dmnOutputColumn("result", "result"),
			dmnRuleXML([]string{"> 100"}, []string{"\"high\""}))
		deployBusinessRuleBundle(t, engine, "noMatchProcess", `flowable:decisionRef="decisionNoMatch"`, dmn)
		_, err := startBusinessRule(t, engine, "noMatchProcess", api.Variable{Name: "amount", Value: 1})
		requireBusinessRuleError(t, err, "ENGINE-1915")
	})
}

// TestBusinessRuleTask_PlainProcessUnchanged 验证普通流程仍停在用户任务。
// @param t 测试上下文
func TestBusinessRuleTask_PlainProcessUnchanged(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	instance, err := startBusinessRule(t, engine, "simple", api.Variable{Name: "initiator", Value: "alice"})
	if err != nil {
		t.Fatalf("普通流程启动失败: %v", err)
	}
	if instance.Ended {
		t.Fatalf("普通流程应停在用户任务")
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "approve" || tasks[0].Assignee != "alice" {
		t.Fatalf("普通流程任务行为发生变化: %+v", tasks)
	}
}

// TestBusinessRuleTask_RuleOrderAllMatches 验证 RULE ORDER 无结果变量时逐列写入全部匹配值。
// @param t 测试上下文
func TestBusinessRuleTask_RuleOrderAllMatches(t *testing.T) {
	engine := newTestEngine(t)
	outputs := dmnOutputColumn("level", "等级") + dmnOutputColumn("note", "备注")
	dmn := dmnTableDocument("decisionRuleAll", "tableRuleAll", "RULE ORDER",
		dmnInputColumn("amountInput", "amount"), outputs,
		dmnRuleXML([]string{"> 0"}, []string{`"A"`, `"first"`})+
			dmnRuleXML([]string{"> 10"}, []string{`"B"`, "-"})+
			dmnRuleXML([]string{"-"}, []string{`"C"`, `"last"`}))
	deployBusinessRuleBundle(t, engine, "ruleAllProcess", `flowable:decisionRef="decisionRuleAll"`, dmn)

	instance, err := startBusinessRule(t, engine, "ruleAllProcess", api.Variable{Name: "amount", Value: 50})
	if err != nil {
		t.Fatalf("启动流程失败: %v", err)
	}
	if !reflect.DeepEqual(instance.Variables["level"], []any{"A", "B", "C"}) {
		t.Fatalf("RULE ORDER 多结果输出列错误: %#v", instance.Variables["level"])
	}
	// 未产生值的输出项被跳过，不写入空值
	if !reflect.DeepEqual(instance.Variables["note"], []any{"first", "last"}) {
		t.Fatalf("RULE ORDER 跳过未产生值错误: %#v", instance.Variables["note"])
	}
}

// TestBusinessRuleTask_Collect 验证 COLLECT 的收集与 SUM/MIN/MAX/COUNT 聚合。
// @param t 测试上下文
func TestBusinessRuleTask_Collect(t *testing.T) {
	t.Run("无聚合器返回全部匹配", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocument("decisionCollectAll", "tableCollectAll", "COLLECT",
			dmnInputColumn("amountInput", "amount"), dmnOutputColumn("result", "结果"),
			dmnRuleXML([]string{"-"}, []string{`"a"`})+
				dmnRuleXML([]string{"-"}, []string{`"b"`})+
				dmnRuleXML([]string{"> 100"}, []string{`"c"`}))
		deployBusinessRuleBundle(t, engine, "collectAllProcess", `flowable:decisionRef="decisionCollectAll" flowable:resultVariable="result"`, dmn)
		instance, err := startBusinessRule(t, engine, "collectAllProcess", api.Variable{Name: "amount", Value: 1})
		if err != nil {
			t.Fatalf("启动流程失败: %v", err)
		}
		if !reflect.DeepEqual(instance.Variables["result"], []any{"a", "b"}) {
			t.Fatalf("COLLECT 无聚合器应返回全部匹配: %#v", instance.Variables["result"])
		}
	})

	aggregations := []struct {
		name        string
		aggregation string
		want        float64
	}{
		{name: "SUM 聚合", aggregation: "SUM", want: 60},
		{name: "MIN 聚合", aggregation: "MIN", want: 10},
		{name: "MAX 聚合", aggregation: "MAX", want: 30},
		{name: "COUNT 聚合", aggregation: "COUNT", want: 3},
	}
	for _, testCase := range aggregations {
		t.Run(testCase.name, func(t *testing.T) {
			engine := newTestEngine(t)
			outputs := dmnOutputColumnWithValues("total", "合计", "number", "10,20,30")
			dmn := dmnTableDocumentWithAggregation("decisionAggregate", "tableAggregate", "COLLECT", testCase.aggregation,
				dmnInputColumn("amountInput", "amount"), outputs,
				dmnRuleXML([]string{"-"}, []string{"10"})+
					dmnRuleXML([]string{"-"}, []string{"20"})+
					dmnRuleXML([]string{"-"}, []string{"30"}))
			processID := "aggregate" + strings.ToLower(testCase.aggregation)
			deployBusinessRuleBundle(t, engine, processID, `flowable:decisionRef="decisionAggregate" flowable:resultVariable="total"`, dmn)
			instance, err := startBusinessRule(t, engine, processID, api.Variable{Name: "amount", Value: 1})
			if err != nil {
				t.Fatalf("启动流程失败: %v", err)
			}
			// 聚合结果统一为 float64，与 Flowable 的 Double 一致
			if !reflect.DeepEqual(instance.Variables["total"], testCase.want) {
				t.Fatalf("聚合结果期望 %v，实际 %#v", testCase.want, instance.Variables["total"])
			}
		})
	}

	t.Run("多输出聚合报错", func(t *testing.T) {
		engine := newTestEngine(t)
		outputs := dmnOutputColumnWithValues("first", "第一列", "number", "1,2") +
			dmnOutputColumnWithValues("second", "第二列", "number", "1,2")
		dmn := dmnTableDocumentWithAggregation("decisionMultiAggregate", "tableMultiAggregate", "COLLECT", "SUM",
			dmnInputColumn("amountInput", "amount"), outputs,
			dmnRuleXML([]string{"-"}, []string{"1", "2"}))
		deployBusinessRuleBundle(t, engine, "multiAggregateProcess", `flowable:decisionRef="decisionMultiAggregate" flowable:resultVariable="decision"`, dmn)
		_, err := startBusinessRule(t, engine, "multiAggregateProcess", api.Variable{Name: "amount", Value: 1})
		requireBusinessRuleError(t, err, "ENGINE-1921")
	})

	t.Run("COUNT 非数值输出报错", func(t *testing.T) {
		engine := newTestEngine(t)
		outputs := dmnOutputColumnWithValues("total", "合计", "number", "1,2")
		dmn := dmnTableDocumentWithAggregation("decisionCountText", "tableCountText", "COLLECT", "COUNT",
			dmnInputColumn("amountInput", "amount"), outputs,
			dmnRuleXML([]string{"-"}, []string{`"a"`}))
		deployBusinessRuleBundle(t, engine, "countTextProcess", `flowable:decisionRef="decisionCountText" flowable:resultVariable="total"`, dmn)
		_, err := startBusinessRule(t, engine, "countTextProcess", api.Variable{Name: "amount", Value: 1})
		requireBusinessRuleError(t, err, "ENGINE-1921")
	})

	t.Run("非数值聚合报错", func(t *testing.T) {
		engine := newTestEngine(t)
		outputs := dmnOutputColumnWithValues("total", "合计", "number", "1,2")
		dmn := dmnTableDocumentWithAggregation("decisionSumText", "tableSumText", "COLLECT", "SUM",
			dmnInputColumn("amountInput", "amount"), outputs,
			dmnRuleXML([]string{"-"}, []string{`"a"`}))
		deployBusinessRuleBundle(t, engine, "sumTextProcess", `flowable:decisionRef="decisionSumText" flowable:resultVariable="total"`, dmn)
		_, err := startBusinessRule(t, engine, "sumTextProcess", api.Variable{Name: "amount", Value: 1})
		requireBusinessRuleError(t, err, "ENGINE-1921")
	})

	t.Run("单输出类型不是 number 聚合报错", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocumentWithAggregation("decisionStringAggregate", "tableStringAggregate", "COLLECT", "SUM",
			dmnInputColumn("amountInput", "amount"), dmnOutputColumn("total", "合计"),
			dmnRuleXML([]string{"-"}, []string{"1"}))
		deployBusinessRuleBundle(t, engine, "stringAggregateProcess", `flowable:decisionRef="decisionStringAggregate" flowable:resultVariable="total"`, dmn)
		_, err := startBusinessRule(t, engine, "stringAggregateProcess", api.Variable{Name: "amount", Value: 1})
		requireBusinessRuleError(t, err, "ENGINE-1921")
	})

	t.Run("单输出缺少类型聚合报错", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocumentWithAggregation("decisionNoTypeAggregate", "tableNoTypeAggregate", "COLLECT", "COUNT",
			dmnInputColumn("amountInput", "amount"), `<output id="total" name="合计"/>`,
			dmnRuleXML([]string{"-"}, []string{"1"}))
		deployBusinessRuleBundle(t, engine, "noTypeAggregateProcess", `flowable:decisionRef="decisionNoTypeAggregate" flowable:resultVariable="total"`, dmn)
		_, err := startBusinessRule(t, engine, "noTypeAggregateProcess", api.Variable{Name: "amount", Value: 1})
		requireBusinessRuleError(t, err, "ENGINE-1921")
	})

	t.Run("非法聚合器", func(t *testing.T) {
		engine := newTestEngine(t)
		outputs := dmnOutputColumn("result", "结果")
		dmn := dmnTableDocumentWithAggregation("decisionBadAggregation", "tableBadAggregation", "COLLECT", "AVG",
			dmnInputColumn("amountInput", "amount"), outputs,
			dmnRuleXML([]string{"-"}, []string{"1"}))
		deployBusinessRuleBundle(t, engine, "badAggregationProcess", `flowable:decisionRef="decisionBadAggregation" flowable:resultVariable="result"`, dmn)
		_, err := startBusinessRule(t, engine, "badAggregationProcess", api.Variable{Name: "amount", Value: 1})
		requireBusinessRuleError(t, err, "ENGINE-1921")
	})

	t.Run("无匹配", func(t *testing.T) {
		engine := newTestEngine(t)
		outputs := dmnOutputColumn("result", "结果")
		dmn := dmnTableDocument("decisionCollectEmpty", "tableCollectEmpty", "COLLECT",
			dmnInputColumn("amountInput", "amount"), outputs,
			dmnRuleXML([]string{"> 100"}, []string{`"a"`}))
		deployBusinessRuleBundle(t, engine, "collectEmptyProcess", `flowable:decisionRef="decisionCollectEmpty" flowable:resultVariable="result"`, dmn)
		_, err := startBusinessRule(t, engine, "collectEmptyProcess", api.Variable{Name: "amount", Value: 1})
		requireBusinessRuleError(t, err, "ENGINE-1915")
	})
}

// TestBusinessRuleTask_OutputOrderAndPriority 验证 OUTPUT ORDER 与 PRIORITY 的顺序值排序。
// @param t 测试上下文
func TestBusinessRuleTask_OutputOrderAndPriority(t *testing.T) {
	orderedRules := dmnRuleXML([]string{"-"}, []string{`"medium"`}) +
		dmnRuleXML([]string{"-"}, []string{`"low"`}) +
		dmnRuleXML([]string{"-"}, []string{`"high"`})
	levelOutput := dmnOutputColumnWithValues("level", "等级", "string", "high,medium,low")

	t.Run("OUTPUT ORDER 返回全部并按顺序值排序", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocument("decisionOutputOrder", "tableOutputOrder", "OUTPUT ORDER",
			dmnInputColumn("amountInput", "amount"), levelOutput, orderedRules)
		deployBusinessRuleBundle(t, engine, "outputOrderProcess", `flowable:decisionRef="decisionOutputOrder" flowable:resultVariable="level"`, dmn)
		instance, err := startBusinessRule(t, engine, "outputOrderProcess", api.Variable{Name: "amount", Value: 1})
		if err != nil {
			t.Fatalf("启动流程失败: %v", err)
		}
		if !reflect.DeepEqual(instance.Variables["level"], []any{"high", "medium", "low"}) {
			t.Fatalf("OUTPUT ORDER 排序错误: %#v", instance.Variables["level"])
		}
	})

	t.Run("PRIORITY 取排序后第一条", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocument("decisionPriority", "tablePriority", "PRIORITY",
			dmnInputColumn("amountInput", "amount"), levelOutput, orderedRules)
		deployBusinessRuleBundle(t, engine, "priorityProcess", `flowable:decisionRef="decisionPriority" flowable:resultVariable="level"`, dmn)
		instance, err := startBusinessRule(t, engine, "priorityProcess", api.Variable{Name: "amount", Value: 1})
		if err != nil {
			t.Fatalf("启动流程失败: %v", err)
		}
		if instance.Variables["level"] != "high" {
			t.Fatalf("PRIORITY 应取顺序值最前的规则: %#v", instance.Variables["level"])
		}
	})

	t.Run("PRIORITY 无匹配", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocument("decisionPriorityEmpty", "tablePriorityEmpty", "PRIORITY",
			dmnInputColumn("amountInput", "amount"), levelOutput,
			dmnRuleXML([]string{"> 100"}, []string{`"high"`}))
		deployBusinessRuleBundle(t, engine, "priorityEmptyProcess", `flowable:decisionRef="decisionPriorityEmpty" flowable:resultVariable="level"`, dmn)
		_, err := startBusinessRule(t, engine, "priorityEmptyProcess", api.Variable{Name: "amount", Value: 1})
		requireBusinessRuleError(t, err, "ENGINE-1915")
	})

	t.Run("按输出列顺序依次比较", func(t *testing.T) {
		engine := newTestEngine(t)
		outputs := dmnOutputColumnWithValues("grade", "等级", "string", "A,B") +
			dmnOutputColumnWithValues("rank", "序号", "number", "1,2")
		dmn := dmnTableDocument("decisionMultiOrder", "tableMultiOrder", "OUTPUT ORDER",
			dmnInputColumn("amountInput", "amount"), outputs,
			dmnRuleXML([]string{"-"}, []string{`"B"`, "1"})+
				dmnRuleXML([]string{"-"}, []string{`"A"`, "2"})+
				dmnRuleXML([]string{"-"}, []string{`"A"`, "1"}))
		deployBusinessRuleBundle(t, engine, "multiOrderProcess", `flowable:decisionRef="decisionMultiOrder" flowable:resultVariable="decision"`, dmn)
		instance, err := startBusinessRule(t, engine, "multiOrderProcess", api.Variable{Name: "amount", Value: 1})
		if err != nil {
			t.Fatalf("启动流程失败: %v", err)
		}
		expected := []any{
			map[string]any{"grade": "A", "rank": 1},
			map[string]any{"grade": "A", "rank": 2},
			map[string]any{"grade": "B", "rank": 1},
		}
		if !reflect.DeepEqual(instance.Variables["decision"], expected) {
			t.Fatalf("多列顺序值排序错误: %#v", instance.Variables["decision"])
		}
	})

	t.Run("顺序值之外的值排在最前", func(t *testing.T) {
		engine := newTestEngine(t)
		outputs := dmnOutputColumnWithValues("level", "等级", "string", "b,c")
		dmn := dmnTableDocument("decisionUnknownValue", "tableUnknownValue", "OUTPUT ORDER",
			dmnInputColumn("amountInput", "amount"), outputs,
			dmnRuleXML([]string{"-"}, []string{`"c"`})+
				dmnRuleXML([]string{"-"}, []string{`"b"`})+
				dmnRuleXML([]string{"-"}, []string{`"a"`}))
		deployBusinessRuleBundle(t, engine, "unknownValueProcess", `flowable:decisionRef="decisionUnknownValue" flowable:resultVariable="level"`, dmn)
		instance, err := startBusinessRule(t, engine, "unknownValueProcess", api.Variable{Name: "amount", Value: 1})
		if err != nil {
			t.Fatalf("启动流程失败: %v", err)
		}
		// 与 Flowable 的 indexOf 语义一致：不在列表中的值下标为 -1，排在最前
		if !reflect.DeepEqual(instance.Variables["level"], []any{"a", "b", "c"}) {
			t.Fatalf("顺序值之外的值应排在最前: %#v", instance.Variables["level"])
		}
	})

	t.Run("缺少顺序值报错", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocument("decisionNoOrder", "tableNoOrder", "OUTPUT ORDER",
			dmnInputColumn("amountInput", "amount"), dmnOutputColumn("level", "等级"),
			dmnRuleXML([]string{"-"}, []string{`"A"`}))
		deployBusinessRuleBundle(t, engine, "noOrderProcess", `flowable:decisionRef="decisionNoOrder" flowable:resultVariable="level"`, dmn)
		_, err := startBusinessRule(t, engine, "noOrderProcess", api.Variable{Name: "amount", Value: 1})
		requireBusinessRuleError(t, err, "ENGINE-1922")
	})

	t.Run("PRIORITY 缺少顺序值报错", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocument("decisionPriorityNoOrder", "tablePriorityNoOrder", "PRIORITY",
			dmnInputColumn("amountInput", "amount"), dmnOutputColumn("level", "等级"),
			dmnRuleXML([]string{"-"}, []string{`"A"`}))
		deployBusinessRuleBundle(t, engine, "priorityNoOrderProcess", `flowable:decisionRef="decisionPriorityNoOrder" flowable:resultVariable="level"`, dmn)
		_, err := startBusinessRule(t, engine, "priorityNoOrderProcess", api.Variable{Name: "amount", Value: 1})
		requireBusinessRuleError(t, err, "ENGINE-1922")
	})
	t.Run("输出值不可排序报错", func(t *testing.T) {
		engine := newTestEngine(t)
		outputs := dmnOutputColumnWithValues("payload", "载荷", "string", "a,b")
		dmn := dmnTableDocument("decisionBadOrderValue", "tableBadOrderValue", "OUTPUT ORDER",
			dmnInputColumn("amountInput", "amount"), outputs,
			dmnRuleXML([]string{"-"}, []string{"${payload}"}))
		deployBusinessRuleBundle(t, engine, "badOrderValueProcess", `flowable:decisionRef="decisionBadOrderValue" flowable:resultVariable="payload"`, dmn)
		_, err := startBusinessRule(t, engine, "badOrderValueProcess",
			api.Variable{Name: "payload", Value: map[string]any{"x": 1}})
		requireBusinessRuleError(t, err, "ENGINE-1922")
	})
}

// TestBusinessRuleTask_MultiResultOutputs 验证多条结果下结果变量与按列写入。
// @param t 测试上下文
func TestBusinessRuleTask_MultiResultOutputs(t *testing.T) {
	outputs := dmnOutputColumn("level", "等级") + dmnOutputColumn("note", "备注")
	rules := dmnRuleXML([]string{"-"}, []string{`"A"`, `"ok"`}) +
		dmnRuleXML([]string{"-"}, []string{`"B"`, "-"}) +
		dmnRuleXML([]string{"-"}, []string{`"C"`, `"x"`})

	t.Run("多行多输出结果变量", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocument("decisionMultiResult", "tableMultiResult", "RULE ORDER",
			dmnInputColumn("amountInput", "amount"), outputs, rules)
		deployBusinessRuleBundle(t, engine, "multiResultProcess", `flowable:decisionRef="decisionMultiResult" flowable:resultVariable="decision"`, dmn)
		instance, err := startBusinessRule(t, engine, "multiResultProcess", api.Variable{Name: "amount", Value: 1})
		if err != nil {
			t.Fatalf("启动流程失败: %v", err)
		}
		expected := []any{
			map[string]any{"level": "A", "note": "ok"},
			map[string]any{"level": "B"},
			map[string]any{"level": "C", "note": "x"},
		}
		if !reflect.DeepEqual(instance.Variables["decision"], expected) {
			t.Fatalf("多结果变量错误: %#v", instance.Variables["decision"])
		}
	})

	t.Run("多行无结果变量按列写入", func(t *testing.T) {
		engine := newTestEngine(t)
		dmn := dmnTableDocument("decisionMultiColumn", "tableMultiColumn", "RULE ORDER",
			dmnInputColumn("amountInput", "amount"), outputs, rules)
		deployBusinessRuleBundle(t, engine, "multiColumnProcess", `flowable:decisionRef="decisionMultiColumn"`, dmn)
		instance, err := startBusinessRule(t, engine, "multiColumnProcess", api.Variable{Name: "amount", Value: 1})
		if err != nil {
			t.Fatalf("启动流程失败: %v", err)
		}
		if !reflect.DeepEqual(instance.Variables["level"], []any{"A", "B", "C"}) {
			t.Fatalf("多结果输出列 level 错误: %#v", instance.Variables["level"])
		}
		// 第二条规则的 "-" 不产生值，列内跳过
		if !reflect.DeepEqual(instance.Variables["note"], []any{"ok", "x"}) {
			t.Fatalf("多结果输出列 note 错误: %#v", instance.Variables["note"])
		}
	})

	t.Run("多行整列无值时跳过", func(t *testing.T) {
		engine := newTestEngine(t)
		outputs := dmnOutputColumn("level", "等级") + dmnOutputColumn("unused", "未用")
		dmn := dmnTableDocument("decisionSkipColumn", "tableSkipColumn", "RULE ORDER",
			dmnInputColumn("amountInput", "amount"), outputs,
			dmnRuleXML([]string{"-"}, []string{`"A"`, "-"})+
				dmnRuleXML([]string{"-"}, []string{`"B"`, "-"}))
		deployBusinessRuleBundle(t, engine, "skipColumnProcess", `flowable:decisionRef="decisionSkipColumn"`, dmn)
		instance, err := startBusinessRule(t, engine, "skipColumnProcess",
			api.Variable{Name: "amount", Value: 1}, api.Variable{Name: "unused", Value: "keep"})
		if err != nil {
			t.Fatalf("启动流程失败: %v", err)
		}
		if !reflect.DeepEqual(instance.Variables["level"], []any{"A", "B"}) {
			t.Fatalf("多结果输出列 level 错误: %#v", instance.Variables["level"])
		}
		if instance.Variables["unused"] != "keep" {
			t.Fatalf("整列无值不应覆盖已有变量: %#v", instance.Variables["unused"])
		}
	})

	t.Run("多行结果变量缺少名称", func(t *testing.T) {
		engine := newTestEngine(t)
		unnamed := dmnOutputColumn("named", "有名称") + "<output/>"
		dmn := dmnTableDocument("decisionResultNoName", "tableResultNoName", "RULE ORDER",
			dmnInputColumn("amountInput", "amount"), unnamed,
			dmnRuleXML([]string{"-"}, []string{`"A"`, `"x"`})+
				dmnRuleXML([]string{"-"}, []string{`"B"`, `"y"`}))
		deployBusinessRuleBundle(t, engine, "resultNoNameProcess", `flowable:decisionRef="decisionResultNoName" flowable:resultVariable="decision"`, dmn)
		_, err := startBusinessRule(t, engine, "resultNoNameProcess", api.Variable{Name: "amount", Value: 1})
		requireBusinessRuleError(t, err, "ENGINE-1920")
	})
	t.Run("多行输出列缺少名称", func(t *testing.T) {
		engine := newTestEngine(t)
		unnamed := dmnOutputColumn("named", "有名称") + "<output/>"
		dmn := dmnTableDocument("decisionMultiNoName", "tableMultiNoName", "RULE ORDER",
			dmnInputColumn("amountInput", "amount"), unnamed,
			dmnRuleXML([]string{"-"}, []string{`"A"`, `"x"`})+
				dmnRuleXML([]string{"-"}, []string{`"B"`, `"y"`}))
		deployBusinessRuleBundle(t, engine, "multiNoNameProcess", `flowable:decisionRef="decisionMultiNoName"`, dmn)
		_, err := startBusinessRule(t, engine, "multiNoNameProcess", api.Variable{Name: "amount", Value: 1})
		requireBusinessRuleError(t, err, "ENGINE-1920")
	})
}
