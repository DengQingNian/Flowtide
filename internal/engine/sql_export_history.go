package engine

import (
	"context"
	"sort"
	"time"

	"github.com/flowable-go/flowable/api"
	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
)

// ExportHistoricRows 导出历史流程、活动、任务与变量行。
// @param ctx 上下文
// @return 行列表
// @return 导出错误
func (e *Engine) ExportHistoricRows(ctx context.Context) ([]flowablerow.Row, error) {
	return e.readRows(e.historicRowsLocked)
}

// historicRowsLocked 在锁内导出历史行。
// @return 行列表
// @return 导出错误
func (e *Engine) historicRowsLocked() ([]flowablerow.Row, error) {

	rows := make([]flowablerow.Row, 0)
	processes := make([]*api.HistoricProcessInstance, 0, len(e.state.HistoricProcesses))
	for _, historic := range e.state.HistoricProcesses {
		processes = append(processes, historic)
	}
	sort.Slice(processes, func(i, j int) bool { return processes[i].ID < processes[j].ID })
	for _, historic := range processes {
		instance, _ := e.findInstance(historic.ID)
		row, rowErr := historyRow("ACT_HI_PROCINST", []string{"ID_", "REV_", "PROC_INST_ID_", "BUSINESS_KEY_", "PROC_DEF_ID_", "START_TIME_", "END_TIME_", "DURATION_", "DELETE_REASON_", "TENANT_ID_", "NAME_"})
		if rowErr != nil {
			return nil, rowErr
		}
		duration := int64(0)
		if historic.EndTime != nil {
			duration = historic.EndTime.Sub(historic.StartTime).Milliseconds()
		}
		_ = row.Set("ID_", historic.ID)
		_ = row.Set("REV_", 1)
		_ = row.Set("PROC_INST_ID_", historic.ID)
		_ = row.Set("BUSINESS_KEY_", historic.BusinessKey)
		_ = row.Set("PROC_DEF_ID_", historic.ProcessDefinitionID)
		_ = row.Set("START_TIME_", historic.StartTime)
		_ = row.Set("END_TIME_", optionalTimeValue(historic.EndTime))
		_ = row.Set("DURATION_", duration)
		_ = row.Set("DELETE_REASON_", historic.DeleteReason)
		_ = row.Set("TENANT_ID_", historic.TenantID)
		_ = row.Set("NAME_", historic.Name)
		if instance != nil && instance.TenantID != "" {
			_ = row.Set("TENANT_ID_", instance.TenantID)
		}
		rows = append(rows, *row)
	}

	activities := make([]*api.HistoricActivityInstance, 0, len(e.state.HistoricActivities))
	for _, historic := range e.state.HistoricActivities {
		activities = append(activities, historic)
	}
	sort.Slice(activities, func(i, j int) bool { return activities[i].ID < activities[j].ID })
	for _, historic := range activities {
		instance, _ := e.findInstance(historic.ProcessInstanceID)
		row, rowErr := historyRow("ACT_HI_ACTINST", []string{"ID_", "REV_", "PROC_DEF_ID_", "PROC_INST_ID_", "EXECUTION_ID_", "ACT_ID_", "ACT_NAME_", "ACT_TYPE_", "START_TIME_", "END_TIME_", "DURATION_", "TENANT_ID_"})
		if rowErr != nil {
			return nil, rowErr
		}
		definitionID, tenant, executionID := "", "", historic.ProcessInstanceID
		if instance != nil {
			definitionID = instance.ProcessDefinitionID
			tenant = instance.TenantID
			executionID = instance.RootExecutionID
		}
		duration := historic.DurationMillis
		_ = row.Set("ID_", historic.ID)
		_ = row.Set("REV_", 1)
		_ = row.Set("PROC_DEF_ID_", definitionID)
		_ = row.Set("PROC_INST_ID_", historic.ProcessInstanceID)
		_ = row.Set("EXECUTION_ID_", executionID)
		_ = row.Set("ACT_ID_", historic.ActivityID)
		_ = row.Set("ACT_NAME_", historic.ActivityName)
		_ = row.Set("ACT_TYPE_", historic.ActivityType)
		_ = row.Set("START_TIME_", historic.StartTime)
		_ = row.Set("END_TIME_", optionalTimeValue(historic.EndTime))
		_ = row.Set("DURATION_", duration)
		_ = row.Set("TENANT_ID_", tenant)
		rows = append(rows, *row)
	}

	tasks := make([]*api.HistoricTaskInstance, 0, len(e.state.HistoricTasks))
	for _, historic := range e.state.HistoricTasks {
		tasks = append(tasks, historic)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	for _, historic := range tasks {
		instance, _ := e.findInstance(historic.ProcessInstanceID)
		row, rowErr := historyRow("ACT_HI_TASKINST", []string{"ID_", "REV_", "PROC_DEF_ID_", "TASK_DEF_KEY_", "PROC_INST_ID_", "NAME_", "ASSIGNEE_", "START_TIME_", "END_TIME_", "DURATION_", "TENANT_ID_"})
		if rowErr != nil {
			return nil, rowErr
		}
		definitionID, tenant := "", ""
		if instance != nil {
			definitionID = instance.ProcessDefinitionID
			tenant = instance.TenantID
		}
		_ = row.Set("ID_", historic.ID)
		_ = row.Set("REV_", 1)
		_ = row.Set("PROC_DEF_ID_", definitionID)
		_ = row.Set("TASK_DEF_KEY_", historic.TaskDefinitionKey)
		_ = row.Set("PROC_INST_ID_", historic.ProcessInstanceID)
		_ = row.Set("NAME_", historic.Name)
		_ = row.Set("ASSIGNEE_", historic.Assignee)
		_ = row.Set("START_TIME_", historic.StartTime)
		_ = row.Set("END_TIME_", optionalTimeValue(historic.EndTime))
		_ = row.Set("DURATION_", historic.DurationMillis)
		_ = row.Set("TENANT_ID_", tenant)
		rows = append(rows, *row)
	}

	variables := make([]*api.HistoricVariableInstance, 0, len(e.state.HistoricVariables))
	for _, historic := range e.state.HistoricVariables {
		variables = append(variables, historic)
	}
	sort.Slice(variables, func(i, j int) bool { return variables[i].ID < variables[j].ID })
	for _, historic := range variables {
		row, rowErr := historyRow("ACT_HI_VARINST", []string{"ID_", "REV_", "PROC_INST_ID_", "EXECUTION_ID_", "TASK_ID_", "NAME_", "VAR_TYPE_", "LONG_", "DOUBLE_", "TEXT_", "CREATE_TIME_", "LAST_UPDATED_TIME_"})
		if rowErr != nil {
			return nil, rowErr
		}
		varType, longValue, doubleValue, textValue := variableColumns(historic.Value)
		_ = row.Set("ID_", historic.ID)
		_ = row.Set("REV_", 1)
		_ = row.Set("PROC_INST_ID_", historic.ProcessInstanceID)
		_ = row.Set("EXECUTION_ID_", historic.ExecutionID)
		_ = row.Set("TASK_ID_", historic.TaskID)
		_ = row.Set("NAME_", historic.Name)
		_ = row.Set("VAR_TYPE_", varType)
		_ = row.Set("LONG_", longValue)
		_ = row.Set("DOUBLE_", doubleValue)
		_ = row.Set("TEXT_", textValue)
		_ = row.Set("CREATE_TIME_", historic.CreateTime)
		_ = row.Set("LAST_UPDATED_TIME_", historic.LastUpdatedTime)
		rows = append(rows, *row)
	}
	comments := make([]*api.Comment, 0, len(e.state.Comments))
	for _, comment := range e.state.Comments {
		comments = append(comments, comment)
	}
	sort.Slice(comments, func(i, j int) bool { return comments[i].ID < comments[j].ID })
	for _, comment := range comments {
		row, rowErr := historyRow("ACT_HI_COMMENT", []string{"ID_", "TYPE_", "TIME_", "USER_ID_", "TASK_ID_", "PROC_INST_ID_", "ACTION_", "MESSAGE_"})
		if rowErr != nil {
			return nil, rowErr
		}
		_ = row.Set("ID_", comment.ID)
		_ = row.Set("TYPE_", comment.Type)
		_ = row.Set("TIME_", comment.Time)
		_ = row.Set("USER_ID_", comment.UserID)
		_ = row.Set("TASK_ID_", comment.TaskID)
		_ = row.Set("PROC_INST_ID_", comment.ProcessInstanceID)
		_ = row.Set("ACTION_", "AddComment")
		_ = row.Set("MESSAGE_", comment.Message)
		rows = append(rows, *row)
	}
	attachments := make([]*api.Attachment, 0, len(e.state.Attachments))
	for _, attachment := range e.state.Attachments {
		attachments = append(attachments, attachment)
	}
	sort.Slice(attachments, func(i, j int) bool { return attachments[i].ID < attachments[j].ID })
	for _, attachment := range attachments {
		row, rowErr := historyRow("ACT_HI_ATTACHMENT", []string{"ID_", "REV_", "NAME_", "DESCRIPTION_", "TYPE_", "TASK_ID_", "PROC_INST_ID_", "URL_", "TIME_"})
		if rowErr != nil {
			return nil, rowErr
		}
		_ = row.Set("ID_", attachment.ID)
		_ = row.Set("REV_", 1)
		_ = row.Set("NAME_", attachment.Name)
		_ = row.Set("DESCRIPTION_", attachment.Description)
		_ = row.Set("TYPE_", attachment.Type)
		_ = row.Set("TASK_ID_", attachment.TaskID)
		_ = row.Set("PROC_INST_ID_", attachment.ProcessInstanceID)
		_ = row.Set("URL_", attachment.URL)
		_ = row.Set("TIME_", attachment.Time)
		rows = append(rows, *row)
	}
	details := make([]*api.HistoricDetail, 0, len(e.state.HistoricDetails))
	for _, detail := range e.state.HistoricDetails {
		details = append(details, detail)
	}
	sort.Slice(details, func(i, j int) bool { return details[i].ID < details[j].ID })
	for _, detail := range details {
		row, rowErr := historyRow("ACT_HI_DETAIL", []string{"ID_", "TYPE_", "PROC_INST_ID_", "EXECUTION_ID_", "TASK_ID_", "ACT_INST_ID_", "NAME_", "VAR_TYPE_", "REV_", "TIME_", "LONG_", "DOUBLE_", "TEXT_"})
		if rowErr != nil {
			return nil, rowErr
		}
		varType, longValue, doubleValue, textValue := variableColumns(detail.Value)
		_ = row.Set("ID_", detail.ID)
		_ = row.Set("TYPE_", detail.Type)
		_ = row.Set("PROC_INST_ID_", detail.ProcessInstanceID)
		_ = row.Set("EXECUTION_ID_", detail.ExecutionID)
		_ = row.Set("TASK_ID_", detail.TaskID)
		_ = row.Set("ACT_INST_ID_", detail.ActivityID)
		_ = row.Set("NAME_", detail.Name)
		_ = row.Set("VAR_TYPE_", varType)
		_ = row.Set("REV_", 1)
		_ = row.Set("TIME_", detail.Time)
		_ = row.Set("LONG_", longValue)
		_ = row.Set("DOUBLE_", doubleValue)
		_ = row.Set("TEXT_", textValue)
		rows = append(rows, *row)
	}
	links := make([]*api.IdentityLink, 0, len(e.state.HistoricLinks))
	for _, link := range e.state.HistoricLinks {
		links = append(links, link)
	}
	sort.Slice(links, func(i, j int) bool { return links[i].ID < links[j].ID })
	for _, link := range links {
		row, rowErr := historyRow("ACT_HI_IDENTITYLINK", []string{"ID_", "GROUP_ID_", "TYPE_", "USER_ID_", "TASK_ID_", "CREATE_TIME_", "PROC_INST_ID_"})
		if rowErr != nil {
			return nil, rowErr
		}
		_ = row.Set("ID_", link.ID)
		_ = row.Set("GROUP_ID_", link.GroupID)
		_ = row.Set("TYPE_", link.Type)
		_ = row.Set("USER_ID_", link.UserID)
		_ = row.Set("TASK_ID_", link.TaskID)
		_ = row.Set("CREATE_TIME_", link.CreateTime)
		_ = row.Set("PROC_INST_ID_", link.ProcessInstanceID)
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Table == rows[j].Table {
			return rowID(rows[i]) < rowID(rows[j])
		}
		return rows[i].Table < rows[j].Table
	})
	return rows, nil
}

// historyRow 创建带插入列的历史行。
// @param table 表名
// @param columns 插入列
// @return 行
// @return 错误
func historyRow(table string, columns []string) (*flowablerow.Row, error) {
	row, err := flowablerow.NewRow(table)
	if err != nil {
		return nil, err
	}
	if err := row.SetInsertColumns(columns); err != nil {
		return nil, err
	}
	return row, nil
}

// optionalTimeValue 将时间指针转换为可空值。
// @param value 时间指针
// @return 时间或 nil
func optionalTimeValue(value *time.Time) any {
	if value == nil {
		return nil
	}
	return *value
}
