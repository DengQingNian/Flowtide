package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
	"github.com/DengQingNian/Flowtide/internal/engine"
	"github.com/DengQingNian/Flowtide/internal/idgen"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// facadeFixture 是两级审批线性流程语料（发起人 → 审批 → 结束）。
const facadeFixture = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="facadeProc" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="apply"/>
    <userTask id="apply" name="申请" flowable:assignee="${initiator}" stepCode="S00"/>
    <sequenceFlow id="f2" sourceRef="apply" targetRef="approve"/>
    <userTask id="approve" name="审批" stepCode="S01"/>
    <sequenceFlow id="f3" sourceRef="approve" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// newFacadeService 构造门面测试环境：引擎 + 语料部署 + 钩子注册 + 预置身份。
// @param t 测试上下文
// @return 服务
// @return 引擎
func newFacadeService(t *testing.T) (*Service, api.Engine) {
	t.Helper()
	eng, err := engine.New(engine.Config{
		HistoryLevel: engine.HistoryAudit,
		Clock:        clock.NewFake(time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC)),
		IDs:          idgen.NewUUIDv7(),
	})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close(context.Background()) })
	// 预置发起人与审批人身份（意见/待办姓名补全依赖）
	_, _ = eng.IdentityService().CreateUser(context.Background(), api.User{ID: "u1", FirstName: "张", LastName: "三"})
	_, _ = eng.IdentityService().CreateUser(context.Background(), api.User{ID: "u2", FirstName: "李", LastName: "四"})
	if _, err := eng.RepositoryService().Deploy(tenant.With(context.Background(), "bizA"), api.DeploymentRequest{
		Name:      "facade",
		TenantID:  "bizA",
		Resources: map[string][]byte{"facade.bpmn20.xml": []byte(facadeFixture)},
	}); err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	service := NewWorkflowService(eng)
	// 注册钩子用租户上下文（WarmupAll 按租户过滤定义；生产环境由租户中间件保证）
	if err := service.RegisterExecutorRewriteHook(tenant.With(context.Background(), "bizA")); err != nil {
		t.Fatalf("注册钩子失败: %v", err)
	}
	return service, eng
}

// TestFacadeStartCompleteFullLoop 验证门面发起→预存→办结→待办已办→表单状态全链路。
// @param t 测试上下文
func TestFacadeStartCompleteFullLoop(t *testing.T) {
	service, _ := newFacadeService(t)
	ctx := tenant.With(context.Background(), "bizA")

	// 发起
	started, err := service.StartProcess(ctx, &StartProcessInstanceVo{
		ProcessDefinitionKey: "facadeProc", Title: "请假申请", Operator: "u1",
		TenantID: "bizA", BusinessKey: "BIZ-100",
	})
	if err != nil {
		t.Fatalf("发起失败: %v", err)
	}
	if !started.Success || started.Data == nil {
		t.Fatalf("发起返回异常: %+v", started)
	}
	data := started.Data.(map[string]any)
	taskID, _ := data["taskId"].(string)
	instanceID, _ := data["processInstanceId"].(string)
	if taskID == "" || instanceID == "" {
		t.Fatalf("发起结果缺失: %+v", data)
	}

	// 部署判定与运行实例判定
	deployed, err := service.IsProcessDeployed(ctx, "bizA", "facadeProc")
	if err != nil || deployed.Data != true {
		t.Fatalf("部署判定错误: %+v err=%v", deployed, err)
	}
	running, err := service.IsExistRunningProcess(ctx, "bizA", "facadeProc", "BIZ-100")
	if err != nil || running.Data != true {
		t.Fatalf("运行判定错误: %+v err=%v", running, err)
	}

	// 办结首任务（Java 协议：Request 携带下一步预存，注入 operationType 门控）
	completed, err := service.Complete(ctx, &CompleteTaskVo{
		BaseProcessVo: BaseProcessVo{
			TaskID: taskID, UserCode: "u1", Message: "同意", ProcessInstanceID: instanceID, Type: CommentTypeSP,
		},
		Variables: map[string]any{"operationType": "agreeComplete"},
		Request: &NextActivityRequest{
			TaskID: taskID,
			NextActivityList: []*NextActivityVo{
				{ActivityID: "approve", ExecutorIDs: []string{"u2"}},
			},
			SendIdentityID: "u1", SendUserName: "张三",
		},
	})
	if err != nil {
		t.Fatalf("办结失败: %v", err)
	}
	result := completed.Data.(map[string]any)
	nextIDs, _ := result["nextTaskIds"].([]string)
	if result["finished"] != false || len(nextIDs) != 1 {
		t.Fatalf("办结结果异常: %+v", result)
	}

	// 执行人重写：审批任务办理人应为预存的 u2
	task, err := service.engine.TaskService().GetTask(ctx, nextIDs[0])
	if err != nil {
		t.Fatalf("查询下一任务失败: %v", err)
	}
	if task.Assignee != "u2" {
		t.Fatalf("执行人重写未生效: %q", task.Assignee)
	}

	// 待办/已办
	todo, err := service.GetTodoTaskListByIdentityID(ctx, "u2", PageQuery{Size: 10})
	if err != nil {
		t.Fatalf("待办查询失败: %v", err)
	}
	todoPage := todo.Data.(PageResult[*TodoTaskView])
	if len(todoPage.Data) != 1 {
		t.Fatalf("u2 待办数量错误: %d", len(todoPage.Data))
	}
	done, err := service.GetDoneTaskListByIdentityID(ctx, "u1", PageQuery{Size: 10})
	if err != nil {
		t.Fatalf("已办查询失败: %v", err)
	}
	donePage := done.Data.(PageResult[*DoneTaskView])
	if len(donePage.Data) != 1 {
		t.Fatalf("u1 已办数量错误: %d", len(donePage.Data))
	}

	// 表单状态（流转中聚合待办人）
	status, err := service.GetWorkflowStatusByBusinessID(ctx, "bizA", "facadeProc", "BIZ-100")
	if err != nil {
		t.Fatalf("状态查询失败: %v", err)
	}
	statusData := status.Data.(map[string]any)
	if statusData["flowStatus"] != FlowStatusRunning {
		t.Fatalf("流转状态错误: %+v", statusData)
	}

	// 消息/历史列表与任务详情
	messages, err := service.GetTaskMessageListByBusinessID(ctx, "bizA", "BIZ-100")
	if err != nil || len(messages.Data.([]*MessageExtend)) != 1 {
		t.Fatalf("消息列表异常: err=%v", err)
	}
	histories, err := service.GetTaskHistoryListByBusinessID(ctx, "bizA", "BIZ-100")
	// 发起镜像 1 条 + 办结生成的新待办镜像 1 条
	if err != nil || len(histories.Data.([]*HistoryExtend)) != 2 {
		t.Fatalf("历史列表异常: err=%v", err)
	}
	info, err := service.GetTaskInfoByTaskID(ctx, nextIDs[0])
	if err != nil || info.Data == nil {
		t.Fatalf("任务详情异常: err=%v", err)
	}
}

// TestFacadeVariablesAndDocument 验证全局变量门面与表单回写。
// @param t 测试上下文
func TestFacadeVariablesAndDocument(t *testing.T) {
	service, _ := newFacadeService(t)
	ctx := tenant.With(context.Background(), "bizA")
	started, err := service.StartProcess(ctx, &StartProcessInstanceVo{
		ProcessDefinitionKey: "facadeProc", Title: "变量流程", Operator: "u1",
		TenantID: "bizA", BusinessKey: "BIZ-200",
	})
	if err != nil {
		t.Fatalf("发起失败: %v", err)
	}
	instanceID := started.Data.(map[string]any)["processInstanceId"].(string)

	// 变量写读
	if _, err := service.UpdateProcessGlobalVariable(ctx, instanceID, map[string]any{"k1": "v1", "entity": map[string]any{"amount": 3}}); err != nil {
		t.Fatalf("变量写入失败: %v", err)
	}
	variables, err := service.GetProcessGlobalVariableByProcessInstanceID(ctx, instanceID, []string{"k1", "missing"})
	if err != nil {
		t.Fatalf("变量读取失败: %v", err)
	}
	varData := variables.Data.(map[string]any)
	if varData["k1"] != "v1" {
		t.Fatalf("变量值错误: %+v", varData)
	}
	if _, exists := varData["missing"]; exists {
		t.Fatalf("未请求变量不应返回")
	}
	byBusiness, err := service.GetProcessGlobalVariableByBusinessID(ctx, "bizA", "BIZ-200", nil)
	if err != nil {
		t.Fatalf("按业务键读变量失败: %v", err)
	}
	if byBusiness.Data.(map[string]any)["k1"] != "v1" {
		t.Fatalf("按业务键变量值错误: %+v", byBusiness.Data)
	}

	// 表单状态回写（Draft 开始时间已在发起时记录，此处刷新不报错即可）
	if _, err := service.UpdateWorkflowDocumentByProcessInstanceIDOrTaskID(ctx, instanceID, "", nil); err != nil {
		t.Fatalf("表单回写失败: %v", err)
	}

	// 未部署定义判定
	deployed, err := service.IsProcessDeployed(ctx, "bizA", "noSuchKey")
	if err != nil || deployed.Data != false {
		t.Fatalf("未部署判定错误: %+v err=%v", deployed, err)
	}
	// 无实例业务键判定
	running, err := service.IsExistRunningProcess(ctx, "bizA", "facadeProc", "BIZ-404")
	if err != nil || running.Data != false {
		t.Fatalf("无实例判定错误: %+v err=%v", running, err)
	}
}

// TestFacadeValidationError 验证门面参数校验。
// @param t 测试上下文
func TestFacadeValidationError(t *testing.T) {
	service, _ := newFacadeService(t)
	ctx := tenant.With(context.Background(), "bizA")
	if _, err := service.StartProcess(ctx, nil); err == nil {
		t.Fatalf("空发起请求应报错")
	}
	if _, err := service.Complete(ctx, nil); err == nil {
		t.Fatalf("空办结请求应报错")
	}
	if _, err := service.SaveAgreeNextActivity(ctx, &NextActivityRequest{TaskID: "t"}); err == nil {
		t.Fatalf("空下一步列表应报错")
	}
	if _, err := service.IsProcessDeployed(ctx, "bizA", ""); err == nil {
		t.Fatalf("空定义 Key 应报错")
	}
	if _, err := service.IsExistRunningProcess(ctx, "bizA", "k", ""); err == nil {
		t.Fatalf("空业务键应报错")
	}
	if _, err := service.GetWorkflowStatusByBusinessID(ctx, "bizA", "k", "BIZ-NONE"); err == nil {
		t.Fatalf("未关联单据应报错")
	}
	if _, err := service.UpdateProcessGlobalVariable(ctx, "", map[string]any{}); err == nil {
		t.Fatalf("空实例 ID 应报错")
	}
	if _, err := service.UpdateWorkflowDocumentByProcessInstanceIDOrTaskID(ctx, "", "", nil); err == nil {
		t.Fatalf("实例与任务全空应报错")
	}
	if _, err := service.GetProcessGlobalVariableByBusinessID(ctx, "bizA", "BIZ-NONE", nil); err == nil {
		t.Fatalf("无实例业务键读变量应报错")
	}
}
