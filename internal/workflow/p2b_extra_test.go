// p2b_extra_test.go 撤回/终止/重置域服务的错误传播与分支补齐测试（P2 波2-B）。
// 通过错误注入存储桩覆盖收尾链各错误分支；通过白盒直调覆盖防御分支；
// 通过补充语料覆盖首步执行人解析的全部优先级分支（executorList/候选用户/候选组/
// initiator 兜底）。文件内桩类型与语料 Key 均以 p2b 前缀命名，避免与其他测试文件冲突。
package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// ─── 错误注入存储桩（按开关返回错误，其余方法透传内嵌端口） ───

// p2bFailingNextStore 是带错误注入的下一步存储桩。
type p2bFailingNextStore struct {
	NextActivityStore
	failDeleteByInstance  bool // DeleteByProcessInstance 注入错误
	failListHis           bool // ListHisByInstance 注入错误
	failDeleteByActs      bool // DeleteByProcessInstanceAndActivityIDs 注入错误
	failDeleteForRollback bool // DeleteForRollback 注入错误
	failInsertDual        bool // InsertDual 注入错误
}

// DeleteByProcessInstance 按开关返回错误。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 错误
func (f *p2bFailingNextStore) DeleteByProcessInstance(ctx context.Context, processInstanceID string) error {
	// 错误注入点
	if f.failDeleteByInstance {
		return errors.New("注入错误")
	}
	return f.NextActivityStore.DeleteByProcessInstance(ctx, processInstanceID)
}

// ListHisByInstance 按开关返回错误。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 记录列表
// @return 错误
func (f *p2bFailingNextStore) ListHisByInstance(ctx context.Context, processInstanceID string) ([]*NextActivity, error) {
	// 错误注入点
	if f.failListHis {
		return nil, errors.New("注入错误")
	}
	return f.NextActivityStore.ListHisByInstance(ctx, processInstanceID)
}

// DeleteByProcessInstanceAndActivityIDs 按开关返回错误。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param nextActivityIDs 节点 Id 集合
// @return 错误
func (f *p2bFailingNextStore) DeleteByProcessInstanceAndActivityIDs(ctx context.Context, processInstanceID string, nextActivityIDs []string) error {
	// 错误注入点
	if f.failDeleteByActs {
		return errors.New("注入错误")
	}
	return f.NextActivityStore.DeleteByProcessInstanceAndActivityIDs(ctx, processInstanceID, nextActivityIDs)
}

// DeleteForRollback 按开关返回错误。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param preTaskID 来源任务 Id
// @param keepActivityID 保留节点 Id
// @return 错误
func (f *p2bFailingNextStore) DeleteForRollback(ctx context.Context, processInstanceID, preTaskID, keepActivityID string) error {
	// 错误注入点
	if f.failDeleteForRollback {
		return errors.New("注入错误")
	}
	return f.NextActivityStore.DeleteForRollback(ctx, processInstanceID, preTaskID, keepActivityID)
}

// InsertDual 按开关返回错误。
// @param ctx 操作上下文
// @param act 下一步记录
// @return 错误
func (f *p2bFailingNextStore) InsertDual(ctx context.Context, act *NextActivity) error {
	// 错误注入点
	if f.failInsertDual {
		return errors.New("注入错误")
	}
	return f.NextActivityStore.InsertDual(ctx, act)
}

// p2bFailingMessageStore 是带错误注入的待办扩展存储桩。
type p2bFailingMessageStore struct {
	MessageExtendStore
	failDeleteTask     bool // DeleteByTaskID 注入错误
	failDeleteInstance bool // DeleteByInstance 注入错误
	failDeleteBusiness bool // DeleteByBusinessID 注入错误
	failGetTask        bool // GetByTaskID 注入错误
	failSave           bool // Save 注入错误
}

// Save 按开关返回错误。
// @param ctx 操作上下文
// @param msg 待办扩展
// @return 错误
func (f *p2bFailingMessageStore) Save(ctx context.Context, msg *MessageExtend) error {
	// 错误注入点
	if f.failSave {
		return errors.New("注入错误")
	}
	return f.MessageExtendStore.Save(ctx, msg)
}

// DeleteByTaskID 按开关返回错误。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 错误
func (f *p2bFailingMessageStore) DeleteByTaskID(ctx context.Context, taskID string) error {
	// 错误注入点
	if f.failDeleteTask {
		return errors.New("注入错误")
	}
	return f.MessageExtendStore.DeleteByTaskID(ctx, taskID)
}

// DeleteByInstance 按开关返回错误。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 错误
func (f *p2bFailingMessageStore) DeleteByInstance(ctx context.Context, processInstanceID string) error {
	// 错误注入点
	if f.failDeleteInstance {
		return errors.New("注入错误")
	}
	return f.MessageExtendStore.DeleteByInstance(ctx, processInstanceID)
}

// DeleteByBusinessID 按开关返回错误。
// @param ctx 操作上下文
// @param businessID 业务单据 Id
// @param tenantID 租户
// @return 错误
func (f *p2bFailingMessageStore) DeleteByBusinessID(ctx context.Context, businessID, tenantID string) error {
	// 错误注入点
	if f.failDeleteBusiness {
		return errors.New("注入错误")
	}
	return f.MessageExtendStore.DeleteByBusinessID(ctx, businessID, tenantID)
}

// GetByTaskID 按开关返回错误。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 待办扩展
// @return 是否存在
// @return 错误
func (f *p2bFailingMessageStore) GetByTaskID(ctx context.Context, taskID string) (*MessageExtend, bool, error) {
	// 错误注入点
	if f.failGetTask {
		return nil, false, errors.New("注入错误")
	}
	return f.MessageExtendStore.GetByTaskID(ctx, taskID)
}

// p2bFailingHistoryStore 是带错误注入的已办扩展存储桩。
type p2bFailingHistoryStore struct {
	HistoryExtendStore
	failDeleteInstance bool // DeleteByInstance 注入错误
	failSave           bool // Save 注入错误
	failDeleteBusiness bool // DeleteByBusinessID 注入错误
}

// Save 按开关返回错误。
// @param ctx 操作上下文
// @param h 已办扩展
// @return 错误
func (f *p2bFailingHistoryStore) Save(ctx context.Context, h *HistoryExtend) error {
	// 错误注入点
	if f.failSave {
		return errors.New("注入错误")
	}
	return f.HistoryExtendStore.Save(ctx, h)
}

// DeleteByBusinessID 按开关返回错误。
// @param ctx 操作上下文
// @param businessID 业务单据 Id
// @param tenantID 租户
// @return 错误
func (f *p2bFailingHistoryStore) DeleteByBusinessID(ctx context.Context, businessID, tenantID string) error {
	// 错误注入点
	if f.failDeleteBusiness {
		return errors.New("注入错误")
	}
	return f.HistoryExtendStore.DeleteByBusinessID(ctx, businessID, tenantID)
}

// DeleteByInstance 按开关返回错误。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 错误
func (f *p2bFailingHistoryStore) DeleteByInstance(ctx context.Context, processInstanceID string) error {
	// 错误注入点
	if f.failDeleteInstance {
		return errors.New("注入错误")
	}
	return f.HistoryExtendStore.DeleteByInstance(ctx, processInstanceID)
}

// p2bFailingOperationStore 是带错误注入的操作记录存储桩。
type p2bFailingOperationStore struct {
	OperationRecordStore
	failSave bool // Save 注入错误
}

// Save 按开关返回错误。
// @param ctx 操作上下文
// @param r 操作记录
// @return 错误
func (f *p2bFailingOperationStore) Save(ctx context.Context, r *OperationRecord) error {
	// 错误注入点
	if f.failSave {
		return errors.New("注入错误")
	}
	return f.OperationRecordStore.Save(ctx, r)
}

// p2bFailingDocumentStore 是带错误注入的表单状态存储桩。
type p2bFailingDocumentStore struct {
	DocumentStore
	failUpdateInstance bool // UpdateByInstance 注入错误
	failMarkDeleted    bool // MarkDeletedByBusinessID 注入错误
	failSave           bool // Save 注入错误
}

// UpdateByInstance 按开关返回错误。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param changes 变更列
// @return 错误
func (f *p2bFailingDocumentStore) UpdateByInstance(ctx context.Context, processInstanceID string, changes map[string]any) error {
	// 错误注入点
	if f.failUpdateInstance {
		return errors.New("注入错误")
	}
	return f.DocumentStore.UpdateByInstance(ctx, processInstanceID, changes)
}

// MarkDeletedByBusinessID 按开关返回错误。
// @param ctx 操作上下文
// @param businessID 业务单据 Id
// @param tenantID 租户
// @return 错误
func (f *p2bFailingDocumentStore) MarkDeletedByBusinessID(ctx context.Context, businessID, tenantID string) error {
	// 错误注入点
	if f.failMarkDeleted {
		return errors.New("注入错误")
	}
	return f.DocumentStore.MarkDeletedByBusinessID(ctx, businessID, tenantID)
}

// Save 按开关返回错误。
// @param ctx 操作上下文
// @param d 单据状态
// @return 错误
func (f *p2bFailingDocumentStore) Save(ctx context.Context, d *Document) error {
	// 错误注入点
	if f.failSave {
		return errors.New("注入错误")
	}
	return f.DocumentStore.Save(ctx, d)
}

// p2bFailingCommentTasks 是带错误注入的意见引擎桩（TaskCommentEngine）。
type p2bFailingCommentTasks struct {
	failAddProcess bool // AddProcessComment 注入错误
}

// AddComment 透传（不做任务级评论）。
// @param ctx 操作上下文
// @param taskID 任务 ID
// @param userID 作者
// @param message 内容
// @return 评论
// @return 错误
func (f *p2bFailingCommentTasks) AddComment(ctx context.Context, taskID, userID, message string) (api.Comment, error) {
	return api.Comment{}, errors.New("注入错误")
}

// AddProcessComment 按开关返回错误。
// @param ctx 操作上下文
// @param processInstanceID 实例 ID
// @param userID 作者
// @param message 内容
// @return 评论
// @return 错误
func (f *p2bFailingCommentTasks) AddProcessComment(ctx context.Context, processInstanceID, userID, message string) (api.Comment, error) {
	// 错误注入点
	if f.failAddProcess {
		return api.Comment{}, errors.New("注入错误")
	}
	return api.Comment{}, nil
}

// ListInstanceComments 返回空评论列表。
// @param ctx 操作上下文
// @param processInstanceID 实例 ID
// @return 评论列表
// @return 错误
func (f *p2bFailingCommentTasks) ListInstanceComments(ctx context.Context, processInstanceID string) ([]api.Comment, error) {
	return nil, nil
}

// ─── 补充语料：首步执行人解析各优先级分支 ───

// p2bExecListProcess 语料：EL 节点 executorList（person alice + org grp1）→ B → end。
const p2bExecListProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="p2bExecList" name="执行人列表流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="EL"/>
    <userTask id="EL" name="执行人列表节点" flowable:assignee="alice"
              executorList='[{"identityId":"alice","executorType":"person"},{"orgId":"grp1","executorType":"org"}]'/>
    <sequenceFlow id="f1" sourceRef="EL" targetRef="B"/>
    <userTask id="B" name="节点乙" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f2" sourceRef="B" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// p2bCandUsersProcess 语料：CU 节点 candidateUsers 多人 → B → end。
const p2bCandUsersProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="p2bCandUsers" name="候选用户流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="CU"/>
    <userTask id="CU" name="候选用户节点" flowable:candidateUsers="alice,bob"/>
    <sequenceFlow id="f1" sourceRef="CU" targetRef="B"/>
    <userTask id="B" name="节点乙" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f2" sourceRef="CU" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// p2bCandGroupsProcess 语料：CG 节点 candidateGroups（grp1 有成员 bob）→ B → end。
const p2bCandGroupsProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="p2bCandGroups" name="候选组流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="CG"/>
    <userTask id="CG" name="候选组节点" flowable:candidateGroups="grp1"/>
    <sequenceFlow id="f1" sourceRef="CG" targetRef="B"/>
    <userTask id="B" name="节点乙" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f2" sourceRef="CG" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// p2bNoInitiatorProcess 语料：NI 节点 assignee=${initiator} 但发起时不注入该变量
// （任务办理人解析为空，重置后经实例发起人兜底重建）→ B → end。
const p2bNoInitiatorProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="p2bNoInitiator" name="发起人兜底流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="NI"/>
    <userTask id="NI" name="发起人节点" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f1" sourceRef="NI" targetRef="B"/>
    <userTask id="B" name="节点乙" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f2" sourceRef="NI" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// p2bDeployExtra 向重置测试引擎部署补充语料并创建组数据。
// @param t 测试上下文
// @param f 重置测试环境
func p2bDeployExtra(t *testing.T, f *rstFixture) {
	t.Helper()
	if _, err := f.engine.RepositoryService().Deploy(context.Background(), api.DeploymentRequest{
		Name:     "p2b-extra",
		TenantID: "dev",
		Resources: map[string][]byte{
			"p2bexeclist.bpmn20.xml":   []byte(p2bExecListProcess),
			"p2bcandusers.bpmn20.xml":  []byte(p2bCandUsersProcess),
			"p2bcandgroups.bpmn20.xml": []byte(p2bCandGroupsProcess),
			"p2bnoinit.bpmn20.xml":     []byte(p2bNoInitiatorProcess),
		},
	}); err != nil {
		t.Fatalf("部署补充语料失败: %v", err)
	}
	for _, user := range []api.User{{ID: "alice", DisplayName: "张三"}, {ID: "bob", DisplayName: "李四"}} {
		if _, err := f.engine.IdentityService().CreateUser(context.Background(), user); err != nil {
			t.Fatalf("创建用户失败: %v", err)
		}
	}
	if _, err := f.engine.IdentityService().CreateGroup(context.Background(), api.Group{ID: "grp1", Name: "审批组"}); err != nil {
		t.Fatalf("创建组失败: %v", err)
	}
	if err := f.engine.IdentityService().AddMembership(context.Background(), "bob", "grp1"); err != nil {
		t.Fatalf("添加组成员失败: %v", err)
	}
}

// p2bRebuiltIdentities 收集实例 A 节点重建预存记录的执行人集合。
// @param t 测试上下文
// @param f 重置测试环境
// @param instanceID 实例 Id
// @return 执行人 Id 列表
func p2bRebuiltIdentities(t *testing.T, f *rstFixture, instanceID string) []string {
	t.Helper()
	used, err := f.stores.Next.ListUsedByInstance(context.Background(), instanceID)
	if err != nil {
		t.Fatalf("查询已消耗预存失败: %v", err)
	}
	ids := make([]string, 0, len(used))
	for _, record := range used {
		if record.NextActivityID == "A" || record.NextActivityID == "EL" || record.NextActivityID == "CU" || record.NextActivityID == "CG" || record.NextActivityID == "NI" {
			ids = append(ids, record.IdentityID)
		}
	}
	return ids
}

// TestResetRebuildExecutorVariants 验证首步执行人重建的全部分析分支：
// executorList（person+org 组展开）→ candidateUsers → candidateGroups → initiator 兜底。
// @param t 测试上下文
func TestResetRebuildExecutorVariants(t *testing.T) {
	fx := newResetFixture(t)
	p2bDeployExtra(t, fx)
	ctx := context.Background()
	// executorList：person 直取 + org 组展开成员（跳过其余优先级）
	instanceEL, elTask := rstStart(t, fx, "p2bExecList", "biz-execlist")
	// 办结 EL 后停在 B，重置回首 EL
	rstComplete(t, fx, elTask.ID, "alice")
	if err := fx.reset.Reset(ctx, instanceEL.ID, "carl"); err != nil {
		t.Fatalf("executorList 重置失败: %v", err)
	}
	ids := p2bRebuiltIdentities(t, fx, instanceEL.ID)
	if len(ids) != 2 {
		t.Fatalf("executorList 应重建 2 条执行人记录: %v", ids)
	}
	// candidateUsers：多人逐项解析
	instanceCU, cuTask := rstStart(t, fx, "p2bCandUsers", "biz-candusers")
	rstComplete(t, fx, cuTask.ID, "alice")
	if err := fx.reset.Reset(ctx, instanceCU.ID, "carl"); err != nil {
		t.Fatalf("candidateUsers 重置失败: %v", err)
	}
	ids = p2bRebuiltIdentities(t, fx, instanceCU.ID)
	if len(ids) != 2 {
		t.Fatalf("candidateUsers 应重建 2 条执行人记录: %v", ids)
	}
	// candidateGroups：组展开成员（grp1 → bob）
	instanceCG, cgTask := rstStart(t, fx, "p2bCandGroups", "biz-candgroups")
	rstComplete(t, fx, cgTask.ID, "alice")
	if err := fx.reset.Reset(ctx, instanceCG.ID, "carl"); err != nil {
		t.Fatalf("candidateGroups 重置失败: %v", err)
	}
	ids = p2bRebuiltIdentities(t, fx, instanceCG.ID)
	if len(ids) != 1 || ids[0] != "bob" {
		t.Fatalf("candidateGroups 应重建 bob 执行人记录: %v", ids)
	}
	// initiator 兜底：发起时不注入 initiator 变量（rstStart 定制：无变量发起），
	// 任务办理人为空（${initiator} 变量缺失解析为空串），重置后经实例发起人解析
	instanceNI, err := fx.engine.RuntimeService().StartProcessInstance(ctx, api.StartRequest{
		ProcessDefinitionKey: "p2bNoInitiator",
		BusinessKey:          "biz-noinit",
		Name:                 "标题-noinit",
		TenantID:             "dev",
		UserID:               "alice",
	})
	if err != nil {
		t.Fatalf("无变量发起失败: %v", err)
	}
	niTask := rstFirstTask(t, fx, instanceNI.ID)
	rstComplete(t, fx, niTask.ID, "alice")
	// 操作人用不存在的身份（ghost）：覆盖用户展示名查询失败分支
	if err := fx.reset.Reset(ctx, instanceNI.ID, "ghost"); err != nil {
		t.Fatalf("initiator 兜底重置失败: %v", err)
	}
	ids = p2bRebuiltIdentities(t, fx, instanceNI.ID)
	if len(ids) != 1 || ids[0] != "alice" {
		t.Fatalf("initiator 兜底应重建 alice 执行人记录: %v", ids)
	}
}

// TestResetTailChainErrors 验证重置收尾链各错误分支的传播（引擎重置已生效，
// 收尾失败整体报错返回）。
// @param t 测试上下文
func TestResetTailChainErrors(t *testing.T) {
	fx := newResetFixture(t)
	ctx := context.Background()
	// 各错误变体：[名称, 构造重置服务]
	variants := []struct {
		name  string
		build func() *ResetService
	}{
		{"意见写入失败", func() *ResetService {
			comments := NewCommentService(&p2bFailingCommentTasks{failAddProcess: true}, fx.engine.HistoryService(), fx.engine.IdentityService())
			return NewResetService(fx.engine, fx.meta, fx.next, comments, fx.operations, fx.messages, fx.histories, fx.documents, fx.fakeClock)
		}},
		{"操作记录失败", func() *ResetService {
			ops := NewOperationRecordService(&p2bFailingOperationStore{failSave: true, OperationRecordStore: fx.stores.Operation}, fx.engine.HistoryService(), engineInstanceQuery{runtime: fx.engine.RuntimeService(), history: fx.engine.HistoryService()}, fx.engine.RepositoryService(), fx.engine.IdentityService(), fx.fakeClock)
			return NewResetService(fx.engine, fx.meta, fx.next, fx.comments, ops, fx.messages, fx.histories, fx.documents, fx.fakeClock)
		}},
		{"运行态预存删除失败", func() *ResetService {
			next := NewNextActivityService(&p2bFailingNextStore{failDeleteByInstance: true, NextActivityStore: fx.stores.Next}, fx.fakeClock)
			return NewResetService(fx.engine, fx.meta, next, fx.comments, fx.operations, fx.messages, fx.histories, fx.documents, fx.fakeClock)
		}},
		{"历史预存查询失败", func() *ResetService {
			next := NewNextActivityService(&p2bFailingNextStore{failListHis: true, NextActivityStore: fx.stores.Next}, fx.fakeClock)
			return NewResetService(fx.engine, fx.meta, next, fx.comments, fx.operations, fx.messages, fx.histories, fx.documents, fx.fakeClock)
		}},
		{"待办删除失败", func() *ResetService {
			messages := NewMessageExtendService(&p2bFailingMessageStore{failDeleteInstance: true, MessageExtendStore: fx.stores.Message}, fx.fakeClock)
			return NewResetService(fx.engine, fx.meta, fx.next, fx.comments, fx.operations, messages, fx.histories, fx.documents, fx.fakeClock)
		}},
		{"已办删除失败", func() *ResetService {
			histories := NewHistoryExtendService(&p2bFailingHistoryStore{failDeleteInstance: true, HistoryExtendStore: fx.stores.History}, fx.fakeClock)
			return NewResetService(fx.engine, fx.meta, fx.next, fx.comments, fx.operations, fx.messages, histories, fx.documents, fx.fakeClock)
		}},
		{"首步预生成失败", func() *ResetService {
			next := NewNextActivityService(&p2bFailingNextStore{failInsertDual: true, NextActivityStore: fx.stores.Next}, fx.fakeClock)
			return NewResetService(fx.engine, fx.meta, next, fx.comments, fx.operations, fx.messages, fx.histories, fx.documents, fx.fakeClock)
		}},
		{"待办镜像查询失败", func() *ResetService {
			messages := NewMessageExtendService(&p2bFailingMessageStore{failGetTask: true, MessageExtendStore: fx.stores.Message}, fx.fakeClock)
			return NewResetService(fx.engine, fx.meta, fx.next, fx.comments, fx.operations, messages, fx.histories, fx.documents, fx.fakeClock)
		}},
		{"表单刷新失败", func() *ResetService {
			documents := NewDocumentService(&p2bFailingDocumentStore{failUpdateInstance: true, DocumentStore: fx.stores.Document}, fx.stores.Message, fx.stores.Operation, engineInstanceQuery{runtime: fx.engine.RuntimeService(), history: fx.engine.HistoryService()}, fx.engine.HistoryService(), fx.fakeClock)
			return NewResetService(fx.engine, fx.meta, fx.next, fx.comments, fx.operations, fx.messages, fx.histories, documents, fx.fakeClock)
		}},
	}
	for _, variant := range variants {
		// 每个变体独立实例：引擎重置在收尾失败前已生效
		instance, taskA := rstStart(t, fx, "rstFlow", "biz-err-"+variant.name)
		rstSeedInstanceData(t, fx, instance, taskA)
		rstComplete(t, fx, taskA.ID, "alice")
		svc := variant.build()
		if err := svc.Reset(ctx, instance.ID, "carl"); err == nil {
			t.Fatalf("%s 应传播错误", variant.name)
		}
	}
	// nil 时钟回退真实时钟（构造分支覆盖）
	svc := NewResetService(fx.engine, fx.meta, fx.next, fx.comments, fx.operations, fx.messages, fx.histories, fx.documents, nil)
	if svc == nil {
		t.Fatalf("nil 时钟构造应回退真实时钟")
	}
}

// TestRollbackTailChainErrors 验证撤回收尾链各错误分支的传播与遗留清理白盒分支。
// @param t 测试上下文
func TestRollbackTailChainErrors(t *testing.T) {
	fx := newRollbackFixture(t)
	ctx := context.Background()
	// 各错误变体：[名称, 构造撤回服务]
	variants := []struct {
		name  string
		build func() *RollbackService
	}{
		{"意见写入失败", func() *RollbackService {
			comments := NewCommentService(&p2bFailingCommentTasks{failAddProcess: true}, fx.engine.HistoryService(), fx.engine.IdentityService())
			return NewRollbackService(fx.engine, fx.next, comments, fx.operations, fx.messages, fx.histories, fx.documents, fx.fakeClock)
		}},
		{"操作记录失败", func() *RollbackService {
			ops := NewOperationRecordService(&p2bFailingOperationStore{failSave: true, OperationRecordStore: fx.stores.Operation}, fx.engine.HistoryService(), engineInstanceQuery{runtime: fx.engine.RuntimeService(), history: fx.engine.HistoryService()}, fx.engine.RepositoryService(), fx.engine.IdentityService(), fx.fakeClock)
			return NewRollbackService(fx.engine, fx.next, fx.comments, ops, fx.messages, fx.histories, fx.documents, fx.fakeClock)
		}},
		{"撤回预存删除失败", func() *RollbackService {
			next := NewNextActivityService(&p2bFailingNextStore{failDeleteForRollback: true, NextActivityStore: fx.stores.Next}, fx.fakeClock)
			return NewRollbackService(fx.engine, next, fx.comments, fx.operations, fx.messages, fx.histories, fx.documents, fx.fakeClock)
		}},
		{"待办遗留删除失败", func() *RollbackService {
			messages := NewMessageExtendService(&p2bFailingMessageStore{failDeleteTask: true, MessageExtendStore: fx.stores.Message}, fx.fakeClock)
			return NewRollbackService(fx.engine, fx.next, fx.comments, fx.operations, messages, fx.histories, fx.documents, fx.fakeClock)
		}},
		{"表单刷新失败", func() *RollbackService {
			documents := NewDocumentService(&p2bFailingDocumentStore{failUpdateInstance: true, DocumentStore: fx.stores.Document}, fx.stores.Message, fx.stores.Operation, engineInstanceQuery{runtime: fx.engine.RuntimeService(), history: fx.engine.HistoryService()}, fx.engine.HistoryService(), fx.fakeClock)
			return NewRollbackService(fx.engine, fx.next, fx.comments, fx.operations, fx.messages, fx.histories, documents, fx.fakeClock)
		}},
	}
	for _, variant := range variants {
		// 每个变体独立实例：引擎撤回在收尾失败前已生效
		instance, taskA := rbtStart(t, fx, "biz-err", "alice")
		rbtSeedTaskData(t, fx, instance, taskA)
		rbtComplete(t, fx, taskA.ID, "alice")
		svc := variant.build()
		if err := svc.Rollback(ctx, instance.ID, taskA.ID, "carol"); err == nil {
			t.Fatalf("%s 应传播错误", variant.name)
		}
	}
	// nil 时钟构造分支
	svc := NewRollbackService(fx.engine, fx.next, fx.comments, fx.operations, fx.messages, fx.histories, fx.documents, nil)
	if svc == nil {
		t.Fatalf("nil 时钟构造应回退真实时钟")
	}
	// 白盒：cleanupStaleTodos 对仍在运行的任务跳过（continue 分支）
	instance, taskA := rbtStart(t, fx, "biz-live", "alice")
	rbtComplete(t, fx, taskA.ID, "alice")
	liveTask := rbtFirstTask(t, fx, instance.ID)
	rbtSeedTaskData(t, fx, instance, liveTask)
	hisTask := api.HistoricTaskInstance{ID: taskA.ID, ProcessInstanceID: instance.ID, TaskDefinitionKey: "A"}
	if err := svc.cleanupStaleTodos(ctx, hisTask, []string{liveTask.ID}); err != nil {
		t.Fatalf("运行中任务待办应跳过不删: %v", err)
	}
	if _, found, _ := fx.messages.GetByTaskID(ctx, liveTask.ID); !found {
		t.Fatalf("运行中任务的待办不应被清理")
	}
	// 被撤回任务自身待办兜底清理（快照中不存在的已办结任务）
	if err := svc.cleanupStaleTodos(ctx, hisTask, nil); err != nil {
		t.Fatalf("兜底清理失败: %v", err)
	}
}

// TestTerminateTailChainErrors 验证终止/删除/挂起激活/重激活收尾链各错误分支的传播。
// @param t 测试上下文
func TestTerminateTailChainErrors(t *testing.T) {
	fx := newTerminateFixture(t)
	ctx := context.Background()
	instances := engineInstanceQuery{runtime: fx.engine.RuntimeService(), history: fx.engine.HistoryService()}
	// 各错误变体：[名称, 动作]
	type action func(svc *TerminateService, instance api.ProcessInstance, task api.Task) error
	variants := []struct {
		name  string
		build func() *TerminateService
		run   action
	}{
		{"终止意见失败", func() *TerminateService {
			comments := NewCommentService(&p2bFailingCommentTasks{failAddProcess: true}, fx.engine.HistoryService(), fx.engine.IdentityService())
			return NewTerminateService(fx.engine, fx.next, comments, fx.operations, fx.messages, fx.histories, fx.documents, fx.fakeClock)
		}, func(svc *TerminateService, instance api.ProcessInstance, _ api.Task) error {
			return svc.StopByInstance(ctx, instance.ID, "carl", "")
		}},
		{"终止操作记录失败", func() *TerminateService {
			ops := NewOperationRecordService(&p2bFailingOperationStore{failSave: true, OperationRecordStore: fx.stores.Operation}, fx.engine.HistoryService(), instances, fx.engine.RepositoryService(), fx.engine.IdentityService(), fx.fakeClock)
			return NewTerminateService(fx.engine, fx.next, fx.comments, ops, fx.messages, fx.histories, fx.documents, fx.fakeClock)
		}, func(svc *TerminateService, instance api.ProcessInstance, _ api.Task) error {
			return svc.StopByInstance(ctx, instance.ID, "carl", "")
		}},
		{"终止预存删除失败", func() *TerminateService {
			next := NewNextActivityService(&p2bFailingNextStore{failDeleteByInstance: true, NextActivityStore: fx.stores.Next}, fx.fakeClock)
			return NewTerminateService(fx.engine, next, fx.comments, fx.operations, fx.messages, fx.histories, fx.documents, fx.fakeClock)
		}, func(svc *TerminateService, instance api.ProcessInstance, _ api.Task) error {
			return svc.StopByInstance(ctx, instance.ID, "carl", "")
		}},
		{"终止待办删除失败", func() *TerminateService {
			messages := NewMessageExtendService(&p2bFailingMessageStore{failDeleteInstance: true, MessageExtendStore: fx.stores.Message}, fx.fakeClock)
			return NewTerminateService(fx.engine, fx.next, fx.comments, fx.operations, messages, fx.histories, fx.documents, fx.fakeClock)
		}, func(svc *TerminateService, instance api.ProcessInstance, _ api.Task) error {
			return svc.StopByInstance(ctx, instance.ID, "carl", "")
		}},
		{"终止已办删除失败", func() *TerminateService {
			histories := NewHistoryExtendService(&p2bFailingHistoryStore{failDeleteInstance: true, HistoryExtendStore: fx.stores.History}, fx.fakeClock)
			return NewTerminateService(fx.engine, fx.next, fx.comments, fx.operations, fx.messages, histories, fx.documents, fx.fakeClock)
		}, func(svc *TerminateService, instance api.ProcessInstance, _ api.Task) error {
			return svc.StopByInstance(ctx, instance.ID, "carl", "")
		}},
		{"终止表单落位失败", func() *TerminateService {
			documents := NewDocumentService(&p2bFailingDocumentStore{failUpdateInstance: true, DocumentStore: fx.stores.Document}, fx.stores.Message, fx.stores.Operation, instances, fx.engine.HistoryService(), fx.fakeClock)
			return NewTerminateService(fx.engine, fx.next, fx.comments, fx.operations, fx.messages, fx.histories, documents, fx.fakeClock)
		}, func(svc *TerminateService, instance api.ProcessInstance, _ api.Task) error {
			return svc.StopByInstance(ctx, instance.ID, "carl", "")
		}},
		{"删除操作记录失败", func() *TerminateService {
			ops := NewOperationRecordService(&p2bFailingOperationStore{failSave: true, OperationRecordStore: fx.stores.Operation}, fx.engine.HistoryService(), instances, fx.engine.RepositoryService(), fx.engine.IdentityService(), fx.fakeClock)
			return NewTerminateService(fx.engine, fx.next, fx.comments, ops, fx.messages, fx.histories, fx.documents, fx.fakeClock, WithTerminateDocumentStore(fx.stores.Document))
		}, func(svc *TerminateService, instance api.ProcessInstance, _ api.Task) error {
			return svc.DeleteInstance(ctx, instance.ID, "carl", "")
		}},
		{"删除待办清理失败", func() *TerminateService {
			messages := NewMessageExtendService(&p2bFailingMessageStore{failDeleteInstance: true, MessageExtendStore: fx.stores.Message}, fx.fakeClock)
			return NewTerminateService(fx.engine, fx.next, fx.comments, fx.operations, messages, fx.histories, fx.documents, fx.fakeClock, WithTerminateDocumentStore(fx.stores.Document))
		}, func(svc *TerminateService, instance api.ProcessInstance, _ api.Task) error {
			return svc.DeleteInstance(ctx, instance.ID, "carl", "")
		}},
		{"删除表单标记失败", func() *TerminateService {
			docStore := &p2bFailingDocumentStore{failMarkDeleted: true, DocumentStore: fx.stores.Document}
			documents := NewDocumentService(docStore, fx.stores.Message, fx.stores.Operation, instances, fx.engine.HistoryService(), fx.fakeClock)
			return NewTerminateService(fx.engine, fx.next, fx.comments, fx.operations, fx.messages, fx.histories, documents, fx.fakeClock, WithTerminateDocumentStore(docStore))
		}, func(svc *TerminateService, instance api.ProcessInstance, _ api.Task) error {
			return svc.DeleteInstance(ctx, instance.ID, "carl", "")
		}},
		{"挂起意见失败", func() *TerminateService {
			comments := NewCommentService(&p2bFailingCommentTasks{failAddProcess: true}, fx.engine.HistoryService(), fx.engine.IdentityService())
			return NewTerminateService(fx.engine, fx.next, comments, fx.operations, fx.messages, fx.histories, fx.documents, fx.fakeClock)
		}, func(svc *TerminateService, instance api.ProcessInstance, _ api.Task) error {
			return svc.Suspend(ctx, instance.ID, "carl")
		}},
		{"激活操作记录失败", func() *TerminateService {
			ops := NewOperationRecordService(&p2bFailingOperationStore{failSave: true, OperationRecordStore: fx.stores.Operation}, fx.engine.HistoryService(), instances, fx.engine.RepositoryService(), fx.engine.IdentityService(), fx.fakeClock)
			return NewTerminateService(fx.engine, fx.next, fx.comments, ops, fx.messages, fx.histories, fx.documents, fx.fakeClock)
		}, func(svc *TerminateService, instance api.ProcessInstance, _ api.Task) error {
			// 先挂起成功再激活（激活链写 LCJH 记录时注入失败）
			plain := NewTerminateService(fx.engine, fx.next, fx.comments, fx.operations, fx.messages, fx.histories, fx.documents, fx.fakeClock)
			if err := plain.Suspend(ctx, instance.ID, "carl"); err != nil {
				return err
			}
			return svc.Activate(ctx, instance.ID, "carl")
		}},
		{"重激活意见失败", func() *TerminateService {
			comments := NewCommentService(&p2bFailingCommentTasks{failAddProcess: true}, fx.engine.HistoryService(), fx.engine.IdentityService())
			return NewTerminateService(fx.engine, fx.next, comments, fx.operations, fx.messages, fx.histories, fx.documents, fx.fakeClock)
		}, func(svc *TerminateService, instance api.ProcessInstance, task api.Task) error {
			trmComplete(t, fx, task.ID, "alice")
			trmComplete(t, fx, trmFirstTask(t, fx, instance.ID).ID, "alice")
			return svc.Reactivate(ctx, instance.ID, "", "carl")
		}},
		{"重激活表单重建失败", func() *TerminateService {
			docStore := &p2bFailingDocumentStore{failSave: true, DocumentStore: fx.stores.Document}
			documents := NewDocumentService(docStore, fx.stores.Message, fx.stores.Operation, instances, fx.engine.HistoryService(), fx.fakeClock)
			return NewTerminateService(fx.engine, fx.next, fx.comments, fx.operations, fx.messages, fx.histories, documents, fx.fakeClock)
		}, func(svc *TerminateService, instance api.ProcessInstance, task api.Task) error {
			trmComplete(t, fx, task.ID, "alice")
			trmComplete(t, fx, trmFirstTask(t, fx, instance.ID).ID, "alice")
			return svc.Reactivate(ctx, instance.ID, "", "carl")
		}},
		{"重激活待办查询失败", func() *TerminateService {
			messages := NewMessageExtendService(&p2bFailingMessageStore{failGetTask: true, MessageExtendStore: fx.stores.Message}, fx.fakeClock)
			return NewTerminateService(fx.engine, fx.next, fx.comments, fx.operations, messages, fx.histories, fx.documents, fx.fakeClock)
		}, func(svc *TerminateService, instance api.ProcessInstance, task api.Task) error {
			trmComplete(t, fx, task.ID, "alice")
			trmComplete(t, fx, trmFirstTask(t, fx, instance.ID).ID, "alice")
			return svc.Reactivate(ctx, instance.ID, "", "carl")
		}},
	}
	for _, variant := range variants {
		// 每个变体独立实例：引擎动作在收尾失败前已生效
		instance, taskA := trmStart(t, fx, "biz-err")
		trmSeedInstanceData(t, fx, instance, taskA)
		svc := variant.build()
		if err := variant.run(svc, instance, taskA); err == nil {
			t.Fatalf("%s 应传播错误", variant.name)
		}
	}
	// nil 时钟构造分支
	svc := NewTerminateService(fx.engine, fx.next, fx.comments, fx.operations, fx.messages, fx.histories, fx.documents, nil)
	if svc == nil {
		t.Fatalf("nil 时钟构造应回退真实时钟")
	}
	// 未注入单据存储端口：删除链路跳过 document 逻辑删除
	instance, taskA := trmStart(t, fx, "biz-nostore")
	trmSeedInstanceData(t, fx, instance, taskA)
	if err := svc.DeleteInstance(ctx, instance.ID, "carl", ""); err != nil {
		t.Fatalf("未注入单据端口的删除应成功: %v", err)
	}
	if _, found, _ := fx.documents.GetByBusinessID(ctx, instance.BusinessKey, instance.TenantID); !found {
		t.Fatalf("未注入单据端口时 document 不应被逻辑删除")
	}
	// 白盒：createInstanceTaskTodo 幂等跳过（已有待办 / 无办理人）
	liveInstance, liveTask := trmStart(t, fx, "biz-whitebox")
	created, err := createInstanceTaskTodo(ctx, fx.engine, fx.messages, fx.histories, liveInstance, liveTask, "carl")
	if err != nil || !created {
		t.Fatalf("无待办任务应创建待办: %v %v", created, err)
	}
	created, err = createInstanceTaskTodo(ctx, fx.engine, fx.messages, fx.histories, liveInstance, liveTask, "carl")
	if err != nil || created {
		t.Fatalf("已有待办任务应跳过: %v %v", created, err)
	}
	orphan := liveTask
	orphan.Assignee = ""
	created, err = createInstanceTaskTodo(ctx, fx.engine, fx.messages, fx.histories, liveInstance, orphan, "carl")
	if err != nil || created {
		t.Fatalf("无办理人任务应跳过: %v %v", created, err)
	}
	// 白盒：regenerateTodos 实例不存在错误传播
	if err := fx.terminate.regenerateTodos(ctx, "pi-missing", "carl"); err == nil {
		t.Fatalf("实例不存在应传播错误")
	}
	// 业务键无实例时清扫扩展数据（消息删除注入失败 → 传播）
	bizMessages := NewMessageExtendService(&p2bFailingMessageStore{failDeleteBusiness: true, MessageExtendStore: fx.stores.Message}, fx.fakeClock)
	bizSvc := NewTerminateService(fx.engine, fx.next, fx.comments, fx.operations, bizMessages, fx.histories, fx.documents, fx.fakeClock, WithTerminateDocumentStore(fx.stores.Document))
	if err := bizSvc.DeleteByBusinessID(ctx, "dev", "biz-none", "carl"); err == nil {
		t.Fatalf("业务键待办清扫失败应传播错误")
	}
	// 空业务键实例的删除：document 逻辑删除分支跳过（businessKey 为空）
	noBiz, err := fx.engine.RuntimeService().StartProcessInstance(ctx, api.StartRequest{
		ProcessDefinitionKey: "trmFlow",
		Name:                 "无业务键实例",
		TenantID:             "dev",
		UserID:               "alice",
	})
	if err != nil {
		t.Fatalf("无业务键发起失败: %v", err)
	}
	if err := fx.terminate.DeleteInstance(ctx, noBiz.ID, "carl", ""); err != nil {
		t.Fatalf("无业务键实例删除应成功: %v", err)
	}
}
