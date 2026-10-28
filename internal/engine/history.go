package engine

import (
	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
	"time"
)

// recordProcessStart 写入历史流程实例。
// @param cc 命令上下文
// @param instance 流程实例
func (e *Engine) recordProcessStart(cc *commandContext, instance *instanceState) {
	if !e.historyAtLeast(HistoryInstance) {
		return
	}
	historic := &api.HistoricProcessInstance{
		ID:                   instance.ID,
		ProcessDefinitionID:  instance.ProcessDefinitionID,
		ProcessDefinitionKey: instance.ProcessDefinitionKey,
		BusinessKey:          instance.BusinessKey,
		Name:                 instance.Name,
		TenantID:             instance.TenantID,
		StartTime:            instance.StartTime,
	}
	e.state.HistoricProcesses[historic.ID] = historic
}

// recordProcessEnd 更新历史流程实例结束信息。
// @param cc 命令上下文
// @param instance 流程实例
func (e *Engine) recordProcessEnd(cc *commandContext, instance *instanceState) {
	if !e.historyAtLeast(HistoryInstance) {
		return
	}
	historic, ok := e.state.HistoricProcesses[instance.ID]
	if !ok {
		return
	}
	historic.EndTime = cloneTime(&cc.now)
	historic.Ended = true
	historic.DeleteReason = instance.DeleteReason
}

// recordActivityStart 写入历史活动实例。
// @param cc 命令上下文
// @param execution 执行
// @param element 活动元素
// @return 历史记录 ID
func (e *Engine) recordActivityStart(cc *commandContext, execution *executionState, element model.FlowElement) string {
	if !e.historyAtLeast(HistoryActivity) {
		return ""
	}
	id := e.newID()
	historic := &api.HistoricActivityInstance{
		ID:                id,
		ProcessInstanceID: execution.ProcessInstanceID,
		ActivityID:        element.GetID(),
		ActivityName:      element.GetName(),
		ActivityType:      element.ElementType(),
		StartTime:         cc.now,
	}
	e.state.HistoricActivities[id] = historic
	return id
}

// recordActivityEnd 更新历史活动结束信息。
// @param cc 命令上下文
// @param historicID 历史记录 ID
func (e *Engine) recordActivityEnd(cc *commandContext, historicID string) {
	if historicID == "" {
		return
	}
	historic, ok := e.state.HistoricActivities[historicID]
	if !ok || historic.EndTime != nil {
		return
	}
	historic.EndTime = cloneTime(&cc.now)
	historic.DurationMillis = cc.now.Sub(historic.StartTime).Milliseconds()
}

// recordTaskStart 写入历史任务实例。
// @param cc 命令上下文
// @param task 任务
// @return 历史记录 ID
func (e *Engine) recordTaskStart(cc *commandContext, task *taskState) string {
	if !e.taskHistoryEnabled() {
		return ""
	}
	id := task.Task.ID
	historic := &api.HistoricTaskInstance{
		ID:                id,
		Name:              task.Task.Name,
		ProcessInstanceID: task.Task.ProcessInstanceID,
		TaskDefinitionKey: task.Task.TaskDefinitionKey,
		Assignee:          task.Task.Assignee,
		FormKey:           task.Task.FormKey,
		TenantID:          task.Task.TenantID,
		StartTime:         task.Task.CreateTime,
	}
	e.state.HistoricTasks[id] = historic
	e.recordTaskIdentityLinks(cc, task)
	return id
}

// recordTaskEnd 更新历史任务结束信息。
// @param cc 命令上下文
// @param task 任务
func (e *Engine) recordTaskEnd(cc *commandContext, task *taskState) {
	if task.HistoricTaskID == "" {
		return
	}
	historic, ok := e.state.HistoricTasks[task.HistoricTaskID]
	if !ok {
		return
	}
	historic.EndTime = cloneTime(&cc.now)
	historic.DurationMillis = cc.now.Sub(historic.StartTime).Milliseconds()
	historic.Assignee = task.Task.Assignee
}

// taskHistoryEnabled 判断是否记录任务历史。
// @return 是否启用
func (e *Engine) taskHistoryEnabled() bool {
	// 与参考实现一致：task 级别或 audit 及以上记录任务历史
	return e.cfg.HistoryLevel == HistoryTask || e.historyAtLeast(HistoryAudit)
}

// recordHistoricVariable 记录历史变量。
// @param execution 执行
// @param name 变量名
// @param value 变量值
func (e *Engine) recordHistoricVariable(execution *executionState, name string, value any) {
	if !e.historyAtLeast(HistoryActivity) {
		return
	}
	key := execution.ID + "|" + name
	now := e.cfg.Clock.Now()
	if existing, ok := e.state.HistoricVariables[key]; ok {
		existing.Value = value
		existing.Type = variableTypeName(value)
		existing.LastUpdatedTime = now
		e.recordHistoricDetail(execution.ProcessInstanceID, execution.ID, "", execution.ActivityID, name, value, now)
		return
	}
	historic := &api.HistoricVariableInstance{
		ID:                e.newID(),
		Name:              name,
		Type:              variableTypeName(value),
		ProcessInstanceID: execution.ProcessInstanceID,
		ExecutionID:       execution.ID,
		Value:             value,
		CreateTime:        now,
		LastUpdatedTime:   now,
	}
	e.state.HistoricVariables[key] = historic
	e.recordHistoricDetail(execution.ProcessInstanceID, execution.ID, "", execution.ActivityID, name, value, now)
}

// recordTaskHistoricVariable 记录独立任务的历史变量。
// @param task 任务
// @param name 变量名
// @param value 变量值
// @param now 时间
func (e *Engine) recordTaskHistoricVariable(task *taskState, name string, value any, now time.Time) {
	if !e.historyAtLeast(HistoryActivity) {
		return
	}
	key := "task|" + task.Task.ID + "|" + name
	if existing, ok := e.state.HistoricVariables[key]; ok {
		existing.Value = value
		existing.Type = variableTypeName(value)
		existing.LastUpdatedTime = now
		e.recordHistoricDetail(task.Task.ProcessInstanceID, "", task.Task.ID, task.ActivityID, name, value, now)
		return
	}
	historic := &api.HistoricVariableInstance{
		ID:                e.newID(),
		Name:              name,
		Type:              variableTypeName(value),
		ProcessInstanceID: task.Task.ProcessInstanceID,
		TaskID:            task.Task.ID,
		Value:             value,
		CreateTime:        now,
		LastUpdatedTime:   now,
	}
	e.state.HistoricVariables[key] = historic
	e.recordHistoricDetail(task.Task.ProcessInstanceID, "", task.Task.ID, task.ActivityID, name, value, now)
}

// recordHistoricDetail 记录变量明细。
// @param processInstanceID 实例 ID
// @param executionID 执行 ID
// @param taskID 任务 ID
// @param activityID 活动 ID
// @param name 变量名
// @param value 变量值
// @param now 时间
func (e *Engine) recordHistoricDetail(processInstanceID, executionID, taskID, activityID, name string, value any, now time.Time) {
	detail := &api.HistoricDetail{
		ID:                e.newID(),
		Type:              "variableUpdate",
		ProcessInstanceID: processInstanceID,
		ExecutionID:       executionID,
		TaskID:            taskID,
		ActivityID:        activityID,
		Name:              name,
		VariableType:      variableTypeName(value),
		Value:             value,
		Time:              now,
	}
	e.state.HistoricDetails[detail.ID] = detail
}

// variableTypeName 返回变量类型名。
// @param value 变量值
// @return 类型名
func variableTypeName(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "boolean"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "long"
	case float32, float64:
		return "double"
	case []byte:
		return "bytes"
	default:
		return "json"
	}
}
