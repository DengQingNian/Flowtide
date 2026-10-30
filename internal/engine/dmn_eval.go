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

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
	bpmnxml "github.com/DengQingNian/Flowtide/internal/bpmn/xml"
	"github.com/DengQingNian/Flowtide/internal/expr"
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
	dmnErrAggregation     = "ENGINE-1921" // 聚合器属性或聚合输出值非法
	dmnErrOutputOrder     = "ENGINE-1922" // OUTPUT ORDER / PRIORITY 无法按顺序值排序
)

// DMN 命中策略取值，与 Flowable HitPolicy 一致。
const (
	dmnPolicyFirst       = "FIRST"        // 首条匹配
	dmnPolicyUnique      = "UNIQUE"       // 唯一匹配
	dmnPolicyAny         = "ANY"          // 多条匹配但输出一致
	dmnPolicyRuleOrder   = "RULE ORDER"   // 文档顺序返回全部匹配
	dmnPolicyOutputOrder = "OUTPUT ORDER" // 按输出列顺序值返回全部匹配
	dmnPolicyPriority    = "PRIORITY"     // 按输出列顺序值取第一条
	dmnPolicyCollect     = "COLLECT"      // 收集全部匹配或按聚合器聚合
)

// DMN 内置聚合器取值，与 Flowable BuiltinAggregator 一致。
const (
	dmnAggregationSum   = "SUM"   // 求和
	dmnAggregationMin   = "MIN"   // 最小值
	dmnAggregationMax   = "MAX"   // 最大值
	dmnAggregationCount = "COUNT" // 计数
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
	ID          string           // 决策表 ID
	HitPolicy   string           // 原始命中策略
	Aggregation string           // 规范化后的聚合器，空表示不做聚合
	Inputs      []dmnInputModel  // 输入列
	Outputs     []dmnOutputModel // 输出列
	Rules       []dmnRuleModel   // 规则，保持文档顺序
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
	ID     string // 输出 ID
	Name   string // 输出名称
	Type   string // 输出类型
	Values []any  // outputValues 预定义顺序值，空表示未配置
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
	rows, err := e.evaluateDMNTable(cc.ctx, decision.Table, e.variablesOf(execution))
	if err != nil {
		return err
	}
	if task.ResultVariable != "" {
		value, valueErr := dmnResultVariableRowsValue(rows)
		if valueErr != nil {
			return valueErr
		}
		e.setVariable(execution, task.ResultVariable, value)
		return nil
	}
	return e.writeDMNOutputVariables(execution, rows)
}

// writeDMNOutputVariables 未配置结果变量时按输出列写入流程变量。
// 单条结果按输出列写标量；多条结果把每个输出列写成 []any 并跳过未产生值的行。
// @param execution 当前执行
// @param rows 匹配结果行
// @return 写入错误
func (e *Engine) writeDMNOutputVariables(execution *executionState, rows [][]dmnOutputValue) error {
	if len(rows) == 1 {
		// 单条结果沿用标量写入语义，输出为 "-" 时跳过
		for _, value := range rows[0] {
			if !value.Set {
				continue
			}
			name, nameErr := dmnOutputVariableName(value)
			if nameErr != nil {
				return nameErr
			}
			e.setVariable(execution, name, value.Value)
		}
		return nil
	}
	// 多条结果按输出列聚合为切片，列内未产生值的行直接跳过
	for index := range rows[0] {
		values := make([]any, 0, len(rows))
		for _, row := range rows {
			if !row[index].Set {
				continue
			}
			values = append(values, row[index].Value)
		}
		if len(values) == 0 {
			// 整列都没有产生值时保持已有变量不变
			continue
		}
		name, nameErr := dmnOutputVariableName(rows[0][index])
		if nameErr != nil {
			return nameErr
		}
		e.setVariable(execution, name, values)
	}
	return nil
}

// dmnOutputVariableName 返回输出列写入流程变量时使用的变量名。
// @param value 输出列结果
// @return 变量名，优先 id，其次 name
// @return 缺少 id 与 name 时的 400 错误
func dmnOutputVariableName(value dmnOutputValue) (string, error) {
	name := value.ID
	if name == "" {
		name = value.Name
	}
	if name == "" {
		return "", dmnBadRequest(dmnErrOutputName, "决策表输出列缺少 id 与 name，无法写入流程变量")
	}
	return name, nil
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
		table, tableErr := buildDMNTableModel(tableNode)
		if tableErr != nil {
			return nil, tableErr
		}
		decisions = append(decisions, &dmnDecisionModel{
			ID:    decisionID,
			Key:   decisionID,
			Name:  decisionNode.Attr("name"),
			Table: table,
		})
	}
	return decisions, nil
}

// buildDMNTableModel 将 decisionTable 节点转换为求值模型。
// @param tableNode 决策表节点
// @return 决策表求值模型
// @return 解析错误，聚合器或输出顺序值非法时返回 400
func buildDMNTableModel(tableNode *bpmnxml.Node) (*dmnTableModel, error) {
	rawAggregation := tableNode.Attr("aggregation")
	if rawAggregation == "" {
		// 兼容部分文档使用 aggregator 作为属性名
		rawAggregation = tableNode.Attr("aggregator")
	}
	aggregation, aggregationErr := normalizeDMNAggregation(rawAggregation)
	if aggregationErr != nil {
		return nil, aggregationErr
	}
	table := &dmnTableModel{
		ID:          tableNode.Attr("id"),
		HitPolicy:   tableNode.Attr("hitPolicy"),
		Aggregation: aggregation,
		Inputs:      make([]dmnInputModel, 0),
		Outputs:     make([]dmnOutputModel, 0),
		Rules:       make([]dmnRuleModel, 0),
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
		output := dmnOutputModel{
			ID:   outputNode.Attr("id"),
			Name: outputNode.Attr("name"),
			Type: outputType,
		}
		if valuesNode, ok := outputNode.Child("outputValues"); ok {
			values, valuesErr := parseDMNOutputValues(valuesNode.TextContent(), outputType)
			if valuesErr != nil {
				// 顺序值解析失败会让 OUTPUT ORDER / PRIORITY 结果不可判定，按 400 拒绝
				return nil, dmnBadRequest(dmnErrOutputOrder, fmt.Sprintf("决策表输出列 %q 的顺序值非法: %v", dmnOutputName(output), valuesErr))
			}
			output.Values = values
		}
		table.Outputs = append(table.Outputs, output)
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
	return table, nil
}

// normalizeDMNAggregation 规范化 decisionTable 的聚合器属性。
// 只接受 SUM、MIN、MAX、COUNT 四种取值，大小写不敏感，缺省返回空串。
// @param raw 原始聚合器属性值
// @return 规范化后的聚合器，空串表示不做聚合
// @return 非法聚合器的 400 错误
func normalizeDMNAggregation(raw string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(raw))
	if normalized == "" {
		return "", nil
	}
	switch normalized {
	case dmnAggregationSum, dmnAggregationMin, dmnAggregationMax, dmnAggregationCount:
		// 合法聚合器
		return normalized, nil
	default:
		return "", dmnBadRequest(dmnErrAggregation, "决策表聚合器非法: "+raw)
	}
}

// parseDMNOutputValues 解析输出列的 outputValues 顺序值列表。
// 多个值按顶层逗号分隔，引号内的逗号不拆分。
// @param text outputValues 文本
// @param outputType 输出列类型，用于类型转换
// @return 顺序值列表，文本为空时返回 nil
// @return 解析错误
func parseDMNOutputValues(text, outputType string) ([]any, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, nil
	}
	parts, err := splitDMNAlternatives(trimmed)
	if err != nil {
		return nil, err
	}
	values := make([]any, 0, len(parts))
	for _, part := range parts {
		value, valueErr := parseDMNOutputValue(strings.TrimSpace(part), outputType)
		if valueErr != nil {
			return nil, valueErr
		}
		values = append(values, value)
	}
	return values, nil
}

// parseDMNOutputValue 按输出列类型转换单个顺序值。
// 类型缺失时按布尔、数字、字符串的顺序推断。
// @param text 顺序值原文，可带引号
// @param outputType 输出列类型
// @return 顺序值
// @return 解析错误
func parseDMNOutputValue(text, outputType string) (any, error) {
	literal := text
	if len(literal) > 0 && (literal[0] == '"' || literal[0] == '\'') {
		unquoted, err := unquoteDMNString(literal)
		if err != nil {
			return nil, err
		}
		// 去掉引号后继续按输出类型转换
		literal = unquoted
	}
	switch strings.ToLower(strings.TrimSpace(outputType)) {
	case "number":
		number, ok := parseDMNNumberValue(literal)
		if !ok {
			return nil, fmt.Errorf("顺序值 %q 不是数字", text)
		}
		return number, nil
	case "boolean", "bool":
		switch strings.ToLower(strings.TrimSpace(literal)) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return nil, fmt.Errorf("顺序值 %q 不是布尔值", text)
		}
	case "string":
		return literal, nil
	default:
		// 类型缺失时按字面量推断，无法识别时按字符串处理
		switch strings.ToLower(strings.TrimSpace(literal)) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
		if number, ok := parseDMNNumberValue(literal); ok {
			return number, nil
		}
		return literal, nil
	}
}

// evaluateDMNTable 按命中策略求值决策表并返回匹配结果行。
// 每行按输出列顺序保存一条规则的结果；单结果策略只返回一行。
// @param ctx 上下文，用于取消表达式求值
// @param table 决策表
// @param vars 流程变量快照
// @return 匹配结果行，每行与输出列一一对应
// @return 求值错误
func (e *Engine) evaluateDMNTable(ctx context.Context, table *dmnTableModel, vars map[string]any) ([][]dmnOutputValue, error) {
	policy := normalizeDMNHitPolicy(table.HitPolicy)
	switch policy {
	case dmnPolicyFirst, dmnPolicyUnique, dmnPolicyAny, dmnPolicyRuleOrder,
		dmnPolicyOutputOrder, dmnPolicyPriority, dmnPolicyCollect:
		// 支持的命中策略
	default:
		return nil, dmnBadRequest(dmnErrHitPolicy, "不支持的决策表命中策略: "+table.HitPolicy)
	}
	// COLLECT 使用聚合器时的部署约束，与 Flowable sanityCheckDecisionTable 保持一致
	if policy == dmnPolicyCollect {
		if validateErr := validateDMNCollectionAggregation(table); validateErr != nil {
			return nil, validateErr
		}
	}
	inputs := make([]any, len(table.Inputs))
	for index, input := range table.Inputs {
		value, err := evalDMNInputExpression(ctx, input.Expression, vars)
		if err != nil {
			return nil, dmnBadRequest(dmnErrInputExpression, fmt.Sprintf("决策表输入表达式 %q 求值失败: %v", input.Expression, err))
		}
		inputs[index] = value
	}
	matches := make([][]dmnOutputValue, 0, len(table.Rules))
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
		// FIRST 命中首条后立即返回，与 Flowable 的 ContinueEvaluating 行为一致
		if policy == dmnPolicyFirst {
			return [][]dmnOutputValue{values}, nil
		}
		matches = append(matches, values)
		// UNIQUE 出现第二条匹配立即报冲突
		if policy == dmnPolicyUnique && len(matches) > 1 {
			return nil, dmnBadRequest(dmnErrUniqueConflict, "决策表命中策略为 UNIQUE，但有多条规则同时匹配")
		}
	}
	// 没有任何规则匹配时统一报错，避免静默写入空结果
	if len(matches) == 0 {
		return nil, dmnBadRequest(dmnErrNoMatch, "决策表没有匹配的规则")
	}
	switch policy {
	case dmnPolicyUnique:
		return matches[:1], nil
	case dmnPolicyAny:
		// ANY 允许多条匹配，但要求逐列输出一致
		for _, other := range matches[1:] {
			if !dmnOutputsEqual(matches[0], other) {
				return nil, dmnBadRequest(dmnErrAnyConflict, "决策表命中策略为 ANY，但多条规则的输出不一致")
			}
		}
		return matches[:1], nil
	case dmnPolicyOutputOrder:
		return sortDMNOutputOrder(table, matches)
	case dmnPolicyPriority:
		sorted, err := sortDMNOutputOrder(table, matches)
		if err != nil {
			return nil, err
		}
		// PRIORITY 只取排序后的第一条
		return sorted[:1], nil
	case dmnPolicyCollect:
		return collectDMNMatches(table, matches)
	default:
		// RULE ORDER 按文档顺序返回全部匹配结果
		return matches, nil
	}
}

// sortDMNOutputOrder 按输出列的 outputValues 顺序值稳定排序匹配结果。
// 输出值不在顺序列表中时按 Flowable indexOf 语义视为 -1，排在列表值之前。
// @param table 决策表
// @param rows 匹配结果行
// @return 排序后的结果行
// @return 排序错误
func sortDMNOutputOrder(table *dmnTableModel, rows [][]dmnOutputValue) ([][]dmnOutputValue, error) {
	hasOrder := false
	for _, output := range table.Outputs {
		if len(output.Values) > 0 {
			hasOrder = true
			break
		}
	}
	// 没有顺序值列表时无法判断优先级，按 Flowable 严格模式返回 400
	if !hasOrder {
		return nil, dmnBadRequest(dmnErrOutputOrder, "命中策略 "+normalizeDMNHitPolicy(table.HitPolicy)+" 需要在输出列配置 outputValues 顺序值")
	}
	// 排序前先校验输出值可比较，避免比较函数无法返回错误
	for _, row := range rows {
		for index, output := range table.Outputs {
			if len(output.Values) == 0 {
				continue
			}
			if !dmnOrderableValue(row[index].Value) {
				return nil, dmnBadRequest(dmnErrOutputOrder, fmt.Sprintf("输出列 %q 的值不是可排序的标量: %v", dmnOutputName(output), row[index].Value))
			}
		}
	}
	sorted := make([][]dmnOutputValue, len(rows))
	copy(sorted, rows)
	// 稳定排序，顺序值相同的规则保持文档顺序
	sort.SliceStable(sorted, func(left, right int) bool {
		return compareDMNOutputRows(table, sorted[left], sorted[right]) < 0
	})
	return sorted, nil
}

// compareDMNOutputRows 按输出列声明顺序比较两行结果的顺序值下标。
// @param table 决策表
// @param left 左行
// @param right 右行
// @return 负数表示左行在前，0 表示顺序相同
func compareDMNOutputRows(table *dmnTableModel, left, right []dmnOutputValue) int {
	for index, output := range table.Outputs {
		if len(output.Values) == 0 {
			continue
		}
		leftIndex := dmnOrderIndex(output.Values, left[index].Value)
		rightIndex := dmnOrderIndex(output.Values, right[index].Value)
		if leftIndex != rightIndex {
			return leftIndex - rightIndex
		}
	}
	return 0
}

// dmnOrderIndex 返回值在 outputValues 顺序列表中的下标。
// @param values 顺序值列表
// @param value 输出值
// @return 下标，未命中或空值时返回 -1
func dmnOrderIndex(values []any, value any) int {
	for index, candidate := range values {
		if dmnValuesEqual(candidate, value) {
			return index
		}
	}
	return -1
}

// dmnOrderableValue 判断输出值是否属于可参与顺序排序的标量。
// 支持数字、字符串、布尔与 nil；其他类型无法与顺序值比较。
// @param value 输出值
// @return 是否可排序
func dmnOrderableValue(value any) bool {
	if value == nil {
		return true
	}
	switch value.(type) {
	case string, bool:
		return true
	}
	_, isNumber := dmnNumberToFloat(value)
	return isNumber
}

// validateDMNCollectionAggregation 校验 COLLECT 聚合的部署约束。
// 与 Flowable 的 sanityCheckDecisionTable 一致：配置聚合器时输出列必须恰好一列且类型为 number。
// @param table 决策表
// @return 违反约束时的 400 错误
func validateDMNCollectionAggregation(table *dmnTableModel) error {
	if table.Aggregation == "" {
		return nil
	}
	// 多输出无法确定聚合目标，按 Flowable 严格模式拒绝
	if len(table.Outputs) != 1 {
		return dmnBadRequest(dmnErrAggregation, "命中策略 COLLECT 使用聚合器时只允许一个输出列")
	}
	outputType := strings.ToLower(strings.TrimSpace(table.Outputs[0].Type))
	// COUNT 与 SUM/MIN/MAX 一样要求输出列类型为 number
	if outputType != "number" {
		return dmnBadRequest(dmnErrAggregation, "命中策略 COLLECT 使用聚合器时输出列类型必须是 number")
	}
	return nil
}

// collectDMNMatches 按 COLLECT 策略汇总匹配结果。
// 未配置聚合器时返回全部匹配结果；配置聚合器时聚合唯一输出列并返回单条结果，
// 调用方需先通过 validateDMNCollectionAggregation 校验输出列数与类型。
// @param table 决策表
// @param rows 匹配结果行
// @return 结果行
// @return 聚合错误
func collectDMNMatches(table *dmnTableModel, rows [][]dmnOutputValue) ([][]dmnOutputValue, error) {
	if table.Aggregation == "" {
		return rows, nil
	}
	column := dmnFirstPopulatedOutput(table, rows)
	if column < 0 {
		// 匹配规则均未产生输出值时返回单条空结果，与 Flowable 的空决策结果一致
		return [][]dmnOutputValue{dmnEmptyOutputRow(table.Outputs)}, nil
	}
	value, err := aggregateDMNColumn(table.Aggregation, rows, column)
	if err != nil {
		return nil, err
	}
	result := dmnEmptyOutputRow(table.Outputs)
	result[column].Value = value
	result[column].Set = true
	return [][]dmnOutputValue{result}, nil
}

// dmnFirstPopulatedOutput 返回第一个在匹配行中产生值的输出列下标。
// @param table 决策表
// @param rows 匹配结果行
// @return 输出列下标，全部为空时返回 -1
func dmnFirstPopulatedOutput(table *dmnTableModel, rows [][]dmnOutputValue) int {
	for index := range table.Outputs {
		for _, row := range rows {
			if row[index].Set {
				return index
			}
		}
	}
	return -1
}

// dmnEmptyOutputRow 构造只保留输出列 id/name 的空结果行。
// @param outputs 输出列
// @return 空结果行
func dmnEmptyOutputRow(outputs []dmnOutputModel) []dmnOutputValue {
	row := make([]dmnOutputValue, len(outputs))
	for index, output := range outputs {
		row[index] = dmnOutputValue{ID: output.ID, Name: output.Name}
	}
	return row
}

// aggregateDMNColumn 对指定输出列执行 SUM、MIN、MAX 或 COUNT 聚合。
// 四种聚合都要求输出值是数字，与 Flowable 将输出统一转换为 Double 的行为一致。
// @param aggregation 规范化后的聚合器
// @param rows 匹配结果行
// @param column 输出列下标
// @return 聚合值，统一为 float64，与 Flowable 的 Double 结果一致
// @return 聚合错误
func aggregateDMNColumn(aggregation string, rows [][]dmnOutputValue, column int) (any, error) {
	values := make([]float64, 0, len(rows))
	for _, row := range rows {
		if !row[column].Set {
			continue
		}
		number, ok := dmnNumberToFloat(row[column].Value)
		if !ok {
			return nil, dmnBadRequest(dmnErrAggregation, fmt.Sprintf("聚合器 %s 要求输出值为数字，实际为 %v", aggregation, row[column].Value))
		}
		values = append(values, number)
	}
	switch aggregation {
	case dmnAggregationSum:
		sum := 0.0
		for _, value := range values {
			sum += value
		}
		return sum, nil
	case dmnAggregationMin:
		minimum := values[0]
		for _, value := range values[1:] {
			if value < minimum {
				minimum = value
			}
		}
		return minimum, nil
	case dmnAggregationMax:
		maximum := values[0]
		for _, value := range values[1:] {
			if value > maximum {
				maximum = value
			}
		}
		return maximum, nil
	default:
		// 剩余聚合器为 COUNT，返回有值的行数
		return float64(len(values)), nil
	}
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

// dmnResultVariableRowsValue 把匹配结果行转换为结果变量的值。
// 单行沿用单条结果规则（单输出标量、多输出映射）；多行返回逐行转换后的 []any。
// @param rows 匹配结果行
// @return 结果变量值
// @return 转换错误
func dmnResultVariableRowsValue(rows [][]dmnOutputValue) (any, error) {
	if len(rows) == 1 {
		return dmnResultVariableValue(rows[0])
	}
	values := make([]any, 0, len(rows))
	for _, row := range rows {
		value, err := dmnResultVariableValue(row)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
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
