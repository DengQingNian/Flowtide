package engine

import (
	"context"
	"sort"

	"github.com/flowable-go/flowable/api"
	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
)

// QueryHistoricTaskInstances 按条件分页查询历史任务。
// 行仓储模式下等值与时间条件可下推 SQL；不满足下推条件时回退内存过滤，语义一致。
// @param ctx 上下文
// @param filter 查询条件
// @return 分页历史任务
// @return 查询错误
func (e *Engine) QueryHistoricTaskInstances(ctx context.Context, filter api.HistoricTaskInstanceFilter) (api.Page[api.HistoricTaskInstance], error) {
	result, err := e.read(func() (any, error) {
		// 满足安全条件时使用行仓储 SQL，否则回退内存过滤
		if e.canPushdownHistoricTaskQuery(ctx, filter) {
			return e.queryHistoricTaskInstancesFromRowQuery(ctx, filter)
		}
		return e.queryHistoricTaskInstancesFromMemory(ctx, filter)
	})
	if err != nil {
		return api.Page[api.HistoricTaskInstance]{}, err
	}
	return result.(api.Page[api.HistoricTaskInstance]), nil
}

// canPushdownHistoricTaskQuery 判断历史任务查询是否可以安全下推到行仓储 SQL。
// 生效租户非空时以 TENANT_ID_ 等值条件下推；无租户条件且存在非默认租户历史任务或历史实例时回退内存。
// @param ctx 上下文
// @param filter 查询条件
// @return 是否下推
func (e *Engine) canPushdownHistoricTaskQuery(ctx context.Context, filter api.HistoricTaskInstanceFilter) bool {
	// 未配置行仓储时只能保留内存路径
	if e.rowRepository == nil {
		return false
	}
	// 生效租户：请求过滤优先，其次上下文/默认租户
	effective := e.effectiveTenantOf(ctx, filter.TenantID)
	// 无租户条件时，内存存在非默认租户历史数据会导致 SQL 无租户条件多算总数与挤占分页
	if effective == "" && e.hasNonDefaultTenantTaskHistory() {
		return false
	}
	return true
}

// hasNonDefaultTenantTaskHistory 判断内存历史状态是否存在非默认租户的历史任务或历史实例。
// 历史任务的租户归属即历史实例租户，历史任务 DTO 自身也记录租户，两处任一非默认即回退。
// @return 是否存在非默认租户历史数据
func (e *Engine) hasNonDefaultTenantTaskHistory() bool {
	for _, historic := range e.state.HistoricTasks {
		// 历史任务自身记录非默认租户
		if historic.TenantID != "" {
			return true
		}
	}
	for _, historic := range e.state.HistoricProcesses {
		// 历史实例记录非默认租户时其下历史任务同样归属该租户
		if historic.TenantID != "" {
			return true
		}
	}
	return false
}

// queryHistoricTaskInstancesFromRowQuery 通过行仓储下推查询历史任务并按分页返回。
// @param ctx 上下文
// @param filter 查询条件
// @return 分页历史任务
// @return 查询错误
func (e *Engine) queryHistoricTaskInstancesFromRowQuery(ctx context.Context, filter api.HistoricTaskInstanceFilter) (api.Page[api.HistoricTaskInstance], error) {
	// 生效租户与下推门禁一致：请求过滤优先，其次上下文/默认租户
	effectiveTenant := e.effectiveTenantOf(ctx, filter.TenantID)
	query := flowablerow.HistoricTaskInstanceQuery{
		ID:                   filter.ID,
		Name:                 filter.Name,
		Assignee:             filter.Assignee,
		TaskDefinitionKey:    filter.TaskDefinitionKey,
		ProcessInstanceID:    filter.ProcessInstanceID,
		ProcessDefinitionID:  filter.ProcessDefinitionID,
		ProcessDefinitionKey: filter.ProcessDefinitionKey,
		TenantID:             effectiveTenant,
		StartedAfter:         filter.StartedAfter,
		StartedBefore:        filter.StartedBefore,
		FinishedAfter:        filter.FinishedAfter,
		FinishedBefore:       filter.FinishedBefore,
		Finished:             filter.Finished,
		Start:                filter.Start,
		Size:                 filter.Size,
	}
	ids, total, err := e.rowRepository.QueryHistoricTaskInstanceIDs(ctx, query)
	// SQL 查询失败必须返回错误，不能静默回退内存路径掩盖数据库问题
	if err != nil {
		return api.Page[api.HistoricTaskInstance]{}, err
	}
	tasks := make([]api.HistoricTaskInstance, 0, len(ids))
	for _, id := range ids {
		// SQL 结果与内存状态可能短暂不一致，不存在的 ID 直接跳过
		historic, ok := e.state.HistoricTasks[id]
		if !ok {
			continue
		}
		if e.allowHistoricResource(ctx, historic.ProcessInstanceID, historic.ID) != nil {
			continue
		}
		tasks = append(tasks, *historic)
	}
	start, size := normalizePage(filter.Start, filter.Size)
	return api.Page[api.HistoricTaskInstance]{Data: tasks, Total: total, Start: start, Size: size}, nil
}

// queryHistoricTaskInstancesFromMemory 在内存中过滤、排序并分页历史任务。
// @param ctx 上下文
// @param filter 查询条件
// @return 分页历史任务
// @return 查询错误
func (e *Engine) queryHistoricTaskInstancesFromMemory(ctx context.Context, filter api.HistoricTaskInstanceFilter) (api.Page[api.HistoricTaskInstance], error) {
	matched := make([]api.HistoricTaskInstance, 0)
	for _, historic := range e.state.HistoricTasks {
		if e.allowHistoricResource(ctx, historic.ProcessInstanceID, historic.ID) != nil {
			continue
		}
		if !e.matchesHistoricTaskInstanceFilter(*historic, filter) {
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
		return api.Page[api.HistoricTaskInstance]{Data: []api.HistoricTaskInstance{}, Total: total, Start: start, Size: size}, nil
	}
	end := len(matched)
	if size < len(matched)-start {
		end = start + size
	}
	return api.Page[api.HistoricTaskInstance]{Data: matched[start:end], Total: total, Start: start, Size: size}, nil
}

// matchesHistoricTaskInstanceFilter 判断历史任务是否满足可下推字段的内存匹配语义。
// @param historic 历史任务 DTO
// @param filter 查询条件
// @return 是否匹配
func (e *Engine) matchesHistoricTaskInstanceFilter(historic api.HistoricTaskInstance, filter api.HistoricTaskInstanceFilter) bool {
	// 空条件表示不过滤；等值条件要求完全一致
	if filter.ID != "" && historic.ID != filter.ID {
		return false
	}
	if filter.Name != "" && historic.Name != filter.Name {
		return false
	}
	if filter.Assignee != "" && historic.Assignee != filter.Assignee {
		return false
	}
	if filter.TaskDefinitionKey != "" && historic.TaskDefinitionKey != filter.TaskDefinitionKey {
		return false
	}
	if filter.ProcessInstanceID != "" && historic.ProcessInstanceID != filter.ProcessInstanceID {
		return false
	}
	if filter.ProcessDefinitionID != "" && historic.ProcessDefinitionID != filter.ProcessDefinitionID {
		return false
	}
	// 租户为空表示不过滤；非空时要求完全一致，与 SQL TENANT_ID_ 等值语义一致
	if filter.TenantID != "" && historic.TenantID != filter.TenantID {
		return false
	}
	// 结束状态以结束时间是否为空判断，与 REST finished 语义一致
	if filter.Finished != nil && (historic.EndTime != nil) != *filter.Finished {
		return false
	}
	// 边界语义与 SQL 一致：下界含等于、上界含等于
	if filter.StartedAfter != nil && historic.StartTime.Before(*filter.StartedAfter) {
		return false
	}
	if filter.StartedBefore != nil && historic.StartTime.After(*filter.StartedBefore) {
		return false
	}
	// 历史任务 DTO 未冗余定义 Key，需要经流程定义反查；定义缺失且有 Key 条件时视为不匹配
	if filter.ProcessDefinitionKey != "" {
		definition, ok := e.findDefinition(historic.ProcessDefinitionID)
		if !ok {
			return false
		}
		if definition.Info.Key != filter.ProcessDefinitionKey {
			return false
		}
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
