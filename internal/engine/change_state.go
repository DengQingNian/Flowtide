package engine

import (
	"context"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// ChangeActivityState 将执行移动到新的活动（仅支持单目标活动）。
// @param ctx 上下文
// @param executionID 执行 ID
// @param activityIDs 目标活动 ID 列表
// @return 迁移错误
func (e *Engine) ChangeActivityState(ctx context.Context, executionID string, activityIDs []string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		execution, ok := e.findExecution(executionID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "执行不存在", api.ErrNotFound)
		}
		if execution.Ended {
			return nil, api.NewError("ENGINE-0660", "执行已结束", api.ErrConflict)
		}
		instance, ok := e.findInstance(execution.ProcessInstanceID)
		if !ok || instance.Ended {
			return nil, api.NewError("ENGINE-0660", "流程实例已结束", api.ErrConflict)
		}
		if err := e.allowTenant(cc.ctx, instance.TenantID); err != nil {
			return nil, err
		}
		if instance.Suspended {
			return nil, api.NewError("ENGINE-0661", "流程实例已挂起", api.ErrConflict)
		}
		if execution.ID == instance.RootExecutionID && execution.ActivityID == "" {
			// 根作用域不承载活动；单分支迁移沿用其唯一活动子执行，禁止任意选择并发分支。
			var activity *executionState
			for _, child := range e.state.Executions {
				if child.ParentID != execution.ID || child.Ended {
					continue
				}
				if activity != nil {
					return nil, api.NewError("ENGINE-0663", "实例存在多个活动分支，不能单执行迁移", api.ErrConflict)
				}
				activity = child
			}
			if activity == nil {
				return nil, api.NewError("ENGINE-0660", "实例没有可迁移的活动执行", api.ErrConflict)
			}
			execution = activity
		}
		if len(activityIDs) == 0 {
			return nil, api.NewError("ENGINE-0662", "目标活动不能为空", api.ErrBadRequest)
		}
		// 当前实现只支持单目标；多目标会引入并发作用域，留待后续扩展
		if len(activityIDs) > 1 {
			return nil, api.NewError("ENGINE-0663", "暂不支持多目标活动迁移", api.ErrBadRequest)
		}
		definition, ok := e.findDefinition(execution.DefinitionID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
		}
		process := processOfDefinition(definition)
		if process == nil {
			return nil, api.NewError("ENGINE-0501", "流程定义不包含流程", nil)
		}
		targetID := activityIDs[0]
		element, ok := findElement(process, targetID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "目标活动不存在: "+targetID, api.ErrNotFound)
		}
		if _, isNode := element.(model.FlowNode); !isNode {
			return nil, api.NewError("ENGINE-0664", "目标活动不是流程节点", api.ErrBadRequest)
		}

		// 活动容器可能仍有后代，迁移前统一清理整棵分支，不能留下原任务继续执行。
		for _, current := range e.descendantsOf(execution) {
			e.cancelExecutionTasks(cc, current.ID, "change-state")
			e.cancelBoundaryTimers(current.ID)
			e.removeSubscriptionsForExecution(current.ID)
			e.recordActivityEnd(cc, current.CurrentHistActivityID)
			current.CurrentHistActivityID = ""
			current.PendingBranches = 0
			if current.ID != execution.ID {
				current.Ended, current.Active, current.Waiting = true, false, false
			}
		}
		execution.Waiting = false
		execution.Active = true
		execution.ActivityID = targetID
		cc.agenda.schedule(&continueOperation{executionID: execution.ID, activityID: targetID})
		return nil, cc.agenda.run(cc)
	})
	return err
}

// ChangeProcessInstanceState 将实例根执行移动到新的活动。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @param activityIDs 目标活动 ID 列表
// @return 迁移错误
func (e *Engine) ChangeProcessInstanceState(ctx context.Context, processInstanceID string, activityIDs []string) error {
	if _, err := e.GetProcessInstance(ctx, processInstanceID); err != nil {
		return err
	}
	executions, err := e.ListExecutions(ctx, processInstanceID)
	if err != nil {
		return err
	}
	rootID := ""
	for _, execution := range executions {
		if execution.ParentID == "" && !execution.Ended {
			rootID = execution.ID
			break
		}
	}
	if rootID == "" {
		return api.NewError("ENGINE-0660", "实例没有可迁移的根执行", api.ErrConflict)
	}
	return e.ChangeActivityState(ctx, rootID, activityIDs)
}

// cancelExecutionTasks 取消执行上的运行中任务。
// @param cc 命令上下文
// @param executionID 执行 ID
// @param reason 取消原因
func (e *Engine) cancelExecutionTasks(cc *commandContext, executionID, reason string) {
	for id, task := range e.state.Tasks {
		if task.ExecutionID != executionID {
			continue
		}
		e.recordTaskLog(cc, task, "delete", "", reason)
		e.recordTaskEnd(cc, task)
		if historic, ok := e.state.HistoricTasks[task.HistoricTaskID]; ok {
			historic.DeleteReason = reason
		}
		delete(e.state.Tasks, id)
	}
}

// moveExecutionsToSingleActivity 将多条执行合并迁移到单一目标活动（G1）。
// 对位 Java moveExecutionsToSingleActivityId(...)：全部执行先清场，合并为一条在目标活动继续。
// 多条并行分支收拢时父作用域 PendingBranches 重算为 1，保证后续结束传播自洽（蓝图风险点 1）。
// @param cc 命令上下文
// @param executions 待迁移执行（须为同父兄弟）
// @param targetActivityID 目标活动 ID
// @param reason 任务取消原因
// @return 迁移后承载目标活动的存活执行
// @return 迁移错误
func (e *Engine) moveExecutionsToSingleActivity(cc *commandContext, executions []*executionState, targetActivityID, reason string) (*executionState, error) {
	if len(executions) == 0 {
		return nil, api.NewError("ENGINE-0685", "没有可迁移的执行", api.ErrBadRequest)
	}
	survivor := executions[0]
	e.cleanupExecutions(cc, executions, reason)
	for _, execution := range executions {
		if execution.ID != survivor.ID {
			execution.Ended, execution.Active, execution.Waiting = true, false, false
		}
	}
	survivor.Ended = false
	survivor.Active = true
	survivor.Waiting = false
	survivor.Concurrent = false
	survivor.ActivityID = targetActivityID
	// 多条并行分支收拢为一条：父作用域只等待存活执行这最后一支
	if len(executions) > 1 {
		if parent, ok := e.findExecution(survivor.ParentID); ok {
			parent.PendingBranches = 1
		}
	}
	cc.agenda.schedule(&continueOperation{executionID: survivor.ID, activityID: targetActivityID})
	return survivor, nil
}

// cleanupExecutions 清理执行及其后代的任务、定时器、订阅与活动历史，并结束后代执行。
// @param cc 命令上下文
// @param executions 待清理执行列表
// @param reason 任务取消原因
func (e *Engine) cleanupExecutions(cc *commandContext, executions []*executionState, reason string) {
	for _, execution := range executions {
		for _, current := range e.descendantsOf(execution) {
			e.cancelExecutionTasks(cc, current.ID, reason)
			e.cancelBoundaryTimers(current.ID)
			e.removeSubscriptionsForExecution(current.ID)
			e.recordActivityEnd(cc, current.CurrentHistActivityID)
			current.CurrentHistActivityID = ""
			current.PendingBranches = 0
			// 根执行本体由调用方决定存活状态，后代一律结束
			if current.ID != execution.ID {
				current.Ended, current.Active, current.Waiting = true, false, false
			}
		}
	}
}

// createJoinBranchExecutions 为汇聚网关补造"其余分支已到齐"的分支执行（G4）。
// 对位 Java createTargetInSpecialGatewayEndExecutions：为 incoming 中除已到分支外各建子执行，
// 子执行进入网关时由 joinBranches 自动扣减父作用域 PendingBranches。
// @param cc 命令上下文
// @param parent 汇聚网关所在分支的父执行
// @param gateway 汇聚网关节点
// @param skipCount 已计入的到达分支数（退回重入场景为 1）
// @return 创建错误
func (e *Engine) createJoinBranchExecutions(cc *commandContext, parent *executionState, gateway model.FlowNode, skipCount int) error {
	count := len(gateway.GetIncoming()) - skipCount
	// 已到分支覆盖全部汇聚数时无需补造
	if count <= 0 {
		return nil
	}
	for i := 0; i < count; i++ {
		child := &executionState{
			ID:                e.newID(),
			ProcessInstanceID: parent.ProcessInstanceID,
			DefinitionID:      parent.DefinitionID,
			ParentID:          parent.ID,
			ActivityID:        gateway.GetID(),
			Active:            true,
			Concurrent:        true,
			Variables:         make(map[string]any),
		}
		e.state.Executions[child.ID] = child
		cc.agenda.schedule(&continueOperation{executionID: child.ID, activityID: gateway.GetID()})
	}
	return nil
}

// moveToEndEvent 将实例的全部活动执行软终止到首个正常结束事件（G10）。
// 对位 Java doStopProcessByTaskId 的软终止语义：保留 end 事件与实例结束历史。
// @param cc 命令上下文
// @param processInstanceID 实例 ID
// @return 终止错误
func (e *Engine) moveToEndEvent(cc *commandContext, processInstanceID string) error {
	instance, ok := e.findInstance(processInstanceID)
	if !ok {
		return api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
	}
	if err := e.allowTenant(cc.ctx, instance.TenantID); err != nil {
		return err
	}
	// 挂起实例拒绝软终止
	if instance.Suspended {
		return api.NewError("ENGINE-0661", "流程实例已挂起", api.ErrConflict)
	}
	if instance.Ended {
		return api.NewError("ENGINE-0680", "流程实例已结束", api.ErrConflict)
	}
	definition, ok := e.findDefinition(instance.ProcessDefinitionID)
	if !ok {
		return api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
	}
	process := processOfDefinition(definition)
	if process == nil {
		return api.NewError("ENGINE-0501", "流程定义不包含流程", nil)
	}
	end := firstNormalEndEvent(process)
	if end == nil {
		return api.NewError("ENGINE-0687", "流程没有可用的结束事件", nil)
	}
	executions := e.topActivityExecutions(processInstanceID)
	if len(executions) == 0 {
		return api.NewError("ENGINE-0685", "实例没有可终止的活动执行", api.ErrConflict)
	}
	if _, err := e.moveExecutionsToSingleActivity(cc, executions, end.ID, "stop"); err != nil {
		return err
	}
	return cc.agenda.run(cc)
}

// firstNormalEndEvent 返回首个正常结束事件（非终止/取消/错误）；没有正常结束时退回首个结束事件。
// @param process 流程
// @return 结束事件，不存在时为 nil
func firstNormalEndEvent(process *model.Process) *model.EndEvent {
	var first *model.EndEvent
	for _, id := range process.Order {
		end, ok := process.Elements[id].(*model.EndEvent)
		if !ok {
			continue
		}
		if first == nil {
			first = end
		}
		// 首选正常结束事件，避开错误/取消/终止语义
		if !end.Terminate && !end.Cancel && end.ErrorRef == "" {
			return end
		}
	}
	return first
}
