package engine

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
)

// FormService 返回表单服务。
// @return 表单服务
func (e *Engine) FormService() api.FormService { return e }

// GetStartFormData 返回流程启动表单。
// @param ctx 上下文
// @param processDefinitionID 流程定义 ID
// @return 表单数据
// @return 查询错误
func (e *Engine) GetStartFormData(ctx context.Context, processDefinitionID string) (api.FormData, error) {
	result, err := e.read(func() (any, error) {
		definition, ok := e.findDefinition(processDefinitionID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
		}
		process := processOfDefinition(definition)
		if process == nil {
			return nil, api.NewError("ENGINE-0501", "流程定义不包含流程", nil)
		}
		start := firstStartEvent(process)
		if start == nil {
			return nil, api.NewError("ENGINE-0104", "流程缺少开始事件", nil)
		}
		return api.FormData{FormKey: start.FormKey, Properties: toFormProperties(start.FormProperties, nil)}, nil
	})
	if err != nil {
		return api.FormData{}, err
	}
	return result.(api.FormData), nil
}

// GetTaskFormData 返回任务表单。
// @param ctx 上下文
// @param taskID 任务 ID
// @return 表单数据
// @return 查询错误
func (e *Engine) GetTaskFormData(ctx context.Context, taskID string) (api.FormData, error) {
	result, err := e.read(func() (any, error) {
		task, ok := e.findTask(taskID)
		if !ok {
			return nil, api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
		}
		definition, ok := e.findDefinition(task.Task.ProcessDefinitionID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
		}
		process := processOfDefinition(definition)
		if process == nil {
			return nil, api.NewError("ENGINE-0501", "流程定义不包含流程", nil)
		}
		element, exists := findElement(process, task.ActivityID)
		if !exists {
			return nil, api.NewError("ENGINE-0501", "任务活动不存在", api.ErrNotFound)
		}
		userTask, isUserTask := element.(*model.UserTask)
		if !isUserTask {
			return nil, api.NewError("ENGINE-1201", "任务活动不是用户任务", nil)
		}
		values := make(map[string]any)
		if execution, exists := e.findExecution(task.ExecutionID); exists {
			for name, value := range e.variablesOf(execution) {
				values[name] = value
			}
		}
		for name, value := range task.LocalVariables {
			values[name] = value
		}
		return api.FormData{FormKey: userTask.FormKey, Properties: toFormProperties(userTask.FormProperties, values)}, nil
	})
	if err != nil {
		return api.FormData{}, err
	}
	return result.(api.FormData), nil
}

// SubmitTaskFormData 提交任务表单并完成任务。
// @param ctx 上下文
// @param taskID 任务 ID
// @param values 字段值
// @return 提交错误
func (e *Engine) SubmitTaskFormData(ctx context.Context, taskID string, values map[string]any) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		task, ok := e.findTask(taskID)
		if !ok {
			return nil, api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
		}
		definition, ok := e.findDefinition(task.Task.ProcessDefinitionID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
		}
		process := processOfDefinition(definition)
		element, exists := findElement(process, task.ActivityID)
		if !exists {
			return nil, api.NewError("ENGINE-0501", "任务活动不存在", api.ErrNotFound)
		}
		userTask, isUserTask := element.(*model.UserTask)
		if !isUserTask {
			return nil, api.NewError("ENGINE-1201", "任务活动不是用户任务", nil)
		}
		variables := make([]api.Variable, 0, len(values))
		for _, property := range userTask.FormProperties {
			raw, provided := values[property.ID]
			if !property.Writable {
				if provided {
					return nil, api.NewError("ENGINE-1202", "表单字段不可写: "+property.ID, api.ErrBadRequest)
				}
				continue
			}
			if !provided {
				if property.Required {
					return nil, api.NewError("ENGINE-1203", "表单字段必填: "+property.ID, api.ErrBadRequest)
				}
				continue
			}
			converted, convertErr := convertFormValue(raw, property.Type)
			if convertErr != nil {
				return nil, api.NewError("ENGINE-1204", "表单字段类型错误: "+property.ID, convertErr)
			}
			variables = append(variables, api.Variable{Name: property.ID, Value: converted})
		}
		return nil, e.completeTask(cc, taskID, api.CompleteTaskRequest{Variables: variables})
	})
	return err
}

// toFormProperties 将模型字段转换为 API 字段。
// @param properties 模型字段
// @param values 当前值
// @return API 字段
func toFormProperties(properties []*model.FormProperty, values map[string]any) []api.FormProperty {
	out := make([]api.FormProperty, 0, len(properties))
	for _, property := range properties {
		item := api.FormProperty{
			ID:       property.ID,
			Name:     property.Name,
			Type:     property.Type,
			Required: property.Required,
			Readable: property.Readable,
			Writable: property.Writable,
			Value:    property.Value,
		}
		if values != nil {
			if value, ok := values[property.ID]; ok {
				item.Value = value
			}
		}
		for _, enumValue := range property.EnumValues {
			item.EnumValues = append(item.EnumValues, api.FormValue{ID: enumValue.ID, Name: enumValue.Name})
		}
		out = append(out, item)
	}
	return out
}

// convertFormValue 按字段类型转换值。
// @param raw 原始值
// @param formType 字段类型
// @return 转换结果
// @return 转换错误
func convertFormValue(raw any, formType string) (any, error) {
	switch strings.ToLower(formType) {
	case "", "string":
		return raw, nil
	case "long", "integer":
		switch value := raw.(type) {
		case int:
			return value, nil
		case int64:
			return value, nil
		case float64:
			return int64(value), nil
		case string:
			return strconv.ParseInt(value, 10, 64)
		default:
			return nil, strconv.ErrSyntax
		}
	case "double":
		switch value := raw.(type) {
		case float64:
			return value, nil
		case int:
			return float64(value), nil
		case string:
			return strconv.ParseFloat(value, 64)
		default:
			return nil, strconv.ErrSyntax
		}
	case "boolean":
		switch value := raw.(type) {
		case bool:
			return value, nil
		case string:
			return strconv.ParseBool(value)
		default:
			return nil, strconv.ErrSyntax
		}
	case "date":
		switch value := raw.(type) {
		case time.Time:
			return value, nil
		case string:
			return time.Parse(time.RFC3339, value)
		default:
			return nil, strconv.ErrSyntax
		}
	default:
		return raw, nil
	}
}
