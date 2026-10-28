package api

import "context"

// FormValue 是表单枚举值。
type FormValue struct {
	ID   string `json:"id"`   // 值 ID
	Name string `json:"name"` // 显示名
}

// FormProperty 是表单字段。
type FormProperty struct {
	ID         string      `json:"id"`                   // 字段 ID
	Name       string      `json:"name,omitempty"`       // 显示名
	Type       string      `json:"type,omitempty"`       // 类型
	Required   bool        `json:"required"`             // 是否必填
	Readable   bool        `json:"readable"`             // 是否可读
	Writable   bool        `json:"writable"`             // 是否可写
	Value      any         `json:"value,omitempty"`      // 当前值
	EnumValues []FormValue `json:"enumValues,omitempty"` // 枚举值
}

// FormData 是表单数据。
type FormData struct {
	FormKey    string         `json:"formKey,omitempty"` // 表单 Key
	Properties []FormProperty `json:"formProperties"`    // 字段列表
}

// FormDefinition 是部署内 .form 资源解析出的表单定义。
type FormDefinition struct {
	Key          string         `json:"key"`          // 表单 Key
	Name         string         `json:"name"`         // 表单名称
	DeploymentID string         `json:"deploymentId"` // 所在部署 ID
	ResourceName string         `json:"resourceName"` // 部署资源名
	TenantID     string         `json:"tenantId"`     // 租户
	Fields       []FormProperty `json:"fields"`       // 字段列表
}

// FormService 提供启动表单与任务表单。
type FormService interface {
	// GetStartFormData 返回流程启动表单。
	// @param ctx 上下文
	// @param processDefinitionID 流程定义 ID
	// @return 表单数据
	// @return 查询错误
	GetStartFormData(ctx context.Context, processDefinitionID string) (FormData, error)
	// GetTaskFormData 返回任务表单。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @return 表单数据
	// @return 查询错误
	GetTaskFormData(ctx context.Context, taskID string) (FormData, error)
	// ListFormDefinitions 返回流程定义所在部署的表单定义。
	// @param ctx 上下文
	// @param processDefinitionID 流程定义 ID
	// @return 表单定义列表，按资源名排序
	// @return 查询错误
	ListFormDefinitions(ctx context.Context, processDefinitionID string) ([]FormDefinition, error)
	// SubmitTaskFormData 提交任务表单并完成任务。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @param values 字段值
	// @return 提交错误
	SubmitTaskFormData(ctx context.Context, taskID string, values map[string]any) error
}
