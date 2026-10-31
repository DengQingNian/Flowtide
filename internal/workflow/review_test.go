package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
	"github.com/DengQingNian/Flowtide/internal/engine"
	"github.com/DengQingNian/Flowtide/internal/idgen"
)

// ─── 审阅影子任务测试语料（全部为本文件专用，前缀 rv 避免与他人语料冲突）───

// rvReviewProcess 是审阅测试语料：apply(${initiator}) → handle(${initiator}) → end。
const rvReviewProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="rvProc" name="审阅测试流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="apply"/>
    <userTask id="apply" name="申请" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f1" sourceRef="apply" targetRef="handle"/>
    <userTask id="handle" name="办理" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f2" sourceRef="handle" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// rvUsers 是审阅测试用户（Id → 姓名）。
var rvUsers = []api.User{
	{ID: "alice", DisplayName: "张三"},
	{ID: "bob", DisplayName: "李四"},
	{ID: "carl", DisplayName: "王五"},
}

// rvFixture 是审阅影子任务测试环境（内存引擎 + 内存存储 + 审阅服务）。
type rvFixture struct {
	t       *testing.T          // 测试上下文
	engine  api.Engine          // 内存引擎
	reviews *MemReviewTaskStore // 影子任务内存存储
	extends *MemActExtendStore  // 审阅人绑定内存存储
	svc     *ReviewTaskService  // 审阅域服务
	fake    *clock.Fake         // 假时钟
	base    time.Time           // 基准时间（种子数据确定性排序用）
	seq     int                 // 种子序号（绑定创建时间递增，规避 map 遍历随机序）
}

// newRVFixture 构造审阅测试环境（部署语料、建用户与组、装配服务）。
// 组：grpA（成员 alice/bob，组织绑定用）、grpB（成员 carl，角色绑定用）、dynRole（无成员，触发待解析）。
// @param t 测试上下文
// @return 测试环境
func newRVFixture(t *testing.T) *rvFixture {
	t.Helper()
	fake := clock.NewFake(time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC))
	eng, err := engine.New(engine.Config{
		HistoryLevel: engine.HistoryAudit,
		Clock:        fake,
		IDs:          idgen.NewUUIDv7(),
		TenantID:     "dev",
	})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close(context.Background()) })
	if _, err := eng.RepositoryService().Deploy(context.Background(), api.DeploymentRequest{
		Name:     "review-test",
		TenantID: "dev",
		Resources: map[string][]byte{
			"rvProc.bpmn20.xml": []byte(rvReviewProcess),
		},
	}); err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	ctx := context.Background()
	for _, user := range rvUsers {
		if _, err := eng.IdentityService().CreateUser(ctx, user); err != nil {
			t.Fatalf("创建用户 %s 失败: %v", user.ID, err)
		}
	}
	for _, group := range []api.Group{{ID: "grpA", Name: "组A"}, {ID: "grpB", Name: "组B"}, {ID: "dynRole", Name: "空角色"}} {
		if _, err := eng.IdentityService().CreateGroup(ctx, group); err != nil {
			t.Fatalf("创建组 %s 失败: %v", group.ID, err)
		}
	}
	for _, pair := range [][2]string{{"alice", "grpA"}, {"bob", "grpA"}, {"carl", "grpB"}} {
		if err := eng.IdentityService().AddMembership(ctx, pair[0], pair[1]); err != nil {
			t.Fatalf("添加成员 %v 失败: %v", pair, err)
		}
	}
	fx := &rvFixture{t: t, engine: eng, fake: fake, base: time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)}
	fx.reviews = NewMemReviewTaskStore(fake)
	fx.extends = NewMemActExtendStore()
	fx.svc = NewReviewTaskService(eng, fx.reviews, fx.extends, fake)
	return fx
}

// rvSeedBinding 播种一条审阅人绑定。
// @param activityID BPMN 节点 ID
// @param ownerType 审阅人类型（person/org/role）
// @param ownerValue 审阅人值
// @return 播种后的绑定
func (f *rvFixture) rvSeedBinding(activityID, ownerType, ownerValue string) *ActExtend {
	f.t.Helper()
	// 绑定创建时间按种子序递增，保证内存存储排序输出确定（Go map 遍历序随机）
	f.seq++
	ext := &ActExtend{
		BaseColumns:          BaseColumns{ID: "rvext-" + activityID + "-" + ownerType + "-" + ownerValue, CreateTime: f.base.Add(time.Duration(f.seq) * time.Second), UpdateTime: f.base.Add(time.Duration(f.seq) * time.Second), TenantID: "dev"},
		ActivityID:           activityID,
		ActivityName:         "节点" + activityID,
		ProcessDefinitionKey: "rvProc",
		OwnerType:            ownerType,
		OwnerValue:           ownerValue,
	}
	if err := f.extends.Save(context.Background(), ext); err != nil {
		f.t.Fatalf("播种审阅绑定失败: %v", err)
	}
	return ext
}

// rvStart 以 alice 发起流程实例并返回实例与首个任务。
// @return 流程实例
// @return 首个任务
func (f *rvFixture) rvStart() (api.ProcessInstance, api.Task) {
	f.t.Helper()
	instance, err := f.engine.RuntimeService().StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "rvProc",
		BusinessKey:          "BIZ-rv-1",
		Name:                 "审阅-实例",
		TenantID:             "dev",
		UserID:               "alice",
		Variables:            []api.Variable{{Name: VarInitiator, Value: "alice"}},
	})
	if err != nil {
		f.t.Fatalf("发起失败: %v", err)
	}
	return instance, f.rvTask(instance.ID)
}

// rvTask 返回实例当前首个任务。
// @param instanceID 实例 Id
// @return 首个任务
func (f *rvFixture) rvTask(instanceID string) api.Task {
	f.t.Helper()
	page, err := f.engine.TaskService().ListTasks(context.Background(), api.TaskFilter{ProcessInstanceID: instanceID, Size: 10})
	if err != nil {
		f.t.Fatalf("查询任务失败: %v", err)
	}
	if len(page.Data) == 0 {
		f.t.Fatalf("实例 %s 无任务", instanceID)
	}
	return page.Data[0]
}

// rvComplete 以指定变量完成任务。
// @param taskID 任务 Id
// @param operator 操作人
// @param vars 流程变量
func (f *rvFixture) rvComplete(taskID, operator string, vars map[string]any) {
	f.t.Helper()
	req := api.CompleteTaskRequest{UserID: operator}
	for name, value := range vars {
		req.Variables = append(req.Variables, api.Variable{Name: name, Value: value})
	}
	if err := f.engine.TaskService().Complete(context.Background(), taskID, req); err != nil {
		f.t.Fatalf("办结任务失败: %v", err)
	}
}

// ─── SyncForTask：person 直用 / org/role 展开 / 幂等 ───

// TestReviewSyncPersonBinding person 绑定直用 owner_value，影子任务字段对齐任务与实例。
// @param t 测试上下文
func TestReviewSyncPersonBinding(t *testing.T) {
	fx := newRVFixture(t)
	fx.rvSeedBinding("apply", ActOwnerTypePerson, "carl")
	instance, task := fx.rvStart()
	created, err := fx.svc.SyncForTask(context.Background(), &task, "alice", "张三")
	if err != nil {
		t.Fatalf("同步审阅任务失败: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("应生成 1 条影子任务: %d", len(created))
	}
	rt := created[0]
	// 关键字段对齐任务与实例
	if rt.TaskID != task.ID || rt.TaskName != "申请" || rt.TaskKey != "apply" {
		t.Fatalf("任务字段不符: %+v", rt)
	}
	if rt.ProcessInstanceID != instance.ID || rt.BusinessKey != "BIZ-rv-1" || rt.ProcessName != "审阅-实例" {
		t.Fatalf("实例字段不符: %+v", rt)
	}
	if rt.ProcessDefinitionKey != "rvProc" || rt.ProcessDefinitionName != "审阅测试流程" || rt.ProcessDefinitionID == "" {
		t.Fatalf("定义字段不符: %+v", rt)
	}
	if rt.Reviewer != "carl" || rt.Status != ReviewStatusPending || rt.SenderID != "alice" || rt.SenderName != "张三" {
		t.Fatalf("审阅人/发送人/状态不符: %+v", rt)
	}
	if rt.TenantID != "dev" || rt.ID == "" || rt.StartTime.IsZero() || rt.CreateTime.IsZero() {
		t.Fatalf("审计列/租户不符: %+v", rt)
	}
	if !rt.StartTime.Equal(rt.CreateTime) {
		t.Fatalf("start_time 应与生成时间一致: %+v", rt)
	}
}

// TestReviewSyncIdempotent 同任务重复同步不产生重复影子任务（task_id + reviewer 幂等）。
// @param t 测试上下文
func TestReviewSyncIdempotent(t *testing.T) {
	fx := newRVFixture(t)
	fx.rvSeedBinding("apply", ActOwnerTypePerson, "carl")
	_, task := fx.rvStart()
	if _, err := fx.svc.SyncForTask(context.Background(), &task, "alice", "张三"); err != nil {
		t.Fatalf("首次同步失败: %v", err)
	}
	created, err := fx.svc.SyncForTask(context.Background(), &task, "alice", "张三")
	if err != nil {
		t.Fatalf("重复同步失败: %v", err)
	}
	if len(created) != 0 {
		t.Fatalf("重复同步不应新增: %d", len(created))
	}
	rows, err := fx.svc.ListByInstance(context.Background(), task.ProcessInstanceID)
	if err != nil {
		t.Fatalf("查询实例影子任务失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("实例影子任务应仍为 1 条: %d", len(rows))
	}
}

// TestReviewSyncOrgAndRoleExpand org/role 绑定经身份服务展开为组成员。
// @param t 测试上下文
func TestReviewSyncOrgAndRoleExpand(t *testing.T) {
	fx := newRVFixture(t)
	fx.rvSeedBinding("apply", ActOwnerTypeOrg, "grpA")
	fx.rvSeedBinding("apply", ActOwnerTypeRole, "grpB")
	_, task := fx.rvStart()
	created, err := fx.svc.SyncForTask(context.Background(), &task, "alice", "张三")
	if err != nil {
		t.Fatalf("同步审阅任务失败: %v", err)
	}
	// grpA 展开为 alice/bob，grpB 展开为 carl
	if len(created) != 3 {
		t.Fatalf("应展开出 3 条影子任务: %+v", created)
	}
	reviewers := make([]string, 0, len(created))
	for _, rt := range created {
		reviewers = append(reviewers, rt.Reviewer)
		if rt.Status != ReviewStatusPending {
			t.Fatalf("展开成功的影子任务应为待审阅: %+v", rt)
		}
	}
	assertSameStrings(t, reviewers, []string{"alice", "bob", "carl"})
}

// TestReviewSyncUnresolvedKeepsOriginal org/role 展开失败保留原值并置待解析。
// @param t 测试上下文
func TestReviewSyncUnresolvedKeepsOriginal(t *testing.T) {
	fx := newRVFixture(t)
	// noOrg 组不存在、dynRole 组存在但无成员，两种展开失败形态都保留原值
	fx.rvSeedBinding("apply", ActOwnerTypeOrg, "noOrg")
	fx.rvSeedBinding("apply", ActOwnerTypeRole, "dynRole")
	_, task := fx.rvStart()
	created, err := fx.svc.SyncForTask(context.Background(), &task, "alice", "张三")
	if err != nil {
		t.Fatalf("同步审阅任务失败: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("应生成 2 条待解析影子任务: %+v", created)
	}
	for _, rt := range created {
		if rt.Status != ReviewStatusUnresolved {
			t.Fatalf("展开失败应置待解析: %+v", rt)
		}
		if rt.Reviewer != "noOrg" && rt.Reviewer != "dynRole" {
			t.Fatalf("展开失败应保留原值: %+v", rt)
		}
	}
	// 待解析不计入未读
	count, err := fx.svc.CountUnreadByReviewer(context.Background(), "noOrg", "dev")
	if err != nil {
		t.Fatalf("未读计数失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("待解析不应计入未读: %d", count)
	}
}

// TestReviewSyncNoBindingAndParamErrors 节点无绑定为空列表；参数缺失/未知定义报错。
// @param t 测试上下文
func TestReviewSyncNoBindingAndParamErrors(t *testing.T) {
	fx := newRVFixture(t)
	_, task := fx.rvStart()
	// 无绑定节点：空列表非错误
	created, err := fx.svc.SyncForTask(context.Background(), &task, "alice", "张三")
	if err != nil {
		t.Fatalf("无绑定同步不应报错: %v", err)
	}
	if len(created) != 0 {
		t.Fatalf("无绑定应返回空列表: %+v", created)
	}
	// nil 任务
	if _, err := fx.svc.SyncForTask(context.Background(), nil, "alice", "张三"); err == nil {
		t.Fatal("nil 任务应报错")
	}
	// 任务缺实例标识
	broken := task
	broken.ProcessInstanceID = ""
	if _, err := fx.svc.SyncForTask(context.Background(), &broken, "alice", "张三"); err == nil {
		t.Fatal("缺实例标识应报错")
	}
	// 未知定义
	ghost := task
	ghost.ProcessDefinitionID = "no-such-def"
	if _, err := fx.svc.SyncForTask(context.Background(), &ghost, "alice", "张三"); err == nil {
		t.Fatal("未知定义应报错")
	}
	// 未注入引擎
	bare := NewReviewTaskService(nil, fx.reviews, fx.extends, fx.fake)
	if _, err := bare.SyncForTask(context.Background(), &task, "alice", "张三"); err == nil {
		t.Fatal("未注入引擎应报错")
	}
}

// TestReviewSyncSameTaskMultipleNodes 同实例不同任务分别同步，互不串扰（幂等键含任务 Id）。
// @param t 测试上下文
func TestReviewSyncSameTaskMultipleNodes(t *testing.T) {
	fx := newRVFixture(t)
	fx.rvSeedBinding("apply", ActOwnerTypePerson, "carl")
	fx.rvSeedBinding("handle", ActOwnerTypeOrg, "grpA")
	instance, task := fx.rvStart()
	if _, err := fx.svc.SyncForTask(context.Background(), &task, "alice", "张三"); err != nil {
		t.Fatalf("apply 同步失败: %v", err)
	}
	// 推进到 handle
	fx.rvComplete(task.ID, "alice", nil)
	handleTask := fx.rvTask(instance.ID)
	created, err := fx.svc.SyncForTask(context.Background(), &handleTask, "alice", "张三")
	if err != nil {
		t.Fatalf("handle 同步失败: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("handle 应展开 2 条影子任务: %+v", created)
	}
	rows, err := fx.svc.ListByInstance(context.Background(), instance.ID)
	if err != nil {
		t.Fatalf("查询实例影子任务失败: %v", err)
	}
	// apply 1 条 + handle 2 条 = 3 条
	if len(rows) != 3 {
		t.Fatalf("实例影子任务总数应为 3: %d", len(rows))
	}
}

// ─── Complete：办结回写 ───

// TestReviewCompleteWriteBack 办结回写 review_time/opinion/status/updater。
// @param t 测试上下文
func TestReviewCompleteWriteBack(t *testing.T) {
	fx := newRVFixture(t)
	fx.rvSeedBinding("apply", ActOwnerTypePerson, "carl")
	_, task := fx.rvStart()
	created, err := fx.svc.SyncForTask(context.Background(), &task, "alice", "张三")
	if err != nil {
		t.Fatalf("同步审阅任务失败: %v", err)
	}
	fx.fake.Advance(2 * time.Hour)
	done, err := fx.svc.Complete(context.Background(), created[0].ID, "同意申请", "carl")
	if err != nil {
		t.Fatalf("办结审阅失败: %v", err)
	}
	if done.Status != ReviewStatusDone || done.Opinion != "同意申请" {
		t.Fatalf("办结回写不符: %+v", done)
	}
	if done.ReviewTime == nil || !done.ReviewTime.Equal(fx.fake.Now()) {
		t.Fatalf("审阅时间不符: %+v", done)
	}
	if done.Updater != "carl" || !done.UpdateTime.Equal(fx.fake.Now()) {
		t.Fatalf("操作人/更新时间不符: %+v", done)
	}
	// 存储回读一致
	rt, found, err := fx.svc.Get(context.Background(), created[0].ID)
	if err != nil || !found {
		t.Fatalf("回读影子任务失败: found=%v err=%v", found, err)
	}
	if rt.Status != ReviewStatusDone || rt.Opinion != "同意申请" || rt.ReviewTime == nil {
		t.Fatalf("存储回写不符: %+v", rt)
	}
}

// TestReviewCompleteErrors 空参数与未知影子任务报错。
// @param t 测试上下文
func TestReviewCompleteErrors(t *testing.T) {
	fx := newRVFixture(t)
	if _, err := fx.svc.Complete(context.Background(), "", "意见", "op"); err == nil {
		t.Fatal("空影子任务 Id 应报错")
	}
	if _, err := fx.svc.Complete(context.Background(), "no-such-rt", "意见", "op"); err == nil {
		t.Fatal("未知影子任务应报错")
	}
}

// ─── 查询：分页 / 租户隔离 / 未读计数 / 实例列表 ───

// rvSeedReview 直接向存储播种一条影子任务（绕过服务以控制租户与时间）。
// @param id 影子任务 Id
// @param reviewer 审阅人
// @param tenantID 租户
// @param status 状态
// @param startAt 生成时间
// @param fx 参数 fx
func rvSeedReview(fx *rvFixture, id, reviewer, tenantID string, status int, startAt time.Time) {
	fx.t.Helper()
	rt := &ReviewTask{
		BaseColumns:       BaseColumns{ID: id, Creator: "op", CreateTime: startAt, UpdateTime: startAt, TenantID: tenantID},
		StartTime:         startAt,
		TaskID:            "task-" + id,
		ProcessInstanceID: "pi-" + id,
		TaskKey:           "apply",
		Reviewer:          reviewer,
		Status:            status,
	}
	if err := fx.reviews.Insert(context.Background(), rt); err != nil {
		fx.t.Fatalf("播种影子任务失败: %v", err)
	}
}

// TestReviewListByReviewerPaging 分页与租户隔离：start/size 切页、跨租户互不可见。
// @param t 测试上下文
func TestReviewListByReviewerPaging(t *testing.T) {
	fx := newRVFixture(t)
	base := time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)
	// dev 租户 3 条 + other 租户 1 条
	rvSeedReview(fx, "r1", "carl", "dev", ReviewStatusPending, base)
	rvSeedReview(fx, "r2", "carl", "dev", ReviewStatusDone, base.Add(time.Hour))
	rvSeedReview(fx, "r3", "carl", "dev", ReviewStatusPending, base.Add(2*time.Hour))
	rvSeedReview(fx, "r4", "carl", "other", ReviewStatusPending, base)
	ctx := context.Background()
	// 第一页 2 条，总数 3（dev 租户）
	page1, err := fx.svc.ListByReviewer(ctx, "carl", "dev", PageQuery{Start: 0, Size: 2})
	if err != nil {
		t.Fatalf("分页查询失败: %v", err)
	}
	if page1.Total != 3 || len(page1.Data) != 2 {
		t.Fatalf("第一页不符: total=%d n=%d", page1.Total, len(page1.Data))
	}
	// 第二页 1 条
	page2, err := fx.svc.ListByReviewer(ctx, "carl", "dev", PageQuery{Start: 2, Size: 2})
	if err != nil {
		t.Fatalf("分页查询失败: %v", err)
	}
	if page2.Total != 3 || len(page2.Data) != 1 {
		t.Fatalf("第二页不符: total=%d n=%d", page2.Total, len(page2.Data))
	}
	// 租户隔离：other 租户只有 1 条
	pageOther, err := fx.svc.ListByReviewer(ctx, "carl", "other", PageQuery{})
	if err != nil {
		t.Fatalf("租户隔离查询失败: %v", err)
	}
	if pageOther.Total != 1 || pageOther.Data[0].ID != "r4" {
		t.Fatalf("租户隔离不符: %+v", pageOther)
	}
	// 空租户不过滤
	pageAll, err := fx.svc.ListByReviewer(ctx, "carl", "", PageQuery{})
	if err != nil {
		t.Fatalf("空租户查询失败: %v", err)
	}
	if pageAll.Total != 4 {
		t.Fatalf("空租户应返回全部: %d", pageAll.Total)
	}
	// 按 start_time 升序
	if page1.Data[0].ID != "r1" || page1.Data[1].ID != "r2" {
		t.Fatalf("排序不符: %+v", page1.Data)
	}
}

// TestReviewCountUnread 未读计数：仅待审阅计入，办结后递减。
// @param t 测试上下文
func TestReviewCountUnread(t *testing.T) {
	fx := newRVFixture(t)
	base := time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)
	rvSeedReview(fx, "c1", "carl", "dev", ReviewStatusPending, base)
	rvSeedReview(fx, "c2", "carl", "dev", ReviewStatusPending, base.Add(time.Hour))
	rvSeedReview(fx, "c3", "carl", "dev", ReviewStatusDone, base.Add(2*time.Hour))
	ctx := context.Background()
	count, err := fx.svc.CountUnreadByReviewer(ctx, "carl", "dev")
	if err != nil {
		t.Fatalf("未读计数失败: %v", err)
	}
	if count != 2 {
		t.Fatalf("未读数应为 2: %d", count)
	}
	// 办结一条后递减
	if _, err := fx.svc.Complete(ctx, "c1", "同意", "carl"); err != nil {
		t.Fatalf("办结失败: %v", err)
	}
	count, err = fx.svc.CountUnreadByReviewer(ctx, "carl", "dev")
	if err != nil {
		t.Fatalf("未读计数失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("办结后未读数应为 1: %d", count)
	}
	// 租户隔离计数
	if err := fx.reviews.Insert(ctx, &ReviewTask{
		BaseColumns: BaseColumns{ID: "c4", CreateTime: base, UpdateTime: base, TenantID: "other"},
		StartTime:   base, TaskID: "task-c4", Reviewer: "carl", Status: ReviewStatusPending,
	}); err != nil {
		t.Fatalf("播种失败: %v", err)
	}
	count, err = fx.svc.CountUnreadByReviewer(ctx, "carl", "dev")
	if err != nil {
		t.Fatalf("未读计数失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("跨租户不应计入未读: %d", count)
	}
}

// TestReviewListByInstanceAndDelete 实例列表按生成时间升序，删除按实例清理。
// @param t 测试上下文
func TestReviewListByInstanceAndDelete(t *testing.T) {
	fx := newRVFixture(t)
	base := time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)
	rvSeedReview(fx, "d1", "carl", "dev", ReviewStatusPending, base)
	rvSeedReview(fx, "d2", "bob", "dev", ReviewStatusPending, base.Add(time.Hour))
	rvSeedReview(fx, "d3", "carl", "dev", ReviewStatusPending, base.Add(2*time.Hour))
	ctx := context.Background()
	rows, err := fx.svc.ListByInstance(ctx, "pi-d1")
	if err != nil {
		t.Fatalf("实例查询失败: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "d1" {
		t.Fatalf("实例列表不符: %+v", rows)
	}
	if _, err := fx.svc.ListByInstance(ctx, "pi-d2"); err != nil {
		t.Fatalf("实例查询失败: %v", err)
	}
	// 删除实例 d1 的影子任务
	if err := fx.reviews.DeleteByInstance(ctx, "pi-d1"); err != nil {
		t.Fatalf("按实例删除失败: %v", err)
	}
	rt, found, err := fx.reviews.Get(ctx, "d1")
	if err != nil || found {
		t.Fatalf("删除后不应可查: found=%v err=%v", found, err)
	}
	_ = rt
	// 其余实例不受影响
	rows, err = fx.svc.ListByInstance(ctx, "pi-d2")
	if err != nil || len(rows) != 1 {
		t.Fatalf("其他实例不受影响: n=%d err=%v", len(rows), err)
	}
}

// ─── 内存存储语义：克隆 / 主键查询 / Update 缺行静默 ───

// TestReviewMemStoreClone 内存存储克隆语义：返回副本修改不影响内部状态。
// @param t 测试上下文
func TestReviewMemStoreClone(t *testing.T) {
	fx := newRVFixture(t)
	base := time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)
	rvSeedReview(fx, "k1", "carl", "dev", ReviewStatusPending, base)
	ctx := context.Background()
	rt, found, err := fx.reviews.Get(ctx, "k1")
	if err != nil || !found {
		t.Fatalf("查询失败: found=%v err=%v", found, err)
	}
	// 修改副本
	rt.Reviewer = "hacker"
	rt.Opinion = "篡改"
	if rt.ReviewTime != nil {
		t.Fatal("初始 ReviewTime 应为 nil")
	}
	rt2, _, _ := fx.reviews.Get(ctx, "k1")
	if rt2.Reviewer != "carl" || rt2.Opinion != "" {
		t.Fatalf("内部状态被副本修改污染: %+v", rt2)
	}
	// Get 缺失
	if _, found, _ := fx.reviews.Get(ctx, "missing"); found {
		t.Fatal("缺失主键应返回不存在")
	}
	// Update 缺行静默
	if err := fx.reviews.Update(ctx, &ReviewTask{BaseColumns: BaseColumns{ID: "missing"}}); err != nil {
		t.Fatalf("Update 缺行应静默: %v", err)
	}
	// cloneReview 对 nil 的防御
	if rvCloneReview(nil) != nil || rvCloneActExtend(nil) != nil {
		t.Fatal("nil 克隆应返回 nil")
	}
}

// TestReviewMemStoreClockFallback 空时钟回退真实时钟（构造路径覆盖）。
// @param t 测试上下文
func TestReviewMemStoreClockFallback(t *testing.T) {
	store := NewMemReviewTaskStore(nil)
	if store.Now().IsZero() {
		t.Fatal("真实时钟回退失败")
	}
}

// ─── ActExtendStore：绑定存取 ───

// TestReviewActExtendStore 绑定保存/按节点查询/按定义查询/删除。
// @param t 测试上下文
func TestReviewActExtendStore(t *testing.T) {
	fx := newRVFixture(t)
	ctx := context.Background()
	first := fx.rvSeedBinding("apply", ActOwnerTypePerson, "carl")
	second := fx.rvSeedBinding("apply", ActOwnerTypeOrg, "grpA")
	fx.rvSeedBinding("handle", ActOwnerTypeRole, "grpB")
	// 按定义+节点查询
	bindings, err := fx.extends.GetByDefinitionKeyAndActivity(ctx, "rvProc", "apply")
	if err != nil {
		t.Fatalf("按节点查询绑定失败: %v", err)
	}
	if len(bindings) != 2 || bindings[0].ID != first.ID || bindings[1].ID != second.ID {
		t.Fatalf("按节点绑定不符: %+v", bindings)
	}
	// 按定义查询
	all, err := fx.extends.ListByDefinitionKey(ctx, "rvProc")
	if err != nil {
		t.Fatalf("按定义查询绑定失败: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("定义绑定数不符: %d", len(all))
	}
	// 删除后不再返回
	if err := fx.extends.Delete(ctx, second.ID); err != nil {
		t.Fatalf("删除绑定失败: %v", err)
	}
	bindings, err = fx.extends.GetByDefinitionKeyAndActivity(ctx, "rvProc", "apply")
	if err != nil || len(bindings) != 1 {
		t.Fatalf("删除后绑定不符: n=%d err=%v", len(bindings), err)
	}
	// 未定义 Key 查询为空
	empty, err := fx.extends.ListByDefinitionKey(ctx, "no-such")
	if err != nil || len(empty) != 0 {
		t.Fatalf("未知定义应为空: n=%d err=%v", len(empty), err)
	}
}

// ─── 端到端：发起 → 首任务同步 → 影子任务生成 → 办结回写 ───

// TestReviewEndToEnd 发起→首任务 SyncForTask→影子任务生成→办结回写 全链路。
// @param t 测试上下文
func TestReviewEndToEnd(t *testing.T) {
	fx := newRVFixture(t)
	fx.rvSeedBinding("apply", ActOwnerTypePerson, "carl")
	fx.rvSeedBinding("handle", ActOwnerTypeOrg, "grpA")
	instance, task := fx.rvStart()
	// 1. 首任务同步审阅人
	created, err := fx.svc.SyncForTask(context.Background(), &task, "alice", "张三")
	if err != nil {
		t.Fatalf("首任务同步失败: %v", err)
	}
	if len(created) != 1 || created[0].Reviewer != "carl" {
		t.Fatalf("首任务影子任务不符: %+v", created)
	}
	// 2. 审阅人未读可见
	count, err := fx.svc.CountUnreadByReviewer(context.Background(), "carl", "dev")
	if err != nil || count != 1 {
		t.Fatalf("审阅人未读应为 1: %d err=%v", count, err)
	}
	// 3. 办结回写
	if _, err := fx.svc.Complete(context.Background(), created[0].ID, "已阅", "carl"); err != nil {
		t.Fatalf("办结回写失败: %v", err)
	}
	count, err = fx.svc.CountUnreadByReviewer(context.Background(), "carl", "dev")
	if err != nil || count != 0 {
		t.Fatalf("办结后未读应为 0: %d err=%v", count, err)
	}
	// 4. 流程推进到 handle，org 绑定展开两人
	fx.rvComplete(task.ID, "alice", nil)
	handleTask := fx.rvTask(instance.ID)
	created, err = fx.svc.SyncForTask(context.Background(), &handleTask, "alice", "张三")
	if err != nil {
		t.Fatalf("handle 同步失败: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("handle 影子任务应为 2 条: %+v", created)
	}
	// 5. 实例级查询聚合 3 条影子任务
	rows, err := fx.svc.ListByInstance(context.Background(), instance.ID)
	if err != nil || len(rows) != 3 {
		t.Fatalf("实例影子任务聚合不符: n=%d err=%v", len(rows), err)
	}
}

// ─── 错误注入桩与回退路径 ───

// rvFailingReviewStore 是带错误注入的审阅影子任务存储桩（覆盖服务层错误传播路径）。
type rvFailingReviewStore struct {
	ReviewTaskStore      // 内嵌端口，未注入开关的方法走真实实现
	failGet         bool // Get 注入错误
	failList        bool // ListByInstance 注入错误
	failInsert      bool // Insert 注入错误
	failPage        bool // ListByReviewer 注入错误
	failUpdate      bool // Update 注入错误
}

// Update 按开关返回错误。
// @param ctx 操作上下文
// @param rt 影子任务
// @return 错误
func (f *rvFailingReviewStore) Update(ctx context.Context, rt *ReviewTask) error {
	// 错误注入点
	if f.failUpdate {
		return errors.New("注入错误")
	}
	return f.ReviewTaskStore.Update(ctx, rt)
}

// Get 按开关返回错误。
// @param ctx 操作上下文
// @param id 影子任务 Id
// @return 影子任务
// @return 是否存在
// @return 错误
func (f *rvFailingReviewStore) Get(ctx context.Context, id string) (*ReviewTask, bool, error) {
	// 错误注入点
	if f.failGet {
		return nil, false, errors.New("注入错误")
	}
	return f.ReviewTaskStore.Get(ctx, id)
}

// ListByInstance 按开关返回错误。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 列表
// @return 错误
func (f *rvFailingReviewStore) ListByInstance(ctx context.Context, processInstanceID string) ([]*ReviewTask, error) {
	// 错误注入点
	if f.failList {
		return nil, errors.New("注入错误")
	}
	return f.ReviewTaskStore.ListByInstance(ctx, processInstanceID)
}

// Insert 按开关返回错误。
// @param ctx 操作上下文
// @param rt 影子任务
// @return 错误
func (f *rvFailingReviewStore) Insert(ctx context.Context, rt *ReviewTask) error {
	// 错误注入点
	if f.failInsert {
		return errors.New("注入错误")
	}
	return f.ReviewTaskStore.Insert(ctx, rt)
}

// ListByReviewer 按开关返回错误。
// @param ctx 操作上下文
// @param reviewer 审阅人 Id
// @param tenantID 租户
// @param page 分页参数
// @return 当前页列表
// @return 总数
// @return 错误
func (f *rvFailingReviewStore) ListByReviewer(ctx context.Context, reviewer, tenantID string, page PageQuery) ([]*ReviewTask, int64, error) {
	// 错误注入点
	if f.failPage {
		return nil, 0, errors.New("注入错误")
	}
	return f.ReviewTaskStore.ListByReviewer(ctx, reviewer, tenantID, page)
}

// rvFailingActExtendStore 是带错误注入的审阅人绑定存储桩。
type rvFailingActExtendStore struct {
	ActExtendStore      // 内嵌端口
	failLookup     bool // GetByDefinitionKeyAndActivity 注入错误
}

// GetByDefinitionKeyAndActivity 按开关返回错误。
// @param ctx 操作上下文
// @param definitionKey 流程定义 Key
// @param activityID 节点 ID
// @return 绑定列表
// @return 错误
func (f *rvFailingActExtendStore) GetByDefinitionKeyAndActivity(ctx context.Context, definitionKey, activityID string) ([]*ActExtend, error) {
	// 错误注入点
	if f.failLookup {
		return nil, errors.New("注入错误")
	}
	return f.ActExtendStore.GetByDefinitionKeyAndActivity(ctx, definitionKey, activityID)
}

// TestReviewServiceErrorPaths 服务层存储错误传播：绑定查询/实例列表/插入/主键查询/分页。
// @param t 测试上下文
func TestReviewServiceErrorPaths(t *testing.T) {
	fx := newRVFixture(t)
	fx.rvSeedBinding("apply", ActOwnerTypePerson, "carl")
	_, task := fx.rvStart()
	ctx := context.Background()
	// 绑定查询错误
	failExt := &rvFailingActExtendStore{ActExtendStore: fx.extends, failLookup: true}
	svc := NewReviewTaskService(fx.engine, fx.reviews, failExt, fx.fake)
	if _, err := svc.SyncForTask(ctx, &task, "alice", "张三"); err == nil {
		t.Fatal("绑定查询错误应传播")
	}
	// 实例列表（幂等检查）错误
	failList := &rvFailingReviewStore{ReviewTaskStore: fx.reviews, failList: true}
	svc = NewReviewTaskService(fx.engine, failList, fx.extends, fx.fake)
	if _, err := svc.SyncForTask(ctx, &task, "alice", "张三"); err == nil {
		t.Fatal("实例列表错误应传播")
	}
	// 插入错误
	failInsert := &rvFailingReviewStore{ReviewTaskStore: fx.reviews, failInsert: true}
	svc = NewReviewTaskService(fx.engine, failInsert, fx.extends, fx.fake)
	if _, err := svc.SyncForTask(ctx, &task, "alice", "张三"); err == nil {
		t.Fatal("插入错误应传播")
	}
	// 办结主键查询错误
	failGet := &rvFailingReviewStore{ReviewTaskStore: fx.reviews, failGet: true}
	svc = NewReviewTaskService(fx.engine, failGet, fx.extends, fx.fake)
	if _, err := svc.Complete(ctx, "rt-1", "意见", "op"); err == nil {
		t.Fatal("主键查询错误应传播")
	}
	// 分页查询错误
	failPage := &rvFailingReviewStore{ReviewTaskStore: fx.reviews, failPage: true}
	svc = NewReviewTaskService(fx.engine, failPage, fx.extends, fx.fake)
	if _, err := svc.ListByReviewer(ctx, "carl", "dev", PageQuery{}); err == nil {
		t.Fatal("分页查询错误应传播")
	}
}

// rvFailRuntimeService 是运行态查询错误注入桩（仅 GetProcessInstance 失效，其余透传）。
type rvFailRuntimeService struct {
	api.RuntimeService // 内嵌运行态服务，未覆写方法透传
}

// GetProcessInstance 恒返回不存在错误（模拟 SQL 引擎已结束/已删除实例的运行态查询）。
// @param ctx 上下文
// @param id 实例 ID
// @return 实例
// @return 不存在错误
func (rvFailRuntimeService) GetProcessInstance(ctx context.Context, id string) (api.ProcessInstance, error) {
	return api.ProcessInstance{}, api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
}

// rvFailHistoryService 是历史查询错误注入桩（仅 GetHistoricProcessInstance 失效，其余透传）。
type rvFailHistoryService struct {
	api.HistoryService // 内嵌历史服务，未覆写方法透传
}

// GetHistoricProcessInstance 恒返回不存在错误（模拟实例无任何历史）。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @return 历史实例
// @return 不存在错误
func (rvFailHistoryService) GetHistoricProcessInstance(ctx context.Context, processInstanceID string) (api.HistoricProcessInstance, error) {
	return api.HistoricProcessInstance{}, api.NewError("ENGINE-0404", "历史流程实例不存在", api.ErrNotFound)
}

// rvFailRuntimeEngine 是运行态查询失效的引擎包装（其余服务透传）。
type rvFailRuntimeEngine struct {
	api.Engine // 内嵌引擎门面
}

// RuntimeService 返回错误注入的运行态服务。
// @return 运行态服务
func (e rvFailRuntimeEngine) RuntimeService() api.RuntimeService {
	return rvFailRuntimeService{RuntimeService: e.Engine.RuntimeService()}
}

// rvGoneInstanceEngine 是运行态与历史实例查询均失效的引擎包装。
type rvGoneInstanceEngine struct {
	api.Engine // 内嵌引擎门面
}

// RuntimeService 返回错误注入的运行态服务。
// @return 运行态服务
func (e rvGoneInstanceEngine) RuntimeService() api.RuntimeService {
	return rvFailRuntimeService{RuntimeService: e.Engine.RuntimeService()}
}

// HistoryService 返回错误注入的历史服务。
// @return 历史服务
func (e rvGoneInstanceEngine) HistoryService() api.HistoryService {
	return rvFailHistoryService{HistoryService: e.Engine.HistoryService()}
}

// TestReviewSyncHistoricFallback 运行态查不到实例（SQL 引擎已结束形态）时回退历史实例同步。
// @param t 测试上下文
func TestReviewSyncHistoricFallback(t *testing.T) {
	fx := newRVFixture(t)
	fx.rvSeedBinding("apply", ActOwnerTypePerson, "carl")
	_, task := fx.rvStart()
	if _, err := fx.svc.SyncForTask(context.Background(), &task, "alice", "张三"); err != nil {
		t.Fatalf("首次同步失败: %v", err)
	}
	// 运行态查询失效、历史可查：走历史回退分支（校验 business_key/实例名取自历史）
	svc := NewReviewTaskService(rvFailRuntimeEngine{Engine: fx.engine}, fx.reviews, fx.extends, fx.fake)
	created, err := svc.SyncForTask(context.Background(), &task, "alice", "张三")
	if err != nil {
		t.Fatalf("历史回退同步失败: %v", err)
	}
	// 幂等：同任务+审阅人影子任务仍在，不应新增
	if len(created) != 0 {
		t.Fatalf("历史回退同步不应新增: %d", len(created))
	}
}

// TestReviewSyncInstanceGone 运行态与历史均查不到实例时报错（双查失败）。
// @param t 测试上下文
func TestReviewSyncInstanceGone(t *testing.T) {
	fx := newRVFixture(t)
	fx.rvSeedBinding("apply", ActOwnerTypePerson, "carl")
	_, task := fx.rvStart()
	svc := NewReviewTaskService(rvGoneInstanceEngine{Engine: fx.engine}, fx.reviews, fx.extends, fx.fake)
	if _, err := svc.SyncForTask(context.Background(), &task, "alice", "张三"); err == nil {
		t.Fatal("实例与历史均缺失应报错")
	}
}

// TestReviewServiceNilClock 空时钟回退真实时钟（构造路径覆盖）。
// @param t 测试上下文
func TestReviewServiceNilClock(t *testing.T) {
	svc := NewReviewTaskService(nil, NewMemReviewTaskStore(nil), NewMemActExtendStore(), nil)
	if svc == nil || svc.newID == nil {
		t.Fatal("空时钟构造应完整")
	}
}

// rvFailIdentityEngine 是身份服务错误注入引擎包装（ListMembers 失效，其余透传）。
type rvFailIdentityEngine struct {
	api.Engine // 内嵌引擎门面
}

// rvFailIdentityService 是身份服务错误注入桩。
type rvFailIdentityService struct {
	api.IdentityService // 内嵌身份服务
}

// ListMembers 恒返回错误（模拟身份库故障）。
// @param ctx 上下文
// @param groupID 组 ID
// @return 用户列表
// @return 错误
func (rvFailIdentityService) ListMembers(ctx context.Context, groupID string) ([]api.User, error) {
	return nil, errors.New("身份库故障")
}

// IdentityService 返回错误注入的身份服务。
// @return 身份服务
func (e rvFailIdentityEngine) IdentityService() api.IdentityService {
	return rvFailIdentityService{IdentityService: e.Engine.IdentityService()}
}

// TestReviewSyncIdentityError org/role 展开遇身份库故障保留原值置待解析。
// @param t 测试上下文
func TestReviewSyncIdentityError(t *testing.T) {
	fx := newRVFixture(t)
	fx.rvSeedBinding("apply", ActOwnerTypeOrg, "grpA")
	_, task := fx.rvStart()
	svc := NewReviewTaskService(rvFailIdentityEngine{Engine: fx.engine}, fx.reviews, fx.extends, fx.fake)
	created, err := svc.SyncForTask(context.Background(), &task, "alice", "张三")
	if err != nil {
		t.Fatalf("身份故障同步不应报错: %v", err)
	}
	if len(created) != 1 || created[0].Reviewer != "grpA" || created[0].Status != ReviewStatusUnresolved {
		t.Fatalf("身份故障应保留原值置待解析: %+v", created)
	}
}

// rvEmptyIdentityEngine 是身份服务空成员注入引擎包装（成员存在但均无 Id）。
type rvEmptyIdentityEngine struct {
	api.Engine // 内嵌引擎门面
}

// rvEmptyIdentityService 是身份服务空成员桩。
type rvEmptyIdentityService struct {
	api.IdentityService // 内嵌身份服务
}

// ListMembers 返回无 Id 的成员（模拟脏数据形态）。
// @param ctx 上下文
// @param groupID 组 ID
// @return 用户列表
// @return 错误
func (rvEmptyIdentityService) ListMembers(ctx context.Context, groupID string) ([]api.User, error) {
	return []api.User{{ID: "", DisplayName: "脏数据"}}, nil
}

// IdentityService 返回空成员注入的身份服务。
// @return 身份服务
func (e rvEmptyIdentityEngine) IdentityService() api.IdentityService {
	return rvEmptyIdentityService{IdentityService: e.Engine.IdentityService()}
}

// TestReviewSyncIdentityEmptyMembers 展开成员全部无 Id 时按展开失败处理。
// @param t 测试上下文
func TestReviewSyncIdentityEmptyMembers(t *testing.T) {
	fx := newRVFixture(t)
	fx.rvSeedBinding("apply", ActOwnerTypeRole, "grpB")
	_, task := fx.rvStart()
	svc := NewReviewTaskService(rvEmptyIdentityEngine{Engine: fx.engine}, fx.reviews, fx.extends, fx.fake)
	created, err := svc.SyncForTask(context.Background(), &task, "alice", "张三")
	if err != nil {
		t.Fatalf("空成员同步不应报错: %v", err)
	}
	if len(created) != 1 || created[0].Reviewer != "grpB" || created[0].Status != ReviewStatusUnresolved {
		t.Fatalf("成员全无 Id 应保留原值置待解析: %+v", created)
	}
}

// TestReviewCompleteUpdateError 办结回写存储失败传播。
// @param t 测试上下文
func TestReviewCompleteUpdateError(t *testing.T) {
	fx := newRVFixture(t)
	fx.rvSeedBinding("apply", ActOwnerTypePerson, "carl")
	_, task := fx.rvStart()
	created, err := fx.svc.SyncForTask(context.Background(), &task, "alice", "张三")
	if err != nil {
		t.Fatalf("同步失败: %v", err)
	}
	failUpdate := &rvFailingReviewStore{ReviewTaskStore: fx.reviews, failUpdate: true}
	svc := NewReviewTaskService(fx.engine, failUpdate, fx.extends, fx.fake)
	if _, err := svc.Complete(context.Background(), created[0].ID, "意见", "op"); err == nil {
		t.Fatal("回写失败应传播")
	}
}

// TestReviewMemStoreUpdateDefaults 内存回写缺省：update_time 零值取存储时钟。
// @param t 测试上下文
func TestReviewMemStoreUpdateDefaults(t *testing.T) {
	fx := newRVFixture(t)
	base := time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)
	rvSeedReview(fx, "u1", "carl", "dev", ReviewStatusPending, base)
	// update_time 零值、updater 为空：回写后更新时间取存储时钟、updater 保留原值
	if err := fx.reviews.Update(context.Background(), &ReviewTask{
		BaseColumns: BaseColumns{ID: "u1"},
		Status:      ReviewStatusDone,
		Opinion:     "同意",
	}); err != nil {
		t.Fatalf("回写失败: %v", err)
	}
	rt, found, _ := fx.reviews.Get(context.Background(), "u1")
	if !found || rt.Status != ReviewStatusDone || rt.Opinion != "同意" {
		t.Fatalf("回写结果不符: %+v", rt)
	}
	if rt.Updater != "" || !rt.UpdateTime.Equal(fx.fake.Now()) {
		t.Fatalf("缺省审计列不符: updater=%q update=%v", rt.Updater, rt.UpdateTime)
	}
}
