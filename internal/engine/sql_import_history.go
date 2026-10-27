package engine

import (
	"context"

	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
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
		return nil, nil
	})
	return err
}
