package rest

import (
	"net/http"
	"strings"

	"github.com/flowable-go/flowable/api"
)

// isTimerJob 判断是否为定时器作业。
// @param job 作业
// @return 是否定时器
func isTimerJob(job api.JobInfo) bool {
	return !job.Executable && strings.Contains(job.Type, "timer")
}

// handleListTimerJobs 返回定时器作业。
// @param w 响应
// @param r 请求
func (s *Server) handleListTimerJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.engine.ManagementService().ListJobs(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	filtered := make([]api.JobInfo, 0, len(jobs))
	for _, job := range jobs {
		// 挂起实例的定时器作业归入 suspended-jobs
		if isTimerJob(job) && !s.suspendedJob(r, job) {
			filtered = append(filtered, job)
		}
	}
	start, size := pageParams(r)
	writePage(w, filtered, start, size)
}

// handleGetTimerJob 返回定时器作业详情。
// @param w 响应
// @param r 请求
func (s *Server) handleGetTimerJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.engine.ManagementService().GetJob(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	if !isTimerJob(job) || s.suspendedJob(r, job) {
		writeError(w, api.NewError("ENGINE-1502", "定时器作业不存在", api.ErrNotFound))
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// handleListSuspendedJobs 返回挂起实例的作业。
// @param w 响应
// @param r 请求
func (s *Server) handleListSuspendedJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.engine.ManagementService().ListJobs(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	filtered := make([]api.JobInfo, 0, len(jobs))
	for _, job := range jobs {
		if s.suspendedJob(r, job) {
			filtered = append(filtered, job)
		}
	}
	start, size := pageParams(r)
	writePage(w, filtered, start, size)
}

// handleGetSuspendedJob 返回挂起作业详情。
// @param w 响应
// @param r 请求
func (s *Server) handleGetSuspendedJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.engine.ManagementService().GetJob(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	if !s.suspendedJob(r, job) {
		writeError(w, api.NewError("ENGINE-1503", "挂起作业不存在", api.ErrNotFound))
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// suspendedJob 判断作业是否属于挂起实例。
// @param r 请求
// @param job 作业
// @return 是否挂起
func (s *Server) suspendedJob(r *http.Request, job api.JobInfo) bool {
	if job.ProcessInstanceID == "" {
		return false
	}
	instance, err := s.engine.RuntimeService().GetProcessInstance(r.Context(), job.ProcessInstanceID)
	if err != nil {
		return false
	}
	return instance.Suspended
}

// handleListHistoryJobs 返回历史作业。
// 当前未实现异步历史，固定返回空列表。
// @param w 响应
// @param r 请求
func (s *Server) handleListHistoryJobs(w http.ResponseWriter, r *http.Request) {
	start, size := pageParams(r)
	writePage(w, []api.JobInfo{}, start, size)
}

// handleGetHistoryJob 返回历史作业详情。
// @param w 响应
// @param r 请求
func (s *Server) handleGetHistoryJob(w http.ResponseWriter, r *http.Request) {
	writeError(w, api.NewError("ENGINE-1504", "历史作业不存在", api.ErrNotFound))
}

// handleDeleteJob 删除可执行作业。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteJob(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.ManagementService().DeleteJob(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleJobStacktrace 返回可执行作业异常栈。
// @param w 响应
// @param r 请求
func (s *Server) handleJobStacktrace(w http.ResponseWriter, r *http.Request) {
	job, err := s.engine.ManagementService().GetJob(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeStacktrace(w, job)
}

// handleDeadLetterJobStacktrace 返回死信作业异常栈。
// @param w 响应
// @param r 请求
func (s *Server) handleDeadLetterJobStacktrace(w http.ResponseWriter, r *http.Request) {
	job, err := s.engine.ManagementService().GetDeadLetterJob(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeStacktrace(w, job)
}

// handleTimerJobStacktrace 返回定时器作业异常栈。
// @param w 响应
// @param r 请求
func (s *Server) handleTimerJobStacktrace(w http.ResponseWriter, r *http.Request) {
	job, err := s.engine.ManagementService().GetJob(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	if !isTimerJob(job) || s.suspendedJob(r, job) {
		writeError(w, api.NewError("ENGINE-1502", "定时器作业不存在", api.ErrNotFound))
		return
	}
	writeStacktrace(w, job)
}

// handleSuspendedJobStacktrace 返回挂起作业异常栈。
// @param w 响应
// @param r 请求
func (s *Server) handleSuspendedJobStacktrace(w http.ResponseWriter, r *http.Request) {
	job, err := s.engine.ManagementService().GetJob(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	if !s.suspendedJob(r, job) {
		writeError(w, api.NewError("ENGINE-1503", "挂起作业不存在", api.ErrNotFound))
		return
	}
	writeStacktrace(w, job)
}

// writeStacktrace 输出异常栈文本。
// @param w 响应
// @param job 作业
func writeStacktrace(w http.ResponseWriter, job api.JobInfo) {
	if strings.TrimSpace(job.Exception) == "" {
		writeError(w, api.NewError("ENGINE-1505", "作业没有异常栈", api.ErrNotFound))
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(job.Exception))
}
