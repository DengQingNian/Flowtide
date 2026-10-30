package engine

import (
	"context"
	"sort"

	"github.com/DengQingNian/Flowtide/api"
	flowablerow "github.com/DengQingNian/Flowtide/internal/persistence/flowable"
)

// QueryHistoricActivityInstances 按条件分页查询历史活动。
// 行仓储模式下等值与时间条件可下推 SQL；不满足下推条件时回退内存过滤，语义一致。
// @param ctx 上下文
// @param filter 查询条件
// @return 分页历史活动
// @return 查询错误
func (e *Engine) QueryHistoricActivityInstances(ctx context.Context, filter api.HistoricActivityInstanceFilter) (api.Page[api.HistoricActivityInstance], error) {
	result, err := e.read(func() (any, error) {
		// 满足安全条件时使用行仓储 SQL，否则回退内存过滤
		if e.canPushdownHistoricActivityQuery(ctx, filter) {
			return e.queryHistoricActivityInstancesFromRowQuery(ctx, filter)
		}
		return e.queryHistoricActivityInstancesFromMemory(ctx, filter)
	})
	if err != nil {
		return api.Page[api.HistoricActivityInstance]{}, err
	}
	return result.(api.Page[api.HistoricActivityInstance]), nil
}

// canPushdownHistoricActivityQuery 判断历史活动查询是否可以安全下推到行仓储 SQL。
// 生效租户非空时以 TENANT_ID_ 等值条件下推；无租户条件且存在非默认租户历史数据时回退内存。
// @param ctx 上下文
// @param filter 查询条件
// @return 是否下推
func (e *Engine) canPushdownHistoricActivityQuery(ctx context.Context, filter api.HistoricActivityInstanceFilter) bool {
	// 未配置行仓储时只能保留内存路径
	if e.rowRepository == nil {
		return false
	}
	// 生效租户：请求过滤优先，其次上下文/默认租户
	effective := e.effectiveTenantOf(ctx, filter.TenantID)
	// 无租户条件时，内存存在非默认租户历史数据会导致 SQL 无租户条件多算总数与挤占分页
	if effective == "" && e.hasNonDefaultTenantActivityHistory() {
		return false
	}
	return true
}

// hasNonDefaultTenantActivityHistory 判断内存历史状态是否存在非默认租户的历史活动或历史实例。
// 历史活动的租户归属即历史实例租户，历史活动 DTO 自身也记录租户，两处任一非默认即回退。
// @return 是否存在非默认租户历史数据
func (e *Engine) hasNonDefaultTenantActivityHistory() bool {
	for _, historic := range e.state.HistoricActivities {
		// 历史活动自身记录非默认租户
		if historic.TenantID != "" {
			return true
		}
	}
	for _, historic := range e.state.HistoricProcesses {
		// 历史实例记录非默认租户时其下历史活动同样归属该租户
		if historic.TenantID != "" {
			return true
		}
	}
	return false
}

// queryHistoricActivityInstancesFromRowQuery 通过行仓储下推查询历史活动并按分页返回。
// @param ctx 上下文
// @param filter 查询条件
// @return 分页历史活动
// @return 查询错误
func (e *Engine) queryHistoricActivityInstancesFromRowQuery(ctx context.Context, filter api.HistoricActivityInstanceFilter) (api.Page[api.HistoricActivityInstance], error) {
	// 生效租户与下推门禁一致：请求过滤优先，其次上下文/默认租户
	effectiveTenant := e.effectiveTenantOf(ctx, filter.TenantID)
	query := flowablerow.HistoricActivityInstanceQuery{
		ID:                   filter.ID,
		ProcessInstanceID:    filter.ProcessInstanceID,
		ProcessDefinitionID:  filter.ProcessDefinitionID,
		ProcessDefinitionKey: filter.ProcessDefinitionKey,
		ActivityID:           filter.ActivityID,
		ActivityName:         filter.ActivityName,
		ActivityType:         filter.ActivityType,
		Assignee:             filter.Assignee,
		TenantID:             effectiveTenant,
		StartedAfter:         filter.StartedAfter,
		StartedBefore:        filter.StartedBefore,
		FinishedAfter:        filter.FinishedAfter,
		FinishedBefore:       filter.FinishedBefore,
		Finished:             filter.Finished,
		Start:                filter.Start,
		Size:                 filter.Size,
	}
	ids, total, err := e.rowRepository.QueryHistoricActivityInstanceIDs(ctx, query)
	// SQL 查询失败必须返回错误，不能静默回退内存路径掩盖数据库问题
	if err != nil {
		return api.Page[api.HistoricActivityInstance]{}, err
	}
	activities := make([]api.HistoricActivityInstance, 0, len(ids))
	for _, id := range ids {
		// SQL 结果与内存状态可能短暂不一致，不存在的 ID 直接跳过
		historic, ok := e.state.HistoricActivities[id]
		if !ok {
			continue
		}
		if e.allowHistoricResource(ctx, historic.ProcessInstanceID, "") != nil {
			continue
		}
		activities = append(activities, *historic)
	}
	start, size := normalizePage(filter.Start, filter.Size)
	return api.Page[api.HistoricActivityInstance]{Data: activities, Total: total, Start: start, Size: size}, nil
}

// queryHistoricActivityInstancesFromMemory 在内存中过滤、排序并分页历史活动。
// @param ctx 上下文
// @param filter 查询条件
// @return 分页历史活动
// @return 查询错误
func (e *Engine) queryHistoricActivityInstancesFromMemory(ctx context.Context, filter api.HistoricActivityInstanceFilter) (api.Page[api.HistoricActivityInstance], error) {
	matched := make([]api.HistoricActivityInstance, 0)
	for _, historic := range e.state.HistoricActivities {
		if e.allowHistoricResource(ctx, historic.ProcessInstanceID, "") != nil {
			continue
		}
		if !e.matchesHistoricActivityInstanceFilter(*historic, filter) {
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
		return api.Page[api.HistoricActivityInstance]{Data: []api.HistoricActivityInstance{}, Total: total, Start: start, Size: size}, nil
	}
	end := len(matched)
	if size < len(matched)-start {
		end = start + size
	}
	return api.Page[api.HistoricActivityInstance]{Data: matched[start:end], Total: total, Start: start, Size: size}, nil
}

// matchesHistoricActivityInstanceFilter 判断历史活动是否满足可下推字段的内存匹配语义。
// @param historic 历史活动 DTO
// @param filter 查询条件
// @return 是否匹配
func (e *Engine) matchesHistoricActivityInstanceFilter(historic api.HistoricActivityInstance, filter api.HistoricActivityInstanceFilter) bool {
	// 空条件表示不过滤；等值条件要求完全一致
	if filter.ID != "" && historic.ID != filter.ID {
		return false
	}
	if filter.ProcessInstanceID != "" && historic.ProcessInstanceID != filter.ProcessInstanceID {
		return false
	}
	if filter.ProcessDefinitionID != "" && historic.ProcessDefinitionID != filter.ProcessDefinitionID {
		return false
	}
	if filter.ActivityID != "" && historic.ActivityID != filter.ActivityID {
		return false
	}
	if filter.ActivityName != "" && historic.ActivityName != filter.ActivityName {
		return false
	}
	if filter.ActivityType != "" && historic.ActivityType != filter.ActivityType {
		return false
	}
	if filter.Assignee != "" && historic.Assignee != filter.Assignee {
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
	// 历史活动 DTO 未冗余定义 Key，需要经流程定义反查；定义缺失且有 Key 条件时视为不匹配
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
