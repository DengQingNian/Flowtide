package engine

import (
	"context"
	"fmt"

	"github.com/flowable-go/flowable/api"
)

// ListIdentityLinks 返回任务参与者关联。
// @param ctx 上下文
// @param taskID 任务 ID
// @return 关联列表
// @return 查询错误
func (e *Engine) ListIdentityLinks(ctx context.Context, taskID string) ([]api.IdentityLink, error) {
	result, err := e.read(func() (any, error) {
		task, ok := e.findTask(taskID)
		if !ok {
			return nil, api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
		}
		links := make([]api.IdentityLink, 0)
		if task.Task.Assignee != "" {
			links = append(links, api.IdentityLink{Type: "assignee", UserID: task.Task.Assignee, TaskID: taskID, ProcessInstanceID: task.Task.ProcessInstanceID})
		}
		if task.Task.Owner != "" {
			links = append(links, api.IdentityLink{Type: "owner", UserID: task.Task.Owner, TaskID: taskID, ProcessInstanceID: task.Task.ProcessInstanceID})
		}
		for _, userID := range task.CandidateUsers {
			links = append(links, api.IdentityLink{Type: "candidate", UserID: userID, TaskID: taskID, ProcessInstanceID: task.Task.ProcessInstanceID})
		}
		for _, groupID := range task.CandidateGroups {
			links = append(links, api.IdentityLink{Type: "candidate", GroupID: groupID, TaskID: taskID, ProcessInstanceID: task.Task.ProcessInstanceID})
		}
		return links, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.IdentityLink), nil
}

// AddCandidateUser 添加候选用户。
// @param ctx 上下文
// @param taskID 任务 ID
// @param userID 用户 ID
// @return 添加错误
func (e *Engine) AddCandidateUser(ctx context.Context, taskID, userID string) error {
	return e.modifyCandidates(ctx, taskID, userID, "", true)
}

// DeleteCandidateUser 移除候选用户。
// @param ctx 上下文
// @param taskID 任务 ID
// @param userID 用户 ID
// @return 删除错误
func (e *Engine) DeleteCandidateUser(ctx context.Context, taskID, userID string) error {
	return e.modifyCandidates(ctx, taskID, userID, "", false)
}

// AddCandidateGroup 添加候选组。
// @param ctx 上下文
// @param taskID 任务 ID
// @param groupID 组 ID
// @return 添加错误
func (e *Engine) AddCandidateGroup(ctx context.Context, taskID, groupID string) error {
	return e.modifyCandidates(ctx, taskID, "", groupID, true)
}

// DeleteCandidateGroup 移除候选组。
// @param ctx 上下文
// @param taskID 任务 ID
// @param groupID 组 ID
// @return 删除错误
func (e *Engine) DeleteCandidateGroup(ctx context.Context, taskID, groupID string) error {
	return e.modifyCandidates(ctx, taskID, "", groupID, false)
}

// modifyCandidates 修改任务候选人或候选组。
// @param ctx 上下文
// @param taskID 任务 ID
// @param userID 用户 ID
// @param groupID 组 ID
// @param add 是否添加
// @return 修改错误
func (e *Engine) modifyCandidates(ctx context.Context, taskID, userID, groupID string, add bool) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		task, ok := e.findTask(taskID)
		if !ok {
			return nil, api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
		}
		if err := e.allowTask(ctx, task.Task.ID); err != nil {
			return nil, err
		}
		if userID != "" {
			if add {
				if !contains(task.CandidateUsers, userID) {
					task.CandidateUsers = append(task.CandidateUsers, userID)
				}
			} else {
				task.CandidateUsers = removeString(task.CandidateUsers, userID)
			}
			task.Task.CandidateUsers = task.CandidateUsers
		}
		if groupID != "" {
			if add {
				if !contains(task.CandidateGroups, groupID) {
					task.CandidateGroups = append(task.CandidateGroups, groupID)
				}
			} else {
				task.CandidateGroups = removeString(task.CandidateGroups, groupID)
			}
			task.Task.CandidateGroups = task.CandidateGroups
		}
		e.recordTaskIdentityLinks(cc, task)
		// 原版在候选成员加入时写独立任务日志；移除使用对应 REMOVED 事件。
		if add {
			data := ""
			if userID != "" {
				data = fmt.Sprintf(`{"userId":%q,"type":"candidate"}`, userID)
			} else {
				data = fmt.Sprintf(`{"groupId":%q,"type":"candidate"}`, groupID)
			}
			e.recordTaskLog(cc, task, "USER_TASK_IDENTITY_LINK_ADDED", actorFromContext(cc.ctx), data)
		}
		return nil, nil
	})
	return err
}

// removeString 从列表中移除字符串。
// @param values 列表
// @param target 目标
// @return 新列表
func removeString(values []string, target string) []string {
	out := values[:0]
	for _, value := range values {
		if value != target {
			out = append(out, value)
		}
	}
	return out
}
