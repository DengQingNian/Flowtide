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
	// ListModels 返回全部流程模型。
	// @param ctx 上下文
	// @return 模型列表
	// @return 查询错误
	ListModels(ctx context.Context) ([]Model, error)
	// GetModel 查询流程模型。
	// @param ctx 上下文
	// @param modelID 模型 ID
	// @return 模型
	// @return 查询错误
	GetModel(ctx context.Context, modelID string) (Model, error)
	// CreateModel 创建流程模型。
	// @param ctx 上下文
	// @param req 创建请求
	// @return 模型
	// @return 创建错误
	CreateModel(ctx context.Context, req CreateModelRequest) (Model, error)
	// UpdateModel 更新流程模型。
	// @param ctx 上下文
	// @param modelID 模型 ID
	// @param update 更新内容
	// @return 更新后的模型
	// @return 更新错误
	UpdateModel(ctx context.Context, modelID string, update ModelUpdate) (Model, error)
	// DeleteModel 删除流程模型。
	// @param ctx 上下文
	// @param modelID 模型 ID
	// @return 删除错误
	DeleteModel(ctx context.Context, modelID string) error
	// GetModelSource 返回模型编辑源。
	// @param ctx 上下文
	// @param modelID 模型 ID
	// @return 源内容
	// @return 查询错误
	GetModelSource(ctx context.Context, modelID string) ([]byte, error)
	// SetModelSource 设置模型编辑源。
	// @param ctx 上下文
	// @param modelID 模型 ID
	// @param source 源内容
	// @return 设置错误
	SetModelSource(ctx context.Context, modelID string, source []byte) error
	// GetModelSourceExtra 返回模型扩展源码。
	// @param ctx 上下文
	// @param modelID 模型 ID
	// @return 扩展源
	// @return 查询错误
	GetModelSourceExtra(ctx context.Context, modelID string) ([]byte, error)
	// SetModelSourceExtra 设置模型扩展源码。
	// @param ctx 上下文
	// @param modelID 模型 ID
	// @param source 扩展源
	// @return 设置错误
	SetModelSourceExtra(ctx context.Context, modelID string, source []byte) error
	// ListDefinitionIdentityLinks 返回流程定义候选发起人。
	// @param ctx 上下文
	// @param definitionID 流程定义 ID
	// @return 链接列表
	// @return 查询错误
	ListDefinitionIdentityLinks(ctx context.Context, definitionID string) ([]IdentityLink, error)
	// GetDefinitionIdentityLink 查询单个候选发起人。
	// @param ctx 上下文
	// @param definitionID 流程定义 ID
	// @param family users 或 groups
	// @param identityID 用户或组 ID
	// @return 链接
	// @return 查询错误
	GetDefinitionIdentityLink(ctx context.Context, definitionID, family, identityID string) (IdentityLink, error)
	// AddDefinitionIdentityLink 添加候选发起人。
	// @param ctx 上下文
	// @param definitionID 流程定义 ID
	// @param link 链接
	// @return 创建后的链接
	// @return 创建错误
	AddDefinitionIdentityLink(ctx context.Context, definitionID string, link IdentityLink) (IdentityLink, error)
	// DeleteDefinitionIdentityLink 删除候选发起人。
	// @param ctx 上下文
	// @param definitionID 流程定义 ID
	// @param family users 或 groups
	// @param identityID 用户或组 ID
	// @return 删除错误
	DeleteDefinitionIdentityLink(ctx context.Context, definitionID, family, identityID string) error
}

// StartRequest 是启动流程实例的请求。
type StartRequest struct {
	ProcessDefinitionKey string     // 流程定义 Key
	ProcessDefinitionID  string     // 流程定义 ID，优先于 Key
	BusinessKey          string     // 业务键
	Name                 string     // 实例名称
	TenantID             string     // 租户
	UserID               string     // 发起人，用于参与者身份链接
	Variables            []Variable // 启动变量
	TransientVariables   []Variable // 瞬态变量，仅启动命令内可见
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
	// GetExecutionVariables 返回执行可见变量。
	// @param ctx 上下文
	// @param executionID 执行 ID
	// @return 变量快照
	// @return 查询错误
	GetExecutionVariables(ctx context.Context, executionID string) (map[string]any, error)
	// GetExecutionVariable 返回执行可见变量。
	// @param ctx 上下文
	// @param executionID 执行 ID
	// @param name 变量名
	// @return 变量值
	// @return 查询错误
	GetExecutionVariable(ctx context.Context, executionID, name string) (any, error)
	// SetExecutionVariable 设置执行局部变量。
	// @param ctx 上下文
	// @param executionID 执行 ID
	// @param name 变量名
	// @param value 变量值
	// @return 设置错误
	SetExecutionVariable(ctx context.Context, executionID, name string, value any) error
	// DeleteExecutionVariable 删除执行局部变量。
	// @param ctx 上下文
	// @param executionID 执行 ID
	// @param name 变量名
	// @return 删除错误
	DeleteExecutionVariable(ctx context.Context, executionID, name string) error
	// DeleteAllExecutionVariables 清空执行局部变量。
	// @param ctx 上下文
	// @param executionID 执行 ID
	// @return 删除错误
	DeleteAllExecutionVariables(ctx context.Context, executionID string) error
	// DeleteAllInstanceVariables 清空流程实例变量。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @return 删除错误
	DeleteAllInstanceVariables(ctx context.Context, processInstanceID string) error
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
	// GetVariable 返回流程实例变量。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @param name 变量名
	// @return 变量值
	// @return 查询错误
	GetVariable(ctx context.Context, processInstanceID, name string) (any, error)
	// SetVariable 设置流程实例变量。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @param name 变量名
	// @param value 变量值
	// @return 设置错误
	SetVariable(ctx context.Context, processInstanceID, name string, value any) error
	// DeleteVariable 删除流程实例变量。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @param name 变量名
	// @return 删除错误
	DeleteVariable(ctx context.Context, processInstanceID, name string) error
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
	// MessageEventReceivedForExecution 向指定执行投递消息。
	// @param ctx 上下文
	// @param executionID 执行 ID
	// @param messageName 消息名
	// @param variables 变量
	// @return 投递错误
	MessageEventReceivedForExecution(ctx context.Context, executionID, messageName string, variables []Variable) error
	// SignalEventReceivedForExecution 向指定执行投递信号。
	// @param ctx 上下文
	// @param executionID 执行 ID
	// @param signalName 信号名
	// @param variables 变量
	// @return 投递错误
	SignalEventReceivedForExecution(ctx context.Context, executionID, signalName string, variables []Variable) error
	// TriggerExecution 触发等待中的执行。
	// @param ctx 上下文
	// @param executionID 执行 ID
	// @param variables 写入变量
	// @return 触发错误
	TriggerExecution(ctx context.Context, executionID string, variables []Variable) error
	// TriggerExecutionWithTransient 触发等待中的执行并写入瞬态变量。
	// @param ctx 上下文
	// @param executionID 执行 ID
	// @param variables 持久变量
	// @param transientVariables 瞬态变量
	// @return 触发错误
	TriggerExecutionWithTransient(ctx context.Context, executionID string, variables, transientVariables []Variable) error
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
	// ValidateMigration 校验实例迁移，不修改状态。
	// @param ctx 上下文
	// @param req 迁移请求
	// @return 校验消息
	// @return 查询错误
	ValidateMigration(ctx context.Context, req MigrationRequest) ([]string, error)
	// ChangeActivityState 将执行移动到新的活动。
	// @param ctx 上下文
	// @param executionID 执行 ID
	// @param activityIDs 目标活动 ID 列表
	// @return 迁移错误
	ChangeActivityState(ctx context.Context, executionID string, activityIDs []string) error
	// ChangeProcessInstanceState 将实例根执行移动到新的活动。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @param activityIDs 目标活动 ID 列表
	// @return 迁移错误
	ChangeProcessInstanceState(ctx context.Context, processInstanceID string, activityIDs []string) error
	// ListInstanceIdentityLinks 返回流程实例身份链接。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @return 链接列表
	// @return 查询错误
	ListInstanceIdentityLinks(ctx context.Context, processInstanceID string) ([]IdentityLink, error)
	// AddInstanceIdentityLink 添加流程实例身份链接。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @param link 链接
	// @return 创建后的链接
	// @return 创建错误
	AddInstanceIdentityLink(ctx context.Context, processInstanceID string, link IdentityLink) (IdentityLink, error)
	// DeleteInstanceIdentityLink 删除流程实例身份链接。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @param family users 或 groups
	// @param identityID 用户或组 ID
	// @param linkType 链接类型
	// @return 删除错误
	DeleteInstanceIdentityLink(ctx context.Context, processInstanceID, family, identityID, linkType string) error
	// ListEventSubscriptions 返回事件订阅。
	// @param ctx 上下文
	// @param processInstanceID 流程实例 ID，空串表示全部
	// @return 订阅列表
	// @return 查询错误
	ListEventSubscriptions(ctx context.Context, processInstanceID string) ([]EventSubscription, error)
	// ListActivityInstances 返回运行中活动实例。
	// @param ctx 上下文
	// @param processInstanceID 流程实例 ID，空串表示全部
	// @return 活动实例列表
	// @return 查询错误
	ListActivityInstances(ctx context.Context, processInstanceID string) ([]ActivityInstance, error)
	// GetActiveActivityIDs 返回执行及子执行的活动 ID。
	// @param ctx 上下文
	// @param executionID 执行 ID
	// @return 活动 ID 列表
	// @return 查询错误
	GetActiveActivityIDs(ctx context.Context, executionID string) ([]string, error)
	// ListVariableInstances 返回运行时变量实例。
	// @param ctx 上下文
	// @param filter 查询条件
	// @return 变量实例列表
	// @return 查询错误
	ListVariableInstances(ctx context.Context, filter VariableInstanceFilter) ([]VariableInstance, error)
	// GetVariableInstance 查询运行时变量实例。
	// @param ctx 上下文
	// @param variableID 变量实例 ID
	// @return 变量实例
	// @return 查询错误
	GetVariableInstance(ctx context.Context, variableID string) (VariableInstance, error)
	// GetEventSubscription 查询事件订阅。
	// @param ctx 上下文
	// @param subscriptionID 订阅 ID
	// @return 订阅
	// @return 查询错误
	GetEventSubscription(ctx context.Context, subscriptionID string) (EventSubscription, error)
	// EvaluateConditions 显式求值流程实例的全部条件事件订阅。
	// @param ctx 上下文
	// @param processInstanceID 流程实例 ID
	// @return 求值错误
	EvaluateConditions(ctx context.Context, processInstanceID string) error
	// InjectUserTask 在流程实例中注入动态用户任务。
	// @param ctx 上下文
	// @param processInstanceID 流程实例 ID
	// @param req 注入请求
	// @return 任务
	// @return 注入错误
	InjectUserTask(ctx context.Context, processInstanceID string, req InjectRequest) (Task, error)
}

// MigrationRequest 是流程实例迁移请求。
type MigrationRequest struct {
	ProcessInstanceID         string            // 源实例 ID
	TargetProcessDefinitionID string            // 目标流程定义 ID
	ActivityMappings          map[string]string // 源活动 ID 到目标活动 ID
}

// CompleteTaskRequest 是完成任务请求。
type CompleteTaskRequest struct {
	UserID             string     // 操作人
	Variables          []Variable // 流程变量
	LocalVariables     []Variable // 任务局部变量
	TransientVariables []Variable // 瞬态变量，仅完成命令内可见
}

// TaskListener 是 BPMN 任务监听器。
type TaskListener func(ctx context.Context, task *Task) error

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
	// CreateTask 创建独立任务。
	// @param ctx 上下文
	// @param req 创建请求
	// @return 任务
	// @return 创建错误
	CreateTask(ctx context.Context, req CreateTaskRequest) (Task, error)
	// UpdateTask 更新任务属性。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @param update 更新内容
	// @return 更新后的任务
	// @return 更新错误
	UpdateTask(ctx context.Context, taskID string, update TaskUpdate) (Task, error)
	// DeleteTask 删除任务。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @param cascadeHistory 是否级联删除历史
	// @param deleteReason 删除原因
	// @return 删除错误
	DeleteTask(ctx context.Context, taskID string, cascadeHistory bool, deleteReason string) error
	// ListSubTasks 返回子任务。
	// @param ctx 上下文
	// @param taskID 父任务 ID
	// @return 子任务列表
	// @return 查询错误
	ListSubTasks(ctx context.Context, taskID string) ([]Task, error)
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
	// GetTaskVariable 返回任务变量。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @param name 变量名
	// @return 变量值
	// @return 查询错误
	GetTaskVariable(ctx context.Context, taskID, name string) (any, error)
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
	// DeleteAllTaskVariables 清空任务局部变量。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @return 删除错误
	DeleteAllTaskVariables(ctx context.Context, taskID string) error
	// ListTaskEvents 返回任务事件。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @return 事件列表
	// @return 查询错误
	ListTaskEvents(ctx context.Context, taskID string) ([]TaskEvent, error)
	// GetTaskEvent 查询单个任务事件。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @param eventID 事件 ID
	// @return 事件
	// @return 查询错误
	GetTaskEvent(ctx context.Context, taskID string, eventID int64) (TaskEvent, error)
	// DeleteTaskEvent 删除任务事件。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @param eventID 事件 ID
	// @return 删除错误
	DeleteTaskEvent(ctx context.Context, taskID string, eventID int64) error
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
	// Resolve 处理委派任务，将处理人恢复为所有者。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @param userID 操作人，非空时必须为所有者
	// @return 处理错误
	Resolve(ctx context.Context, taskID, userID string) error
	// AddComment 添加任务评论。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @param userID 作者
	// @param message 内容
	// @return 评论
	// @return 添加错误
	AddComment(ctx context.Context, taskID, userID, message string) (Comment, error)
	// AddProcessComment 添加流程实例评论。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @param userID 作者
	// @param message 内容
	// @return 评论
	// @return 添加错误
	AddProcessComment(ctx context.Context, processInstanceID, userID, message string) (Comment, error)
	// ListProcessComments 返回流程实例评论。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @return 评论列表
	// @return 查询错误
	ListProcessComments(ctx context.Context, processInstanceID string) ([]Comment, error)
	// ListInstanceComments 返回流程实例下的全部评论，含任务评论。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @return 评论列表
	// @return 查询错误
	ListInstanceComments(ctx context.Context, processInstanceID string) ([]Comment, error)
	// ListComments 返回任务评论。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @return 评论列表
	// @return 查询错误
	ListComments(ctx context.Context, taskID string) ([]Comment, error)
	// GetComment 查询单条评论。
	// @param ctx 上下文
	// @param commentID 评论 ID
	// @return 评论
	// @return 查询错误
	GetComment(ctx context.Context, commentID string) (Comment, error)
	// DeleteComment 删除评论。
	// @param ctx 上下文
	// @param commentID 评论 ID
	// @return 删除错误
	DeleteComment(ctx context.Context, commentID string) error
	// AddProcessAttachment 添加流程实例附件。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @param attachment 附件
	// @return 附件
	// @return 添加错误
	AddProcessAttachment(ctx context.Context, processInstanceID string, attachment Attachment) (Attachment, error)
	// ListProcessAttachments 返回流程实例附件。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @return 附件列表
	// @return 查询错误
	ListProcessAttachments(ctx context.Context, processInstanceID string) ([]Attachment, error)
	// AddAttachment 添加任务附件。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @param attachment 附件
	// @return 附件
	// @return 添加错误
	AddAttachment(ctx context.Context, taskID string, attachment Attachment) (Attachment, error)
	// DeleteAttachment 删除附件。
	// @param ctx 上下文
	// @param attachmentID 附件 ID
	// @return 删除错误
	DeleteAttachment(ctx context.Context, attachmentID string) error
	// ListAttachments 返回任务附件。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @return 附件列表
	// @return 查询错误
	ListAttachments(ctx context.Context, taskID string) ([]Attachment, error)
	// GetAttachment 查询单个附件。
	// @param ctx 上下文
	// @param attachmentID 附件 ID
	// @return 附件
	// @return 查询错误
	GetAttachment(ctx context.Context, attachmentID string) (Attachment, error)
	// SetAttachmentContent 设置附件二进制内容。
	// @param ctx 上下文
	// @param attachmentID 附件 ID
	// @param data 内容
	// @return 设置错误
	SetAttachmentContent(ctx context.Context, attachmentID string, data []byte) error
	// GetAttachmentContent 返回附件二进制内容。
	// @param ctx 上下文
	// @param attachmentID 附件 ID
	// @return 内容
	// @return 查询错误
	GetAttachmentContent(ctx context.Context, attachmentID string) ([]byte, error)
	// RegisterTaskListener 注册 BPMN 任务监听器。
	// @param name 监听器名（class/delegateExpression）
	// @param listener 监听器
	RegisterTaskListener(name string, listener TaskListener)
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
	// DeleteHistoricProcessInstance 删除历史流程实例及其关联记录。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @return 删除错误
	DeleteHistoricProcessInstance(ctx context.Context, processInstanceID string) error
	// DeleteHistoricTaskInstance 删除历史任务。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @return 删除错误
	DeleteHistoricTaskInstance(ctx context.Context, taskID string) error
	// DeleteHistoricDetail 删除历史变量明细。
	// @param ctx 上下文
	// @param detailID 明细 ID
	// @return 删除错误
	DeleteHistoricDetail(ctx context.Context, detailID string) error
	// DeleteHistoricTaskLog 删除任务日志。
	// @param ctx 上下文
	// @param logID 日志 ID
	// @return 删除错误
	DeleteHistoricTaskLog(ctx context.Context, logID int64) error
	// GetHistoricProcessInstance 查询历史流程实例。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @return 历史实例
	// @return 查询错误
	GetHistoricProcessInstance(ctx context.Context, processInstanceID string) (HistoricProcessInstance, error)
	// GetHistoricActivityInstance 查询历史活动。
	// @param ctx 上下文
	// @param activityID 历史活动 ID
	// @return 历史活动
	// @return 查询错误
	GetHistoricActivityInstance(ctx context.Context, activityID string) (HistoricActivityInstance, error)
	// GetHistoricVariableInstance 查询历史变量。
	// @param ctx 上下文
	// @param variableID 历史变量 ID
	// @return 历史变量
	// @return 查询错误
	GetHistoricVariableInstance(ctx context.Context, variableID string) (HistoricVariableInstance, error)
	// DeleteHistoricVariableInstance 删除历史变量。
	// @param ctx 上下文
	// @param variableID 历史变量 ID
	// @return 删除错误
	DeleteHistoricVariableInstance(ctx context.Context, variableID string) error
	// GetHistoricTaskInstance 查询历史任务。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @return 历史任务
	// @return 查询错误
	GetHistoricTaskInstance(ctx context.Context, taskID string) (HistoricTaskInstance, error)
	// ListHistoricTaskLogs 返回任务生命周期日志。
	// @param ctx 上下文
	// @param taskID 任务 ID，空串表示全部
	// @return 日志列表
	// @return 查询错误
	ListHistoricTaskLogs(ctx context.Context, taskID string) ([]HistoricTaskLog, error)
	// ListHistoricDetails 返回历史变量明细。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @return 明细列表
	// @return 查询错误
	ListHistoricDetails(ctx context.Context, processInstanceID string) ([]HistoricDetail, error)
	// GetHistoricDetail 查询历史变量明细。
	// @param ctx 上下文
	// @param detailID 明细 ID
	// @return 明细
	// @return 查询错误
	GetHistoricDetail(ctx context.Context, detailID string) (HistoricDetail, error)
	// GetHistoricVariableByName 按实例与变量名查询历史变量。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @param name 变量名
	// @return 历史变量
	// @return 查询错误
	GetHistoricVariableByName(ctx context.Context, processInstanceID, name string) (HistoricVariableInstance, error)
	// GetHistoricTaskVariableByName 按任务与变量名查询历史变量。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @param name 变量名
	// @return 历史变量
	// @return 查询错误
	GetHistoricTaskVariableByName(ctx context.Context, taskID, name string) (HistoricVariableInstance, error)
	// GetHistoricTaskFormData 返回历史任务表单。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @return 表单数据
	// @return 查询错误
	GetHistoricTaskFormData(ctx context.Context, taskID string) (FormData, error)
	// ListHistoricIdentityLinks 返回历史身份链接。
	// @param ctx 上下文
	// @param taskID 任务 ID，空串表示全部
	// @return 链接列表
	// @return 查询错误
	ListHistoricIdentityLinks(ctx context.Context, taskID string) ([]IdentityLink, error)
	// ListHistoricProcessIdentityLinks 返回流程实例的历史身份链接。
	// @param ctx 上下文
	// @param processInstanceID 实例 ID
	// @return 链接列表
	// @return 查询错误
	ListHistoricProcessIdentityLinks(ctx context.Context, processInstanceID string) ([]IdentityLink, error)
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
	// GetJob 查询可执行作业。
	// @param ctx 上下文
	// @param jobID 作业 ID
	// @return 作业信息
	// @return 查询错误
	GetJob(ctx context.Context, jobID string) (JobInfo, error)
	// GetDeadLetterJob 查询死信作业。
	// @param ctx 上下文
	// @param jobID 作业 ID
	// @return 作业信息
	// @return 查询错误
	GetDeadLetterJob(ctx context.Context, jobID string) (JobInfo, error)
	// SetJobRetries 设置作业重试次数。
	// @param ctx 上下文
	// @param jobID 作业 ID
	// @param retries 重试次数
	// @return 设置错误
	SetJobRetries(ctx context.Context, jobID string, retries int) error
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
	// DeleteJob 删除可执行作业。
	// @param ctx 上下文
	// @param jobID 作业 ID
	// @return 删除错误
	DeleteJob(ctx context.Context, jobID string) error
	// CreateProperty 创建自定义引擎属性。
	// @param ctx 上下文
	// @param name 名称
	// @param value 值
	// @return 创建错误
	CreateProperty(ctx context.Context, name, value string) error
	// UpdateProperty 更新自定义引擎属性。
	// @param ctx 上下文
	// @param name 名称
	// @param value 值
	// @return 更新错误
	UpdateProperty(ctx context.Context, name, value string) error
	// DeleteProperty 删除自定义引擎属性。
	// @param ctx 上下文
	// @param name 名称
	// @return 删除错误
	DeleteProperty(ctx context.Context, name string) error
	// Properties 返回引擎属性键值。
	// @param ctx 上下文
	// @return 属性映射
	// @return 查询错误
	Properties(ctx context.Context) (map[string]string, error)
	// TableCount 返回各 ACT_* 表的行数。
	// @param ctx 上下文
	// @return 表名到行数
	// @return 查询错误
	TableCount(ctx context.Context) (map[string]int64, error)
	// GetTableMetaData 返回表的列元数据。
	// @param ctx 上下文
	// @param tableName 表名
	// @return 列元数据
	// @return 查询错误
	GetTableMetaData(ctx context.Context, tableName string) (TableMetaData, error)
	// QueryTableData 分页查询表数据。
	// @param ctx 上下文
	// @param tableName 表名
	// @param start 偏移
	// @param size 每页数量
	// @param sortColumn 排序列，空串表示默认顺序
	// @param order 排序方向，asc 或 desc
	// @return 分页数据
	// @return 查询错误
	QueryTableData(ctx context.Context, tableName string, start, size int, sortColumn, order string) (Page[map[string]any], error)
	// ListBatches 返回全部批处理作业。
	// @param ctx 上下文
	// @return 批次列表
	// @return 查询错误
	ListBatches(ctx context.Context) ([]Batch, error)
	// GetBatch 查询批处理作业。
	// @param ctx 上下文
	// @param batchID 批次 ID
	// @return 批次
	// @return 查询错误
	GetBatch(ctx context.Context, batchID string) (Batch, error)
	// ListBatchParts 返回批次的分片列表。
	// @param ctx 上下文
	// @param batchID 批次 ID
	// @return 分片列表
	// @return 查询错误
	ListBatchParts(ctx context.Context, batchID string) ([]BatchPart, error)
	// GetBatchPart 查询批处理分片。
	// @param ctx 上下文
	// @param partID 分片 ID
	// @return 分片
	// @return 查询错误
	GetBatchPart(ctx context.Context, partID string) (BatchPart, error)
	// BatchMigrateProcessInstances 批量迁移流程实例并记录批次与分片。
	// @param ctx 上下文
	// @param targetDefinitionID 目标流程定义 ID
	// @param instanceIDs 流程实例 ID 列表
	// @param activityMappings 源活动 ID 到目标活动 ID
	// @return 最终批次
	// @return 迁移错误
	BatchMigrateProcessInstances(ctx context.Context, targetDefinitionID string, instanceIDs []string, activityMappings map[string]string) (Batch, error)
}
