package engine

import (
	"context"
	"sort"

	"github.com/flowable-go/flowable/api"
	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
)

// QueryHistoricVariableInstances 按条件分页查询历史变量。
// 行仓储模式下等值条件可下推 SQL；不满足下推条件时回退内存过滤，语义一致。
// ACT_HI_VARINST 未持久化租户列，因此任何租户作用域（生效租户非空或存在非默认租户历史）一律回退内存。
// @param ctx 上下文
// @param filter 查询条件
// @return 分页历史变量
// @return 查询错误
func (e *Engine) QueryHistoricVariableInstances(ctx context.Context, filter api.HistoricVariableInstanceFilter) (api.Page[api.HistoricVariableInstance], error) {
	result, err := e.read(func() (any, error) {
		// 满足安全条件时使用行仓储 SQL，否则回退内存过滤
		if e.canPushdownHistoricVariableQuery(ctx) {
			return e.queryHistoricVariableInstancesFromRowQuery(ctx, filter)
		}
		return e.queryHistoricVariableInstancesFromMemory(ctx, filter)
	})
	if err != nil {
		return api.Page[api.HistoricVariableInstance]{}, err
	}
	return result.(api.Page[api.HistoricVariableInstance]), nil
}

// canPushdownHistoricVariableQuery 判断历史变量查询是否可以安全下推到行仓储 SQL。
// 历史变量表没有租户列，生效租户非空或内存存在非默认租户历史数据时无法在 SQL 内收敛租户，必须回退内存。
// @param ctx 上下文
// @return 是否下推
func (e *Engine) canPushdownHistoricVariableQuery(ctx context.Context) bool {
	// 未配置行仓储时只能保留内存路径
	if e.rowRepository == nil {
		return false
	}
	// 租户作用域无法下推：TENANT_ID_ 缺失会让 SQL 统计到其他租户的变量
	if e.effectiveTenantOf(ctx, "") != "" {
		return false
	}
	// 默认租户视图下若内存存在非默认租户历史实例或独立历史任务，其变量同样归属其他租户
	if e.hasNonDefaultTenantVariableHistory() {
		return false
	}
	return true
}

// hasNonDefaultTenantVariableHistory 判断内存历史状态是否存在非默认租户的历史变量来源。
// 历史变量没有租户列，其租户由所属历史实例或独立历史任务决定，两处任一非默认即回退内存。
// @return 是否存在非默认租户历史变量来源
func (e *Engine) hasNonDefaultTenantVariableHistory() bool {
	for _, historic := range e.state.HistoricProcesses {
		// 流程变量与任务变量都随历史实例归属租户
		if historic.TenantID != "" {
			return true
		}
	}
	for _, historic := range e.state.HistoricTasks {
		// 无历史实例的独立任务，其任务变量同样归属任务租户
		if historic.TenantID != "" {
			return true
		}
	}
	return false
}

// queryHistoricVariableInstancesFromRowQuery 通过行仓储下推查询历史变量并按分页返回。
// @param ctx 上下文
// @param filter 查询条件
// @return 分页历史变量
// @return 查询错误
func (e *Engine) queryHistoricVariableInstancesFromRowQuery(ctx context.Context, filter api.HistoricVariableInstanceFilter) (api.Page[api.HistoricVariableInstance], error) {
	query := flowablerow.HistoricVariableInstanceQuery{
		ID:                filter.ID,
		Name:              filter.Name,
		ProcessInstanceID: filter.ProcessInstanceID,
		ExecutionID:       filter.ExecutionID,
		TaskID:            filter.TaskID,
		VariableType:      filter.VariableType,
		Start:             filter.Start,
		Size:              filter.Size,
	}
	ids, total, err := e.rowRepository.QueryHistoricVariableInstanceIDs(ctx, query)
	// SQL 查询失败必须返回错误，不能静默回退内存路径掩盖数据库问题
	if err != nil {
		return api.Page[api.HistoricVariableInstance]{}, err
	}
	variables := make([]api.HistoricVariableInstance, 0, len(ids))
	for _, id := range ids {
		// SQL 结果与内存状态可能短暂不一致，不存在的 ID 直接跳过
		historic, ok := e.state.HistoricVariables[id]
		if !ok {
			continue
		}
		if e.allowHistoricResource(ctx, historic.ProcessInstanceID, historic.TaskID) != nil {
			continue
		}
		variables = append(variables, *historic)
	}
	start, size := normalizePage(filter.Start, filter.Size)
	return api.Page[api.HistoricVariableInstance]{Data: variables, Total: total, Start: start, Size: size}, nil
}

// queryHistoricVariableInstancesFromMemory 在内存中过滤、排序并分页历史变量。
// @param ctx 上下文
// @param filter 查询条件
// @return 分页历史变量
// @return 查询错误
func (e *Engine) queryHistoricVariableInstancesFromMemory(ctx context.Context, filter api.HistoricVariableInstanceFilter) (api.Page[api.HistoricVariableInstance], error) {
	matched := make([]api.HistoricVariableInstance, 0)
	for _, historic := range e.state.HistoricVariables {
		if e.allowHistoricResource(ctx, historic.ProcessInstanceID, historic.TaskID) != nil {
			continue
		}
		if !matchesHistoricVariableInstanceFilter(*historic, filter) {
			continue
		}
		matched = append(matched, *historic)
	}
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].Name == matched[j].Name {
			return matched[i].ID < matched[j].ID
		}
		return matched[i].Name < matched[j].Name
	})
	start, size := normalizePage(filter.Start, filter.Size)
	total := int64(len(matched))
	// 先比较剩余条数再相加，避免极大偏移量与每页数量相加溢出
	if start >= len(matched) {
		return api.Page[api.HistoricVariableInstance]{Data: []api.HistoricVariableInstance{}, Total: total, Start: start, Size: size}, nil
	}
	end := len(matched)
	if size < len(matched)-start {
		end = start + size
	}
	return api.Page[api.HistoricVariableInstance]{Data: matched[start:end], Total: total, Start: start, Size: size}, nil
}

// matchesHistoricVariableInstanceFilter 判断历史变量是否满足可下推字段的内存匹配语义。
// @param historic 历史变量 DTO
// @param filter 查询条件
// @return 是否匹配
func matchesHistoricVariableInstanceFilter(historic api.HistoricVariableInstance, filter api.HistoricVariableInstanceFilter) bool {
	// 空条件表示不过滤；等值条件要求完全一致
	if filter.ID != "" && historic.ID != filter.ID {
		return false
	}
	if filter.Name != "" && historic.Name != filter.Name {
		return false
	}
	if filter.ProcessInstanceID != "" && historic.ProcessInstanceID != filter.ProcessInstanceID {
		return false
	}
	if filter.ExecutionID != "" && historic.ExecutionID != filter.ExecutionID {
		return false
	}
	if filter.TaskID != "" && historic.TaskID != filter.TaskID {
		return false
	}
	if filter.VariableType != "" && historic.Type != filter.VariableType {
		return false
	}
	return true
}
