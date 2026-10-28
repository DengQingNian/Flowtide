package migration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/engine"
	statestore "github.com/flowable-go/flowable/internal/persistence/state"
	"github.com/flowable-go/flowable/internal/tenant"
)

// StateFormat 是离线状态文件的格式标识，与 -convert 报告中的 preparedState.format 保持一致。
const StateFormat = "flowable-go-state"

// StateReport 描述离线状态文件的只读恢复校验结果，不包含变量值、部署源码或连接凭据。
type StateReport struct {
	Format                   string   `json:"format"`                   // 状态格式标识
	Version                  int      `json:"version"`                  // 状态格式版本
	Bytes                    int      `json:"bytes"`                    // 状态文件字节数
	SHA256                   string   `json:"sha256"`                   // 状态文件 SHA-256 摘要
	Deployments              int      `json:"deployments"`              // 部署数量
	Definitions              int      `json:"definitions"`              // 流程定义数量
	RunningInstances         int      `json:"runningInstances"`         // 运行中流程实例数量
	EndedInstances           int      `json:"endedInstances"`           // 已结束历史流程实例数量
	Tasks                    int      `json:"tasks"`                    // 当前用户任务数量
	HistoricProcessInstances int      `json:"historicProcessInstances"` // 历史流程实例数量
	HistoricTaskInstances    int      `json:"historicTaskInstances"`    // 历史任务数量
	HistoricVariables        int      `json:"historicVariables"`        // 历史变量数量
	SequenceFlowHistory      bool     `json:"sequenceFlowHistory"`      // 状态是否带顺序流历史标记
	Limitations              []string `json:"limitations"`              // 校验边界与无法覆盖的内容
}

// stateMetadata 只解析校验与枚举租户所需的最小字段，不读取变量值、附件内容或部署源码。
type stateMetadata struct {
	Version                   int            `json:"version"`                   // 状态格式版本
	RecordSequenceFlowHistory bool           `json:"recordSequenceFlowHistory"` // 顺序流历史标记
	Deployments               []tenantInfo   `json:"deployments"`               // 部署及其租户
	Definitions               []tenantInfo   `json:"definitions"`               // 流程定义及其租户
	Instances                 []tenantRecord `json:"instances"`                 // 运行实例及其租户
	Tasks                     []tenantTask   `json:"tasks"`                     // 当前任务及其租户
	HistoricProcesses         []tenantRecord `json:"historicProcesses"`         // 历史流程实例及其租户
}

// tenantInfo 描述带 info 包装的部署或流程定义的最小租户字段。
type tenantInfo struct {
	Info struct {
		TenantID string `json:"tenantId"` // 资源租户
	} `json:"info"`
}

// tenantRecord 描述引擎内部直接序列化的实例租户字段；字段名大小写由 encoding/json 忽略。
type tenantRecord struct {
	TenantID string `json:"tenantId"` // 资源租户
}

// tenantTask 描述任务状态中内嵌的 Task 租户字段。
type tenantTask struct {
	Task struct {
		TenantID string `json:"tenantId"` // 任务租户
	} `json:"Task"`
}

// VerifyState 在隔离内存引擎中恢复离线状态文件并汇总实体数量，不连接数据库也不写回文件。
// @param ctx 校验上下文，取消或超时后返回错误
// @param data 离线状态文件原始字节，函数不会修改输入
// @return 完整状态报告，任何解析、恢复或统计失败时为零值报告
// @return 空文件、JSON、版本、恢复、统计或取消错误
func VerifyState(ctx context.Context, data []byte) (StateReport, error) {
	// 已取消的上下文不进入解析与恢复，避免无意义的恢复工作。
	if err := ctx.Err(); err != nil {
		return StateReport{}, fmt.Errorf("校验上下文已取消: %w", err)
	}
	// 空文件和仅含空白的内容都不是可恢复状态。
	if len(bytes.TrimSpace(data)) == 0 {
		return StateReport{}, errors.New("状态文件为空")
	}
	var metadata stateMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return StateReport{}, fmt.Errorf("解析状态文件失败: %w", err)
	}
	// 负版本不可能是历史兼容格式，直接拒绝而不进入恢复。
	if metadata.Version < 0 {
		return StateReport{}, fmt.Errorf("不支持的快照版本: %d", metadata.Version)
	}
	digest := sha256.Sum256(data)
	store := statestore.NewMemory()
	if err := store.Save(ctx, data); err != nil {
		return StateReport{}, fmt.Errorf("写入隔离状态存储失败: %w", err)
	}
	// 复用引擎统一恢复入口；未知新版本、损坏类型与恢复失败均由该入口拒绝。
	workflow, err := engine.New(engine.Config{StateStore: store, HistoryLevel: engine.HistoryAudit})
	if err != nil {
		return StateReport{}, fmt.Errorf("恢复离线状态失败: %w", err)
	}
	report, err := collectStateReport(ctx, workflow, metadata)
	if err != nil {
		return StateReport{}, err
	}
	// 恢复与统计期间也可能超时，输出前再次确认，避免返回不完整结果。
	if err := ctx.Err(); err != nil {
		return StateReport{}, fmt.Errorf("校验上下文已取消: %w", err)
	}
	report.Format = StateFormat
	report.Version = metadata.Version
	report.Bytes = len(data)
	report.SHA256 = hex.EncodeToString(digest[:])
	report.SequenceFlowHistory = metadata.RecordSequenceFlowHistory
	report.Limitations = stateLimitations()
	return report, nil
}

// collectStateReport 使用公共查询 API 逐租户统计恢复后的状态实体。
// @param ctx 校验上下文
// @param workflow 已恢复的隔离内存引擎
// @param metadata 仅用于枚举租户的状态元数据
// @return 实体数量报告，失败时为零值报告
// @return 取消或查询错误
func collectStateReport(ctx context.Context, workflow *engine.Engine, metadata stateMetadata) (StateReport, error) {
	var report StateReport
	// 同一流程实例可能同时出现在运行列表和历史列表，先按实例 ID 归并租户，避免重复统计历史行。
	instances := make(map[string]string)
	// 状态可能由显式租户转换产生；逐租户查询才能覆盖全部实体，且不使用引擎私有字段。
	for _, tenantID := range stateTenants(metadata) {
		if err := ctx.Err(); err != nil {
			return StateReport{}, fmt.Errorf("校验上下文已取消: %w", err)
		}
		tenantCtx := tenant.With(ctx, tenantID)
		deployments, err := workflow.ListDeployments(tenantCtx)
		if err != nil {
			return StateReport{}, fmt.Errorf("统计部署失败: %w", err)
		}
		report.Deployments += len(deployments)
		definitions, err := workflow.ListProcessDefinitions(tenantCtx)
		if err != nil {
			return StateReport{}, fmt.Errorf("统计流程定义失败: %w", err)
		}
		report.Definitions += len(definitions)
		running, err := workflow.ListProcessInstances(tenantCtx)
		if err != nil {
			return StateReport{}, fmt.Errorf("统计运行实例失败: %w", err)
		}
		report.RunningInstances += len(running)
		for _, instance := range running {
			instances[instance.ID] = tenantID
		}
		historic, err := workflow.ListHistoricProcessInstances(tenantCtx)
		if err != nil {
			return StateReport{}, fmt.Errorf("统计历史流程实例失败: %w", err)
		}
		report.HistoricProcessInstances += len(historic)
		for _, instance := range historic {
			// 只有具备结束时间的历史流程实例才算已结束，运行中实例仍计入历史容器。
			if instance.EndTime != nil {
				report.EndedInstances++
			}
			if _, exists := instances[instance.ID]; !exists {
				instances[instance.ID] = tenantID
			}
		}
		tasks, err := workflow.ListTasks(tenantCtx, api.TaskFilter{Size: 1})
		if err != nil {
			return StateReport{}, fmt.Errorf("统计当前任务失败: %w", err)
		}
		report.Tasks += int(tasks.Total)
	}
	// 历史任务与历史变量按实例查询，累计结果不包含任何变量值或任务内容。
	for instanceID, tenantID := range instances {
		if err := ctx.Err(); err != nil {
			return StateReport{}, fmt.Errorf("校验上下文已取消: %w", err)
		}
		tenantCtx := tenant.With(ctx, tenantID)
		tasks, err := workflow.ListHistoricTaskInstances(tenantCtx, instanceID)
		if err != nil {
			return StateReport{}, fmt.Errorf("统计历史任务失败: %w", err)
		}
		report.HistoricTaskInstances += len(tasks)
		variables, err := workflow.ListHistoricVariableInstances(tenantCtx, instanceID)
		if err != nil {
			return StateReport{}, fmt.Errorf("统计历史变量失败: %w", err)
		}
		report.HistoricVariables += len(variables)
	}
	return report, nil
}

// stateTenants 汇总状态中可能出现的全部租户，默认租户始终参与查询。
// @param metadata 状态元数据
// @return 去重并排序的租户标识
func stateTenants(metadata stateMetadata) []string {
	seen := map[string]bool{"": true}
	for _, deployment := range metadata.Deployments {
		addTenant(seen, deployment.Info.TenantID)
	}
	for _, definition := range metadata.Definitions {
		addTenant(seen, definition.Info.TenantID)
	}
	for _, instance := range metadata.Instances {
		addTenant(seen, instance.TenantID)
	}
	for _, task := range metadata.Tasks {
		addTenant(seen, task.Task.TenantID)
	}
	for _, instance := range metadata.HistoricProcesses {
		addTenant(seen, instance.TenantID)
	}
	tenants := make([]string, 0, len(seen))
	for tenantID := range seen {
		tenants = append(tenants, tenantID)
	}
	// 固定查询顺序便于测试与排查，不影响只读统计结果。
	sort.Strings(tenants)
	return tenants
}

// addTenant 将首次出现的租户加入集合。
// @param seen 已收集租户集合
// @param tenantID 租户标识
// @return 无
func addTenant(seen map[string]bool, tenantID string) {
	// 只记录首次出现的租户，避免同一租户重复查询。
	if !seen[tenantID] {
		seen[tenantID] = true
	}
}

// stateLimitations 返回恢复校验始终存在的边界说明。
// @return 边界说明
func stateLimitations() []string {
	return []string{
		"只校验状态文件可解析并在隔离内存引擎中恢复，不证明其能被生产服务长期接管。",
		"报告只包含格式摘要、实体数量和固定边界说明，不包含变量值、部署源码或连接凭据。",
		"恢复成功不替代停写、备份、执行者切换与回退演练；禁止 Java 与 Go 同时处理同一批实例。",
		"历史任务与历史变量按运行中及历史流程实例逐实例统计，未归属任何流程实例的历史行不计入。",
		"校验不连接源数据库，也不核对状态与源库的数量是否一致；数量对照必须在停写快照上另行执行。",
	}
}
