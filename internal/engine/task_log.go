package engine

import (
	"context"
	"encoding/json"

	"github.com/flowable-go/flowable/api"
)

// actorOrValue 返回可信操作人，缺失时回退到请求值。
// @param ctx 调用方上下文
// @param value 请求中的操作人
// @return 操作人
func actorOrValue(ctx context.Context, value string) string {
	if actor := actorFromContext(ctx); actor != "" {
		return actor
	}
	return value
}

// jsonNullableString 将可选文本转为原版 JSON 表示，空值为 null 字面量。
// @param value 文本
// @return JSON 字符串
func jsonNullableString(value string) string {
	if value == "" {
		return "null"
	}
	b, _ := json.Marshal(value)
	return string(b)
}

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
