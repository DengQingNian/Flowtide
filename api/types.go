package api

import "time"

// Variable 表示一个流程变量。
type Variable struct {
	Name  string `json:"name"`            // 变量名
	Type  string `json:"type,omitempty"`  // 类型，留空由引擎推断
	Value any    `json:"value"`           // 变量值
	Scope string `json:"scope,omitempty"` // global 或 local
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
	ID           string `json:"id"`           // 流程定义 ID
	Key          string `json:"key"`          // 流程定义 Key
	Name         string `json:"name"`         // 名称
	Version      int    `json:"version"`      // 版本
	DeploymentID string `json:"deploymentId"` // 部署 ID
	ResourceName string `json:"resourceName"` // 资源名
	TenantID     string `json:"tenantId"`     // 租户
	Suspended    bool   `json:"suspended"`    // 是否挂起
}

// ProcessInstance 是流程实例。
type ProcessInstance struct {
	ID                   string         `json:"id"`                   // 实例 ID
	ProcessDefinitionID  string         `json:"processDefinitionId"`  // 流程定义 ID
	ProcessDefinitionKey string         `json:"processDefinitionKey"` // 流程定义 Key
	BusinessKey          string         `json:"businessKey"`          // 业务键
	Name                 string         `json:"name"`                 // 实例名称
	TenantID             string         `json:"tenantId"`             // 租户
	StartTime            time.Time      `json:"startTime"`            // 开始时间
	EndTime              *time.Time     `json:"endTime,omitempty"`    // 结束时间
	Ended                bool           `json:"ended"`                // 是否结束
	Suspended            bool           `json:"suspended"`            // 是否挂起
	Variables            map[string]any `json:"variables,omitempty"`  // 变量快照
}

// Task 是用户任务。
type Task struct {
	ID                  string         `json:"id"`                        // 任务 ID
	Name                string         `json:"name"`                      // 任务名称
	Description         string         `json:"description"`               // 描述
	ProcessInstanceID   string         `json:"processInstanceId"`         // 流程实例 ID
	ProcessDefinitionID string         `json:"processDefinitionId"`       // 流程定义 ID
	TaskDefinitionKey   string         `json:"taskDefinitionKey"`         // BPMN 任务 ID
	Assignee            string         `json:"assignee,omitempty"`        // 处理人
	Owner               string         `json:"owner,omitempty"`           // 所有者
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
	TenantID             string // 租户
	Start                int    // 偏移量
	Size                 int    // 每页数量
}

// HistoricProcessInstance 是历史流程实例。
type HistoricProcessInstance struct {
	ID                   string     `json:"id"`                     // 实例 ID
	ProcessDefinitionID  string     `json:"processDefinitionId"`    // 流程定义 ID
	ProcessDefinitionKey string     `json:"processDefinitionKey"`   // 流程定义 Key
	BusinessKey          string     `json:"businessKey"`            // 业务键
	Name                 string     `json:"name"`                   // 名称
	TenantID             string     `json:"tenantId"`               // 租户
	StartTime            time.Time  `json:"startTime"`              // 开始时间
	EndTime              *time.Time `json:"endTime,omitempty"`      // 结束时间
	DeleteReason         string     `json:"deleteReason,omitempty"` // 删除原因
	Ended                bool       `json:"ended"`                  // 是否结束
}

// HistoricActivityInstance 是历史活动实例。
type HistoricActivityInstance struct {
	ID                string     `json:"id"`                // 历史记录 ID
	ProcessInstanceID string     `json:"processInstanceId"` // 流程实例 ID
	ActivityID        string     `json:"activityId"`        // 活动 ID
	ActivityName      string     `json:"activityName"`      // 活动名称
	ActivityType      string     `json:"activityType"`      // 活动类型
	StartTime         time.Time  `json:"startTime"`         // 开始时间
	EndTime           *time.Time `json:"endTime,omitempty"` // 结束时间
	DurationMillis    int64      `json:"durationMillis"`    // 持续时间毫秒
}

// HistoricTaskInstance 是历史任务实例。
type HistoricTaskInstance struct {
	ID                string     `json:"id"`                     // 任务 ID
	Name              string     `json:"name"`                   // 名称
	ProcessInstanceID string     `json:"processInstanceId"`      // 流程实例 ID
	TaskDefinitionKey string     `json:"taskDefinitionKey"`      // 任务定义 Key
	Assignee          string     `json:"assignee,omitempty"`     // 处理人
	FormKey           string     `json:"formKey,omitempty"`      // 表单 Key
	TenantID          string     `json:"tenantId,omitempty"`     // 租户
	DeleteReason      string     `json:"deleteReason,omitempty"` // 删除原因
	StartTime         time.Time  `json:"startTime"`              // 开始时间
	EndTime           *time.Time `json:"endTime,omitempty"`      // 结束时间
	DurationMillis    int64      `json:"durationMillis"`         // 持续时间毫秒
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
	Message           string    `json:"message"`                     // 内容
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
	ID                  string `json:"id"`                   // 执行 ID
	ProcessInstanceID   string `json:"processInstanceId"`    // 流程实例 ID
	ProcessDefinitionID string `json:"processDefinitionId"`  // 流程定义 ID
	ParentID            string `json:"parentId,omitempty"`   // 父执行 ID
	ActivityID          string `json:"activityId,omitempty"` // 当前活动 ID
	Active              bool   `json:"active"`               // 是否活跃
	Ended               bool   `json:"ended"`                // 是否结束
	Waiting             bool   `json:"waiting"`              // 是否等待
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

// HistoricDetail 是历史变量明细。
type HistoricDetail struct {
	ID                string    `json:"id"`                          // 明细 ID
	Type              string    `json:"type"`                        // 类型
	ProcessInstanceID string    `json:"processInstanceId,omitempty"` // 流程实例 ID
	ExecutionID       string    `json:"executionId,omitempty"`       // 执行 ID
	TaskID            string    `json:"taskId,omitempty"`            // 任务 ID
	ActivityID        string    `json:"activityId,omitempty"`        // 活动 ID
	Name              string    `json:"name"`                        // 变量名
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
}
