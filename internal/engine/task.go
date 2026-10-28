package engine

import (
	"context"
	"sort"
	"strings"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
	"github.com/flowable-go/flowable/internal/expr"
)

// createUserTask 创建用户任务并让执行进入等待。
// @param cc 命令上下文
// @param execution 执行
// @param node 用户任务节点
// @param process 流程
// @return 创建错误
func (e *Engine) createUserTask(cc *commandContext, execution *executionState, node *model.UserTask) error {
	vars := e.variablesOf(execution)
	assignee, err := resolveString(node.Assignee, vars)
	if err != nil {
		return api.NewError("ENGINE-0801", "任务处理人表达式错误", err)
	}
	priority := 50
	if strings.TrimSpace(node.Priority) != "" {
		parsed, parseErr := evalInt(node.Priority, vars)
		if parseErr != nil {
			return api.NewError("ENGINE-0802", "任务优先级表达式错误", parseErr)
		}
		priority = parsed
	}
	candidateUsers, err := resolveList(node.CandidateUsers, vars)
	if err != nil {
		return api.NewError("ENGINE-0803", "候选用户表达式错误", err)
	}
	candidateGroups, err := resolveList(node.CandidateGroups, vars)
	if err != nil {
		return api.NewError("ENGINE-0804", "候选组表达式错误", err)
	}
	task := &taskState{
		ExecutionID:     execution.ID,
		ActivityID:      node.ID,
		CandidateUsers:  candidateUsers,
		CandidateGroups: candidateGroups,
		LocalVariables:  make(map[string]any),
		Task: api.Task{
			ID:                  e.newID(),
			Name:                node.Name,
			Description:         node.Documentation,
			ProcessInstanceID:   execution.ProcessInstanceID,
			ProcessDefinitionID: execution.DefinitionID,
			TaskDefinitionKey:   node.ID,
			Assignee:            assignee,
			Priority:            priority,
			FormKey:             node.FormKey,
			Category:            node.Category,
			CreateTime:          cc.now,
			DueDate:             parseDueDate(node.DueDate, vars),
			CandidateUsers:      candidateUsers,
			CandidateGroups:     candidateGroups,
		},
	}
	task.HistoricTaskID = e.recordTaskStart(cc, task)
	e.state.Tasks[task.Task.ID] = task
	e.recordTaskIdentityLinks(cc, task)
	e.recordTaskLog(cc, task, "create", task.Task.Assignee, "")
	execution.Waiting = true
	execution.ActivityID = node.ID
	return nil
}

// resolveString 求值字符串表达式。
// @param raw 原始表达式
// @param vars 变量
// @return 字符串结果
// @return 错误
func resolveString(raw string, vars map[string]any) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	return expr.EvalString(raw, vars)
}

// resolveList 求值字符串列表表达式。
// @param raws 原始表达式列表
// @param vars 变量
// @return 结果列表
// @return 错误
func resolveList(raws []string, vars map[string]any) ([]string, error) {
	if len(raws) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(raws))
	for _, raw := range raws {
		value, err := resolveString(raw, vars)
		if err != nil {
			return nil, err
		}
		if value != "" {
			out = append(out, value)
		}
	}
	return out, nil
}

// GetTask 查询任务。
// @param ctx 上下文
// @param id 任务 ID
// @return 任务
// @return 查询错误
func (e *Engine) GetTask(ctx context.Context, id string) (api.Task, error) {
	result, err := e.read(func() (any, error) {
		task, ok := e.findTask(id)
		if !ok {
			return nil, api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
		}
		if err := e.allowTask(ctx, task.Task.ID); err != nil {
			return nil, err
		}
		return e.toTaskDTO(task), nil
	})
	if err != nil {
		return api.Task{}, err
	}
	return result.(api.Task), nil
}

// toTaskDTO 将任务状态转换为 DTO。
// @param task 任务状态
// @return 任务 DTO
func (e *Engine) toTaskDTO(task *taskState) api.Task {
	dto := task.Task
	dto.Variables = make(map[string]any)
	if execution, ok := e.findExecution(task.ExecutionID); ok {
		for key, value := range e.variablesOf(execution) {
			dto.Variables[key] = value
		}
	}
	for key, value := range task.LocalVariables {
		dto.Variables[key] = value
	}
	if len(dto.Variables) == 0 {
		dto.Variables = nil
	}
	return dto
}

// ListTasks 按条件查询任务。
// @param ctx 上下文
// @param filter 查询条件
// @return 分页结果
// @return 查询错误
func (e *Engine) ListTasks(ctx context.Context, filter api.TaskFilter) (api.Page[api.Task], error) {
	result, err := e.read(func() (any, error) {
		tasks := make([]api.Task, 0)
		for _, task := range e.state.Tasks {
			if !e.matchesTask(task, filter) {
				continue
			}
			tasks = append(tasks, e.toTaskDTO(task))
		}
		sort.Slice(tasks, func(i, j int) bool {
			if tasks[i].CreateTime.Equal(tasks[j].CreateTime) {
				return tasks[i].ID < tasks[j].ID
			}
			return tasks[i].CreateTime.Before(tasks[j].CreateTime)
		})
		total := int64(len(tasks))
		start, size := normalizePage(filter.Start, filter.Size)
		// 切片边界保护
		if start >= len(tasks) {
			return api.Page[api.Task]{Data: []api.Task{}, Total: total, Start: start, Size: size}, nil
		}
		end := start + size
		if end > len(tasks) {
			end = len(tasks)
		}
		return api.Page[api.Task]{Data: tasks[start:end], Total: total, Start: start, Size: size}, nil
	})
	if err != nil {
		return api.Page[api.Task]{}, err
	}
	return result.(api.Page[api.Task]), nil
}

// matchesTask 判断任务是否满足过滤条件。
// @param task 任务
// @param filter 过滤条件
// @return 是否匹配
func (e *Engine) matchesTask(task *taskState, filter api.TaskFilter) bool {
	if filter.Assignee != "" && task.Task.Assignee != filter.Assignee {
		return false
	}
	if filter.Owner != "" && task.Task.Owner != filter.Owner {
		return false
	}
	if filter.ProcessInstanceID != "" && task.Task.ProcessInstanceID != filter.ProcessInstanceID {
		return false
	}
	if filter.ParentTaskID != "" && task.Task.ParentTaskID != filter.ParentTaskID {
		return false
	}
	if filter.ProcessDefinitionKey != "" {
		instance, ok := e.state.Instances[task.Task.ProcessInstanceID]
		if !ok || instance.ProcessDefinitionKey != filter.ProcessDefinitionKey {
			return false
		}
	}
	if filter.TenantID != "" {
		tenant := task.Task.TenantID
		if instance, ok := e.findInstance(task.Task.ProcessInstanceID); ok {
			tenant = instance.TenantID
		}
		if tenant != filter.TenantID {
			return false
		}
	}
	if filter.CandidateUser != "" {
		if task.Task.Assignee != "" {
			return false
		}
		if contains(task.CandidateUsers, filter.CandidateUser) {
			return true
		}
		for _, group := range task.CandidateGroups {
			if e.state.Memberships[filter.CandidateUser+"|"+group] {
				return true
			}
		}
		return false
	}
	return true
}

// contains 判断字符串是否在列表中。
// @param values 列表
// @param target 目标
// @return 是否存在
func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// normalizePage 规范化分页参数。
// @param start 偏移
// @param size 每页数量
// @return 偏移
// @return 每页数量
func normalizePage(start, size int) (int, int) {
	if start < 0 {
		start = 0
	}
	if size <= 0 {
		size = 10
	}
	if size > 1000 {
		size = 1000
	}
	return start, size
}

// Claim 签收任务。
// @param ctx 上下文
// @param taskID 任务 ID
// @param userID 签收人
// @return 签收错误
func (e *Engine) Claim(ctx context.Context, taskID, userID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		task, ok := e.findTask(taskID)
		if !ok {
			return nil, api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
		}
		if err := e.allowTask(ctx, task.Task.ID); err != nil {
			return nil, err
		}
		instance, _ := e.findInstance(task.Task.ProcessInstanceID)
		if instance != nil && instance.Suspended {
			return nil, api.NewError("ENGINE-0810", "流程实例已挂起", api.ErrConflict)
		}
		if strings.TrimSpace(userID) == "" {
			return nil, api.NewError("ENGINE-0806", "签收人不能为空", api.ErrBadRequest)
		}
		if task.Task.Assignee != "" {
			return nil, api.NewError("ENGINE-0807", "任务已被签收", api.ErrConflict)
		}
		task.Task.Assignee = userID
		if historic, exists := e.state.HistoricTasks[task.HistoricTaskID]; exists {
			historic.Assignee = userID
		}
		e.recordTaskLog(cc, task, "claim", userID, "")
		return nil, nil
	})
	return err
}

// Complete 完成任务。
// @param ctx 上下文
// @param taskID 任务 ID
// @param req 完成请求
// @return 完成错误
func (e *Engine) Complete(ctx context.Context, taskID string, req api.CompleteTaskRequest) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		return nil, e.completeTask(cc, taskID, req)
	})
	return err
}

// completeTask 在命令内完成任务。
// @param cc 命令上下文
// @param taskID 任务 ID
// @param req 完成请求
// @return 完成错误
func (e *Engine) completeTask(cc *commandContext, taskID string, req api.CompleteTaskRequest) error {
	task, ok := e.findTask(taskID)
	if !ok {
		return api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
	}
	if err := e.allowTask(cc.ctx, task.Task.ID); err != nil {
		return err
	}
	instance, _ := e.findInstance(task.Task.ProcessInstanceID)
	if instance != nil && instance.Suspended {
		return api.NewError("ENGINE-0810", "流程实例已挂起", api.ErrConflict)
	}
	if req.UserID != "" && task.Task.Assignee != "" && task.Task.Assignee != req.UserID {
		return api.NewError("ENGINE-0808", "任务处理人不匹配", api.ErrForbidden)
	}
	// 独立任务没有执行，完成时只记录任务变量并移除任务
	if task.ExecutionID == "" {
		for _, variable := range req.Variables {
			task.LocalVariables[variable.Name] = variable.Value
			e.recordTaskHistoricVariable(task, variable.Name, variable.Value, cc.now)
		}
		for _, variable := range req.LocalVariables {
			task.LocalVariables[variable.Name] = variable.Value
			e.recordTaskHistoricVariable(task, variable.Name, variable.Value, cc.now)
		}
		e.recordTaskLog(cc, task, "complete", req.UserID, "")
		e.recordTaskEnd(cc, task)
		delete(e.state.Tasks, taskID)
		return nil
	}
	execution, ok := e.findExecution(task.ExecutionID)
	if !ok {
		return api.NewError("ENGINE-0405", "任务所属执行不存在", nil)
	}
	for _, variable := range req.Variables {
		e.setVariable(execution, variable.Name, variable.Value)
	}
	for _, variable := range req.LocalVariables {
		task.LocalVariables[variable.Name] = variable.Value
		e.recordHistoricDetail(task.Task.ProcessInstanceID, task.ExecutionID, task.Task.ID, task.ActivityID, variable.Name, variable.Value, cc.now)
	}
	e.recordTaskLog(cc, task, "complete", req.UserID, "")
	e.recordTaskEnd(cc, task)
	delete(e.state.Tasks, taskID)
	e.cancelBoundaryTimers(execution.ID)
	e.cancelBoundarySubscriptions(execution.ID)
	e.registerCompensation(cc, execution, task.ActivityID)
	execution.Waiting = false
	e.recordActivityEnd(cc, execution.CurrentHistActivityID)
	execution.CurrentHistActivityID = ""
	cc.agenda.schedule(&takeOutgoingOperation{executionID: execution.ID, activityID: task.ActivityID})
	return cc.agenda.run(cc)
}

// Delegate 委派任务。
// @param ctx 上下文
// @param taskID 任务 ID
// @param userID 被委托人
// @return 委派错误
func (e *Engine) Delegate(ctx context.Context, taskID, userID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		task, ok := e.findTask(taskID)
		if !ok {
			return nil, api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
		}
		if err := e.allowTask(ctx, task.Task.ID); err != nil {
			return nil, err
		}
		instance, _ := e.findInstance(task.Task.ProcessInstanceID)
		if instance != nil && instance.Suspended {
			return nil, api.NewError("ENGINE-0810", "流程实例已挂起", api.ErrConflict)
		}
		if task.Task.Assignee == "" {
			return nil, api.NewError("ENGINE-0809", "任务未签收，无法委派", api.ErrConflict)
		}
		if strings.TrimSpace(userID) == "" {
			return nil, api.NewError("ENGINE-0806", "被委托人不能为空", api.ErrBadRequest)
		}
		task.Task.Owner = task.Task.Assignee
		task.Task.Assignee = userID
		e.recordTaskLog(cc, task, "delegate", userID, "")
		return nil, nil
	})
	return err
}

// sortInstances 按开始时间排序实例。
// @param instances 实例列表
func sortInstances(instances []api.ProcessInstance) {
	sort.Slice(instances, func(i, j int) bool {
		if instances[i].StartTime.Equal(instances[j].StartTime) {
			return instances[i].ID < instances[j].ID
		}
		return instances[i].StartTime.Before(instances[j].StartTime)
	})
}
