package engine

import (
	"context"
	"encoding/json"
	"path"
	"sort"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// formResourceExtension 是部署内表单资源的扩展名。
const formResourceExtension = ".form"

// formDefinitionDocument 是 .form 资源的 JSON 文档结构。
type formDefinitionDocument struct {
	Key    string              `json:"key"`    // 表单 Key，可为空
	Name   string              `json:"name"`   // 表单名称
	Fields *[]api.FormProperty `json:"fields"` // 字段列表，必须存在
}

// ListFormDefinitions 返回流程定义所在部署的表单定义。
// @param ctx 上下文
// @param processDefinitionID 流程定义 ID
// @return 表单定义列表，按资源名排序
// @return 查询错误
func (e *Engine) ListFormDefinitions(ctx context.Context, processDefinitionID string) ([]api.FormDefinition, error) {
	result, err := e.read(func() (any, error) {
		definition, ok := e.findDefinition(processDefinitionID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
		}
		// 租户校验沿用流程定义所属租户
		if err := e.allowTenant(ctx, definition.Info.TenantID); err != nil {
			return nil, err
		}
		return e.formDefinitionsInDeployment(definition.Info.DeploymentID, definition.Info.TenantID)
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.FormDefinition), nil
}

// formDefinitionsInDeployment 解析部署内全部 .form 资源。
// @param deploymentID 部署 ID
// @param tenantID 流程定义租户
// @return 表单定义列表，按资源名排序
// @return 解析错误
func (e *Engine) formDefinitionsInDeployment(deploymentID, tenantID string) ([]api.FormDefinition, error) {
	deployment, ok := e.state.Deployments[deploymentID]
	if !ok {
		// 定义指向不存在的部署时返回空列表，避免读取时 panic
		return []api.FormDefinition{}, nil
	}
	names := make([]string, 0)
	for name := range deployment.Resources {
		if isFormResource(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	forms := make([]api.FormDefinition, 0, len(names))
	for _, name := range names {
		form, err := parseFormDefinition(name, deployment.Resources[name], deploymentID, tenantID)
		if err != nil {
			return nil, err
		}
		forms = append(forms, form)
	}
	return forms, nil
}

// findFormDefinition 按 Key 查找部署内表单定义。
// @param deploymentID 部署 ID
// @param tenantID 流程定义租户
// @param formKey 表单 Key
// @return 表单定义
// @return 是否找到
// @return 解析错误
func (e *Engine) findFormDefinition(deploymentID, tenantID, formKey string) (api.FormDefinition, bool, error) {
	forms, err := e.formDefinitionsInDeployment(deploymentID, tenantID)
	if err != nil {
		return api.FormDefinition{}, false, err
	}
	for _, form := range forms {
		if form.Key == formKey {
			return form, true, nil
		}
	}
	return api.FormDefinition{}, false, nil
}

// isFormResource 判断资源是否为 .form 表单定义。
// @param name 资源名
// @return 是否为表单资源
func isFormResource(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), formResourceExtension)
}

// formResourceKey 从资源名推导表单 Key，即去掉目录与扩展名。
// @param resourceName 资源名
// @return 表单 Key
func formResourceKey(resourceName string) string {
	base := path.Base(resourceName)
	// 仅在扩展名完整时截断，避免误伤无扩展名资源
	if len(base) > len(formResourceExtension) && strings.EqualFold(path.Ext(base), formResourceExtension) {
		return base[:len(base)-len(formResourceExtension)]
	}
	return base
}

// parseFormDefinition 将 .form 资源解析为表单定义。
// @param resourceName 资源名
// @param data 资源内容
// @param deploymentID 部署 ID
// @param tenantID 流程定义租户
// @return 表单定义
// @return 解析错误
func parseFormDefinition(resourceName string, data []byte, deploymentID, tenantID string) (api.FormDefinition, error) {
	document := formDefinitionDocument{}
	if err := json.Unmarshal(data, &document); err != nil {
		// 保留 JSON 语法细节，同时用 ErrBadRequest 让 REST 返回 400
		return api.FormDefinition{}, api.NewError("ENGINE-1205", "表单定义 JSON 解析失败: "+resourceName+": "+err.Error(), api.ErrBadRequest)
	}
	// fields 是必填项，缺失时不能静默退化为空表单
	if document.Fields == nil {
		return api.FormDefinition{}, api.NewError("ENGINE-1206", "表单定义缺少 fields: "+resourceName, api.ErrBadRequest)
	}
	key := document.Key
	if strings.TrimSpace(key) == "" {
		key = formResourceKey(resourceName)
	}
	fields := make([]api.FormProperty, 0, len(*document.Fields))
	fields = append(fields, (*document.Fields)...)
	return api.FormDefinition{
		Key:          key,
		Name:         document.Name,
		DeploymentID: deploymentID,
		ResourceName: resourceName,
		TenantID:     tenantID,
		Fields:       fields,
	}, nil
}

// resolveFormProperties 优先使用同部署 .form 字段，未找到时回退 BPMN 字段。
// @param deploymentID 部署 ID
// @param tenantID 流程定义租户
// @param formKey 表单 Key
// @param bpmnProperties BPMN 表单字段
// @param values 当前变量值
// @return 表单字段
// @return 解析错误
func (e *Engine) resolveFormProperties(deploymentID, tenantID, formKey string, bpmnProperties []*model.FormProperty, values map[string]any) ([]api.FormProperty, error) {
	if formKey != "" {
		form, found, err := e.findFormDefinition(deploymentID, tenantID, formKey)
		if err != nil {
			return nil, err
		}
		if found {
			return formDefinitionProperties(form.Fields, values), nil
		}
	}
	return toFormProperties(bpmnProperties, values), nil
}

// formDefinitionProperties 将 .form 字段转换为 API 字段并填充当前值。
// @param fields 表单定义字段
// @param values 当前变量值
// @return API 字段
func formDefinitionProperties(fields []api.FormProperty, values map[string]any) []api.FormProperty {
	out := make([]api.FormProperty, 0, len(fields))
	for _, field := range fields {
		item := field
		item.Value = nil
		item.EnumValues = append([]api.FormValue(nil), field.EnumValues...)
		if values != nil {
			if value, ok := values[field.ID]; ok {
				item.Value = value
			}
		}
		out = append(out, item)
	}
	return out
}
