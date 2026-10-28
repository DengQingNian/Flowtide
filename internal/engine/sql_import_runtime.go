package engine

import (
	"context"
	"encoding/json"
	"time"

	"github.com/flowable-go/flowable/api"
	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
)

// ImportRuntimeRows 从运行时行恢复执行、任务与变量。
// @param ctx 上下文
// @param rows 行列表
// @return 导入错误
func (e *Engine) ImportRuntimeRows(ctx context.Context, rows []flowablerow.Row) error {
	snapshot, err := flowablerow.DecodeRuntimeRows(rows)
	if err != nil {
		return err
	}
	_, err = e.execute(ctx, func(cc *commandContext) (any, error) {
		for _, execution := range snapshot.Executions {
			definition, ok := e.findDefinition(execution.DefinitionID)
			if !ok {
				return nil, api.NewError("ENGINE-0404", "导入执行缺少流程定义: "+execution.DefinitionID, api.ErrBadRequest)
			}
			instance, exists := e.state.Instances[execution.ProcessInstanceID]
			if !exists {
				instance = &instanceState{
					ID:                   execution.ProcessInstanceID,
					ProcessDefinitionID:  execution.DefinitionID,
					ProcessDefinitionKey: definition.Info.Key,
					BusinessKey:          execution.BusinessKey,
					TenantID:             execution.TenantID,
					StartTime:            execution.StartTime,
					Suspended:            execution.Suspended,
					RootExecutionID:      execution.ID,
				}
				e.state.Instances[instance.ID] = instance
			}
			if execution.ParentID == "" {
				instance.RootExecutionID = execution.ID
			}
			e.state.Executions[execution.ID] = &executionState{
				ID:                  execution.ID,
				ProcessInstanceID:   execution.ProcessInstanceID,
				DefinitionID:        execution.DefinitionID,
				ParentID:            execution.ParentID,
				ActivityID:          execution.ActivityID,
				Active:              execution.Active,
				Scope:               execution.Scope,
				Concurrent:          execution.Concurrent,
				ScopeRoot:           execution.Scope && execution.ParentID != "",
				EventSubProcessRoot: execution.EventScope,
				MultiInstanceRoot:   execution.MIRoot,
				Variables:           make(map[string]any),
			}
		}
		for _, task := range snapshot.Tasks {
			// 独立任务没有执行，允许 EXECUTION_ID_ 为空
			if task.ExecutionID != "" {
				if _, ok := e.state.Executions[task.ExecutionID]; !ok {
					return nil, api.NewError("ENGINE-0405", "导入任务缺少执行: "+task.ExecutionID, api.ErrBadRequest)
				}
			}
			e.state.Tasks[task.ID] = &taskState{
				Task: api.Task{
					ID:                  task.ID,
					Name:                task.Name,
					Description:         task.Description,
					ProcessInstanceID:   task.ProcessInstanceID,
					ProcessDefinitionID: task.DefinitionID,
					TaskDefinitionKey:   task.TaskDefinitionKey,
					ParentTaskID:        task.ParentTaskID,
					Assignee:            task.Assignee,
					Owner:               task.Owner,
					Priority:            task.Priority,
					FormKey:             task.FormKey,
					Category:            task.Category,
					TenantID:            task.TenantID,
					CreateTime:          task.CreateTime,
					DueDate:             task.DueDate,
				},
				ExecutionID:    task.ExecutionID,
				ActivityID:     task.TaskDefinitionKey,
				LocalVariables: make(map[string]any),
			}
		}
		for _, variable := range snapshot.Variables {
			value, valueErr := runtimeVariableValue(variable)
			if valueErr != nil {
				return nil, valueErr
			}
			if variable.TaskID != "" {
				task, ok := e.state.Tasks[variable.TaskID]
				if !ok {
					return nil, api.NewError("ENGINE-0805", "导入变量缺少任务: "+variable.TaskID, api.ErrBadRequest)
				}
				task.LocalVariables[variable.Name] = value
				continue
			}
			execution, ok := e.state.Executions[variable.ExecutionID]
			if !ok {
				return nil, api.NewError("ENGINE-0405", "导入变量缺少执行: "+variable.ExecutionID, api.ErrBadRequest)
			}
			execution.Variables[variable.Name] = value
		}
		for _, task := range e.state.Tasks {
			if execution, ok := e.state.Executions[task.ExecutionID]; ok {
				execution.Waiting = true
			}
		}
		for _, job := range snapshot.Jobs {
			if job.DefinitionID != "" {
				if _, ok := e.findDefinition(job.DefinitionID); !ok {
					return nil, api.NewError("ENGINE-0404", "导入作业缺少流程定义: "+job.DefinitionID, api.ErrBadRequest)
				}
			}
			if job.ProcessInstanceID != "" {
				if _, ok := e.findInstance(job.ProcessInstanceID); !ok {
					return nil, api.NewError("ENGINE-0404", "导入作业缺少实例: "+job.ProcessInstanceID, api.ErrBadRequest)
				}
			}
			due := time.Time{}
			if job.DueDate != nil {
				due = *job.DueDate
			}
			e.state.Jobs[job.ID] = &jobState{
				ID:                  job.ID,
				Type:                job.Type,
				ProcessInstanceID:   job.ProcessInstanceID,
				ProcessDefinitionID: job.DefinitionID,
				ExecutionID:         job.ExecutionID,
				ActivityID:          job.ActivityID,
				Retries:             job.Retries,
				Exception:           job.Exception,
				DueDate:             due,
				CreatedAt:           job.CreatedAt,
				RepeatRemaining:     job.RepeatRemaining,
				RepeatInfinite:      job.RepeatInfinite,
				IntervalMillis:      job.IntervalMillis,
			}
		}
		for _, job := range snapshot.DeadLetters {
			due := time.Time{}
			if job.DueDate != nil {
				due = *job.DueDate
			}
			e.state.DeadLetters[job.ID] = &jobState{
				ID:                  job.ID,
				Type:                job.Type,
				ProcessInstanceID:   job.ProcessInstanceID,
				ProcessDefinitionID: job.DefinitionID,
				ExecutionID:         job.ExecutionID,
				ActivityID:          job.ActivityID,
				Retries:             0,
				Exception:           job.Exception,
				DueDate:             due,
				CreatedAt:           job.CreatedAt,
				RepeatRemaining:     job.RepeatRemaining,
				RepeatInfinite:      job.RepeatInfinite,
				IntervalMillis:      job.IntervalMillis,
			}
		}
		for _, subscription := range snapshot.Subscriptions {
			if _, ok := e.findDefinition(subscription.DefinitionID); !ok {
				return nil, api.NewError("ENGINE-0404", "导入订阅缺少流程定义: "+subscription.DefinitionID, api.ErrBadRequest)
			}
			if subscription.ExecutionID != "" {
				if _, ok := e.state.Executions[subscription.ExecutionID]; !ok {
					return nil, api.NewError("ENGINE-0405", "导入订阅缺少执行: "+subscription.ExecutionID, api.ErrBadRequest)
				}
			}
			state := &subscriptionState{
				ID:                  subscription.ID,
				ProcessDefinitionID: subscription.DefinitionID,
				ProcessInstanceID:   subscription.ProcessInstanceID,
				ExecutionID:         subscription.ExecutionID,
				EventType:           subscription.EventType,
				EventName:           subscription.EventName,
				ActivityID:          subscription.ActivityID,
				ScopeExecutionID:    subscription.ScopeID,
				TenantID:            subscription.TenantID,
				CreatedAt:           subscription.CreatedAt,
			}
			switch subscription.ScopeType {
			case "start":
				state.StartEvent = true
			case "boundary":
				state.BoundaryEventID = subscription.ScopeDefinitionID
			case "event-subprocess":
				state.EventSubProcessStartID = subscription.ScopeDefinitionID
				state.EventSubProcessID = subscription.ScopeDefinitionKey
			case "gateway":
				state.GatewayGroup = subscription.SubScopeID
			}
			e.state.Subscriptions[state.ID] = state
		}
		for _, link := range snapshot.Links {
			// 任务身份链接按任务动态计算，不单独恢复
			if link.ProcessInstanceID == "" || link.TaskID != "" {
				continue
			}
			instance, ok := e.findInstance(link.ProcessInstanceID)
			if !ok {
				return nil, api.NewError("ENGINE-0404", "导入身份链接缺少实例: "+link.ProcessInstanceID, api.ErrBadRequest)
			}
			identity := &api.IdentityLink{
				ID:                link.ID,
				Type:              link.Type,
				UserID:            link.UserID,
				GroupID:           link.GroupID,
				TaskID:            link.TaskID,
				ProcessInstanceID: link.ProcessInstanceID,
				CreateTime:        instance.StartTime,
			}
			e.state.InstanceLinks[identity.ID] = identity
		}
		return nil, nil
	})
	return err
}

// runtimeVariableValue 将变量行转换为 Go 值。
// @param variable 变量行
// @return 变量值
// @return 转换错误
func runtimeVariableValue(variable flowablerow.RuntimeVariable) (any, error) {
	switch variable.Type {
	case "string":
		if variable.Text == nil {
			return "", nil
		}
		return *variable.Text, nil
	case "boolean":
		if variable.Long == nil {
			return false, nil
		}
		return *variable.Long != 0, nil
	case "long":
		if variable.Long == nil {
			return int64(0), nil
		}
		return *variable.Long, nil
	case "double":
		if variable.Double == nil {
			return 0.0, nil
		}
		return *variable.Double, nil
	case "null":
		return nil, nil
	case "json":
		if variable.Text == nil {
			return nil, nil
		}
		var value any
		if err := json.Unmarshal([]byte(*variable.Text), &value); err != nil {
			return nil, api.NewError("ENGINE-0913", "导入 JSON 变量失败", err)
		}
		return value, nil
	default:
		if variable.Text != nil {
			return *variable.Text, nil
		}
		if variable.Long != nil {
			return *variable.Long, nil
		}
		if variable.Double != nil {
			return *variable.Double, nil
		}
		return nil, nil
	}
}
