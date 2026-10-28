package engine

import (
	"context"
	"sort"

	"github.com/flowable-go/flowable/api"
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
		if _, ok := e.state.Jobs[jobID]; !ok {
			return nil, api.NewError("ENGINE-1501", "作业不存在", api.ErrNotFound)
		}
		if err := e.allowJob(ctx, e.state.Jobs[jobID]); err != nil {
			return nil, err
		}
		delete(e.state.Jobs, jobID)
		return nil, nil
	})
	return err
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
