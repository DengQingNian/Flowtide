package flowable

import (
	"encoding/json"
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
	ParentTaskID      string     // 父任务 ID
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
type RuntimeJob struct {
	ID                string     // 作业 ID
	Type              string     // 作业类型
	ExecutionID       string     // 执行 ID
	ProcessInstanceID string     // 实例 ID
	DefinitionID      string     // 定义 ID
	ActivityID        string     // 活动 ID
	Retries           int        // 重试次数
	Exception         string     // 异常
	DueDate           *time.Time // 到期时间
	CreatedAt         time.Time  // 创建时间
	TenantID          string     // 租户
	RepeatRemaining   int        // 剩余重复次数
	RepeatInfinite    bool       // 是否无限重复
	IntervalMillis    int64      // 重复间隔
	DeadLetter        bool       // 是否死信
}

// RuntimeSubscription 是事件订阅行。
type RuntimeSubscription struct {
	ID                 string    // 订阅 ID
	EventType          string    // 事件类型
	EventName          string    // 事件名
	Condition          string    // 条件表达式（条件事件）
	ExecutionID        string    // 执行 ID
	ProcessInstanceID  string    // 实例 ID
	ActivityID         string    // 活动 ID
	CreatedAt          time.Time // 创建时间
	DefinitionID       string    // 定义 ID
	ScopeType          string    // 作用域类型
	ScopeDefinitionID  string    // 作用域定义 ID
	ScopeDefinitionKey string    // 作用域定义 Key
	ScopeID            string    // 作用域执行 ID
	SubScopeID         string    // 子作用域 ID
	TenantID           string    // 租户
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

// RuntimeIdentityLink 是运行时身份链接行。
type RuntimeIdentityLink struct {
	ID                  string // 链接 ID
	GroupID             string // 组 ID
	Type                string // 类型
	UserID              string // 用户 ID
	TaskID              string // 任务 ID
	ProcessInstanceID   string // 实例 ID
	ProcessDefinitionID string // 定义 ID
}

// RuntimeSnapshot 是运行时行恢复结果。
type RuntimeSnapshot struct {
	Executions    []RuntimeExecution    // 执行
	Tasks         []RuntimeTask         // 任务
	Variables     []RuntimeVariable     // 变量
	Jobs          []RuntimeJob          // 作业
	DeadLetters   []RuntimeJob          // 死信作业
	Subscriptions []RuntimeSubscription // 订阅
	Links         []RuntimeIdentityLink // 身份链接
}

// DecodeRuntimeRows 将运行时行恢复为快照。
// @param rows 行列表
// @return 运行时快照
// @return 解析错误
func DecodeRuntimeRows(rows []Row) (RuntimeSnapshot, error) {
	snapshot := RuntimeSnapshot{
		Executions:    make([]RuntimeExecution, 0),
		Tasks:         make([]RuntimeTask, 0),
		Variables:     make([]RuntimeVariable, 0),
		Jobs:          make([]RuntimeJob, 0),
		DeadLetters:   make([]RuntimeJob, 0),
		Subscriptions: make([]RuntimeSubscription, 0),
		Links:         make([]RuntimeIdentityLink, 0),
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
		case "ACT_RU_JOB":
			job, err := decodeJob(row)
			if err != nil {
				return RuntimeSnapshot{}, err
			}
			snapshot.Jobs = append(snapshot.Jobs, job)
		case "ACT_RU_DEADLETTER_JOB":
			job, err := decodeJob(row)
			if err != nil {
				return RuntimeSnapshot{}, err
			}
			job.DeadLetter = true
			snapshot.DeadLetters = append(snapshot.DeadLetters, job)
		case "ACT_RU_IDENTITYLINK":
			link, err := decodeRuntimeLink(row)
			if err != nil {
				return RuntimeSnapshot{}, err
			}
			snapshot.Links = append(snapshot.Links, link)
		case "ACT_RU_EVENT_SUBSCR":
			subscription, err := decodeSubscription(row)
			if err != nil {
				return RuntimeSnapshot{}, err
			}
			snapshot.Subscriptions = append(snapshot.Subscriptions, subscription)
		default:
			return RuntimeSnapshot{}, api.NewError("ROW-0020", "不支持的表: "+row.Table, api.ErrBadRequest)
		}
	}
	sort.Slice(snapshot.Executions, func(i, j int) bool { return snapshot.Executions[i].ID < snapshot.Executions[j].ID })
	sort.Slice(snapshot.Tasks, func(i, j int) bool { return snapshot.Tasks[i].ID < snapshot.Tasks[j].ID })
	sort.Slice(snapshot.Variables, func(i, j int) bool { return snapshot.Variables[i].ID < snapshot.Variables[j].ID })
	sort.Slice(snapshot.Jobs, func(i, j int) bool { return snapshot.Jobs[i].ID < snapshot.Jobs[j].ID })
	sort.Slice(snapshot.DeadLetters, func(i, j int) bool { return snapshot.DeadLetters[i].ID < snapshot.DeadLetters[j].ID })
	sort.Slice(snapshot.Subscriptions, func(i, j int) bool { return snapshot.Subscriptions[i].ID < snapshot.Subscriptions[j].ID })
	sort.Slice(snapshot.Links, func(i, j int) bool { return snapshot.Links[i].ID < snapshot.Links[j].ID })
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

// decodeRuntimeLink 解析运行时身份链接行。
// @param row 行
// @return 链接
// @return 错误
func decodeRuntimeLink(row Row) (RuntimeIdentityLink, error) {
	text := func(column string) (string, error) {
		value, err := optionalString(row, column)
		if err != nil || value == nil {
			return "", err
		}
		return *value, nil
	}
	id, err := stringValue(row, "ID_")
	if err != nil {
		return RuntimeIdentityLink{}, err
	}
	groupID, err := text("GROUP_ID_")
	if err != nil {
		return RuntimeIdentityLink{}, err
	}
	linkType, err := stringValue(row, "TYPE_")
	if err != nil {
		return RuntimeIdentityLink{}, err
	}
	userID, err := text("USER_ID_")
	if err != nil {
		return RuntimeIdentityLink{}, err
	}
	taskID, err := text("TASK_ID_")
	if err != nil {
		return RuntimeIdentityLink{}, err
	}
	processInstanceID, err := text("PROC_INST_ID_")
	if err != nil {
		return RuntimeIdentityLink{}, err
	}
	processDefinitionID, err := text("PROC_DEF_ID_")
	if err != nil {
		return RuntimeIdentityLink{}, err
	}
	return RuntimeIdentityLink{
		ID: id, GroupID: groupID, Type: linkType, UserID: userID, TaskID: taskID,
		ProcessInstanceID: processInstanceID, ProcessDefinitionID: processDefinitionID,
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
	parentTask, err := optionalString(row, "PARENT_TASK_ID_")
	if err != nil {
		return RuntimeTask{}, err
	}
	parentTaskID := ""
	if parentTask != nil {
		parentTaskID = *parentTask
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
		TaskDefinitionKey: taskKey, ParentTaskID: parentTaskID, Owner: owner, Assignee: assignee, Category: category,
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

// jobHandlerConfig 是作业配置。
type jobHandlerConfig struct {
	Remaining      int   // 剩余次数
	Infinite       bool  // 是否无限
	IntervalMillis int64 // 间隔毫秒
}

// decodeJob 解析作业行。
// @param row 行
// @return 作业
// @return 错误
func decodeJob(row Row) (RuntimeJob, error) {
	id, err := stringValue(row, "ID_")
	if err != nil {
		return RuntimeJob{}, err
	}
	jobType, err := stringValue(row, "TYPE_")
	if err != nil {
		return RuntimeJob{}, err
	}
	executionID, err := stringValue(row, "EXECUTION_ID_")
	if err != nil {
		return RuntimeJob{}, err
	}
	processInstanceID, err := stringValue(row, "PROCESS_INSTANCE_ID_")
	if err != nil {
		return RuntimeJob{}, err
	}
	definitionID, err := stringValue(row, "PROC_DEF_ID_")
	if err != nil {
		return RuntimeJob{}, err
	}
	activityID, err := stringValue(row, "ELEMENT_ID_")
	if err != nil {
		return RuntimeJob{}, err
	}
	tenant, err := stringValue(row, "TENANT_ID_")
	if err != nil {
		return RuntimeJob{}, err
	}
	retries, err := optionalInt(row, "RETRIES_")
	if err != nil {
		return RuntimeJob{}, err
	}
	exception, err := optionalString(row, "EXCEPTION_MSG_")
	if err != nil {
		return RuntimeJob{}, err
	}
	due, err := optionalTime(row, "DUEDATE_")
	if err != nil {
		return RuntimeJob{}, err
	}
	created, err := optionalTime(row, "CREATE_TIME_")
	if err != nil {
		return RuntimeJob{}, err
	}
	handlerType, err := optionalString(row, "HANDLER_TYPE_")
	if err != nil {
		return RuntimeJob{}, err
	}
	if handlerType != nil && *handlerType != "" {
		jobType = *handlerType
	}
	config := jobHandlerConfig{}
	if raw, err := optionalString(row, "HANDLER_CFG_"); err != nil {
		return RuntimeJob{}, err
	} else if raw != nil && *raw != "" {
		if err := json.Unmarshal([]byte(*raw), &config); err != nil {
			return RuntimeJob{}, api.NewError("ROW-0015", "作业配置解析失败", err)
		}
	}
	job := RuntimeJob{
		ID: id, Type: jobType, ExecutionID: executionID, ProcessInstanceID: processInstanceID,
		DefinitionID: definitionID, ActivityID: activityID, TenantID: tenant,
		RepeatRemaining: config.Remaining, RepeatInfinite: config.Infinite, IntervalMillis: config.IntervalMillis,
	}
	if retries != nil {
		job.Retries = int(*retries)
	}
	if exception != nil {
		job.Exception = *exception
	}
	job.DueDate = due
	if created != nil {
		job.CreatedAt = *created
	}
	return job, nil
}

// decodeSubscription 解析事件订阅行。
// @param row 行
// @return 订阅
// @return 错误
func decodeSubscription(row Row) (RuntimeSubscription, error) {
	id, err := stringValue(row, "ID_")
	if err != nil {
		return RuntimeSubscription{}, err
	}
	eventType, err := stringValue(row, "EVENT_TYPE_")
	if err != nil {
		return RuntimeSubscription{}, err
	}
	eventName, err := stringValue(row, "EVENT_NAME_")
	if err != nil {
		return RuntimeSubscription{}, err
	}
	executionID, err := stringValue(row, "EXECUTION_ID_")
	if err != nil {
		return RuntimeSubscription{}, err
	}
	processInstanceID, err := stringValue(row, "PROC_INST_ID_")
	if err != nil {
		return RuntimeSubscription{}, err
	}
	activityID, err := stringValue(row, "ACTIVITY_ID_")
	if err != nil {
		return RuntimeSubscription{}, err
	}
	created, err := timeValue(row, "CREATED_")
	if err != nil {
		return RuntimeSubscription{}, err
	}
	definitionID, err := stringValue(row, "PROC_DEF_ID_")
	if err != nil {
		return RuntimeSubscription{}, err
	}
	tenant, err := stringValue(row, "TENANT_ID_")
	if err != nil {
		return RuntimeSubscription{}, err
	}
	scopeType, err := stringValue(row, "SCOPE_TYPE_")
	if err != nil {
		return RuntimeSubscription{}, err
	}
	scopeDefinitionID, err := stringValue(row, "SCOPE_DEFINITION_ID_")
	if err != nil {
		return RuntimeSubscription{}, err
	}
	scopeDefinitionKey, err := stringValue(row, "SCOPE_DEFINITION_KEY_")
	if err != nil {
		return RuntimeSubscription{}, err
	}
	scopeID, err := stringValue(row, "SCOPE_ID_")
	if err != nil {
		return RuntimeSubscription{}, err
	}
	subScopeID, err := stringValue(row, "SUB_SCOPE_ID_")
	if err != nil {
		return RuntimeSubscription{}, err
	}
	configuration, err := optionalString(row, "CONFIGURATION_")
	if err != nil {
		return RuntimeSubscription{}, err
	}
	condition := ""
	if eventType == "condition" && configuration != nil {
		condition = *configuration
	}
	return RuntimeSubscription{
		ID: id, EventType: eventType, EventName: eventName, Condition: condition, ExecutionID: executionID,
		ProcessInstanceID: processInstanceID, ActivityID: activityID, CreatedAt: created,
		DefinitionID: definitionID, ScopeType: scopeType, ScopeDefinitionID: scopeDefinitionID,
		ScopeDefinitionKey: scopeDefinitionKey, ScopeID: scopeID, SubScopeID: subScopeID, TenantID: tenant,
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
