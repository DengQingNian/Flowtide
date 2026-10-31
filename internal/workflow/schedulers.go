// schedulers.go 提供 workflow 调度器（P4.4-P4.5，平铺于 internal/workflow 包以贴合现包风格；
// 对位 Java FlowableScheduleTask / ReminderSchedule / ExceptionMonitorSchedule）：
// 1) 自动办结 AutoCompleteTick（noHandleAutoComplete 开关，5 分钟，逾期待办按预存下一步自动完成/退回）；
// 2) 超时提醒 TimeoutRemindTick（task-timeout-remind 开关，每日 8 点）；
// 3) 催办扫描 RemindScanTick（send-workflow-remind 开关，30 秒，RemindService.ScanDue → 送达）；
// 4) 僵尸监控 ZombieScanTick（5 分钟，运行中实例与有待办实例差集 → NotTaskMessageException）。
// 全部 tick 可注入 trigger 函数并支持测试直调（不真 sleep）。
package workflow

import (
	"context"
	"sync"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
)

// 调度周期常量（对位 Java @Scheduled 注解取值）。
const (
	// AutoCompleteInterval 自动办结扫描周期：5 分钟（Java cron "0 */5 * * * ?"）。
	AutoCompleteInterval = 5 * time.Minute
	// RemindScanInterval 催办扫描周期：30 秒（Java fixedDelay=30000）。
	RemindScanInterval = 30 * time.Second
	// ZombieScanInterval 僵尸监控周期：5 分钟（Java fixedDelay=1000*60*5）。
	ZombieScanInterval = 5 * time.Minute
	// TimeoutRemindHour 超时提醒每日触发时刻：8 点（Java cron "0 0 8 * * ?"）。
	TimeoutRemindHour = 8
)

// AutoCompleteBackExecutor 是自动办结退回执行器端口（P2-B BackService 完成后由主控经适配器接线；
// nil 时退回型自动办结跳过并保持待办不变）。
type AutoCompleteBackExecutor interface {
	// BackToStepTask 退回任务到指定步骤（对位 Java processesService.backToStepTask）。
	// @param ctx 操作上下文
	// @param vo 退回请求（TargetActivityID 必填）
	// @return 错误
	BackToStepTask(ctx context.Context, vo *api.BackTaskVo) error
}

// SchedulerConfig 是调度器装配配置（全部依赖注入，测试可用轻量桩）。
type SchedulerConfig struct {
	Engine     api.Engine // 引擎门面（僵尸监控实例查询）
	Options    Options    // 调度开关与时钟（Service.Options() 透传）
	TodoSource TodoSource // 待办数据源（自动办结/超时提醒/僵尸监控）
	Rules      ScheduleRuleProvider // 步骤规则（autoComplete/timeLimit），nil 用空实现
	Complete   *CompleteService     // 自动办结（同意型）执行器
	Next       *NextActivityService // 下一步预存读取（自动办结的下一步来源）
	Back       AutoCompleteBackExecutor // 退回执行器（P2-B 后接线，nil 跳过退回型）
	Remind     *RemindService       // 催办服务（扫描 + 送达）
	Notifier   Notifier             // 催办送达器，nil 用日志实现
	Exceptions *ExceptionService    // 异常记录服务（僵尸监控落库）
}

// Scheduler 是 workflow 调度器聚合：Start 启动后台 goroutine，Stop 优雅停止；
// 四个 tick 方法可单独直调（测试不真 sleep），亦可经 AutoCompleteFn 等字段替换触发实现。
type Scheduler struct {
	opts       Options              // 调度开关与时钟
	engine     api.Engine           // 引擎门面
	todoSource TodoSource           // 待办数据源
	rules      ScheduleRuleProvider // 步骤规则
	complete   *CompleteService     // 自动办结执行器
	next       *NextActivityService // 预存读取
	back       AutoCompleteBackExecutor // 退回执行器
	remind     *RemindService       // 催办服务
	notifier   Notifier             // 送达器
	exceptions *ExceptionService    // 异常记录服务

	// 可注入触发函数（nil 时使用内置 tick；测试可替换以断言调度触发）
	AutoCompleteFn func(ctx context.Context) error // 自动办结触发函数
	TimeoutRemindFn func(ctx context.Context) error // 超时提醒触发函数
	RemindScanFn   func(ctx context.Context) error // 催办扫描触发函数
	ZombieScanFn   func(ctx context.Context) error // 僵尸监控触发函数

	autoCompleteInterval time.Duration // 自动办结周期
	remindInterval       time.Duration // 催办扫描周期
	zombieInterval       time.Duration // 僵尸监控周期
	timeoutHour          int           // 超时提醒每日时刻

	stop chan struct{} // 停止信号
	done chan struct{} // 停止完成信号
	mu   sync.Mutex    // 保护 running
}

// NewScheduler 创建调度器（不启动，需显式 Start）。
// @param cfg 装配配置
// @return 调度器
func NewScheduler(cfg SchedulerConfig) *Scheduler {
	c := cfg.Options.Clock
	// 未注入时钟回退真实时钟（与门面装配一致；同步回写保证 clockNow 判空无需二次兜底）
	if c == nil {
		c = clock.Real{}
	}
	cfg.Options.Clock = c
	rules := cfg.Rules
	// 规则缺省空实现（his_step 规则接线前全部判空跳过）
	if rules == nil {
		rules = NopScheduleRuleProvider{}
	}
	notifier := cfg.Notifier
	// 送达器缺省日志实现（P6 接真实渠道）
	if notifier == nil {
		notifier = LogNotifier{}
	}
	return &Scheduler{
		opts:                 cfg.Options,
		engine:               cfg.Engine,
		todoSource:           cfg.TodoSource,
		rules:                rules,
		complete:             cfg.Complete,
		next:                 cfg.Next,
		back:                 cfg.Back,
		remind:               cfg.Remind,
		notifier:             notifier,
		exceptions:           cfg.Exceptions,
		autoCompleteInterval: AutoCompleteInterval,
		remindInterval:       RemindScanInterval,
		zombieInterval:       ZombieScanInterval,
		timeoutHour:          TimeoutRemindHour,
		// stop/done 缺省 nil（未启动态），由 Start 创建、Stop 复位
	}
}

// Start 启动调度器后台循环（重复调用幂等；Stop 后可重启）。
// @param ctx 上下文（传递给各 tick；取消后循环内的 tick 以取消态执行）
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 已启动则幂等返回（stop 非 nil 表示循环在跑，与 Stop 的 stop=nil 约定一致）
	if s.stop != nil {
		return
	}
	// 未启动（含 Stop 后重启）：重建停止/完成通道
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	stop, done := s.stop, s.done
	go func() {
		defer close(done)
		s.loop(ctx, stop)
	}()
}

// Stop 停止调度器并等待循环退出（未启动时直接返回）。
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if s.stop == nil {
		s.mu.Unlock()
		return
	}
	stop, done := s.stop, s.done
	s.stop = nil
	s.done = make(chan struct{})
	s.mu.Unlock()
	close(stop)
	// 等待循环退出（循环内 tick 均为快查询，2 秒兜底避免外部死等）
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}

// loop 调度主循环：三个 ticker + 一个对齐每日时刻的 timer（Stop 关闭后全部释放）。
// @param ctx 上下文
// @param stop 停止信号
func (s *Scheduler) loop(ctx context.Context, stop <-chan struct{}) {
	acTicker := time.NewTicker(s.autoCompleteInterval)
	defer acTicker.Stop()
	rmTicker := time.NewTicker(s.remindInterval)
	defer rmTicker.Stop()
	zbTicker := time.NewTicker(s.zombieInterval)
	defer zbTicker.Stop()
	// 超时提醒对齐每日 timeoutHour 点（启动即计算下次触发；wall-clock 真实等待，测试经直调 tick 绕过）
	timeoutTimer := time.NewTimer(time.Until(nextDailyRun(s.clockNow(), s.timeoutHour)))
	defer timeoutTimer.Stop()
	for {
		select {
		case <-stop:
			return
		case <-acTicker.C:
			_ = s.runAutoComplete(ctx)
		case <-rmTicker.C:
			_ = s.runRemindScan(ctx)
		case <-zbTicker.C:
			_ = s.runZombieScan(ctx)
		case <-timeoutTimer.C:
			_ = s.runTimeoutRemind(ctx)
			// 触发后重排到下一个每日时刻
			timeoutTimer.Reset(time.Until(nextDailyRun(s.clockNow(), s.timeoutHour)))
		}
	}
}

// clockNow 读取当前时间（时钟恒非 nil，构造时已兜底）。
// @return 当前时间
func (s *Scheduler) clockNow() time.Time {
	return s.opts.Clock.Now()
}

// runAutoComplete 调用注入的触发函数或内置自动办结 tick。
// @param ctx 上下文
// @return 错误
func (s *Scheduler) runAutoComplete(ctx context.Context) error {
	// 注入触发函数优先（测试直调通道）
	if s.AutoCompleteFn != nil {
		return s.AutoCompleteFn(ctx)
	}
	return s.AutoCompleteTick(ctx)
}

// runTimeoutRemind 调用注入的触发函数或内置 tick。
// @param ctx 上下文
// @return 错误
func (s *Scheduler) runTimeoutRemind(ctx context.Context) error {
	if s.TimeoutRemindFn != nil {
		return s.TimeoutRemindFn(ctx)
	}
	return s.TimeoutRemindTick(ctx)
}

// runRemindScan 调用注入的触发函数或内置 tick。
// @param ctx 上下文
// @return 错误
func (s *Scheduler) runRemindScan(ctx context.Context) error {
	if s.RemindScanFn != nil {
		return s.RemindScanFn(ctx)
	}
	return s.RemindScanTick(ctx)
}

// runZombieScan 调用注入的触发函数或内置 tick。
// @param ctx 上下文
// @return 错误
func (s *Scheduler) runZombieScan(ctx context.Context) error {
	if s.ZombieScanFn != nil {
		return s.ZombieScanFn(ctx)
	}
	return s.ZombieScanTick(ctx)
}

// nextDailyRun 计算下一个每日 hour 点整的触发时间（已过当日 hour 点则排到次日）。
// @param now 当前时间
// @param hour 触发时刻（0-23）
// @return 下次触发时间
func nextDailyRun(now time.Time, hour int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
	// 当日时刻已过则排次日（含恰好等于当前时间的边界：取次日避免 0 间隔忙转）
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// ─── 自动办结 ─────────────────────────────────────────────

// AutoCompleteTick 自动办结扫描（对位 Java FlowableScheduleTask#autoCompleteTask:44）：
// 逾期待办（autoComplete=1/2 且 timeLimit 到期）按预存下一步自动完成（意见"同意（自动办结）"type=SP），
// autoComplete=2 且 allowReturn 走退回执行器（意见"退回（自动办结）"）。
// P5 依赖缺口：Java 经推演服务 getNextUserTaskAndUserList/getBackNodesByTaskId 决定去向，
// Go 推演服务未实现（P5 补），本实现以"同意方向预存记录 / 退回目标预存记录"为去向来源，
// 预存缺失时跳过该待办（记录日志不报错）。
// @param ctx 操作上下文
// @return 错误
func (s *Scheduler) AutoCompleteTick(ctx context.Context) error {
	// 开关关闭不触发（Java 同款首行判开关）
	if !s.opts.NoHandleAutoComplete {
		return nil
	}
	// 待办数据源缺失无事可做
	if s.todoSource == nil {
		return nil
	}
	todos, err := s.todoSource.ListAll(ctx)
	if err != nil {
		return err
	}
	now := s.clockNow()
	for _, todo := range todos {
		// 逐待办判定并执行，单条失败不阻断其余待办
		if err := s.autoCompleteOne(ctx, todo, now); err != nil {
			continue
		}
	}
	return nil
}

// autoCompleteOne 处理单条逾期待办的自动办结。
// @param ctx 操作上下文
// @param todo 待办
// @param now 当前时间
// @return 错误
func (s *Scheduler) autoCompleteOne(ctx context.Context, todo *MessageExtend, now time.Time) error {
	// 步骤规则读取失败按无规则跳过
	rule, err := s.rules.AutoCompleteRuleOf(ctx, todo.TaskStepCode)
	if err != nil {
		return nil
	}
	// 未配置自动办结类型跳过
	if rule.AutoComplete != 1 && rule.AutoComplete != 2 {
		return nil
	}
	// 逾期判定：任务接收时间 + timeLimit <= now（timeLimit 未配置无法判定）
	if !s.overdue(todo, rule, now) {
		return nil
	}
	// 同意型：按预存下一步自动完成
	if rule.AutoComplete == 1 {
		return s.autoCompleteAgree(ctx, todo)
	}
	// 退回型：需允许退回且退回执行器已接线（P2-B BackService 完成后主控接线，nil 跳过）
	if rule.AllowReturn == 0 || s.back == nil {
		return nil
	}
	return s.autoCompleteReturn(ctx, todo, rule)
}

// overdue 逾期判定（对位 Java getAutoCompleteTaskMessageList 的 DATE_ADD 时限计算）：
// 任务接收时间 + timeLimit <= now；接收时间或时限缺失返回 false。
// @param todo 待办
// @param rule 自动办结规则
// @param now 当前时间
// @return 是否逾期
func (s *Scheduler) overdue(todo *MessageExtend, rule AutoCompleteRule, now time.Time) bool {
	// 时限未配置或接收时间缺失无法判定
	if rule.TimeLimit == nil || todo.TaskReceiptTime == nil {
		return false
	}
	due := todo.TaskReceiptTime.Add(time.Duration(durationMillis(*rule.TimeLimit, rule.TimeUnit)) * time.Millisecond)
	return !due.After(now)
}

// autoCompleteAgree 同意型自动办结（对位 Java agreeTask:129）：以预存下一步记录组装
// CompleteTaskVo（意见"同意（自动办结）"type=SP），经办结服务完成并走完整收尾链
// （预存消耗/待办/已办/表单状态机）。
// @param ctx 操作上下文
// @param todo 逾期待办
// @return 错误
func (s *Scheduler) autoCompleteAgree(ctx context.Context, todo *MessageExtend) error {
	// 依赖缺失无法执行（装配由主控保证，测试可单独注入）
	if s.complete == nil || s.next == nil {
		return nil
	}
	// 预存下一步记录（同意方向：pre_task_id=当前任务、used=0）
	records, err := s.next.ListByPreTask(ctx, todo.TaskID)
	if err != nil {
		return err
	}
	// 预存缺失：推演服务（P5）未实现前的降级——跳过该待办
	if len(records) == 0 {
		return nil
	}
	vo := &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{
			TaskID:            todo.TaskID,
			ProcessInstanceID: todo.ProcessInstanceID,
			UserCode:          todo.OwnerID,
			Message:           AutoCompleteAgreeMessage,
			Type:              CommentTypeSP,
		},
		Request: nextActivityRequestOf(records),
	}
	// 经办结服务自动完成（内部含表单状态机刷新，Java 补调 updateWorkflowDocument 已并入收尾链）
	_, err = s.complete.Complete(ctx, vo)
	return err
}

// autoCompleteReturn 退回型自动办结（对位 Java backTask:181）：以退回目标的预存记录组装
// BackTaskVo（意见"退回（自动办结）"），经注入的退回执行器完成。
// 退回目标取步骤规则的 back_step_codes 首个；目标预存缺失时跳过（推演服务 P5 补）。
// @param ctx 操作上下文
// @param todo 逾期待办
// @param rule 自动办结规则
// @return 错误
func (s *Scheduler) autoCompleteReturn(ctx context.Context, todo *MessageExtend, rule AutoCompleteRule) error {
	// 退回目标步骤代号缺失无法定位退回节点
	if rule.BackStepCode == "" {
		return nil
	}
	// 退回目标的执行人取该节点预存记录（退回服务自身也有兜底，此处提供 ExecutorIDs 供其还原）
	var executorIDs []string
	if s.next != nil {
		records, err := s.next.ListUnusedByInstanceAndActivity(ctx, todo.ProcessInstanceID, rule.BackStepCode)
		if err != nil {
			return err
		}
		for _, record := range records {
			executorIDs = append(executorIDs, record.IdentityID)
		}
		// 目标节点无预存记录：退回去向未知（推演服务 P5 补齐后由主控增强），跳过该待办
		if len(executorIDs) == 0 {
			return nil
		}
	}
	vo := &api.BackTaskVo{
		TaskID:           todo.TaskID,
		TargetActivityID: rule.BackStepCode,
		UserCode:         todo.OwnerID,
		Message:          AutoCompleteReturnMessage,
		Type:             CommentTypeTH,
	}
	// 经退回执行器自动退回（P2-B BackService 适配器由主控注入）
	return s.back.BackToStepTask(ctx, vo)
}

// nextActivityRequestOf 将预存记录按节点聚合为预存请求（同节点多执行人合并）。
// @param records 预存记录列表
// @return 预存请求
func nextActivityRequestOf(records []*NextActivity) *api.NextActivityRequest {
	request := &api.NextActivityRequest{NextActivityList: []*api.NextActivityVo{}}
	index := make(map[string]int, len(records))
	for _, record := range records {
		if record.Used != 0 {
			continue
		}
		vo := &api.NextActivityVo{ActivityID: record.NextActivityID}
		if position, ok := index[record.NextActivityID]; ok {
			// 同节点执行人合并到既有条目
			request.NextActivityList[position].ExecutorIDs = append(request.NextActivityList[position].ExecutorIDs, record.IdentityID)
			continue
		}
		vo.ExecutorIDs = []string{record.IdentityID}
		index[record.NextActivityID] = len(request.NextActivityList)
		request.NextActivityList = append(request.NextActivityList, vo)
	}
	return request
}

// ─── 超时提醒 ─────────────────────────────────────────────

// TimeoutRemindTick 超时提醒（对位 Java FlowableScheduleTask#taskTimeoutRemind:114，每日 8 点）：
// 超时待办（接收时间 + 步骤催办时限已过）逐条送达"任务超时提醒"。
// @param ctx 操作上下文
// @return 错误
func (s *Scheduler) TimeoutRemindTick(ctx context.Context) error {
	// 开关关闭不触发
	if !s.opts.TaskTimeoutRemind {
		return nil
	}
	// 依赖缺失无事可做
	if s.todoSource == nil {
		return nil
	}
	todos, err := s.todoSource.ListAll(ctx)
	if err != nil {
		return err
	}
	now := s.clockNow()
	for _, todo := range todos {
		// 单条失败不阻断其余待办
		if err := s.timeoutRemindOne(ctx, todo, now); err != nil {
			continue
		}
	}
	return nil
}

// timeoutRemindOne 送达单条超时提醒（Java msgRemind："您的【步骤】任务已超时，请及时处理。"）。
// @param ctx 操作上下文
// @param todo 待办
// @param now 当前时间
// @return 错误
func (s *Scheduler) timeoutRemindOne(ctx context.Context, todo *MessageExtend, now time.Time) error {
	// 催办时限按步骤规则读取（未配置的步骤不提醒）
	limit, unit, err := s.rules.RemindLimitOf(ctx, todo.TaskStepCode)
	if err != nil || limit == nil {
		return nil
	}
	// 接收时间缺失无法推算到期
	if todo.TaskReceiptTime == nil {
		return nil
	}
	due := todo.TaskReceiptTime.Add(time.Duration(durationMillis(*limit, unit)) * time.Millisecond)
	// 未超时跳过
	if due.After(now) {
		return nil
	}
	channels, err := s.channelsOf(ctx, todo)
	if err != nil {
		return err
	}
	// 经送达器提醒待办处理人（Java msgRemind 文案）
	return s.notifier.Notify(ctx, &RemindNotification{
		ReceiverID: todo.OwnerID,
		Subject:    todo.Title,
		Content:    "您的【" + todo.TaskStepName + "】任务已超时，请及时处理。",
		Channels:   channels,
		Kind:       RemindKindTimeout,
	})
}

// channelsOf 读取待办渠道（催办服务注入时复用其渠道提供方，否则返回空）。
// @param ctx 操作上下文
// @param todo 待办
// @return 渠道列表
// @return 错误
func (s *Scheduler) channelsOf(ctx context.Context, todo *MessageExtend) ([]string, error) {
	// 未装配催办服务时无渠道来源
	if s.remind == nil {
		return nil, nil
	}
	return s.remind.channels.ChannelsOf(ctx, todo)
}

// ─── 催办扫描 ─────────────────────────────────────────────

// RemindScanTick 催办扫描（对位 Java ReminderSchedule#updateWithCheckWorkflowReminds:92）：
// RemindService.ScanDue 判定到期命中 → 逐命中送达（展开对象 → Notifier）→ sent/retry 标记。
// @param ctx 操作上下文
// @return 错误
func (s *Scheduler) RemindScanTick(ctx context.Context) error {
	// 开关关闭不触发
	if !s.opts.SendWorkflowRemind {
		return nil
	}
	// 催办服务缺失无事可做
	if s.remind == nil {
		return nil
	}
	hits, err := s.remind.ScanDue(ctx, s.clockNow())
	if err != nil {
		return err
	}
	for _, hit := range hits {
		// 逐命中送达（对象展开/去重/标记均在 Deliver 内），单条失败不阻断其余
		if _, err := s.remind.Deliver(ctx, hit, s.notifier); err != nil {
			continue
		}
	}
	return nil
}

// ─── 僵尸监控 ─────────────────────────────────────────────

// ZombieScanTick 僵尸流程监控（对位 Java ExceptionMonitorSchedule#saveflowTaskException:65）：
// 运行中实例与有待办实例求差集，无待办的运行实例记 NotTaskMessageException（"流程未生成待办"），
// 已记录的实例不重复落库。
// @param ctx 操作上下文
// @return 错误
func (s *Scheduler) ZombieScanTick(ctx context.Context) error {
	// 异常服务或实例查询缺失无事可做
	if s.exceptions == nil || s.engine == nil {
		return nil
	}
	page, err := s.engine.RuntimeService().QueryProcessInstances(ctx, api.ProcessInstanceFilter{Size: zombieScanPageSize})
	if err != nil {
		return err
	}
	if len(page.Data) == 0 {
		return nil
	}
	// 有待办的实例集合
	todos, err := s.todoSource.ListAll(ctx)
	if err != nil {
		return err
	}
	withTodo := make(map[string]bool, len(todos))
	for _, todo := range todos {
		withTodo[todo.ProcessInstanceID] = true
	}
	// 差集：运行中且无待办
	zombies := make([]api.ProcessInstance, 0, 4)
	for _, instance := range page.Data {
		// 有待办的实例非僵尸
		if withTodo[instance.ID] {
			continue
		}
		zombies = append(zombies, instance)
	}
	// 逐实例记录（RecordZombieInstance 内部对已记录实例去重跳过）
	for _, instance := range zombies {
		if _, err := s.exceptions.RecordZombieInstance(ctx, instance); err != nil {
			return err
		}
	}
	return nil
}

// zombieScanPageSize 是僵尸监控单次扫描的实例数量上限（单租户运行实例数量级远低于该值）。
const zombieScanPageSize = 500
