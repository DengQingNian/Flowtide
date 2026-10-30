package workflow

import "testing"

// TestCommentTypeEnumMsgs 验证意见类型代码到中文映射与 006 规格 §2 表格逐字一致。
// 注：规格标题写"26 种"，表格实际列出 27 个代码，本测试按表格全集断言（P1 对照 Java 枚举源码复核）。
// @param t 测试上下文
func TestCommentTypeEnumMsgs(t *testing.T) {
	expect := map[string]string{
		"SP":   "审批",
		"SQ":   "授权",
		"TH":   "退回",
		"CFTG": "重复跳过",
		"CH":   "撤回",
		"XT":   "协同",
		"ZC":   "暂存",
		"LCZZ": "流程终止",
		"QS":   "签收",
		"LCCZ": "流程重置",
		"WP":   "委派",
		"LCZT": "流程暂停",
		"ZH":   "知会",
		"LCJH": "流程激活",
		"ZY":   "转阅",
		"LCSC": "流程删除",
		"YY":   "已阅",
		"BZJH": "步骤激活",
		"ZB":   "转办",
		"PS":   "评审",
		"QJQ":  "前加签",
		"TZ":   "跳转",
		"HJQ":  "后加签",
		"TJ":   "提交",
		"XTZX": "系统执行",
		"CXTJ": "重新提交",
		"SPJS": "审批结束",
	}
	if len(CommentTypeEnumMsgs) != len(expect) {
		t.Fatalf("意见类型映射应含 %d 项，实际 %d 项", len(expect), len(CommentTypeEnumMsgs))
	}
	for code, msg := range expect {
		if CommentTypeEnumMsgs[code] != msg {
			t.Errorf("意见类型 %s 应为 %q，实际 %q", code, msg, CommentTypeEnumMsgs[code])
		}
	}
	// 常量与映射键一致性抽查
	for _, pair := range [][2]string{
		{CommentTypeSP, "SP"}, {CommentTypeTH, "TH"}, {CommentTypeCH, "CH"},
		{CommentTypeLCZZ, "LCZZ"}, {CommentTypeQJQ, "QJQ"}, {CommentTypeSPJS, "SPJS"},
	} {
		if pair[0] != pair[1] {
			t.Errorf("意见类型常量值应与代码一致: %q != %q", pair[0], pair[1])
		}
	}
}

// TestGetEnumMsgByType 验证意见类型中文名查询。
// @param t 测试上下文
func TestGetEnumMsgByType(t *testing.T) {
	if got := GetEnumMsgByType("SP"); got != "审批" {
		t.Errorf("SP 应返回 审批，实际 %q", got)
	}
	if got := GetEnumMsgByType("XTZX"); got != "系统执行" {
		t.Errorf("XTZX 应返回 系统执行，实际 %q", got)
	}
	if got := GetEnumMsgByType("UNKNOWN"); got != "" {
		t.Errorf("未知类型应返回空串，实际 %q", got)
	}
}

// TestStepCodeHelpers 验证保留步骤代号判断函数。
// @param t 测试上下文
func TestStepCodeHelpers(t *testing.T) {
	// 签收步骤：4 个保留代号命中，其余不命中
	for _, code := range []string{StepCodeSign, StepCodeSign1, StepCodeSign2, StepCodeEngineerSign} {
		if !IsSignStep(code) {
			t.Errorf("IsSignStep(%q) 应为 true", code)
		}
	}
	for _, code := range []string{"end", "Draft", "sign", "Node1", ""} {
		if IsSignStep(code) {
			t.Errorf("IsSignStep(%q) 应为 false", code)
		}
	}
	if !IsEndStep(StepCodeEnd) || IsEndStep("End") || IsEndStep("") {
		t.Error("IsEndStep 仅 end 命中")
	}
	if !IsDraftStep(StepCodeDraft) || IsDraftStep("draft") || IsDraftStep("") {
		t.Error("IsDraftStep 仅 Draft 命中")
	}
}

// TestVariableKeyConstants 验证流程变量 key 常量与 006 规格 §1 全集逐字兼容。
// @param t 测试上下文
func TestVariableKeyConstants(t *testing.T) {
	expect := map[string]string{
		VarInitiator:              "initiator",
		VarBusinessKey:            "businessKey",
		VarTitle:                  "title",
		VarProcessDefinitionKey:   "processDefinitionKey",
		VarRemark:                 "remark",
		VarEntity:                 "entity",
		VarTableName:              "tableName",
		VarOperationType:          "operationType",
		VarExecutorList:           "executorList",
		VarExecutor:               "executor",
		VarNrOfInstances:          "nrOfInstances",
		VarCountersignPass:        "countersignPass",
		VarCountersignReturn:      "countersignReturn",
		VarTotalWeight:            "totalWeight",
		VarWeightThreshold:        "weightThreshold",
		VarCountersignByWeight:    "countersignByWeight",
		VarCustomExecutorList:     "customExecutorList",
		VarRandomSort:             "randomSort",
		VarSkipExpressionEnabled:  "_FLOWABLE_SKIP_EXPRESSION_ENABLED",
		VarRollbackAssigneePrefix: "ROLLBACK_ASSIGNEE_PREFIX_",
		VarRollbackTaskTypePrefix: "ROLLBACK_TASK_TYPE_PREFIX_",
		VarNrOfActiveInstances:    "nrOfActiveInstances",
		VarNrOfCompletedInstances: "nrOfCompletedInstances",
		VarLoopCounter:            "loopCounter",
		VarPass:                   "pass",
		VarResult:                 "result",
		VarSignList:               "signList",
		VarSigner:                 "signer",
	}
	for constant, literal := range expect {
		if constant != literal {
			t.Errorf("变量 key 常量应逐字兼容: %q != %q", constant, literal)
		}
	}
}

// TestProtocolConstants 验证操作类型与其余协议常量。
// @param t 测试上下文
func TestProtocolConstants(t *testing.T) {
	// 执行人重写门控只有两种合法 operationType
	if OperationTypeAgreeComplete != "agreeComplete" || OperationTypeCompleteToTarget != "completeToTarget" {
		t.Error("operationType 常量应与协议一致")
	}
	if FlowSubmitterName != "提交人" {
		t.Errorf("发起人节点显示名应为 提交人，实际 %q", FlowSubmitterName)
	}
	if AddSignScopeAfter != "after" || AddSignScopeBefore != "before" {
		t.Error("加签 scopeType 常量应与协议一致")
	}
	if SpecialGatewayBeginSuffix != "_begin" || SpecialGatewayEndSuffix != "_end" {
		t.Error("并行网关虚拟节点后缀应与协议一致")
	}
	if SuspensionStateValue != 2 || ActivateStateValue != 1 {
		t.Error("挂起/激活状态值应与协议一致")
	}
	if AutoCompleteAgreeMessage != "同意（自动办结）" || AutoCompleteReturnMessage != "退回（自动办结）" {
		t.Error("自动办结固定文案应与协议一致")
	}
}
