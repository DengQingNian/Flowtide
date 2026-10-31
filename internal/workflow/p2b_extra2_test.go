// p2b_extra2_test.go 终止域入参校验分支与镜像创建错误传播补齐测试（P2 波2-B）。
// 覆盖：各服务空参拒绝、无业务键实例的删除/重激活防御分支、预存历史重复节点去重、
// 未注入单据端口的业务键清扫跳过、待办/已办镜像创建失败与删除链错误传播。
package workflow

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
)

// TestTerminateParamGuardsAndBranches 验证终止域各入参校验分支与防御分支。
// @param t 测试上下文
func TestTerminateParamGuardsAndBranches(t *testing.T) {
	fx := newTerminateFixture(t)
	ctx := context.Background()
	instance, _ := trmStart(t, fx, "biz-params")
	// StopByInstance 空操作人
	if err := fx.terminate.StopByInstance(ctx, instance.ID, " ", ""); err == nil || !strings.Contains(err.Error(), "identityId") {
		t.Fatalf("空操作人应拒绝: %v", err)
	}
	// DeleteInstance 空实例 / 空操作人
	if err := fx.terminate.DeleteInstance(ctx, " ", "carl", ""); err == nil || !strings.Contains(err.Error(), "processInstanceId") {
		t.Fatalf("空实例 Id 应拒绝: %v", err)
	}
	if err := fx.terminate.DeleteInstance(ctx, instance.ID, " ", ""); err == nil || !strings.Contains(err.Error(), "identityId") {
		t.Fatalf("空操作人应拒绝: %v", err)
	}
	// Suspend / Activate 空操作人
	if err := fx.terminate.Suspend(ctx, instance.ID, " "); err == nil || !strings.Contains(err.Error(), "identityId") {
		t.Fatalf("挂起空操作人应拒绝: %v", err)
	}
	if err := fx.terminate.Activate(ctx, instance.ID, " "); err == nil || !strings.Contains(err.Error(), "identityId") {
		t.Fatalf("激活空操作人应拒绝: %v", err)
	}
	// Reactivate 空历史实例 / 空操作人
	if err := fx.terminate.Reactivate(ctx, " ", "", "carl"); err == nil || !strings.Contains(err.Error(), "processInstanceId") {
		t.Fatalf("重激活空实例应拒绝: %v", err)
	}
	if err := fx.terminate.Reactivate(ctx, instance.ID, "", " "); err == nil || !strings.Contains(err.Error(), "identityId") {
		t.Fatalf("重激活空操作人应拒绝: %v", err)
	}
	// 无业务键实例重激活：document 重建业务键缺失分支跳过
	noBiz, err := fx.engine.RuntimeService().StartProcessInstance(ctx, api.StartRequest{
		ProcessDefinitionKey: "trmFlow",
		Name:                 "无业务键重激活",
		TenantID:             "dev",
		UserID:               "alice",
	})
	if err != nil {
		t.Fatalf("发起失败: %v", err)
	}
	trmComplete(t, fx, trmFirstTask(t, fx, noBiz.ID).ID, "alice")
	trmComplete(t, fx, trmFirstTask(t, fx, noBiz.ID).ID, "alice")
	if err := fx.terminate.Reactivate(ctx, noBiz.ID, "", "carl"); err != nil {
		t.Fatalf("无业务键重激活应成功: %v", err)
	}
	// 预存历史重复节点去重（continue 分支）：同节点两条历史记录归并一个清理键
	dup, dupTask := trmStart(t, fx, "biz-dupnext")
	trmSeedInstanceData(t, fx, dup, dupTask)
	if err := fx.next.InsertDual(ctx, &NextActivity{
		BaseColumns:       BaseColumns{TenantID: dup.TenantID},
		NextActivityID:    "B",
		IdentityID:        "carl",
		ProcessInstanceID: dup.ID,
		Used:              0,
		PreTaskID:         dupTask.ID,
		Version:           2,
	}, "alice"); err != nil {
		t.Fatalf("播种重复预存失败: %v", err)
	}
	fx.fakeClock.Advance(time.Minute)
	if err := fx.terminate.StopByInstance(ctx, dup.ID, "carl", ""); err != nil {
		t.Fatalf("重复预存实例终止失败: %v", err)
	}
	hisRecords, _ := fx.next.ListHisByInstance(ctx, dup.ID)
	if len(hisRecords) != 0 {
		t.Fatalf("重复节点历史预存应全清: %+v", hisRecords)
	}
	// DeleteByBusinessID 未注入单据端口：document 逻辑删除跳过（nil 分支）
	plainSvc := NewTerminateService(fx.engine, fx.next, fx.comments, fx.operations, fx.messages, fx.histories, fx.documents, fx.fakeClock)
	if err := plainSvc.DeleteByBusinessID(ctx, "dev", "biz-none-2", "carl"); err != nil {
		t.Fatalf("无实例业务键清扫应成功: %v", err)
	}
}

// TestTerminateCreateTodoStoreErrors 验证待办/已办镜像创建失败与删除链错误传播。
// @param t 测试上下文
func TestTerminateCreateTodoStoreErrors(t *testing.T) {
	fx := newTerminateFixture(t)
	ctx := context.Background()
	// 待办 Save 失败（重激活链 createInstanceTaskTodo → messages.CreateForTask）
	finishAndReactivate := func(svc *TerminateService) error {
		instance, taskA := trmStart(t, fx, "biz-todoerr")
		trmComplete(t, fx, taskA.ID, "alice")
		trmComplete(t, fx, trmFirstTask(t, fx, instance.ID).ID, "alice")
		return svc.Reactivate(ctx, instance.ID, "", "carl")
	}
	msgFail := NewMessageExtendService(&p2bFailingMessageStore{failSave: true, MessageExtendStore: fx.stores.Message}, fx.fakeClock)
	svc := NewTerminateService(fx.engine, fx.next, fx.comments, fx.operations, msgFail, fx.histories, fx.documents, fx.fakeClock)
	if err := finishAndReactivate(svc); err == nil {
		t.Fatalf("待办创建失败应传播错误")
	}
	// 已办镜像 Save 失败
	hisFail := NewHistoryExtendService(&p2bFailingHistoryStore{failSave: true, HistoryExtendStore: fx.stores.History}, fx.fakeClock)
	svc = NewTerminateService(fx.engine, fx.next, fx.comments, fx.operations, fx.messages, hisFail, fx.documents, fx.fakeClock)
	if err := finishAndReactivate(svc); err == nil {
		t.Fatalf("已办镜像创建失败应传播错误")
	}
	// DeleteInstance 预存删除失败传播
	instance, taskA := trmStart(t, fx, "biz-delnext")
	trmSeedInstanceData(t, fx, instance, taskA)
	nextFail := NewNextActivityService(&p2bFailingNextStore{failDeleteByInstance: true, NextActivityStore: fx.stores.Next}, fx.fakeClock)
	svc = NewTerminateService(fx.engine, nextFail, fx.comments, fx.operations, fx.messages, fx.histories, fx.documents, fx.fakeClock, WithTerminateDocumentStore(fx.stores.Document))
	if err := svc.DeleteInstance(ctx, instance.ID, "carl", ""); err == nil {
		t.Fatalf("删除预存失败应传播错误")
	}
	// DeleteByBusinessID 逐实例删除失败传播（循环内 DeleteInstance 错误）
	instance2, taskB2 := trmStart(t, fx, "biz-batcherr")
	trmSeedInstanceData(t, fx, instance2, taskB2)
	svc = NewTerminateService(fx.engine, nextFail, fx.comments, fx.operations, fx.messages, fx.histories, fx.documents, fx.fakeClock, WithTerminateDocumentStore(fx.stores.Document))
	if err := svc.DeleteByBusinessID(ctx, "dev", "biz-batcherr", "carl"); err == nil {
		t.Fatalf("批量删除内实例删除失败应传播错误")
	}
	// DeleteByBusinessID 已办清扫失败传播
	hisBizFail := NewHistoryExtendService(&p2bFailingHistoryStore{failDeleteBusiness: true, HistoryExtendStore: fx.stores.History}, fx.fakeClock)
	svc = NewTerminateService(fx.engine, fx.next, fx.comments, fx.operations, fx.messages, hisBizFail, fx.documents, fx.fakeClock, WithTerminateDocumentStore(fx.stores.Document))
	if err := svc.DeleteByBusinessID(ctx, "dev", "biz-none-3", "carl"); err == nil {
		t.Fatalf("业务键已办清扫失败应传播错误")
	}
}
