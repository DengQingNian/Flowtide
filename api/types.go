package api

import "time"

// Variable 表示一个流程变量。
type Variable struct {
	Name  string `json:"name"`            // 变量名
	Type  string `json:"type,omitempty"`  // 类型，留空由引擎推断
	Value any    `json:"value"`           // 变量值
	Scope string `json:"scope,omitempty"` // global 或 local
}

// AsyncVariableRequest 是事务内持久排队的变量更新请求；实例和执行标识必须二选一。
type AsyncVariableRequest struct {
	ProcessInstanceID string     // 实例标识，变量写入实例根作用域并忽略变量 scope
	ExecutionID       string     // 执行标识，支持统一的 local 或 global 作用域
	Variables         []Variable // 同一作业原子更新的变量
	CreateOnly        bool       // 排队前拒绝已存在的变量，对应集合 POST
	RequireExisting   bool       // 排队前要求变量存在，对应单变量 PUT
}

// DeploymentRequest 是部署请求。
type DeploymentRequest struct {
	Name      string            // 部署名称
	TenantID  string            // 租户
	Resources map[string][]byte // 资源名到内容
}

// Deployment 是部署信息。
type Deployment struct {
	ID         string    `json:"id"`         // 部署 ID
	Name       string    `json:"name"`       // 部署名称
	TenantID   string    `json:"tenantId"`   // 租户
	DeployTime time.Time `json:"deployTime"` // 部署时间
}

// Model 是流程模型元数据。
type Model struct {
	ID             string    `json:"id"`                     // 模型 ID
	Name           string    `json:"name"`                   // 名称
	Key            string    `json:"key"`                    // Key
	Category       string    `json:"category,omitempty"`     // 分类
	Version        int       `json:"version"`                // 版本
	MetaInfo       string    `json:"metaInfo,omitempty"`     // 元信息
	DeploymentID   string    `json:"deploymentId,omitempty"` // 部署 ID
	TenantID       string    `json:"tenantId,omitempty"`     // 租户
	CreateTime     time.Time `json:"createTime"`             // 创建时间
	LastUpdateTime time.Time `json:"lastUpdateTime"`         // 最后更新时间
}

// CreateModelRequest 是模型创建请求。
type CreateModelRequest struct {
	Name     string // 名称
	Key      string // Key
	Category string // 分类
	MetaInfo string // 元信息
	TenantID string // 租户
	Version  int    // 版本，0 使用默认值
}

// ModelUpdate 是模型更新，nil 表示不修改。
type ModelUpdate struct {
	Name         *string // 名称
	Key          *string // Key
	Category     *string // 分类
	MetaInfo     *string // 元信息
	DeploymentID *string // 部署 ID
	TenantID     *string // 租户
	Version      *int    // 版本
}

// ProcessDefinition 是流程定义。
type ProcessDefinition struct {
	ID           string `json:"id"`                    // 流程定义 ID
	Key          string `json:"key"`                   // 流程定义 Key
	Name         string `json:"name"`                  // 名称
	Description  string `json:"description,omitempty"` // 流程说明
	Version      int    `json:"version"`               // 版本
	DeploymentID string `json:"deploymentId"`          // 部署 ID
	ResourceName string `json:"resourceName"`          // 资源名
	TenantID     string `json:"tenantId"`              // 租户
	Suspended    bool   `json:"suspended"`             // 是否挂起
}

// ProcessInstance 是流程实例。
type ProcessInstance struct {
	ID                        string         `json:"id"`                                  // 实例 ID
	ProcessDefinitionID       string         `json:"processDefinitionId"`                 // 流程定义 ID
	ProcessDefinitionKey      string         `json:"processDefinitionKey"`                // 流程定义 Key
	BusinessKey               string         `json:"businessKey"`                         // 业务键
	Name                      string         `json:"name"`                                // 实例名称
	ActivityID                string         `json:"activityId,omitempty"`                // 根执行当前活动
	StartUserID               string         `json:"startUserId,omitempty"`               // 发起人
	BusinessStatus            string         `json:"businessStatus,omitempty"`            // 业务状态
	DueDate                   *time.Time     `json:"dueDate,omitempty"`                   // 截止时间
	ClaimTime                 *time.Time     `json:"claimTime,omitempty"`                 // 签收时间
	ClaimedBy                 string         `json:"claimedBy,omitempty"`                 // 签收人
	CallbackID                string         `json:"callbackId,omitempty"`                // 回调标识
	CallbackType              string         `json:"callbackType,omitempty"`              // 回调类型
	ReferenceID               string         `json:"referenceId,omitempty"`               // 关联标识
	ReferenceType             string         `json:"referenceType,omitempty"`             // 关联类型
	PropagatedStageInstanceID string         `json:"propagatedStageInstanceId,omitempty"` // 传播阶段标识
	SuperProcessInstanceID    string         `json:"superProcessInstanceId,omitempty"`    // 父流程实例标识
	TenantID                  string         `json:"tenantId"`                            // 租户
	StartTime                 time.Time      `json:"startTime"`                           // 开始时间
	EndTime                   *time.Time     `json:"endTime,omitempty"`                   // 结束时间
	Ended                     bool           `json:"ended"`                               // 是否结束
	Suspended                 bool           `json:"suspended"`                           // 是否挂起
	Variables                 map[string]any `json:"variables,omitempty"`                 // 变量快照
}

// ProcessInstanceUpdate 是单次事务内的实例属性更新或签收请求，空指针表示保持原值。
type ProcessInstanceUpdate struct {
	Name           *string    // 实例名称
	BusinessKey    *string    // 业务键
	BusinessStatus *string    // 业务状态
	DueDate        *time.Time // 截止时间
	ClaimUserID    *string    // 签收人，空字符串表示取消签收
}

// Task 是用户任务。
type Task struct {
	ID                  string         `json:"id"`                        // 任务 ID
	ExecutionID         string         `json:"executionId,omitempty"`     // 所属执行 ID
	ClaimTime           *time.Time     `json:"claimTime,omitempty"`       // 最近签收时间
	Suspended           bool           `json:"suspended,omitempty"`       // 所属实例是否挂起
	Name                string         `json:"name"`                      // 任务名称
	Description         string         `json:"description"`               // 描述
	ProcessInstanceID   string         `json:"processInstanceId"`         // 流程实例 ID
	ProcessDefinitionID string         `json:"processDefinitionId"`       // 流程定义 ID
	TaskDefinitionKey   string         `json:"taskDefinitionKey"`         // BPMN 任务 ID
	Assignee            string         `json:"assignee,omitempty"`        // 处理人
	Owner               string         `json:"owner,omitempty"`           // 所有者
	DelegationState     string         `json:"delegationState,omitempty"` // 委派状态：PENDING/RESOLVED
	Priority            int            `json:"priority"`                  // 优先级
	FormKey             string         `json:"formKey,omitempty"`         // 表单 Key
	Category            string         `json:"category,omitempty"`        // 分类
	ParentTaskID        string         `json:"parentTaskId,omitempty"`    // 父任务 ID
	TenantID            string         `json:"tenantId,omitempty"`        // 租户
	CreateTime          time.Time      `json:"createTime"`                // 创建时间
	DueDate             *time.Time     `json:"dueDate,omitempty"`         // 到期时间
	CandidateUsers      []string       `json:"candidateUsers,omitempty"`  // 候选用户
	CandidateGroups     []string       `json:"candidateGroups,omitempty"` // 候选组
	Variables           map[string]any `json:"variables,omitempty"`       // 任务变量
}

// CreateTaskRequest 是独立任务创建请求。
type CreateTaskRequest struct {
	Name         string     // 名称
	Description  string     // 描述
	Assignee     string     // 处理人
	Owner        string     // 所有者
	Priority     int        // 优先级，0 使用默认值
	DueDate      *time.Time // 到期时间
	Category     string     // 分类
	FormKey      string     // 表单 Key
	ParentTaskID string     // 父任务 ID
	TenantID     string     // 租户
}

// InjectRequest 是动态活动注入请求。
type InjectRequest struct {
	InjectionType string `json:"injectionType,omitempty"` // 注入类型，当前仅支持 task
	ID            string `json:"id,omitempty"`            // 动态任务 ID
	Name          string `json:"name"`                    // 任务名称
	Assignee      string `json:"assignee,omitempty"`      // 处理人
	TaskID        string `json:"taskId,omitempty"`        // 并行注入的参考任务 ID
}

// TaskUpdate 是任务属性更新，nil 表示不修改。
type TaskUpdate struct {
	Name         *string    // 名称
	Description  *string    // 描述
	Assignee     *string    // 处理人
	Owner        *string    // 所有者
	Priority     *int       // 优先级
	DueDate      *time.Time // 到期时间
	ClearDueDate bool       // 清空到期时间
	Category     *string    // 分类
	FormKey      *string    // 表单 Key
	UserID       string     // 操作人，用于审计日志
}

// TaskFilter 是任务查询条件。
type TaskFilter struct {
	Assignee             string // 处理人
	Owner                string // 所有者
	ProcessInstanceID    string // 流程实例 ID
	ProcessDefinitionKey string // 流程定义 Key
	CandidateUser        string // 候选人
	ParentTaskID         string // 父任务 ID
	DelegationState      string // 委派状态
	TenantID             string // 租户
	Start                int    // 偏移量
	Size                 int    // 每页数量
}

// ProcessInstanceFilter 是运行中实例的查询条件。
// 仅包含可下推 SQL 的字段；名称、模糊匹配与键集合等仍由调用方在内存过滤。
type ProcessInstanceFilter struct {
	ID                   string     // 流程实例 ID
	ProcessDefinitionID  string     // 流程定义 ID
	ProcessDefinitionKey string     // 流程定义 Key
	BusinessKey          string     // 业务键
	TenantID             string     // 租户，空表示不过滤
	Suspended            *bool      // 是否挂起，空表示不过滤
	StartedAfter         *time.Time // 开始时间下界，含边界
	StartedBefore        *time.Time // 开始时间上界，含边界
	Start                int        // 偏移量
	Size                 int        // 每页数量
}

// HistoricProcessInstanceFilter 是历史流程实例的查询条件。
// 仅包含可下推 SQL 的字段；名称、模糊匹配、键集合与删除原因等仍由调用方在内存过滤。
type HistoricProcessInstanceFilter struct {
	ID                   string     // 流程实例 ID
	ProcessDefinitionID  string     // 流程定义 ID
	ProcessDefinitionKey string     // 流程定义 Key
	BusinessKey          string     // 业务键
	TenantID             string     // 租户，空表示不过滤
	StartedAfter         *time.Time // 开始时间下界，含边界
	StartedBefore        *time.Time // 开始时间上界，含边界
	FinishedAfter        *time.Time // 结束时间下界，含边界
	FinishedBefore       *time.Time // 结束时间上界，含边界
	Finished             *bool      // 是否已结束，空表示不过滤
	Start                int        // 偏移量
	Size                 int        // 每页数量
}

// HistoricProcessInstance 是历史流程实例。
type HistoricProcessInstance struct {
	ID                        string     `json:"id"`                                  // 实例 ID
	Revision                  int        `json:"revision,omitempty"`                  // 乐观锁版本
	ProcessDefinitionID       string     `json:"processDefinitionId"`                 // 流程定义 ID
	ProcessDefinitionKey      string     `json:"processDefinitionKey"`                // 流程定义 Key
	BusinessKey               string     `json:"businessKey"`                         // 业务键
	Name                      string     `json:"name"`                                // 名称
	TenantID                  string     `json:"tenantId"`                            // 租户
	StartTime                 time.Time  `json:"startTime"`                           // 开始时间
	EndTime                   *time.Time `json:"endTime,omitempty"`                   // 结束时间
	DurationMillis            int64      `json:"durationMillis,omitempty"`            // 持续时间毫秒
	StartUserID               string     `json:"startUserId,omitempty"`               // 发起人
	StartActivityID           string     `json:"startActivityId,omitempty"`           // 开始活动
	EndActivityID             string     `json:"endActivityId,omitempty"`             // 结束活动
	SuperProcessInstanceID    string     `json:"superProcessInstanceId,omitempty"`    // 父流程实例
	CallbackID                string     `json:"callbackId,omitempty"`                // 回调标识
	CallbackType              string     `json:"callbackType,omitempty"`              // 回调类型
	ReferenceID               string     `json:"referenceId,omitempty"`               // 外部引用
	ReferenceType             string     `json:"referenceType,omitempty"`             // 外部引用类型
	PropagatedStageInstanceID string     `json:"propagatedStageInstanceId,omitempty"` // 传播阶段实例
	BusinessStatus            string     `json:"businessStatus,omitempty"`            // 业务状态
	DueDate                   *time.Time `json:"dueDate,omitempty"`                   // 到期时间
	ClaimTime                 *time.Time `json:"claimTime,omitempty"`                 // 签收时间
	ClaimedBy                 string     `json:"claimedBy,omitempty"`                 // 签收人
	EndUserID                 string     `json:"endUserId,omitempty"`                 // 结束操作人
	State                     string     `json:"state,omitempty"`                     // 实例状态
	DeleteReason              string     `json:"deleteReason,omitempty"`              // 删除原因
	Ended                     bool       `json:"ended"`                               // 是否结束
}

// HistoricTaskInstanceFilter 是历史任务的查询条件。
// 仅包含可下推 SQL 的字段；模糊匹配等仍由调用方在内存过滤。
type HistoricTaskInstanceFilter struct {
	ID                   string     // 历史/任务 ID
	Name                 string     // 任务名称
	Assignee             string     // 处理人
	TaskDefinitionKey    string     // 任务定义 Key
	ProcessInstanceID    string     // 流程实例 ID
	ProcessDefinitionID  string     // 流程定义 ID
	ProcessDefinitionKey string     // 流程定义 Key
	TenantID             string     // 租户，空表示不过滤
	StartedAfter         *time.Time // 开始时间下界，含边界
	StartedBefore        *time.Time // 开始时间上界，含边界
	FinishedAfter        *time.Time // 结束时间下界，含边界
	FinishedBefore       *time.Time // 结束时间上界，含边界
	Finished             *bool      // 是否已结束，空表示不过滤
	Start                int        // 偏移量
	Size                 int        // 每页数量
}

// HistoricActivityInstance 是历史活动实例。
// 字段与 ACT_HI_ACTINST 列一一对应，转换 Java 存量数据时不能因查询层未使用而丢失原始列。
type HistoricActivityInstance struct {
	ID                    string     `json:"id"`                              // 历史记录 ID
	Revision              int        `json:"revision,omitempty"`              // 乐观锁版本
	ProcessDefinitionID   string     `json:"processDefinitionId,omitempty"`   // 流程定义 ID
	ProcessInstanceID     string     `json:"processInstanceId"`               // 流程实例 ID
	ExecutionID           string     `json:"executionId,omitempty"`           // 执行 ID
	ActivityID            string     `json:"activityId"`                      // 活动 ID
	TaskID                string     `json:"taskId,omitempty"`                // 对应用户任务 ID
	CallProcessInstanceID string     `json:"callProcessInstanceId,omitempty"` // 调用活动实例 ID
	ActivityName          string     `json:"activityName"`                    // 活动名称
	ActivityType          string     `json:"activityType"`                    // 活动类型
	Assignee              string     `json:"assignee,omitempty"`              // 处理人
	CompletedBy           string     `json:"completedBy,omitempty"`           // 完成人
	StartTime             time.Time  `json:"startTime"`                       // 开始时间
	EndTime               *time.Time `json:"endTime,omitempty"`               // 结束时间
	TransactionOrder      int        `json:"transactionOrder,omitempty"`      // 事务顺序
	DurationMillis        int64      `json:"durationMillis"`                  // 持续时间毫秒
	DeleteReason          string     `json:"deleteReason,omitempty"`          // 删除原因
	TenantID              string     `json:"tenantId,omitempty"`              // 租户
}

// HistoricTaskInstance 是历史任务实例。
// 字段与 ACT_HI_TASKINST 列一一对应，保证 Java 存量任务在离线转换后仍可无损写回。
type HistoricTaskInstance struct {
	ID                        string     `json:"id"`                                  // 任务 ID
	Revision                  int        `json:"revision,omitempty"`                  // 乐观锁版本
	ProcessDefinitionID       string     `json:"processDefinitionId,omitempty"`       // 流程定义 ID
	TaskDefinitionID          string     `json:"taskDefinitionId,omitempty"`          // 任务定义 ID
	Name                      string     `json:"name"`                                // 名称
	ProcessInstanceID         string     `json:"processInstanceId"`                   // 流程实例 ID
	ExecutionID               string     `json:"executionId,omitempty"`               // 执行 ID
	ScopeID                   string     `json:"scopeId,omitempty"`                   // 作用域 ID
	SubScopeID                string     `json:"subScopeId,omitempty"`                // 子作用域 ID
	ScopeType                 string     `json:"scopeType,omitempty"`                 // 作用域类型
	ScopeDefinitionID         string     `json:"scopeDefinitionId,omitempty"`         // 作用域定义 ID
	PropagatedStageInstanceID string     `json:"propagatedStageInstanceId,omitempty"` // 传播阶段实例 ID
	State                     string     `json:"state,omitempty"`                     // 任务状态
	TaskDefinitionKey         string     `json:"taskDefinitionKey"`                   // 任务定义 Key
	ParentTaskID              string     `json:"parentTaskId,omitempty"`              // 父任务 ID
	Description               string     `json:"description,omitempty"`               // 描述
	Owner                     string     `json:"owner,omitempty"`                     // 所有者
	Assignee                  string     `json:"assignee,omitempty"`                  // 处理人
	FormKey                   string     `json:"formKey,omitempty"`                   // 表单 Key
	TenantID                  string     `json:"tenantId,omitempty"`                  // 租户
	DeleteReason              string     `json:"deleteReason,omitempty"`              // 删除原因
	StartTime                 time.Time  `json:"startTime"`                           // 开始时间
	InProgressTime            *time.Time `json:"inProgressTime,omitempty"`            // 开始处理时间
	InProgressStartedBy       string     `json:"inProgressStartedBy,omitempty"`       // 开始处理人
	ClaimTime                 *time.Time `json:"claimTime,omitempty"`                 // 签收时间
	ClaimedBy                 string     `json:"claimedBy,omitempty"`                 // 签收人
	SuspendedTime             *time.Time `json:"suspendedTime,omitempty"`             // 挂起时间
	SuspendedBy               string     `json:"suspendedBy,omitempty"`               // 挂起操作人
	EndTime                   *time.Time `json:"endTime,omitempty"`                   // 结束时间
	CompletedBy               string     `json:"completedBy,omitempty"`               // 完成人
	DurationMillis            int64      `json:"durationMillis"`                      // 持续时间毫秒
	Priority                  int        `json:"priority,omitempty"`                  // 优先级
	InProgressDueDate         *time.Time `json:"inProgressDueDate,omitempty"`         // 处理截止时间
	DueDate                   *time.Time `json:"dueDate,omitempty"`                   // 截止时间
	Category                  string     `json:"category,omitempty"`                  // 分类
	LastUpdatedTime           *time.Time `json:"lastUpdatedTime,omitempty"`           // 最后更新时间
}

// TaskEvent 是任务事件。
type TaskEvent struct {
	ID                  int64     `json:"id"`                            // 事件 ID
	Action              string    `json:"action"`                        // 动作
	Timestamp           time.Time `json:"timestamp"`                     // 发生时间
	UserID              string    `json:"userId,omitempty"`              // 操作人
	TaskID              string    `json:"taskId,omitempty"`              // 任务 ID
	ExecutionID         string    `json:"executionId,omitempty"`         // 执行 ID
	ProcessInstanceID   string    `json:"processInstanceId,omitempty"`   // 流程实例 ID
	ProcessDefinitionID string    `json:"processDefinitionId,omitempty"` // 流程定义 ID
	TenantID            string    `json:"tenantId,omitempty"`            // 租户
	Data                string    `json:"data,omitempty"`                // 附加数据
}

// IdentityLink 是任务/流程参与者关联。
type IdentityLink struct {
	ID                  string    `json:"id,omitempty"`                  // 关联 ID
	Type                string    `json:"type"`                          // 类型
	UserID              string    `json:"userId,omitempty"`              // 用户 ID
	GroupID             string    `json:"groupId,omitempty"`             // 组 ID
	TaskID              string    `json:"taskId,omitempty"`              // 任务 ID
	ProcessInstanceID   string    `json:"processInstanceId,omitempty"`   // 流程实例 ID
	ProcessDefinitionID string    `json:"processDefinitionId,omitempty"` // 流程定义 ID
	CreateTime          time.Time `json:"createTime,omitempty"`          // 创建时间
}

// Comment 是任务评论。
type Comment struct {
	ID                string    `json:"id"`                          // 评论 ID
	TaskID            string    `json:"taskId,omitempty"`            // 任务 ID
	ProcessInstanceID string    `json:"processInstanceId,omitempty"` // 流程实例 ID
	Type              string    `json:"type"`                        // 评论类型
	Action            string    `json:"action,omitempty"`            // 动作，事件型记录使用
	Message           string    `json:"message"`                     // 内容
	FullMessage       string    `json:"fullMessage,omitempty"`       // 完整内容
	UserID            string    `json:"userId,omitempty"`            // 作者
	Time              time.Time `json:"time"`                        // 创建时间
}

// Attachment 是任务附件。
type Attachment struct {
	ID                string    `json:"id"`                          // 附件 ID
	TaskID            string    `json:"taskId,omitempty"`            // 任务 ID
	ProcessInstanceID string    `json:"processInstanceId,omitempty"` // 流程实例 ID
	Name              string    `json:"name"`                        // 名称
	Description       string    `json:"description,omitempty"`       // 描述
	Type              string    `json:"type,omitempty"`              // 类型
	URL               string    `json:"url,omitempty"`               // URL
	ContentURL        string    `json:"contentUrl,omitempty"`        // 内容下载地址
	Time              time.Time `json:"time"`                        // 创建时间
}

// VariableInstance 是运行时变量实例。
type VariableInstance struct {
	ID                string `json:"id"`                          // 变量实例 ID
	Name              string `json:"name"`                        // 变量名
	Type              string `json:"type,omitempty"`              // 变量类型
	Value             any    `json:"value,omitempty"`             // 变量值
	ProcessInstanceID string `json:"processInstanceId,omitempty"` // 流程实例 ID
	ExecutionID       string `json:"executionId,omitempty"`       // 执行 ID
	TaskID            string `json:"taskId,omitempty"`            // 任务 ID
	Scope             string `json:"scope,omitempty"`             // 作用域
}

// VariableInstanceFilter 是变量实例查询条件。
type VariableInstanceFilter struct {
	ProcessInstanceID string // 流程实例 ID
	ExecutionID       string // 执行 ID
	TaskID            string // 任务 ID
	Name              string // 变量名
	NameLike          string // 变量名模糊匹配
}

// ActivityInstance 是运行中活动实例。
type ActivityInstance struct {
	ID                  string     `json:"id"`                            // 活动实例 ID
	ActivityID          string     `json:"activityId"`                    // 活动 ID
	ActivityName        string     `json:"activityName,omitempty"`        // 活动名称
	ActivityType        string     `json:"activityType,omitempty"`        // 活动类型
	ProcessDefinitionID string     `json:"processDefinitionId,omitempty"` // 流程定义 ID
	ProcessInstanceID   string     `json:"processInstanceId,omitempty"`   // 流程实例 ID
	ExecutionID         string     `json:"executionId,omitempty"`         // 执行 ID
	TaskID              string     `json:"taskId,omitempty"`              // 任务 ID
	Assignee            string     `json:"assignee,omitempty"`            // 处理人
	TenantID            string     `json:"tenantId,omitempty"`            // 租户
	StartTime           *time.Time `json:"startTime,omitempty"`           // 开始时间
	EndTime             *time.Time `json:"endTime,omitempty"`             // 结束时间
	DurationInMillis    *int64     `json:"durationInMillis,omitempty"`    // 持续时间
}

// EventSubscription 是消息/信号事件订阅。
type EventSubscription struct {
	ID                  string    `json:"id"`                            // 订阅 ID
	URL                 string    `json:"url,omitempty"`                 // 资源地址
	EventType           string    `json:"eventType"`                     // 事件类型
	EventName           string    `json:"eventName"`                     // 事件名
	ExecutionID         string    `json:"executionId,omitempty"`         // 执行 ID
	ProcessInstanceID   string    `json:"processInstanceId,omitempty"`   // 流程实例 ID
	ActivityID          string    `json:"activityId,omitempty"`          // 活动 ID
	ProcessDefinitionID string    `json:"processDefinitionId,omitempty"` // 流程定义 ID
	TenantID            string    `json:"tenantId,omitempty"`            // 租户
	Created             time.Time `json:"created"`                       // 创建时间
}

// ExecutionInfo 是执行实例信息。
type ExecutionInfo struct {
	ProcessDefinitionKey string `json:"processDefinitionKey"`       // 流程定义 Key
	ID                   string `json:"id"`                         // 执行 ID
	ProcessInstanceID    string `json:"processInstanceId"`          // 流程实例 ID
	ProcessDefinitionID  string `json:"processDefinitionId"`        // 流程定义 ID
	ParentID             string `json:"parentId,omitempty"`         // 父执行 ID
	SuperExecutionID     string `json:"superExecutionId,omitempty"` // 父调用活动执行 ID
	TenantID             string `json:"tenantId,omitempty"`         // 租户标识
	Suspended            bool   `json:"suspended,omitempty"`        // 是否挂起
	ActivityID           string `json:"activityId,omitempty"`       // 当前活动 ID
	Active               bool   `json:"active"`                     // 是否活跃
	Ended                bool   `json:"ended"`                      // 是否结束
	Waiting              bool   `json:"waiting"`                    // 是否等待
}

// HistoricTaskLog 是任务生命周期日志。
type HistoricTaskLog struct {
	ID                  int64     `json:"id"`                            // 日志 ID
	Type                string    `json:"type"`                          // 事件类型
	TaskID              string    `json:"taskId,omitempty"`              // 任务 ID
	Time                time.Time `json:"time"`                          // 时间
	UserID              string    `json:"userId,omitempty"`              // 操作人
	Data                string    `json:"data,omitempty"`                // 附加数据
	ExecutionID         string    `json:"executionId,omitempty"`         // 执行 ID
	ProcessInstanceID   string    `json:"processInstanceId,omitempty"`   // 实例 ID
	ProcessDefinitionID string    `json:"processDefinitionId,omitempty"` // 定义 ID
	TenantID            string    `json:"tenantId,omitempty"`            // 租户
}

// HistoricActivityInstanceFilter 是历史活动的查询条件。
// 仅包含可下推 SQL 的字段；模糊匹配等仍由调用方在内存过滤。
type HistoricActivityInstanceFilter struct {
	ID                   string     // 历史活动 ID
	ProcessInstanceID    string     // 流程实例 ID
	ProcessDefinitionID  string     // 流程定义 ID
	ProcessDefinitionKey string     // 流程定义 Key
	ActivityID           string     // 活动 ID
	ActivityName         string     // 活动名称
	ActivityType         string     // 活动类型
	Assignee             string     // 处理人
	TenantID             string     // 租户，空表示不过滤
	StartedAfter         *time.Time // 开始时间下界，含边界
	StartedBefore        *time.Time // 开始时间上界，含边界
	FinishedAfter        *time.Time // 结束时间下界，含边界
	FinishedBefore       *time.Time // 结束时间上界，含边界
	Finished             *bool      // 是否已结束，空表示不过滤
	Start                int        // 偏移量
	Size                 int        // 每页数量
}

// HistoricVariableInstanceFilter 是历史变量的查询条件。
// 仅包含可下推 SQL 的字段；模糊匹配与租户等仍由调用方在内存过滤。
type HistoricVariableInstanceFilter struct {
	ID                string // 历史/变量 ID
	Name              string // 变量名
	ProcessInstanceID string // 流程实例 ID
	ExecutionID       string // 执行 ID
	TaskID            string // 任务 ID
	VariableType      string // 变量类型
	Start             int    // 偏移量
	Size              int    // 每页数量
}

// HistoricDetail 是历史变量明细。
type HistoricDetail struct {
	ID                string    `json:"id"`                          // 明细 ID
	Type              string    `json:"type"`                        // 类型
	ProcessInstanceID string    `json:"processInstanceId,omitempty"` // 流程实例 ID
	ExecutionID       string    `json:"executionId,omitempty"`       // 执行 ID
	TaskID            string    `json:"taskId,omitempty"`            // 任务 ID
	ActivityID        string    `json:"activityId,omitempty"`        // 活动 ID
	Name              string    `json:"name"`                        // 变量名
	Revision          int       `json:"revision"`                    // 变量修订号
	VariableType      string    `json:"variableType,omitempty"`      // 变量类型
	Value             any       `json:"value,omitempty"`             // 变量值
	Time              time.Time `json:"time"`                        // 记录时间
}

// HistoricVariableInstance 是历史变量实例。
type HistoricVariableInstance struct {
	ID                string    `json:"id"`                    // 历史变量 ID
	Name              string    `json:"name"`                  // 变量名
	Type              string    `json:"type,omitempty"`        // 变量类型
	ProcessInstanceID string    `json:"processInstanceId"`     // 流程实例 ID
	ExecutionID       string    `json:"executionId,omitempty"` // 执行 ID
	TaskID            string    `json:"taskId,omitempty"`      // 任务 ID
	Value             any       `json:"value,omitempty"`       // 变量值
	CreateTime        time.Time `json:"createTime"`            // 创建时间
	LastUpdatedTime   time.Time `json:"lastUpdatedTime"`       // 最后更新时间
}

// TableInfo 描述一张 ACT_* 表及当前行数。
type TableInfo struct {
	Name  string `json:"name"`          // 表名
	Count int64  `json:"count"`         // 行数
	URL   string `json:"url,omitempty"` // 资源地址
}

// TableMetaData 描述表的列元数据。
type TableMetaData struct {
	TableName   string   `json:"tableName"`   // 表名
	ColumnNames []string `json:"columnNames"` // 列名
	ColumnTypes []string `json:"columnTypes"` // 列类型
}

// Page 是通用分页结果。
type Page[T any] struct {
	Data  []T    `json:"data"`            // 数据
	Total int64  `json:"total"`           // 总数
	Start int    `json:"start"`           // 偏移
	Size  int    `json:"size"`            // 每页数量
	Sort  string `json:"sort,omitempty"`  // 排序字段
	Order string `json:"order,omitempty"` // 排序方向
}

// Batch 是批处理作业。
// 对应 Flowable 的 ACT_RU_BATCH 表，但仅保存内存快照所需的字段。
type Batch struct {
	ID        string     `json:"id"`                 // 批次 ID
	Type      string     `json:"type"`               // 批次类型
	Total     int        `json:"total"`              // 分片总数
	Completed int        `json:"completed"`          // 已完成分片数
	Status    string     `json:"status"`             // 批次状态
	StartTime time.Time  `json:"startTime"`          // 开始时间
	EndTime   *time.Time `json:"endTime,omitempty"`  // 结束时间
	TenantID  string     `json:"tenantId,omitempty"` // 租户
	Document  []byte     `json:"document,omitempty"` // 批次文档，旧快照可为空
}

// BatchPart 是批处理分片。
type BatchPart struct {
	ID                string     `json:"id"`                          // 分片 ID
	BatchID           string     `json:"batchId"`                     // 所属批次 ID
	Type              string     `json:"type"`                        // 分片类型
	ProcessInstanceID string     `json:"processInstanceId,omitempty"` // 流程实例 ID
	Status            string     `json:"status"`                      // 分片状态
	StartTime         time.Time  `json:"startTime"`                   // 开始时间
	EndTime           *time.Time `json:"endTime,omitempty"`           // 结束时间
	ErrorMessage      string     `json:"errorMessage,omitempty"`      // 失败原因
	Document          []byte     `json:"document,omitempty"`          // 分片文档，旧快照可为空
}
