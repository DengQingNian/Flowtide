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

// RunDueJobs 执行全部到期作业。
// 每个作业独立事务，失败只影响该作业。
// @param ctx 上下文
// @return 执行数量
// @return 执行错误
func (e *Engine) RunDueJobs(ctx context.Context) (int, error) {
	ids, err := e.dueJobIDs(ctx)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, id := range ids {
		executed, runErr := e.runOneJob(ctx, id)
		if runErr != nil {
			return total, runErr
		}
		total += executed
	}
	return total, nil
}

// dueJobIDs 返回到期且实例未挂起的作业 ID。
// @param ctx 上下文
// @return 作业 ID 列表
// @return 查询错误
func (e *Engine) dueJobIDs(ctx context.Context) ([]string, error) {
	result, err := e.read(func() (any, error) {
		type dueJob struct {
			id  string
			due time.Time
		}
		due := make([]dueJob, 0)
		now := e.cfg.Clock.Now()
		for _, job := range e.state.Jobs {
			if job.DueDate.After(now) {
				continue
			}
			if job.ProcessInstanceID != "" {
				if instance, ok := e.findInstance(job.ProcessInstanceID); ok && instance.Suspended {
					continue
				}
			}
			due = append(due, dueJob{id: job.ID, due: job.DueDate})
		}
		sort.Slice(due, func(i, j int) bool {
			if due[i].due.Equal(due[j].due) {
				return due[i].id < due[j].id
			}
			return due[i].due.Before(due[j].due)
		})
		ids := make([]string, len(due))
		for i, item := range due {
			ids[i] = item.id
		}
		return ids, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]string), nil
}

// runOneJob 在独立事务中执行单个作业。
// @param ctx 上下文
// @param jobID 作业 ID
// @return 执行数量
// @return 执行错误
func (e *Engine) runOneJob(ctx context.Context, jobID string) (int, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		job, ok := e.state.Jobs[jobID]
		if !ok {
			return 0, nil
		}
		backup := cloneState(e.state)
		delete(e.state.Jobs, job.ID)
		handleFailure := func(runErr error) (any, error) {
			e.state = backup
			e.handleJobFailure(job, runErr, cc.now)
			return 1, nil
		}
		if runErr := e.triggerJob(cc, job); runErr != nil {
			return handleFailure(runErr)
		}
		if runErr := cc.agenda.run(cc); runErr != nil {
			return handleFailure(runErr)
		}
		return 1, nil
	})
	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

// handleJobFailure 处理作业失败：重试或移入死信。
// @param job 作业
// @param err 失败原因
// @param now 当前时间
func (e *Engine) handleJobFailure(job *jobState, err error, now time.Time) {
	retries := job.Retries
	if retries <= 0 {
		retries = 3
	}
	delete(e.state.Jobs, job.ID)
	job.Exception = err.Error()
	if retries <= 1 {
		job.Retries = 0
		e.state.DeadLetters[job.ID] = job
		return
	}
	job.Retries = retries - 1
	job.DueDate = now.Add(10 * time.Second)
	e.state.Jobs[job.ID] = job
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
		if err != nil {
			return err
		}
		e.rescheduleTimerJob(job, cc.now)
		return nil
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
		Retries:           3,
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
	// 非中断重复边界定时器：重新排期
	e.rescheduleTimerJob(job, cc.now)
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
			Retries:           3,
			ProcessInstanceID: execution.ProcessInstanceID,
			ExecutionID:       execution.ID,
			BoundaryEventID:   boundary.ID,
			ActivityID:        element.GetID(),
			CancelActivity:    boundary.CancelActivity,
			DueDate:           due,
			CreatedAt:         cc.now,
		}
		if boundary.Timer.TimeCycle != "" {
			cycle, cycleErr := parseTimerCycle(boundary.Timer.TimeCycle)
			if cycleErr != nil {
				return api.NewError("ENGINE-0605", "边界定时器重复周期解析失败", cycleErr)
			}
			job.RepeatRemaining = cycle.remaining
			job.RepeatInfinite = cycle.infinite
			job.IntervalMillis = cycle.interval.Milliseconds()
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
			Retries:             3,
			ProcessDefinitionID: definition.Info.ID,
			ActivityID:          start.ID,
			DueDate:             due,
			CreatedAt:           cc.now,
		}
		if start.Timer.TimeCycle != "" {
			cycle, cycleErr := parseTimerCycle(start.Timer.TimeCycle)
			if cycleErr != nil {
				return api.NewError("ENGINE-0604", "定时启动重复周期解析失败", cycleErr)
			}
			job.RepeatRemaining = cycle.remaining
			job.RepeatInfinite = cycle.infinite
			job.IntervalMillis = cycle.interval.Milliseconds()
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
		Retries:           3,
		ProcessInstanceID: execution.ProcessInstanceID,
		ExecutionID:       execution.ID,
		ActivityID:        activityID,
		DueDate:           due,
		CreatedAt:         cc.now,
	}
	e.state.Jobs[job.ID] = job
	return nil
}

// timerCycle 是重复定时器配置。
type timerCycle struct {
	interval  time.Duration // 间隔
	remaining int           // 首次触发后的剩余次数
	infinite  bool          // 是否无限
}

// parseTimerCycle 解析 ISO-8601 重复周期。
// @param value 重复表达式，如 R3/PT1M
// @return 周期配置
// @return 解析错误
func parseTimerCycle(value string) (timerCycle, error) {
	parts := strings.Split(value, "/")
	if len(parts) < 2 {
		return timerCycle{}, fmt.Errorf("非法重复周期: %s", value)
	}
	interval, err := parseISODuration(parts[len(parts)-1])
	if err != nil {
		return timerCycle{}, err
	}
	countPart := parts[0]
	if !strings.HasPrefix(strings.ToUpper(countPart), "R") {
		return timerCycle{}, fmt.Errorf("重复周期缺少 R 前缀: %s", value)
	}
	digits := countPart[1:]
	if digits == "" {
		return timerCycle{interval: interval, infinite: true}, nil
	}
	count, err := strconv.Atoi(digits)
	if err != nil || count <= 0 {
		return timerCycle{}, fmt.Errorf("非法重复次数: %s", countPart)
	}
	return timerCycle{interval: interval, remaining: count - 1}, nil
}

// rescheduleTimerJob 为重复定时器创建下一次作业。
// @param job 已触发的作业
// @param now 当前时间
func (e *Engine) rescheduleTimerJob(job *jobState, now time.Time) {
	if job.IntervalMillis <= 0 || (!job.RepeatInfinite && job.RepeatRemaining <= 0) {
		return
	}
	next := *job
	next.ID = e.newID()
	next.DueDate = now.Add(time.Duration(job.IntervalMillis) * time.Millisecond)
	next.Exception = ""
	if !next.RepeatInfinite {
		next.RepeatRemaining--
	}
	e.state.Jobs[next.ID] = &next
}
