package engine

import (
	"context"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// DoMoveActivity 普通节点跳转（对位 Java ProcessesUtil#moveActivity:3526-3548）。
// 语义：实例存在且未挂起 → 来源活动上的活动执行迁移到目标活动（单执行场景与 Java
// moveActivityIdTo 等价；多条并行分支收拢涉及 PendingBranches 重算，显式拒绝）；
// 迁移会取消来源分支的运行中任务并按引擎审计留痕。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @param fromActivityID 来源活动 ID
// @param toActivityID 目标活动 ID
// @return 迁移错误
func (e *Engine) DoMoveActivity(ctx context.Context, processInstanceID, fromActivityID, toActivityID string) error {
	// 前置校验走只读事务；实际迁移交 ChangeActivityState（命令事务不可嵌套）。
	var sourceID string
	if _, err := e.read(func() (any, error) {
		instance, ok := e.findInstance(processInstanceID)
		if !ok || instance.Ended {
			return nil, api.NewError("ENGINE-0660", "流程实例不存在或已结束", api.ErrConflict)
		}
		if err := e.allowTenant(ctx, instance.TenantID); err != nil {
			return nil, err
		}
		if instance.Suspended {
			// 对位 Java baseProcessService.isSuspended 的「流程已暂停，请联系管理员激活」
			return nil, api.NewError("ENGINE-0661", "流程实例已挂起", api.ErrConflict)
		}
		source := e.findActiveExecutionsByActivityIDs(processInstanceID, map[string]bool{fromActivityID: true})
		// 圈复杂度 >= 3：来源缺失与多分支拒绝是两条独立失败口径。
		if len(source) == 0 {
			return nil, api.NewError("ENGINE-0665", "来源活动没有可迁移执行: "+fromActivityID, api.ErrConflict)
		}
		if len(source) > 1 {
			return nil, api.NewError("ENGINE-0665", "来源活动存在多条并行分支，暂不支持整支收拢迁移: "+fromActivityID, api.ErrConflict)
		}
		definition, ok := e.findDefinition(source[0].DefinitionID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
		}
		element, found := findElement(processOfDefinition(definition), toActivityID)
		if !found {
			return nil, api.NewError("ENGINE-0404", "目标活动不存在: "+toActivityID, api.ErrNotFound)
		}
		if _, isNode := element.(model.FlowNode); !isNode {
			return nil, api.NewError("ENGINE-0664", "目标活动不是流程节点", api.ErrBadRequest)
		}
		sourceID = source[0].ID
		return nil, nil
	}); err != nil {
		return err
	}
	return e.ChangeActivityState(ctx, sourceID, []string{toActivityID})
}

// DoMoveExecution 执行迁移（对位 Java ProcessesUtil#moveExecution:3610-3632 moveExecutionToActivityId）。
// 无来源节点语义：把实例根执行迁移到目标活动；ChangeProcessInstanceState 已含
// 挂起拒绝/根执行定位/目标节点校验/清场续跑全链路。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @param toActivityID 目标活动 ID
// @return 迁移错误
func (e *Engine) DoMoveExecution(ctx context.Context, processInstanceID, toActivityID string) error {
	return e.ChangeProcessInstanceState(ctx, processInstanceID, []string{toActivityID})
}
