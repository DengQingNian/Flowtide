package engine

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/DengQingNian/Flowtide/api"
)

// ListJobs 返回可执行作业。
// @param ctx 上下文
// @return 作业列表
// @return 查询错误
func (e *Engine) ListJobs(ctx context.Context) ([]api.JobInfo, error) {
	result, err := e.read(func() (any, error) {
		out := make([]api.JobInfo, 0, len(e.state.Jobs))
		for _, job := range e.state.Jobs {
			if e.allowJob(ctx, job) != nil {
				continue
			}
			out = append(out, toJobInfo(job))
		}
		sortJobInfos(out)
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.JobInfo), nil
}

// DeleteJob 删除可执行作业。
// @param ctx 上下文
// @param jobID 作业 ID
// @return 删除错误
func (e *Engine) DeleteJob(ctx context.Context, jobID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		job, ok := e.state.Jobs[jobID]
		// 定时器与挂起作业属于其他作业桶，不能从可执行作业入口删除
		if !ok || !e.executableJob(job) {
			return nil, api.NewError("ENGINE-1501", "作业不存在", api.ErrNotFound)
		}
		if err := e.allowJob(ctx, job); err != nil {
			return nil, err
		}
		delete(e.state.Jobs, jobID)
		return nil, nil
	})
	return err
}

// DeleteTimerJob 删除定时器作业。
// 仅接受未挂起、未被移入可执行桶的定时器作业。
// @param ctx 上下文
// @param jobID 作业 ID
// @return 删除错误
func (e *Engine) DeleteTimerJob(ctx context.Context, jobID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		job, ok := e.state.Jobs[jobID]
		// 跨桶删除返回 404，避免误删可执行或挂起作业
		if !ok || !e.timerJob(job) {
			return nil, api.NewError("ENGINE-1502", "定时器作业不存在", api.ErrNotFound)
		}
		if err := e.allowJob(ctx, job); err != nil {
			return nil, err
		}
		delete(e.state.Jobs, jobID)
		return nil, nil
	})
	return err
}

// DeleteSuspendedJob 删除挂起作业。
// @param ctx 上下文
// @param jobID 作业 ID
// @return 删除错误
func (e *Engine) DeleteSuspendedJob(ctx context.Context, jobID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		job, ok := e.state.Jobs[jobID]
		// 只有所属流程实例处于挂起状态的作业才属于挂起桶
		if !ok || !e.suspendedJob(job) {
			return nil, api.NewError("ENGINE-1503", "挂起作业不存在", api.ErrNotFound)
		}
		if err := e.allowJob(ctx, job); err != nil {
			return nil, err
		}
		delete(e.state.Jobs, jobID)
		return nil, nil
	})
	return err
}

// DeleteDeadLetterJob 删除死信作业。
// @param ctx 上下文
// @param jobID 作业 ID
// @return 删除错误
func (e *Engine) DeleteDeadLetterJob(ctx context.Context, jobID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		job, ok := e.state.DeadLetters[jobID]
		if !ok {
			return nil, api.NewError("ENGINE-1504", "死信作业不存在", api.ErrNotFound)
		}
		if err := e.allowJob(ctx, job); err != nil {
			return nil, err
		}
		delete(e.state.DeadLetters, jobID)
		return nil, nil
	})
	return err
}

// MoveTimerJobToExecutable 将定时器作业移入可执行作业桶。
// 作业保留原处理器类型，仅不再作为定时器参与到期调度。
// @param ctx 上下文
// @param jobID 作业 ID
// @return 移动错误
func (e *Engine) MoveTimerJobToExecutable(ctx context.Context, jobID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		job, ok := e.state.Jobs[jobID]
		if !ok || !e.timerJob(job) {
			return nil, api.NewError("ENGINE-1502", "定时器作业不存在", api.ErrNotFound)
		}
		if err := e.allowJob(ctx, job); err != nil {
			return nil, err
		}
		job.Executable = true
		job.DueDate = cc.now
		if job.Retries <= 0 {
			job.Retries = 3
		}
		job.Exception = ""
		return nil, nil
	})
	return err
}

// RescheduleTimerJob 修改定时器作业的到期时间。
// @param ctx 上下文
// @param jobID 作业 ID
// @param dueDate RFC3339 到期时间
// @return 修改错误
func (e *Engine) RescheduleTimerJob(ctx context.Context, jobID, dueDate string) error {
	parsed, parseErr := time.Parse(time.RFC3339, dueDate)
	// 非法日期属于请求错误，必须在事务外拒绝
	if parseErr != nil {
		return api.NewError("ENGINE-1506", "到期时间格式非法", api.ErrBadRequest)
	}
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		job, ok := e.state.Jobs[jobID]
		if !ok || !e.timerJob(job) {
			return nil, api.NewError("ENGINE-1502", "定时器作业不存在", api.ErrNotFound)
		}
		if err := e.allowJob(ctx, job); err != nil {
			return nil, err
		}
		job.DueDate = parsed
		return nil, nil
	})
	return err
}

// timerJob 判断作业是否属于定时器桶。
// @param job 作业
// @return 是否定时器作业
func (e *Engine) timerJob(job *jobState) bool {
	return !job.Executable && strings.Contains(job.Type, "timer") && !e.suspendedJob(job)
}

// suspendedJob 判断作业是否属于挂起桶。
// @param job 作业
// @return 是否挂起作业
func (e *Engine) suspendedJob(job *jobState) bool {
	if job.ProcessInstanceID == "" {
		return false
	}
	instance, ok := e.findInstance(job.ProcessInstanceID)
	return ok && instance.Suspended
}

// executableJob 判断作业是否属于可执行桶。
// @param job 作业
// @return 是否可执行作业
func (e *Engine) executableJob(job *jobState) bool {
	return !e.timerJob(job) && !e.suspendedJob(job)
}

// ListDeadLetterJobs 返回死信作业。
// @param ctx 上下文
// @return 死信作业列表
// @return 查询错误
func (e *Engine) ListDeadLetterJobs(ctx context.Context) ([]api.JobInfo, error) {
	result, err := e.read(func() (any, error) {
		out := make([]api.JobInfo, 0, len(e.state.DeadLetters))
		for _, job := range e.state.DeadLetters {
			if e.allowJob(ctx, job) != nil {
				continue
			}
			out = append(out, toJobInfo(job))
		}
		sortJobInfos(out)
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.JobInfo), nil
}

// MoveDeadLetterJobToExecutable 将死信作业恢复为可执行作业。
// @param ctx 上下文
// @param jobID 作业 ID
// @param retries 恢复后的重试次数
// @return 恢复错误
func (e *Engine) MoveDeadLetterJobToExecutable(ctx context.Context, jobID string, retries int) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		job, ok := e.state.DeadLetters[jobID]
		if !ok {
			return nil, api.NewError("ENGINE-0404", "死信作业不存在", api.ErrNotFound)
		}
		if err := e.allowJob(ctx, job); err != nil {
			return nil, err
		}
		if retries <= 0 {
			retries = 3
		}
		delete(e.state.DeadLetters, jobID)
		job.Retries = retries
		job.DueDate = cc.now
		job.Exception = ""
		// 死信作业恢复后归入可执行桶，即使原类型为定时器
		job.Executable = true
		e.state.Jobs[job.ID] = job
		return nil, nil
	})
	return err
}

// toJobInfo 将作业状态转换为 DTO。
// @param job 作业
// @return 作业信息
func toJobInfo(job *jobState) api.JobInfo {
	return api.JobInfo{
		ID:                job.ID,
		Type:              job.Type,
		ProcessInstanceID: job.ProcessInstanceID,
		ExecutionID:       job.ExecutionID,
		ActivityID:        job.ActivityID,
		Retries:           job.Retries,
		Exception:         job.Exception,
		DueDate:           job.DueDate,
		Executable:        job.Executable,
	}
}

// sortJobInfos 按到期时间排序作业。
// @param jobs 作业列表
func sortJobInfos(jobs []api.JobInfo) {
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].DueDate.Equal(jobs[j].DueDate) {
			return jobs[i].ID < jobs[j].ID
		}
		return jobs[i].DueDate.Before(jobs[j].DueDate)
	})
}

// GetJob 查询可执行作业。
// @param ctx 上下文
// @param jobID 作业 ID
// @return 作业信息
// @return 查询错误
func (e *Engine) GetJob(ctx context.Context, jobID string) (api.JobInfo, error) {
	result, err := e.read(func() (any, error) {
		job, ok := e.state.Jobs[jobID]
		if !ok {
			return nil, api.NewError("ENGINE-0404", "作业不存在", api.ErrNotFound)
		}
		if err := e.allowJob(ctx, job); err != nil {
			return nil, err
		}
		return toJobInfo(job), nil
	})
	if err != nil {
		return api.JobInfo{}, err
	}
	return result.(api.JobInfo), nil
}

// GetDeadLetterJob 查询死信作业。
// @param ctx 上下文
// @param jobID 作业 ID
// @return 作业信息
// @return 查询错误
func (e *Engine) GetDeadLetterJob(ctx context.Context, jobID string) (api.JobInfo, error) {
	result, err := e.read(func() (any, error) {
		job, ok := e.state.DeadLetters[jobID]
		if !ok {
			return nil, api.NewError("ENGINE-0404", "死信作业不存在", api.ErrNotFound)
		}
		if err := e.allowJob(ctx, job); err != nil {
			return nil, err
		}
		return toJobInfo(job), nil
	})
	if err != nil {
		return api.JobInfo{}, err
	}
	return result.(api.JobInfo), nil
}

// SetJobRetries 设置作业重试次数。
// @param ctx 上下文
// @param jobID 作业 ID
// @param retries 重试次数
// @return 设置错误
func (e *Engine) SetJobRetries(ctx context.Context, jobID string, retries int) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		job, ok := e.state.Jobs[jobID]
		if !ok {
			return nil, api.NewError("ENGINE-0404", "作业不存在", api.ErrNotFound)
		}
		if err := e.allowJob(ctx, job); err != nil {
			return nil, err
		}
		if retries < 0 {
			return nil, api.NewError("ENGINE-0916", "重试次数不能为负", api.ErrBadRequest)
		}
		job.Retries = retries
		return nil, nil
	})
	return err
}
