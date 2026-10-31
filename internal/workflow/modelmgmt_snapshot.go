package workflow

import (
	"context"
	"strings"
	"sync"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
)

// 部署快照域错误码（沿用模型管理 WORKFLOW-30xx 段）。
const (
	errCodeMMSnapBlank     = "WORKFLOW-3009" // 部署快照参数缺失（租户/模型代号/部署 ID）
	errCodeMMSnapStoreMiss = "WORKFLOW-3010" // 部署快照依赖存储未配置
)

// ─── 存储端口 ─────────────────────────────────────────────

// HisStepStore 是 workflow_his_step（步骤部署快照）的存储端口。
// 快照以部署维度隔离（deployment_id 引擎内唯一，运行期规则字典只读），
// 方法语义为既有 StepStore 端口（见 steprules.go）的超集：任何 HisStepStore
// 实现均可直接充任 StepStore（编译期断言见本文件末尾），主线程装配时把
// 同一实现同时传给 NewStepRuleService 与 NewDeploySnapshotService 即可。
type HisStepStore interface {
	// GetByDeploymentAndActivity 按部署与节点定位单条快照规则。
	// @param ctx 操作上下文
	// @param deploymentID 部署 ID（=流程定义的 deploymentId）
	// @param activityID 节点 ID（=步骤代号 step_code）
	// @return 快照规则
	// @return 是否存在
	// @return 查询错误
	GetByDeploymentAndActivity(ctx context.Context, deploymentID, activityID string) (*StepRule, bool, error)
	// ListByDeployment 查询部署下全部快照规则（按 sort 升序）。
	// @param ctx 操作上下文
	// @param deploymentID 部署 ID
	// @return 快照规则列表（无记录返回空列表）
	// @return 查询错误
	ListByDeployment(ctx context.Context, deploymentID string) ([]*StepRule, error)
	// Put 批量替换部署的全部快照规则（同 deploymentID 重复调用替换而非追加，幂等）。
	// @param ctx 操作上下文
	// @param deploymentID 部署 ID
	// @param rules 快照规则列表（空列表表示清空该部署快照）
	// @return 错误
	Put(ctx context.Context, deploymentID string, rules []*StepRule) error
	// DeleteByDeployment 删除部署的全部快照规则。
	// @param ctx 操作上下文
	// @param deploymentID 部署 ID
	// @return 错误
	DeleteByDeployment(ctx context.Context, deploymentID string) error
}

// ─── 内存实现 ─────────────────────────────────────────────

// MemHisStepStore 是 HisStepStore 的内存实现（部署维度 map + 存取双向克隆）。
type MemHisStepStore struct {
	mu    sync.RWMutex           // 读写锁
	rules map[string][]*StepRule // deploymentID → 规则列表（按 sort 升序）
}

// NewMemHisStepStore 创建内存快照存储。
// @return 内存存储
func NewMemHisStepStore() *MemHisStepStore {
	return &MemHisStepStore{rules: make(map[string][]*StepRule)}
}

// 编译期断言：内存实现满足快照端口。
var _ HisStepStore = (*MemHisStepStore)(nil)

// 编译期断言：快照端口为 StepStore 超集，可直接充任运行期规则存储。
var _ StepStore = (*MemHisStepStore)(nil)

// mmCloneRule 深拷贝快照规则（全值列，值拷贝即可）。
// @param r 原规则
// @return 副本
func mmCloneRule(r *StepRule) *StepRule {
	if r == nil {
		return nil
	}
	copied := *r
	return &copied
}

// mmCloneRules 深拷贝规则列表（保持顺序）。
// @param rules 原规则列表
// @return 副本列表
func mmCloneRules(rules []*StepRule) []*StepRule {
	out := make([]*StepRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, mmCloneRule(rule))
	}
	return out
}

// Put 批量替换部署的全部快照规则（替换语义保证同部署重复快照幂等；空列表清空）。
// @param ctx 操作上下文
// @param deploymentID 部署 ID
// @param rules 快照规则列表
// @return 错误
// @param _ 接口占位参数（实现不使用）
func (m *MemHisStepStore) Put(_ context.Context, deploymentID string, rules []*StepRule) error {
	// 部署 ID 是快照定位键，缺失即调用方违约
	if strings.TrimSpace(deploymentID) == "" {
		return apiError(errCodeMMSnapBlank, "快照写入缺少部署 ID")
	}
	copied := mmCloneRules(rules)
	// 统一归属部署，避免调用方漏填导致规则漂移
	for _, rule := range copied {
		rule.DeploymentID = deploymentID
	}
	// 插入排序：sort 升序，同号按步骤代号（与 StepStore 读取顺序约定一致）
	for i := 1; i < len(copied); i++ {
		for j := i; j > 0 && mmRuleLess(copied[j], copied[j-1]); j-- {
			copied[j], copied[j-1] = copied[j-1], copied[j]
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rules[deploymentID] = copied
	return nil
}

// GetByDeploymentAndActivity 按部署与节点定位单条快照规则（返回副本）。
// @param ctx 操作上下文
// @param deploymentID 部署 ID
// @param activityID 节点 ID
// @return 快照规则副本
// @return 是否存在
// @return 查询错误
// @param _ 接口占位参数（实现不使用）
func (m *MemHisStepStore) GetByDeploymentAndActivity(_ context.Context, deploymentID, activityID string) (*StepRule, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, rule := range m.rules[deploymentID] {
		if rule.StepCode == activityID {
			return mmCloneRule(rule), true, nil
		}
	}
	return nil, false, nil
}

// ListByDeployment 查询部署下全部快照规则（存储内已按 sort 升序）。
// @param ctx 操作上下文
// @param deploymentID 部署 ID
// @return 快照规则副本列表
// @return 查询错误
// @param _ 接口占位参数（实现不使用）
func (m *MemHisStepStore) ListByDeployment(_ context.Context, deploymentID string) ([]*StepRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return mmCloneRules(m.rules[deploymentID]), nil
}

// DeleteByDeployment 删除部署的全部快照规则。
// @param ctx 操作上下文
// @param deploymentID 部署 ID
// @return 错误
// @param _ 接口占位参数（实现不使用）
func (m *MemHisStepStore) DeleteByDeployment(_ context.Context, deploymentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rules, deploymentID)
	return nil
}

// mmRuleLess 比较两个快照规则的排序键（sort 升序，同号按步骤代号）。
// @param a 左操作数
// @param b 右操作数
// @return a 是否排在 b 前
func mmRuleLess(a, b *StepRule) bool {
	// 同排序号回退代号比较，保证输出顺序确定
	if a.Sort != b.Sort {
		return a.Sort < b.Sort
	}
	return a.StepCode < b.StepCode
}

// ─── 部署快照服务 ─────────────────────────────────────────────

// DeploySnapshotService 是部署快照域服务（P5）。
// 行为规格：部署流程定义时把模型的设计时步骤（workflow_step）转换为运行期
// 规则快照（workflow_his_step，字段映射复用 StepRule），并回写模型最近部署
// 时间（workflow_model.last_deploy_time）；同 deploymentID 重复快照替换而非
// 追加（幂等）。
type DeploySnapshotService struct {
	models   ModelMgmtStore  // 模型存储（定位模型 + 回写部署时间），可 nil（报错）
	designs  StepDesignStore // 设计时步骤存储，可 nil（报错）
	hisSteps HisStepStore    // 快照存储，可 nil（报错）
	clock    clock.Clock     // 时间源（部署时间回写）
	newID    func() string   // ID 生成函数（保留给后续行级快照扩展）
}

// NewDeploySnapshotService 构造部署快照服务。
// @param models 模型存储端口
// @param designs 设计时步骤存储端口
// @param hisSteps 快照存储端口
// @param c 时间源，nil 时回退真实时钟
// @return 部署快照服务
func NewDeploySnapshotService(models ModelMgmtStore, designs StepDesignStore, hisSteps HisStepStore, c clock.Clock) *DeploySnapshotService {
	if c == nil {
		c = clock.Real{}
	}
	return &DeploySnapshotService{models: models, designs: designs, hisSteps: hisSteps, clock: c, newID: DefaultIDGen()}
}

// SnapshotForDeploy 为一次部署生成步骤快照。
// 流程：按（租户+模型代号）定位模型 → 读取设计时步骤 → 逐条映射为 StepRule
// → 批量替换该部署的快照（幂等）→ 回写模型最近部署时间。
// @param ctx 操作上下文
// @param tenantID 租户
// @param modelCode 模型代号（=流程定义 key）
// @param deploymentID 部署 ID（快照归属，必填）
// @param definitionID 流程定义 ID（预留：workflow_his_step 无定义列，运行期经部署 ID 关联定义，当前不落库）
// @return 生成的快照规则列表
// @return 错误
func (s *DeploySnapshotService) SnapshotForDeploy(ctx context.Context, tenantID, modelCode, deploymentID, definitionID string) ([]*StepRule, error) {
	// 定位参数（租户/代号/部署）缺失统一拒绝；definitionID 预留不校验
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(modelCode) == "" || strings.TrimSpace(deploymentID) == "" {
		return nil, apiError(errCodeMMSnapBlank, "部署快照缺少租户、模型代号或部署 ID")
	}
	// 依赖存储缺失防御（嵌入式装配遗漏时明确报错而非静默空快照）
	if s.models == nil || s.designs == nil || s.hisSteps == nil {
		return nil, apiError(errCodeMMSnapStoreMiss, "部署快照依赖的存储未配置")
	}
	model, found, err := s.models.GetByCode(ctx, tenantID, modelCode)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, api.NewError(errCodeMMModelMissing, "模型不存在: "+modelCode, api.ErrNotFound)
	}
	designs, err := s.designs.ListByModel(ctx, model.ID)
	if err != nil {
		return nil, err
	}
	// 设计时步骤 → 运行期规则（字段映射与 StepRule 定义一一对应）
	rules := make([]*StepRule, 0, len(designs))
	for _, design := range designs {
		rules = append(rules, mmStepRuleOfDesign(design, deploymentID))
	}
	// 批量替换保证同部署重复快照幂等（建议调用方以 pwf WithTx 包裹写库原子性）
	if err := s.hisSteps.Put(ctx, deploymentID, rules); err != nil {
		return nil, err
	}
	// 回写模型最近部署时间
	if err := s.models.UpdateLastDeployTime(ctx, model.ID, s.clock.Now()); err != nil {
		return nil, err
	}
	return rules, nil
}

// mmStepRuleOfDesign 将设计时步骤映射为运行期快照规则。
// 映射关系（design → workflow_his_step 列）：Name→step_name、StepCode→step_code、
// AllowReturn/BackStepCodes/ReturnApprovalStep/CountersignPercentReturn/ReturnPercent
// 原样透传（数据库形态字符串，由 StepRuleService 按 Java 语义解释）、
// Sort 取整（DDL 双精度，快照排序号 StepRule 为整型）、URL→url。
// @param d 设计时步骤
// @param deploymentID 部署 ID（快照归属）
// @return 快照规则
func mmStepRuleOfDesign(d *StepDesign, deploymentID string) *StepRule {
	return &StepRule{
		DeploymentID:             deploymentID,
		StepCode:                 d.StepCode,
		StepName:                 d.Name,
		AllowReturn:              d.AllowReturn,
		BackStepCodes:            d.BackStepCodes,
		ReturnApprovalStep:       d.ReturnApprovalStep,
		CountersignPercentReturn: d.CountersignPercentReturn,
		ReturnPercent:            d.ReturnPercent,
		// 排序号 double → int 截断（设计端排序号为整数序，运行期仅用于展示排序）
		Sort: int(d.Sort),
		URL:  d.URL,
	}
}
