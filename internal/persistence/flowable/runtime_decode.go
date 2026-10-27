package flowable

import (
	"sort"
	"time"

	"github.com/flowable-go/flowable/api"
)

// RuntimeExecution 是运行时执行行。
type RuntimeExecution struct {
	ID                string    // 执行 ID
	ProcessInstanceID string    // 流程实例 ID
	DefinitionID      string    // 流程定义 ID
	ParentID          string    // 父执行 ID
	ActivityID        string    // 当前活动 ID
	BusinessKey       string    // 业务键
	TenantID          string    // 租户
	StartTime         time.Time // 开始时间
	Active            bool      // 是否活跃
	Concurrent        bool      // 是否并发
	Scope             bool      // 是否作用域
	EventScope        bool      // 是否事件作用域
	MIRoot            bool      // 是否多实例根
	Suspended         bool      // 是否挂起
}

// RuntimeTask 是运行时任务行。
type RuntimeTask struct {
	ID                string     // 任务 ID
	ExecutionID       string     // 执行 ID
	ProcessInstanceID string     // 流程实例 ID
	DefinitionID      string     // 流程定义 ID
	Name              string     // 名称
	Description       string     // 描述
	TaskDefinitionKey string     // 任务定义 Key
	Owner             string     // 所有者
	Assignee          string     // 处理人
	Category          string     // 分类
	FormKey           string     // 表单 Key
	TenantID          string     // 租户
	Priority          int        // 优先级
	CreateTime        time.Time  // 创建时间
	DueDate           *time.Time // 到期时间
	Suspended         bool       // 是否挂起
}

// RuntimeVariable 是运行时变量行。
type RuntimeVariable struct {
	ID                string   // 变量 ID
	Type              string   // 类型
	Name              string   // 名称
	ExecutionID       string   // 执行 ID
	ProcessInstanceID string   // 实例 ID
	TaskID            string   // 任务 ID
	Long              *int64   // 长整型
	Double            *float64 // 浮点
	Text              *string  // 文本
}

// RuntimeSnapshot 是运行时行恢复结果。
type RuntimeSnapshot struct {
	Executions []RuntimeExecution // 执行
	Tasks      []RuntimeTask      // 任务
	Variables  []RuntimeVariable  // 变量
}

// DecodeRuntimeRows 将运行时行恢复为快照。
// @param rows 行列表
// @return 运行时快照
// @return 解析错误
func DecodeRuntimeRows(rows []Row) (RuntimeSnapshot, error) {
	snapshot := RuntimeSnapshot{
		Executions: make([]RuntimeExecution, 0),
		Tasks:      make([]RuntimeTask, 0),
		Variables:  make([]RuntimeVariable, 0),
	}
	for _, row := range rows {
		switch row.Table {
		case "ACT_RU_EXECUTION":
			execution, err := decodeExecution(row)
			if err != nil {
				return RuntimeSnapshot{}, err
			}
			snapshot.Executions = append(snapshot.Executions, execution)
		case "ACT_RU_TASK":
			task, err := decodeTask(row)
			if err != nil {
				return RuntimeSnapshot{}, err
			}
			snapshot.Tasks = append(snapshot.Tasks, task)
		case "ACT_RU_VARIABLE":
			variable, err := decodeVariable(row)
			if err != nil {
				return RuntimeSnapshot{}, err
			}
			snapshot.Variables = append(snapshot.Variables, variable)
		default:
			return RuntimeSnapshot{}, api.NewError("ROW-0020", "不支持的表: "+row.Table, api.ErrBadRequest)
		}
	}
	sort.Slice(snapshot.Executions, func(i, j int) bool { return snapshot.Executions[i].ID < snapshot.Executions[j].ID })
	sort.Slice(snapshot.Tasks, func(i, j int) bool { return snapshot.Tasks[i].ID < snapshot.Tasks[j].ID })
	sort.Slice(snapshot.Variables, func(i, j int) bool { return snapshot.Variables[i].ID < snapshot.Variables[j].ID })
	return snapshot, nil
}

// decodeExecution 解析执行行。
// @param row 行
// @return 执行
// @return 错误
func decodeExecution(row Row) (RuntimeExecution, error) {
	id, err := stringValue(row, "ID_")
	if err != nil {
		return RuntimeExecution{}, err
	}
	processInstanceID, err := stringValue(row, "PROC_INST_ID_")
	if err != nil {
		return RuntimeExecution{}, err
	}
	definitionID, err := stringValue(row, "PROC_DEF_ID_")
	if err != nil {
		return RuntimeExecution{}, err
	}
	parentID, err := stringValue(row, "PARENT_ID_")
	if err != nil {
		return RuntimeExecution{}, err
	}
	activityID, err := stringValue(row, "ACT_ID_")
	if err != nil {
		return RuntimeExecution{}, err
	}
	businessKey, err := stringValue(row, "BUSINESS_KEY_")
	if err != nil {
		return RuntimeExecution{}, err
	}
	tenant, err := stringValue(row, "TENANT_ID_")
	if err != nil {
		return RuntimeExecution{}, err
	}
	startTime, err := timeValue(row, "START_TIME_")
	if err != nil {
		return RuntimeExecution{}, err
	}
	active, err := boolValue(row, "IS_ACTIVE_")
	if err != nil {
		return RuntimeExecution{}, err
	}
	concurrent, err := boolValue(row, "IS_CONCURRENT_")
	if err != nil {
		return RuntimeExecution{}, err
	}
	scope, err := boolValue(row, "IS_SCOPE_")
	if err != nil {
		return RuntimeExecution{}, err
	}
	eventScope, err := boolValue(row, "IS_EVENT_SCOPE_")
	if err != nil {
		return RuntimeExecution{}, err
	}
	miRoot, err := boolValue(row, "IS_MI_ROOT_")
	if err != nil {
		return RuntimeExecution{}, err
	}
	suspension, err := intValue(row, "SUSPENSION_STATE_")
	if err != nil {
		return RuntimeExecution{}, err
	}
	return RuntimeExecution{
		ID: id, ProcessInstanceID: processInstanceID, DefinitionID: definitionID,
		ParentID: parentID, ActivityID: activityID, BusinessKey: businessKey,
		TenantID: tenant, StartTime: startTime, Active: active, Concurrent: concurrent,
		Scope: scope, EventScope: eventScope, MIRoot: miRoot, Suspended: suspension == 2,
	}, nil
}

// decodeTask 解析任务行。
// @param row 行
// @return 任务
// @return 错误
func decodeTask(row Row) (RuntimeTask, error) {
	text := func(column string) (string, error) { return stringValue(row, column) }
	id, err := text("ID_")
	if err != nil {
		return RuntimeTask{}, err
	}
	executionID, err := text("EXECUTION_ID_")
	if err != nil {
		return RuntimeTask{}, err
	}
	processInstanceID, err := text("PROC_INST_ID_")
	if err != nil {
		return RuntimeTask{}, err
	}
	definitionID, err := text("PROC_DEF_ID_")
	if err != nil {
		return RuntimeTask{}, err
	}
	name, err := text("NAME_")
	if err != nil {
		return RuntimeTask{}, err
	}
	description, err := text("DESCRIPTION_")
	if err != nil {
		return RuntimeTask{}, err
	}
	taskKey, err := text("TASK_DEF_KEY_")
	if err != nil {
		return RuntimeTask{}, err
	}
	owner, err := text("OWNER_")
	if err != nil {
		return RuntimeTask{}, err
	}
	assignee, err := text("ASSIGNEE_")
	if err != nil {
		return RuntimeTask{}, err
	}
	category, err := text("CATEGORY_")
	if err != nil {
		return RuntimeTask{}, err
	}
	formKey, err := text("FORM_KEY_")
	if err != nil {
		return RuntimeTask{}, err
	}
	tenant, err := text("TENANT_ID_")
	if err != nil {
		return RuntimeTask{}, err
	}
	priority, err := intValue(row, "PRIORITY_")
	if err != nil {
		return RuntimeTask{}, err
	}
	createTime, err := timeValue(row, "CREATE_TIME_")
	if err != nil {
		return RuntimeTask{}, err
	}
	due, err := optionalTime(row, "DUE_DATE_")
	if err != nil {
		return RuntimeTask{}, err
	}
	suspension, err := intValue(row, "SUSPENSION_STATE_")
	if err != nil {
		return RuntimeTask{}, err
	}
	return RuntimeTask{
		ID: id, ExecutionID: executionID, ProcessInstanceID: processInstanceID,
		DefinitionID: definitionID, Name: name, Description: description,
		TaskDefinitionKey: taskKey, Owner: owner, Assignee: assignee, Category: category,
		FormKey: formKey, TenantID: tenant, Priority: priority, CreateTime: createTime,
		DueDate: due, Suspended: suspension == 2,
	}, nil
}

// decodeVariable 解析变量行。
// @param row 行
// @return 变量
// @return 错误
func decodeVariable(row Row) (RuntimeVariable, error) {
	id, err := stringValue(row, "ID_")
	if err != nil {
		return RuntimeVariable{}, err
	}
	variableType, err := stringValue(row, "TYPE_")
	if err != nil {
		return RuntimeVariable{}, err
	}
	name, err := stringValue(row, "NAME_")
	if err != nil {
		return RuntimeVariable{}, err
	}
	executionID, err := stringValue(row, "EXECUTION_ID_")
	if err != nil {
		return RuntimeVariable{}, err
	}
	processInstanceID, err := stringValue(row, "PROC_INST_ID_")
	if err != nil {
		return RuntimeVariable{}, err
	}
	taskID, err := stringValue(row, "TASK_ID_")
	if err != nil {
		return RuntimeVariable{}, err
	}
	longValue, err := optionalInt(row, "LONG_")
	if err != nil {
		return RuntimeVariable{}, err
	}
	doubleValue, err := optionalFloat(row, "DOUBLE_")
	if err != nil {
		return RuntimeVariable{}, err
	}
	textValue, err := optionalString(row, "TEXT_")
	if err != nil {
		return RuntimeVariable{}, err
	}
	return RuntimeVariable{
		ID: id, Type: variableType, Name: name, ExecutionID: executionID,
		ProcessInstanceID: processInstanceID, TaskID: taskID,
		Long: longValue, Double: doubleValue, Text: textValue,
	}, nil
}

// boolValue 读取布尔列。
// @param row 行
// @param column 列名
// @return 布尔
// @return 错误
func boolValue(row Row, column string) (bool, error) {
	value, ok := row.Values[column]
	if !ok {
		return false, api.NewError("ROW-0013", "缺少列: "+column, api.ErrBadRequest)
	}
	switch typed := value.(type) {
	case bool:
		return typed, nil
	case int:
		return typed != 0, nil
	case int64:
		return typed != 0, nil
	case float64:
		return typed != 0, nil
	default:
		return false, api.NewError("ROW-0014", "列不是布尔值: "+column, api.ErrBadRequest)
	}
}

// optionalString 读取可空字符串。
// @param row 行
// @param column 列名
// @return 字符串指针
// @return 错误
func optionalString(row Row, column string) (*string, error) {
	value, ok := row.Values[column]
	if !ok || value == nil {
		return nil, nil
	}
	text, ok := value.(string)
	if !ok {
		return nil, api.NewError("ROW-0014", "列不是字符串: "+column, api.ErrBadRequest)
	}
	return &text, nil
}

// optionalInt 读取可空整数。
// @param row 行
// @param column 列名
// @return 整数指针
// @return 错误
func optionalInt(row Row, column string) (*int64, error) {
	value, ok := row.Values[column]
	if !ok || value == nil {
		return nil, nil
	}
	switch typed := value.(type) {
	case int:
		converted := int64(typed)
		return &converted, nil
	case int64:
		return &typed, nil
	case float64:
		converted := int64(typed)
		return &converted, nil
	default:
		return nil, api.NewError("ROW-0014", "列不是整数: "+column, api.ErrBadRequest)
	}
}

// optionalFloat 读取可空浮点。
// @param row 行
// @param column 列名
// @return 浮点指针
// @return 错误
func optionalFloat(row Row, column string) (*float64, error) {
	value, ok := row.Values[column]
	if !ok || value == nil {
		return nil, nil
	}
	switch typed := value.(type) {
	case float64:
		return &typed, nil
	case int64:
		converted := float64(typed)
		return &converted, nil
	case int:
		converted := float64(typed)
		return &converted, nil
	default:
		return nil, api.NewError("ROW-0014", "列不是浮点数: "+column, api.ErrBadRequest)
	}
}

// optionalTime 读取可空时间。
// @param row 行
// @param column 列名
// @return 时间指针
// @return 错误
func optionalTime(row Row, column string) (*time.Time, error) {
	value, ok := row.Values[column]
	if !ok || value == nil {
		return nil, nil
	}
	switch typed := value.(type) {
	case time.Time:
		return &typed, nil
	case string:
		parsed, err := time.Parse(time.RFC3339, typed)
		if err != nil {
			return nil, api.NewError("ROW-0014", "时间格式错误: "+column, err)
		}
		return &parsed, nil
	default:
		return nil, api.NewError("ROW-0014", "列不是时间: "+column, api.ErrBadRequest)
	}
}
