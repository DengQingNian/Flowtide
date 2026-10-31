// Package api — workflow.go 定义 workflow 二次封装的门面接口与线上协议类型。
//
// 本文件对位 Java 侧工作流业务 API：StartProcessInstanceVo /
// CompleteTaskVo / NextActivityRequest / R 等协议类型与 Dubbo 门面
// ProcessesService 的方法分组，全部为嵌入业务系统直接依赖的公共契约。
// 字段命名与 005 接口清单 §4、006 流程变量与意见类型约定保持逐字兼容。
package api

import (
	"context"
	"encoding/json"
)

// StartProcessInstanceVo 是发起流程实例请求。
// 对应 Java model/entity/process/StartProcessInstanceVo.java，必填字段：
// ProcessDefinitionKey、Title、Operator、TenantID、BusinessKey。
type StartProcessInstanceVo struct {
	ProcessDefinitionKey string         `json:"processDefinitionKey"` // 流程定义 Key（必填）
	Title                string         `json:"title"`                // 实例标题（必填，同时写入实例名）
	Operator             string         `json:"operator"`             // 发起人身份 Id（必填）
	TenantID             string         `json:"tenantId"`             // 租户 = 业务系统代号 systemSymbol（必填）
	BusinessKey          string         `json:"businessKey"`          // 业务单据 ID（必填）
	Remark               string         `json:"remark,omitempty"`     // 备注
	Variables            map[string]any `json:"variables,omitempty"`  // 附加流程变量（entity/tableName 等由业务方注入）
}

// BaseProcessVo 是流程办结类操作的基础请求。
// 对应 Java BaseProcessVo.java，Type 取意见类型常量（见 internal/workflow 的 CommentType* 常量）。
type BaseProcessVo struct {
	TaskID            string `json:"taskId"`            // 任务 ID（必填）
	UserCode          string `json:"userCode"`          // 操作人身份 Id（必填）
	Message           string `json:"message"`           // 审批意见内容（必填）
	ProcessInstanceID string `json:"processInstanceId"` // 流程实例 ID（必填）
	Type              string `json:"type"`              // 意见类型 CommentTypeEnum 代码（必填，如 SP/TH/CH）
}

// CompleteTaskVo 是办结任务请求，继承 BaseProcessVo。
// 对应 Java CompleteTaskVo.java；TargetActivityID 非空时语义为直送目标节点。
type CompleteTaskVo struct {
	BaseProcessVo                    // 继承基础字段
	Variables         map[string]any `json:"variables,omitempty"`         // 流程变量
	TargetActivityID  string         `json:"targetActivityId,omitempty"`  // 直送目标节点 ID（流程结束时为保留代号 end）
	CountersignResult int            `json:"countersignResult,omitempty"` // 会签结论：0 否决 / 1 通过
	// CountersignResultSet 标记 countersignResult 是否由调用方显式设置。
	// Java CompleteTaskVo 该字段为 Integer，null 时抛"未知会签任务执行结果"；
	// Go 裸 int 无法区分"未传"与"显式 0（否决）"，016 号审计引入本标记补齐语义：
	// 未设置时权重会签节点按 Java 抛 WORKFLOW-2010，而非静默记为否决并扣 rejectWeight。
	CountersignResultSet bool                 `json:"-"`
	SenderStepCode       string               `json:"senderStepCode,omitempty"` // 发送方步骤代号
	SenderStepName       string               `json:"senderStepName,omitempty"` // 发送方步骤名称
	Request              *NextActivityRequest `json:"request,omitempty"`        // 下一步预存请求（目标为 end 时可省略）
}

// completeTaskVoAlias 是 UnmarshalJSON 的别名类型，避免递归调用自身。
type completeTaskVoAlias CompleteTaskVo

// UnmarshalJSON 解析办结请求并标记 countersignResult 是否显式出现。
// 与 Java Integer 为 null 可区分的语义对齐：JSON 中缺省或显式 null 时
// CountersignResultSet 为 false，服务层据此抛出"未知会签任务执行结果"。
// @param data JSON 原始字节
// @return 解析错误
func (v *CompleteTaskVo) UnmarshalJSON(data []byte) error {
	alias := (*completeTaskVoAlias)(v)
	if err := json.Unmarshal(data, alias); err != nil {
		return err
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	raw, exists := probe["countersignResult"]
	// 显式 null 与缺省同义：均视为未设置
	v.CountersignResultSet = exists && string(raw) != "null"
	return nil
}

// NextActivityRequest 是下一步节点与处理人预存请求。
// 对应 Java model/request/NextActivityRequest.java，用于发起/办结前的"下一步预生成-消耗"模型。
type NextActivityRequest struct {
	TaskID           string            `json:"taskId"`                   // 当前任务 ID（必填）
	NextActivityList []*NextActivityVo `json:"nextActivityList"`         // 下一步节点与处理人列表（必填）
	SendIdentityID   string            `json:"sendIdentityId,omitempty"` // 发送人身份 Id
	SendUserName     string            `json:"sendUserName,omitempty"`   // 发送人姓名
}

// NextActivityVo 描述单个下一步节点及其处理人。
type NextActivityVo struct {
	ActivityID    string   `json:"activityId"`              // 目标节点 ID（流程结束填保留代号 end）
	ActivityName  string   `json:"activityName,omitempty"`  // 目标节点名称
	ExecutorIDs   []string `json:"executorIds,omitempty"`   // 处理人身份 Id 列表
	ExecutorNames []string `json:"executorNames,omitempty"` // 处理人姓名列表（展示用）
}

// BackTaskVo 是任务退回请求。
// 对应 Java BackTaskVo.java（退回入参：taskId、targetActivityId 等）。
type BackTaskVo struct {
	TaskID           string `json:"taskId"`                     // 当前任务 ID（必填）
	TargetActivityID string `json:"targetActivityId,omitempty"` // 退回目标节点 ID（空时由服务端计算可退节点）
	UserCode         string `json:"userCode"`                   // 操作人身份 Id（必填）
	Message          string `json:"message,omitempty"`          // 退回意见
	Type             string `json:"type,omitempty"`             // 意见类型，退回场景通常为 TH
}

// AddSignRequest 是前后加签请求。
// 对应 Java AddSignTaskVo（taskId/userCode/signPersoneds/flag/message）。
type AddSignRequest struct {
	TaskID    string   `json:"taskId"`              // 当前任务 Id（必填；首层加签时改造为父任务）
	Operator  string   `json:"operator"`            // 操作人（加签人身份 Id，必填）
	Assignees []string `json:"assignees"`           // 被加签人身份 Id 列表（必填，至少一人）
	ScopeType string   `json:"scopeType,omitempty"` // 作用域：后加签 "after" / 前加签 "before"（必填）
	Message   string   `json:"message,omitempty"`   // 加签意见（域服务意见留痕约束）
}

// DelSignRequest 是减签请求（对位任务级减签语义；Java 无任务级减签对位源码，
// 行为按加签的逆操作定义：删子任务 + 意见 + 剩余为空时恢复父任务）。
type DelSignRequest struct {
	ParentTaskID string `json:"parentTaskId"`      // 父任务 Id（必填）
	SubTaskID    string `json:"subTaskId"`         // 待减掉的子任务 Id（必填）
	Operator     string `json:"operator"`          // 操作人身份 Id（必填）
	Message      string `json:"message,omitempty"` // 减签意见，空时使用默认文案
}

// R 是 workflow 门面统一返回结构。
// 对应 Java api 模块 enm/R.java：code 取 "200"/"500"；出错时不填 Msg，
// 只填 ErrorMessage（兼容 Java @JsonProperty("ErrorMessage") 的大写字段名）与 Success=false。
type R struct {
	Code         string `json:"code"`                   // 状态码："200" 成功 / "500" 失败
	Msg          string `json:"msg,omitempty"`          // 成功消息
	ErrorMessage string `json:"ErrorMessage,omitempty"` // 失败消息（字段名大写 E 兼容 Java 协议）
	Data         any    `json:"data,omitempty"`         // 业务数据
	Success      bool   `json:"success"`                // 是否成功
}

// ROk 构造成功返回。
// @param data 业务数据
// @return 成功 R（code=200、success=true）
func ROk(data any) R {
	return R{Code: "200", Data: data, Success: true}
}

// ROkMsg 构造带消息的成功返回。
// @param msg 成功消息
// @param data 业务数据
// @return 成功 R（code=200、success=true）
func ROkMsg(msg string, data any) R {
	return R{Code: "200", Msg: msg, Data: data, Success: true}
}

// RErr 构造失败返回。
// @param code 状态码，通常 "500"
// @param errorMessage 失败消息
// @return 失败 R（success=false，不填 Msg）
func RErr(code, errorMessage string) R {
	return R{Code: code, ErrorMessage: errorMessage, Success: false}
}

// PageQuery 是 workflow 门面列表方法的分页参数。
// 对应 Java 侧 params Map → PageUtil 的公共分页语义。
type PageQuery struct {
	Start int `json:"start"` // 偏移量，默认 0
	Size  int `json:"size"`  // 每页数量，默认 10
}

// WorkflowFacade 是 workflow 二次封装对外门面，两路复用：
// REST /workflow/{模块}/{方法}（D8 Java 兼容路径）与嵌入式 api.Engine 使用者。
// 对应 Java Dubbo 门面 ProcessesService（方法按 005 §2.2 分组裁剪：排除桩端点与 Dubbo 专属）。
// 全部方法返回协议层 R + Go 侧 error；未实现阶段统一返回 WORKFLOW-0501 错误。
type WorkflowFacade interface {
	// ── 发起 ──

	// StartProcess 发起流程实例（注入 initiator/businessKey/title 等协议变量）。
	// @param ctx 上下文
	// @param vo 发起请求
	// @return 统一返回（data 含 processInstanceId）
	// @return 错误
	StartProcess(ctx context.Context, vo *StartProcessInstanceVo) (R, error)
	// StartProcessAndFirstStep 发起流程并预存首步下一步信息。
	// @param ctx 上下文
	// @param vo 发起请求
	// @return 统一返回
	// @return 错误
	StartProcessAndFirstStep(ctx context.Context, vo *StartProcessInstanceVo) (R, error)

	// ── 办结/退回/撤回/直送/重置 ──

	// Complete 办结任务（写意见、表单状态，按 request 预存下一步）。
	// @param ctx 上下文
	// @param vo 办结请求
	// @return 统一返回
	// @return 错误
	Complete(ctx context.Context, vo *CompleteTaskVo) (R, error)
	// BackToStepTask 退回任务到指定步骤（任意节点退回）。
	// @param ctx 上下文
	// @param vo 退回请求
	// @return 统一返回
	// @return 错误
	BackToStepTask(ctx context.Context, vo *BackTaskVo) (R, error)
	// BackTask 退回任务到上一节点（仅串行流程）。
	// @param ctx 上下文
	// @param vo 退回请求
	// @return 统一返回
	// @return 错误
	BackTask(ctx context.Context, vo *BackTaskVo) (R, error)
	// DoRollbackTask 撤回任务（发起人/特权人撤回已办任务，策略族执行）。
	// @param ctx 上下文
	// @param processInstanceID 流程实例 ID
	// @param taskID 待撤回任务 ID
	// @param operator 操作人身份 Id
	// @return 统一返回
	// @return 错误
	DoRollbackTask(ctx context.Context, processInstanceID, taskID, operator string) (R, error)
	// CompleteToTargetActivity 直送：办结并提交至指定目标节点。
	// @param ctx 上下文
	// @param vo 办结请求（TargetActivityID 必填）
	// @return 统一返回
	// @return 错误
	CompleteToTargetActivity(ctx context.Context, vo *CompleteTaskVo) (R, error)
	// DoResetProcess 重置流程实例（清空运行态回到发起节点）。
	// @param ctx 上下文
	// @param processInstanceID 流程实例 ID
	// @param operator 操作人身份 Id
	// @return 统一返回
	// @return 错误
	DoResetProcess(ctx context.Context, processInstanceID, operator string) (R, error)

	// ── 待办/已办 ──

	// GetTodoTaskListByIdentityID 按身份 Id 分页查询待办任务。
	// @param ctx 上下文
	// @param identityID 身份 Id
	// @param query 分页参数
	// @return 统一返回（data 为分页任务列表）
	// @return 错误
	GetTodoTaskListByIdentityID(ctx context.Context, identityID string, query PageQuery) (R, error)
	// GetDoneTaskListByIdentityID 按身份 Id 分页查询已办任务。
	// @param ctx 上下文
	// @param identityID 身份 Id
	// @param query 分页参数
	// @return 统一返回（data 为分页任务列表）
	// @return 错误
	GetDoneTaskListByIdentityID(ctx context.Context, identityID string, query PageQuery) (R, error)

	// ── 下一步推演与预存 ──

	// GetNextUserTaskAndUserList 推演当前任务之后的节点与候选处理人。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @return 统一返回（data 为节点与用户列表）
	// @return 错误
	GetNextUserTaskAndUserList(ctx context.Context, taskID string) (R, error)
	// SaveAgreeNextActivity 预存"同意"方向的下一步节点与处理人（目标为 end 时跳过）。
	// @param ctx 上下文
	// @param req 预存请求
	// @return 统一返回
	// @return 错误
	SaveAgreeNextActivity(ctx context.Context, req *NextActivityRequest) (R, error)

	// ── 状态/变量 ──

	// IsProcessDeployed 判断流程定义是否已部署。
	// @param ctx 上下文
	// @param tenantID 租户（业务系统代号）
	// @param processDefinitionKey 流程定义 Key
	// @return 统一返回（data 为 bool）
	// @return 错误
	IsProcessDeployed(ctx context.Context, tenantID, processDefinitionKey string) (R, error)
	// IsExistRunningProcess 判断业务键下是否存在运行中实例。
	// @param ctx 上下文
	// @param tenantID 租户（业务系统代号）
	// @param processDefinitionKey 流程定义 Key
	// @param businessKey 业务单据 ID
	// @return 统一返回（data 为 bool）
	// @return 错误
	IsExistRunningProcess(ctx context.Context, tenantID, processDefinitionKey, businessKey string) (R, error)
	// GetWorkflowStatusByBusinessID 查询业务单据的流程状态。
	// @param ctx 上下文
	// @param tenantID 租户（业务系统代号）
	// @param processDefinitionKey 流程定义 Key
	// @param businessKey 业务单据 ID
	// @return 统一返回（data 为状态描述）
	// @return 错误
	GetWorkflowStatusByBusinessID(ctx context.Context, tenantID, processDefinitionKey, businessKey string) (R, error)
	// UpdateProcessGlobalVariable 更新流程实例全局变量。
	// @param ctx 上下文
	// @param processInstanceID 流程实例 ID
	// @param variables 变量名到值
	// @return 统一返回
	// @return 错误
	UpdateProcessGlobalVariable(ctx context.Context, processInstanceID string, variables map[string]any) (R, error)
	// GetProcessGlobalVariableByProcessInstanceID 按实例查询全局变量。
	// @param ctx 上下文
	// @param processInstanceID 流程实例 ID
	// @param names 变量名列表，nil 表示全部
	// @return 统一返回（data 为变量映射）
	// @return 错误
	GetProcessGlobalVariableByProcessInstanceID(ctx context.Context, processInstanceID string, names []string) (R, error)
	// GetProcessGlobalVariableByBusinessID 按业务键查询运行实例的全局变量。
	// @param ctx 上下文
	// @param tenantID 租户（业务系统代号）
	// @param businessKey 业务单据 ID
	// @param names 变量名列表，nil 表示全部
	// @return 统一返回（data 为变量映射）
	// @return 错误
	GetProcessGlobalVariableByBusinessID(ctx context.Context, tenantID, businessKey string, names []string) (R, error)

	// ── 表单回写 ──

	// UpdateWorkflowDocumentByProcessInstanceIDOrTaskID 按实例 ID 或任务 ID 回写业务表单状态。
	// @param ctx 上下文
	// @param processInstanceID 流程实例 ID（与 taskID 至少一个非空）
	// @param taskID 任务 ID
	// @param document 回写数据
	// @return 统一返回
	// @return 错误
	UpdateWorkflowDocumentByProcessInstanceIDOrTaskID(ctx context.Context, processInstanceID, taskID string, document map[string]any) (R, error)

	// ── 调整处理人 ──

	// DoTaskOwnerAdjustment 按节点调整运行中任务的处理人。
	// @param ctx 上下文
	// @param processInstanceID 流程实例 ID
	// @param taskKey 目标节点 Key
	// @param fromOwner 原处理人，空表示不限
	// @param toOwner 新处理人
	// @param operator 操作人身份 Id
	// @return 统一返回
	// @return 错误
	DoTaskOwnerAdjustment(ctx context.Context, processInstanceID, taskKey, fromOwner, toOwner, operator string) (R, error)
	// DoTaskOwnerAdjustmentByID 按任务 ID 调整处理人。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @param toOwner 新处理人
	// @param operator 操作人身份 Id
	// @return 统一返回
	// @return 错误
	DoTaskOwnerAdjustmentByID(ctx context.Context, taskID, toOwner, operator string) (R, error)

	// ── 消息/历史 ──

	// GetTaskMessageListByBusinessID 按业务单据查询任务消息扩展列表。
	// @param ctx 上下文
	// @param tenantID 租户（业务系统代号）
	// @param businessKey 业务单据 ID
	// @return 统一返回（data 为消息列表）
	// @return 错误
	GetTaskMessageListByBusinessID(ctx context.Context, tenantID, businessKey string) (R, error)
	// GetTaskHistoryListByProcessInstanceID 按实例查询任务历史扩展列表。
	// @param ctx 上下文
	// @param processInstanceID 流程实例 ID
	// @return 统一返回（data 为历史列表）
	// @return 错误
	GetTaskHistoryListByProcessInstanceID(ctx context.Context, processInstanceID string) (R, error)
	// GetTaskHistoryListByBusinessID 按业务单据查询任务历史扩展列表。
	// @param ctx 上下文
	// @param tenantID 租户（业务系统代号）
	// @param businessKey 业务单据 ID
	// @return 统一返回（data 为历史列表）
	// @return 错误
	GetTaskHistoryListByBusinessID(ctx context.Context, tenantID, businessKey string) (R, error)
	// GetTaskHistoryListByTaskID 按任务查询历史扩展记录。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @return 统一返回（data 为历史记录）
	// @return 错误
	GetTaskHistoryListByTaskID(ctx context.Context, taskID string) (R, error)

	// ── 任务详情 ──

	// GetTaskInfoByTaskID 按任务 ID 查询任务详情（含 workflow 扩展列）。
	// Java 方法名 geTaskInfoByTaskId（拼写保留）。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @return 统一返回（data 为任务详情）
	// @return 错误
	GetTaskInfoByTaskID(ctx context.Context, taskID string) (R, error)

	// ── 退回辅助（P2.B）──

	// GetBackNodes 查询任务的可退回节点列表（前端"退回"按钮数据源）。
	// @param ctx 上下文
	// @param taskID 当前任务 ID
	// @return 统一返回（data 为可退节点视图列表）
	// @return 错误
	GetBackNodes(ctx context.Context, taskID string) (R, error)

	// ── 软终止/挂起/重激活/删除（P2.C）──

	// DoStopProcessInstance 软终止流程实例（按实例 ID，写终态留痕）。
	// @param ctx 上下文
	// @param processInstanceID 流程实例 ID
	// @param operator 操作人身份 Id
	// @param message 终止意见
	// @return 统一返回
	// @return 错误
	DoStopProcessInstance(ctx context.Context, processInstanceID, operator, message string) (R, error)
	// DoStopTaskByTaskID 按任务软终止其所在流程实例。
	// @param ctx 上下文
	// @param taskID 任务 ID
	// @param operator 操作人身份 Id
	// @return 统一返回
	// @return 错误
	DoStopTaskByTaskID(ctx context.Context, taskID, operator string) (R, error)
	// DoDeleteProcessInstance 删除流程实例（运行态删除 + 历史留痕）。
	// @param ctx 上下文
	// @param processInstanceID 流程实例 ID
	// @param operator 操作人身份 Id
	// @param reason 删除原因
	// @return 统一返回
	// @return 错误
	DoDeleteProcessInstance(ctx context.Context, processInstanceID, operator, reason string) (R, error)
	// DoDeleteProcessByBusinessID 按业务单据删除其流程实例。
	// @param ctx 上下文
	// @param tenantID 租户（业务系统代号）
	// @param businessKey 业务单据 ID
	// @param operator 操作人身份 Id
	// @return 统一返回
	// @return 错误
	DoDeleteProcessByBusinessID(ctx context.Context, tenantID, businessKey, operator string) (R, error)
	// DoSuspendProcessInstance 挂起流程实例。
	// @param ctx 上下文
	// @param processInstanceID 流程实例 ID
	// @param operator 操作人身份 Id
	// @return 统一返回
	// @return 错误
	DoSuspendProcessInstance(ctx context.Context, processInstanceID, operator string) (R, error)
	// DoActivateProcessInstance 恢复挂起的流程实例。
	// @param ctx 上下文
	// @param processInstanceID 流程实例 ID
	// @param operator 操作人身份 Id
	// @return 统一返回
	// @return 错误
	DoActivateProcessInstance(ctx context.Context, processInstanceID, operator string) (R, error)
	// DoReactivateProcess 重激活已结束实例到指定节点（原实例复活，ID 不变）。
	// @param ctx 上下文
	// @param historicProcessInstanceID 历史实例 ID
	// @param targetActivityID 重激活目标节点 ID（空表示回开始事件后首个用户任务）
	// @param operator 操作人身份 Id
	// @return 统一返回（data 含 processInstanceId）
	// @return 错误
	DoReactivateProcess(ctx context.Context, historicProcessInstanceID, targetActivityID, operator string) (R, error)

	// ── 加签减签/会签（P3.A）──

	// AddExecution 加签：为当前任务追加办理人（前加签/后加签）。
	// @param ctx 上下文
	// @param req 加签请求
	// @return 统一返回（data 为加签结果）
	// @return 错误
	AddExecution(ctx context.Context, req AddSignRequest) (R, error)
	// DelExecution 减签：移除被加签人子任务并按需恢复父任务。
	// @param ctx 上下文
	// @param req 减签请求
	// @return 统一返回（data 为减签结果）
	// @return 错误
	DelExecution(ctx context.Context, req DelSignRequest) (R, error)
	// AddMultiInstanceMember 向多实例（会签）活动动态追加一个实例成员。
	// @param ctx 上下文
	// @param processInstanceID 流程实例 ID
	// @param activityID 多实例活动 ID
	// @param assignee 新成员身份 Id
	// @return 统一返回
	// @return 错误
	AddMultiInstanceMember(ctx context.Context, processInstanceID, activityID, assignee string) (R, error)
	// RemoveMultiInstanceMember 移除多实例（会签）活动的一个实例成员。
	// @param ctx 上下文
	// @param processInstanceID 流程实例 ID
	// @param activityID 多实例活动 ID
	// @param assignee 成员身份 Id
	// @param executionID 被移除实例的执行 ID
	// @param completed 移除时是否视作已完成
	// @return 统一返回
	// @return 错误
	RemoveMultiInstanceMember(ctx context.Context, processInstanceID, activityID, assignee, executionID string, completed bool) (R, error)
	// CompleteSign 会签比例结论：按已办比例判定会签是否通过。
	// @param ctx 上下文
	// @param taskID 会签任务 ID
	// @param countersignResult 会签结论：0 否决 / 1 通过
	// @return 统一返回（data 为是否达成比例）
	// @return 错误
	CompleteSign(ctx context.Context, taskID string, countersignResult int) (R, error)

	// ── 下一步推演（P5.B）──

	// GetNextUserTaskAndUserListByKey 未发起实例按流程定义推演（不含办结伪节点）。
	// @param ctx 上下文
	// @param tenantID 租户（业务系统代号）
	// @param processDefinitionKey 流程定义 Key（取最新版本）
	// @param businessKey 业务单据 Id（预留透传）
	// @param vars 表单/条件变量
	// @return 统一返回（data 为节点与用户视图列表）
	// @return 错误
	GetNextUserTaskAndUserListByKey(ctx context.Context, tenantID, processDefinitionKey, businessKey string, vars map[string]any) (R, error)
}
