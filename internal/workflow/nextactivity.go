package workflow

import (
	"context"

	"github.com/DengQingNian/Flowtide/internal/clock"
)

// NextActivityService 是"下一步预生成-消耗"记账域服务。
// 行为规格对位 Java WorkflowNextActivityServiceImpl + ProcessesUtil#saveFirstTaskUserNextActivity:1142：
// 所有写操作先写 workflow_next_activity（used=0 预生成）并双写 workflow_his_next_activity，
// 任务创建时执行人重写钩子读 used=0 记录回填处理人并置 used=1、回写 task_id。
// 版本号语义：同实例同节点每预生成一轮 = 已消耗（used=1）最大版本 + 1
// （Java getNextActivityLatestVersionByProcInstAndTaskKey 仅统计 used=1 记录）。
type NextActivityService struct {
	store NextActivityStore // 存储端口
	clock clock.Clock       // 时间源（审计列填充）
	newID func() string     // ID 生成函数
}

// NewNextActivityService 创建下一步预生成域服务。
// @param store 存储端口
// @param c 时间源，nil 时回退真实时钟
// @return 域服务
func NewNextActivityService(store NextActivityStore, c clock.Clock) *NextActivityService {
	if c == nil {
		c = clock.Real{}
	}
	return &NextActivityService{store: store, clock: c, newID: DefaultIDGen()}
}

// InsertDual 写入一条下一步记录并双写历史表。
// version 为该实例该节点当前最大已消耗版本 + 1（调用方显式传入正数版本时尊重原值）。
// @param ctx 操作上下文
// @param act 下一步记录（必填 process_instance_id/next_activity_id/identity_id）
// @param operator 操作人身份 Id（写入 creator/updater）
// @return 错误
func (s *NextActivityService) InsertDual(ctx context.Context, act *NextActivity, operator string) error {
	// 关键字段校验：三者是"预生成-消耗"模型的最小定位键
	if act == nil || act.ProcessInstanceID == "" || act.NextActivityID == "" || act.IdentityID == "" {
		return apiError("WORKFLOW-1001", "下一步记录缺少实例/节点/执行人信息")
	}
	// 版本号缺省时按已消耗最大版本 + 1 推算（Java nextVersion 语义）
	if act.Version <= 0 {
		latest, err := s.store.LatestVersion(ctx, act.ProcessInstanceID, act.NextActivityID)
		if err != nil {
			return err
		}
		act.Version = latest + 1
	}
	fillBase(&act.BaseColumns, s.newID, s.clock.Now, operator, act.TenantID)
	return s.store.InsertDual(ctx, act)
}

// SaveFirstTaskUser 首步预生成（对位 Java publishAndGetFirstTaskByKey:1142 的
// saveFirstTaskUserNextActivity：首步记录 used=1、taskId=首任务 ID、preTaskId 不设置；
// 版本号 = 已消耗最大版本 + 1）。
// @param ctx 操作上下文
// @param req 首步预生成请求
// @return 错误
func (s *NextActivityService) SaveFirstTaskUser(ctx context.Context, req FirstStepRequest) error {
	ids := cleanIdentityIDs(req.IdentityIDs)
	// 与 Java 一致：执行人为空直接拒绝（保存下一步骤信息异常：执行人为空）
	if req.ProcessInstanceID == "" || req.ActivityID == "" || len(ids) == 0 {
		return apiError("WORKFLOW-1002", "首步预生成缺少实例/节点/执行人信息")
	}
	// 版本号整批共用一次推算（Java 在循环外取 nextVersion）
	version, err := s.store.LatestVersion(ctx, req.ProcessInstanceID, req.ActivityID)
	if err != nil {
		return err
	}
	version++
	for i, id := range ids {
		act := &NextActivity{
			BaseColumns:       BaseColumns{TenantID: req.TenantID},
			NextActivityID:    req.ActivityID,
			GroupID:           pairGroupID(req.GroupIDs, i, len(ids)),
			IdentityID:        id,
			SendIdentityID:    req.SendIdentityID,
			SendUserName:      req.SendUserName,
			ProcessInstanceID: req.ProcessInstanceID,
			// Java 首步语义：used=1 且带 taskId（首个任务已创建，记录即视为已消耗）
			Used:      1,
			PreTaskID: req.PreTaskID,
			TaskID:    req.TaskID,
			Version:   version,
		}
		fillBase(&act.BaseColumns, s.newID, s.clock.Now, firstNonEmpty(req.Creator, req.SendIdentityID), req.TenantID)
		if err := s.store.InsertDual(ctx, act); err != nil {
			return err
		}
	}
	return nil
}

// SaveAgreeNext 预存"同意"方向下一步（对位 Java saveAgreeNextActivity：
// used=0、taskId 空、preTaskId=当前任务；目标为保留代号 end 时由调用方跳过不落库）。
// @param ctx 操作上下文
// @param req 预存请求
// @return 错误
func (s *NextActivityService) SaveAgreeNext(ctx context.Context, req NextActivitySaveRequest) error {
	// 与退回预存共用同一落库形态（Java 两方法持久化语义一致）
	return s.savePlainNext(ctx, req)
}

// SaveBackToStepNext 预存"退回到指定步骤"方向下一步（对位 Java saveBackToStepNextActivity：
// used=0、taskId 空、preTaskId=当前任务，与同意预存同构，独立方法保持语义可追溯）。
// @param ctx 操作上下文
// @param req 预存请求
// @return 错误
func (s *NextActivityService) SaveBackToStepNext(ctx context.Context, req NextActivitySaveRequest) error {
	return s.savePlainNext(ctx, req)
}

// savePlainNext 落库 used=0 的预生成记录（同意/退回/直送共用形态）。
// @param ctx 操作上下文
// @param req 预存请求
// @return 错误
func (s *NextActivityService) savePlainNext(ctx context.Context, req NextActivitySaveRequest) error {
	ids := cleanIdentityIDs(req.IdentityIDs)
	// 与 Java 一致：执行人为空直接拒绝
	if req.ProcessInstanceID == "" || req.ActivityID == "" || len(ids) == 0 {
		return apiError("WORKFLOW-1003", "保存下一步骤信息异常：缺少实例/节点/执行人")
	}
	// 版本号整批共用一次推算
	version, err := s.store.LatestVersion(ctx, req.ProcessInstanceID, req.ActivityID)
	if err != nil {
		return err
	}
	version++
	for i, id := range ids {
		act := &NextActivity{
			BaseColumns:       BaseColumns{TenantID: req.TenantID},
			NextActivityID:    req.ActivityID,
			GroupID:           pairGroupID(req.GroupIDs, i, len(ids)),
			IdentityID:        id,
			SendIdentityID:    req.SendIdentityID,
			SendUserName:      req.SendUserName,
			ProcessInstanceID: req.ProcessInstanceID,
			Used:              0,
			PreTaskID:         req.PreTaskID,
			Version:           version,
		}
		fillBase(&act.BaseColumns, s.newID, s.clock.Now, req.Creator, req.TenantID)
		if err := s.store.InsertDual(ctx, act); err != nil {
			return err
		}
	}
	return nil
}

// SaveResolveNext 预存"处理委派/还原"方向下一步（对位 Java saveResolveNextActivity：
// used=1、taskId=当前任务、groupId 继承既有记录；未找到既有记录时不落库，与 Java 一致）。
// @param ctx 操作上下文
// @param req 还原预存请求
// @return 错误
func (s *NextActivityService) SaveResolveNext(ctx context.Context, req ResolveNextSaveRequest) error {
	ids := cleanIdentityIDs(req.IdentityIDs)
	// 与 Java 一致：执行人为空直接拒绝
	if req.ProcessInstanceID == "" || req.ActivityID == "" || len(ids) == 0 || req.TaskID == "" {
		return apiError("WORKFLOW-1004", "保存还原下一步信息异常：缺少实例/节点/任务/执行人")
	}
	// 继承既有记录的候选组（Java findNextActivityByParams → 取 groupId）
	source, found, err := s.store.FindAssignSource(ctx, req.ProcessInstanceID, req.ActivityID, req.SendIdentityID, req.TaskID)
	if err != nil {
		return err
	}
	// Java 语义：未找到既有记录时跳过插入
	if !found {
		return nil
	}
	// 版本号整批共用一次推算
	version, err := s.store.LatestVersion(ctx, req.ProcessInstanceID, req.ActivityID)
	if err != nil {
		return err
	}
	version++
	for _, id := range ids {
		act := &NextActivity{
			BaseColumns:       BaseColumns{TenantID: req.TenantID},
			NextActivityID:    req.ActivityID,
			GroupID:           source.GroupID,
			IdentityID:        id,
			SendIdentityID:    req.SendIdentityID,
			SendUserName:      req.SendUserName,
			ProcessInstanceID: req.ProcessInstanceID,
			// Java 还原语义：used=1 且 taskId=当前任务（同任务内处理人变更，无需重写钩子消耗）
			Used:      1,
			PreTaskID: req.TaskID,
			TaskID:    req.TaskID,
			Version:   version,
		}
		fillBase(&act.BaseColumns, s.newID, s.clock.Now, req.Creator, req.TenantID)
		if err := s.store.InsertDual(ctx, act); err != nil {
			return err
		}
	}
	return nil
}

// ListUnusedByInstanceAndActivity 查询实例下指定节点未消耗（used=0）的预生成记录。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param activityID 节点 ID
// @return 记录列表
// @return 错误
func (s *NextActivityService) ListUnusedByInstanceAndActivity(ctx context.Context, processInstanceID, activityID string) ([]*NextActivity, error) {
	return s.store.ListUnusedByInstanceAndActivity(ctx, processInstanceID, activityID)
}

// ListUnusedByInstance 查询实例下全部未消耗记录（执行人重写钩子读取）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 记录列表
// @return 错误
func (s *NextActivityService) ListUnusedByInstance(ctx context.Context, processInstanceID string) ([]*NextActivity, error) {
	return s.store.ListUnusedByInstance(ctx, processInstanceID)
}

// GetFirstUnusedByActivity 按创建时间升序取实例下指定节点的第一条未消耗记录。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param activityID 节点 ID
// @return 记录
// @return 是否存在
// @return 错误
func (s *NextActivityService) GetFirstUnusedByActivity(ctx context.Context, processInstanceID, activityID string) (*NextActivity, bool, error) {
	return s.store.GetFirstUnusedByActivity(ctx, processInstanceID, activityID)
}

// MarkUsedAndWriteBack 置 used=1 并回写任务 ID（执行人重写钩子消耗入口，两套表同步）。
// @param ctx 操作上下文
// @param ids 记录 Id 集合
// @param taskID 任务 Id
// @return 错误
func (s *NextActivityService) MarkUsedAndWriteBack(ctx context.Context, ids []string, taskID string) error {
	// 空集合与空任务 ID 直接拒绝，防止无意义消耗
	if len(ids) == 0 || taskID == "" {
		return apiError("WORKFLOW-1005", "消耗下一步记录缺少记录 Id 或任务 Id")
	}
	return s.store.MarkUsedAndWriteBack(ctx, ids, taskID)
}

// DeleteByProcessInstanceAndActivityIDs 按实例与节点集合删除预生成记录（运行态+历史，退回/转办清理）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param activityIDs 节点 ID 集合
// @return 错误
func (s *NextActivityService) DeleteByProcessInstanceAndActivityIDs(ctx context.Context, processInstanceID string, activityIDs []string) error {
	return s.store.DeleteByProcessInstanceAndActivityIDs(ctx, processInstanceID, activityIDs)
}

// DeleteForBackToStep 退回清理：删除实例下目标节点（含已消耗）的全部运行态记录。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param activityID 目标节点 ID
// @return 错误
func (s *NextActivityService) DeleteForBackToStep(ctx context.Context, processInstanceID, activityID string) error {
	return s.store.DeleteForBackToStep(ctx, processInstanceID, activityID)
}

// DeleteForRollback 撤回清理：删除实例下以 preTaskID 为来源、目标节点之外的记录（仅运行态）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param preTaskID 来源任务 Id
// @param keepActivityID 保留的目标节点 ID
// @return 错误
func (s *NextActivityService) DeleteForRollback(ctx context.Context, processInstanceID, preTaskID, keepActivityID string) error {
	return s.store.DeleteForRollback(ctx, processInstanceID, preTaskID, keepActivityID)
}

// DeleteForResolve 处理委派清理：删除实例下指定节点+任务+执行人的记录。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param activityID 节点 ID
// @param taskID 任务 Id
// @param identityID 执行人 Id
// @return 错误
func (s *NextActivityService) DeleteForResolve(ctx context.Context, processInstanceID, activityID, taskID, identityID string) error {
	return s.store.DeleteForResolve(ctx, processInstanceID, activityID, taskID, identityID)
}

// DeleteByProcessInstance 删除实例的全部运行态记录（删除实例/重置流程时调用）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 错误
func (s *NextActivityService) DeleteByProcessInstance(ctx context.Context, processInstanceID string) error {
	return s.store.DeleteByProcessInstance(ctx, processInstanceID)
}

// ListByPreTask 查询以上一任务为来源的预生成记录（退回/撤回清理入口）。
// @param ctx 操作上下文
// @param preTaskID 上一任务 Id
// @return 记录列表
// @return 错误
func (s *NextActivityService) ListByPreTask(ctx context.Context, preTaskID string) ([]*NextActivity, error) {
	return s.store.ListByPreTask(ctx, preTaskID)
}

// ListByTaskID 查询已挂到任务上的预生成记录。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 记录列表
// @return 错误
func (s *NextActivityService) ListByTaskID(ctx context.Context, taskID string) ([]*NextActivity, error) {
	return s.store.ListByTaskID(ctx, taskID)
}

// LatestVersion 返回实例下指定节点当前最大已消耗版本号（无记录为 0）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param taskKey 节点 ID
// @return 最大版本号
// @return 错误
func (s *NextActivityService) LatestVersion(ctx context.Context, processInstanceID, taskKey string) (int, error) {
	return s.store.LatestVersion(ctx, processInstanceID, taskKey)
}

// ListHisByInstance 查询实例全部历史记录（轨迹回放，按版本升序）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 记录列表
// @return 错误
func (s *NextActivityService) ListHisByInstance(ctx context.Context, processInstanceID string) ([]*NextActivity, error) {
	return s.store.ListHisByInstance(ctx, processInstanceID)
}

// ListHisByTaskID 查询历史表中任务相关且非自环的记录（撤回链路回溯）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 记录列表
// @return 错误
func (s *NextActivityService) ListHisByTaskID(ctx context.Context, taskID string) ([]*NextActivity, error) {
	return s.store.ListHisByTaskID(ctx, taskID)
}
