package engine

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
	bpmnxml "github.com/flowable-go/flowable/internal/bpmn/xml"
	"github.com/flowable-go/flowable/internal/expr"
)

// DMN 求值相关的稳定错误码，统一映射为 400。
const (
	dmnErrMissingRef      = "ENGINE-1910" // 业务规则任务缺少 decisionRef
	dmnErrDecisionMissing = "ENGINE-1911" // 部署内找不到决策
	dmnErrTableInvalid    = "ENGINE-1912" // 决策表缺少输出或表结构非法
	dmnErrInputExpression = "ENGINE-1913" // 输入表达式求值失败
	dmnErrInputEntry      = "ENGINE-1914" // 输入条件解析失败
	dmnErrNoMatch         = "ENGINE-1915" // 没有匹配的规则
	dmnErrUniqueConflict  = "ENGINE-1916" // UNIQUE 策略命中多条规则
	dmnErrAnyConflict     = "ENGINE-1917" // ANY 策略命中规则输出不一致
	dmnErrHitPolicy       = "ENGINE-1918" // 不支持的命中策略
	dmnErrOutputEntry     = "ENGINE-1919" // 输出项解析或求值失败
	dmnErrOutputName      = "ENGINE-1920" // 输出列缺少 id 与 name
)

// dmnDecisionModel 是求值使用的决策模型，保留规则原文。
type dmnDecisionModel struct {
	ID    string         // 决策 ID，缺失时回退决策表 ID
	Key   string         // 决策 Key，与 ID 一致
	Name  string         // 决策名称
	Table *dmnTableModel // 决策表
}

// dmnTableModel 是求值使用的决策表模型。
type dmnTableModel struct {
	ID        string           // 决策表 ID
	HitPolicy string           // 原始命中策略
	Inputs    []dmnInputModel  // 输入列
	Outputs   []dmnOutputModel // 输出列
	Rules     []dmnRuleModel   // 规则，保持文档顺序
}

// dmnInputModel 是决策表输入列。
type dmnInputModel struct {
	ID         string // 输入 ID
	Label      string // 输入标签
	Expression string // 输入表达式原文
	Type       string // 输入类型
}

// dmnOutputModel 是决策表输出列。
type dmnOutputModel struct {
	ID   string // 输出 ID
	Name string // 输出名称
	Type string // 输出类型
}

// dmnRuleModel 是一条规则及其输入、输出单元格原文。
type dmnRuleModel struct {
	Inputs  []string // 输入条件原文
	Outputs []string // 输出项原文
}

// dmnOutputValue 是单个输出列的求值结果。
type dmnOutputValue struct {
	ID    string // 输出 ID
	Name  string // 输出名称
	Value any    // 输出值
	Set   bool   // 是否产生值，"-" 表示跳过
}

// executeBusinessRuleTask 求值业务规则任务引用的 DMN 决策表并写入流程变量。
// @param cc 命令上下文
// @param execution 当前执行
// @param task 业务规则任务
// @return 执行错误
func (e *Engine) executeBusinessRuleTask(cc *commandContext, execution *executionState, task *model.BusinessRuleTask) error {
	decisionRef := strings.TrimSpace(task.DecisionRef)
	if decisionRef == "" {
		return dmnBadRequest(dmnErrMissingRef, "业务规则任务缺少决策引用 decisionRef")
	}
	definition, ok := e.findDefinition(execution.DefinitionID)
	if !ok {
		return api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
	}
	decision, err := e.findDMNDecision(definition, decisionRef)
	if err != nil {
		return err
	}
	if decision.Table == nil || len(decision.Table.Outputs) == 0 {
		return dmnBadRequest(dmnErrTableInvalid, "决策表没有输出列: "+decisionRef)
	}
	values, err := e.evaluateDMNTable(cc.ctx, decision.Table, e.variablesOf(execution))
	if err != nil {
		return err
	}
	if task.ResultVariable != "" {
		value, valueErr := dmnResultVariableValue(values)
		if valueErr != nil {
			return valueErr
		}
		e.setVariable(execution, task.ResultVariable, value)
		return nil
	}
	// 未配置结果变量时，按输出列的 id/name 分别写入流程变量
	for _, value := range values {
		if !value.Set {
			continue
		}
		name := value.ID
		if name == "" {
			name = value.Name
		}
		if name == "" {
			return dmnBadRequest(dmnErrOutputName, "决策表输出列缺少 id 与 name，无法写入流程变量")
		}
		e.setVariable(execution, name, value.Value)
	}
	return nil
}

// findDMNDecision 在流程定义所在部署中按 decision id 或 decision table id 查找决策。
// @param definition 流程定义
// @param decisionRef 决策引用
// @return 决策模型
// @return 查找错误
func (e *Engine) findDMNDecision(definition *definitionState, decisionRef string) (*dmnDecisionModel, error) {
	deployment, ok := e.state.Deployments[definition.Info.DeploymentID]
	if !ok {
		return nil, dmnBadRequest(dmnErrDecisionMissing, "流程定义所在部署不存在: "+definition.Info.DeploymentID)
	}
	names := make([]string, 0)
	for name := range deployment.Resources {
		if isDMNResource(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	// 按资源名排序遍历，保证同名决策的查找结果稳定
	for _, name := range names {
		decisions, err := parseDMNDecisions(name, deployment.Resources[name])
		if err != nil {
			return nil, err
		}
		for _, decision := range decisions {
			if decision.ID == decisionRef || decision.Table.ID == decisionRef {
				return decision, nil
			}
		}
	}
	return nil, dmnBadRequest(dmnErrDecisionMissing, "部署内未找到 DMN 决策: "+decisionRef)
}

// parseDMNDecisions 将单个 .dmn 资源解析为求值决策模型列表。
// 与 dmn.go 的元数据解析保持一致，只复用其 Node 树与决策遍历能力。
// @param resourceName 资源名
// @param data 资源内容
// @return 决策模型列表
// @return 解析错误
func parseDMNDecisions(resourceName string, data []byte) ([]*dmnDecisionModel, error) {
	root, err := bpmnxml.Decode(bytes.NewReader(data), bpmnxml.MaxBPMNSize)
	if err != nil {
		return nil, api.NewError("ENGINE-1207", "DMN XML 解析失败: "+resourceName+": "+err.Error(), api.ErrBadRequest)
	}
	if root.Name.Local != "definitions" {
		return nil, api.NewError("ENGINE-1207", "DMN 根元素必须是 definitions: "+resourceName, api.ErrBadRequest)
	}
	decisionNodes := make([]*bpmnxml.Node, 0)
	collectDecisionNodes(root, &decisionNodes)
	decisions := make([]*dmnDecisionModel, 0, len(decisionNodes))
	for _, decisionNode := range decisionNodes {
		tableNode, ok := decisionNode.Child("decisionTable")
		if !ok {
			// 没有决策表的 decision 不参与求值
			continue
		}
		decisionID := decisionNode.Attr("id")
		tableID := tableNode.Attr("id")
		if tableID == "" {
			tableID = decisionID
		}
		if decisionID == "" {
			decisionID = tableID
		}
		decisions = append(decisions, &dmnDecisionModel{
			ID:    decisionID,
			Key:   decisionID,
			Name:  decisionNode.Attr("name"),
			Table: buildDMNTableModel(tableNode),
		})
	}
	return decisions, nil
}

// buildDMNTableModel 将 decisionTable 节点转换为求值模型。
// @param tableNode 决策表节点
// @return 决策表求值模型
func buildDMNTableModel(tableNode *bpmnxml.Node) *dmnTableModel {
	table := &dmnTableModel{
		ID:        tableNode.Attr("id"),
		HitPolicy: tableNode.Attr("hitPolicy"),
		Inputs:    make([]dmnInputModel, 0),
		Outputs:   make([]dmnOutputModel, 0),
		Rules:     make([]dmnRuleModel, 0),
	}
	for _, inputNode := range tableNode.ChildrenNamed("input") {
		input := dmnInputModel{ID: inputNode.Attr("id"), Label: inputNode.Attr("label")}
		if expressionNode, ok := inputNode.Child("inputExpression"); ok {
			input.Expression = expressionNode.TextContent()
			if input.Expression == "" {
				// 兼容把表达式放在 text 属性中的文档
				input.Expression = expressionNode.Attr("text")
			}
			input.Type = expressionNode.Attr("typeRef")
		}
		table.Inputs = append(table.Inputs, input)
	}
	for _, outputNode := range tableNode.ChildrenNamed("output") {
		outputType := outputNode.Attr("type")
		if outputType == "" {
			outputType = outputNode.Attr("typeRef")
		}
		table.Outputs = append(table.Outputs, dmnOutputModel{
			ID:   outputNode.Attr("id"),
			Name: outputNode.Attr("name"),
			Type: outputType,
		})
	}
	for _, ruleNode := range tableNode.ChildrenNamed("rule") {
		rule := dmnRuleModel{
			Inputs:  make([]string, 0),
			Outputs: make([]string, 0),
		}
		for _, entry := range ruleNode.ChildrenNamed("inputEntry") {
			rule.Inputs = append(rule.Inputs, entry.TextContent())
		}
		for _, entry := range ruleNode.ChildrenNamed("outputEntry") {
			rule.Outputs = append(rule.Outputs, entry.TextContent())
		}
		table.Rules = append(table.Rules, rule)
	}
	return table
}

// evaluateDMNTable 按命中策略求值决策表。
// @param ctx 上下文，用于取消表达式求值
// @param table 决策表
// @param vars 流程变量快照
// @return 输出列结果，顺序与输出列一致
// @return 求值错误
func (e *Engine) evaluateDMNTable(ctx context.Context, table *dmnTableModel, vars map[string]any) ([]dmnOutputValue, error) {
	policy := normalizeDMNHitPolicy(table.HitPolicy)
	switch policy {
	case "FIRST", "UNIQUE", "ANY", "RULE ORDER":
		// 支持的命中策略
	default:
		return nil, dmnBadRequest(dmnErrHitPolicy, "不支持的决策表命中策略: "+table.HitPolicy)
	}
	inputs := make([]any, len(table.Inputs))
	for index, input := range table.Inputs {
		value, err := evalDMNInputExpression(ctx, input.Expression, vars)
		if err != nil {
			return nil, dmnBadRequest(dmnErrInputExpression, fmt.Sprintf("决策表输入表达式 %q 求值失败: %v", input.Expression, err))
		}
		inputs[index] = value
	}
	matches := make([][]dmnOutputValue, 0, 1)
	for ruleIndex, rule := range table.Rules {
		if len(rule.Inputs) > len(table.Inputs) {
			return nil, dmnBadRequest(dmnErrInputEntry, fmt.Sprintf("决策表第 %d 条规则的输入条件多于输入列", ruleIndex+1))
		}
		matched, err := matchDMNInputs(table, rule, inputs, ruleIndex)
		if err != nil {
			return nil, err
		}
		if !matched {
			continue
		}
		values, err := evaluateDMNOutputs(ctx, rule, table.Outputs, vars)
		if err != nil {
			return nil, err
		}
		switch policy {
		case "FIRST", "RULE ORDER":
			// RULE ORDER 简化为顺序首个匹配，与 FIRST 一致
			return values, nil
		default:
			matches = append(matches, values)
			if policy == "UNIQUE" && len(matches) > 1 {
				return nil, dmnBadRequest(dmnErrUniqueConflict, "决策表命中策略为 UNIQUE，但有多条规则同时匹配")
			}
		}
	}
	if policy == "UNIQUE" || policy == "ANY" {
		if len(matches) == 0 {
			return nil, dmnBadRequest(dmnErrNoMatch, "决策表没有匹配的规则")
		}
		if policy == "ANY" {
			for _, other := range matches[1:] {
				if !dmnOutputsEqual(matches[0], other) {
					return nil, dmnBadRequest(dmnErrAnyConflict, "决策表命中策略为 ANY，但多条规则的输出不一致")
				}
			}
		}
		return matches[0], nil
	}
	return nil, dmnBadRequest(dmnErrNoMatch, "决策表没有匹配的规则")
}

// matchDMNInputs 判断单条规则的全部输入条件是否匹配。
// @param table 决策表
// @param rule 规则
// @param inputs 输入值
// @param ruleIndex 规则下标
// @return 是否匹配
// @return 匹配错误
func matchDMNInputs(table *dmnTableModel, rule dmnRuleModel, inputs []any, ruleIndex int) (bool, error) {
	for index := range table.Inputs {
		entry := "-"
		if index < len(rule.Inputs) {
			entry = rule.Inputs[index]
		}
		matched, err := matchDMNInputEntry(entry, inputs[index])
		if err != nil {
			return false, dmnBadRequest(dmnErrInputEntry, fmt.Sprintf("决策表第 %d 条规则的第 %d 个输入条件非法: %v", ruleIndex+1, index+1, err))
		}
		if !matched {
			return false, nil
		}
	}
	return true, nil
}

// evaluateDMNOutputs 求值一条规则的输出单元格。
// @param ctx 上下文
// @param rule 规则
// @param outputs 输出列
// @param vars 流程变量快照
// @return 输出列结果
// @return 求值错误
func evaluateDMNOutputs(ctx context.Context, rule dmnRuleModel, outputs []dmnOutputModel, vars map[string]any) ([]dmnOutputValue, error) {
	if len(rule.Outputs) > len(outputs) {
		return nil, dmnBadRequest(dmnErrOutputEntry, "决策表规则的输出项多于输出列")
	}
	values := make([]dmnOutputValue, len(outputs))
	for index, output := range outputs {
		values[index] = dmnOutputValue{ID: output.ID, Name: output.Name}
		if index >= len(rule.Outputs) {
			// 缺失的输出项按 "-" 跳过
			continue
		}
		value, set, err := evalDMNOutputEntry(ctx, rule.Outputs[index], vars)
		if err != nil {
			return nil, dmnBadRequest(dmnErrOutputEntry, fmt.Sprintf("决策表输出列 %q 解析失败: %v", dmnOutputName(output), err))
		}
		values[index].Value = value
		values[index].Set = set
	}
	return values, nil
}

// dmnResultVariableValue 把输出结果转换为单个结果变量的值。
// 单输出返回标量，多输出返回按 id/name 索引的映射。
// @param values 输出列结果
// @return 结果变量值
// @return 转换错误
func dmnResultVariableValue(values []dmnOutputValue) (any, error) {
	if len(values) == 1 {
		return values[0].Value, nil
	}
	result := make(map[string]any, len(values))
	for _, value := range values {
		if !value.Set {
			continue
		}
		name := value.ID
		if name == "" {
			name = value.Name
		}
		if name == "" {
			return nil, dmnBadRequest(dmnErrOutputName, "多输出决策表的输出列缺少 id 与 name")
		}
		result[name] = value.Value
	}
	return result, nil
}

// evalDMNInputExpression 编译并求值输入表达式。
// @param ctx 上下文
// @param source 表达式原文
// @param vars 流程变量快照
// @return 输入值
// @return 求值错误
func evalDMNInputExpression(ctx context.Context, source string, vars map[string]any) (any, error) {
	text := strings.TrimSpace(source)
	if text == "" {
		return nil, fmt.Errorf("输入表达式为空")
	}
	compiled, err := expr.CompileExpression(text)
	if err != nil {
		return nil, err
	}
	return compiled.Eval(ctx, &expr.Env{Vars: vars})
}

// evalDMNOutputEntry 求值单个输出单元格。
// @param ctx 上下文
// @param entry 输出项原文
// @param vars 流程变量快照
// @return 输出值
// @return 是否产生值，false 表示 "-" 跳过
// @return 求值错误
func evalDMNOutputEntry(ctx context.Context, entry string, vars map[string]any) (any, bool, error) {
	text := strings.TrimSpace(entry)
	if text == "" || text == "-" {
		return nil, false, nil
	}
	if text[0] == '"' || text[0] == '\'' {
		value, err := unquoteDMNString(text)
		if err != nil {
			return nil, false, err
		}
		return value, true, nil
	}
	switch strings.ToLower(text) {
	case "true":
		return true, true, nil
	case "false":
		return false, true, nil
	}
	if strings.HasPrefix(text, "${") || strings.HasPrefix(text, "#{") {
		compiled, err := expr.CompileExpression(text)
		if err != nil {
			return nil, false, err
		}
		value, err := compiled.Eval(ctx, &expr.Env{Vars: vars})
		if err != nil {
			return nil, false, err
		}
		return value, true, nil
	}
	if number, ok := parseDMNNumberValue(text); ok {
		return number, true, nil
	}
	// 未加引号的文本按字符串字面量处理，兼容简化写法
	return text, true, nil
}

// matchDMNInputEntry 判断输入值是否匹配输入单元格。
// 支持 "-"、逗号分隔多值、字符串、布尔、数字比较与区间。
// @param entry 输入条件原文
// @param value 输入值
// @return 是否匹配
// @return 解析错误
func matchDMNInputEntry(entry string, value any) (bool, error) {
	text := strings.TrimSpace(entry)
	if text == "" || text == "-" {
		return true, nil
	}
	alternatives, err := splitDMNAlternatives(text)
	if err != nil {
		return false, err
	}
	// 任一分支匹配即视为命中
	for _, alternative := range alternatives {
		matched, matchErr := matchDMNAlternative(strings.TrimSpace(alternative), value)
		if matchErr != nil {
			return false, matchErr
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

// splitDMNAlternatives 按顶层逗号拆分多值条件，忽略引号内的逗号。
// @param text 条件原文
// @return 分支列表
// @return 拆分错误
func splitDMNAlternatives(text string) ([]string, error) {
	parts := make([]string, 0, 2)
	start := 0
	var quote byte
	for index := 0; index < len(text); index++ {
		current := text[index]
		if quote != 0 {
			if current == '\\' {
				// 跳过转义字符，避免把转义引号当成分隔符
				index++
				continue
			}
			if current == quote {
				quote = 0
			}
			continue
		}
		switch current {
		case '\'', '"':
			quote = current
		case ',':
			parts = append(parts, text[start:index])
			start = index + 1
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("字符串缺少闭合引号: %s", text)
	}
	parts = append(parts, text[start:])
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return nil, fmt.Errorf("条件包含空分支: %s", text)
		}
	}
	return parts, nil
}

// matchDMNAlternative 判断单个条件分支是否匹配。
// @param text 条件分支
// @param value 输入值
// @return 是否匹配
// @return 解析错误
func matchDMNAlternative(text string, value any) (bool, error) {
	if text == "-" {
		return true, nil
	}
	if low, lowInclusive, high, highInclusive, isRange, err := parseDMNRange(text); isRange {
		if err != nil {
			return false, err
		}
		return dmnMatchRange(value, low, lowInclusive, high, highInclusive), nil
	}
	if operator := dmnComparisonOperator(text); operator != "" {
		operandText := strings.TrimSpace(text[len(operator):])
		if operandText == "" {
			return false, fmt.Errorf("比较条件缺少操作数: %s", text)
		}
		literal, err := parseDMNOperand(operandText)
		if err != nil {
			return false, err
		}
		if operator == "=" || operator == "==" || operator == "!=" {
			equal := dmnInputEqual(value, literal)
			if operator == "!=" {
				return !equal, nil
			}
			return equal, nil
		}
		return dmnInputCompare(value, operator, literal)
	}
	literal, err := parseDMNOperand(text)
	if err != nil {
		return false, err
	}
	return dmnInputEqual(value, literal), nil
}

// dmnComparisonOperator 返回条件开头的比较运算符，不存在时返回空串。
// @param text 条件分支
// @return 比较运算符
func dmnComparisonOperator(text string) string {
	for _, operator := range []string{"!=", "<=", ">=", "==", "=", "<", ">"} {
		if strings.HasPrefix(text, operator) {
			return operator
		}
	}
	return ""
}

// parseDMNRange 解析区间条件，如 [1..10]、(1..10]、[1..10) 与 ]1..10[。
// @param text 条件分支
// @return 下界
// @return 下界是否包含
// @return 上界
// @return 上界是否包含
// @return 是否为区间
// @return 解析错误
func parseDMNRange(text string) (any, bool, any, bool, bool, error) {
	if len(text) < 5 {
		return nil, false, nil, false, false, nil
	}
	first, last := text[0], text[len(text)-1]
	if !(first == '[' || first == '(' || first == ']') || !(last == ']' || last == ')' || last == '[') {
		return nil, false, nil, false, false, nil
	}
	body := text[1 : len(text)-1]
	index := strings.Index(body, "..")
	if index < 0 {
		return nil, false, nil, false, false, nil
	}
	lowText := strings.TrimSpace(body[:index])
	highText := strings.TrimSpace(body[index+2:])
	lowInclusive := first == '['
	highInclusive := last == ']'
	var low any
	var high any
	var err error
	if lowText != "" {
		low, err = parseDMNOperand(lowText)
		if err != nil {
			return nil, false, nil, false, true, fmt.Errorf("区间下界非法: %v", err)
		}
		if _, isBool := low.(bool); isBool {
			return nil, false, nil, false, true, fmt.Errorf("区间下界不支持布尔值")
		}
	}
	if highText != "" {
		high, err = parseDMNOperand(highText)
		if err != nil {
			return nil, false, nil, false, true, fmt.Errorf("区间上界非法: %v", err)
		}
		if _, isBool := high.(bool); isBool {
			return nil, false, nil, false, true, fmt.Errorf("区间上界不支持布尔值")
		}
	}
	if low == nil && high == nil {
		return nil, false, nil, false, true, fmt.Errorf("区间缺少端点")
	}
	return low, lowInclusive, high, highInclusive, true, nil
}

// dmnMatchRange 判断输入值是否落在区间内。
// @param value 输入值
// @param low 下界，nil 表示无下界
// @param lowInclusive 下界是否包含
// @param high 上界，nil 表示无上界
// @param highInclusive 上界是否包含
// @return 是否落在区间内
func dmnMatchRange(value, low any, lowInclusive bool, high any, highInclusive bool) bool {
	if value == nil {
		return false
	}
	if low != nil {
		comparison, ok := dmnCompareValues(value, low)
		if !ok || comparison < 0 || (comparison == 0 && !lowInclusive) {
			return false
		}
	}
	if high != nil {
		comparison, ok := dmnCompareValues(value, high)
		if !ok || comparison > 0 || (comparison == 0 && !highInclusive) {
			return false
		}
	}
	return true
}

// parseDMNOperand 解析条件中的字符串、布尔或数字字面量。
// @param text 字面量原文
// @return 字面量值
// @return 解析错误
func parseDMNOperand(text string) (any, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, fmt.Errorf("字面量为空")
	}
	if trimmed[0] == '"' || trimmed[0] == '\'' {
		return unquoteDMNString(trimmed)
	}
	switch strings.ToLower(trimmed) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	if number, ok := parseDMNNumberValue(trimmed); ok {
		return number, nil
	}
	return nil, fmt.Errorf("不支持的字面量 %q", trimmed)
}

// unquoteDMNString 去除字符串字面量的引号并还原转义引号。
// @param text 带引号的字符串
// @return 字符串值
// @return 解析错误
func unquoteDMNString(text string) (string, error) {
	if len(text) < 2 {
		return "", fmt.Errorf("字符串缺少闭合引号: %s", text)
	}
	quote := text[0]
	if text[len(text)-1] != quote {
		return "", fmt.Errorf("字符串缺少闭合引号: %s", text)
	}
	inner := text[1 : len(text)-1]
	var builder strings.Builder
	builder.Grow(len(inner))
	for index := 0; index < len(inner); index++ {
		current := inner[index]
		if current == '\\' && index+1 < len(inner) {
			next := inner[index+1]
			// 只还原转义引号与转义反斜杠，其余反斜杠保留原样
			if next == quote || next == '\\' {
				builder.WriteByte(next)
				index++
				continue
			}
		}
		if current == quote {
			return "", fmt.Errorf("字符串包含未转义引号: %s", text)
		}
		builder.WriteByte(current)
	}
	return builder.String(), nil
}

// parseDMNNumberValue 解析数字字面量，整数优先返回 int，其余返回 float64。
// @param text 数字原文
// @return 数字值
// @return 是否为合法数字
func parseDMNNumberValue(text string) (any, bool) {
	if text == "" {
		return nil, false
	}
	number, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return nil, false
	}
	if !strings.ContainsAny(text, ".eE") {
		if integer, intErr := strconv.ParseInt(text, 10, 64); intErr == nil {
			// 超出平台 int 范围时保留 float64，避免截断
			if int64(int(integer)) == integer {
				return int(integer), true
			}
		}
	}
	return number, true
}

// dmnNumberToFloat 将任意数值类型转换为 float64。
// @param value 值
// @return 浮点值
// @return 是否为数值
func dmnNumberToFloat(value any) (float64, bool) {
	if value == nil {
		return 0, false
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(reflected.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(reflected.Uint()), true
	case reflect.Float32, reflect.Float64:
		return reflected.Float(), true
	default:
		return 0, false
	}
}

// dmnCompareValues 比较两个同类型值，类型不匹配时返回 false。
// @param left 左值
// @param right 右值
// @return 比较结果：负数表示小于，0 表示相等，正数表示大于
// @return 是否可比较
func dmnCompareValues(left, right any) (int, bool) {
	leftNumber, leftIsNumber := dmnNumberToFloat(left)
	rightNumber, rightIsNumber := dmnNumberToFloat(right)
	if leftIsNumber && rightIsNumber {
		switch {
		case leftNumber < rightNumber:
			return -1, true
		case leftNumber > rightNumber:
			return 1, true
		default:
			return 0, true
		}
	}
	leftString, leftIsString := left.(string)
	rightString, rightIsString := right.(string)
	if leftIsString && rightIsString {
		return strings.Compare(leftString, rightString), true
	}
	return 0, false
}

// dmnInputEqual 判断输入值与字面量是否严格类型相等。
// @param value 输入值
// @param literal 字面量
// @return 是否相等
func dmnInputEqual(value, literal any) bool {
	if value == nil {
		return false
	}
	switch typed := literal.(type) {
	case bool:
		actual, ok := value.(bool)
		return ok && actual == typed
	case string:
		actual, ok := value.(string)
		return ok && actual == typed
	default:
		expected, expectedOK := dmnNumberToFloat(literal)
		actual, actualOK := dmnNumberToFloat(value)
		return expectedOK && actualOK && expected == actual
	}
}

// dmnInputCompare 对输入值执行 <、<=、>、>= 比较。
// @param value 输入值
// @param operator 比较运算符
// @param literal 字面量
// @return 是否满足比较
// @return 解析错误
func dmnInputCompare(value any, operator string, literal any) (bool, error) {
	if _, isBool := literal.(bool); isBool {
		return false, fmt.Errorf("布尔字面量不支持比较运算")
	}
	if value == nil {
		return false, nil
	}
	comparison, ok := dmnCompareValues(value, literal)
	if !ok {
		return false, nil
	}
	switch operator {
	case "<":
		return comparison < 0, nil
	case "<=":
		return comparison <= 0, nil
	case ">":
		return comparison > 0, nil
	case ">=":
		return comparison >= 0, nil
	default:
		return false, fmt.Errorf("未知比较运算符: %s", operator)
	}
}

// dmnOutputsEqual 判断两组输出列结果是否完全一致，用于 ANY 策略。
// @param left 左结果
// @param right 右结果
// @return 是否一致
func dmnOutputsEqual(left, right []dmnOutputValue) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Set != right[index].Set {
			return false
		}
		if !left[index].Set {
			continue
		}
		if !dmnValuesEqual(left[index].Value, right[index].Value) {
			return false
		}
	}
	return true
}

// dmnValuesEqual 按数值、布尔、字符串与深度相等比较输出值。
// @param left 左值
// @param right 右值
// @return 是否相等
func dmnValuesEqual(left, right any) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	leftNumber, leftIsNumber := dmnNumberToFloat(left)
	rightNumber, rightIsNumber := dmnNumberToFloat(right)
	if leftIsNumber || rightIsNumber {
		return leftIsNumber && rightIsNumber && leftNumber == rightNumber
	}
	return reflect.DeepEqual(left, right)
}

// normalizeDMNHitPolicy 规范化命中策略，空值按 FIRST 处理。
// @param raw 原始策略
// @return 规范化后的策略
func normalizeDMNHitPolicy(raw string) string {
	normalized := strings.ToUpper(strings.TrimSpace(raw))
	normalized = strings.ReplaceAll(normalized, "_", " ")
	normalized = strings.Join(strings.Fields(normalized), " ")
	if normalized == "" {
		return "FIRST"
	}
	return normalized
}

// dmnOutputName 返回输出列的展示名，优先 id。
// @param output 输出列
// @return 展示名
func dmnOutputName(output dmnOutputModel) string {
	if output.ID != "" {
		return output.ID
	}
	return output.Name
}

// dmnBadRequest 构造映射为 400 的引擎错误。
// @param code 错误码
// @param message 中文消息
// @return 引擎错误
func dmnBadRequest(code, message string) *api.Error {
	return api.NewError(code, message, api.ErrBadRequest)
}
