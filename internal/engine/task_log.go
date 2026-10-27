package engine

import (
	"github.com/flowable-go/flowable/api"
)

// recordTaskLog 记录任务生命周期日志。
// @param cc 命令上下文
// @param task 任务
// @param eventType 事件类型
// @param userID 操作人
// @param data 附加数据
func (e *Engine) recordTaskLog(cc *commandContext, task *taskState, eventType, userID, data string) {
	if !e.historyAtLeast(HistoryTask) {
		return
	}
	e.state.NextTaskLogID++
	tenant := ""
	if instance, ok := e.findInstance(task.Task.ProcessInstanceID); ok {
		tenant = instance.TenantID
	}
	log := &api.HistoricTaskLog{
		ID:                  e.state.NextTaskLogID,
		Type:                eventType,
		TaskID:              task.Task.ID,
		Time:                cc.now,
		UserID:              userID,
		Data:                data,
		ExecutionID:         task.ExecutionID,
		ProcessInstanceID:   task.Task.ProcessInstanceID,
		ProcessDefinitionID: task.Task.ProcessDefinitionID,
		TenantID:            tenant,
	}
	e.state.TaskLogs[log.ID] = log
}
