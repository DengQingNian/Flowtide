package flowable

import (
	"sort"

	"github.com/flowable-go/flowable/api"
)

// HistoricSnapshot 是历史行恢复结果。
type HistoricSnapshot struct {
	Processes   []api.HistoricProcessInstance  // 历史流程
	Activities  []api.HistoricActivityInstance // 历史活动
	Tasks       []api.HistoricTaskInstance     // 历史任务
	Variables   []api.HistoricVariableInstance // 历史变量
	Comments    []api.Comment                  // 评论
	Attachments []api.Attachment               // 附件
	Links       []api.IdentityLink             // 身份链接
	Details     []api.HistoricDetail           // 变量明细
	TaskLogs    []api.HistoricTaskLog          // 任务日志
}

// DecodeHistoricRows 将历史行恢复为快照。
// @param rows 行列表
// @return 历史快照
// @return 解析错误
func DecodeHistoricRows(rows []Row) (HistoricSnapshot, error) {
	snapshot := HistoricSnapshot{
		Processes:   make([]api.HistoricProcessInstance, 0),
		Activities:  make([]api.HistoricActivityInstance, 0),
		Tasks:       make([]api.HistoricTaskInstance, 0),
		Variables:   make([]api.HistoricVariableInstance, 0),
		Comments:    make([]api.Comment, 0),
		Attachments: make([]api.Attachment, 0),
		Links:       make([]api.IdentityLink, 0),
		Details:     make([]api.HistoricDetail, 0),
		TaskLogs:    make([]api.HistoricTaskLog, 0),
	}
	arrays, err := variableByteArrays(rows)
	if err != nil {
		return HistoricSnapshot{}, err
	}
	for _, row := range rows {
		switch row.Table {
		case "ACT_GE_BYTEARRAY":
			continue
		case "ACT_HI_PROCINST":
			process, err := decodeHistoricProcess(row)
			if err != nil {
				return HistoricSnapshot{}, err
			}
			snapshot.Processes = append(snapshot.Processes, process)
		case "ACT_HI_ACTINST":
			activity, err := decodeHistoricActivity(row)
			if err != nil {
				return HistoricSnapshot{}, err
			}
			snapshot.Activities = append(snapshot.Activities, activity)
		case "ACT_HI_TASKINST":
			task, err := decodeHistoricTask(row)
			if err != nil {
				return HistoricSnapshot{}, err
			}
			snapshot.Tasks = append(snapshot.Tasks, task)
		case "ACT_HI_VARINST":
			variable, err := decodeHistoricVariable(row, arrays)
			if err != nil {
				return HistoricSnapshot{}, err
			}
			snapshot.Variables = append(snapshot.Variables, variable)
		case "ACT_HI_COMMENT":
			comment, err := decodeHistoricComment(row)
			if err != nil {
				return HistoricSnapshot{}, err
			}
			snapshot.Comments = append(snapshot.Comments, comment)
		case "ACT_HI_TSK_LOG":
			log, err := decodeHistoricTaskLog(row)
			if err != nil {
				return HistoricSnapshot{}, err
			}
			snapshot.TaskLogs = append(snapshot.TaskLogs, log)
		case "ACT_HI_DETAIL":
			detail, err := decodeHistoricDetail(row, arrays)
			if err != nil {
				return HistoricSnapshot{}, err
			}
			snapshot.Details = append(snapshot.Details, detail)
		case "ACT_HI_IDENTITYLINK":
			link, err := decodeHistoricIdentityLink(row)
			if err != nil {
				return HistoricSnapshot{}, err
			}
			snapshot.Links = append(snapshot.Links, link)
		case "ACT_HI_ATTACHMENT":
			attachment, err := decodeHistoricAttachment(row)
			if err != nil {
				return HistoricSnapshot{}, err
			}
			snapshot.Attachments = append(snapshot.Attachments, attachment)
		default:
			return HistoricSnapshot{}, api.NewError("ROW-0030", "不支持的历史表: "+row.Table, api.ErrBadRequest)
		}
	}
	sort.Slice(snapshot.Processes, func(i, j int) bool { return snapshot.Processes[i].ID < snapshot.Processes[j].ID })
	sort.Slice(snapshot.Activities, func(i, j int) bool { return snapshot.Activities[i].ID < snapshot.Activities[j].ID })
	sort.Slice(snapshot.Tasks, func(i, j int) bool { return snapshot.Tasks[i].ID < snapshot.Tasks[j].ID })
	sort.Slice(snapshot.Variables, func(i, j int) bool { return snapshot.Variables[i].ID < snapshot.Variables[j].ID })
	sort.Slice(snapshot.Comments, func(i, j int) bool { return snapshot.Comments[i].ID < snapshot.Comments[j].ID })
	sort.Slice(snapshot.Attachments, func(i, j int) bool { return snapshot.Attachments[i].ID < snapshot.Attachments[j].ID })
	sort.Slice(snapshot.Links, func(i, j int) bool { return snapshot.Links[i].ID < snapshot.Links[j].ID })
	sort.Slice(snapshot.Details, func(i, j int) bool { return snapshot.Details[i].ID < snapshot.Details[j].ID })
	sort.Slice(snapshot.TaskLogs, func(i, j int) bool { return snapshot.TaskLogs[i].ID < snapshot.TaskLogs[j].ID })
	return snapshot, nil
}

// decodeHistoricProcess 解析历史流程行。
// @param row 行
// @return 历史流程
// @return 错误
func decodeHistoricProcess(row Row) (api.HistoricProcessInstance, error) {
	processInstanceID, err := stringValue(row, "PROC_INST_ID_")
	if err != nil {
		return api.HistoricProcessInstance{}, err
	}
	businessKey, err := stringValue(row, "BUSINESS_KEY_")
	if err != nil {
		return api.HistoricProcessInstance{}, err
	}
	definitionID, err := stringValue(row, "PROC_DEF_ID_")
	if err != nil {
		return api.HistoricProcessInstance{}, err
	}
	startTime, err := timeValue(row, "START_TIME_")
	if err != nil {
		return api.HistoricProcessInstance{}, err
	}
	endTime, err := optionalTime(row, "END_TIME_")
	if err != nil {
		return api.HistoricProcessInstance{}, err
	}
	deleteReason, err := stringValue(row, "DELETE_REASON_")
	if err != nil {
		return api.HistoricProcessInstance{}, err
	}
	tenant, err := stringValue(row, "TENANT_ID_")
	if err != nil {
		return api.HistoricProcessInstance{}, err
	}
	name, err := stringValue(row, "NAME_")
	if err != nil {
		return api.HistoricProcessInstance{}, err
	}
	return api.HistoricProcessInstance{
		ID: processInstanceID, ProcessDefinitionID: definitionID, BusinessKey: businessKey,
		Name: name, TenantID: tenant, StartTime: startTime, EndTime: endTime,
		DeleteReason: deleteReason, Ended: endTime != nil,
	}, nil
}

// decodeHistoricActivity 解析历史活动行。
// @param row 行
// @return 历史活动
// @return 错误
func decodeHistoricActivity(row Row) (api.HistoricActivityInstance, error) {
	id, err := stringValue(row, "ID_")
	if err != nil {
		return api.HistoricActivityInstance{}, err
	}
	processInstanceID, err := stringValue(row, "PROC_INST_ID_")
	if err != nil {
		return api.HistoricActivityInstance{}, err
	}
	activityID, err := stringValue(row, "ACT_ID_")
	if err != nil {
		return api.HistoricActivityInstance{}, err
	}
	activityName, err := stringValue(row, "ACT_NAME_")
	if err != nil {
		return api.HistoricActivityInstance{}, err
	}
	activityType, err := stringValue(row, "ACT_TYPE_")
	if err != nil {
		return api.HistoricActivityInstance{}, err
	}
	startTime, err := timeValue(row, "START_TIME_")
	if err != nil {
		return api.HistoricActivityInstance{}, err
	}
	endTime, err := optionalTime(row, "END_TIME_")
	if err != nil {
		return api.HistoricActivityInstance{}, err
	}
	duration, err := optionalInt(row, "DURATION_")
	if err != nil {
		return api.HistoricActivityInstance{}, err
	}
	activity := api.HistoricActivityInstance{
		ID: id, ProcessInstanceID: processInstanceID, ActivityID: activityID,
		ActivityName: activityName, ActivityType: activityType, StartTime: startTime, EndTime: endTime,
	}
	if duration != nil {
		activity.DurationMillis = *duration
	}
	return activity, nil
}

// decodeHistoricTask 解析历史任务行。
// @param row 行
// @return 历史任务
// @return 错误
func decodeHistoricTask(row Row) (api.HistoricTaskInstance, error) {
	id, err := stringValue(row, "ID_")
	if err != nil {
		return api.HistoricTaskInstance{}, err
	}
	name, err := stringValue(row, "NAME_")
	if err != nil {
		return api.HistoricTaskInstance{}, err
	}
	processInstanceID, err := stringValue(row, "PROC_INST_ID_")
	if err != nil {
		return api.HistoricTaskInstance{}, err
	}
	taskKey, err := stringValue(row, "TASK_DEF_KEY_")
	if err != nil {
		return api.HistoricTaskInstance{}, err
	}
	assignee, err := stringValue(row, "ASSIGNEE_")
	if err != nil {
		return api.HistoricTaskInstance{}, err
	}
	startTime, err := timeValue(row, "START_TIME_")
	if err != nil {
		return api.HistoricTaskInstance{}, err
	}
	endTime, err := optionalTime(row, "END_TIME_")
	if err != nil {
		return api.HistoricTaskInstance{}, err
	}
	duration, err := optionalInt(row, "DURATION_")
	if err != nil {
		return api.HistoricTaskInstance{}, err
	}
	task := api.HistoricTaskInstance{
		ID: id, Name: name, ProcessInstanceID: processInstanceID,
		TaskDefinitionKey: taskKey, Assignee: assignee, StartTime: startTime, EndTime: endTime,
	}
	if duration != nil {
		task.DurationMillis = *duration
	}
	return task, nil
}

// decodeHistoricVariable 解析历史变量行。
// @param row 行
// @param arrays 同批次字节数据
// @return 历史变量
// @return 错误
func decodeHistoricVariable(row Row, arrays ...map[string][]byte) (api.HistoricVariableInstance, error) {
	id, err := stringValue(row, "ID_")
	if err != nil {
		return api.HistoricVariableInstance{}, err
	}
	name, err := stringValue(row, "NAME_")
	if err != nil {
		return api.HistoricVariableInstance{}, err
	}
	variableType, err := stringValue(row, "VAR_TYPE_")
	if err != nil {
		return api.HistoricVariableInstance{}, err
	}
	processInstanceID, err := stringValue(row, "PROC_INST_ID_")
	if err != nil {
		return api.HistoricVariableInstance{}, err
	}
	executionID, err := stringValue(row, "EXECUTION_ID_")
	if err != nil {
		return api.HistoricVariableInstance{}, err
	}
	taskID, err := stringValue(row, "TASK_ID_")
	if err != nil {
		return api.HistoricVariableInstance{}, err
	}
	longValue, err := optionalInt(row, "LONG_")
	if err != nil {
		return api.HistoricVariableInstance{}, err
	}
	doubleValue, err := optionalFloat(row, "DOUBLE_")
	if err != nil {
		return api.HistoricVariableInstance{}, err
	}
	textValue, err := optionalString(row, "TEXT_")
	if err != nil {
		return api.HistoricVariableInstance{}, err
	}
	createTime, err := timeValue(row, "CREATE_TIME_")
	if err != nil {
		return api.HistoricVariableInstance{}, err
	}
	updated, err := optionalTime(row, "LAST_UPDATED_TIME_")
	if err != nil {
		return api.HistoricVariableInstance{}, err
	}
	value, err := storedVariableValue(row, variableType, longValue, doubleValue, textValue, arrays)
	if err != nil {
		return api.HistoricVariableInstance{}, err
	}
	historic := api.HistoricVariableInstance{
		ID: id, Name: name, Type: variableType, ProcessInstanceID: processInstanceID,
		ExecutionID: executionID, TaskID: taskID, Value: value, CreateTime: createTime,
	}
	if updated != nil {
		historic.LastUpdatedTime = *updated
	}
	return historic, nil
}

// historicVariableValue 将历史变量列转换为值。
// @param variableType 类型
// @param longValue LONG_
// @param doubleValue DOUBLE_
// @param textValue TEXT_
// @return 值
// @return 错误
func historicVariableValue(variableType string, longValue *int64, doubleValue *float64, textValue *string) (any, error) {
	return VariableValue(variableType, longValue, doubleValue, textValue, nil)
}

// decodeHistoricComment 解析历史评论行。
// @param row 行
// @return 评论
// @return 错误
func decodeHistoricComment(row Row) (api.Comment, error) {
	id, err := stringValue(row, "ID_")
	if err != nil {
		return api.Comment{}, err
	}
	commentType, err := stringValue(row, "TYPE_")
	if err != nil {
		return api.Comment{}, err
	}
	commentTime, err := timeValue(row, "TIME_")
	if err != nil {
		return api.Comment{}, err
	}
	userID, err := stringValue(row, "USER_ID_")
	if err != nil {
		return api.Comment{}, err
	}
	taskID, err := stringValue(row, "TASK_ID_")
	if err != nil {
		return api.Comment{}, err
	}
	processInstanceID, err := stringValue(row, "PROC_INST_ID_")
	if err != nil {
		return api.Comment{}, err
	}
	message, err := stringValue(row, "MESSAGE_")
	if err != nil {
		return api.Comment{}, err
	}
	return api.Comment{ID: id, Type: commentType, Time: commentTime, UserID: userID, TaskID: taskID, ProcessInstanceID: processInstanceID, Message: message}, nil
}

// decodeHistoricAttachment 解析历史附件行。
// @param row 行
// @return 附件
// @return 错误
func decodeHistoricAttachment(row Row) (api.Attachment, error) {
	id, err := stringValue(row, "ID_")
	if err != nil {
		return api.Attachment{}, err
	}
	name, err := stringValue(row, "NAME_")
	if err != nil {
		return api.Attachment{}, err
	}
	description, err := stringValue(row, "DESCRIPTION_")
	if err != nil {
		return api.Attachment{}, err
	}
	attachmentType, err := stringValue(row, "TYPE_")
	if err != nil {
		return api.Attachment{}, err
	}
	taskID, err := stringValue(row, "TASK_ID_")
	if err != nil {
		return api.Attachment{}, err
	}
	processInstanceID, err := stringValue(row, "PROC_INST_ID_")
	if err != nil {
		return api.Attachment{}, err
	}
	url, err := stringValue(row, "URL_")
	if err != nil {
		return api.Attachment{}, err
	}
	attachmentTime, err := timeValue(row, "TIME_")
	if err != nil {
		return api.Attachment{}, err
	}
	return api.Attachment{ID: id, Name: name, Description: description, Type: attachmentType, TaskID: taskID, ProcessInstanceID: processInstanceID, URL: url, Time: attachmentTime}, nil
}

// decodeHistoricIdentityLink 解析历史身份链接。
// @param row 行
// @return 链接
// @return 错误
func decodeHistoricIdentityLink(row Row) (api.IdentityLink, error) {
	id, err := stringValue(row, "ID_")
	if err != nil {
		return api.IdentityLink{}, err
	}
	groupID, err := stringValue(row, "GROUP_ID_")
	if err != nil {
		return api.IdentityLink{}, err
	}
	linkType, err := stringValue(row, "TYPE_")
	if err != nil {
		return api.IdentityLink{}, err
	}
	userID, err := stringValue(row, "USER_ID_")
	if err != nil {
		return api.IdentityLink{}, err
	}
	taskID, err := stringValue(row, "TASK_ID_")
	if err != nil {
		return api.IdentityLink{}, err
	}
	created, err := timeValue(row, "CREATE_TIME_")
	if err != nil {
		return api.IdentityLink{}, err
	}
	processInstanceID, err := stringValue(row, "PROC_INST_ID_")
	if err != nil {
		return api.IdentityLink{}, err
	}
	return api.IdentityLink{ID: id, GroupID: groupID, Type: linkType, UserID: userID, TaskID: taskID, ProcessInstanceID: processInstanceID, CreateTime: created}, nil
}

// decodeHistoricDetail 解析历史变量明细。
// @param row 行
// @return 明细
// @return 错误
// @param arrays 同批次字节数据
func decodeHistoricDetail(row Row, arrays ...map[string][]byte) (api.HistoricDetail, error) {
	id, err := stringValue(row, "ID_")
	if err != nil {
		return api.HistoricDetail{}, err
	}
	detailType, err := stringValue(row, "TYPE_")
	if err != nil {
		return api.HistoricDetail{}, err
	}
	processInstanceID, err := stringValue(row, "PROC_INST_ID_")
	if err != nil {
		return api.HistoricDetail{}, err
	}
	executionID, err := stringValue(row, "EXECUTION_ID_")
	if err != nil {
		return api.HistoricDetail{}, err
	}
	taskID, err := stringValue(row, "TASK_ID_")
	if err != nil {
		return api.HistoricDetail{}, err
	}
	activityID, err := stringValue(row, "ACT_INST_ID_")
	if err != nil {
		return api.HistoricDetail{}, err
	}
	name, err := stringValue(row, "NAME_")
	if err != nil {
		return api.HistoricDetail{}, err
	}
	variableType, err := stringValue(row, "VAR_TYPE_")
	if err != nil {
		return api.HistoricDetail{}, err
	}
	detailTime, err := timeValue(row, "TIME_")
	if err != nil {
		return api.HistoricDetail{}, err
	}
	longValue, err := optionalInt(row, "LONG_")
	if err != nil {
		return api.HistoricDetail{}, err
	}
	doubleValue, err := optionalFloat(row, "DOUBLE_")
	if err != nil {
		return api.HistoricDetail{}, err
	}
	textValue, err := optionalString(row, "TEXT_")
	if err != nil {
		return api.HistoricDetail{}, err
	}
	value, err := storedVariableValue(row, variableType, longValue, doubleValue, textValue, arrays)
	if err != nil {
		return api.HistoricDetail{}, err
	}
	return api.HistoricDetail{
		ID: id, Type: detailType, ProcessInstanceID: processInstanceID, ExecutionID: executionID,
		TaskID: taskID, ActivityID: activityID, Name: name, VariableType: variableType,
		Value: value, Time: detailTime,
	}, nil
}

// decodeHistoricTaskLog 解析任务日志。
// @param row 行
// @return 日志
// @return 错误
func decodeHistoricTaskLog(row Row) (api.HistoricTaskLog, error) {
	id, err := intValue(row, "ID_")
	if err != nil {
		return api.HistoricTaskLog{}, err
	}
	logType, err := stringValue(row, "TYPE_")
	if err != nil {
		return api.HistoricTaskLog{}, err
	}
	taskID, err := stringValue(row, "TASK_ID_")
	if err != nil {
		return api.HistoricTaskLog{}, err
	}
	logTime, err := timeValue(row, "TIME_STAMP_")
	if err != nil {
		return api.HistoricTaskLog{}, err
	}
	userID, err := stringValue(row, "USER_ID_")
	if err != nil {
		return api.HistoricTaskLog{}, err
	}
	data, err := stringValue(row, "DATA_")
	if err != nil {
		return api.HistoricTaskLog{}, err
	}
	executionID, err := stringValue(row, "EXECUTION_ID_")
	if err != nil {
		return api.HistoricTaskLog{}, err
	}
	processInstanceID, err := stringValue(row, "PROC_INST_ID_")
	if err != nil {
		return api.HistoricTaskLog{}, err
	}
	definitionID, err := stringValue(row, "PROC_DEF_ID_")
	if err != nil {
		return api.HistoricTaskLog{}, err
	}
	tenant, err := stringValue(row, "TENANT_ID_")
	if err != nil {
		return api.HistoricTaskLog{}, err
	}
	return api.HistoricTaskLog{
		ID: int64(id), Type: logType, TaskID: taskID, Time: logTime, UserID: userID,
		Data: data, ExecutionID: executionID, ProcessInstanceID: processInstanceID,
		ProcessDefinitionID: definitionID, TenantID: tenant,
	}, nil
}
