package flowable

import (
	"testing"
	"time"
)

// historicProcessRow 构造历史流程行。
// @param id 实例 ID
// @return 行
// @param t 测试上下文
func historicProcessRow(t *testing.T, id string) Row {
	t.Helper()
	row, _ := NewRow("ACT_HI_PROCINST")
	_ = row.Set("ID_", id)
	_ = row.Set("PROC_INST_ID_", id)
	_ = row.Set("BUSINESS_KEY_", "bk")
	_ = row.Set("PROC_DEF_ID_", "d1")
	_ = row.Set("START_TIME_", time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	_ = row.Set("END_TIME_", time.Date(2026, 10, 28, 1, 0, 0, 0, time.UTC))
	_ = row.Set("DELETE_REASON_", "done")
	_ = row.Set("TENANT_ID_", "t1")
	_ = row.Set("NAME_", "Process")
	return *row
}

// historicActivityRow 构造历史活动行。
// @param id 记录 ID
// @return 行
// @param t 测试上下文
func historicActivityRow(t *testing.T, id string) Row {
	t.Helper()
	row, _ := NewRow("ACT_HI_ACTINST")
	_ = row.Set("ID_", id)
	_ = row.Set("PROC_INST_ID_", "i1")
	_ = row.Set("ACT_ID_", "task")
	_ = row.Set("ACT_NAME_", "Task")
	_ = row.Set("ACT_TYPE_", "userTask")
	_ = row.Set("START_TIME_", time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	_ = row.Set("END_TIME_", time.Date(2026, 10, 28, 1, 0, 0, 0, time.UTC))
	_ = row.Set("DURATION_", int64(1000))
	return *row
}

// historicTaskRow 构造历史任务行。
// @param id 任务 ID
// @return 行
// @param t 测试上下文
func historicTaskRow(t *testing.T, id string) Row {
	t.Helper()
	row, _ := NewRow("ACT_HI_TASKINST")
	_ = row.Set("ID_", id)
	_ = row.Set("NAME_", "Task")
	_ = row.Set("PROC_INST_ID_", "i1")
	_ = row.Set("TASK_DEF_KEY_", "task")
	_ = row.Set("ASSIGNEE_", "alice")
	_ = row.Set("START_TIME_", time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	_ = row.Set("END_TIME_", time.Date(2026, 10, 28, 1, 0, 0, 0, time.UTC))
	_ = row.Set("DURATION_", int64(1000))
	return *row
}

// historicVariableRow 构造历史变量行。
// @param id 变量 ID
// @param kind 类型
// @return 行
// @param t 测试上下文
func historicVariableRow(t *testing.T, id, kind string) Row {
	t.Helper()
	row, _ := NewRow("ACT_HI_VARINST")
	_ = row.Set("ID_", id)
	_ = row.Set("NAME_", "v")
	_ = row.Set("VAR_TYPE_", kind)
	_ = row.Set("PROC_INST_ID_", "i1")
	_ = row.Set("EXECUTION_ID_", "e1")
	_ = row.Set("TASK_ID_", "")
	_ = row.Set("LONG_", int64(1))
	_ = row.Set("DOUBLE_", 1.5)
	_ = row.Set("TEXT_", "text")
	_ = row.Set("CREATE_TIME_", time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	_ = row.Set("LAST_UPDATED_TIME_", time.Date(2026, 10, 28, 1, 0, 0, 0, time.UTC))
	return *row
}

// TestDecodeHistoricRows 验证历史行恢复。
// @param t 测试上下文
func TestDecodeHistoricRows(t *testing.T) {
	rows := []Row{
		historicProcessRow(t, "i1"),
		historicActivityRow(t, "a1"),
		historicTaskRow(t, "t1"),
		historicVariableRow(t, "v1", "string"),
	}
	snapshot, err := DecodeHistoricRows(rows)
	if err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if len(snapshot.Processes) != 1 || !snapshot.Processes[0].Ended || snapshot.Processes[0].BusinessKey != "bk" {
		t.Fatalf("历史流程错误: %+v", snapshot.Processes)
	}
	if len(snapshot.Activities) != 1 || snapshot.Activities[0].DurationMillis != 1000 {
		t.Fatalf("历史活动错误: %+v", snapshot.Activities)
	}
	if len(snapshot.Tasks) != 1 || snapshot.Tasks[0].Assignee != "alice" {
		t.Fatalf("历史任务错误: %+v", snapshot.Tasks)
	}
	if len(snapshot.Variables) != 1 || snapshot.Variables[0].Value != "text" {
		t.Fatalf("历史变量错误: %+v", snapshot.Variables)
	}
}

// TestHistoricVariableTypes 验证历史变量类型。
// @param t 测试上下文
func TestHistoricVariableTypes(t *testing.T) {
	if value, err := historicVariableValue("boolean", ptrInt(1), nil, nil); err != nil || value != true {
		t.Fatalf("布尔历史变量错误: %v", err)
	}
	if value, err := historicVariableValue("long", ptrInt(2), nil, nil); err != nil || value != int64(2) {
		t.Fatalf("整数历史变量错误: %v", err)
	}
	if value, err := historicVariableValue("double", nil, ptrFloat(1.5), nil); err != nil || value != 1.5 {
		t.Fatalf("浮点历史变量错误: %v", err)
	}
	if value, err := historicVariableValue("null", nil, nil, nil); err != nil || value != nil {
		t.Fatalf("空历史变量错误: %v", err)
	}
	text := `{"a":1}`
	if _, err := historicVariableValue("json", nil, nil, &text); err != nil {
		t.Fatalf("JSON 历史变量错误: %v", err)
	}
	bad := "{"
	if _, err := historicVariableValue("json", nil, nil, &bad); err == nil {
		t.Fatalf("非法 JSON 历史变量应失败")
	}
	unknown := "x"
	if _, err := historicVariableValue("unknown", nil, nil, &unknown); err == nil {
		t.Fatalf("未知历史类型回退错误")
	}
}

// TestDecodeHistoricRows_Errors 验证历史行错误。
// @param t 测试上下文
func TestDecodeHistoricRows_Errors(t *testing.T) {
	cases := [][]Row{
		{{Table: "BAD"}},
		{func() Row { row := historicProcessRow(t, "i1"); delete(row.Values, "START_TIME_"); return row }()},
		{func() Row { row := historicProcessRow(t, "i1"); row.Values["END_TIME_"] = 1; return row }()},
		{func() Row { row := historicActivityRow(t, "a1"); delete(row.Values, "ACT_ID_"); return row }()},
		{func() Row { row := historicActivityRow(t, "a1"); row.Values["DURATION_"] = "x"; return row }()},
		{func() Row { row := historicTaskRow(t, "t1"); delete(row.Values, "NAME_"); return row }()},
		{func() Row { row := historicVariableRow(t, "v1", "string"); delete(row.Values, "VAR_TYPE_"); return row }()},
		{func() Row { row := historicVariableRow(t, "v1", "string"); row.Values["LONG_"] = "x"; return row }()},
		{func() Row { row := historicVariableRow(t, "v1", "string"); row.Values["DOUBLE_"] = "x"; return row }()},
	}
	for _, rows := range cases {
		if _, err := DecodeHistoricRows(rows); err == nil {
			t.Fatalf("期望历史解析失败: %+v", rows)
		}
	}
	processColumns := []string{"PROC_INST_ID_", "BUSINESS_KEY_", "PROC_DEF_ID_", "START_TIME_", "DELETE_REASON_", "TENANT_ID_", "NAME_"}
	for _, column := range processColumns {
		row := historicProcessRow(t, "i1")
		delete(row.Values, column)
		if _, err := decodeHistoricProcess(row); err == nil {
			t.Fatalf("历史流程缺列 %s 应失败", column)
		}
	}
	activityColumns := []string{"PROC_INST_ID_", "ACT_ID_", "ACT_NAME_", "ACT_TYPE_", "START_TIME_"}
	for _, column := range activityColumns {
		row := historicActivityRow(t, "a1")
		delete(row.Values, column)
		if _, err := decodeHistoricActivity(row); err == nil {
			t.Fatalf("历史活动缺列 %s 应失败", column)
		}
	}
	taskColumns := []string{"NAME_", "PROC_INST_ID_", "TASK_DEF_KEY_", "ASSIGNEE_", "START_TIME_"}
	for _, column := range taskColumns {
		row := historicTaskRow(t, "t1")
		delete(row.Values, column)
		if _, err := decodeHistoricTask(row); err == nil {
			t.Fatalf("历史任务缺列 %s 应失败", column)
		}
	}
	variableColumns := []string{"NAME_", "VAR_TYPE_", "PROC_INST_ID_", "EXECUTION_ID_", "TASK_ID_", "CREATE_TIME_"}
	for _, column := range variableColumns {
		row := historicVariableRow(t, "v1", "string")
		delete(row.Values, column)
		if _, err := decodeHistoricVariable(row); err == nil {
			t.Fatalf("历史变量缺列 %s 应失败", column)
		}
	}
	if value, _ := historicVariableValue("string", nil, nil, nil); value != "" {
		t.Fatalf("空字符串回退错误")
	}
	if value, _ := historicVariableValue("boolean", nil, nil, nil); value != false {
		t.Fatalf("空布尔回退错误")
	}
	if value, _ := historicVariableValue("long", nil, nil, nil); value != int64(0) {
		t.Fatalf("空整数回退错误")
	}
	if value, _ := historicVariableValue("double", nil, nil, nil); value != 0.0 {
		t.Fatalf("空浮点回退错误")
	}
	if _, err := historicVariableValue("unknown", nil, nil, nil); err == nil {
		t.Fatalf("未知类型空值回退错误")
	}
}

// TestDecodeHistoricJavaColumns 验证 Java 任务、活动与评论列在行解码时无损保留。
// @param t 测试上下文
// @return 无
func TestDecodeHistoricJavaColumns(t *testing.T) {
	started := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	processRow := historicProcessRow(t, "p1")
	for column, value := range map[string]any{
		"REV_": int64(2), "DURATION_": int64(120000), "START_USER_ID_": "alice",
		"START_ACT_ID_": "start", "END_ACT_ID_": "end", "BUSINESS_STATUS_": "done",
		"CLAIMED_BY_": "bob", "END_USER_ID_": "carol", "STATE_": "completed",
		"DUE_DATE_": started, "CLAIM_TIME_": started,
	} {
		processRow.Values[column] = value
	}
	process, err := decodeHistoricProcess(processRow)
	if err != nil {
		t.Fatal(err)
	}
	if process.Revision != 2 || process.DurationMillis != 120000 || process.StartUserID != "alice" || process.StartActivityID != "start" || process.EndActivityID != "end" || process.BusinessStatus != "done" || process.ClaimedBy != "bob" || process.EndUserID != "carol" || process.State != "completed" || process.DueDate == nil || process.ClaimTime == nil {
		t.Fatalf("历史流程 Java 列丢失：%+v", process)
	}
	activityRow := historicActivityRow(t, "a1")
	for column, value := range map[string]any{
		"REV_": int64(2), "PROC_DEF_ID_": "d1", "EXECUTION_ID_": "e1", "TASK_ID_": "t1",
		"CALL_PROC_INST_ID_": "call", "ASSIGNEE_": "alice", "COMPLETED_BY_": "bob",
		"TRANSACTION_ORDER_": int64(3), "DELETE_REASON_": "done", "TENANT_ID_": "tenant-a",
	} {
		activityRow.Values[column] = value
	}
	activity, err := decodeHistoricActivity(activityRow)
	if err != nil {
		t.Fatal(err)
	}
	if activity.Revision != 2 || activity.ProcessDefinitionID != "d1" || activity.ExecutionID != "e1" || activity.TaskID != "t1" || activity.CallProcessInstanceID != "call" || activity.Assignee != "alice" || activity.CompletedBy != "bob" || activity.TransactionOrder != 3 || activity.DeleteReason != "done" || activity.TenantID != "tenant-a" {
		t.Fatalf("历史活动 Java 列丢失：%+v", activity)
	}
	taskRow := historicTaskRow(t, "t1")
	for column, value := range map[string]any{
		"REV_": int64(4), "PROC_DEF_ID_": "d1", "TASK_DEF_ID_": "td1", "EXECUTION_ID_": "e1",
		"STATE_": "claimed", "DESCRIPTION_": "说明", "OWNER_": "owner", "FORM_KEY_": "form",
		"CATEGORY_": "audit", "TENANT_ID_": "tenant-a", "DELETE_REASON_": "approved",
		"IN_PROGRESS_STARTED_BY_": "alice", "CLAIMED_BY_": "alice", "SUSPENDED_BY_": "bob",
		"COMPLETED_BY_": "carol", "PRIORITY_": int64(60), "IN_PROGRESS_TIME_": started,
		"CLAIM_TIME_": started, "SUSPENDED_TIME_": started, "IN_PROGRESS_DUE_DATE_": started,
		"DUE_DATE_": started, "LAST_UPDATED_TIME_": started,
	} {
		taskRow.Values[column] = value
	}
	task, err := decodeHistoricTask(taskRow)
	if err != nil {
		t.Fatal(err)
	}
	if task.Revision != 4 || task.ProcessDefinitionID != "d1" || task.TaskDefinitionID != "td1" || task.ExecutionID != "e1" || task.State != "claimed" || task.Description != "说明" || task.Owner != "owner" || task.FormKey != "form" || task.Category != "audit" || task.TenantID != "tenant-a" || task.DeleteReason != "approved" || task.InProgressStartedBy != "alice" || task.ClaimedBy != "alice" || task.SuspendedBy != "bob" || task.CompletedBy != "carol" || task.Priority != 60 || task.InProgressTime == nil || task.ClaimTime == nil || task.SuspendedTime == nil || task.InProgressDueDate == nil || task.DueDate == nil || task.LastUpdatedTime == nil {
		t.Fatalf("历史任务 Java 列丢失：%+v", task)
	}
	commentRow := historicCommentRow(t, "c1")
	commentRow.Values["TYPE_"] = "event"
	commentRow.Values["ACTION_"] = "AddUserLink"
	commentRow.Values["FULL_MSG_"] = "完整内容"
	comment, err := decodeHistoricComment(commentRow)
	if err != nil || comment.Action != "AddUserLink" || comment.FullMessage != "完整内容" {
		t.Fatalf("历史评论 Java 列丢失：%+v %v", comment, err)
	}
}

// TestVariableValueExactIntegerText 验证 Java 大整数使用 TEXT_ 精确文本而非失真的 LONG_。
// @param t 测试上下文
// @return 无
func TestVariableValueExactIntegerText(t *testing.T) {
	exact := "9007199254740993"
	value, err := VariableValue("long", ptrInt(9007199254740992), nil, &exact, nil)
	if err != nil || value != int64(9007199254740993) {
		t.Fatalf("大整数未按文本精确恢复：%T %v %v", value, value, err)
	}
	invalid := "not-a-number"
	if _, err := VariableValue("integer", ptrInt(1), nil, &invalid, nil); err == nil {
		t.Fatal("非法整数文本未拒绝")
	}
}

// ptrInt 返回整数指针。
// @param value 值
// @return 指针
func ptrInt(value int64) *int64 { return &value }

// ptrFloat 返回浮点指针。
// @param value 值
// @return 指针
func ptrFloat(value float64) *float64 { return &value }

// historicCommentRow 构造历史评论行。
// @param id 评论 ID
// @return 行
// @param t 测试上下文
func historicCommentRow(t *testing.T, id string) Row {
	t.Helper()
	row, _ := NewRow("ACT_HI_COMMENT")
	_ = row.Set("ID_", id)
	_ = row.Set("TYPE_", "comment")
	_ = row.Set("TIME_", time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	_ = row.Set("USER_ID_", "alice")
	_ = row.Set("TASK_ID_", "t1")
	_ = row.Set("PROC_INST_ID_", "i1")
	_ = row.Set("MESSAGE_", "ok")
	return *row
}

// historicAttachmentRow 构造历史附件行。
// @param id 附件 ID
// @return 行
// @param t 测试上下文
func historicAttachmentRow(t *testing.T, id string) Row {
	t.Helper()
	row, _ := NewRow("ACT_HI_ATTACHMENT")
	_ = row.Set("ID_", id)
	_ = row.Set("NAME_", "doc")
	_ = row.Set("DESCRIPTION_", "desc")
	_ = row.Set("TYPE_", "url")
	_ = row.Set("TASK_ID_", "t1")
	_ = row.Set("PROC_INST_ID_", "i1")
	_ = row.Set("URL_", "http://example.com")
	_ = row.Set("TIME_", time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	return *row
}

// TestDecodeHistoricCommentAttachment 验证评论/附件恢复。
// @param t 测试上下文
func TestDecodeHistoricCommentAttachment(t *testing.T) {
	snapshot, err := DecodeHistoricRows([]Row{historicCommentRow(t, "c1"), historicAttachmentRow(t, "a1")})
	if err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if len(snapshot.Comments) != 1 || snapshot.Comments[0].Message != "ok" {
		t.Fatalf("评论恢复错误: %+v", snapshot.Comments)
	}
	if len(snapshot.Attachments) != 1 || snapshot.Attachments[0].Name != "doc" {
		t.Fatalf("附件恢复错误: %+v", snapshot.Attachments)
	}
}

// TestDecodeHistoricCommentAttachmentErrors 验证评论/附件错误。
// @param t 测试上下文
func TestDecodeHistoricCommentAttachmentErrors(t *testing.T) {
	commentColumns := []string{"TYPE_", "TIME_", "USER_ID_", "TASK_ID_", "PROC_INST_ID_", "MESSAGE_"}
	for _, column := range commentColumns {
		row := historicCommentRow(t, "c1")
		delete(row.Values, column)
		if _, err := decodeHistoricComment(row); err == nil {
			t.Fatalf("评论缺列 %s 应失败", column)
		}
	}
	attachmentColumns := []string{"NAME_", "DESCRIPTION_", "TYPE_", "TASK_ID_", "PROC_INST_ID_", "URL_", "TIME_"}
	for _, column := range attachmentColumns {
		row := historicAttachmentRow(t, "a1")
		delete(row.Values, column)
		if _, err := decodeHistoricAttachment(row); err == nil {
			t.Fatalf("附件缺列 %s 应失败", column)
		}
	}
}

// historicLinkRow 构造历史身份链接行。
// @param id 链接 ID
// @return 行
// @param t 测试上下文
func historicLinkRow(t *testing.T, id string) Row {
	t.Helper()
	row, _ := NewRow("ACT_HI_IDENTITYLINK")
	_ = row.Set("ID_", id)
	_ = row.Set("GROUP_ID_", "dev")
	_ = row.Set("TYPE_", "candidate")
	_ = row.Set("USER_ID_", "alice")
	_ = row.Set("TASK_ID_", "t1")
	_ = row.Set("CREATE_TIME_", time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	_ = row.Set("PROC_INST_ID_", "i1")
	return *row
}

// TestDecodeHistoricIdentityLink 验证历史身份链接恢复。
// @param t 测试上下文
func TestDecodeHistoricIdentityLink(t *testing.T) {
	snapshot, err := DecodeHistoricRows([]Row{historicLinkRow(t, "l1")})
	if err != nil || len(snapshot.Links) != 1 {
		t.Fatalf("链接恢复失败: %v %+v", err, snapshot.Links)
	}
	if snapshot.Links[0].UserID != "alice" || snapshot.Links[0].GroupID != "dev" {
		t.Fatalf("链接内容错误: %+v", snapshot.Links[0])
	}
	columns := []string{"GROUP_ID_", "TYPE_", "USER_ID_", "TASK_ID_", "CREATE_TIME_", "PROC_INST_ID_"}
	for _, column := range columns {
		row := historicLinkRow(t, "l1")
		delete(row.Values, column)
		if _, err := decodeHistoricIdentityLink(row); err == nil {
			t.Fatalf("链接缺列 %s 应失败", column)
		}
	}
}

// historicDetailRow 构造历史明细行。
// @param id 明细 ID
// @return 行
// @param t 测试上下文
func historicDetailRow(t *testing.T, id string) Row {
	t.Helper()
	row, _ := NewRow("ACT_HI_DETAIL")
	_ = row.Set("ID_", id)
	_ = row.Set("TYPE_", "variableUpdate")
	_ = row.Set("PROC_INST_ID_", "i1")
	_ = row.Set("EXECUTION_ID_", "e1")
	_ = row.Set("TASK_ID_", "t1")
	_ = row.Set("ACT_INST_ID_", "a1")
	_ = row.Set("NAME_", "v")
	_ = row.Set("VAR_TYPE_", "string")
	_ = row.Set("TIME_", time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	_ = row.Set("TEXT_", "x")
	return *row
}

// TestDecodeHistoricDetail 验证历史明细恢复。
// @param t 测试上下文
func TestDecodeHistoricDetail(t *testing.T) {
	snapshot, err := DecodeHistoricRows([]Row{historicDetailRow(t, "d1")})
	if err != nil || len(snapshot.Details) != 1 {
		t.Fatalf("明细恢复失败: %v %+v", err, snapshot.Details)
	}
	if snapshot.Details[0].Name != "v" || snapshot.Details[0].Value != "x" {
		t.Fatalf("明细内容错误: %+v", snapshot.Details[0])
	}
	columns := []string{"TYPE_", "PROC_INST_ID_", "EXECUTION_ID_", "TASK_ID_", "ACT_INST_ID_", "NAME_", "VAR_TYPE_", "TIME_"}
	for _, column := range columns {
		row := historicDetailRow(t, "d1")
		delete(row.Values, column)
		if _, err := decodeHistoricDetail(row); err == nil {
			t.Fatalf("明细缺列 %s 应失败", column)
		}
	}
}

// historicTaskLogRow 构造任务日志行。
// @param id 日志 ID
// @return 行
// @param t 测试上下文
func historicTaskLogRow(t *testing.T, id int64) Row {
	t.Helper()
	row, _ := NewRow("ACT_HI_TSK_LOG")
	_ = row.Set("ID_", id)
	_ = row.Set("TYPE_", "create")
	_ = row.Set("TASK_ID_", "t1")
	_ = row.Set("TIME_STAMP_", time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	_ = row.Set("USER_ID_", "alice")
	_ = row.Set("DATA_", "d")
	_ = row.Set("EXECUTION_ID_", "e1")
	_ = row.Set("PROC_INST_ID_", "i1")
	_ = row.Set("PROC_DEF_ID_", "p1")
	_ = row.Set("TENANT_ID_", "t1")
	return *row
}

// TestDecodeHistoricTaskLog 验证任务日志恢复。
// @param t 测试上下文
func TestDecodeHistoricTaskLog(t *testing.T) {
	snapshot, err := DecodeHistoricRows([]Row{historicTaskLogRow(t, 1)})
	if err != nil || len(snapshot.TaskLogs) != 1 {
		t.Fatalf("任务日志恢复失败: %v %+v", err, snapshot.TaskLogs)
	}
	if snapshot.TaskLogs[0].ID != 1 || snapshot.TaskLogs[0].UserID != "alice" {
		t.Fatalf("任务日志内容错误: %+v", snapshot.TaskLogs[0])
	}
	columns := []string{"TYPE_", "TASK_ID_", "TIME_STAMP_", "USER_ID_", "DATA_", "EXECUTION_ID_", "PROC_INST_ID_", "PROC_DEF_ID_", "TENANT_ID_"}
	for _, column := range columns {
		row := historicTaskLogRow(t, 1)
		delete(row.Values, column)
		if _, err := decodeHistoricTaskLog(row); err == nil {
			t.Fatalf("任务日志缺列 %s 应失败", column)
		}
	}
}
