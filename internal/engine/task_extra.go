package engine

import (
	"context"
	"sort"
	"strings"

	"github.com/flowable-go/flowable/api"
)

// AddComment 添加任务评论。
// @param ctx 上下文
// @param taskID 任务 ID
// @param userID 作者
// @param message 内容
// @return 评论
// @return 添加错误
func (e *Engine) AddComment(ctx context.Context, taskID, userID, message string) (api.Comment, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		task, ok := e.findTask(taskID)
		if !ok {
			return nil, api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
		}
		if err := e.allowTask(ctx, task.Task.ID); err != nil {
			return nil, err
		}
		if strings.TrimSpace(message) == "" {
			return nil, api.NewError("ENGINE-1101", "评论内容不能为空", api.ErrBadRequest)
		}
		comment := &api.Comment{
			ID:                e.newID(),
			TaskID:            taskID,
			ProcessInstanceID: task.Task.ProcessInstanceID,
			Type:              "comment",
			Message:           message,
			UserID:            userID,
			Time:              cc.now,
		}
		e.state.Comments[comment.ID] = comment
		return *comment, nil
	})
	if err != nil {
		return api.Comment{}, err
	}
	return result.(api.Comment), nil
}

// AddProcessComment 添加流程实例评论。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @param userID 作者
// @param message 内容
// @return 评论
// @return 添加错误
func (e *Engine) AddProcessComment(ctx context.Context, processInstanceID, userID, message string) (api.Comment, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		instance, ok := e.findInstance(processInstanceID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(ctx, instance.TenantID); err != nil {
			return nil, err
		}
		if strings.TrimSpace(message) == "" {
			return nil, api.NewError("ENGINE-1101", "评论内容不能为空", api.ErrBadRequest)
		}
		comment := &api.Comment{
			ID:                e.newID(),
			ProcessInstanceID: processInstanceID,
			Type:              "comment",
			Message:           message,
			UserID:            userID,
			Time:              cc.now,
		}
		e.state.Comments[comment.ID] = comment
		return *comment, nil
	})
	if err != nil {
		return api.Comment{}, err
	}
	return result.(api.Comment), nil
}

// ListProcessComments 返回流程实例评论。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @return 评论列表
// @return 查询错误
func (e *Engine) ListProcessComments(ctx context.Context, processInstanceID string) ([]api.Comment, error) {
	result, err := e.read(func() (any, error) {
		out := make([]api.Comment, 0)
		for _, comment := range e.state.Comments {
			if comment.ProcessInstanceID == processInstanceID && comment.TaskID == "" {
				out = append(out, *comment)
			}
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Time.Equal(out[j].Time) {
				return out[i].ID < out[j].ID
			}
			return out[i].Time.Before(out[j].Time)
		})
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.Comment), nil
}

// ListInstanceComments 返回流程实例下的全部评论，含任务评论。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @return 评论列表
// @return 查询错误
func (e *Engine) ListInstanceComments(ctx context.Context, processInstanceID string) ([]api.Comment, error) {
	result, err := e.read(func() (any, error) {
		out := make([]api.Comment, 0)
		for _, comment := range e.state.Comments {
			if comment.ProcessInstanceID == processInstanceID {
				out = append(out, *comment)
			}
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Time.Equal(out[j].Time) {
				return out[i].ID < out[j].ID
			}
			return out[i].Time.Before(out[j].Time)
		})
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.Comment), nil
}

// ListComments 返回任务评论。
// @param ctx 上下文
// @param taskID 任务 ID
// @return 评论列表
// @return 查询错误
func (e *Engine) ListComments(ctx context.Context, taskID string) ([]api.Comment, error) {
	result, err := e.read(func() (any, error) {
		out := make([]api.Comment, 0)
		for _, comment := range e.state.Comments {
			if taskID != "" && comment.TaskID != taskID {
				continue
			}
			out = append(out, *comment)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Time.Equal(out[j].Time) {
				return out[i].ID < out[j].ID
			}
			return out[i].Time.Before(out[j].Time)
		})
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.Comment), nil
}

// GetComment 查询单条评论。
// @param ctx 上下文
// @param commentID 评论 ID
// @return 评论
// @return 查询错误
func (e *Engine) GetComment(ctx context.Context, commentID string) (api.Comment, error) {
	result, err := e.read(func() (any, error) {
		comment, ok := e.state.Comments[commentID]
		if !ok {
			return nil, api.NewError("ENGINE-0404", "评论不存在", api.ErrNotFound)
		}
		if comment.TaskID != "" {
			if err := e.allowTask(ctx, comment.TaskID); err != nil {
				return nil, err
			}
		} else if comment.ProcessInstanceID != "" {
			if err := e.allowInstance(ctx, comment.ProcessInstanceID); err != nil {
				return nil, err
			}
		}
		return *comment, nil
	})
	if err != nil {
		return api.Comment{}, err
	}
	return result.(api.Comment), nil
}

// DeleteComment 删除评论。
// @param ctx 上下文
// @param commentID 评论 ID
// @return 删除错误
func (e *Engine) DeleteComment(ctx context.Context, commentID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		if _, ok := e.state.Comments[commentID]; !ok {
			return nil, api.NewError("ENGINE-0404", "评论不存在", api.ErrNotFound)
		}
		delete(e.state.Comments, commentID)
		return nil, nil
	})
	return err
}

// AddProcessAttachment 添加流程实例附件。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @param attachment 附件
// @return 附件
// @return 添加错误
func (e *Engine) AddProcessAttachment(ctx context.Context, processInstanceID string, attachment api.Attachment) (api.Attachment, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		instance, ok := e.findInstance(processInstanceID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(ctx, instance.TenantID); err != nil {
			return nil, err
		}
		if strings.TrimSpace(attachment.Name) == "" {
			return nil, api.NewError("ENGINE-1102", "附件名称不能为空", api.ErrBadRequest)
		}
		attachment.ID = e.newID()
		attachment.ProcessInstanceID = processInstanceID
		attachment.Time = cc.now
		copied := attachment
		e.state.Attachments[copied.ID] = &copied
		return copied, nil
	})
	if err != nil {
		return api.Attachment{}, err
	}
	return result.(api.Attachment), nil
}

// ListProcessAttachments 返回流程实例附件。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @return 附件列表
// @return 查询错误
func (e *Engine) ListProcessAttachments(ctx context.Context, processInstanceID string) ([]api.Attachment, error) {
	result, err := e.read(func() (any, error) {
		out := make([]api.Attachment, 0)
		for _, attachment := range e.state.Attachments {
			if attachment.ProcessInstanceID == processInstanceID && attachment.TaskID == "" {
				out = append(out, *attachment)
			}
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Time.Equal(out[j].Time) {
				return out[i].ID < out[j].ID
			}
			return out[i].Time.Before(out[j].Time)
		})
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.Attachment), nil
}

// AddAttachment 添加任务附件。
// @param ctx 上下文
// @param taskID 任务 ID
// @param attachment 附件
// @return 附件
// @return 添加错误
func (e *Engine) AddAttachment(ctx context.Context, taskID string, attachment api.Attachment) (api.Attachment, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		task, ok := e.findTask(taskID)
		if !ok {
			return nil, api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
		}
		if err := e.allowTask(ctx, task.Task.ID); err != nil {
			return nil, err
		}
		if strings.TrimSpace(attachment.Name) == "" {
			return nil, api.NewError("ENGINE-1102", "附件名称不能为空", api.ErrBadRequest)
		}
		attachment.ID = e.newID()
		attachment.TaskID = taskID
		attachment.ProcessInstanceID = task.Task.ProcessInstanceID
		attachment.Time = cc.now
		copied := attachment
		e.state.Attachments[copied.ID] = &copied
		return copied, nil
	})
	if err != nil {
		return api.Attachment{}, err
	}
	return result.(api.Attachment), nil
}

// ListAttachments 返回任务附件。
// @param ctx 上下文
// @param taskID 任务 ID
// @return 附件列表
// @return 查询错误
func (e *Engine) ListAttachments(ctx context.Context, taskID string) ([]api.Attachment, error) {
	result, err := e.read(func() (any, error) {
		out := make([]api.Attachment, 0)
		for _, attachment := range e.state.Attachments {
			if taskID != "" && attachment.TaskID != taskID {
				continue
			}
			out = append(out, *attachment)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Time.Equal(out[j].Time) {
				return out[i].ID < out[j].ID
			}
			return out[i].Time.Before(out[j].Time)
		})
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.Attachment), nil
}

// GetAttachment 查询单个附件。
// @param ctx 上下文
// @param attachmentID 附件 ID
// @return 附件
// @return 查询错误
func (e *Engine) GetAttachment(ctx context.Context, attachmentID string) (api.Attachment, error) {
	result, err := e.read(func() (any, error) {
		attachment, ok := e.state.Attachments[attachmentID]
		if !ok {
			return nil, api.NewError("ENGINE-0404", "附件不存在", api.ErrNotFound)
		}
		if attachment.TaskID != "" {
			if err := e.allowTask(ctx, attachment.TaskID); err != nil {
				return nil, err
			}
		} else if attachment.ProcessInstanceID != "" {
			if err := e.allowInstance(ctx, attachment.ProcessInstanceID); err != nil {
				return nil, err
			}
		}
		return *attachment, nil
	})
	if err != nil {
		return api.Attachment{}, err
	}
	return result.(api.Attachment), nil
}

// DeleteAttachment 删除附件。
// @param ctx 上下文
// @param attachmentID 附件 ID
// @return 删除错误
func (e *Engine) DeleteAttachment(ctx context.Context, attachmentID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		if _, ok := e.state.Attachments[attachmentID]; !ok {
			return nil, api.NewError("ENGINE-0404", "附件不存在", api.ErrNotFound)
		}
		delete(e.state.Attachments, attachmentID)
		return nil, nil
	})
	return err
}
