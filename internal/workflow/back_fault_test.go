package workflow

import (
	"context"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// 故障注入桩通用模式：嵌入接口并以真实实现初始化（未覆写方法透传真实实现），
// 仅覆写需要注入失败的少量方法。全部桩仅供错误分支覆盖测试使用。

// faultTaskSvc 是故障任务服务（GetTask/ListTasks/UpdateTask 可注入失败）。
type faultTaskSvc struct {
	api.TaskService
	failGet    bool // GetTask 返回普通错误（非 NotFound）
	getMissing bool // GetTask 返回 NotFound
	failList   bool // ListTasks 失败
	failUpdate bool // UpdateTask 失败
}

// GetTask 查询任务（可注入失败）。
// @param ctx 参数 ctx
// @param id 参数 id
// @return 返回值
// @return 返回值
func (s *faultTaskSvc) GetTask(ctx context.Context, id string) (api.Task, error) {
	if s.failGet {
		return api.Task{}, assertError("任务查询故障")
	}
	if s.getMissing {
		return api.Task{}, api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
	}
	return s.TaskService.GetTask(ctx, id)
}

// ListTasks 查询任务列表（可注入失败）。
// @param ctx 参数 ctx
// @param filter 参数 filter
// @return 返回值
// @return 返回值
func (s *faultTaskSvc) ListTasks(ctx context.Context, filter api.TaskFilter) (api.Page[api.Task], error) {
	if s.failList {
		return api.Page[api.Task]{}, assertError("任务列表故障")
	}
	return s.TaskService.ListTasks(ctx, filter)
}

// UpdateTask 更新任务（可注入失败）。
// @param ctx 参数 ctx
// @param taskID 参数 taskID
// @param update 参数 update
// @return 返回值
// @return 返回值
func (s *faultTaskSvc) UpdateTask(ctx context.Context, taskID string, update api.TaskUpdate) (api.Task, error) {
	if s.failUpdate {
		return api.Task{}, assertError("任务更新故障")
	}
	return s.TaskService.UpdateTask(ctx, taskID, update)
}

// faultEngine 是退回/直送故障注入引擎：嵌入真实引擎并覆写故障目标服务。
type faultEngine struct {
	api.Engine
	failGetTask    bool  // TaskService.GetTask 普通错误
	taskMissing    bool  // TaskService.GetTask NotFound
	failRepo       bool  // GetProcessDefinition 失败
	failBpmn       bool  // GetBpmnModel 失败
	emptyModel     bool  // GetBpmnModel 返回空模型
	failHistAct    bool  // QueryHistoricActivityInstances 失败
	failTaskList   bool  // ListTasks 失败
	failTaskUpd    bool  // UpdateTask 失败
	failHisTaskQry bool  // QueryHistoricTaskInstances 失败
	extErr         error // WorkflowExtService 退回/直送错误
}

// TaskService 返回故障任务服务。
// @return 任务服务
func (f *faultEngine) TaskService() api.TaskService {
	return &faultTaskSvc{TaskService: f.Engine.TaskService(), failGet: f.failGetTask, getMissing: f.taskMissing, failList: f.failTaskList, failUpdate: f.failTaskUpd}
}

// RepositoryService 返回故障仓库服务。
// @return 仓库服务
func (f *faultEngine) RepositoryService() api.RepositoryService {
	return &faultRepo{RepositoryService: f.Engine.RepositoryService(), failDefs: f.failRepo, failBpmn: f.failBpmn, emptyModel: f.emptyModel}
}

// HistoryService 返回故障历史服务。
// @return 历史服务
func (f *faultEngine) HistoryService() api.HistoryService {
	return &faultHistory{HistoryService: f.Engine.HistoryService(), failAct: f.failHistAct, failTaskQuery: f.failHisTaskQry}
}

// WorkflowExtService 返回故障扩展服务。
// @return 扩展服务
func (f *faultEngine) WorkflowExtService() api.WorkflowExtService {
	return &faultExt{WorkflowExtService: f.Engine.WorkflowExtService(), extErr: f.extErr}
}

// faultRepo 是故障仓库服务（定义/模型查询可注入失败）。
type faultRepo struct {
	api.RepositoryService
	failDefs   bool // GetProcessDefinition 失败
	failBpmn   bool // GetBpmnModel 失败
	emptyModel bool // GetBpmnModel 返回空模型
}

// GetProcessDefinition 查询定义（可注入失败）。
// @param ctx 参数 ctx
// @param id 参数 id
// @return 返回值
// @return 返回值
func (r *faultRepo) GetProcessDefinition(ctx context.Context, id string) (api.ProcessDefinition, error) {
	if r.failDefs {
		return api.ProcessDefinition{}, assertError("定义查询故障")
	}
	return r.RepositoryService.GetProcessDefinition(ctx, id)
}

// GetBpmnModel 查询模型（可注入失败/空模型）。
// @param ctx 参数 ctx
// @param definitionID 参数 definitionID
// @return 返回值
// @return 返回值
func (r *faultRepo) GetBpmnModel(ctx context.Context, definitionID string) (*model.BpmnModel, error) {
	if r.failBpmn {
		return nil, assertError("模型查询故障")
	}
	bpmn, err := r.RepositoryService.GetBpmnModel(ctx, definitionID)
	if err != nil {
		return nil, err
	}
	// 空模型注入（定义不含流程）
	if r.emptyModel {
		return &model.BpmnModel{}, nil
	}
	return bpmn, nil
}

// faultHistory 是故障历史服务（历史活动/历史任务查询可注入失败）。
type faultHistory struct {
	api.HistoryService
	failAct       bool // QueryHistoricActivityInstances 失败
	failTaskQuery bool // QueryHistoricTaskInstances 失败
}

// QueryHistoricActivityInstances 查询历史活动（可注入失败）。
// @param ctx 参数 ctx
// @param filter 参数 filter
// @return 返回值
// @return 返回值
func (h *faultHistory) QueryHistoricActivityInstances(ctx context.Context, filter api.HistoricActivityInstanceFilter) (api.Page[api.HistoricActivityInstance], error) {
	if h.failAct {
		return api.Page[api.HistoricActivityInstance]{}, assertError("历史活动查询故障")
	}
	return h.HistoryService.QueryHistoricActivityInstances(ctx, filter)
}

// QueryHistoricTaskInstances 分页查询历史任务（可注入失败，退回执行人预存用）。
// @param ctx 参数 ctx
// @param filter 参数 filter
// @return 返回值
// @return 返回值
func (h *faultHistory) QueryHistoricTaskInstances(ctx context.Context, filter api.HistoricTaskInstanceFilter) (api.Page[api.HistoricTaskInstance], error) {
	if h.failTaskQuery {
		return api.Page[api.HistoricTaskInstance]{}, assertError("历史任务查询故障")
	}
	return h.HistoryService.QueryHistoricTaskInstances(ctx, filter)
}

// faultExt 是故障扩展服务（退回/直送可注入错误）。
type faultExt struct {
	api.WorkflowExtService
	extErr error // 注入错误
}

// BackUserTaskToActivity 任意退回（可注入失败）。
// @param ctx 参数 ctx
// @param taskID 参数 taskID
// @param targetActivityID 参数 targetActivityID
// @return 返回值
// @return 返回值
func (f *faultExt) BackUserTaskToActivity(ctx context.Context, taskID, targetActivityID string) (string, error) {
	if f.extErr != nil {
		return "", f.extErr
	}
	return f.WorkflowExtService.BackUserTaskToActivity(ctx, taskID, targetActivityID)
}

// CompleteToTarget 直送（可注入失败）。
// @param ctx 参数 ctx
// @param taskID 参数 taskID
// @param targetActivityID 参数 targetActivityID
// @param variables 参数 variables
// @return 返回值
func (f *faultExt) CompleteToTarget(ctx context.Context, taskID, targetActivityID string, variables map[string]any) error {
	if f.extErr != nil {
		return f.extErr
	}
	return f.WorkflowExtService.CompleteToTarget(ctx, taskID, targetActivityID, variables)
}

// faultStepStore 是故障步骤存储（failGet 立即失败；failGetAfter 第 N 次 Get 后失败，
// 用于覆盖"首次读取成功、后续读取失败"的多阶段分支）。
type faultStepStore struct {
	StepStore
	failGet      bool // 立即失败
	failGetAfter int  // 第 N 次 Get 后开始失败（0 表示不启用）
	getCalls     int  // Get 调用计数
	failList     bool // ListByDeployment 失败
}

// GetByDeploymentAndActivity 按部署+节点定位（可注入失败）。
// @param ctx 参数 ctx
// @param deploymentID 参数 deploymentID
// @param activityID 参数 activityID
// @return 返回值
// @return 返回值
// @return 返回值
func (s *faultStepStore) GetByDeploymentAndActivity(ctx context.Context, deploymentID, activityID string) (*StepRule, bool, error) {
	s.getCalls++
	if s.failGet || (s.failGetAfter > 0 && s.getCalls > s.failGetAfter) {
		return nil, false, assertError("步骤查询故障")
	}
	return s.StepStore.GetByDeploymentAndActivity(ctx, deploymentID, activityID)
}

// ListByDeployment 查询部署步骤（可注入失败）。
// @param ctx 参数 ctx
// @param deploymentID 参数 deploymentID
// @return 返回值
// @return 返回值
func (s *faultStepStore) ListByDeployment(ctx context.Context, deploymentID string) ([]*StepRule, error) {
	if s.failList {
		return nil, assertError("步骤列表故障")
	}
	return s.StepStore.ListByDeployment(ctx, deploymentID)
}

// faultNextStore 是故障下一步存储（历史链查询/按节点删除/按任务查询/按身份删除可注入失败）。
type faultNextStore struct {
	NextActivityStore
	failHisTask    bool // ListHisByTaskID 失败
	failDelete     bool // DeleteByProcessInstanceAndActivityIDs 失败
	failListByTask bool // ListByTaskID 失败
	failResolve    bool // DeleteForResolve 失败
	failLatest     bool // LatestVersion 失败（预存版本推算）
}

// ListHisByTaskID 查询历史记录（可注入失败）。
// @param ctx 参数 ctx
// @param taskID 参数 taskID
// @return 返回值
// @return 返回值
func (s *faultNextStore) ListHisByTaskID(ctx context.Context, taskID string) ([]*NextActivity, error) {
	if s.failHisTask {
		return nil, assertError("历史记录查询故障")
	}
	return s.NextActivityStore.ListHisByTaskID(ctx, taskID)
}

// DeleteByProcessInstanceAndActivityIDs 按节点集合删除（可注入失败）。
// @param ctx 参数 ctx
// @param processInstanceID 参数 processInstanceID
// @param nextActivityIDs 参数 nextActivityIDs
// @return 返回值
func (s *faultNextStore) DeleteByProcessInstanceAndActivityIDs(ctx context.Context, processInstanceID string, nextActivityIDs []string) error {
	if s.failDelete {
		return assertError("预存删除故障")
	}
	return s.NextActivityStore.DeleteByProcessInstanceAndActivityIDs(ctx, processInstanceID, nextActivityIDs)
}

// ListByTaskID 按任务查询记录（可注入失败，直送当前预存清理用）。
// @param ctx 参数 ctx
// @param taskID 参数 taskID
// @return 返回值
// @return 返回值
func (s *faultNextStore) ListByTaskID(ctx context.Context, taskID string) ([]*NextActivity, error) {
	if s.failListByTask {
		return nil, assertError("任务记录查询故障")
	}
	return s.NextActivityStore.ListByTaskID(ctx, taskID)
}

// LatestVersion 查询最大已消耗版本（可注入失败，预存版本推算用）。
// @param ctx 参数 ctx
// @param processInstanceID 参数 processInstanceID
// @param nextActivityID 参数 nextActivityID
// @return 返回值
// @return 返回值
func (s *faultNextStore) LatestVersion(ctx context.Context, processInstanceID, nextActivityID string) (int, error) {
	if s.failLatest {
		return 0, assertError("版本查询故障")
	}
	return s.NextActivityStore.LatestVersion(ctx, processInstanceID, nextActivityID)
}

// DeleteForResolve 按身份删除记录（可注入失败，直送当前预存清理用）。
// @param ctx 参数 ctx
// @param processInstanceID 参数 processInstanceID
// @param nextActivityID 参数 nextActivityID
// @param taskID 参数 taskID
// @param identityID 参数 identityID
// @return 返回值
func (s *faultNextStore) DeleteForResolve(ctx context.Context, processInstanceID, nextActivityID, taskID, identityID string) error {
	if s.failResolve {
		return assertError("预存清理故障")
	}
	return s.NextActivityStore.DeleteForResolve(ctx, processInstanceID, nextActivityID, taskID, identityID)
}

// faultMessageStore 是故障待办存储（删除/插入可注入失败）。
type faultMessageStore struct {
	MessageExtendStore
	failDeleteActs bool // DeleteByProcessInstanceAndActivityIDs 失败
	failDeleteTask bool // DeleteByTaskID 失败
	failCreate     bool // Save 失败（待办重建分支）
	failGet        bool // GetByTaskID 失败（幂等检查分支）
}

// DeleteByProcessInstanceAndActivityIDs 按节点集合删除待办（可注入失败）。
// @param ctx 参数 ctx
// @param processInstanceID 参数 processInstanceID
// @param stepCodes 参数 stepCodes
// @return 返回值
func (s *faultMessageStore) DeleteByProcessInstanceAndActivityIDs(ctx context.Context, processInstanceID string, stepCodes []string) error {
	if s.failDeleteActs {
		return assertError("待办删除故障")
	}
	return s.MessageExtendStore.DeleteByProcessInstanceAndActivityIDs(ctx, processInstanceID, stepCodes)
}

// DeleteByTaskID 按任务删除待办（可注入失败）。
// @param ctx 参数 ctx
// @param taskID 参数 taskID
// @return 返回值
func (s *faultMessageStore) DeleteByTaskID(ctx context.Context, taskID string) error {
	if s.failDeleteTask {
		return assertError("待办删除故障")
	}
	return s.MessageExtendStore.DeleteByTaskID(ctx, taskID)
}

// GetByTaskID 按任务查询待办（可注入失败，重生成幂等检查用）。
// @param ctx 参数 ctx
// @param taskID 参数 taskID
// @return 返回值
// @return 返回值
// @return 返回值
func (s *faultMessageStore) GetByTaskID(ctx context.Context, taskID string) (*MessageExtend, bool, error) {
	if s.failGet {
		return nil, false, assertError("待办查询故障")
	}
	return s.MessageExtendStore.GetByTaskID(ctx, taskID)
}

// Save 插入待办（可注入失败，待办重建分支经服务层 Save 触发）。
// @param ctx 参数 ctx
// @param msg 参数 msg
// @return 返回值
func (s *faultMessageStore) Save(ctx context.Context, msg *MessageExtend) error {
	if s.failCreate {
		return assertError("待办创建故障")
	}
	return s.MessageExtendStore.Save(ctx, msg)
}

// faultHistoryStore 是故障已办存储（回写/插入可注入失败）。
type faultHistoryStore struct {
	HistoryExtendStore
	failFinish bool // UpdateFinishTime 失败
	failCreate bool // Save 失败（已办镜像分支）
}

// UpdateFinishTime 办结回写（可注入失败）。
// @param ctx 参数 ctx
// @param taskID 参数 taskID
// @param ownerID 参数 ownerID
// @param finishTime 参数 finishTime
// @return 返回值
func (s *faultHistoryStore) UpdateFinishTime(ctx context.Context, taskID, ownerID string, finishTime time.Time) error {
	if s.failFinish {
		return assertError("已办回写故障")
	}
	return s.HistoryExtendStore.UpdateFinishTime(ctx, taskID, ownerID, finishTime)
}

// Save 插入已办镜像（可注入失败）。
// @param ctx 参数 ctx
// @param h 参数 h
// @return 返回值
func (s *faultHistoryStore) Save(ctx context.Context, h *HistoryExtend) error {
	if s.failCreate {
		return assertError("已办创建故障")
	}
	return s.HistoryExtendStore.Save(ctx, h)
}

// faultOperationStore 是故障操作记录存储（保存可注入失败）。
type faultOperationStore struct {
	OperationRecordStore
	failSave bool // Save 失败
}

// Save 保存操作记录（可注入失败）。
// @param ctx 参数 ctx
// @param r 参数 r
// @return 返回值
func (s *faultOperationStore) Save(ctx context.Context, r *OperationRecord) error {
	if s.failSave {
		return assertError("操作记录故障")
	}
	return s.OperationRecordStore.Save(ctx, r)
}

// faultModelStore 是故障模型主数据存储（returnSort 查询可注入失败）。
type faultModelStore struct {
	ModelStore
	failQuery bool // ReturnSortOf 失败
}

// ReturnSortOf 读取退回排序方式（可注入失败）。
// @param ctx 参数 ctx
// @param processDefinitionKey 参数 processDefinitionKey
// @return 返回值
// @return 返回值
func (s *faultModelStore) ReturnSortOf(ctx context.Context, processDefinitionKey string) (bool, error) {
	if s.failQuery {
		return false, assertError("模型主数据故障")
	}
	return s.ModelStore.ReturnSortOf(ctx, processDefinitionKey)
}

// faultCompleteToStore 是故障直送记录存储（Save 可注入失败）。
type faultCompleteToStore struct {
	CompleteToActivityStore
	failSave bool // Save 失败
}

// Save 插入直送记录（可注入失败）。
// @param ctx 参数 ctx
// @param record 参数 record
// @return 返回值
func (s *faultCompleteToStore) Save(ctx context.Context, record *CompleteToActivity) error {
	if s.failSave {
		return assertError("直送记录故障")
	}
	return s.CompleteToActivityStore.Save(ctx, record)
}

// errUsers 是故障身份查询（GetUser/ListUserGroups 固定失败）。
type errUsers struct{}

// GetUser 查询用户（固定失败）。
// @param ctx 参数 ctx
// @param userID 参数 userID
// @return 返回值
// @return 返回值
func (e errUsers) GetUser(ctx context.Context, userID string) (api.User, error) {
	return api.User{}, assertError("身份查询故障")
}

// ListUserGroups 查询用户组（固定失败）。
// @param ctx 参数 ctx
// @param userID 参数 userID
// @return 返回值
// @return 返回值
func (e errUsers) ListUserGroups(ctx context.Context, userID string) ([]api.Group, error) {
	return nil, assertError("身份查询故障")
}
