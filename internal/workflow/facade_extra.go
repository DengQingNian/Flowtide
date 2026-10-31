package workflow

// 本文件承载门面收尾能力：审阅绑定（活动扩展）保存/查询、会签权重、签收/委派处理、
// 批量挂起激活、实例分页、身份树、操作按钮聚合。装配复用 facade_ops.go 的 components。

import (
	"context"
	"sort"

	"github.com/DengQingNian/Flowtide/api"
)

// SaveActExtend 保存审阅人绑定（活动扩展，workflow_act_extend 一行）。
// @param ctx 上下文
// @param ext 绑定实体（ActivityID/ProcessDefinitionKey/OwnerType/OwnerValue 必填）
// @return 统一返回（data 为落库实体）
// @return 错误
func (s *Service) SaveActExtend(ctx context.Context, ext *ActExtend) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	if ext == nil || ext.ActivityID == "" || ext.ProcessDefinitionKey == "" {
		return R{}, api.NewError("WORKFLOW-0400", "活动扩展缺少节点或定义 Key", api.ErrBadRequest)
	}
	if err := s.comp.extendsStore.Save(ctx, ext); err != nil {
		return R{}, err
	}
	return ROkMsg("保存成功", ext), nil
}

// ListActExtends 按定义 Key（可加节点）查询审阅人绑定列表。
// @param ctx 上下文
// @param definitionKey 流程定义 Key
// @param activityID 节点 ID（空表示该定义全部）
// @return 统一返回（data 为绑定列表）
// @return 错误
func (s *Service) ListActExtends(ctx context.Context, definitionKey, activityID string) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	if definitionKey == "" {
		return R{}, api.NewError("WORKFLOW-0400", "参数 processDefinitionKey 为空", api.ErrBadRequest)
	}
	var (
		list []*ActExtend
		err  error
	)
	if activityID == "" {
		list, err = s.comp.extendsStore.ListByDefinitionKey(ctx, definitionKey)
	} else {
		list, err = s.comp.extendsStore.GetByDefinitionKeyAndActivity(ctx, definitionKey, activityID)
	}
	if err != nil {
		return R{}, err
	}
	return ROk(list), nil
}

// SaveCountersignWeights 保存节点会签权重配置（/weight/save 数据面）。
// @param ctx 上下文
// @param processDefinitionKey 流程定义 Key
// @param activityID 节点 ID
// @param tenantID 租户（空不隔离）
// @param configs 权重配置列表
// @return 统一返回
// @return 错误
func (s *Service) SaveCountersignWeights(ctx context.Context, processDefinitionKey, activityID, tenantID string, configs []CountersignWeight) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if processDefinitionKey == "" || activityID == "" {
		return R{}, api.NewError("WORKFLOW-0400", "权重保存缺少定义 Key 或节点 ID", api.ErrBadRequest)
	}
	key := CountersignWeightKey{ProcessDefinitionKey: processDefinitionKey, ActivityID: activityID, TenantID: tenantID}
	if err := s.comp.counter.SaveNodeWeights(ctx, key, configs); err != nil {
		return R{}, err
	}
	return ROkMsg("保存成功", nil), nil
}

// ListCountersignWeights 查询节点会签权重配置（/weight/getWeightByProcessDefKeyAndActId 数据面）。
// @param ctx 上下文
// @param processDefinitionKey 流程定义 Key
// @param activityID 节点 ID
// @param tenantID 租户（空不隔离）
// @return 统一返回（data 为权重配置列表）
// @return 错误
func (s *Service) ListCountersignWeights(ctx context.Context, processDefinitionKey, activityID, tenantID string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if processDefinitionKey == "" || activityID == "" {
		return R{}, api.NewError("WORKFLOW-0400", "权重查询缺少定义 Key 或节点 ID", api.ErrBadRequest)
	}
	key := CountersignWeightKey{ProcessDefinitionKey: processDefinitionKey, ActivityID: activityID, TenantID: tenantID}
	// 定义 Id（模型 weightList 降级来源解析用；按 Key 取最新版本）
	definition, err := s.engine.RepositoryService().GetProcessDefinitionByKey(ctx, processDefinitionKey)
	if err != nil {
		return R{}, err
	}
	configs, err := s.comp.counter.ListNodeWeights(ctx, definition.ID, key)
	if err != nil {
		return R{}, err
	}
	return ROk(configs), nil
}

// ClaimTask 签收任务（组任务认领到个人）。
// @param ctx 上下文
// @param taskID 任务 ID
// @param userID 签收人身份 Id
// @return 统一返回
// @return 错误
func (s *Service) ClaimTask(ctx context.Context, taskID, userID string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if taskID == "" || userID == "" {
		return R{}, api.NewError("WORKFLOW-0400", "签收缺少任务或签收人", api.ErrBadRequest)
	}
	if err := s.engine.TaskService().Claim(ctx, taskID, userID); err != nil {
		return R{}, err
	}
	return ROkMsg("签收成功", nil), nil
}

// ResolveTask 处理委派任务（处理人恢复为所有者）。
// @param ctx 上下文
// @param taskID 任务 ID
// @param userID 处理人身份 Id
// @return 统一返回
// @return 错误
func (s *Service) ResolveTask(ctx context.Context, taskID, userID string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if taskID == "" || userID == "" {
		return R{}, api.NewError("WORKFLOW-0400", "委派处理缺少任务或处理人", api.ErrBadRequest)
	}
	if err := s.engine.TaskService().Resolve(ctx, taskID, userID); err != nil {
		return R{}, err
	}
	return ROkMsg("委派处理成功", nil), nil
}

// SuspendProcessInstances 批量挂起流程实例（逐个挂起，失败即中断并返回已处理数）。
// @param ctx 上下文
// @param processInstanceIDs 实例 Id 列表
// @param operator 操作人身份 Id
// @return 统一返回（data 含 suspended 数）
// @return 错误
func (s *Service) SuspendProcessInstances(ctx context.Context, processInstanceIDs []string, operator string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	count := 0
	for _, pid := range processInstanceIDs {
		if pid == "" {
			continue
		}
		if err := s.comp.terminate.Suspend(ctx, pid, operator); err != nil {
			return R{}, err
		}
		count++
	}
	return ROkMsg("挂起成功", map[string]any{"suspended": count}), nil
}

// ActivateProcessInstances 批量恢复挂起的流程实例。
// @param ctx 上下文
// @param processInstanceIDs 实例 Id 列表
// @param operator 操作人身份 Id
// @return 统一返回（data 含 activated 数）
// @return 错误
func (s *Service) ActivateProcessInstances(ctx context.Context, processInstanceIDs []string, operator string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	count := 0
	for _, pid := range processInstanceIDs {
		if pid == "" {
			continue
		}
		if err := s.comp.terminate.Activate(ctx, pid, operator); err != nil {
			return R{}, err
		}
		count++
	}
	return ROkMsg("激活成功", map[string]any{"activated": count}), nil
}

// GetProcessInstancePageList 分页查询运行中实例（引擎全量列表 + 门面内存分页）。
// @param ctx 上下文
// @param query 分页参数
// @return 统一返回（data 为 PageResult，行为实例列表）
// @return 错误
func (s *Service) GetProcessInstancePageList(ctx context.Context, query PageQuery) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	instances, err := s.engine.RuntimeService().ListProcessInstances(ctx)
	if err != nil {
		return R{}, err
	}
	// 按 Id 升序稳定输出（引擎列表顺序依赖内部实现）
	ordered := make([]api.ProcessInstance, len(instances))
	copy(ordered, instances)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	start := query.Start
	if start < 0 || start > len(ordered) {
		start = len(ordered)
	}
	end := start + query.Size
	if query.Size <= 0 || end > len(ordered) {
		if query.Size <= 0 {
			end = len(ordered)
		} else {
			end = len(ordered)
		}
	}
	size := query.Size
	if size <= 0 {
		size = len(ordered)
	}
	page := PageResult[api.ProcessInstance]{Data: ordered[start:end], Total: int64(len(ordered)), Start: start, Size: size}
	return ROk(page), nil
}

// GetIdentityTree 查询内置身份库树（用户与组两级：组 → 组成员用户；未分组用户挂根）。
// @param ctx 上下文
// @return 统一返回（data 为树节点列表）
// @return 错误
func (s *Service) GetIdentityTree(ctx context.Context) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	users, err := s.engine.IdentityService().ListUsers(ctx)
	if err != nil {
		return R{}, err
	}
	groups, err := s.engine.IdentityService().ListGroups(ctx)
	if err != nil {
		return R{}, err
	}
	// 组节点 → 成员；成员归属经组成员查询逐组展开（内置库量级小，允许逐组查询）
	nodes := make([]map[string]any, 0, len(groups)+1)
	membered := map[string]bool{}
	for _, group := range groups {
		members, err := s.engine.IdentityService().ListMembers(ctx, group.ID)
		if err != nil {
			members = nil
		}
		children := make([]map[string]any, 0, len(members))
		for _, member := range members {
			membered[member.ID] = true
			children = append(children, map[string]any{"id": member.ID, "name": userDisplayName(member), "type": "user"})
		}
		nodes = append(nodes, map[string]any{"id": group.ID, "name": group.Name, "type": "group", "children": children})
	}
	// 未分组用户挂根
	ungrouped := make([]map[string]any, 0, len(users))
	for _, user := range users {
		if !membered[user.ID] {
			ungrouped = append(ungrouped, map[string]any{"id": user.ID, "name": userDisplayName(user), "type": "user"})
		}
	}
	if len(ungrouped) > 0 {
		nodes = append(nodes, map[string]any{"id": "ungrouped", "name": "未分组", "type": "group", "children": ungrouped})
	}
	return ROk(nodes), nil
}

// GetOperationButtons 按实例状态聚合可用操作按钮（前端 operationButtonList 数据面）。
// 运行中：终止/挂起/退回/撤回/删除；挂起：激活；已结束：重激活/删除。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @return 统一返回（data 为按钮代号列表）
// @return 错误
func (s *Service) GetOperationButtons(ctx context.Context, processInstanceID string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	instance, err := s.engine.RuntimeService().GetProcessInstance(ctx, processInstanceID)
	if err == nil && !instance.Ended {
		buttons := []string{"stop", "delete", "rollback", "back", "highlight"}
		if instance.Suspended {
			buttons = []string{"activate", "delete", "highlight"}
		}
		return ROk(buttons), nil
	}
	// 运行态查不到 → 已结束实例
	buttons := []string{"reactivate", "delete", "highlight"}
	return ROk(buttons), nil
}
