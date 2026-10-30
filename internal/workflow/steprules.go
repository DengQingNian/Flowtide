package workflow

import (
	"context"
	"strconv"
	"strings"
	"sync"

	"github.com/DengQingNian/Flowtide/api"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
)

// 步骤规则域错误码（退回/步骤规则域使用 24xx 段，26xx 保留给直送域之外的本文件新增）。
const (
	errCodeStepTaskMissing  = "WORKFLOW-2401" // 规则按任务定位失败：任务不存在
	errCodeStepDefMissing   = "WORKFLOW-2402" // 规则按任务定位失败：定义不存在
	errCodeStepStoreMissing = "WORKFLOW-2403" // 步骤规则存储未配置
)

// StepRule 是 workflow_his_step 步骤快照中与退回/会签/展示相关的规则字段子集
// （字段语义对位 Java WorkflowHisStepEntity；step_code 即 BPMN 节点 ID）。
// allow_return/back_step_codes/countersign_percent_return/return_percent 原样保留
// 数据库形态（"0"/"1"/数值串），由服务层按 Java 语义解释。
type StepRule struct {
	DeploymentID             string // 部署 ID（快照归属）
	StepCode                 string // 步骤代号 = BPMN 节点 ID（taskDefinitionKey）
	StepName                 string // 步骤名称
	AllowReturn              string // 是否允许退回原始值（"0" 禁止，其余/缺失允许）
	BackStepCodes            string // 允许退回到的步骤代号白名单（逗号分隔）
	ReturnApprovalStep       string // 退回审批步骤原始值（P2 仅透传，暂无消费方）
	CountersignPercentReturn string // 是否启用会签比例退回原始值（"1" 启用）
	ReturnPercent            string // 退回比例原始值（百分数，如 "50"）
	Sort                     int    // 步骤排序号（可退节点视图排序用）
	URL                      string // 步骤处理地址
}

// StepStore 是步骤规则（workflow_his_step 快照）的存储端口。
// 部署后规则以 deployment_id + step_code 定位，运行期只读。
type StepStore interface {
	// GetByDeploymentAndActivity 按部署与节点定位单条步骤规则。
	// @param ctx 操作上下文
	// @param deploymentID 部署 ID（=流程定义的 deploymentId）
	// @param activityID 节点 ID（=步骤代号 step_code）
	// @return 步骤规则
	// @return 是否存在
	// @return 查询错误
	GetByDeploymentAndActivity(ctx context.Context, deploymentID, activityID string) (*StepRule, bool, error)
	// ListByDeployment 查询部署下全部步骤规则（按 sort 升序）。
	// @param ctx 操作上下文
	// @param deploymentID 部署 ID
	// @return 步骤规则列表（无记录返回空列表）
	// @return 查询错误
	ListByDeployment(ctx context.Context, deploymentID string) ([]*StepRule, error)
}

// ─── 内存实现 ─────────────────────────────────────────────

// MemStepStore 是 StepStore 的内存实现（单测与嵌入式无 DB 模式）。
type MemStepStore struct {
	mu   sync.RWMutex         // 读写锁
	rows map[string]*StepRule // deploymentID|activityID → 规则
}

// NewMemStepStore 创建内存步骤规则存储。
// @return 内存存储
func NewMemStepStore() *MemStepStore {
	return &MemStepStore{rows: make(map[string]*StepRule)}
}

// stepStoreKey 组装内存存储的复合键。
// @param deploymentID 部署 ID
// @param activityID 节点 ID
// @return 复合键
func stepStoreKey(deploymentID, activityID string) string {
	return deploymentID + "|" + activityID
}

// Put 写入/覆盖一条步骤规则（测试播种用）。
// @param rule 步骤规则（DeploymentID + StepCode 定位）
func (m *MemStepStore) Put(rule *StepRule) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[stepStoreKey(rule.DeploymentID, rule.StepCode)] = rule
}

// GetByDeploymentAndActivity 按部署与节点定位单条步骤规则。
// @param ctx 操作上下文
// @param deploymentID 部署 ID
// @param activityID 节点 ID
// @return 步骤规则副本
// @return 是否存在
// @return 查询错误
func (m *MemStepStore) GetByDeploymentAndActivity(_ context.Context, deploymentID, activityID string) (*StepRule, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rule, ok := m.rows[stepStoreKey(deploymentID, activityID)]
	if !ok {
		return nil, false, nil
	}
	copied := *rule
	return &copied, true, nil
}

// ListByDeployment 查询部署下全部步骤规则（按 sort 升序，同 sort 按步骤代号）。
// @param ctx 操作上下文
// @param deploymentID 部署 ID
// @return 步骤规则列表
// @return 查询错误
func (m *MemStepStore) ListByDeployment(_ context.Context, deploymentID string) ([]*StepRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*StepRule, 0, 4)
	for key, rule := range m.rows {
		// 复合键前缀匹配部署维度
		if strings.HasPrefix(key, deploymentID+"|") {
			copied := *rule
			out = append(out, &copied)
		}
	}
	// 排序号升序、同号按代号，保证返回顺序确定
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && (out[j].Sort < out[j-1].Sort || (out[j].Sort == out[j-1].Sort && out[j].StepCode < out[j-1].StepCode)); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

// 编译期断言：内存实现满足端口。
var _ StepStore = (*MemStepStore)(nil)

// ─── SQL 适配 ─────────────────────────────────────────────

// SQLStepStore 是 StepStore 的 SQL 适配器，组合 pwf HisStep 仓储
// （workflow_his_step 行仓储，行结构通用于列级映射）。
type SQLStepStore struct {
	repos *pwf.Repositories // workflow 行仓储聚合
}

// NewSQLStepStore 创建步骤规则 SQL 适配器。
// @param store workflow 仓储入口（internal/persistence/workflow.New）
// @return SQL 适配器
func NewSQLStepStore(store *pwf.Store) *SQLStepStore {
	return &SQLStepStore{repos: pwf.NewRepositories(store)}
}

// GetByDeploymentAndActivity 按部署与节点定位单条步骤规则（复用仓储的部署+代号查询）。
// @param ctx 操作上下文
// @param deploymentID 部署 ID
// @param activityID 节点 ID（=step_code）
// @return 步骤规则
// @return 是否存在
// @return 查询错误
func (s *SQLStepStore) GetByDeploymentAndActivity(ctx context.Context, deploymentID, activityID string) (*StepRule, bool, error) {
	row, found, err := s.repos.HisSteps.GetByDeploymentAndStepCode(ctx, deploymentID, activityID)
	if err != nil || !found {
		return nil, found, err
	}
	return stepRuleFromRow(row), true, nil
}

// ListByDeployment 查询部署下全部步骤规则（仓储按 sort 升序返回）。
// @param ctx 操作上下文
// @param deploymentID 部署 ID
// @return 步骤规则列表
// @return 查询错误
func (s *SQLStepStore) ListByDeployment(ctx context.Context, deploymentID string) ([]*StepRule, error) {
	rows, err := s.repos.HisSteps.ListByDeployment(ctx, deploymentID)
	if err != nil {
		return nil, err
	}
	out := make([]*StepRule, 0, len(rows))
	for _, row := range rows {
		out = append(out, stepRuleFromRow(row))
	}
	return out, nil
}

// stepRuleFromRow 通用行转步骤规则（缺失列给零值）。
// @param row 通用行
// @return 步骤规则
func stepRuleFromRow(row pwf.Row) *StepRule {
	return &StepRule{
		DeploymentID:             rowString(row, "deployment_id"),
		StepCode:                 rowString(row, "step_code"),
		StepName:                 rowString(row, "step_name"),
		AllowReturn:              rowString(row, "allow_return"),
		BackStepCodes:            rowString(row, "back_step_codes"),
		ReturnApprovalStep:       rowString(row, "return_approval_step"),
		CountersignPercentReturn: rowString(row, "countersign_percent_return"),
		ReturnPercent:            rowString(row, "return_percent"),
		Sort:                     rowIntOf(row["sort"]),
		URL:                      rowString(row, "url"),
	}
}

// rowIntOf 通用行值转整型（数据库数值可能以多类型返回）。
// @param value 行值
// @return 整数值，无法解析返回 0
func rowIntOf(value any) int {
	switch typed := value.(type) {
	case nil:
		return 0
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case []byte:
		parsed, err := strconv.Atoi(strings.TrimSpace(string(typed)))
		if err != nil {
			return 0
		}
		return parsed
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

// 编译期断言：SQL 适配满足端口。
var _ StepStore = (*SQLStepStore)(nil)

// ─── 步骤规则服务 ─────────────────────────────────────────────

// StepRuleService 是步骤规则域服务（his_step 端口的消费门面）：
// 提供 allowReturn 开关、backStepCodes 白名单、排序号、步骤名、退回比例等规则读取；
// 并实现 P3-A 定义的 StepRuleProvider（按任务 ID 定位会签比例退回规则）。
type StepRuleService struct {
	store  StepStore  // his_step 存储端口
	engine api.Engine // 引擎门面（PercentReturnRule 按任务定位用，可 nil）
}

// NewStepRuleService 构造步骤规则域服务。
// @param store 步骤规则存储端口
// @param engine 引擎门面（仅按任务 ID 的规则读取需要，可 nil）
// @return 步骤规则服务
func NewStepRuleService(store StepStore, engine api.Engine) *StepRuleService {
	return &StepRuleService{store: store, engine: engine}
}

// rawEnabled 解释 "0"/"1" 布尔原始值（Java "1".equals(String.valueOf(x)) 语义）。
// @param raw 原始值
// @return 是否启用（"1"/"true" 为启用，其余含缺失为否）
func rawEnabled(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	return trimmed == "1" || strings.EqualFold(trimmed, "true")
}

// parsePercent 解析退回比例原始值（百分数）。
// @param raw 原始值
// @return 比例指针，缺失或非法返回 nil（与 Java Double.valueOf 失败即空语义一致）
func parsePercent(raw string) *float64 {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	value, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return nil
	}
	return &value
}

// AllowReturn 判断部署上指定节点是否允许退回（Java："0" 显式禁止，其余/缺失允许）。
// @param ctx 操作上下文
// @param deploymentID 部署 ID
// @param activityID 节点 ID
// @return 是否允许退回
// @return 查询错误
func (s *StepRuleService) AllowReturn(ctx context.Context, deploymentID, activityID string) (bool, error) {
	if s.store == nil {
		// 存储未配置时按允许处理（与 Java hisStep 为 null 的分支一致）
		return true, nil
	}
	step, found, err := s.store.GetByDeploymentAndActivity(ctx, deploymentID, activityID)
	if err != nil || !found {
		return true, err
	}
	// 仅 "0" 显式禁止退回
	return strings.TrimSpace(step.AllowReturn) != "0", nil
}

// BackStepCodes 读取节点的可退回步骤白名单（逗号拆分）。
// @param ctx 操作上下文
// @param deploymentID 部署 ID
// @param activityID 节点 ID
// @return 步骤代号列表（未配置返回空列表）
// @return 查询错误
func (s *StepRuleService) BackStepCodes(ctx context.Context, deploymentID, activityID string) ([]string, error) {
	if s.store == nil {
		return nil, nil
	}
	step, found, err := s.store.GetByDeploymentAndActivity(ctx, deploymentID, activityID)
	if err != nil || !found {
		return nil, err
	}
	return splitStepCodes(step.BackStepCodes), nil
}

// splitStepCodes 拆分逗号分隔的步骤代号并去空白（对位 Java split(",") 的空段剔除）。
// @param raw 原始串
// @return 代号列表
func splitStepCodes(raw string) []string {
	out := make([]string, 0, 4)
	for _, part := range strings.Split(raw, ",") {
		// 空白段剔除（Java Arrays.asList 保留空段，但空代号不参与匹配，等价剔除）
		if code := strings.TrimSpace(part); code != "" {
			out = append(out, code)
		}
	}
	return out
}

// SortOf 读取节点的步骤排序号（未配置返回 0）。
// @param ctx 操作上下文
// @param deploymentID 部署 ID
// @param activityID 节点 ID
// @return 排序号
// @return 查询错误
func (s *StepRuleService) SortOf(ctx context.Context, deploymentID, activityID string) (int, error) {
	if s.store == nil {
		return 0, nil
	}
	step, found, err := s.store.GetByDeploymentAndActivity(ctx, deploymentID, activityID)
	if err != nil || !found {
		return 0, err
	}
	return step.Sort, nil
}

// StepNameOf 读取节点的步骤名称（未配置返回空串）。
// @param ctx 操作上下文
// @param deploymentID 部署 ID
// @param activityID 节点 ID
// @return 步骤名称
// @return 查询错误
func (s *StepRuleService) StepNameOf(ctx context.Context, deploymentID, activityID string) (string, error) {
	if s.store == nil {
		return "", nil
	}
	step, found, err := s.store.GetByDeploymentAndActivity(ctx, deploymentID, activityID)
	if err != nil || !found {
		return "", err
	}
	return step.StepName, nil
}

// ReturnPercentOf 读取节点的会签比例退回配置。
// @param ctx 操作上下文
// @param deploymentID 部署 ID
// @param activityID 节点 ID
// @return 退回比例（百分数），未配置为 nil
// @return 是否启用比例退回（countersign_percent_return=="1"）
// @return 查询错误
func (s *StepRuleService) ReturnPercentOf(ctx context.Context, deploymentID, activityID string) (*float64, bool, error) {
	if s.store == nil {
		return nil, false, nil
	}
	step, found, err := s.store.GetByDeploymentAndActivity(ctx, deploymentID, activityID)
	if err != nil || !found {
		return nil, false, err
	}
	// 未启用比例退回时比例值不参与解释
	if !rawEnabled(step.CountersignPercentReturn) {
		return nil, false, nil
	}
	return parsePercent(step.ReturnPercent), true, nil
}

// PercentReturnRule 按任务定位其节点的会签比例退回规则（实现 P3-A StepRuleProvider）。
// 任务 → 实例定义 → 部署 → his_step 规则链；节点未启用比例退回时 (nil,false,nil)。
// @param ctx 操作上下文
// @param taskID 任务 ID
// @return 规则
// @return 是否启用比例退回
// @return 错误
func (s *StepRuleService) PercentReturnRule(ctx context.Context, taskID string) (*PercentReturnRule, bool, error) {
	// 存储或引擎未注入时无法定位，按未启用处理（P3-A 接口位约定）
	if s.store == nil || s.engine == nil {
		return nil, false, nil
	}
	task, err := s.engine.TaskService().GetTask(ctx, taskID)
	if err != nil {
		return nil, false, api.NewError(errCodeStepTaskMissing, "任务不存在", err)
	}
	definition, err := s.engine.RepositoryService().GetProcessDefinition(ctx, task.ProcessDefinitionID)
	if err != nil {
		return nil, false, api.NewError(errCodeStepDefMissing, "流程定义不存在", err)
	}
	percent, enabled, err := s.ReturnPercentOf(ctx, definition.DeploymentID, task.TaskDefinitionKey)
	if err != nil || !enabled {
		return nil, false, err
	}
	return &PercentReturnRule{CountersignPercentReturn: true, ReturnPercent: percent}, true, nil
}

// 编译期断言：满足 P3-A 比例退回规则端口。
var _ StepRuleProvider = (*StepRuleService)(nil)
