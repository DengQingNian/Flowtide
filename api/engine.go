package api

import (
	"context"
	"time"

	"github.com/flowable-go/flowable/bpmn/model"
)

// Engine 是流程引擎门面，聚合全部服务。
type Engine interface {
	// RepositoryService 返回仓库服务。
	// @return 仓库服务
	RepositoryService() RepositoryService
	// RuntimeService 返回运行时服务。
	// @return 运行时服务
	RuntimeService() RuntimeService
	// TaskService 返回任务服务。
	// @return 任务服务
	TaskService() TaskService
	// HistoryService 返回历史服务。
	// @return 历史服务
	HistoryService() HistoryService
	// ManagementService 返回管理服务。
	// @return 管理服务
	ManagementService() ManagementService
	// IdentityService 返回身份服务。
	// @return 身份服务
	IdentityService() IdentityService
	// FormService 返回表单服务。
	// @return 表单服务
	FormService() FormService
	// Close 关闭引擎。
	// @param ctx 关闭上下文
	// @return 关闭错误
	Close(ctx context.Context) error
}

// RepositoryService 提供部署与流程定义查询。
type RepositoryService interface {
	// Deploy 部署 BPMN 资源。
	// @param ctx 上下文
	// @param req 部署请求
	// @return 部署信息
	// @return 部署错误
	Deploy(ctx context.Context, req DeploymentRequest) (Deployment, error)
	// ListProcessDefinitions 返回全部流程定义。
	// @param ctx 上下文
	// @return 流程定义列表
	// @return 查询错误
	ListProcessDefinitions(ctx context.Context) ([]ProcessDefinition, error)
	// GetProcessDefinition 按 ID 查询流程定义。
	// @param ctx 上下文
	// @param id 流程定义 ID
	// @return 流程定义
	// @return 查询错误
	GetProcessDefinition(ctx context.Context, id string) (ProcessDefinition, error)
	// GetProcessDefinitionByKey 按 Key 查询最新流程定义。
	// @param ctx 上下文
	// @param key 流程定义 Key
	// @return 流程定义
	// @return 查询错误
	GetProcessDefinitionByKey(ctx context.Context, key string) (ProcessDefinition, error)
	// GetBpmnModel 返回流程定义对应的 BPMN 模型。
	// @param ctx 上下文
	// @param definitionID 流程定义 ID
	// @return BPMN 模型
	// @return 查询错误
	GetBpmnModel(ctx context.Context, definitionID string) (*model.BpmnModel, error)
	// GetProcessDefinitionXML 返回流程定义原始 XML。
	// @param ctx 上下文
	// @param definitionID 流程定义 ID
	// @return XML 内容
	// @return 查询错误
	GetProcessDefinitionXML(ctx context.Context, definitionID string) ([]byte, error)
	// ListDeployments 返回全部部署。
	// @param ctx 上下文
	// @return 部署列表
	// @return 查询错误
	ListDeployments(ctx context.Context) ([]Deployment, error)
	// GetDeployment 查询部署。
	// @param ctx 上下文
	// @param deploymentID 部署 ID
	// @return 部署信息
	// @return 查询错误
	GetDeployment(ctx context.Context, deploymentID string) (Deployment, error)
	// ListDeploymentResources 返回部署资源名列表。
	// @param ctx 上下文
	// @param deploymentID 部署 ID
	// @return 资源名列表
	// @return 查询错误
	ListDeploymentResources(ctx context.Context, deploymentID string) ([]string, error)
	// GetDeploymentResource 返回部署资源内容。
	// @param ctx 上下文
	// @param deploymentID 部署 ID
	// @param resourceName 资源名
	// @return 资源内容
	// @return 查询错误
	GetDeploymentResource(ctx context.Context, deploymentID, resourceName string) ([]byte, error)
	// DeleteDeployment 删除部署。
	// @param ctx 上下文
	// @param deploymentID 部署 ID
	// @param cascade 是否级联删除流程定义
	// @return 删除错误
	DeleteDeployment(ctx context.Context, deploymentID string, cascade bool) error
	// SuspendProcessDefinition 挂起流程定义。
	// @param ctx 上下文
	// @param definitionID 流程定义 ID
	// @param includeInstances 是否同时挂起实例
	// @return 挂起错误
	SuspendProcessDefinition(ctx context.Context, definitionID string, includeInstances bool) error
	// ActivateProcessDefinition 激活流程定义。
	// @param ctx 上下文
	// @param definitionID 流程定义 ID
	// @param includeInstances 是否同时激活实例
	// @return 激活错误
	ActivateProcessDefinition(ctx context.Context, definitionID string, includeInstances bool) error
}

// StartRequest 是启动流程实例的请求。
type StartRequest struct {
	ProcessDefinitionKey string     // 流程定义 Key
	ProcessDefinitionID  string     // 流程定义 ID，优先于 Key
	BusinessKey          string     // 业务键
	Name                 string     // 实例名称
	TenantID             string     // 租户
	Variables            []Variable // 启动变量
}

// RuntimeService 提供流程实例与变量操作。
type RuntimeService interface {
	// StartProcessInstance 启动流程实例。
	// @param ctx 上下文
	// @param req 启动请求
	// @return 流程实例
	// @return 启动错误
	StartProcessInstance(ctx context.Context, req StartRequest) (ProcessInstance, error)
	// GetProcessInstance 查询流程实例。
	// @param ctx 上下文
	// @param id 实例 ID
	// @return 流程实例
	// @return 查询错误
	GetProcessInstance(ctx context.Context, id string) (ProcessInstance, error)
	// ListExecutions 返回指定实例的执行树。
	// @param ctx 上下文
	// @param processInstanceID 流程实例 ID，空串表示全部
	// @return 执行列表
	// @return 查询错误
	ListExecutions(ctx context.Context, processInstanceID string) ([]ExecutionInfo, error)
	// GetExecution 查询执行。
	// @param ctx 上下文
	// @param executionID 执行 ID
	// @return 执行信息
	// @return 查询错误
	GetExecution(ctx context.Context, executionID string) (ExecutionInfo, error)
	// ListProcessInstances 返回全部运行中实例。
	// @param ctx 上下文
	// @return 实例列表
	// @return 查询错误
	ListProcessInstances(ctx context.Context) ([]ProcessInstance, error)
	// DeleteProcessInstance 删除流程实例。
	// @param ctx 上下文
	// @param id 实例 ID
	// @param reason 删除原因
	// @return 删除错误
	DeleteProcessInstance(ctx context.Context, id, reason string) error
	// GetVariables 返回实例可见变量。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @return 变量快照
	// @return 查询错误
	GetVariables(ctx context.Context, processInstanceID string) (map[string]any, error)
	// SetVariable 设置流程实例变量。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @param name 变量名
	// @param value 变量值
	// @return 设置错误
	SetVariable(ctx context.Context, processInstanceID, name string, value any) error
	// MessageEventReceived 投递消息，触发一个匹配订阅。
	// @param ctx 上下文
	// @param messageName 消息名
	// @param variables 变量
	// @return 触发数量
	// @return 投递错误
	MessageEventReceived(ctx context.Context, messageName string, variables []Variable) (int, error)
	// SignalEventReceived 广播信号，触发全部匹配订阅。
	// @param ctx 上下文
	// @param signalName 信号名
	// @param variables 变量
	// @return 触发数量
	// @return 投递错误
	SignalEventReceived(ctx context.Context, signalName string, variables []Variable) (int, error)
	// SuspendProcessInstance 挂起流程实例。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @return 挂起错误
	SuspendProcessInstance(ctx context.Context, processInstanceID string) error
	// ActivateProcessInstance 激活流程实例。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @return 激活错误
	ActivateProcessInstance(ctx context.Context, processInstanceID string) error
	// MigrateProcessInstance 将运行中实例迁移到目标流程定义。
	// @param ctx 上下文
	// @param req 迁移请求
	// @return 迁移错误
	MigrateProcessInstance(ctx context.Context, req MigrationRequest) error
}

// MigrationRequest 是流程实例迁移请求。
type MigrationRequest struct {
	ProcessInstanceID         string            // 源实例 ID
	TargetProcessDefinitionID string            // 目标流程定义 ID
	ActivityMappings          map[string]string // 源活动 ID 到目标活动 ID
}

// CompleteTaskRequest 是完成任务请求。
type CompleteTaskRequest struct {
	UserID         string     // 操作人
	Variables      []Variable // 流程变量
	LocalVariables []Variable // 任务局部变量
}

// TaskService 提供用户任务操作。
type TaskService interface {
	// ListTasks 按条件查询任务。
	// @param ctx 上下文
	// @param filter 查询条件
	// @return 分页结果
	// @return 查询错误
	ListTasks(ctx context.Context, filter TaskFilter) (Page[Task], error)
	// GetTask 查询任务。
	// @param ctx 上下文
	// @param id 任务 ID
	// @return 任务
	// @return 查询错误
	GetTask(ctx context.Context, id string) (Task, error)
	// Claim 签收任务。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @param userID 签收人
	// @return 签收错误
	Claim(ctx context.Context, taskID, userID string) error
	// Complete 完成任务。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @param req 完成请求
	// @return 完成错误
	Complete(ctx context.Context, taskID string, req CompleteTaskRequest) error
	// GetTaskVariables 返回任务可见变量。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @return 变量快照
	// @return 查询错误
	GetTaskVariables(ctx context.Context, taskID string) (map[string]any, error)
	// SetTaskVariable 设置任务局部变量。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @param name 变量名
	// @param value 变量值
	// @return 设置错误
	SetTaskVariable(ctx context.Context, taskID, name string, value any) error
	// DeleteTaskVariable 删除任务局部变量。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @param name 变量名
	// @return 删除错误
	DeleteTaskVariable(ctx context.Context, taskID, name string) error
	// ListIdentityLinks 返回任务参与者关联。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @return 关联列表
	// @return 查询错误
	ListIdentityLinks(ctx context.Context, taskID string) ([]IdentityLink, error)
	// AddCandidateUser 添加候选用户。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @param userID 用户 ID
	// @return 添加错误
	AddCandidateUser(ctx context.Context, taskID, userID string) error
	// DeleteCandidateUser 移除候选用户。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @param userID 用户 ID
	// @return 删除错误
	DeleteCandidateUser(ctx context.Context, taskID, userID string) error
	// AddCandidateGroup 添加候选组。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @param groupID 组 ID
	// @return 添加错误
	AddCandidateGroup(ctx context.Context, taskID, groupID string) error
	// DeleteCandidateGroup 移除候选组。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @param groupID 组 ID
	// @return 删除错误
	DeleteCandidateGroup(ctx context.Context, taskID, groupID string) error
	// Delegate 委派任务。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @param userID 被委托人
	// @return 委派错误
	Delegate(ctx context.Context, taskID, userID string) error
	// AddComment 添加任务评论。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @param userID 作者
	// @param message 内容
	// @return 评论
	// @return 添加错误
	AddComment(ctx context.Context, taskID, userID, message string) (Comment, error)
	// ListComments 返回任务评论。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @return 评论列表
	// @return 查询错误
	ListComments(ctx context.Context, taskID string) ([]Comment, error)
	// DeleteComment 删除评论。
	// @param ctx 上下文
	// @param commentID 评论 ID
	// @return 删除错误
	DeleteComment(ctx context.Context, commentID string) error
	// AddAttachment 添加任务附件。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @param attachment 附件
	// @return 附件
	// @return 添加错误
	AddAttachment(ctx context.Context, taskID string, attachment Attachment) (Attachment, error)
	// ListAttachments 返回任务附件。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @return 附件列表
	// @return 查询错误
	ListAttachments(ctx context.Context, taskID string) ([]Attachment, error)
}

// HistoryService 提供历史查询。
type HistoryService interface {
	// ListHistoricProcessInstances 返回历史流程实例。
	// @param ctx 上下文
	// @return 历史实例列表
	// @return 查询错误
	ListHistoricProcessInstances(ctx context.Context) ([]HistoricProcessInstance, error)
	// ListHistoricActivityInstances 返回指定实例的历史活动。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @return 历史活动列表
	// @return 查询错误
	ListHistoricActivityInstances(ctx context.Context, processInstanceID string) ([]HistoricActivityInstance, error)
	// ListHistoricTaskInstances 返回历史任务。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @return 历史任务列表
	// @return 查询错误
	ListHistoricTaskInstances(ctx context.Context, processInstanceID string) ([]HistoricTaskInstance, error)
	// GetHistoricProcessInstance 查询历史流程实例。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @return 历史实例
	// @return 查询错误
	GetHistoricProcessInstance(ctx context.Context, processInstanceID string) (HistoricProcessInstance, error)
	// GetHistoricTaskInstance 查询历史任务。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @return 历史任务
	// @return 查询错误
	GetHistoricTaskInstance(ctx context.Context, taskID string) (HistoricTaskInstance, error)
	// ListHistoricVariableInstances 返回历史变量。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @return 历史变量列表
	// @return 查询错误
	ListHistoricVariableInstances(ctx context.Context, processInstanceID string) ([]HistoricVariableInstance, error)
}

// JobInfo 是作业信息。
type JobInfo struct {
	ID                string    `json:"id"`                          // 作业 ID
	Type              string    `json:"type,omitempty"`              // 作业类型
	ProcessInstanceID string    `json:"processInstanceId,omitempty"` // 流程实例 ID
	ExecutionID       string    `json:"executionId,omitempty"`       // 执行 ID
	ActivityID        string    `json:"activityId,omitempty"`        // 活动 ID
	Retries           int       `json:"retries"`                     // 剩余重试次数
	Exception         string    `json:"exception,omitempty"`         // 最近异常
	DueDate           time.Time `json:"dueDate"`                     // 到期时间
}

// ManagementService 提供引擎管理信息。
type ManagementService interface {
	// EngineInfo 返回引擎版本与配置摘要。
	// @param ctx 上下文
	// @return 引擎信息
	// @return 查询错误
	EngineInfo(ctx context.Context) (map[string]any, error)
	// RunDueJobs 执行全部到期定时器作业。
	// @param ctx 上下文
	// @return 执行数量
	// @return 执行错误
	RunDueJobs(ctx context.Context) (int, error)
	// ListJobs 返回可执行作业。
	// @param ctx 上下文
	// @return 作业列表
	// @return 查询错误
	ListJobs(ctx context.Context) ([]JobInfo, error)
	// ListDeadLetterJobs 返回死信作业。
	// @param ctx 上下文
	// @return 死信作业列表
	// @return 查询错误
	ListDeadLetterJobs(ctx context.Context) ([]JobInfo, error)
	// MoveDeadLetterJobToExecutable 将死信作业恢复为可执行作业。
	// @param ctx 上下文
	// @param jobID 作业 ID
	// @param retries 恢复后的重试次数
	// @return 恢复错误
	MoveDeadLetterJobToExecutable(ctx context.Context, jobID string, retries int) error
}
