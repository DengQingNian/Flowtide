// dict_data.go 是工作流静态字典的条目数据（D9）：
// 键 = 字典码（dict.go 常量），值 = 条目列表。条目值与 Java 协议逐字对齐处已注明来源，
// 其余按调研 007/005 语义自拟合理中文项，P6 契约测试如与线上字典冲突再逐条修正。
package workflow

// errCodeDictUnknown 是字典未知名的错误码。
const errCodeDictUnknown = "WORKFLOW-2201"

// staticDicts 是全部静态字典码表（只读，调用方经 DictService 取副本）。
var staticDicts = map[string][]DictItem{
	// 消息渠道：催办/提醒发送渠道（调研 007 §模型页"消息渠道（多选 messageType）"）
	DictCodeMessageType: {
		{Value: "inner", Label: "站内信"},
		{Value: "email", Label: "邮件"},
		{Value: "sms", Label: "短信"},
		{Value: "wechat", Label: "企业微信"},
		{Value: "dingtalk", Label: "钉钉"},
	},
	// 启用状态（workflow_model.enable、委托 enable 等 0/1 标记列）
	DictCodeEnable: {
		{Value: "1", Label: "启用"},
		{Value: "0", Label: "停用"},
	},
	// 可见性（workflow_model.visible、步骤 visible 等 0/1 标记列）
	DictCodeVisible: {
		{Value: "1", Label: "可见"},
		{Value: "0", Label: "隐藏"},
	},
	// 委托类型：值与 Java WorkflowTaskDelegateEntity.CONSIGNTYPE_* 常量逐字一致
	DictCodeConsignType: {
		{Value: "All", Label: "全部任务"},
		{Value: "SingleProject", Label: "单项目"},
		{Value: "MultiProject", Label: "多项目"},
		{Value: "AllProject", Label: "全部项目"},
		{Value: "SingleFlowCategory", Label: "单流程分类"},
		{Value: "MultiFlowCategory", Label: "多流程分类"},
		{Value: "AllFlowCategory", Label: "全部流程分类"},
	},
	// 委托条件：值即条件串前缀（006 §5：@ProjId='...' / @FlowCategory='...'）
	DictCodeEntrustCondition: {
		{Value: "@ProjId", Label: "项目条件"},
		{Value: "@FlowCategory", Label: "流程分类条件"},
	},
	// 异常类型：值与 Java WorkflowTaskExceptionEntity.EXCEPTION_TYPE_* 四类逐字一致
	DictCodeExceptionType: {
		{Value: "FlowableException", Label: "工作流异常"},
		{Value: "BusinessException", Label: "业务异常"},
		{Value: "RuntimeException", Label: "运行异常"},
		{Value: "NotTaskMessageException", Label: "未生成待办异常"},
	},
	// 提醒目标类型（workflow_remind_target.target_type；006 §5 催办占位：发起人/当前待办人）
	DictCodeTargetType: {
		{Value: "user", Label: "指定用户"},
		{Value: "group", Label: "指定组"},
		{Value: "initiator", Label: "发起人"},
		{Value: "currentHandler", Label: "当前待办人员"},
	},
	// 提醒类型（workflow_remind.enable_pre 预警 / enable_post 延时）
	DictCodeRemindType: {
		{Value: "pre", Label: "预警提醒"},
		{Value: "post", Label: "延时提醒"},
	},
	// 任务类型（调研 007 §步骤配置：taskType（UserTask/Task/ServiceTask/…），值保留 BPMN 命名）
	DictCodeTaskType: {
		{Value: "UserTask", Label: "用户任务"},
		{Value: "Task", Label: "普通任务"},
		{Value: "ServiceTask", Label: "服务任务"},
		{Value: "ReceiveTask", Label: "接收任务"},
		{Value: "ScriptTask", Label: "脚本任务"},
		{Value: "ManualTask", Label: "手工任务"},
	},
	// 流程状态（workflow_document.flow_status 语义归纳）
	DictCodeFlowStatus: {
		{Value: "draft", Label: "草稿"},
		{Value: "running", Label: "运行中"},
		{Value: "finished", Label: "已完成"},
		{Value: "terminated", Label: "已终止"},
		{Value: "suspended", Label: "已挂起"},
	},
	// 异常处理状态（workflow_task_exception.handle_status 流转，P4 接管）
	DictCodeHandleStatus: {
		{Value: "0", Label: "未处理"},
		{Value: "1", Label: "已处理"},
	},
}
