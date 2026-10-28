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
		State:                "running",
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
	historic.State = "completed"
	historic.DeleteReason = instance.DeleteReason
	historic.DurationMillis = cc.now.Sub(historic.StartTime).Milliseconds()
}

// recordActivityStart 写入历史活动实例并触发活动开始监听器。
// 历史级别不足时仍触发开始监听器，但返回空历史 ID。
// @param cc 命令上下文
// @param execution 执行
// @param element 活动元素
// @return 历史记录 ID
func (e *Engine) recordActivityStart(cc *commandContext, execution *executionState, element model.FlowElement) string {
	if !e.historyAtLeast(HistoryActivity) {
		// 历史关闭时仍触发开始监听器，但没有可返回的历史记录 ID
		e.invokeActivityStartExecutionListeners(cc, execution, element)
		return ""
	}
	id := e.newID()
	instance, _ := e.findInstance(execution.ProcessInstanceID)
	tenant := ""
	if instance != nil {
		tenant = instance.TenantID
	}
	historic := &api.HistoricActivityInstance{
		ID:                  id,
		ProcessDefinitionID: execution.DefinitionID,
		ProcessInstanceID:   execution.ProcessInstanceID,
		ExecutionID:         execution.ID,
		ActivityID:          element.GetID(),
		ActivityName:        element.GetName(),
		ActivityType:        element.ElementType(),
		StartTime:           cc.now,
		TransactionOrder:    cc.nextTransactionOrder(),
		TenantID:            tenant,
	}
	e.state.HistoricActivities[id] = historic
	e.invokeActivityStartExecutionListeners(cc, execution, element)
	return id
}

// recordActivityEnd 更新历史活动结束信息并触发活动结束监听器。
// 通过历史记录反查活动与执行；记录不存在时直接返回。
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
	e.invokeActivityEndExecutionListeners(cc, historic)
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
		ID:                  id,
		Name:                task.Task.Name,
		ProcessDefinitionID: task.Task.ProcessDefinitionID,
		ProcessInstanceID:   task.Task.ProcessInstanceID,
		ExecutionID:         task.ExecutionID,
		TaskDefinitionKey:   task.Task.TaskDefinitionKey,
		Assignee:            task.Task.Assignee,
		FormKey:             task.Task.FormKey,
		TenantID:            task.Task.TenantID,
		StartTime:           task.Task.CreateTime,
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
	if existing := e.historicVariableForOwner(key, execution.ID, "", name); existing != nil {
		existing.Value = copyValue(value)
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
		Value:             copyValue(value),
		CreateTime:        now,
		LastUpdatedTime:   now,
	}
	e.state.HistoricVariables[key] = historic
	e.recordHistoricDetail(execution.ProcessInstanceID, execution.ID, "", execution.ActivityID, name, value, now)
}

// recordTaskHistoricVariable 记录流程任务或独立任务的局部变量历史。
// @param task 任务
// @param name 变量名
// @param value 变量值
// @param now 时间
func (e *Engine) recordTaskHistoricVariable(task *taskState, name string, value any, now time.Time) {
	if !e.historyAtLeast(HistoryActivity) {
		return
	}
	key := "task|" + task.Task.ID + "|" + name
	if existing := e.historicVariableForOwner(key, "", task.Task.ID, name); existing != nil {
		existing.Value = copyValue(value)
		existing.Type = variableTypeName(value)
		existing.LastUpdatedTime = now
		e.recordHistoricDetail(task.Task.ProcessInstanceID, task.ExecutionID, task.Task.ID, task.ActivityID, name, value, now)
		return
	}
	historic := &api.HistoricVariableInstance{
		ID:                e.newID(),
		Name:              name,
		Type:              variableTypeName(value),
		ProcessInstanceID: task.Task.ProcessInstanceID,
		ExecutionID:       task.ExecutionID,
		TaskID:            task.Task.ID,
		Value:             copyValue(value),
		CreateTime:        now,
		LastUpdatedTime:   now,
	}
	e.state.HistoricVariables[key] = historic
	e.recordHistoricDetail(task.Task.ProcessInstanceID, task.ExecutionID, task.Task.ID, task.ActivityID, name, value, now)
}

// historicVariableForOwner 查找历史变量并在首次更新时重建从行或快照导入的归属索引。
// @param key 引擎使用的归属复合键
// @param executionID 执行标识，任务局部变量时为空
// @param taskID 任务标识，执行变量时为空
// @param name 变量名称
// @return 已有历史变量，不存在时为空
func (e *Engine) historicVariableForOwner(key, executionID, taskID, name string) *api.HistoricVariableInstance {
	if existing := e.state.HistoricVariables[key]; existing != nil {
		return existing
	}
	// 旧快照按历史行主键建索引，重新关联时保留原主键及创建时间。
	for oldKey, existing := range e.state.HistoricVariables {
		if existing.Name == name && existing.TaskID == taskID && (taskID != "" || existing.ExecutionID == executionID) {
			delete(e.state.HistoricVariables, oldKey)
			e.state.HistoricVariables[key] = existing
			return existing
		}
	}
	return nil
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
		Value:             copyValue(value),
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
		return "binary"
	default:
		return "json"
	}
}
