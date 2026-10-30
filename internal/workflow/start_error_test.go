// start_error_test.go 发起域服务错误路径测试（P1-D）。
// 覆盖：引擎发起失败、首任务查询失败、模型元数据失败、预生成失败、
// 意见/操作记录/待办/已办/表单写失败（全部触发补偿回滚）、组展开容错。
package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// startFailingEngine 是按需故障注入的引擎桩（嵌入空接口，仅覆写被测方法）。
type startFailingEngine struct {
	api.Engine           // 其余方法不触达
	failStart       bool // RuntimeService.StartProcessInstance 失败开关
	failListTasks   bool // TaskService.ListTasks 失败开关
	failListMembers bool // IdentityService.ListMembers 失败开关
}

// RuntimeService 按开关返回故障运行时服务。
// @return 运行时服务
func (f *startFailingEngine) RuntimeService() api.RuntimeService {
	// 故障注入：发起失败（按开关代理）
	if f.failStart {
		return &startFailingRuntimeService{}
	}
	return f.Engine.RuntimeService()
}

// TaskService 按开关返回故障任务服务。
// @return 任务服务
func (f *startFailingEngine) TaskService() api.TaskService {
	// 故障注入：任务查询失败（按开关代理）
	if f.failListTasks {
		return &startFailingTaskService{}
	}
	return f.Engine.TaskService()
}

// startFailingRuntimeService 是发起故障桩。
type startFailingRuntimeService struct {
	api.RuntimeService
}

// StartProcessInstance 返回注入错误。
// @param ctx 上下文
// @param req 启动请求
// @return 流程实例
// @return 错误
func (f *startFailingRuntimeService) StartProcessInstance(ctx context.Context, req api.StartRequest) (api.ProcessInstance, error) {
	return api.ProcessInstance{}, errors.New("注入发起失败")
}

// IdentityService 返回身份服务（按开关代理组成员查询）。
// @return 身份服务
func (f *startFailingEngine) IdentityService() api.IdentityService {
	// 故障注入：组成员查询失败
	if f.failListMembers {
		return &startFailingIdentityService{}
	}
	return f.Engine.IdentityService()
}

// startFailingTaskService 是任务查询故障桩。
type startFailingTaskService struct {
	api.TaskService
}

// ListTasks 返回注入错误。
// @param ctx 上下文
// @param filter 查询条件
// @return 分页结果
// @return 错误
func (f *startFailingTaskService) ListTasks(ctx context.Context, filter api.TaskFilter) (api.Page[api.Task], error) {
	return api.Page[api.Task]{}, errors.New("注入任务查询失败")
}

// startFailingIdentityService 是组成员查询故障桩。
type startFailingIdentityService struct {
	api.IdentityService
}

// ListMembers 返回注入错误。
// @param ctx 上下文
// @param groupID 组 ID
// @return 成员列表
// @return 错误
func (f *startFailingIdentityService) ListMembers(ctx context.Context, groupID string) ([]api.User, error) {
	return nil, errors.New("注入组成员查询失败")
}

// startFailingOperationStore 是操作记录写入故障桩。
type startFailingOperationStore struct {
	OperationRecordStore
}

// Save 返回注入错误。
// @param ctx 操作上下文
// @param r 操作记录
// @return 错误
func (f *startFailingOperationStore) Save(ctx context.Context, r *OperationRecord) error {
	return errors.New("注入操作记录写失败")
}

// startFailingHistoryStore 是已办扩展写入故障桩。
type startFailingHistoryStore struct {
	HistoryExtendStore
}

// Save 返回注入错误。
// @param ctx 操作上下文
// @param h 已办扩展
// @return 错误
func (f *startFailingHistoryStore) Save(ctx context.Context, h *HistoryExtend) error {
	return errors.New("注入已办扩展写失败")
}

// startFailingDocumentStore 是表单状态写入故障桩。
type startFailingDocumentStore struct {
	DocumentStore
}

// Save 返回注入错误。
// @param ctx 操作上下文
// @param d 表单状态
// @return 错误
func (f *startFailingDocumentStore) Save(ctx context.Context, d *Document) error {
	return errors.New("注入表单状态写失败")
}

// startFailingCommentTasks 是任务评论故障桩（意见写失败）。
type startFailingCommentTasks struct {
	TaskCommentEngine
}

// AddComment 返回注入错误。
// @param ctx 操作上下文
// @param taskID 任务 ID
// @param userID 作者
// @param message 内容
// @return 评论
// @return 错误
func (f *startFailingCommentTasks) AddComment(ctx context.Context, taskID, userID, message string) (api.Comment, error) {
	return api.Comment{}, errors.New("注入意见写失败")
}

// rebuildStartService 以可注入的引擎与存储重建发起域服务（错误路径测试用）。
// @param eng 引擎
// @param next 下一步存储
// @param ops 操作记录存储
// @param msg 待办存储
// @param hist 已办存储
// @param doc 表单存储
// @param comments 意见引擎
// @return 发起域服务
func rebuildStartService(eng api.Engine, next NextActivityStore, ops OperationRecordStore, msg MessageExtendStore, hist HistoryExtendStore, doc DocumentStore, comments TaskCommentEngine) *StartService {
	meta := NewModelMetaService(eng)
	return NewStartService(
		eng,
		meta,
		NewNextActivityService(next, nil),
		NewCommentService(comments, eng.HistoryService(), eng.IdentityService()),
		NewOperationRecordService(ops, eng.HistoryService(), &startEngineInstances{runtime: eng.RuntimeService(), history: eng.HistoryService()}, eng.RepositoryService(), eng.IdentityService(), nil),
		NewMessageExtendService(msg, nil),
		NewHistoryExtendService(hist, nil),
		NewDocumentService(doc, msg, ops, &startEngineInstances{runtime: eng.RuntimeService(), history: eng.HistoryService()}, eng.HistoryService(), nil),
	)
}

// assertStartCompensated 断言发起失败后实例被补偿删除。
// @param t 测试上下文
// @param eng 引擎
func assertStartCompensated(t *testing.T, eng api.Engine) {
	t.Helper()
	instances, listErr := eng.RuntimeService().ListProcessInstances(tenantOaCtx())
	if listErr != nil || len(instances) != 0 {
		t.Fatalf("发起失败应补偿删除实例: %v %+v", listErr, instances)
	}
}

// tenantOaCtx 返回绑定租户 oa 的上下文（引擎读操作鉴权用）。
// @return 上下文
func tenantOaCtx() context.Context {
	return tenant.With(context.Background(), "oa")
}

// TestStartEngineStartFailure 验证引擎发起失败直接透传错误。
// @param t 测试上下文
func TestStartEngineStartFailure(t *testing.T) {
	eng, _, _, hook := newStartEnv(t)
	deployStartResource(t, eng, hook, "oa", "base.bpmn20.xml", startCorpusBase)
	broken := &startFailingEngine{Engine: eng, failStart: true}
	start := rebuildStartService(broken, NewMemNextStore(nil), NewMemOperationStore(nil), NewMemMessageStore(nil), NewMemHistoryStore(nil), NewMemDocumentStore(nil), eng.TaskService())
	if _, err := start.Start(tenantOaCtx(), startVO("startProc", "u1")); err == nil {
		t.Fatalf("引擎发起失败应报错")
	}
}

// TestStartFirstTaskQueryFailure 验证首任务查询失败报错并补偿回滚。
// @param t 测试上下文
func TestStartFirstTaskQueryFailure(t *testing.T) {
	eng, _, _, hook := newStartEnv(t)
	seedStartIdentities(t, eng)
	deployStartResource(t, eng, hook, "oa", "base.bpmn20.xml", startCorpusBase)
	broken := &startFailingEngine{Engine: eng, failListTasks: true}
	start := rebuildStartService(broken, NewMemNextStore(nil), NewMemOperationStore(nil), NewMemMessageStore(nil), NewMemHistoryStore(nil), NewMemDocumentStore(nil), eng.TaskService())
	if _, err := start.Start(tenantOaCtx(), startVO("startProc", "u1")); err == nil {
		t.Fatalf("首任务查询失败应报错")
	}
	assertStartCompensated(t, eng)
}

// TestStartWriteFailureRollbacks 验证各落位写失败均报错并补偿回滚（表驱动）。
// @param t 测试上下文
func TestStartWriteFailureRollbacks(t *testing.T) {
	cases := map[string]func(eng api.Engine) *StartService{
		// 意见写失败
		"comment": func(eng api.Engine) *StartService {
			return rebuildStartService(eng, NewMemNextStore(nil), NewMemOperationStore(nil), NewMemMessageStore(nil), NewMemHistoryStore(nil), NewMemDocumentStore(nil), &startFailingCommentTasks{TaskCommentEngine: eng.TaskService()})
		},
		// 操作记录写失败
		"operation": func(eng api.Engine) *StartService {
			return rebuildStartService(eng, NewMemNextStore(nil), &startFailingOperationStore{OperationRecordStore: NewMemOperationStore(nil)}, NewMemMessageStore(nil), NewMemHistoryStore(nil), NewMemDocumentStore(nil), eng.TaskService())
		},
		// 待办写失败
		"message": func(eng api.Engine) *StartService {
			return rebuildStartService(eng, NewMemNextStore(nil), NewMemOperationStore(nil), &failingMessageStore{MessageExtendStore: NewMemMessageStore(nil), failSave: true}, NewMemHistoryStore(nil), NewMemDocumentStore(nil), eng.TaskService())
		},
		// 已办镜像写失败
		"history": func(eng api.Engine) *StartService {
			return rebuildStartService(eng, NewMemNextStore(nil), NewMemOperationStore(nil), NewMemMessageStore(nil), &startFailingHistoryStore{HistoryExtendStore: NewMemHistoryStore(nil)}, NewMemDocumentStore(nil), eng.TaskService())
		},
		// 表单状态写失败
		"document": func(eng api.Engine) *StartService {
			return rebuildStartService(eng, NewMemNextStore(nil), NewMemOperationStore(nil), NewMemMessageStore(nil), NewMemHistoryStore(nil), &startFailingDocumentStore{DocumentStore: NewMemDocumentStore(nil)}, eng.TaskService())
		},
		// 首步预生成写失败
		"nextActivity": func(eng api.Engine) *StartService {
			return rebuildStartService(eng, &failingNextStore{NextActivityStore: NewMemNextStore(nil), failInsertDual: true}, NewMemOperationStore(nil), NewMemMessageStore(nil), NewMemHistoryStore(nil), NewMemDocumentStore(nil), eng.TaskService())
		},
	}
	for name, build := range cases {
		eng, _, _, hook := newStartEnv(t)
		seedStartIdentities(t, eng)
		deployStartResource(t, eng, hook, "oa", "base.bpmn20.xml", startCorpusBase)
		start := build(eng)
		// 各写失败均应报错阻断
		if _, err := start.Start(tenantOaCtx(), startVO("startProc", "u1")); err == nil {
			t.Fatalf("场景 %s 写失败应报错", name)
		}
		// 各写失败均应触发补偿回滚（实例删除）
		assertStartCompensated(t, eng)
	}
}

// TestStartMetaFailure 验证模型元数据查询失败报错并补偿回滚。
// @param t 测试上下文
func TestStartMetaFailure(t *testing.T) {
	eng, _, _, hook := newStartEnv(t)
	seedStartIdentities(t, eng)
	deployStartResource(t, eng, hook, "oa", "base.bpmn20.xml", startCorpusBase)
	// 换用故障模型引擎构造元数据服务（GetBpmnModel 失败）
	brokenMeta := NewModelMetaService(&failingMetaEngine{repo: &failingMetaRepo{failModel: true}})
	start := NewStartService(
		eng,
		brokenMeta,
		NewNextActivityService(NewMemNextStore(nil), nil),
		NewCommentService(eng.TaskService(), eng.HistoryService(), eng.IdentityService()),
		NewOperationRecordService(NewMemOperationStore(nil), eng.HistoryService(), &startEngineInstances{runtime: eng.RuntimeService(), history: eng.HistoryService()}, eng.RepositoryService(), eng.IdentityService(), nil),
		NewMessageExtendService(NewMemMessageStore(nil), nil),
		NewHistoryExtendService(NewMemHistoryStore(nil), nil),
		NewDocumentService(NewMemDocumentStore(nil), NewMemMessageStore(nil), NewMemOperationStore(nil), &startEngineInstances{runtime: eng.RuntimeService(), history: eng.HistoryService()}, eng.HistoryService(), nil),
	)
	if _, err := start.Start(tenantOaCtx(), startVO("startProc", "u1")); err == nil {
		t.Fatalf("模型元数据失败应报错")
	}
	assertStartCompensated(t, eng)
}

// TestStartExpandGroupMembersFailure 验证组展开查询失败时跳过该组不阻断（空判兜底）。
// @param t 测试上下文
func TestStartExpandGroupMembersFailure(t *testing.T) {
	eng, _, _, hook := newStartEnv(t)
	seedStartIdentities(t, eng)
	deployStartResource(t, eng, hook, "oa", "group.bpmn20.xml", startCorpusGroups)
	// ListMembers 注入失败：组展开为空 → 执行人身份不存在报错（Java 同语义）
	broken := &startFailingEngine{Engine: eng, failListMembers: true}
	start := rebuildStartService(broken, NewMemNextStore(nil), NewMemOperationStore(nil), NewMemMessageStore(nil), NewMemHistoryStore(nil), NewMemDocumentStore(nil), eng.TaskService())
	if _, err := start.Start(tenantOaCtx(), startVO("groupProc", "u1")); err == nil {
		t.Fatalf("组展开失败应报错阻断")
	}
	// 直连验证：org 配置在组查询失败时跳过该配置项（不产生执行人）
	identities, groupIDs := start.expandExecutorConfigs(tenantOaCtx(), []ExecutorConfig{{ExecutorType: "org", OrgID: "g1"}})
	if len(identities) != 0 || len(groupIDs) != 0 {
		t.Fatalf("组查询失败应跳过配置项: %v %v", identities, groupIDs)
	}
	// org 配置缺组 Id：跳过
	identities, groupIDs = start.expandExecutorConfigs(tenantOaCtx(), []ExecutorConfig{{ExecutorType: "org"}})
	if len(identities) != 0 || len(groupIDs) != 0 {
		t.Fatalf("缺组 Id 应跳过: %v %v", identities, groupIDs)
	}
}

// TestFirstOf 验证取首元素工具的空列表分支。
// @param t 测试上下文
func TestFirstOf(t *testing.T) {
	if firstOf(nil) != "" {
		t.Fatalf("空列表应返回空串")
	}
	if firstOf([]string{"a", "b"}) != "a" {
		t.Fatalf("应返回首元素")
	}
}
