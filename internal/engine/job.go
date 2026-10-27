package engine

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
)

// RunDueJobs 执行全部到期的定时器作业。
// @param ctx 上下文
// @return 执行数量
// @return 执行错误
func (e *Engine) RunDueJobs(ctx context.Context) (int, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		return e.runDueJobs(cc)
	})
	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

// runDueJobs 在命令内执行到期作业。
// @param cc 命令上下文
// @return 执行数量
// @return 执行错误
func (e *Engine) runDueJobs(cc *commandContext) (int, error) {
	jobs := make([]*jobState, 0)
	for _, job := range e.state.Jobs {
		if job.DueDate.After(cc.now) {
			continue
		}
		if job.ProcessInstanceID != "" {
			if instance, ok := e.findInstance(job.ProcessInstanceID); ok && instance.Suspended {
				continue
			}
		}
		jobs = append(jobs, job)
	}
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].DueDate.Equal(jobs[j].DueDate) {
			return jobs[i].ID < jobs[j].ID
		}
		return jobs[i].DueDate.Before(jobs[j].DueDate)
	})
	executed := 0
	for _, job := range jobs {
		delete(e.state.Jobs, job.ID)
		if err := e.triggerJob(cc, job); err != nil {
			return 0, err
		}
		executed++
	}
	if executed > 0 {
		if err := cc.agenda.run(cc); err != nil {
			return 0, err
		}
	}
	return executed, nil
}

// triggerJob 按类型触发作业。
// @param cc 命令上下文
// @param job 作业
// @return 触发错误
func (e *Engine) triggerJob(cc *commandContext, job *jobState) error {
	switch job.Type {
	case "async-before":
		execution, ok := e.findExecution(job.ExecutionID)
		if !ok || execution.Ended {
			return nil
		}
		execution.Waiting = false
		execution.SkipAsyncBefore = true
		cc.agenda.schedule(&continueOperation{executionID: execution.ID, activityID: job.ActivityID})
		return nil
	case "timer-start":
		definition, ok := e.findDefinition(job.ProcessDefinitionID)
		if !ok || definition.Info.Suspended {
			return nil
		}
		_, err := e.startProcess(cc, api.StartRequest{ProcessDefinitionID: job.ProcessDefinitionID})
		return err
	case "timer-catch":
		execution, ok := e.findExecution(job.ExecutionID)
		if !ok || execution.Ended {
			return nil
		}
		if instance, exists := e.findInstance(execution.ProcessInstanceID); !exists || instance.Ended || instance.Suspended {
			return nil
		}
		e.recordActivityEnd(cc, execution.CurrentHistActivityID)
		execution.CurrentHistActivityID = ""
		execution.Waiting = false
		cc.agenda.schedule(&takeOutgoingOperation{executionID: execution.ID, activityID: job.ActivityID})
		return nil
	case "async-after":
		execution, ok := e.findExecution(job.ExecutionID)
		if !ok || execution.Ended {
			return nil
		}
		execution.Waiting = false
		execution.SkipAsyncAfter = true
		cc.agenda.schedule(&takeOutgoingOperation{executionID: execution.ID, activityID: job.ActivityID})
		return nil
	default:
		return e.triggerBoundaryJob(cc, job)
	}
}

// scheduleAsyncJob 创建异步续跑作业。
// @param cc 命令上下文
// @param execution 执行
// @param activityID 活动 ID
// @param jobType 作业类型
func (e *Engine) scheduleAsyncJob(cc *commandContext, execution *executionState, activityID, jobType string) {
	job := &jobState{
		ID:                e.newID(),
		Type:              jobType,
		ProcessInstanceID: execution.ProcessInstanceID,
		ExecutionID:       execution.ID,
		ActivityID:        activityID,
		DueDate:           cc.now,
		CreatedAt:         cc.now,
	}
	e.state.Jobs[job.ID] = job
}

// triggerBoundaryJob 触发定时器边界事件。
// @param cc 命令上下文
// @param job 定时器作业
// @return 触发错误
func (e *Engine) triggerBoundaryJob(cc *commandContext, job *jobState) error {
	execution, ok := e.findExecution(job.ExecutionID)
	if !ok || execution.Ended {
		return nil
	}
	instance, ok := e.findInstance(execution.ProcessInstanceID)
	if !ok || instance.Ended {
		return nil
	}
	definition, ok := e.findDefinition(execution.DefinitionID)
	if !ok {
		return nil
	}
	process := processOfDefinition(definition)
	if process == nil {
		return nil
	}
	element, ok := findElement(process, job.BoundaryEventID)
	if !ok {
		return nil
	}
	boundary, ok := element.(*model.BoundaryEvent)
	if !ok {
		return nil
	}
	if job.CancelActivity {
		for taskID, task := range e.state.Tasks {
			if task.ExecutionID != execution.ID {
				continue
			}
			e.recordTaskEnd(cc, task)
			delete(e.state.Tasks, taskID)
		}
		e.recordActivityEnd(cc, execution.CurrentHistActivityID)
		execution.CurrentHistActivityID = ""
		e.cancelBoundaryTimers(execution.ID)
		e.cancelBoundarySubscriptions(execution.ID)
		execution.Waiting = false
		execution.ActivityID = boundary.ID
		cc.agenda.schedule(&continueOperation{executionID: execution.ID, activityID: boundary.ID})
		return nil
	}
	// 非中断边界：创建并发执行，原活动继续等待
	child := &executionState{
		ID:                e.newID(),
		ProcessInstanceID: execution.ProcessInstanceID,
		DefinitionID:      execution.DefinitionID,
		ParentID:          execution.ID,
		ActivityID:        boundary.ID,
		Active:            true,
		Variables:         make(map[string]any),
	}
	e.state.Executions[child.ID] = child
	cc.agenda.schedule(&continueOperation{executionID: child.ID, activityID: boundary.ID})
	return nil
}

// scheduleBoundaryEvents 为节点的边界定时器与消息/信号创建订阅。
// @param cc 命令上下文
// @param execution 执行
// @param element 活动元素
// @return 创建错误
func (e *Engine) scheduleBoundaryEvents(cc *commandContext, execution *executionState, element model.FlowElement) error {
	node, ok := element.(model.FlowNode)
	if !ok {
		return nil
	}
	// 多实例边界事件由各子实例负责，避免根执行重复创建
	if execution.MultiInstanceRoot {
		return nil
	}
	definition, _ := e.findDefinition(execution.DefinitionID)
	for _, boundary := range node.GetBoundaryEvents() {
		if boundary.MessageRef != "" || boundary.SignalRef != "" {
			if definition == nil {
				continue
			}
			eventType, name := e.eventTypeAndName(definition.Model, boundary.MessageRef, boundary.SignalRef)
			if eventType == "" {
				continue
			}
			subscription := &subscriptionState{
				ID:                  e.newID(),
				ProcessDefinitionID: definition.Info.ID,
				ProcessInstanceID:   execution.ProcessInstanceID,
				ExecutionID:         execution.ID,
				EventType:           eventType,
				EventName:           name,
				ActivityID:          element.GetID(),
				BoundaryEventID:     boundary.ID,
				TenantID:            definition.Info.TenantID,
				CreatedAt:           cc.now,
			}
			e.state.Subscriptions[subscription.ID] = subscription
		}
		if boundary.Timer == nil {
			continue
		}
		due, exists, err := timerDueDate(cc.now, boundary.Timer)
		if err != nil {
			return api.NewError("ENGINE-0601", "边界定时器解析失败: "+boundary.ID, err)
		}
		if !exists {
			continue
		}
		job := &jobState{
			ID:                e.newID(),
			Type:              "boundary-timer",
			ProcessInstanceID: execution.ProcessInstanceID,
			ExecutionID:       execution.ID,
			BoundaryEventID:   boundary.ID,
			ActivityID:        element.GetID(),
			CancelActivity:    boundary.CancelActivity,
			DueDate:           due,
			CreatedAt:         cc.now,
		}
		e.state.Jobs[job.ID] = job
	}
	return nil
}

// cancelBoundaryTimers 取消执行上的全部定时器作业。
// @param executionID 执行 ID
func (e *Engine) cancelBoundaryTimers(executionID string) {
	for id, job := range e.state.Jobs {
		if job.ExecutionID == executionID {
			delete(e.state.Jobs, id)
		}
	}
}

// timerDueDate 计算定时器到期时间。
// @param now 当前时间
// @param timer 定时器定义
// @return 到期时间
// @return 是否存在
// @return 解析错误
func timerDueDate(now time.Time, timer *model.TimerDefinition) (time.Time, bool, error) {
	if timer == nil {
		return time.Time{}, false, nil
	}
	if strings.TrimSpace(timer.TimeDate) != "" {
		due, err := time.Parse(time.RFC3339, strings.TrimSpace(timer.TimeDate))
		if err != nil {
			return time.Time{}, false, err
		}
		if due.Before(now) {
			due = now
		}
		return due, true, nil
	}
	if strings.TrimSpace(timer.TimeDuration) != "" {
		duration, err := parseISODuration(timer.TimeDuration)
		if err != nil {
			return time.Time{}, false, err
		}
		return now.Add(duration), true, nil
	}
	if strings.TrimSpace(timer.TimeCycle) != "" {
		parts := strings.Split(timer.TimeCycle, "/")
		duration, err := parseISODuration(parts[len(parts)-1])
		if err != nil {
			return time.Time{}, false, err
		}
		return now.Add(duration), true, nil
	}
	return time.Time{}, false, nil
}

// parseISODuration 解析 ISO-8601 持续时间（支持 D/H/M/S）。
// @param value 持续时间文本
// @return 时长
// @return 解析错误
func parseISODuration(value string) (time.Duration, error) {
	text := strings.TrimSpace(value)
	if len(text) < 2 || text[0] != 'P' {
		return 0, fmt.Errorf("非法 ISO 持续时间: %s", value)
	}
	text = text[1:]
	var total time.Duration
	datePart := text
	timePart := ""
	if index := strings.IndexByte(text, 'T'); index >= 0 {
		datePart = text[:index]
		timePart = text[index+1:]
	}
	if datePart != "" {
		parsed, err := parseDurationPart(datePart, map[byte]time.Duration{'D': 24 * time.Hour})
		if err != nil {
			return 0, err
		}
		total += parsed
	}
	if timePart != "" {
		parsed, err := parseDurationPart(timePart, map[byte]time.Duration{
			'H': time.Hour,
			'M': time.Minute,
			'S': time.Second,
		})
		if err != nil {
			return 0, err
		}
		total += parsed
	}
	if total <= 0 {
		return 0, fmt.Errorf("持续时间必须大于零: %s", value)
	}
	return total, nil
}

// parseDurationPart 解析持续时间片段。
// @param part 片段
// @param units 单位映射
// @return 时长
// @return 解析错误
func parseDurationPart(part string, units map[byte]time.Duration) (time.Duration, error) {
	var total time.Duration
	for i := 0; i < len(part); {
		start := i
		for i < len(part) && (part[i] >= '0' && part[i] <= '9' || part[i] == '.') {
			i++
		}
		if start == i || i >= len(part) {
			return 0, fmt.Errorf("非法持续时间片段: %s", part)
		}
		number, err := strconv.ParseFloat(part[start:i], 64)
		if err != nil {
			return 0, err
		}
		unit, ok := units[part[i]]
		if !ok {
			return 0, fmt.Errorf("不支持的时间单位: %c", part[i])
		}
		total += time.Duration(number * float64(unit))
		i++
	}
	return total, nil
}

// scheduleStartTimers 为定时启动事件创建作业。
// @param cc 命令上下文
// @param definition 流程定义
// @param process 流程
// @return 创建错误
func (e *Engine) scheduleStartTimers(cc *commandContext, definition *definitionState, process *model.Process) error {
	for _, start := range process.StartEvents() {
		if start.Timer == nil {
			continue
		}
		due, exists, err := timerDueDate(cc.now, start.Timer)
		if err != nil {
			return api.NewError("ENGINE-0602", "定时启动事件解析失败: "+start.ID, err)
		}
		if !exists {
			continue
		}
		job := &jobState{
			ID:                  e.newID(),
			Type:                "timer-start",
			ProcessDefinitionID: definition.Info.ID,
			ActivityID:          start.ID,
			DueDate:             due,
			CreatedAt:           cc.now,
		}
		e.state.Jobs[job.ID] = job
	}
	return nil
}

// scheduleTimerCatch 为中间定时捕获事件创建作业。
// @param cc 命令上下文
// @param execution 执行
// @param activityID 活动 ID
// @param timer 定时器定义
// @return 创建错误
func (e *Engine) scheduleTimerCatch(cc *commandContext, execution *executionState, activityID string, timer *model.TimerDefinition) error {
	due, exists, err := timerDueDate(cc.now, timer)
	if err != nil {
		return api.NewError("ENGINE-0603", "中间定时捕获事件解析失败: "+activityID, err)
	}
	if !exists {
		return nil
	}
	job := &jobState{
		ID:                e.newID(),
		Type:              "timer-catch",
		ProcessInstanceID: execution.ProcessInstanceID,
		ExecutionID:       execution.ID,
		ActivityID:        activityID,
		DueDate:           due,
		CreatedAt:         cc.now,
	}
	e.state.Jobs[job.ID] = job
	return nil
}
