// executor_rewrite.go 实现执行人重写全局钩子（P1-D 核心闭环后半触发端）。
// 对位 Java 全局监听器 TaskExecutorConfigListener：任务创建时若实例变量 operationType
// 命中门控（agreeComplete/completeToTarget），读取"下一步预生成-消耗"台账（used=0 记录）
// 回填当前任务的办理人/候选人/候选组，并置 used=1、回写 task_id。
// 与 Java 的差异（规格决策）：
//  1. 非会签节点未找到未消耗记录时保留模型默认执行人（兜底），不像 Java 直接抛错；
//  2. 多条记录语义为整体重写候选用户并清空默认办理人（Java candidateUsers 分支
//     "删默认候选重建"的等价形态，SetCandidateUsers 为整体重写）；
//  3. 会签节点每次实例创建消费"恰好一条"未消耗记录（见 rewriteCountersign 注释）。
package workflow

import (
	"context"
	"sort"
	"sync"

	"github.com/DengQingNian/Flowtide/api"
)

// ExecutorRewriteHook 是任务创建全局钩子（执行人重写，TASK_CREATED）。
// 依赖模型元数据服务（读节点执行者配置）与下一步预生成域服务（读/消耗台账）。
// 业务回调 client（P1-C CallbackClient）本阶段不接入，预留构造扩展。
//
// 模型元数据的读取约束（重要）：钩子在引擎命令内同步执行，命令持有引擎互斥写锁，
// 钩子内调用任何引擎公有 API（含 GetBpmnModel）都会重入死锁（已用探针测试证实）。
// 因此本钩子不直接调用 ModelMetaService.UserTaskMeta，而是通过 Warmup 预热的
// 只读缓存取节点元数据；缓存未命中时降级为保留模型默认执行人（兜底），并登记
// 延迟预热动作，供后续任务命中缓存。接线约定：部署新定义/发起后由门面调用
// Warmup（或启动时 WarmupAll）。
type ExecutorRewriteHook struct {
	meta  *ModelMetaService                   // 模型元数据服务（预热数据源）
	next  *NextActivityService                // 下一步预生成-消耗域服务（读 used=0 与回写 used=1）
	mu    sync.RWMutex                        // 保护元数据缓存（命令内只读、锁外预热写）
	cache map[string]map[string]*UserTaskMeta // 定义 ID → 节点 ID → 元数据摘要
}

// NewExecutorRewriteHook 构造执行人重写钩子。
// @param meta 模型元数据服务
// @param next 下一步预生成域服务
// @return 执行人重写钩子
func NewExecutorRewriteHook(meta *ModelMetaService, next *NextActivityService) *ExecutorRewriteHook {
	return &ExecutorRewriteHook{
		meta:  meta,
		next:  next,
		cache: make(map[string]map[string]*UserTaskMeta),
	}
}

// Register 将钩子注册到引擎任务创建事件（TASK_CREATED）。
// 注册后所有用户任务创建命令都会同步执行 Hook；调用方须在引擎使用前完成注册，
// 并按接线约定完成元数据预热（Warmup/WarmupAll）。
// @param eng 引擎门面
func (h *ExecutorRewriteHook) Register(eng api.Engine) {
	eng.AddTaskCreatedHook(h.Hook)
}

// Warmup 预热指定流程定义的全部用户任务元数据缓存（锁外调用）。
// 必须在引擎命令外调用（部署后/发起后/启动时），命令内调用会与引擎写锁重入死锁。
// @param ctx 操作上下文
// @param definitionID 流程定义 ID
// @return 错误
func (h *ExecutorRewriteHook) Warmup(ctx context.Context, definitionID string) error {
	metas, err := h.meta.ListUserTaskMetas(ctx, definitionID)
	if err != nil {
		return err
	}
	index := make(map[string]*UserTaskMeta, len(metas))
	for _, item := range metas {
		index[item.ActivityID] = item
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cache[definitionID] = index
	return nil
}

// WarmupAll 预热引擎当前全部已部署定义的元数据缓存（启动接线用，锁外调用）。
// @param ctx 操作上下文
// @return 错误
func (h *ExecutorRewriteHook) WarmupAll(ctx context.Context) error {
	definitions, err := h.meta.engine.RepositoryService().ListProcessDefinitions(ctx)
	if err != nil {
		return err
	}
	for _, definition := range definitions {
		// 逐定义预热；单个定义失败不阻断其余（该定义任务将走兜底保留模型默认）
		if err := h.Warmup(ctx, definition.ID); err != nil {
			return err
		}
	}
	return nil
}

// cachedUserTaskMeta 命令内只读查询预热的节点元数据（未预热返回 nil）。
// @param definitionID 流程定义 ID
// @param activityID 节点 ID
// @return 元数据摘要，未预热为 nil
func (h *ExecutorRewriteHook) cachedUserTaskMeta(definitionID, activityID string) *UserTaskMeta {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if index, ok := h.cache[definitionID]; ok {
		return index[activityID]
	}
	return nil
}

// Hook 执行人重写钩子主体（api.TaskHook 签名）。
// 流程：门控判定（任务快照变量）→ 读预热的模型执行者配置缓存 → 读未消耗预生成记录 →
// 非会签/会签分支回填。查询失败或回填失败一律返回错误，由引擎使当前命令整体回滚
// （与 Java isFailOnException 相反：Go 引擎侧规格要求错误传播，避免"记录被跳过消耗"
// 的静默错账）。模型元数据缓存未命中时降级保留模型默认执行人并登记延迟预热
// （命令内禁止调用引擎公有 API，会重入死锁）。
// @param ctx 命令上下文（与所在引擎命令共用，命令失败整体回滚）
// @param task 任务快照（含任务可见变量，改写快照无效，必须经 ctl）
// @param ctl 钩子控制（命令内改写办理人/候选人/候选组）
// @return 错误（非空则当前引擎命令失败回滚）
func (h *ExecutorRewriteHook) Hook(ctx context.Context, task *api.Task, ctl api.HookControl) error {
	// 快照缺失（防御）或门控变量未命中时不做任何改写
	if task == nil || ctl == nil || !rewriteGateEnabled(task.Variables) {
		return nil
	}
	// 命令内只读缓存取节点元数据；未预热时降级兜底并延迟预热（锁外执行）
	meta := h.cachedUserTaskMeta(task.ProcessDefinitionID, task.TaskDefinitionKey)
	if meta == nil {
		ctl.Defer(func(deferCtx context.Context) error { return h.Warmup(deferCtx, task.ProcessDefinitionID) })
		return nil
	}
	// 读取该实例该节点的未消耗（used=0）预生成记录
	records, err := h.next.ListUnusedByInstanceAndActivity(ctx, task.ProcessInstanceID, task.TaskDefinitionKey)
	if err != nil {
		return err
	}
	// 会签节点（配置了多实例循环特征）走按实例消费分支
	if meta.MultiInstance != nil {
		return h.rewriteCountersign(ctx, task, ctl, meta, records)
	}
	return h.rewritePlain(ctx, task, ctl, records)
}

// rewriteGateEnabled 门控判定：任务可见变量 operationType 命中合法操作类型时才执行重写。
// 对位 Java TaskExecutorConfigListener 的 "agreeComplete".equals / "completeToTarget".equals 判定。
// 变量读取只使用任务快照（钩子在引擎命令内同步执行，禁止调用引擎公有 API，会重入死锁）。
// @param variables 任务可见变量快照
// @return 是否命中门控
func rewriteGateEnabled(variables map[string]any) bool {
	// 无变量快照直接不触发（快照语义由引擎保证：创建任务命令内必带任务可见变量）
	if len(variables) == 0 {
		return false
	}
	switch variables[VarOperationType] {
	// 两种合法操作类型：同意办结与直送目标节点（与 Java 逐字一致）
	case OperationTypeAgreeComplete, OperationTypeCompleteToTarget:
		return true
	default:
		return false
	}
}

// rewritePlain 非会签节点的执行人回填。
// 汇总未消耗记录的执行人/候选组：单条执行人 → SetAssignee；多条执行人 →
// SetCandidateUsers 整体重写并清空默认办理人（Java candidateUsers 分支删默认候选重建
// 语义）；仅候选组记录 → SetCandidateGroups。回填后整批 MarkUsedAndWriteBack。
// 无可用记录或记录脏数据（执行人与组全空）时保留模型默认执行人（兜底返回 nil）。
// @param ctx 操作上下文
// @param task 任务快照
// @param ctl 钩子控制
// @param records 未消耗预生成记录
// @return 错误
func (h *ExecutorRewriteHook) rewritePlain(ctx context.Context, task *api.Task, ctl api.HookControl, records []*NextActivity) error {
	// 兜底：无未消耗记录时保留模型默认执行人（规格决策，区别于 Java 抛错）
	if len(records) == 0 {
		return nil
	}
	identities := distinctNonEmpty(identityIDsOf(records))
	groupIDs := distinctNonEmpty(groupIDsOf(records))
	// 聚合结果按字典序输出：记录来自存储端口，同版本同创建时间的记录顺序不定
	//（内存实现按 map 迭代），排序保证回填集合确定性（集合语义，顺序无业务含义）
	sort.Strings(identities)
	sort.Strings(groupIDs)
	// 记录存在但执行人与候选组全空（脏记录）：同样兜底保留模型默认，不消费
	if len(identities) == 0 && len(groupIDs) == 0 {
		return nil
	}
	switch {
	// 单执行人：直接改写办理人（Java identityId 分支）
	case len(identities) == 1:
		if err := ctl.SetAssignee(task.ID, identities[0]); err != nil {
			return err
		}
	// 多执行人：整体重写候选用户并清空默认办理人（Java 删默认候选重建的等价形态）
	case len(identities) > 1:
		if err := ctl.SetCandidateUsers(task.ID, identities); err != nil {
			return err
		}
		// 清空默认办理人：多人语义下不应保留单一代办人
		if err := ctl.SetAssignee(task.ID, ""); err != nil {
			return err
		}
	default:
		// 记录仅携带候选组（无执行人）：只重写候选组
	}
	// 记录带候选组时同步整体重写候选组（组预生成记录为"每人一行+共享组"形态）
	if len(groupIDs) > 0 {
		if err := ctl.SetCandidateGroups(task.ID, groupIDs); err != nil {
			return err
		}
	}
	// 整批消耗：置 used=1 并回写当前任务 ID（运行态与历史表同步）
	return h.next.MarkUsedAndWriteBack(ctx, recordIDsOf(records), task.ID)
}

// rewriteCountersign 会签（多实例）节点的执行人回填。
//
// 消费约定（幂等性核心）：多实例每创建一个实例都会触发一次本钩子；每次调用
// ListUnusedByInstanceAndActivity 只能看到当前未被消耗的记录，本钩子按记录顺序
// 消费"恰好一条"，消费后该记录 used=1 并回写本实例任务 ID，后续实例的钩子调用
// 不再看到它。因此预生成 N 条记录时，N 个实例依次创建即按序消费 N 条，一一对应、
// 互不重复；多实例完成/回滚使命令失败时，消耗与任务创建同事务回滚，记录恢复
// 未消耗，重试后仍按序消费。
//
// 记录顺序 =（版本号升序，创建时间升序，记录 Id 升序）三级排序：同批次预生成
// 记录版本与创建时间可能相同，存储端口对同键记录的顺序不定（内存实现按 map
// 迭代），故钩子内按 Id 终局排序（UUIDv7 时间有序，等价插入顺序）保证确定性。
//
// executorType=person：优先按当前实例办理人匹配同身份记录（对位 Java
// getCountersignAssigneeListByProcInstAndTaskKey），实例本无办理人时按序消费
// 第一条；executorType=group：按任务第一个候选组匹配记录（对位 Java
// identityLinks[0]），任务未解析出候选组时按序消费第一条带组记录。
// @param ctx 操作上下文
// @param task 任务快照
// @param ctl 钩子控制
// @param meta 节点模型元数据
// @param records 未消耗预生成记录
// @return 错误
func (h *ExecutorRewriteHook) rewriteCountersign(ctx context.Context, task *api.Task, ctl api.HookControl, meta *UserTaskMeta, records []*NextActivity) error {
	// 兜底：无未消耗记录时保留模型默认执行人（集合变量已驱动实例创建）
	if len(records) == 0 {
		return nil
	}
	// 三级排序保证"按序消费"确定性（同版本同创建时间按 Id，即插入顺序）
	ordered := make([]*NextActivity, len(records))
	copy(ordered, records)
	sort.SliceStable(ordered, func(i, j int) bool {
		// 版本号优先：跨批次预生成按版本轮转
		if ordered[i].Version != ordered[j].Version {
			return ordered[i].Version < ordered[j].Version
		}
		// 创建时间次之：同版本按落库时间
		if !ordered[i].CreateTime.Equal(ordered[j].CreateTime) {
			return ordered[i].CreateTime.Before(ordered[j].CreateTime)
		}
		// Id 终局排序：同批次内按插入顺序（UUIDv7 时间有序）
		return ordered[i].ID < ordered[j].ID
	})
	// 执行者类型：模型 executorType 扩展属性优先，回退 executorList 首项，缺省 person
	executorType := firstNonEmpty(meta.ExecutorType, firstExecutorConfigType(meta.ExecutorListRaw), "person")
	// 组类型走候选组匹配分支，其余（person/org/role）按人消费
	if executorType == "group" {
		return h.consumeCountersignGroup(ctx, task, ctl, ordered)
	}
	return h.consumeCountersignPerson(ctx, task, ctl, ordered)
}

// consumeCountersignPerson 会签按人消费：消耗恰好一条记录并将记录执行人回填为
// 任务候选用户（单人集合，整体重写幂等）。
// @param ctx 操作上下文
// @param task 任务快照
// @param ctl 钩子控制
// @param records 未消耗预生成记录
// @return 错误
func (h *ExecutorRewriteHook) consumeCountersignPerson(ctx context.Context, task *api.Task, ctl api.HookControl, records []*NextActivity) error {
	// 优先匹配与当前实例办理人同身份的记录（Java 按 assignee 匹配语义）
	target := firstRecordWhere(records, func(r *NextActivity) bool {
		return task.Assignee != "" && r.IdentityID == task.Assignee
	})
	// 办理人未命中且实例本无办理人（模型未设办理人表达式）时按序消费第一条；
	// 实例已有办理人但记录未覆盖时不误消费他人记录，保留现状（兜底）
	if target == nil && task.Assignee == "" {
		target = firstRecordWhere(records, func(r *NextActivity) bool { return r.IdentityID != "" })
	}
	// 脏记录（执行人全空）兜底：不消费，保留模型默认
	if target == nil {
		return nil
	}
	// 逐人回填候选人（规格语义：匹配 identity_id 行 SetCandidateUsers + MarkUsed）
	if err := ctl.SetCandidateUsers(task.ID, []string{target.IdentityID}); err != nil {
		return err
	}
	return h.next.MarkUsedAndWriteBack(ctx, []string{target.ID}, task.ID)
}

// consumeCountersignGroup 会签按组消费：消耗恰好一条记录并将记录候选组回填为任务候选组。
// @param ctx 操作上下文
// @param task 任务快照
// @param ctl 钩子控制
// @param records 未消耗预生成记录
// @return 错误
func (h *ExecutorRewriteHook) consumeCountersignGroup(ctx context.Context, task *api.Task, ctl api.HookControl, records []*NextActivity) error {
	// Java 取任务第一个身份链接的候选组定位匹配行，这里从快照候选组取第一个
	firstGroup := ""
	if len(task.CandidateGroups) > 0 {
		firstGroup = task.CandidateGroups[0]
	}
	target := firstRecordWhere(records, func(r *NextActivity) bool {
		return firstGroup != "" && r.GroupID == firstGroup
	})
	// 任务未解析出候选组时按序消费第一条带组记录；已有候选组但记录未覆盖时
	// 不误消费他组记录，保留现状（兜底）
	if target == nil && firstGroup == "" {
		target = firstRecordWhere(records, func(r *NextActivity) bool { return r.GroupID != "" })
	}
	// 脏记录（候选组全空）兜底：不消费，保留模型默认
	if target == nil {
		return nil
	}
	// 回填候选组（整体重写，幂等）
	if err := ctl.SetCandidateGroups(task.ID, []string{target.GroupID}); err != nil {
		return err
	}
	return h.next.MarkUsedAndWriteBack(ctx, []string{target.ID}, task.ID)
}

// identityIDsOf 提取记录的执行人 Id 列表。
// @param records 预生成记录
// @return 执行人 Id 列表
func identityIDsOf(records []*NextActivity) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.IdentityID)
	}
	return ids
}

// groupIDsOf 提取记录的候选组 Id 列表。
// @param records 预生成记录
// @return 候选组 Id 列表
func groupIDsOf(records []*NextActivity) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.GroupID)
	}
	return ids
}

// recordIDsOf 提取记录主键 Id 列表（消耗回写定位用）。
// @param records 预生成记录
// @return 记录 Id 列表
func recordIDsOf(records []*NextActivity) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.ID)
	}
	return ids
}

// distinctNonEmpty 去重并剔除空串（保持首次出现顺序）。
// @param values 原始列表
// @return 去重后的非空列表
func distinctNonEmpty(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		// 空串不参与回填，避免清空语义被脏数据触发
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// firstRecordWhere 返回第一条满足谓词的记录。
// @param records 预生成记录
// @param match 谓词
// @return 命中记录，未命中为 nil
func firstRecordWhere(records []*NextActivity, match func(*NextActivity) bool) *NextActivity {
	for _, record := range records {
		if match(record) {
			return record
		}
	}
	return nil
}

// firstExecutorConfigType 返回 executorList 首项的执行者类型（无配置返回空串）。
// @param raw executorList 原始串
// @return 执行者类型
func firstExecutorConfigType(raw string) string {
	configs := ParseExecutorConfigs(raw)
	// 无配置或首项未标类型时返回空串，由调用方继续回退
	if len(configs) == 0 {
		return ""
	}
	return configs[0].ExecutorType
}
