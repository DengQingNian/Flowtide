package api

import "context"

// DecisionDefinition 是部署内 .dmn 资源解析出的决策定义元数据。
type DecisionDefinition struct {
	ID           string `json:"id"`                // 决策定义 ID，取 decision 的 id
	Key          string `json:"key"`               // 决策 Key，取 decision 的 id
	Name         string `json:"name"`              // 决策名称，取 decision 的 name
	DeploymentID string `json:"deploymentId"`      // 所在部署 ID
	ResourceName string `json:"resourceName"`      // 部署资源名
	TenantID     string `json:"tenantId"`          // 租户，沿用流程定义
	Version      int    `json:"version,omitempty"` // 版本，沿用所查询流程定义的版本
}

// DecisionTableInput 是决策表输入列元数据。
type DecisionTableInput struct {
	ID         string `json:"id"`                   // 输入 ID
	Label      string `json:"label,omitempty"`      // 输入显示名
	Expression string `json:"expression,omitempty"` // 输入表达式文本
}

// DecisionTableOutput 是决策表输出列元数据。
type DecisionTableOutput struct {
	ID   string `json:"id"`             // 输出 ID
	Name string `json:"name,omitempty"` // 输出名称
	Type string `json:"type,omitempty"` // 输出类型，兼容 type 与 typeRef 属性
}

// DecisionTableDefinition 是部署内决策表元数据，不包含表达式求值结果。
type DecisionTableDefinition struct {
	ID           string                `json:"id"`           // 决策表 ID，取 decisionTable 的 id，缺失时回退 decision 的 id
	Key          string                `json:"key"`          // 决策 Key，取 decision 的 id
	Name         string                `json:"name"`         // 决策名称，取 decision 的 name
	DeploymentID string                `json:"deploymentId"` // 所在部署 ID
	ResourceName string                `json:"resourceName"` // 部署资源名
	TenantID     string                `json:"tenantId"`     // 租户，沿用流程定义
	HitPolicy    string                `json:"hitPolicy"`    // 命中策略，取 decisionTable 的 hitPolicy
	Inputs       []DecisionTableInput  `json:"inputs"`       // 输入列，按文档顺序
	Outputs      []DecisionTableOutput `json:"outputs"`      // 输出列，按文档顺序
	RuleCount    int                   `json:"ruleCount"`    // 规则数量
}

// DMNService 提供部署内 DMN 决策定义与决策表元数据查询。
// 该接口只读取元数据，不执行决策表达式求值。
type DMNService interface {
	// ListDecisionDefinitions 返回流程定义所在部署的决策定义。
	// @param ctx 上下文
	// @param processDefinitionID 流程定义 ID
	// @return 决策定义列表，按资源名与决策 ID 排序
	// @return 查询错误
	ListDecisionDefinitions(ctx context.Context, processDefinitionID string) ([]DecisionDefinition, error)
	// ListDecisionTables 返回流程定义所在部署的决策表定义。
	// @param ctx 上下文
	// @param processDefinitionID 流程定义 ID
	// @return 决策表定义列表，按资源名与决策 ID 排序
	// @return 查询错误
	ListDecisionTables(ctx context.Context, processDefinitionID string) ([]DecisionTableDefinition, error)
}

// DMNEngine 是可选 DMN 能力引擎接口。
// 独立定义该接口可避免在 Engine 门面中强制增加 DMN 依赖，调用方可用类型断言探测能力。
type DMNEngine interface {
	Engine // 聚合全部基础服务
	// DMNService 返回 DMN 服务。
	// @return DMN 服务
	DMNService() DMNService
}
