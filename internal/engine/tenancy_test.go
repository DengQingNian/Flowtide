package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/tenant"
)

// TestTenantChecks 验证跨租户访问被拒绝。
// @param t 测试上下文
func TestTenantChecks(t *testing.T) {
	engine := newTestEngine(t)
	engine.cfg.TenantID = "t1"
	if _, err := engine.Deploy(context.Background(), api.DeploymentRequest{
		TenantID:  "t1",
		Resources: map[string][]byte{"test.bpmn20.xml": []byte(simpleProcess)},
	}); err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple", TenantID: "t1",
		Variables: []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	task := listTasks(t, engine)[0]
	definition, _ := engine.GetProcessDefinitionByKey(context.Background(), "simple")
	deployments, _ := engine.ListDeployments(context.Background())
	other := tenant.With(context.Background(), "t2")
	if _, err := engine.GetProcessInstance(other, instance.ID); err == nil {
		t.Fatalf("跨租户实例查询应失败")
	}
	if _, err := engine.GetVariables(other, instance.ID); err == nil {
		t.Fatalf("跨租户变量查询应失败")
	}
	if err := engine.SetVariable(other, instance.ID, "x", 1); err == nil {
		t.Fatalf("跨租户变量设置应失败")
	}
	if err := engine.DeleteVariable(other, instance.ID, "x"); err == nil {
		t.Fatalf("跨租户变量删除应失败")
	}
	if err := engine.SuspendProcessInstance(other, instance.ID); err == nil {
		t.Fatalf("跨租户挂起应失败")
	}
	if _, err := engine.GetTask(other, task.ID); err == nil {
		t.Fatalf("跨租户任务查询应失败")
	}
	if err := engine.Complete(other, task.ID, api.CompleteTaskRequest{}); err == nil {
		t.Fatalf("跨租户完成任务应失败")
	}
	if err := engine.Claim(other, task.ID, "bob"); err == nil {
		t.Fatalf("跨租户签收应失败")
	}
	if err := engine.Delegate(other, task.ID, "bob"); err == nil {
		t.Fatalf("跨租户委派应失败")
	}
	if _, err := engine.GetTaskVariables(other, task.ID); err == nil {
		t.Fatalf("跨租户任务变量应失败")
	}
	if _, err := engine.AddComment(other, task.ID, "u", "m"); err == nil {
		t.Fatalf("跨租户评论应失败")
	}
	if _, err := engine.AddAttachment(other, task.ID, api.Attachment{Name: "x"}); err == nil {
		t.Fatalf("跨租户附件应失败")
	}
	if _, err := engine.GetProcessDefinition(other, definition.ID); err == nil {
		t.Fatalf("跨租户定义查询应失败")
	}
	if _, err := engine.GetProcessDefinitionXML(other, definition.ID); err == nil {
		t.Fatalf("跨租户定义 XML 应失败")
	}
	if _, err := engine.GetDeployment(other, deployments[0].ID); err == nil {
		t.Fatalf("跨租户部署查询应失败")
	}
	if _, err := engine.ListDeploymentResources(other, deployments[0].ID); err == nil {
		t.Fatalf("跨租户资源列表应失败")
	}
	if err := engine.DeleteDeployment(other, deployments[0].ID, true); err == nil {
		t.Fatalf("跨租户删除部署应失败")
	}
	if err := engine.MigrateProcessInstance(other, api.MigrationRequest{
		ProcessInstanceID: instance.ID, TargetProcessDefinitionID: definition.ID,
	}); err == nil {
		t.Fatalf("跨租户迁移应失败")
	}
}

// TestTenantHelpers 验证租户辅助函数。
// @param t 测试上下文
func TestTenantHelpers(t *testing.T) {
	engine := newTestEngine(t)
	ctx := tenant.With(context.Background(), "t1")
	if err := engine.allowTenant(ctx, "t1"); err != nil {
		t.Fatalf("同租户应通过: %v", err)
	}
	if err := engine.allowTenant(ctx, "t2"); err == nil {
		t.Fatalf("跨租户应失败")
	}
	if err := engine.allowTenant(context.Background(), "t2"); err == nil {
		t.Fatal("空租户不得访问非默认租户")
	}
	if err := engine.allowInstance(ctx, "missing"); err == nil {
		t.Fatalf("缺失实例应失败")
	}
	if err := engine.allowTask(ctx, "missing"); err == nil {
		t.Fatalf("缺失任务应失败")
	}
	if err := engine.allowDefinition(ctx, "missing"); err == nil {
		t.Fatalf("缺失定义应失败")
	}
}
