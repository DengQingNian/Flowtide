package engine

import (
	"bytes"
	"context"
	"sort"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
	bpmnxml "github.com/DengQingNian/Flowtide/internal/bpmn/xml"
)

// dmnResourceExtension 是部署内 DMN 资源的扩展名。
const dmnResourceExtension = ".dmn"

// dmnDecisionEntry 是一条决策定义与其决策表的配对结果。
type dmnDecisionEntry struct {
	Definition api.DecisionDefinition      // 决策定义元数据
	Table      api.DecisionTableDefinition // 决策表元数据
}

// DMNService 返回 DMN 服务。
// @return DMN 服务
func (e *Engine) DMNService() api.DMNService { return e }

// ListDecisionDefinitions 返回流程定义所在部署的决策定义。
// @param ctx 上下文
// @param processDefinitionID 流程定义 ID
// @return 决策定义列表，按资源名与决策 ID 排序
// @return 查询错误
func (e *Engine) ListDecisionDefinitions(ctx context.Context, processDefinitionID string) ([]api.DecisionDefinition, error) {
	entries, err := e.decisionEntries(ctx, processDefinitionID)
	if err != nil {
		return nil, err
	}
	decisions := make([]api.DecisionDefinition, 0, len(entries))
	for _, entry := range entries {
		decisions = append(decisions, entry.Definition)
	}
	return decisions, nil
}

// ListDecisionTables 返回流程定义所在部署的决策表定义。
// @param ctx 上下文
// @param processDefinitionID 流程定义 ID
// @return 决策表定义列表，按资源名与决策 ID 排序
// @return 查询错误
func (e *Engine) ListDecisionTables(ctx context.Context, processDefinitionID string) ([]api.DecisionTableDefinition, error) {
	entries, err := e.decisionEntries(ctx, processDefinitionID)
	if err != nil {
		return nil, err
	}
	tables := make([]api.DecisionTableDefinition, 0, len(entries))
	for _, entry := range entries {
		tables = append(tables, entry.Table)
	}
	return tables, nil
}

// decisionEntries 读取流程定义所在部署的全部 DMN 决策元数据。
// @param ctx 上下文
// @param processDefinitionID 流程定义 ID
// @return 决策与决策表配对列表，按资源名与决策 ID 排序
// @return 查询错误
func (e *Engine) decisionEntries(ctx context.Context, processDefinitionID string) ([]dmnDecisionEntry, error) {
	result, err := e.read(func() (any, error) {
		definition, ok := e.findDefinition(processDefinitionID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
		}
		// 租户校验沿用流程定义所属租户
		if err := e.allowTenant(ctx, definition.Info.TenantID); err != nil {
			return nil, err
		}
		deployment, ok := e.state.Deployments[definition.Info.DeploymentID]
		if !ok {
			// 定义指向不存在的部署时返回空列表，避免读取时 panic
			return []dmnDecisionEntry{}, nil
		}
		names := make([]string, 0)
		for name := range deployment.Resources {
			if isDMNResource(name) {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		entries := make([]dmnDecisionEntry, 0, len(names))
		for _, name := range names {
			parsed, parseErr := parseDMNResource(name, deployment.Resources[name], definition.Info.DeploymentID, definition.Info.TenantID, definition.Info.Version)
			if parseErr != nil {
				return nil, parseErr
			}
			entries = append(entries, parsed...)
		}
		// 同一资源内可能有多个 decision，再按决策 ID 稳定排序
		sort.SliceStable(entries, func(i, j int) bool {
			if entries[i].Definition.ResourceName != entries[j].Definition.ResourceName {
				return entries[i].Definition.ResourceName < entries[j].Definition.ResourceName
			}
			return entries[i].Definition.Key < entries[j].Definition.Key
		})
		return entries, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]dmnDecisionEntry), nil
}

// isDMNResource 判断资源是否为 .dmn 决策资源，扩展名大小写不敏感。
// @param name 资源名
// @return 是否为 DMN 资源
func isDMNResource(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), dmnResourceExtension)
}

// parseDMNResource 将单个 .dmn 资源解析为决策元数据列表。
// @param resourceName 资源名
// @param data 资源内容
// @param deploymentID 部署 ID
// @param tenantID 流程定义租户
// @param version 流程定义版本
// @return 决策与决策表配对列表
// @return 解析错误
func parseDMNResource(resourceName string, data []byte, deploymentID, tenantID string, version int) ([]dmnDecisionEntry, error) {
	root, err := bpmnxml.Decode(bytes.NewReader(data), bpmnxml.MaxBPMNSize)
	if err != nil {
		// Cause 固定为 ErrBadRequest，确保 REST 映射为 400；细节保留在消息中
		return nil, api.NewError("ENGINE-1207", "DMN XML 解析失败: "+resourceName+": "+err.Error(), api.ErrBadRequest)
	}
	if root.Name.Local != "definitions" {
		return nil, api.NewError("ENGINE-1207", "DMN 根元素必须是 definitions: "+resourceName, api.ErrBadRequest)
	}
	decisionNodes := make([]*bpmnxml.Node, 0)
	collectDecisionNodes(root, &decisionNodes)
	entries := make([]dmnDecisionEntry, 0, len(decisionNodes))
	for _, decisionNode := range decisionNodes {
		tableNode, ok := decisionNode.Child("decisionTable")
		if !ok {
			// 无决策表的 decision 不产生决策定义
			continue
		}
		entries = append(entries, buildDMNDecisionEntry(decisionNode, tableNode, resourceName, deploymentID, tenantID, version))
	}
	return entries, nil
}

// collectDecisionNodes 递归收集 definitions 下直接或嵌套的 decision 节点。
// @param node 当前节点
// @param out 收集结果
func collectDecisionNodes(node *bpmnxml.Node, out *[]*bpmnxml.Node) {
	for _, child := range node.Children {
		if child.Name.Local == "decision" {
			*out = append(*out, child)
			// decision 内部不再继续查找，避免异常文档产生重复定义
			continue
		}
		collectDecisionNodes(child, out)
	}
}

// buildDMNDecisionEntry 将 decision 与 decisionTable 节点转换为 API 元数据。
// @param decisionNode decision 节点
// @param tableNode decisionTable 节点
// @param resourceName 资源名
// @param deploymentID 部署 ID
// @param tenantID 流程定义租户
// @param version 流程定义版本
// @return 决策与决策表配对
func buildDMNDecisionEntry(decisionNode, tableNode *bpmnxml.Node, resourceName, deploymentID, tenantID string, version int) dmnDecisionEntry {
	decisionID := decisionNode.Attr("id")
	name := decisionNode.Attr("name")
	tableID := tableNode.Attr("id")
	if tableID == "" {
		// decisionTable 缺少 id 时回退 decision id，保证 ID 非空
		tableID = decisionID
	}
	defID := decisionID
	if defID == "" {
		defID = tableID
	}
	inputs := make([]api.DecisionTableInput, 0)
	for _, inputNode := range tableNode.ChildrenNamed("input") {
		input := api.DecisionTableInput{ID: inputNode.Attr("id"), Label: inputNode.Attr("label")}
		if expressionNode, ok := inputNode.Child("inputExpression"); ok {
			input.Expression = expressionNode.TextContent()
			if input.Expression == "" {
				// 兼容把表达式放在 text 属性中的文档
				input.Expression = expressionNode.Attr("text")
			}
		}
		inputs = append(inputs, input)
	}
	outputs := make([]api.DecisionTableOutput, 0)
	for _, outputNode := range tableNode.ChildrenNamed("output") {
		outputType := outputNode.Attr("type")
		if outputType == "" {
			outputType = outputNode.Attr("typeRef")
		}
		outputs = append(outputs, api.DecisionTableOutput{
			ID:   outputNode.Attr("id"),
			Name: outputNode.Attr("name"),
			Type: outputType,
		})
	}
	return dmnDecisionEntry{
		Definition: api.DecisionDefinition{
			ID:           defID,
			Key:          defID,
			Name:         name,
			DeploymentID: deploymentID,
			ResourceName: resourceName,
			TenantID:     tenantID,
			Version:      version,
		},
		Table: api.DecisionTableDefinition{
			ID:           tableID,
			Key:          defID,
			Name:         name,
			DeploymentID: deploymentID,
			ResourceName: resourceName,
			TenantID:     tenantID,
			HitPolicy:    tableNode.Attr("hitPolicy"),
			Inputs:       inputs,
			Outputs:      outputs,
			RuleCount:    len(tableNode.ChildrenNamed("rule")),
		},
	}
}
