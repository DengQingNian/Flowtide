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
	Info         api.ProcessDefinition // 流程定义信息
	Model        *model.BpmnModel      // 解析后的模型
	XML          []byte                // 原始 XML
	StarterLinks []*api.IdentityLink   // 候选发起人链接
}

// instanceState 是流程实例的运行时状态。
type instanceState struct {
	ID                        string     // 实例 ID
	ProcessDefinitionID       string     // 流程定义 ID
	ProcessDefinitionKey      string     // 流程定义 Key
	BusinessKey               string     // 业务键
	Name                      string     // 名称
	StartUserID               string     // 发起人
	BusinessStatus            string     // 业务状态
	DueDate                   *time.Time // 截止时间
	ClaimTime                 *time.Time // 签收时间
	ClaimedBy                 string     // 签收人
	CallbackID                string     // 回调标识
	CallbackType              string     // 回调类型
	ReferenceID               string     // 关联标识
	ReferenceType             string     // 关联类型
	PropagatedStageInstanceID string     // 传播阶段标识
	TenantID                  string     // 租户
	StartTime                 time.Time  // 开始时间
	EndTime                   *time.Time // 结束时间
	Ended                     bool       // 是否结束
	Suspended                 bool       // 是否挂起
	DeleteReason              string     // 删除原因
	SuperExecutionID          string     // 父调用活动执行 ID
	RootExecutionID           string     // 根执行 ID
}

// executionState 是执行树节点。
type executionState struct {
	ID                          string         // 执行 ID
	ProcessInstanceID           string         // 流程实例 ID
	DefinitionID                string         // 流程定义 ID
	ParentID                    string         // 父执行 ID
	ActivityID                  string         // 当前活动 ID
	Active                      bool           // 是否活跃
	Scope                       bool           // 是否作用域
	Concurrent                  bool           // 是否并发分支
	Ended                       bool           // 是否结束
	Waiting                     bool           // 是否等待外部事件
	ScopeRoot                   bool           // 是否子流程作用域根执行
	MultiInstanceRoot           bool           // 是否多实例根执行
	MIChild                     bool           // 是否多实例子执行
	MIValues                    []any          // 多实例元素值
	MISequential                bool           // 是否串行多实例
	MIActivityID                string         // 多实例活动 ID
	MIElementVariable           string         // 元素变量名
	MICompletionCondition       string         // 完成条件
	MIIndex                     int            // 子实例序号
	EventSubProcessRoot         bool           // 是否事件子流程根执行
	EventSubProcessInterrupting bool           // 事件子流程是否中断
	SkipAsyncBefore             bool           // 本次进入跳过异步前
	SkipAsyncAfter              bool           // 本次离开跳过异步后
	PendingBranches             int            // 等待的并行分支数
	CurrentHistActivityID       string         // 当前活动历史记录 ID
	Variables                   map[string]any // 局部变量
	Transient                   map[string]any `json:"-"` // 瞬态变量，仅命令内可见
}

// jobState 是定时器作业。
type jobState struct {
	ID                  string    `json:"id"`                        // 作业 ID
	Type                string    `json:"type"`                      // 作业类型：boundary-timer/async-before/async-after
	ProcessInstanceID   string    `json:"processInstanceId"`         // 流程实例 ID
	ProcessDefinitionID string    `json:"processDefinitionId"`       // 流程定义 ID（启动定时器）
	ExecutionID         string    `json:"executionId"`               // 执行 ID
	BoundaryEventID     string    `json:"boundaryEventId"`           // 边界事件 ID
	ActivityID          string    `json:"activityId"`                // 依附活动 ID
	CancelActivity      bool      `json:"cancelActivity"`            // 是否中断
	Retries             int       `json:"retries"`                   // 剩余重试次数
	RepeatRemaining     int       `json:"repeatRemaining"`           // 剩余重复次数
	RepeatInfinite      bool      `json:"repeatInfinite"`            // 是否无限重复
	IntervalMillis      int64     `json:"intervalMillis"`            // 重复间隔毫秒
	Exception           string    `json:"exception,omitempty"`       // 最近异常
	DueDate             time.Time `json:"dueDate"`                   // 到期时间
	CreatedAt           time.Time `json:"createdAt"`                 // 创建时间
	Executable          bool      `json:"executable,omitempty"`      // 是否已被移入可执行作业桶
	VariablePayload     []byte    `json:"variablePayload,omitempty"` // 明确类型编码的异步变量载荷，行模式外置到字节表
}

// subscriptionState 是消息/信号订阅。
type subscriptionState struct {
	ID                     string    `json:"id"`                     // 订阅 ID
	ProcessDefinitionID    string    `json:"processDefinitionId"`    // 流程定义 ID
	ProcessInstanceID      string    `json:"processInstanceId"`      // 流程实例 ID，启动订阅为空
	ExecutionID            string    `json:"executionId"`            // 执行 ID
	EventType              string    `json:"eventType"`              // message 或 signal
	EventName              string    `json:"eventName"`              // 事件名
	ActivityID             string    `json:"activityId"`             // 活动 ID
	BoundaryEventID        string    `json:"boundaryEventId"`        // 边界事件 ID
	GatewayGroup           string    `json:"gatewayGroup"`           // 事件网关竞争组
	EventSubProcessID      string    `json:"eventSubProcessId"`      // 事件子流程 ID
	EventSubProcessStartID string    `json:"eventSubProcessStartId"` // 事件子流程开始事件 ID
	ScopeExecutionID       string    `json:"scopeExecutionId"`       // 作用域执行 ID
	Interrupting           bool      `json:"interrupting"`           // 是否中断作用域
	StartEvent             bool      `json:"startEvent"`             // 是否启动订阅
	Condition              string    `json:"condition,omitempty"`    // 条件表达式，仅条件事件使用
	TenantID               string    `json:"tenantId"`               // 租户
	CreatedAt              time.Time `json:"createdAt"`              // 创建时间
}

// compensationState 是已登记补偿处理器。
type compensationState struct {
	ID                string    `json:"id"`                // 记录 ID
	ProcessInstanceID string    `json:"processInstanceId"` // 流程实例 ID
	ActivityID        string    `json:"activityId"`        // 已完成活动 ID
	HandlerID         string    `json:"handlerId"`         // 补偿处理器 ID
	CreatedAt         time.Time `json:"createdAt"`         // 登记时间
}

// taskState 是用户任务的运行时状态。
type taskState struct {
	Task            api.Task                       // 任务数据
	ExecutionID     string                         // 所属执行
	ActivityID      string                         // BPMN 活动 ID
	HistoricTaskID  string                         // 历史任务 ID
	CandidateUsers  []string                       // 候选用户
	CandidateGroups []string                       // 候选组
	LocalVariables  map[string]any                 // 任务局部变量
	Listeners       []model.TaskListenerDefinition // 任务监听器
}

// modelState 是流程模型及其编辑源。
type modelState struct {
	Info        api.Model // 模型元数据
	Source      []byte    // 编辑源
	SourceExtra []byte    // 扩展源码
}

// batchState 是批处理作业的运行时状态。
type batchState struct {
	Info  api.Batch                 // 批次信息
	Parts map[string]*api.BatchPart // 分片，key=分片 ID
}

// attachmentContentState 是附件二进制内容。
type attachmentContentState struct {
	Data []byte `json:"data"` // 内容
}

// userPictureState 是用户头像。
type userPictureState struct {
	Data     []byte `json:"data"`     // 图片数据
	MimeType string `json:"mimeType"` // MIME 类型
}

// state 是引擎的全部可变状态。
type state struct {
	Deployments        map[string]*deploymentState              // 部署
	Definitions        map[string]*definitionState              // 流程定义
	Models             map[string]*modelState                   // 流程模型
	Instances          map[string]*instanceState                // 流程实例
	Executions         map[string]*executionState               // 执行
	Tasks              map[string]*taskState                    // 任务
	Jobs               map[string]*jobState                     // 定时器作业
	DeadLetters        map[string]*jobState                     // 死信作业
	Subscriptions      map[string]*subscriptionState            // 事件订阅
	Compensations      map[string][]*compensationState          // 补偿登记
	Batches            map[string]*batchState                   // 批处理作业
	HistoricProcesses  map[string]*api.HistoricProcessInstance  // 历史流程实例
	HistoricActivities map[string]*api.HistoricActivityInstance // 历史活动
	HistoricTasks      map[string]*api.HistoricTaskInstance     // 历史任务
	HistoricVariables  map[string]*api.HistoricVariableInstance // 历史变量
	InstanceLinks      map[string]*api.IdentityLink             // 流程实例身份链接
	HistoricLinks      map[string]*api.IdentityLink             // 历史身份链接
	HistoricDetails    map[string]*api.HistoricDetail           // 历史变量明细
	TaskLogs           map[int64]*api.HistoricTaskLog           // 任务日志
	NextTaskLogID      int64                                    // 下一个任务日志 ID
	Comments           map[string]*api.Comment                  // 评论
	Attachments        map[string]*api.Attachment               // 附件
	AttachmentContents map[string]*attachmentContentState       // 附件内容
	Users              map[string]*api.User                     // 用户
	UserInfo           map[string]map[string]*string            // 用户信息
	UserPictures       map[string]*userPictureState             // 用户头像
	Groups             map[string]*api.Group                    // 组
	Roles              map[string]*api.Role                     // 角色
	Memberships        map[string]bool                          // 成员关系，key=user|group
	Properties         map[string]string                        // 自定义引擎属性
	Versions           map[string]int                           // 流程定义 Key 到最大版本
	// RecordSequenceFlowHistory 表示此状态由 Java 迁移续跑创建，需补记顺序流历史。
	RecordSequenceFlowHistory bool
}

// newState 创建空状态。
// @return 状态
func newState() *state {
	return &state{
		Deployments:        make(map[string]*deploymentState),
		Definitions:        make(map[string]*definitionState),
		Models:             make(map[string]*modelState),
		Instances:          make(map[string]*instanceState),
		Executions:         make(map[string]*executionState),
		Tasks:              make(map[string]*taskState),
		Jobs:               make(map[string]*jobState),
		DeadLetters:        make(map[string]*jobState),
		Subscriptions:      make(map[string]*subscriptionState),
		Compensations:      make(map[string][]*compensationState),
		Batches:            make(map[string]*batchState),
		HistoricProcesses:  make(map[string]*api.HistoricProcessInstance),
		HistoricActivities: make(map[string]*api.HistoricActivityInstance),
		HistoricTasks:      make(map[string]*api.HistoricTaskInstance),
		HistoricVariables:  make(map[string]*api.HistoricVariableInstance),
		InstanceLinks:      make(map[string]*api.IdentityLink),
		HistoricLinks:      make(map[string]*api.IdentityLink),
		HistoricDetails:    make(map[string]*api.HistoricDetail),
		TaskLogs:           make(map[int64]*api.HistoricTaskLog),
		Comments:           make(map[string]*api.Comment),
		Attachments:        make(map[string]*api.Attachment),
		AttachmentContents: make(map[string]*attachmentContentState),
		Users:              make(map[string]*api.User),
		UserInfo:           make(map[string]map[string]*string),
		UserPictures:       make(map[string]*userPictureState),
		Groups:             make(map[string]*api.Group),
		Roles:              make(map[string]*api.Role),
		Memberships:        make(map[string]bool),
		Properties:         make(map[string]string),
		Versions:           make(map[string]int),
	}
}

// cloneState 深拷贝可变状态，用于命令失败回滚。
// @param source 源状态
// @return 拷贝后的状态
func cloneState(source *state) *state {
	target := newState()
	target.RecordSequenceFlowHistory = source.RecordSequenceFlowHistory
	for id, deployment := range source.Deployments {
		copied := *deployment
		copied.Resources = cloneBytesMap(deployment.Resources)
		target.Deployments[id] = &copied
	}
	for id, definition := range source.Definitions {
		copied := *definition
		copied.StarterLinks = cloneIdentityLinks(definition.StarterLinks)
		target.Definitions[id] = &copied
	}
	for id, model := range source.Models {
		copied := *model
		if model.Source != nil {
			copied.Source = append([]byte(nil), model.Source...)
		}
		if model.SourceExtra != nil {
			copied.SourceExtra = append([]byte(nil), model.SourceExtra...)
		}
		target.Models[id] = &copied
	}
	for id, instance := range source.Instances {
		copied := *instance
		copied.EndTime = cloneTime(instance.EndTime)
		copied.DueDate = cloneTime(instance.DueDate)
		copied.ClaimTime = cloneTime(instance.ClaimTime)
		target.Instances[id] = &copied
	}
	for id, execution := range source.Executions {
		copied := *execution
		copied.Variables = cloneAnyMap(execution.Variables)
		copied.Transient = cloneAnyMap(execution.Transient)
		copied.MIValues = copyValue(execution.MIValues).([]any)
		target.Executions[id] = &copied
	}
	for id, task := range source.Tasks {
		copied := *task
		copied.Task = copyValue(task.Task).(api.Task)
		copied.CandidateUsers = append([]string(nil), task.CandidateUsers...)
		copied.CandidateGroups = append([]string(nil), task.CandidateGroups...)
		copied.LocalVariables = cloneAnyMap(task.LocalVariables)
		target.Tasks[id] = &copied
	}
	for id, job := range source.Jobs {
		copied := *job
		copied.VariablePayload = append([]byte(nil), job.VariablePayload...)
		target.Jobs[id] = &copied
	}
	for id, job := range source.DeadLetters {
		copied := *job
		copied.VariablePayload = append([]byte(nil), job.VariablePayload...)
		target.DeadLetters[id] = &copied
	}
	for id, subscription := range source.Subscriptions {
		copied := *subscription
		target.Subscriptions[id] = &copied
	}
	for instanceID, entries := range source.Compensations {
		copied := make([]*compensationState, 0, len(entries))
		for _, entry := range entries {
			item := *entry
			copied = append(copied, &item)
		}
		target.Compensations[instanceID] = copied
	}
	for id, batch := range source.Batches {
		copied := &batchState{Info: cloneBatchInfo(batch.Info), Parts: make(map[string]*api.BatchPart, len(batch.Parts))}
		for partID, part := range batch.Parts {
			item := *part
			item.EndTime = cloneTime(part.EndTime)
			copied.Parts[partID] = &item
		}
		target.Batches[id] = copied
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
	for id, historic := range source.HistoricVariables {
		copied := copyValue(*historic).(api.HistoricVariableInstance)
		target.HistoricVariables[id] = &copied
	}
	for id, link := range source.InstanceLinks {
		copied := *link
		target.InstanceLinks[id] = &copied
	}
	for id, link := range source.HistoricLinks {
		copied := *link
		target.HistoricLinks[id] = &copied
	}
	for id, detail := range source.HistoricDetails {
		copied := copyValue(*detail).(api.HistoricDetail)
		target.HistoricDetails[id] = &copied
	}
	for id, log := range source.TaskLogs {
		copied := *log
		target.TaskLogs[id] = &copied
	}
	target.NextTaskLogID = source.NextTaskLogID
	for id, user := range source.Users {
		copied := *user
		target.Users[id] = &copied
	}
	// 信息值指针逐项复制，同时保留原版清空后仍存在的键。
	for userID, info := range source.UserInfo {
		copied := make(map[string]*string, len(info))
		for key, value := range info {
			copied[key] = nil
			if value != nil {
				text := *value
				copied[key] = &text
			}
		}
		target.UserInfo[userID] = copied
	}
	for userID, picture := range source.UserPictures {
		copied := *picture
		copied.Data = append([]byte(nil), picture.Data...)
		target.UserPictures[userID] = &copied
	}
	for id, comment := range source.Comments {
		copied := *comment
		target.Comments[id] = &copied
	}
	for id, attachment := range source.Attachments {
		copied := *attachment
		target.Attachments[id] = &copied
	}
	for id, content := range source.AttachmentContents {
		target.AttachmentContents[id] = &attachmentContentState{Data: append([]byte(nil), content.Data...)}
	}
	for id, group := range source.Groups {
		copied := *group
		target.Groups[id] = &copied
	}
	for id, role := range source.Roles {
		copied := *role
		copied.Permissions = append([]string(nil), role.Permissions...)
		target.Roles[id] = &copied
	}
	for key, value := range source.Memberships {
		target.Memberships[key] = value
	}
	for name, value := range source.Properties {
		target.Properties[name] = value
	}
	for key, version := range source.Versions {
		target.Versions[key] = version
	}
	return target
}

// cloneBatchInfo 深拷贝批次信息。
// @param source 源批次
// @return 拷贝后的批次
func cloneBatchInfo(source api.Batch) api.Batch {
	copied := source
	copied.EndTime = cloneTime(source.EndTime)
	// 文档是可变字节切片，必须复制，避免读接口修改内部快照
	copied.Document = append([]byte(nil), source.Document...)
	return copied
}

// cloneAnyMap 深拷贝任意值映射。
// @param source 源映射
// @return 新映射
func cloneAnyMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	target := make(map[string]any, len(source))
	for key, value := range source {
		target[key] = copyValue(value)
	}
	return target
}

// cloneIdentityLinks 深拷贝身份链接。
// @param source 源链接
// @return 拷贝结果
func cloneIdentityLinks(source []*api.IdentityLink) []*api.IdentityLink {
	if source == nil {
		return nil
	}
	out := make([]*api.IdentityLink, 0, len(source))
	for _, link := range source {
		copied := *link
		out = append(out, &copied)
	}
	return out
}

// cloneBytesMap 深拷贝字节映射。
// @param source 源映射
// @return 拷贝结果
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
