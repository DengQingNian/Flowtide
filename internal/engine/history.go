package engine

import (
	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
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
		StartTime:         task.Task.CreateTime,
	}
	e.state.HistoricTasks[id] = historic
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
