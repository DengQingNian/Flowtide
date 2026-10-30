package engine

import (
	"context"
	"sort"

	"github.com/DengQingNian/Flowtide/api"
	flowablerow "github.com/DengQingNian/Flowtide/internal/persistence/flowable"
)

// QueryProcessInstances 按条件分页查询运行中实例。
// 行仓储模式下等值与时间条件可下推 SQL；不满足下推条件时回退内存过滤，语义一致。
// @param ctx 上下文
// @param filter 查询条件
// @return 分页实例
// @return 查询错误
func (e *Engine) QueryProcessInstances(ctx context.Context, filter api.ProcessInstanceFilter) (api.Page[api.ProcessInstance], error) {
	result, err := e.read(func() (any, error) {
		// 满足安全条件时使用行仓储 SQL，否则回退内存过滤
		if e.canPushdownInstanceQuery(ctx, filter) {
			return e.queryProcessInstancesFromRowQuery(ctx, filter)
		}
		return e.queryProcessInstancesFromMemory(ctx, filter)
	})
	if err != nil {
		return api.Page[api.ProcessInstance]{}, err
	}
	return result.(api.Page[api.ProcessInstance]), nil
}

// canPushdownInstanceQuery 判断实例查询是否可以安全下推到行仓储 SQL。
// 生效租户非空时以 TENANT_ID_ 等值条件下推；无租户条件且存在非默认租户数据时回退内存。
// @param ctx 上下文
// @param filter 查询条件
// @return 是否下推
func (e *Engine) canPushdownInstanceQuery(ctx context.Context, filter api.ProcessInstanceFilter) bool {
	// 未配置行仓储时只能保留内存路径
	if e.rowRepository == nil {
		return false
	}
	// 生效租户：请求过滤优先，其次上下文/默认租户
	effective := e.effectiveTenantOf(ctx, filter.TenantID)
	// 无租户条件时，内存存在非默认租户数据会导致 SQL 无租户条件多算总数与挤占分页
	if effective == "" && e.hasNonDefaultTenantData() {
		return false
	}
	return true
}

// queryProcessInstancesFromRowQuery 通过行仓储下推查询实例并按分页返回。
// @param ctx 上下文
// @param filter 查询条件
// @return 分页实例
// @return 查询错误
func (e *Engine) queryProcessInstancesFromRowQuery(ctx context.Context, filter api.ProcessInstanceFilter) (api.Page[api.ProcessInstance], error) {
	// 生效租户与下推门禁一致：请求过滤优先，其次上下文/默认租户
	effectiveTenant := e.effectiveTenantOf(ctx, filter.TenantID)
	query := flowablerow.ProcessInstanceQuery{
		ID:                   filter.ID,
		ProcessDefinitionID:  filter.ProcessDefinitionID,
		ProcessDefinitionKey: filter.ProcessDefinitionKey,
		BusinessKey:          filter.BusinessKey,
		TenantID:             effectiveTenant,
		Suspended:            filter.Suspended,
		StartedAfter:         filter.StartedAfter,
		StartedBefore:        filter.StartedBefore,
		Start:                filter.Start,
		Size:                 filter.Size,
	}
	ids, total, err := e.rowRepository.QueryProcessInstanceIDs(ctx, query)
	// SQL 查询失败必须返回错误，不能静默回退内存路径掩盖数据库问题
	if err != nil {
		return api.Page[api.ProcessInstance]{}, err
	}
	instances := make([]api.ProcessInstance, 0, len(ids))
	for _, id := range ids {
		// SQL 结果与内存状态可能短暂不一致，已结束或不存在的实例直接跳过
		instance, ok := e.state.Instances[id]
		if !ok || instance.Ended {
			continue
		}
		if e.allowTenant(ctx, instance.TenantID) != nil {
			continue
		}
		instances = append(instances, e.toProcessInstance(instance))
	}
	start, size := normalizePage(filter.Start, filter.Size)
	return api.Page[api.ProcessInstance]{Data: instances, Total: total, Start: start, Size: size}, nil
}

// queryProcessInstancesFromMemory 在内存中过滤、排序并分页运行中实例。
// @param ctx 上下文
// @param filter 查询条件
// @return 分页实例
// @return 查询错误
func (e *Engine) queryProcessInstancesFromMemory(ctx context.Context, filter api.ProcessInstanceFilter) (api.Page[api.ProcessInstance], error) {
	matched := make([]api.ProcessInstance, 0)
	for _, instance := range e.state.Instances {
		if e.allowTenant(ctx, instance.TenantID) != nil {
			continue
		}
		// 运行中列表不包含已结束实例
		if instance.Ended {
			continue
		}
		if !matchesProcessInstanceFilter(e.toProcessInstance(instance), filter) {
			continue
		}
		matched = append(matched, e.toProcessInstance(instance))
	}
	sortInstances(matched)
	start, size := normalizePage(filter.Start, filter.Size)
	total := int64(len(matched))
	// 先比较剩余条数再相加，避免极大偏移量与每页数量相加溢出
	if start >= len(matched) {
		return api.Page[api.ProcessInstance]{Data: []api.ProcessInstance{}, Total: total, Start: start, Size: size}, nil
	}
	end := len(matched)
	if size < len(matched)-start {
		end = start + size
	}
	return api.Page[api.ProcessInstance]{Data: matched[start:end], Total: total, Start: start, Size: size}, nil
}

// matchesProcessInstanceFilter 判断实例是否满足可下推字段的内存匹配语义。
// @param instance 实例 DTO
// @param filter 查询条件
// @return 是否匹配
func matchesProcessInstanceFilter(instance api.ProcessInstance, filter api.ProcessInstanceFilter) bool {
	// 空条件表示不过滤；等值条件要求完全一致
	if filter.ID != "" && instance.ID != filter.ID {
		return false
	}
	if filter.ProcessDefinitionID != "" && instance.ProcessDefinitionID != filter.ProcessDefinitionID {
		return false
	}
	if filter.ProcessDefinitionKey != "" && instance.ProcessDefinitionKey != filter.ProcessDefinitionKey {
		return false
	}
	if filter.BusinessKey != "" && instance.BusinessKey != filter.BusinessKey {
		return false
	}
	// 租户为空表示不过滤；非空时要求完全一致，与 SQL TENANT_ID_ 等值语义一致
	if filter.TenantID != "" && instance.TenantID != filter.TenantID {
		return false
	}
	if filter.Suspended != nil && instance.Suspended != *filter.Suspended {
		return false
	}
	// 边界语义与 SQL 一致：下界含等于、上界含等于
	if filter.StartedAfter != nil && instance.StartTime.Before(*filter.StartedAfter) {
		return false
	}
	if filter.StartedBefore != nil && instance.StartTime.After(*filter.StartedBefore) {
		return false
	}
	return true
}

// QueryHistoricProcessInstances 按条件分页查询历史实例。
// 行仓储模式下等值与时间条件可下推 SQL；不满足下推条件时回退内存过滤，语义一致。
// @param ctx 上下文
// @param filter 查询条件
// @return 分页历史实例
// @return 查询错误
func (e *Engine) QueryHistoricProcessInstances(ctx context.Context, filter api.HistoricProcessInstanceFilter) (api.Page[api.HistoricProcessInstance], error) {
	result, err := e.read(func() (any, error) {
		// 满足安全条件时使用行仓储 SQL，否则回退内存过滤
		if e.canPushdownHistoricProcessQuery(ctx, filter) {
			return e.queryHistoricProcessInstancesFromRowQuery(ctx, filter)
		}
		return e.queryHistoricProcessInstancesFromMemory(ctx, filter)
	})
	if err != nil {
		return api.Page[api.HistoricProcessInstance]{}, err
	}
	return result.(api.Page[api.HistoricProcessInstance]), nil
}

// canPushdownHistoricProcessQuery 判断历史实例查询是否可以安全下推到行仓储 SQL。
// 生效租户非空时以 TENANT_ID_ 等值条件下推；无租户条件且存在非默认租户历史数据时回退内存。
// @param ctx 上下文
// @param filter 查询条件
// @return 是否下推
func (e *Engine) canPushdownHistoricProcessQuery(ctx context.Context, filter api.HistoricProcessInstanceFilter) bool {
	// 未配置行仓储时只能保留内存路径
	if e.rowRepository == nil {
		return false
	}
	// 生效租户：请求过滤优先，其次上下文/默认租户
	effective := e.effectiveTenantOf(ctx, filter.TenantID)
	// 无租户条件时，内存存在非默认租户历史数据会导致 SQL 无租户条件多算总数与挤占分页
	if effective == "" && e.hasNonDefaultTenantHistory() {
		return false
	}
	return true
}

// hasNonDefaultTenantHistory 判断内存历史状态是否存在非默认租户的历史实例。
// @return 是否存在非默认租户历史数据
func (e *Engine) hasNonDefaultTenantHistory() bool {
	for _, historic := range e.state.HistoricProcesses {
		// 历史实例自身记录非默认租户
		if historic.TenantID != "" {
			return true
		}
	}
	return false
}

// queryHistoricProcessInstancesFromRowQuery 通过行仓储下推查询历史实例并按分页返回。
// @param ctx 上下文
// @param filter 查询条件
// @return 分页历史实例
// @return 查询错误
func (e *Engine) queryHistoricProcessInstancesFromRowQuery(ctx context.Context, filter api.HistoricProcessInstanceFilter) (api.Page[api.HistoricProcessInstance], error) {
	// 生效租户与下推门禁一致：请求过滤优先，其次上下文/默认租户
	effectiveTenant := e.effectiveTenantOf(ctx, filter.TenantID)
	query := flowablerow.HistoricProcessInstanceQuery{
		ID:                   filter.ID,
		ProcessDefinitionID:  filter.ProcessDefinitionID,
		ProcessDefinitionKey: filter.ProcessDefinitionKey,
		BusinessKey:          filter.BusinessKey,
		TenantID:             effectiveTenant,
		StartedAfter:         filter.StartedAfter,
		StartedBefore:        filter.StartedBefore,
		FinishedAfter:        filter.FinishedAfter,
		FinishedBefore:       filter.FinishedBefore,
		Finished:             filter.Finished,
		Start:                filter.Start,
		Size:                 filter.Size,
	}
	ids, total, err := e.rowRepository.QueryHistoricProcessInstanceIDs(ctx, query)
	// SQL 查询失败必须返回错误，不能静默回退内存路径掩盖数据库问题
	if err != nil {
		return api.Page[api.HistoricProcessInstance]{}, err
	}
	instances := make([]api.HistoricProcessInstance, 0, len(ids))
	for _, id := range ids {
		// SQL 结果与内存状态可能短暂不一致，不存在的 ID 直接跳过
		historic, ok := e.state.HistoricProcesses[id]
		if !ok {
			continue
		}
		if e.allowTenant(ctx, historic.TenantID) != nil {
			continue
		}
		instances = append(instances, *historic)
	}
	start, size := normalizePage(filter.Start, filter.Size)
	return api.Page[api.HistoricProcessInstance]{Data: instances, Total: total, Start: start, Size: size}, nil
}

// queryHistoricProcessInstancesFromMemory 在内存中过滤、排序并分页历史实例。
// @param ctx 上下文
// @param filter 查询条件
// @return 分页历史实例
// @return 查询错误
func (e *Engine) queryHistoricProcessInstancesFromMemory(ctx context.Context, filter api.HistoricProcessInstanceFilter) (api.Page[api.HistoricProcessInstance], error) {
	matched := make([]api.HistoricProcessInstance, 0)
	for _, historic := range e.state.HistoricProcesses {
		if e.allowTenant(ctx, historic.TenantID) != nil {
			continue
		}
		if !matchesHistoricProcessInstanceFilter(*historic, filter) {
			continue
		}
		matched = append(matched, *historic)
	}
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].StartTime.Equal(matched[j].StartTime) {
			return matched[i].ID < matched[j].ID
		}
		return matched[i].StartTime.Before(matched[j].StartTime)
	})
	start, size := normalizePage(filter.Start, filter.Size)
	total := int64(len(matched))
	// 先比较剩余条数再相加，避免极大偏移量与每页数量相加溢出
	if start >= len(matched) {
		return api.Page[api.HistoricProcessInstance]{Data: []api.HistoricProcessInstance{}, Total: total, Start: start, Size: size}, nil
	}
	end := len(matched)
	if size < len(matched)-start {
		end = start + size
	}
	return api.Page[api.HistoricProcessInstance]{Data: matched[start:end], Total: total, Start: start, Size: size}, nil
}

// matchesHistoricProcessInstanceFilter 判断历史实例是否满足可下推字段的内存匹配语义。
// @param historic 历史实例 DTO
// @param filter 查询条件
// @return 是否匹配
func matchesHistoricProcessInstanceFilter(historic api.HistoricProcessInstance, filter api.HistoricProcessInstanceFilter) bool {
	// 空条件表示不过滤；等值条件要求完全一致
	if filter.ID != "" && historic.ID != filter.ID {
		return false
	}
	if filter.ProcessDefinitionID != "" && historic.ProcessDefinitionID != filter.ProcessDefinitionID {
		return false
	}
	if filter.ProcessDefinitionKey != "" && historic.ProcessDefinitionKey != filter.ProcessDefinitionKey {
		return false
	}
	if filter.BusinessKey != "" && historic.BusinessKey != filter.BusinessKey {
		return false
	}
	// 租户为空表示不过滤；非空时要求完全一致，与 SQL TENANT_ID_ 等值语义一致
	if filter.TenantID != "" && historic.TenantID != filter.TenantID {
		return false
	}
	if filter.Finished != nil && historic.Ended != *filter.Finished {
		return false
	}
	// 边界语义与 SQL 一致：下界含等于、上界含等于
	if filter.StartedAfter != nil && historic.StartTime.Before(*filter.StartedAfter) {
		return false
	}
	if filter.StartedBefore != nil && historic.StartTime.After(*filter.StartedBefore) {
		return false
	}
	// 结束时间为空时不满足任何结束区间，与可空时间匹配语义一致
	if historic.EndTime == nil {
		return filter.FinishedAfter == nil && filter.FinishedBefore == nil
	}
	if filter.FinishedAfter != nil && historic.EndTime.Before(*filter.FinishedAfter) {
		return false
	}
	if filter.FinishedBefore != nil && historic.EndTime.After(*filter.FinishedBefore) {
		return false
	}
	return true
}
