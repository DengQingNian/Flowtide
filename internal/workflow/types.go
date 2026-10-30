// Package workflow 提供"复杂流程二次封装"能力（对标 guinsoft-base-workflow）：
// 发起/办结/退回/撤回/直送/加签/会签/委托/催办/调度/推演等门面服务。
//
// 依赖方向：本包仅依赖 api.Engine（七服务）+ api hooks + internal/persistence/workflow，
// 不 import internal/engine（引擎数据操作一律走引擎状态 API）。
package workflow

import (
	"github.com/DengQingNian/Flowtide/api"
)

// 协议类型别名：请求/响应 VO 的权威定义在 api 包（对位 Java workflow-api 模块），
// 此处提供本包内的惯用短名。
type (
	// StartProcessInstanceVo 发起流程实例请求（必填：定义 Key/标题/操作人/租户/业务键）。
	StartProcessInstanceVo = api.StartProcessInstanceVo
	// BaseProcessVo 办结类操作基础请求（任务/操作人/意见/实例/意见类型）。
	BaseProcessVo = api.BaseProcessVo
	// CompleteTaskVo 办结任务请求（含直送目标与会签结论）。
	CompleteTaskVo = api.CompleteTaskVo
	// NextActivityRequest 下一步节点与处理人预存请求。
	NextActivityRequest = api.NextActivityRequest
	// NextActivityVo 单个下一步节点及其处理人。
	NextActivityVo = api.NextActivityVo
	// BackTaskVo 任务退回请求。
	BackTaskVo = api.BackTaskVo
	// R 统一返回结构（code/msg/ErrorMessage/data/success）。
	R = api.R
	// PageQuery 列表方法分页参数。
	PageQuery = api.PageQuery
)

// PageResult 是 workflow 分页结果，字段对齐 Java PageUtil 的公共分页语义。
type PageResult[T any] struct {
	Data  []T   `json:"data"`  // 当前页数据
	Total int64 `json:"total"` // 总条数
	Start int   `json:"start"` // 偏移量
	Size  int   `json:"size"`  // 每页数量
}

// 意见类型 CommentTypeEnum 代码常量（对位 Java enm/CommentTypeEnum.java）。
// 注：006 规格标题写"26 种"，但其 §2 表格逐行列出 27 个代码；为不破坏线上协议，
// 这里按表格全集收录，多余的代码不影响既有调用方。P1 需对照 Java 枚举源码复核。
const (
	CommentTypeSP   = "SP"   // 审批
	CommentTypeSQ   = "SQ"   // 授权
	CommentTypeTH   = "TH"   // 退回
	CommentTypeCFTG = "CFTG" // 重复跳过
	CommentTypeCH   = "CH"   // 撤回
	CommentTypeXT   = "XT"   // 协同
	CommentTypeZC   = "ZC"   // 暂存
	CommentTypeLCZZ = "LCZZ" // 流程终止
	CommentTypeQS   = "QS"   // 签收
	CommentTypeLCCZ = "LCCZ" // 流程重置
	CommentTypeWP   = "WP"   // 委派
	CommentTypeLCZT = "LCZT" // 流程暂停
	CommentTypeZH   = "ZH"   // 知会
	CommentTypeLCJH = "LCJH" // 流程激活
	CommentTypeZY   = "ZY"   // 转阅
	CommentTypeLCSC = "LCSC" // 流程删除
	CommentTypeYY   = "YY"   // 已阅
	CommentTypeBZJH = "BZJH" // 步骤激活
	CommentTypeZB   = "ZB"   // 转办
	CommentTypePS   = "PS"   // 评审
	CommentTypeQJQ  = "QJQ"  // 前加签
	CommentTypeTZ   = "TZ"   // 跳转
	CommentTypeHJQ  = "HJQ"  // 后加签
	CommentTypeTJ   = "TJ"   // 提交
	CommentTypeXTZX = "XTZX" // 系统执行
	CommentTypeCXTJ = "CXTJ" // 重新提交
	CommentTypeSPJS = "SPJS" // 审批结束
)

// CommentTypeEnumMsgs 是意见类型代码到中文名称的映射（协议字典，调用方只读勿改）。
var CommentTypeEnumMsgs = map[string]string{
	CommentTypeSP:   "审批",
	CommentTypeSQ:   "授权",
	CommentTypeTH:   "退回",
	CommentTypeCFTG: "重复跳过",
	CommentTypeCH:   "撤回",
	CommentTypeXT:   "协同",
	CommentTypeZC:   "暂存",
	CommentTypeLCZZ: "流程终止",
	CommentTypeQS:   "签收",
	CommentTypeLCCZ: "流程重置",
	CommentTypeWP:   "委派",
	CommentTypeLCZT: "流程暂停",
	CommentTypeZH:   "知会",
	CommentTypeLCJH: "流程激活",
	CommentTypeZY:   "转阅",
	CommentTypeLCSC: "流程删除",
	CommentTypeYY:   "已阅",
	CommentTypeBZJH: "步骤激活",
	CommentTypeZB:   "转办",
	CommentTypePS:   "评审",
	CommentTypeQJQ:  "前加签",
	CommentTypeTZ:   "跳转",
	CommentTypeHJQ:  "后加签",
	CommentTypeTJ:   "提交",
	CommentTypeXTZX: "系统执行",
	CommentTypeCXTJ: "重新提交",
	CommentTypeSPJS: "审批结束",
}

// GetEnumMsgByType 返回意见类型的中文名称。
// @param commentType 意见类型代码（如 SP/TH/CH）
// @return 中文名称，未知代码返回空串
func GetEnumMsgByType(commentType string) string {
	return CommentTypeEnumMsgs[commentType]
}

// 保留步骤代号常量（006 §5 内置保留值与硬约定）。
const (
	// StepCodeEnd 流程结束代号，用户不可见不可选；直送目标为 end 时跳过预存。
	StepCodeEnd = "end"
	// StepCodeSign 签收步骤代号：出现在待办时 document 置"已完成"。
	StepCodeSign = "Sign"
	// StepCodeSign1 签收步骤代号变体 1。
	StepCodeSign1 = "Sign1"
	// StepCodeSign2 签收步骤代号变体 2。
	StepCodeSign2 = "Sign2"
	// StepCodeEngineerSign 工程师签收步骤代号。
	StepCodeEngineerSign = "EngineerSign"
	// StepCodeDraft 草稿节点代号：taskKey 为 Draft 时 document 记流程开始时间。
	StepCodeDraft = "Draft"
)

// IsSignStep 判断步骤代号是否为签收类保留代号（Sign/Sign1/Sign2/EngineerSign）。
// @param stepCode 步骤代号
// @return 是否签收步骤
func IsSignStep(stepCode string) bool {
	switch stepCode {
	case StepCodeSign, StepCodeSign1, StepCodeSign2, StepCodeEngineerSign:
		return true
	default:
		return false
	}
}

// IsEndStep 判断步骤代号是否为流程结束代号（end）。
// @param stepCode 步骤代号
// @return 是否结束代号
func IsEndStep(stepCode string) bool {
	return stepCode == StepCodeEnd
}

// IsDraftStep 判断步骤代号是否为草稿节点代号（Draft）。
// @param stepCode 步骤代号
// @return 是否草稿代号
func IsDraftStep(stepCode string) bool {
	return stepCode == StepCodeDraft
}

// 流程变量 key 常量（006 §1 变量 key 全集，线上协议逐字兼容）。
const (
	// VarInitiator 发起人身份 Id；发起时注入 =operator，BPMN 中 ${initiator} 即发起人节点。
	VarInitiator = "initiator"
	// VarBusinessKey 业务单据 ID；发起时注入（与引擎 businessKey 冗余存储）。
	VarBusinessKey = "businessKey"
	// VarTitle 实例标题；发起时注入并同步 setProcessInstanceName。
	VarTitle = "title"
	// VarProcessDefinitionKey 流程定义 Key；发起时注入。
	VarProcessDefinitionKey = "processDefinitionKey"
	// VarRemark 备注；发起时注入。
	VarRemark = "remark"
	// VarEntity 业务表单数据（Map 型），驱动条件连线求值与动态角色。
	VarEntity = "entity"
	// VarTableName 业务表名（document.document_table）。
	VarTableName = "tableName"
	// VarOperationType 运行时操作类型；执行人重写门控（只有两种合法值触发回填）。
	VarOperationType = "operationType"
	// VarExecutorList 会签集合变量（countersignCollectionConfig 写入）。
	VarExecutorList = "executorList"
	// VarExecutor 单人执行表达式变量（子流程/会签、撤回策略用）。
	VarExecutor = "executor"
	// VarNrOfInstances 多实例循环总次数（countersignCollectionConfig 强制写入）。
	VarNrOfInstances = "nrOfInstances"
	// VarCountersignPass 会签通过计数（初始化 0，比例退回时累计）。
	VarCountersignPass = "countersignPass"
	// VarCountersignReturn 会签退回计数（初始化 0，比例退回时累计）。
	VarCountersignReturn = "countersignReturn"
	// VarTotalWeight 权重会签累计权重（办结时按 Arith 累计）。
	VarTotalWeight = "totalWeight"
	// VarWeightThreshold 权重会签通过阈值。
	VarWeightThreshold = "weightThreshold"
	// VarCountersignByWeight 权重会签开关标记。
	VarCountersignByWeight = "countersignByWeight"
	// VarCustomExecutorList 业务方指定会签人集合（优先级最高）。
	VarCustomExecutorList = "customExecutorList"
	// VarRandomSort 会签人随机排序开关。
	VarRandomSort = "randomSort"
	// VarSkipExpressionEnabled Flowable 官方跳过表达式开关（配合 skipExpression 自动跳过节点）。
	VarSkipExpressionEnabled = "_FLOWABLE_SKIP_EXPRESSION_ENABLED"
	// VarRollbackAssigneePrefix 撤回后目标执行人变量前缀：
	// 完整变量名 = 前缀 + {procInstId}{taskKey}（RollbackEventListener 读取）。
	VarRollbackAssigneePrefix = "ROLLBACK_ASSIGNEE_PREFIX_"
	// VarRollbackTaskTypePrefix 撤回策略类名变量前缀：
	// 完整变量名 = 前缀 + {procInstId}{taskKey}。
	VarRollbackTaskTypePrefix = "ROLLBACK_TASK_TYPE_PREFIX_"
	// VarNrOfActiveInstances 多实例当前活跃数（Flowable 内置，撤回策略手工修正）。
	VarNrOfActiveInstances = "nrOfActiveInstances"
	// VarNrOfCompletedInstances 多实例已完成数（Flowable 内置，撤回策略手工修正）。
	VarNrOfCompletedInstances = "nrOfCompletedInstances"
	// VarLoopCounter 多实例循环下标（Flowable 内置，撤回策略手工修正）。
	VarLoopCounter = "loopCounter"
	// VarPass 会签演示模板通过标记（仅演示语料 countersign.xml）。
	VarPass = "pass"
	// VarResult 会签演示模板结论标记（仅演示语料 countersign.xml）。
	VarResult = "result"
	// VarSignList 会签演示模板集合变量（仅演示语料 countersign.xml）。
	VarSignList = "signList"
	// VarSigner 会签演示模板元素变量（仅演示语料 countersign.xml）。
	VarSigner = "signer"
)

// 操作类型与其他协议常量（对位 Java constants/FlowConstant.java 与 ProcessesUtil）。
const (
	// OperationTypeAgreeComplete 办结写入的合法操作类型：同意办结（执行人重写门控）。
	OperationTypeAgreeComplete = "agreeComplete"
	// OperationTypeCompleteToTarget 办结写入的合法操作类型：直送目标节点（执行人重写门控）。
	OperationTypeCompleteToTarget = "completeToTarget"
	// FlowSubmitterName 发起人节点显示名（FLOW_SUBMITTER）。
	FlowSubmitterName = "提交人"
	// AddSignScopeAfter 后加签 scopeType（AFTER_ADDSIGN）。
	AddSignScopeAfter = "after"
	// AddSignScopeBefore 前加签 scopeType（BEFORE_ADDSIGN）。
	AddSignScopeBefore = "before"
	// SpecialGatewayBeginSuffix 并行网关成对虚拟节点开始后缀。
	SpecialGatewayBeginSuffix = "_begin"
	// SpecialGatewayEndSuffix 并行网关成对虚拟节点结束后缀。
	SpecialGatewayEndSuffix = "_end"
	// SuspensionStateValue 挂起状态值（SUSPENSION_STATE）。
	SuspensionStateValue = 2
	// ActivateStateValue 激活状态值（ACTIVATE_STATE）。
	ActivateStateValue = 1
)

// 自动办结固定审批文案（006 §2，意见类型 SP/TH）。
const (
	// AutoCompleteAgreeMessage 自动办结同意文案。
	AutoCompleteAgreeMessage = "同意（自动办结）"
	// AutoCompleteReturnMessage 自动办结退回文案。
	AutoCompleteReturnMessage = "退回（自动办结）"
)
