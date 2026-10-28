package engine

import (
	"context"
	"sort"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/tenant"
)

// ListActivityInstances 返回运行中活动实例。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID，空串表示全部
// @return 活动实例列表
// @return 查询错误
func (e *Engine) ListActivityInstances(ctx context.Context, processInstanceID string) ([]api.ActivityInstance, error) {
	result, err := e.read(func() (any, error) {
		requested := e.tenantOf(tenant.From(ctx))
		out := make([]api.ActivityInstance, 0)
		for _, instance := range e.state.Instances {
			if instance.Ended {
				continue
			}
			if processInstanceID != "" && instance.ID != processInstanceID {
				continue
			}
			if requested != instance.TenantID {
				continue
			}
			definition, hasDefinition := e.findDefinition(instance.ProcessDefinitionID)
			for _, execution := range e.state.Executions {
				if execution.ProcessInstanceID != instance.ID || execution.Ended || execution.ActivityID == "" {
					continue
				}
				item := api.ActivityInstance{
					ID:                  execution.ID,
					ActivityID:          execution.ActivityID,
					ProcessDefinitionID: instance.ProcessDefinitionID,
					ProcessInstanceID:   instance.ID,
					ExecutionID:         execution.ID,
					TenantID:            instance.TenantID,
				}
				if hasDefinition {
					if element, ok := findElement(processOfDefinition(definition), execution.ActivityID); ok {
						item.ActivityName = element.GetName()
						item.ActivityType = element.ElementType()
					}
				}
				if task := e.taskForExecution(execution.ID); task != nil {
					item.TaskID = task.ID
					item.Assignee = task.Assignee
				}
				item.StartTime = e.activeActivityStart(instance.ID, execution.ActivityID)
				out = append(out, item)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.ActivityInstance), nil
}

// GetActiveActivityIDs 返回执行及子执行的活动 ID。
// @param ctx 上下文
// @param executionID 执行 ID
// @return 活动 ID 列表
// @return 查询错误
func (e *Engine) GetActiveActivityIDs(ctx context.Context, executionID string) ([]string, error) {
	result, err := e.read(func() (any, error) {
		execution, ok := e.findExecution(executionID)
		if !ok {
			return nil, api.NewError("ENGINE-0405", "执行不存在", api.ErrNotFound)
		}
		if err := e.allowInstance(ctx, execution.ProcessInstanceID); err != nil {
			return nil, err
		}
		children := make(map[string][]string)
		for id, child := range e.state.Executions {
			if child.ParentID != "" {
				children[child.ParentID] = append(children[child.ParentID], id)
			}
		}
		seen := map[string]bool{executionID: true}
		queue := []string{executionID}
		activities := make(map[string]bool)
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			if state, exists := e.state.Executions[current]; exists && state.ActivityID != "" && !state.Ended {
				activities[state.ActivityID] = true
			}
			for _, childID := range children[current] {
				if seen[childID] {
					continue
				}
				seen[childID] = true
				queue = append(queue, childID)
			}
		}
		out := make([]string, 0, len(activities))
		for activityID := range activities {
			out = append(out, activityID)
		}
		sort.Strings(out)
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]string), nil
}

// taskForExecution 返回执行上的任务。
// @param executionID 执行 ID
// @return 任务，不存在时为 nil
func (e *Engine) taskForExecution(executionID string) *api.Task {
	var found *api.Task
	for _, task := range e.state.Tasks {
		if task.ExecutionID != executionID {
			continue
		}
		copied := task.Task
		if found == nil || copied.ID < found.ID {
			found = &copied
		}
	}
	return found
}

// activeActivityStart 返回活动最近一次开始时间。
// @param processInstanceID 实例 ID
// @param activityID 活动 ID
// @return 开始时间，不存在时为 nil
func (e *Engine) activeActivityStart(processInstanceID, activityID string) *time.Time {
	var latest *api.HistoricActivityInstance
	for _, historic := range e.state.HistoricActivities {
		if historic.ProcessInstanceID != processInstanceID || historic.ActivityID != activityID || historic.EndTime != nil {
			continue
		}
		if latest == nil || historic.StartTime.After(latest.StartTime) {
			latest = historic
		}
	}
	if latest == nil {
		return nil
	}
	return cloneTime(&latest.StartTime)
}
