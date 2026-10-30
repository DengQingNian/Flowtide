package workflow

import "time"

// BaseColumns 是 workflow_* 表的公共审计列（003 文档 §2）：
// id varchar(64) 主键由调用方（internal/idgen）生成传入；
// create_time/update_time 由仓储统一填充，调用方显式提供时尊重原值。
type BaseColumns struct {
	ID          string    // 主键
	Creator     string    // 创建人身份 Id
	CreatorName string    // 创建人姓名
	CreateTime  time.Time // 创建时间
	Updater     string    // 更新人身份 Id
	UpdateTime  time.Time // 更新时间
	TenantID    string    // 租户 = 业务系统代号 systemSymbol
}

// ExtendFields 是通用扩展列（extend_s1..s8/i1..i3/f1..f3/d1..d3），
// 与 Java 实体 Extend 注解字段一致：业务系统透传，workflow 层不解释其含义。
// 指针类型区分"未设置"与"零值"，nil 不落库。
type ExtendFields struct {
	ExtendS1 string     // 扩展串 1（Java 会签场景存 projectId，见 P4 委托）
	ExtendS2 string     // 扩展串 2
	ExtendS3 string     // 扩展串 3
	ExtendS4 string     // 扩展串 4
	ExtendS5 string     // 扩展串 5
	ExtendS6 string     // 扩展串 6
	ExtendS7 string     // 扩展串 7
	ExtendS8 string     // 扩展串 8
	ExtendI1 *int64     // 扩展整型 1
	ExtendI2 *int64     // 扩展整型 2
	ExtendI3 *int64     // 扩展整型 3
	ExtendF1 *float64   // 扩展浮点 1
	ExtendF2 *float64   // 扩展浮点 2
	ExtendF3 *float64   // 扩展浮点 3
	ExtendD1 *time.Time // 扩展时间 1
	ExtendD2 *time.Time // 扩展时间 2
	ExtendD3 *time.Time // 扩展时间 3
}

// NextActivity 是 workflow_next_activity / workflow_his_next_activity 的领域实体
// （两表列完全一致，His 双写共用同一结构）。
// "预生成-消耗"模型：写操作预生成 used=0 记录，任务创建时执行人重写钩子
// 置 used=1 并回写 task_id；首步（saveFirstTaskUser）直接写 used=1 + taskId。
type NextActivity struct {
	BaseColumns                // 公共审计列
	NextActivityID    string   // 目标节点 ID（BPMN userTask id；流程结束为保留代号 end，end 不落库由调用方跳过）
	GroupID           string   // 候选组 Id（同一候选组的多条记录共享，供组任务聚合）
	IdentityID        string   // 处理人身份 Id
	SendIdentityID    string   // 发送人身份 Id
	SendUserName      string   // 发送人姓名
	ProcessInstanceID string   // 流程实例 Id
	AgreeWeight       *float64 // 同意权重（权重会签）
	RejectWeight      *float64 // 退回权重（权重会签）
	Used              int      // 消耗标记：0 未消耗（待执行人重写）、1 已消耗
	PreTaskID         string   // 来源任务 Id（产生该预生成的任务）
	TaskID            string   // 消耗后回写的引擎任务 Id（ACT_RU_TASK.ID_）
	Version           int      // 版本号：同实例同节点每预生成一轮 +1（按 his 表 used=1 记录的最大版本推算）
}

// MessageExtend 是 workflow_task_message_extend（待办扩展，待办中心唯一数据源）的领域实体，
// 与 ACT_RU_TASK 1:1（task_id 唯一）。
type MessageExtend struct {
	BaseColumns                     // 公共审计列
	TaskID               string     // 引擎任务 Id（唯一）
	ProcessInstanceID    string     // 流程实例 Id
	Title                string     // 待办标题 = 实例名称
	Message              string     // 待办说明
	BusinessID           string     // 业务单据 Id = businessKey
	TargetURL            string     // 步骤处理地址（his_step.url）
	TaskStepName         string     // 步骤名称（his_step.stepName）
	TaskStepCode         string     // 步骤代号（his_step.stepCode）
	FlowCategoryCode     string     // 流程分类代号 = 定义 Key
	FlowCategoryName     string     // 流程分类名称 = 定义名称
	OwnerID              string     // 处理人身份 Id
	OwnerName            string     // 处理人姓名
	OwnerGroup           string     // 处理人组简称
	OwnerGroupID         string     // 处理人组 Id
	OwnerGroupPaths      string     // 处理人组全称路径
	OwnerGroupPathIDs    string     // 处理人组 Id 路径
	DocumentOwnerID      string     // 起草人身份 Id（=实例发起人）
	DocumentOwnerName    string     // 起草人姓名
	DocumentGroupID      string     // 起草人组 Id
	DocumentGroup        string     // 起草人组简称
	DocumentGroupPaths   string     // 起草人组全称路径
	DocumentGroupPathIDs string     // 起草人组 Id 路径
	SenderID             string     // 发送人身份 Id（=预生成记录 send_identity_id）
	Sender               string     // 发送人姓名（=send_user_name）
	SenderStepName       string     // 发送步骤名称（来源任务名）
	SenderStepCode       string     // 发送步骤代号（来源任务 Key）
	ConsignerID          string     // 受托人身份 Id（委托场景）
	Consigner            string     // 受托人姓名
	IsRead               int        // 是否已读：0 否、1 是
	ReadTime             *time.Time // 阅读时间
	PlanFinishTime       *time.Time // 计划办结时间
	TaskReceiptTime      *time.Time // 签收时间 = 任务创建时间
	TaskStartTime        *time.Time // 开始处理时间
	IsDelay              int        // 是否延时：0 否、1 是
	DelayTime            *time.Time // 延时时间点
	MessageGroupID       string     // 消息组 Id（同组待办联动删除）
	ExtendFields                    // 通用扩展列透传
}

// HistoryExtend 是 workflow_task_history_extend（已办扩展，ACT_HI_TASKINST 1:1）的领域实体。
// Java 侧由待办扩展整行复制生成（BeanUtils.copyProperties），故直接内嵌 MessageExtend。
type HistoryExtend struct {
	MessageExtend             // 与待办扩展同构
	TaskFinishTime *time.Time // 实际办结时间
}

// OperationRecord 是 workflow_operation_record（操作记录/意见留痕）的领域实体。
// type 取意见类型 CommentTypeEnum 代码；"可签名记录"只取 TJ/SP/TZ 三类。
type OperationRecord struct {
	BaseColumns                  // 公共审计列
	BusinessID        string     // 业务单据 Id
	TaskID            string     // 任务 Id
	ProcessInstanceID string     // 流程实例 Id
	Message           string     // 操作说明（审批意见内容）
	IsAgree           *int       // 是否同意：1 同意、0 不同意，nil 表示不适用
	TaskStepCode      string     // 步骤代号（=任务 Key）
	TaskStepName      string     // 步骤名称（=任务名）
	FlowCategoryCode  string     // 流程分类代号 = 定义 Key
	FlowCategoryName  string     // 流程分类名称 = 定义名称
	OwnerID           string     // 处理人身份 Id
	OwnerName         string     // 处理人姓名
	OwnerGroup        string     // 处理人组简称
	OwnerGroupID      string     // 处理人组 Id
	OwnerGroupPaths   string     // 处理人组全称路径
	OwnerGroupPathIDs string     // 处理人组 Id 路径
	DealTime          *time.Time // 处理时间
	TaskReceiptTime   *time.Time // 签收时间（任务创建时间）
	TaskStartTime     *time.Time // 开始处理时间
	TaskFinishTime    *time.Time // 任务办结时间
	Type              string     // 意见类型 CommentTypeEnum 代码（如 TJ/SP/TH）
}

// Document 是 workflow_document（业务单据↔流程实例绑定与状态快照）的领域实体。
// business_id 在租户内唯一；deleted=1 为逻辑删除（手工过滤）。
// 状态机（flow_status，对齐 Java WorkflowDocumentServiceImpl）：
// 未提交 → 流转中 → 已完成/已终止；待办步骤代号含签收类（Sign*）时直接置已完成。
type Document struct {
	BaseColumns                    // 公共审计列
	BusinessID          string     // 业务单据 Id（租户内唯一）
	CreateGroupPathIDs  string     // 创建人组 Id 路径
	CreateGroupPaths    string     // 创建人组全称路径
	CreateGroupID       string     // 创建人组 Id
	CreateGroup         string     // 创建人组简称
	UpdaterName         string     // 更新人姓名
	Deleted             int        // 逻辑删除：0 正常、1 已删除
	DocumentURL         string     // 表单处理地址（=待办 target_url）
	DocumentID          string     // 表单 Id（=business_id）
	DocumentTable       string     // 业务表名（流程变量 tableName）
	FlowTitle           string     // 流程标题（=实例名称）
	FlowTaskID          string     // 当前待办任务 Id 聚合（逗号分隔去重）
	FlowInstanceID      string     // 流程实例 Id
	FlowCategoryID      string     // 流程分类代号 = 定义 Key
	FlowCategory        string     // 流程分类名称 = 定义名称
	FlowActionID        string     // 当前待办步骤代号聚合（逗号分隔去重）
	FlowAction          string     // 当前待办步骤名称聚合（逗号分隔去重）
	FlowStatus          string     // 流程状态（见 FlowStatus* 常量）
	FlowCurrentStepName string     // 当前待办步骤名称聚合（与 flow_action 同值，Java 语义）
	FlowCurrentPersons  string     // 当前待办人姓名聚合（逗号分隔去重）
	FlowStartTime       *time.Time // 流程开始时间（Draft 提交时记）
	FlowFinishTime      *time.Time // 流程完成时间（状态置已完成时记）
	FlowTerminationTime *time.Time // 流程终止时间（状态置已终止时记）
	ExtendFields                   // 通用扩展列透传
}

// GetExtendColumns 返回扩展列的列名全集（与 DDL 顺序一致，供行转换使用）。
// @return 扩展列名列表
func GetExtendColumns() []string {
	return []string{
		"extend_s1", "extend_s2", "extend_s3", "extend_s4", "extend_s5", "extend_s6", "extend_s7", "extend_s8",
		"extend_i1", "extend_i2", "extend_i3",
		"extend_f1", "extend_f2", "extend_f3",
		"extend_d1", "extend_d2", "extend_d3",
	}
}

// document flow_status 状态值常量（对齐 Java WorkflowDocumentServiceImpl 的硬编码中文状态，
// 线上协议即中文字符串，勿改为英文代号）。
const (
	// FlowStatusNotSubmitted 未提交：发起后尚未产生第一条办结记录（操作记录数 <=1）。
	FlowStatusNotSubmitted = "未提交"
	// FlowStatusRunning 流转中：存在待办且未命中结束/终止/签收条件。
	FlowStatusRunning = "流转中"
	// FlowStatusFinished 已完成：实例结束或待办步骤代号命中签收类保留代号。
	FlowStatusFinished = "已完成"
	// FlowStatusTerminated 已终止：最后一条操作记录类型为 LCZZ（流程终止）。
	FlowStatusTerminated = "已终止"
	// FlowStatusUnknown 未知状态：业务键下找不到任何实例（防御分支，Java 同名语义）。
	FlowStatusUnknown = "未知状态"
)
