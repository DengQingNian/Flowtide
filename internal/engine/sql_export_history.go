package engine

import (
	"context"
	"sort"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	flowablerow "github.com/DengQingNian/Flowtide/internal/persistence/flowable"
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
		row, rowErr := historyRow("ACT_HI_PROCINST", []string{"ID_", "REV_", "PROC_INST_ID_", "BUSINESS_KEY_", "PROC_DEF_ID_", "START_TIME_", "END_TIME_", "DURATION_", "START_USER_ID_", "START_ACT_ID_", "END_ACT_ID_", "SUPER_PROCESS_INSTANCE_ID_", "DELETE_REASON_", "TENANT_ID_", "NAME_", "CALLBACK_ID_", "CALLBACK_TYPE_", "REFERENCE_ID_", "REFERENCE_TYPE_", "PROPAGATED_STAGE_INST_ID_", "BUSINESS_STATUS_", "DUE_DATE_", "CLAIM_TIME_", "CLAIMED_BY_", "END_USER_ID_", "STATE_"})
		if rowErr != nil {
			return nil, rowErr
		}
		duration := historic.DurationMillis
		if duration == 0 && historic.EndTime != nil {
			duration = historic.EndTime.Sub(historic.StartTime).Milliseconds()
		}
		revision := historic.Revision
		if revision == 0 {
			revision = 1
		}
		tenant := historic.TenantID
		if instance != nil && instance.TenantID != "" {
			tenant = instance.TenantID
		}
		_ = row.Set("ID_", historic.ID)
		_ = row.Set("REV_", int64(revision))
		_ = row.Set("PROC_INST_ID_", historic.ID)
		_ = row.Set("BUSINESS_KEY_", historic.BusinessKey)
		_ = row.Set("PROC_DEF_ID_", historic.ProcessDefinitionID)
		_ = row.Set("START_TIME_", historic.StartTime)
		_ = row.Set("END_TIME_", optionalTimeValue(historic.EndTime))
		_ = row.Set("DURATION_", duration)
		_ = row.Set("START_USER_ID_", historic.StartUserID)
		_ = row.Set("START_ACT_ID_", historic.StartActivityID)
		_ = row.Set("END_ACT_ID_", historic.EndActivityID)
		_ = row.Set("SUPER_PROCESS_INSTANCE_ID_", historic.SuperProcessInstanceID)
		_ = row.Set("DELETE_REASON_", historic.DeleteReason)
		_ = row.Set("TENANT_ID_", tenant)
		_ = row.Set("NAME_", historic.Name)
		_ = row.Set("CALLBACK_ID_", historic.CallbackID)
		_ = row.Set("CALLBACK_TYPE_", historic.CallbackType)
		_ = row.Set("REFERENCE_ID_", historic.ReferenceID)
		_ = row.Set("REFERENCE_TYPE_", historic.ReferenceType)
		_ = row.Set("PROPAGATED_STAGE_INST_ID_", historic.PropagatedStageInstanceID)
		_ = row.Set("BUSINESS_STATUS_", historic.BusinessStatus)
		_ = row.Set("DUE_DATE_", optionalTimeValue(historic.DueDate))
		_ = row.Set("CLAIM_TIME_", optionalTimeValue(historic.ClaimTime))
		_ = row.Set("CLAIMED_BY_", historic.ClaimedBy)
		_ = row.Set("END_USER_ID_", historic.EndUserID)
		_ = row.Set("STATE_", historic.State)
		rows = append(rows, *row)
	}

	activities := make([]*api.HistoricActivityInstance, 0, len(e.state.HistoricActivities))
	for _, historic := range e.state.HistoricActivities {
		activities = append(activities, historic)
	}
	sort.Slice(activities, func(i, j int) bool { return activities[i].ID < activities[j].ID })
	for _, historic := range activities {
		instance, _ := e.findInstance(historic.ProcessInstanceID)
		row, rowErr := historyRow("ACT_HI_ACTINST", []string{"ID_", "REV_", "PROC_DEF_ID_", "PROC_INST_ID_", "EXECUTION_ID_", "ACT_ID_", "TASK_ID_", "CALL_PROC_INST_ID_", "ACT_NAME_", "ACT_TYPE_", "ASSIGNEE_", "COMPLETED_BY_", "START_TIME_", "END_TIME_", "TRANSACTION_ORDER_", "DURATION_", "DELETE_REASON_", "TENANT_ID_"})
		if rowErr != nil {
			return nil, rowErr
		}
		definitionID, tenant, executionID := historic.ProcessDefinitionID, historic.TenantID, historic.ExecutionID
		if instance != nil {
			if definitionID == "" {
				definitionID = instance.ProcessDefinitionID
			}
			if tenant == "" {
				tenant = instance.TenantID
			}
			if executionID == "" {
				executionID = instance.RootExecutionID
			}
		}
		revision := historic.Revision
		if revision == 0 {
			revision = 1
		}
		_ = row.Set("ID_", historic.ID)
		_ = row.Set("REV_", int64(revision))
		_ = row.Set("PROC_DEF_ID_", definitionID)
		_ = row.Set("PROC_INST_ID_", historic.ProcessInstanceID)
		_ = row.Set("EXECUTION_ID_", executionID)
		_ = row.Set("ACT_ID_", historic.ActivityID)
		_ = row.Set("TASK_ID_", historic.TaskID)
		_ = row.Set("CALL_PROC_INST_ID_", historic.CallProcessInstanceID)
		_ = row.Set("ACT_NAME_", historic.ActivityName)
		_ = row.Set("ACT_TYPE_", historic.ActivityType)
		_ = row.Set("ASSIGNEE_", historic.Assignee)
		_ = row.Set("COMPLETED_BY_", historic.CompletedBy)
		_ = row.Set("START_TIME_", historic.StartTime)
		_ = row.Set("END_TIME_", optionalTimeValue(historic.EndTime))
		_ = row.Set("TRANSACTION_ORDER_", int64(historic.TransactionOrder))
		_ = row.Set("DURATION_", historic.DurationMillis)
		_ = row.Set("DELETE_REASON_", historic.DeleteReason)
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
		row, rowErr := historyRow("ACT_HI_TASKINST", []string{"ID_", "REV_", "PROC_DEF_ID_", "TASK_DEF_ID_", "TASK_DEF_KEY_", "PROC_INST_ID_", "EXECUTION_ID_", "SCOPE_ID_", "SUB_SCOPE_ID_", "SCOPE_TYPE_", "SCOPE_DEFINITION_ID_", "PROPAGATED_STAGE_INST_ID_", "STATE_", "NAME_", "PARENT_TASK_ID_", "DESCRIPTION_", "OWNER_", "ASSIGNEE_", "START_TIME_", "IN_PROGRESS_TIME_", "IN_PROGRESS_STARTED_BY_", "CLAIM_TIME_", "CLAIMED_BY_", "SUSPENDED_TIME_", "SUSPENDED_BY_", "END_TIME_", "COMPLETED_BY_", "DURATION_", "DELETE_REASON_", "PRIORITY_", "IN_PROGRESS_DUE_DATE_", "DUE_DATE_", "FORM_KEY_", "CATEGORY_", "TENANT_ID_", "LAST_UPDATED_TIME_"})
		if rowErr != nil {
			return nil, rowErr
		}
		definitionID, tenant := historic.ProcessDefinitionID, historic.TenantID
		if instance != nil {
			if definitionID == "" {
				definitionID = instance.ProcessDefinitionID
			}
			if tenant == "" {
				tenant = instance.TenantID
			}
		}
		revision := historic.Revision
		if revision == 0 {
			revision = 1
		}
		_ = row.Set("ID_", historic.ID)
		_ = row.Set("REV_", int64(revision))
		_ = row.Set("PROC_DEF_ID_", definitionID)
		_ = row.Set("TASK_DEF_ID_", historic.TaskDefinitionID)
		_ = row.Set("TASK_DEF_KEY_", historic.TaskDefinitionKey)
		_ = row.Set("PROC_INST_ID_", historic.ProcessInstanceID)
		_ = row.Set("EXECUTION_ID_", historic.ExecutionID)
		_ = row.Set("SCOPE_ID_", historic.ScopeID)
		_ = row.Set("SUB_SCOPE_ID_", historic.SubScopeID)
		_ = row.Set("SCOPE_TYPE_", historic.ScopeType)
		_ = row.Set("SCOPE_DEFINITION_ID_", historic.ScopeDefinitionID)
		_ = row.Set("PROPAGATED_STAGE_INST_ID_", historic.PropagatedStageInstanceID)
		_ = row.Set("STATE_", historic.State)
		_ = row.Set("NAME_", historic.Name)
		_ = row.Set("PARENT_TASK_ID_", historic.ParentTaskID)
		_ = row.Set("DESCRIPTION_", historic.Description)
		_ = row.Set("OWNER_", historic.Owner)
		_ = row.Set("ASSIGNEE_", historic.Assignee)
		_ = row.Set("START_TIME_", historic.StartTime)
		_ = row.Set("IN_PROGRESS_TIME_", optionalTimeValue(historic.InProgressTime))
		_ = row.Set("IN_PROGRESS_STARTED_BY_", historic.InProgressStartedBy)
		_ = row.Set("CLAIM_TIME_", optionalTimeValue(historic.ClaimTime))
		_ = row.Set("CLAIMED_BY_", historic.ClaimedBy)
		_ = row.Set("SUSPENDED_TIME_", optionalTimeValue(historic.SuspendedTime))
		_ = row.Set("SUSPENDED_BY_", historic.SuspendedBy)
		_ = row.Set("END_TIME_", optionalTimeValue(historic.EndTime))
		_ = row.Set("COMPLETED_BY_", historic.CompletedBy)
		_ = row.Set("DURATION_", historic.DurationMillis)
		_ = row.Set("DELETE_REASON_", historic.DeleteReason)
		_ = row.Set("PRIORITY_", int64(historic.Priority))
		_ = row.Set("IN_PROGRESS_DUE_DATE_", optionalTimeValue(historic.InProgressDueDate))
		_ = row.Set("DUE_DATE_", optionalTimeValue(historic.DueDate))
		_ = row.Set("FORM_KEY_", historic.FormKey)
		_ = row.Set("CATEGORY_", historic.Category)
		_ = row.Set("TENANT_ID_", tenant)
		_ = row.Set("LAST_UPDATED_TIME_", optionalTimeValue(historic.LastUpdatedTime))
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
		row, rowErr := historyRow("ACT_HI_COMMENT", []string{"ID_", "TYPE_", "TIME_", "USER_ID_", "TASK_ID_", "PROC_INST_ID_", "ACTION_", "MESSAGE_", "FULL_MSG_"})
		if rowErr != nil {
			return nil, rowErr
		}
		// 引擎新增评论没有显式动作，沿用既有 AddComment；Java 事件型记录保留原动作。
		action := comment.Action
		if action == "" {
			action = "AddComment"
		}
		_ = row.Set("ID_", comment.ID)
		_ = row.Set("TYPE_", comment.Type)
		_ = row.Set("TIME_", comment.Time)
		_ = row.Set("USER_ID_", comment.UserID)
		_ = row.Set("TASK_ID_", comment.TaskID)
		_ = row.Set("PROC_INST_ID_", comment.ProcessInstanceID)
		_ = row.Set("ACTION_", action)
		_ = row.Set("MESSAGE_", comment.Message)
		_ = row.Set("FULL_MSG_", comment.FullMessage)
		rows = append(rows, *row)
	}
	attachments := make([]*api.Attachment, 0, len(e.state.Attachments))
	for _, attachment := range e.state.Attachments {
		attachments = append(attachments, attachment)
	}
	sort.Slice(attachments, func(i, j int) bool { return attachments[i].ID < attachments[j].ID })
	for _, attachment := range attachments {
		row, rowErr := historyRow("ACT_HI_ATTACHMENT", []string{"ID_", "REV_", "NAME_", "DESCRIPTION_", "TYPE_", "TASK_ID_", "PROC_INST_ID_", "URL_", "CONTENT_ID_", "TIME_"})
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
		contentID := ""
		if _, ok := e.state.AttachmentContents[attachment.ID]; ok {
			contentID = byteArrayID("", "attachment/"+attachment.ID)
		}
		_ = row.Set("CONTENT_ID_", contentID)
		_ = row.Set("TIME_", attachment.Time)
		rows = append(rows, *row)
	}
	logs := make([]*api.HistoricTaskLog, 0, len(e.state.TaskLogs))
	for _, log := range e.state.TaskLogs {
		logs = append(logs, log)
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].ID < logs[j].ID })
	for _, log := range logs {
		row, rowErr := historyRow("ACT_HI_TSK_LOG", []string{"ID_", "TYPE_", "TASK_ID_", "TIME_STAMP_", "USER_ID_", "DATA_", "EXECUTION_ID_", "PROC_INST_ID_", "PROC_DEF_ID_", "TENANT_ID_"})
		if rowErr != nil {
			return nil, rowErr
		}
		_ = row.Set("ID_", log.ID)
		_ = row.Set("TYPE_", log.Type)
		_ = row.Set("TASK_ID_", log.TaskID)
		_ = row.Set("TIME_STAMP_", log.Time)
		_ = row.Set("USER_ID_", log.UserID)
		_ = row.Set("DATA_", log.Data)
		_ = row.Set("EXECUTION_ID_", log.ExecutionID)
		_ = row.Set("PROC_INST_ID_", log.ProcessInstanceID)
		_ = row.Set("PROC_DEF_ID_", log.ProcessDefinitionID)
		_ = row.Set("TENANT_ID_", log.TenantID)
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
	return expandVariableRows(e.normalizeExecutionReferences(rows))
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
