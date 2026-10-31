package workflow

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
	"github.com/shopspring/decimal"
)

// 会签执行监听器注册名（BPMN flowable:executionListener class 值，
// 对位 Java CountersignCollectionConfigExecutionListener/CountersignByWeightExecutionListener 的注册约定）。
const (
	// ListenerCountersignCollectionConfig 普通会签集合配置监听器（weightType=average 节点）。
	ListenerCountersignCollectionConfig = "countersignCollectionConfig"
	// ListenerCountersignByWeight 权重会签配置监听器（weightType=custom 节点）。
	ListenerCountersignByWeight = "countersignByWeight"
)

// GroupMemberQuery 是组成员查询能力（api.IdentityService 子集，org/role 执行者展开用）。
type GroupMemberQuery interface {
	// ListMembers 返回组成员。
	ListMembers(ctx context.Context, groupID string) ([]api.User, error)
}

// CountersignService 是会签域服务（P3-A）。
// 对位 Java ProcessesUtil#countersignCollectionConfig:4829 / countersignByWeightParameterSet:4247 /
// getCountersignWeight:4291 / complete:3897 比例退回段：
// 集合配置监听器 + 权重监听器 + 比例退回累计判定。权重累计（totalWeight）的办结侧
// 实现在 complete.go accumulateCountersignWeight，本服务不重复。
type CountersignService struct {
	engine    api.Engine                          // 引擎门面（七服务）
	meta      *ModelMetaService                   // 模型元数据（节点执行者/权重配置读取）
	next      *NextActivityService                // 下一步预生成（next_activity 执行者来源），nil 跳过该来源
	weights   CountersignWeightStore              // 权重存储（/weight 接口数据），nil 时仅模型 weightList
	members   GroupMemberQuery                    // 组成员查询（org/role 展开），nil 时保留组 ID 原值
	rules     StepRuleProvider                    // 比例退回规则端口（his_step），nil 时不判定超线
	clock     clock.Clock                         // 时间源
	shuffleFn func([]string)                      // 随机排序函数（测试注入用）
	mu        sync.RWMutex                        // 保护元数据缓存
	cache     map[string]map[string]*UserTaskMeta // 定义 ID → 节点 ID → 元数据（命令内只读）
	newID     func() string                       // ID 生成（权重存储审计列）
}

// CountersignOption 是会签服务选项函数。
type CountersignOption func(*CountersignService)

// WithCountersignNext 注入下一步预生成域服务（next_activity 执行者来源）。
// @param next 下一步预生成域服务
// @return 选项
func WithCountersignNext(next *NextActivityService) CountersignOption {
	return func(s *CountersignService) { s.next = next }
}

// WithCountersignWeights 注入权重存储（workflow_countersign_weight 表端口）。
// @param store 权重存储
// @return 选项
func WithCountersignWeights(store CountersignWeightStore) CountersignOption {
	return func(s *CountersignService) { s.weights = store }
}

// WithCountersignMembers 注入组成员查询（org/role 执行者经身份服务展开）。
// @param members 组成员查询
// @return 选项
func WithCountersignMembers(members GroupMemberQuery) CountersignOption {
	return func(s *CountersignService) { s.members = members }
}

// WithCountersignRules 注入比例退回规则端口（his_step 步骤规则）。
// @param rules 规则端口
// @return 选项
func WithCountersignRules(rules StepRuleProvider) CountersignOption {
	return func(s *CountersignService) { s.rules = rules }
}

// NewCountersignService 构造会签域服务。
// @param engine 引擎门面（七服务）
// @param meta 模型元数据服务
// @param options 可选配置（next_activity 来源、权重存储、组成员展开、时钟）
// @return 会签域服务
func NewCountersignService(engine api.Engine, meta *ModelMetaService, options ...CountersignOption) *CountersignService {
	s := &CountersignService{
		engine: engine,
		meta:   meta,
		clock:  clock.Real{},
		cache:  make(map[string]map[string]*UserTaskMeta),
		newID:  DefaultIDGen(),
	}
	for _, option := range options {
		option(s)
	}
	// 缺省随机排序：本地打乱（Java Collections.shuffle 等价）
	s.shuffleFn = func(values []string) {
		rand.Shuffle(len(values), func(i, j int) { values[i], values[j] = values[j], values[i] })
	}
	return s
}

// RegisterCountersignListeners 注册会签执行监听器（对位 Java listener 包的两个 @Component）。
// 按 BPMN flowable:executionListener 的 class 名分发：countersignCollectionConfig（集合配置）与
// countersignByWeight（权重配置）。引擎不支持扩展监听器注册时返回错误。
// 接线约定：部署新定义后必须调用 Warmup（或 WarmupAll）预热节点元数据缓存——
// 监听器在引擎命令内同步执行，命令持有引擎互斥写锁，监听器内调用 GetBpmnModel 等引擎
// 公有 API 会重入死锁（与 executor_rewrite.go 同一约束），因此监听器只读预热缓存。
// @param eng 引擎门面
// @return 错误
func (s *CountersignService) RegisterCountersignListeners(eng api.Engine) error {
	registrar, ok := eng.(api.ExecutionListenerRegistrar)
	if !ok {
		return api.NewError("WORKFLOW-2100", "引擎不支持扩展执行监听器注册", api.ErrBadRequest)
	}
	registrar.RegisterExecutionListenerHook(ListenerCountersignCollectionConfig, s.collectionConfigListener)
	registrar.RegisterExecutionListenerHook(ListenerCountersignByWeight, s.weightConfigListener)
	return nil
}

// Warmup 预热指定流程定义的全部用户任务元数据缓存（锁外调用）。
// 必须在引擎命令外调用（部署后/发起后/启动时），命令内调用会与引擎写锁重入死锁。
// @param ctx 操作上下文
// @param definitionID 流程定义 ID
// @return 预热错误
func (s *CountersignService) Warmup(ctx context.Context, definitionID string) error {
	metas, err := s.meta.ListUserTaskMetas(ctx, definitionID)
	if err != nil {
		return err
	}
	index := make(map[string]*UserTaskMeta, len(metas))
	for _, meta := range metas {
		index[meta.ActivityID] = meta
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[definitionID] = index
	return nil
}

// WarmupAll 预热全部已部署定义的节点元数据缓存（锁外调用，启动期用）。
// @param ctx 操作上下文
// @return 预热错误
func (s *CountersignService) WarmupAll(ctx context.Context) error {
	definitions, err := s.engine.RepositoryService().ListProcessDefinitions(ctx)
	if err != nil {
		return err
	}
	for _, definition := range definitions {
		// 单个定义预热失败不阻断其余定义（部署损坏等场景），与 rewrite 预热语义一致
		if err := s.Warmup(ctx, definition.ID); err != nil {
			continue
		}
	}
	return nil
}

// cachedUserTaskMeta 命令内只读查询预热的节点元数据。
// @param definitionID 流程定义 ID
// @param activityID 节点 ID
// @return 元数据，未预热返回 nil
func (s *CountersignService) cachedUserTaskMeta(definitionID, activityID string) *UserTaskMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if index, ok := s.cache[definitionID]; ok {
		return index[activityID]
	}
	return nil
}

// collectionConfigListener 集合配置监听器（对位 CountersignCollectionConfigExecutionListener）。
// 节点 start 时写变量 executorList（去重）/nrOfInstances/countersignPass=0/countersignReturn=0。
// @param ctx 命令上下文
// @param execution 执行上下文快照
// @param ctl 监听器控制
// @return 错误
func (s *CountersignService) collectionConfigListener(ctx context.Context, execution api.ListenerExecution, ctl api.ListenerControl) error {
	_, err := s.applyCollectionConfig(ctx, execution, ctl)
	return err
}

// weightConfigListener 权重会签监听器（对位 CountersignByWeightExecutionListener）。
// 先执行集合配置，再校验 weightThreshold 非空（缺省抛"未设置会签权重阈值"），
// 写 totalWeight=0/weightThreshold/countersignByWeight=true。
// @param ctx 命令上下文
// @param execution 执行上下文快照
// @param ctl 监听器控制
// @return 错误
func (s *CountersignService) weightConfigListener(ctx context.Context, execution api.ListenerExecution, ctl api.ListenerControl) error {
	meta, err := s.applyCollectionConfig(ctx, execution, ctl)
	if err != nil {
		return err
	}
	// Java：weightThreshold 缺失 → "未设置会签权重阈值"
	if strings.TrimSpace(meta.WeightThreshold) == "" {
		return apiError("WORKFLOW-2103", "未设置会签权重阈值")
	}
	// 累计权重清零（Java setVariables 全量覆盖语义的等价增量写）
	if err := ctl.SetVariable(VarTotalWeight, float64(0)); err != nil {
		return err
	}
	if err := ctl.SetVariable(VarWeightThreshold, meta.WeightThreshold); err != nil {
		return err
	}
	return ctl.SetVariable(VarCountersignByWeight, true)
}

// applyCollectionConfig 集合配置公共主体（对位 countersignCollectionConfig:4829）。
// 执行者来源优先级（按 Java 源码实现，注意与调研文档 §5 表述相反）：
//  1. 模型 executorType+executorList（person 直用，org/role 经身份服务展开）；
//  2. next_activity 已有记录覆盖（person→identityId，否则 groupId）；
//  3. 业务变量 customExecutorList 最终覆盖（randomSort=true 时先打乱）。
//
// 最终去重，空集合抛"会签集合不能为空"；写 executorList/nrOfInstances 并初始化计数。
// @param ctx 命令上下文
// @param execution 执行上下文快照
// @param ctl 监听器控制
// @return 节点元数据（权重监听器续用）
// @return 错误
func (s *CountersignService) applyCollectionConfig(ctx context.Context, execution api.ListenerExecution, ctl api.ListenerControl) (*UserTaskMeta, error) {
	// Java：节点 ID 缺失 → "会签集合自动配置异常：会签节点不存在"
	activityID := strings.TrimSpace(execution.ActivityID)
	if activityID == "" {
		return nil, apiError("WORKFLOW-2101", "会签集合自动配置异常：会签节点不存在")
	}
	meta := s.cachedUserTaskMeta(execution.ProcessDefinitionID, activityID)
	if meta == nil {
		return nil, apiError("WORKFLOW-2104", "会签节点元数据未预热: "+activityID+"（部署后须调用 Warmup）")
	}
	// 1. 模型执行者配置（person 直用，org/role 展开）
	finalList := s.modelExecutorIDs(ctx, meta)
	// 2. next_activity 已有记录覆盖（Java：以实际预存记录为准）
	if records := s.nextActivityExecutorIDs(ctx, execution, meta); len(records) > 0 {
		finalList = records
	}
	// 3. 业务变量 customExecutorList 最终覆盖（randomSort=true 时打乱）
	if custom := s.customExecutorIDs(execution); len(custom) > 0 {
		finalList = custom
	}
	// Java：最终集合去重
	finalList = distinctStrings(finalList)
	if len(finalList) == 0 {
		return nil, apiError("WORKFLOW-2102", "会签集合不能为空")
	}
	// 2、3 写集合变量与循环总次数（Java removeVariable+setVariable 的等价覆盖）
	if err := ctl.SetVariable(VarExecutorList, finalList); err != nil {
		return nil, err
	}
	if err := ctl.SetVariable(VarNrOfInstances, len(finalList)); err != nil {
		return nil, err
	}
	// 初始化会签比例退回操作结果（仅缺失时置 0：引擎对串行会签会逐实例重放 start 监听器，
	// 无条件覆盖会把已完成实例累计的计数清零）
	s.initCountIfAbsent(execution, ctl, VarCountersignPass)
	s.initCountIfAbsent(execution, ctl, VarCountersignReturn)
	return meta, nil
}

// modelExecutorIDs 读取模型执行者配置为身份列表。
// person 配置直用 identityId；org/role 配置经组成员查询展开为用户列表
// （身份查询未注入时保留组 ID 原值，与 Java 原样透传行为一致）。
// @param ctx 操作上下文
// @param meta 节点元数据
// @return 身份列表
func (s *CountersignService) modelExecutorIDs(ctx context.Context, meta *UserTaskMeta) []string {
	configs := ParseExecutorConfigs(meta.ExecutorListRaw)
	ids := make([]string, 0, len(configs))
	for _, config := range configs {
		// 条目类型缺失时回退节点级 executorType，再退 person
		executorType := config.ExecutorType
		if executorType == "" {
			executorType = meta.ExecutorType
		}
		switch executorType {
		case "org":
			ids = append(ids, s.expandGroupMembers(ctx, config.OrgID)...)
		case "role":
			ids = append(ids, s.expandGroupMembers(ctx, config.RoleID)...)
		default:
			// person 类型直用身份 Id（含逗号分隔退化形态）
			ids = append(ids, config.IdentityID)
		}
	}
	return ids
}

// expandGroupMembers 展开组（org/role）成员为用户列表。
// @param ctx 操作上下文
// @param groupID 组 ID
// @return 用户列表，查询失败或未注入成员查询时保留组 ID 原值
func (s *CountersignService) expandGroupMembers(ctx context.Context, groupID string) []string {
	if groupID == "" {
		return nil
	}
	// 成员查询未注入：保留组 ID 原值（Java 未展开的等价兜底）
	if s.members == nil {
		return []string{groupID}
	}
	users, err := s.members.ListMembers(ctx, groupID)
	// 查询失败按空组兜底，保留组 ID 保证会签集合非空
	if err != nil || len(users) == 0 {
		return []string{groupID}
	}
	ids := make([]string, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.ID)
	}
	return ids
}

// nextActivityExecutorIDs 读取 next_activity 已有预存记录的执行者（Java getListByProcInstAndTaskKey 段）。
// 节点 executorType=person 取 identityId，否则取 groupId；来源未注入或无记录时返回空。
// @param ctx 操作上下文
// @param execution 执行上下文快照
// @param meta 节点元数据
// @return 身份列表
func (s *CountersignService) nextActivityExecutorIDs(ctx context.Context, execution api.ListenerExecution, meta *UserTaskMeta) []string {
	// 来源未注入（嵌入式模式可省）时跳过该来源
	if s.next == nil {
		return nil
	}
	records, err := s.next.ListUnusedByInstanceAndActivity(ctx, execution.ProcessInstanceID, execution.ActivityID)
	if err != nil || len(records) == 0 {
		return nil
	}
	ids := make([]string, 0, len(records))
	for _, record := range records {
		// Java：person 类型取 identityId，其余类型取 groupId
		if meta.ExecutorType == "person" {
			ids = append(ids, record.IdentityID)
		} else {
			ids = append(ids, record.GroupID)
		}
	}
	return ids
}

// customExecutorIDs 读取业务变量 customExecutorList（Java variables.customExecutorList 段）。
// randomSort=true 时先打乱顺序（对位 Collections.shuffle）。
// @param execution 执行上下文快照
// @return 身份列表
func (s *CountersignService) customExecutorIDs(execution api.ListenerExecution) []string {
	raw, ok := execution.Variables[VarCustomExecutorList]
	if !ok || raw == nil {
		return nil
	}
	ids := toStringSlice(raw)
	if len(ids) == 0 {
		return nil
	}
	// randomSort=true 时随机排序（Java Collections.shuffle 语义）
	if randomSort, ok := execution.Variables[VarRandomSort].(bool); ok && randomSort {
		s.shuffleFn(ids)
	}
	return ids
}

// initCountIfAbsent 比例退回计数初始化（仅缺失时写 0）。
// @param execution 执行上下文快照（缺失判定依据）
// @param ctl 监听器控制
// @param name 计数变量名
func (s *CountersignService) initCountIfAbsent(execution api.ListenerExecution, ctl api.ListenerControl, name string) {
	if _, exists := execution.Variables[name]; exists {
		return
	}
	_ = ctl.SetVariable(name, 0)
}

// ─── 权重配置管理（/weight 接口数据用） ─────────────────────────────

// SaveNodeWeights 保存节点权重配置（先删后插，对位 Java workflow_countersign_weight 写入口）。
// @param ctx 操作上下文
// @param key 节点定位键（定义 Key + 节点 ID）
// @param configs 权重配置列表
// @return 错误
func (s *CountersignService) SaveNodeWeights(ctx context.Context, key CountersignWeightKey, configs []CountersignWeight) error {
	// 权重存储未注入时拒绝写入（内存/SQL 适配由构造方选择）
	if s.weights == nil {
		return apiError("WORKFLOW-2105", "会签权重存储未配置")
	}
	return s.weights.SaveByNode(ctx, key, configs)
}

// ListNodeWeights 读取节点权重配置：权重存储优先，其次模型 weightList（ParseWeightList）。
// @param ctx 操作上下文
// @param definitionID 流程定义 ID（模型来源解析用）
// @param key 节点定位键（定义 Key + 节点 ID）
// @return 权重配置列表
// @return 错误
func (s *CountersignService) ListNodeWeights(ctx context.Context, definitionID string, key CountersignWeightKey) ([]CountersignWeight, error) {
	if s.weights != nil {
		configs, err := s.weights.ListByNode(ctx, key)
		// 存储读取失败回退模型 weightList，保证权重会签可降级运行
		if err == nil && len(configs) > 0 {
			return configs, nil
		}
	}
	meta, err := s.meta.UserTaskMeta(ctx, definitionID, key.ActivityID)
	if err != nil {
		return nil, err
	}
	return weightsFromConfigs(ParseWeightList(meta.WeightListRaw)), nil
}

// weightsFromConfigs 执行者配置转权重配置（weightList 与执行者共用 ExecutorConfig 载体）。
// @param configs 执行者配置列表
// @return 权重配置列表
func weightsFromConfigs(configs []ExecutorConfig) []CountersignWeight {
	out := make([]CountersignWeight, 0, len(configs))
	for _, config := range configs {
		if config.IdentityID == "" {
			continue
		}
		out = append(out, CountersignWeight{IdentityID: config.IdentityID, AgreeWeight: config.AgreeWeight, RejectWeight: config.RejectWeight})
	}
	return out
}

// ─── 比例退回基础（对位 complete:3897 会签段） ─────────────────────

// PercentReturnRule 是节点的会签比例退回规则（对位 his_step.countersignPercentReturn/returnPercent）。
type PercentReturnRule struct {
	CountersignPercentReturn bool     // 是否启用比例退回
	ReturnPercent            *float64 // 退回比例（百分数，0-100）
}

// StepRuleProvider 是比例退回规则端口（his_step 步骤规则，P3-A 仅接口位：
// his_step 仓储尚未抽象为引擎外端口，nil 时比例退回恒判定"不超线"）。
type StepRuleProvider interface {
	// PercentReturnRule 返回任务所属节点的会签比例退回规则。
	// @param ctx 操作上下文
	// @param taskID 任务 ID
	// @return 规则
	// @return 是否启用比例退回（false 时调用方跳过判定）
	// @return 错误
	PercentReturnRule(ctx context.Context, taskID string) (*PercentReturnRule, bool, error)
}

// CountersignPercentResult 会签比例退回累计与超线判定（供 complete 链调用）。
// 按会签结论累计 countersignPass/countersignReturn（0 退回 / 1 通过）；
// 全部实例完成后计算退回率 countersignReturn/(pass+return) 与 returnPercent/100 比较
// （shopspring/decimal 定点比较，修复 Java 两位小数格式化的四舍五入误判），超线返回 true。
// @param ctx 操作上下文
// @param taskID 任务 ID（当前会签实例任务）
// @param countersignResult 会签结论：0 否决退回 / 1 通过
// @return 是否超过退回比例（需要触发比例退回）
// @return 错误
func (s *CountersignService) CountersignPercentResult(ctx context.Context, taskID string, countersignResult int) (bool, error) {
	// 规则端口未注入或未启用：不判定超线（P3-A 接口位约定）。
	// 016 号审计：规则端口接入后，任务/定义已不存在的实例原本会向上抛"任务不存在"，
	// 使 /process/complete/sign 在普通实例上由"data=false"退化为失败。按文档 015 §3.2
	// 约定（未启用会签退回规则的实例返回成功且 data=false），规则定位失败按未启用处理；
	// 规则存储故障（WORKFLOW-24xx 之外的存储错误）仍向上抛出，避免掩盖真实故障。
	rule, enabled, err := s.percentReturnRule(ctx, taskID)
	if err != nil {
		if isStepRuleLookupError(err) {
			return false, nil
		}
		return false, err
	}
	if !enabled {
		return false, nil
	}
	// 会签结论合法性：0 否决 / 1 通过（对位 Java complete 的 countersignResult 校验）
	if countersignResult != 0 && countersignResult != 1 {
		return false, apiError("WORKFLOW-2010", "未知会签任务执行结果")
	}
	task, err := s.engine.TaskService().GetTask(ctx, taskID)
	if err != nil {
		return false, api.NewError("WORKFLOW-2004", "任务不存在", err)
	}
	vars, err := s.engine.RuntimeService().GetVariables(ctx, task.ProcessInstanceID)
	if err != nil {
		return false, err
	}
	pass := intOf(vars[VarCountersignPass])
	returned := intOf(vars[VarCountersignReturn])
	// 更新会签操作结果（Java runtimeService.setVariable ±1 语义）
	if countersignResult == 0 {
		returned++
		if err := s.engine.RuntimeService().SetVariable(ctx, task.ProcessInstanceID, VarCountersignReturn, returned); err != nil {
			return false, err
		}
	} else {
		pass++
		if err := s.engine.RuntimeService().SetVariable(ctx, task.ProcessInstanceID, VarCountersignPass, pass); err != nil {
			return false, err
		}
	}
	allInstance := intOf(vars[VarNrOfInstances])
	// Java：未完成全部实例不判定比例
	if pass+returned != allInstance {
		return false, nil
	}
	// Java：returnPercent 缺失 → "未设置会签退回比例"
	if rule.ReturnPercent == nil {
		return false, apiError("WORKFLOW-2106", "未设置会签退回比例")
	}
	// 有退回操作才计算退回比例（Java countersignReturn>0 门控）
	if returned == 0 {
		return false, nil
	}
	return returnRatioOverLine(returned, pass+returned, *rule.ReturnPercent), nil
}

// isStepRuleLookupError 判定是否为"规则定位失败"（任务/定义已不存在）。
// 定位失败按未启用会签比例退回处理，其余错误（存储故障等）继续向上抛出。
// @param err 错误
// @return 是否属于定位失败
func isStepRuleLookupError(err error) bool {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.Code == errCodeStepTaskMissing || apiErr.Code == errCodeStepDefMissing
}

// percentReturnRule 读取节点的比例退回规则（规则端口未注入时恒不启用）。
// @param ctx 操作上下文
// @param taskID 任务 ID
// @return 规则
// @return 是否启用比例退回
// @return 错误
func (s *CountersignService) percentReturnRule(ctx context.Context, taskID string) (*PercentReturnRule, bool, error) {
	if s.rules == nil {
		return nil, false, nil
	}
	return s.rules.PercentReturnRule(ctx, taskID)
}

// returnRatioOverLine 退回率是否达到退回比例阈值（decimal 精确比较，≥ 即超线）。
// @param returned 退回票数
// @param total 有效票数（通过+退回）
// @param returnPercent 退回比例（百分数）
// @return 是否超线
func returnRatioOverLine(returned, total int, returnPercent float64) bool {
	// 退回率 = returned/total（精确有理数）
	ratio := decimal.NewFromInt(int64(returned)).Div(decimal.NewFromInt(int64(total)))
	// 阈值 = returnPercent/100（百分数转小数）
	threshold := decimal.NewFromFloat(returnPercent).Div(decimal.NewFromInt(100))
	// Java：compare >= 0 即超线（返回比例与计算结果相等也触发）
	return ratio.GreaterThanOrEqual(threshold)
}

// intOf 任意数值表示转 int（引擎变量可能是多种数值类型）。
// @param value 变量值
// @return 整数值，无法解析返回 0
func intOf(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

// distinctStrings 字符串去重（保持首次出现顺序，对位 Java stream().distinct()）。
// @param values 原列表
// @return 去重后的列表
func distinctStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		// 空串直接丢弃（Java 侧身份 Id 不可能为空串）
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// toStringSlice 任意集合变量转字符串切片（[]string/[]any 兼容）。
// @param value 变量值
// @return 字符串列表
func toStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			// 元素经字符串化转身份 Id，兼容数字身份列表
			out = append(out, strings.TrimSpace(strings.Trim(fmt.Sprintf("%v", item), `"`)))
		}
		return out
	default:
		return nil
	}
}

// ─── 权重存储端口与内存实现 ─────────────────────────────────────────

// CountersignWeightKey 是权重配置定位键（对位 workflow_countersign_weight 唯一维度：
// 流程定义 Key + 节点 ID，租户可选隔离）。
type CountersignWeightKey struct {
	ProcessDefinitionKey string // 流程定义 Key
	ActivityID           string // 节点 ID
	TenantID             string // 租户，空表示不隔离
}

// CountersignWeight 是单身份的会签权重配置。
type CountersignWeight struct {
	IdentityID   string  // 身份 Id
	AgreeWeight  float64 // 通过权重
	RejectWeight float64 // 拒绝权重
}

// CountersignWeightStore 是会签权重存储端口（workflow_countersign_weight 表，/weight 接口数据用）。
type CountersignWeightStore interface {
	// SaveByNode 先删后插保存节点权重配置（对位 Java 按 definitionKey+activityId 先删后插）。
	// @param ctx 操作上下文
	// @param key 节点定位键
	// @param configs 权重配置列表
	// @return 错误
	SaveByNode(ctx context.Context, key CountersignWeightKey, configs []CountersignWeight) error
	// ListByNode 查询节点权重配置。
	// @param ctx 操作上下文
	// @param key 节点定位键
	// @return 权重配置列表（无记录返回空列表）
	// @return 错误
	ListByNode(ctx context.Context, key CountersignWeightKey) ([]CountersignWeight, error)
}

// MemCountersignWeightStore 是权重存储端口的内存实现（单测与嵌入式无 DB 模式）。
type MemCountersignWeightStore struct {
	mu   sync.RWMutex                                 // 读写锁
	rows map[CountersignWeightKey][]CountersignWeight // 键 → 权重配置
}

// NewMemCountersignWeightStore 创建内存权重存储。
// @return 内存存储
func NewMemCountersignWeightStore() *MemCountersignWeightStore {
	return &MemCountersignWeightStore{rows: make(map[CountersignWeightKey][]CountersignWeight)}
}

// SaveByNode 先删后插保存节点权重配置。
// @param ctx 操作上下文
// @param key 节点定位键
// @param configs 权重配置列表
// @return 错误
// @param _ 接口占位参数（实现不使用）
func (m *MemCountersignWeightStore) SaveByNode(_ context.Context, key CountersignWeightKey, configs []CountersignWeight) error {
	// 定位键不完整时拒绝（对位 Java 唯一维度校验）
	if key.ProcessDefinitionKey == "" || key.ActivityID == "" {
		return apiError("WORKFLOW-2107", "权重配置缺少定义 Key 或节点 ID")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make([]CountersignWeight, 0, len(configs))
	for _, config := range configs {
		// 无身份的配置行无归属意义，跳过
		if config.IdentityID == "" {
			continue
		}
		copied = append(copied, config)
	}
	m.rows[key] = copied
	return nil
}

// ListByNode 查询节点权重配置。
// @param ctx 操作上下文
// @param key 节点定位键
// @return 权重配置列表（无记录返回空列表）
// @return 错误
// @param _ 接口占位参数（实现不使用）
func (m *MemCountersignWeightStore) ListByNode(_ context.Context, key CountersignWeightKey) ([]CountersignWeight, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rows := m.rows[key]
	out := make([]CountersignWeight, 0, len(rows))
	return append(out, rows...), nil
}

// SQLCountersignWeightStore 是权重存储端口的 SQL 适配器，组合 pwf 仓储
// （workflow_countersign_weight 行仓储，先删后插语义经逐主键删除实现）。
type SQLCountersignWeightStore struct {
	repos *pwf.Repositories // workflow 行仓储聚合
	newID func() string     // ID 生成（插入行主键）
}

// NewSQLCountersignWeightStore 创建权重存储 SQL 适配器。
// @param store workflow 仓储入口（internal/persistence/workflow.New）
// @return SQL 适配器
func NewSQLCountersignWeightStore(store *pwf.Store) *SQLCountersignWeightStore {
	return &SQLCountersignWeightStore{repos: pwf.NewRepositories(store), newID: DefaultIDGen()}
}

// SaveByNode 先删后插保存节点权重配置。
// @param ctx 操作上下文
// @param key 节点定位键
// @param configs 权重配置列表
// @return 错误
func (s *SQLCountersignWeightStore) SaveByNode(ctx context.Context, key CountersignWeightKey, configs []CountersignWeight) error {
	// 定位键不完整时拒绝（对位 Java 唯一维度校验）
	if key.ProcessDefinitionKey == "" || key.ActivityID == "" {
		return apiError("WORKFLOW-2107", "权重配置缺少定义 Key 或节点 ID")
	}
	// 1. 先删：按定义 Key + 节点查出旧行后逐主键删除（pwf 无条件删除导出，逐行删除等价）
	oldRows, err := s.repos.CountersignWeights.ListByDefinitionAndActivity(ctx, key.ProcessDefinitionKey, key.ActivityID)
	if err != nil {
		return err
	}
	for _, row := range oldRows {
		if _, err := s.repos.CountersignWeights.DeleteByPK(ctx, rowString(row, "id")); err != nil {
			return err
		}
	}
	// 2. 后插：逐条写入新配置（列集与 workflow_countersign_weight 表定义一致）
	for _, config := range configs {
		// 无身份的配置行无归属意义，跳过
		if config.IdentityID == "" {
			continue
		}
		row := pwf.Row{
			"id":                     s.newID(),
			"activity_id":            key.ActivityID,
			"process_definition_key": key.ProcessDefinitionKey,
			"agree_weight":           config.AgreeWeight,
			"reject_weight":          config.RejectWeight,
			"identity_id":            config.IdentityID,
			"tenant_id":              key.TenantID,
		}
		if err := s.repos.CountersignWeights.Insert(ctx, row); err != nil {
			return err
		}
	}
	return nil
}

// ListByNode 查询节点权重配置。
// @param ctx 操作上下文
// @param key 节点定位键
// @return 权重配置列表（无记录返回空列表）
// @return 错误
func (s *SQLCountersignWeightStore) ListByNode(ctx context.Context, key CountersignWeightKey) ([]CountersignWeight, error) {
	rows, err := s.repos.CountersignWeights.ListByDefinitionAndActivity(ctx, key.ProcessDefinitionKey, key.ActivityID)
	if err != nil {
		return nil, err
	}
	out := make([]CountersignWeight, 0, len(rows))
	for _, row := range rows {
		out = append(out, CountersignWeight{
			IdentityID:   rowString(row, "identity_id"),
			AgreeWeight:  rowFloatOf(row["agree_weight"]),
			RejectWeight: rowFloatOf(row["reject_weight"]),
		})
	}
	return out, nil
}

// rowFloatOf 通用行值转浮点（数据库数值可能以多类型返回）。
// @param value 行值
// @return 浮点值，无法解析返回 0
func rowFloatOf(value any) float64 {
	switch typed := value.(type) {
	case nil:
		return 0
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	case []byte:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(string(typed)), 64)
		if err != nil {
			return 0
		}
		return parsed
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}
