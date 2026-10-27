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
	CreateTime          time.Time      `json:"createTime"`                // 创建时间
	DueDate             *time.Time     `json:"dueDate,omitempty"`         // 到期时间
	CandidateUsers      []string       `json:"candidateUsers,omitempty"`  // 候选用户
	CandidateGroups     []string       `json:"candidateGroups,omitempty"` // 候选组
	Variables           map[string]any `json:"variables,omitempty"`       // 任务变量
}

// TaskFilter 是任务查询条件。
type TaskFilter struct {
	Assignee             string // 处理人
	Owner                string // 所有者
	ProcessInstanceID    string // 流程实例 ID
	ProcessDefinitionKey string // 流程定义 Key
	CandidateUser        string // 候选人
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
	ID                string     `json:"id"`                 // 任务 ID
	Name              string     `json:"name"`               // 名称
	ProcessInstanceID string     `json:"processInstanceId"`  // 流程实例 ID
	TaskDefinitionKey string     `json:"taskDefinitionKey"`  // 任务定义 Key
	Assignee          string     `json:"assignee,omitempty"` // 处理人
	StartTime         time.Time  `json:"startTime"`          // 开始时间
	EndTime           *time.Time `json:"endTime,omitempty"`  // 结束时间
	DurationMillis    int64      `json:"durationMillis"`     // 持续时间毫秒
}

// IdentityLink 是任务/流程参与者关联。
type IdentityLink struct {
	ID                string    `json:"id,omitempty"`                // 关联 ID
	Type              string    `json:"type"`                        // 类型
	UserID            string    `json:"userId,omitempty"`            // 用户 ID
	GroupID           string    `json:"groupId,omitempty"`           // 组 ID
	TaskID            string    `json:"taskId,omitempty"`            // 任务 ID
	ProcessInstanceID string    `json:"processInstanceId,omitempty"` // 流程实例 ID
	CreateTime        time.Time `json:"createTime,omitempty"`        // 创建时间
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
	Time              time.Time `json:"time"`                        // 创建时间
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

// Page 是通用分页结果。
type Page[T any] struct {
	Data  []T    `json:"data"`            // 数据
	Total int64  `json:"total"`           // 总数
	Start int    `json:"start"`           // 偏移
	Size  int    `json:"size"`            // 每页数量
	Sort  string `json:"sort,omitempty"`  // 排序字段
	Order string `json:"order,omitempty"` // 排序方向
}
