package engine

import (
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
)

// deploymentState 是部署的运行时状态。
type deploymentState struct {
	Info      api.Deployment    // 部署信息
	Resources map[string][]byte // 资源内容
}

// definitionState 是流程定义的运行时状态。
type definitionState struct {
	Info  api.ProcessDefinition // 流程定义信息
	Model *model.BpmnModel      // 解析后的模型
	XML   []byte                // 原始 XML
}

// instanceState 是流程实例的运行时状态。
type instanceState struct {
	ID                   string     // 实例 ID
	ProcessDefinitionID  string     // 流程定义 ID
	ProcessDefinitionKey string     // 流程定义 Key
	BusinessKey          string     // 业务键
	Name                 string     // 名称
	TenantID             string     // 租户
	StartTime            time.Time  // 开始时间
	EndTime              *time.Time // 结束时间
	Ended                bool       // 是否结束
	Suspended            bool       // 是否挂起
	DeleteReason         string     // 删除原因
	SuperExecutionID     string     // 父调用活动执行 ID
	RootExecutionID      string     // 根执行 ID
}

// executionState 是执行树节点。
type executionState struct {
	ID                    string         // 执行 ID
	ProcessInstanceID     string         // 流程实例 ID
	DefinitionID          string         // 流程定义 ID
	ParentID              string         // 父执行 ID
	ActivityID            string         // 当前活动 ID
	Active                bool           // 是否活跃
	Scope                 bool           // 是否作用域
	Concurrent            bool           // 是否并发分支
	Ended                 bool           // 是否结束
	Waiting               bool           // 是否等待外部事件
	ScopeRoot             bool           // 是否子流程作用域根执行
	MultiInstanceRoot     bool           // 是否多实例根执行
	MIChild               bool           // 是否多实例子执行
	MIValues              []any          // 多实例元素值
	MISequential          bool           // 是否串行多实例
	MIActivityID          string         // 多实例活动 ID
	MIElementVariable     string         // 元素变量名
	MICompletionCondition string         // 完成条件
	MIIndex               int            // 子实例序号
	SkipAsyncBefore       bool           // 本次进入跳过异步前
	SkipAsyncAfter        bool           // 本次离开跳过异步后
	PendingBranches       int            // 等待的并行分支数
	CurrentHistActivityID string         // 当前活动历史记录 ID
	Variables             map[string]any // 局部变量
}

// jobState 是定时器作业。
type jobState struct {
	ID                string    `json:"id"`                // 作业 ID
	Type              string    `json:"type"`              // 作业类型：boundary-timer/async-before/async-after
	ProcessInstanceID string    `json:"processInstanceId"` // 流程实例 ID
	ExecutionID       string    `json:"executionId"`       // 执行 ID
	BoundaryEventID   string    `json:"boundaryEventId"`   // 边界事件 ID
	ActivityID        string    `json:"activityId"`        // 依附活动 ID
	CancelActivity    bool      `json:"cancelActivity"`    // 是否中断
	DueDate           time.Time `json:"dueDate"`           // 到期时间
	CreatedAt         time.Time `json:"createdAt"`         // 创建时间
}

// subscriptionState 是消息/信号订阅。
type subscriptionState struct {
	ID                  string    `json:"id"`                  // 订阅 ID
	ProcessDefinitionID string    `json:"processDefinitionId"` // 流程定义 ID
	ProcessInstanceID   string    `json:"processInstanceId"`   // 流程实例 ID，启动订阅为空
	ExecutionID         string    `json:"executionId"`         // 执行 ID
	EventType           string    `json:"eventType"`           // message 或 signal
	EventName           string    `json:"eventName"`           // 事件名
	ActivityID          string    `json:"activityId"`          // 活动 ID
	BoundaryEventID     string    `json:"boundaryEventId"`     // 边界事件 ID
	GatewayGroup        string    `json:"gatewayGroup"`        // 事件网关竞争组
	StartEvent          bool      `json:"startEvent"`          // 是否启动订阅
	TenantID            string    `json:"tenantId"`            // 租户
	CreatedAt           time.Time `json:"createdAt"`           // 创建时间
}

// taskState 是用户任务的运行时状态。
type taskState struct {
	Task            api.Task       // 任务数据
	ExecutionID     string         // 所属执行
	ActivityID      string         // BPMN 活动 ID
	HistoricTaskID  string         // 历史任务 ID
	CandidateUsers  []string       // 候选用户
	CandidateGroups []string       // 候选组
	LocalVariables  map[string]any // 任务局部变量
}

// state 是引擎的全部可变状态。
type state struct {
	Deployments        map[string]*deploymentState              // 部署
	Definitions        map[string]*definitionState              // 流程定义
	Instances          map[string]*instanceState                // 流程实例
	Executions         map[string]*executionState               // 执行
	Tasks              map[string]*taskState                    // 任务
	Jobs               map[string]*jobState                     // 定时器作业
	Subscriptions      map[string]*subscriptionState            // 事件订阅
	HistoricProcesses  map[string]*api.HistoricProcessInstance  // 历史流程实例
	HistoricActivities map[string]*api.HistoricActivityInstance // 历史活动
	HistoricTasks      map[string]*api.HistoricTaskInstance     // 历史任务
	Versions           map[string]int                           // 流程定义 Key 到最大版本
}

// newState 创建空状态。
// @return 状态
func newState() *state {
	return &state{
		Deployments:        make(map[string]*deploymentState),
		Definitions:        make(map[string]*definitionState),
		Instances:          make(map[string]*instanceState),
		Executions:         make(map[string]*executionState),
		Tasks:              make(map[string]*taskState),
		Jobs:               make(map[string]*jobState),
		Subscriptions:      make(map[string]*subscriptionState),
		HistoricProcesses:  make(map[string]*api.HistoricProcessInstance),
		HistoricActivities: make(map[string]*api.HistoricActivityInstance),
		HistoricTasks:      make(map[string]*api.HistoricTaskInstance),
		Versions:           make(map[string]int),
	}
}

// cloneState 深拷贝可变状态，用于命令失败回滚。
// @param source 源状态
// @return 拷贝后的状态
func cloneState(source *state) *state {
	target := newState()
	for id, deployment := range source.Deployments {
		copied := *deployment
		copied.Resources = cloneBytesMap(deployment.Resources)
		target.Deployments[id] = &copied
	}
	for id, definition := range source.Definitions {
		copied := *definition
		target.Definitions[id] = &copied
	}
	for id, instance := range source.Instances {
		copied := *instance
		copied.EndTime = cloneTime(instance.EndTime)
		target.Instances[id] = &copied
	}
	for id, execution := range source.Executions {
		copied := *execution
		copied.Variables = cloneAnyMap(execution.Variables)
		target.Executions[id] = &copied
	}
	for id, task := range source.Tasks {
		copied := *task
		copied.Task.Variables = cloneAnyMap(task.Task.Variables)
		copied.LocalVariables = cloneAnyMap(task.LocalVariables)
		target.Tasks[id] = &copied
	}
	for id, job := range source.Jobs {
		copied := *job
		target.Jobs[id] = &copied
	}
	for id, subscription := range source.Subscriptions {
		copied := *subscription
		target.Subscriptions[id] = &copied
	}
	for id, historic := range source.HistoricProcesses {
		copied := *historic
		copied.EndTime = cloneTime(historic.EndTime)
		target.HistoricProcesses[id] = &copied
	}
	for id, historic := range source.HistoricActivities {
		copied := *historic
		copied.EndTime = cloneTime(historic.EndTime)
		target.HistoricActivities[id] = &copied
	}
	for id, historic := range source.HistoricTasks {
		copied := *historic
		copied.EndTime = cloneTime(historic.EndTime)
		target.HistoricTasks[id] = &copied
	}
	for key, version := range source.Versions {
		target.Versions[key] = version
	}
	return target
}

// cloneAnyMap 复制任意值映射（浅拷贝值）。
// @param source 源映射
// @return 新映射
func cloneAnyMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	target := make(map[string]any, len(source))
	for key, value := range source {
		target[key] = value
	}
	return target
}

// cloneBytesMap 复制字节切片映射。
// @param source 源映射
// @return 新映射
func cloneBytesMap(source map[string][]byte) map[string][]byte {
	if source == nil {
		return nil
	}
	target := make(map[string][]byte, len(source))
	for key, value := range source {
		target[key] = append([]byte(nil), value...)
	}
	return target
}

// cloneTime 复制时间指针。
// @param source 时间指针
// @return 新时间指针
func cloneTime(source *time.Time) *time.Time {
	if source == nil {
		return nil
	}
	value := *source
	return &value
}
