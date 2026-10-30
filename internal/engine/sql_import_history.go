package engine

import (
	"context"

	"github.com/DengQingNian/Flowtide/api"
	flowablerow "github.com/DengQingNian/Flowtide/internal/persistence/flowable"
)

// ImportHistoricRows 从历史行恢复历史流程、活动、任务与变量。
// @param ctx 上下文
// @param rows 行列表
// @return 导入错误
func (e *Engine) ImportHistoricRows(ctx context.Context, rows []flowablerow.Row) error {
	snapshot, err := flowablerow.DecodeHistoricRows(rows)
	if err != nil {
		return err
	}
	_, err = e.execute(ctx, func(cc *commandContext) (any, error) {
		for i := range snapshot.Processes {
			process := snapshot.Processes[i]
			e.state.HistoricProcesses[process.ID] = &process
		}
		for i := range snapshot.Activities {
			activity := snapshot.Activities[i]
			e.state.HistoricActivities[activity.ID] = &activity
		}
		for i := range snapshot.Tasks {
			task := snapshot.Tasks[i]
			e.state.HistoricTasks[task.ID] = &task
		}
		for i := range snapshot.Variables {
			variable := snapshot.Variables[i]
			e.state.HistoricVariables[variable.ID] = &variable
		}
		for i := range snapshot.Comments {
			comment := snapshot.Comments[i]
			e.state.Comments[comment.ID] = &comment
		}
		for i := range snapshot.Attachments {
			attachment := snapshot.Attachments[i]
			e.state.Attachments[attachment.ID] = &attachment
		}
		for i := range snapshot.Links {
			link := snapshot.Links[i]
			e.state.HistoricLinks[link.ID] = &link
		}
		for i := range snapshot.Details {
			detail := snapshot.Details[i]
			e.state.HistoricDetails[detail.ID] = &detail
		}
		for i := range snapshot.TaskLogs {
			log := snapshot.TaskLogs[i]
			e.state.TaskLogs[log.ID] = &log
			if log.ID > e.state.NextTaskLogID {
				e.state.NextTaskLogID = log.ID
			}
		}
		return nil, e.restoreHistoricActivityReferences(rows)
	})
	return err
}

// restoreHistoricActivityReferences 按原始执行标识关联尚未结束的活动历史。
// @param rows 原始历史行，包含标准执行引用
// @return 同一执行存在多个当前活动历史时的歧义错误
func (e *Engine) restoreHistoricActivityReferences(rows []flowablerow.Row) error {
	matched := make(map[string]string)
	for _, row := range rows {
		if row.Table != "ACT_HI_ACTINST" {
			continue
		}
		id, _ := row.Values["ID_"].(string)
		historic := e.state.HistoricActivities[id]
		if historic == nil || historic.EndTime != nil {
			continue
		}
		executionID, _ := row.Values["EXECUTION_ID_"].(string)
		execution := e.state.Executions[executionID]
		// 控制扩展已有准确关联时保持原值；Java 行按执行、实例和活动共同匹配。
		if execution == nil || execution.CurrentHistActivityID != "" || execution.ProcessInstanceID != historic.ProcessInstanceID || execution.ActivityID != historic.ActivityID {
			continue
		}
		if previous := matched[executionID]; previous != "" && previous != id {
			return api.NewError("ENGINE-0922", "执行存在多个未结束的活动历史，无法确定续跑关联", api.ErrBadRequest)
		}
		matched[executionID] = id
	}
	for executionID, historicID := range matched {
		e.state.Executions[executionID].CurrentHistActivityID = historicID
	}
	return nil
}
