package engine

import (
	"context"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// TestDeploymentLifecycle 验证部署查询与删除。
// @param t 测试上下文
func TestDeploymentLifecycle(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	deployments, err := engine.ListDeployments(context.Background())
	if err != nil || len(deployments) != 1 {
		t.Fatalf("部署列表错误: %v %d", err, len(deployments))
	}
	deployment := deployments[0]
	if got, err := engine.GetDeployment(context.Background(), deployment.ID); err != nil || got.ID != deployment.ID {
		t.Fatalf("部署查询失败: %v", err)
	}
	resources, err := engine.ListDeploymentResources(context.Background(), deployment.ID)
	if err != nil || len(resources) != 1 {
		t.Fatalf("资源列表错误: %v %+v", err, resources)
	}
	data, err := engine.GetDeploymentResource(context.Background(), deployment.ID, resources[0])
	if err != nil || len(data) == 0 {
		t.Fatalf("资源读取失败: %v", err)
	}
	if err := engine.DeleteDeployment(context.Background(), deployment.ID, false); err == nil {
		t.Fatalf("未级联删除应失败")
	}
	if err := engine.DeleteDeployment(context.Background(), deployment.ID, true); err != nil {
		t.Fatalf("级联删除失败: %v", err)
	}
	if _, err := engine.GetDeployment(context.Background(), deployment.ID); err == nil {
		t.Fatalf("删除后部署应不存在")
	}
	definitions, _ := engine.ListProcessDefinitions(context.Background())
	if len(definitions) != 0 {
		t.Fatalf("级联删除后定义应清空")
	}
}

// TestDeployment_RunningInstance 验证运行实例保护。
// @param t 测试上下文
func TestDeployment_RunningInstance(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	deployment := mustDeployment(t, engine)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if err := engine.DeleteDeployment(context.Background(), deployment.ID, true); err == nil {
		t.Fatalf("运行实例存在时应拒绝删除")
	}
}

// TestDeployment_Errors 验证部署错误。
// @param t 测试上下文
func TestDeployment_Errors(t *testing.T) {
	engine := newTestEngine(t)
	if _, err := engine.GetDeployment(context.Background(), "missing"); err == nil {
		t.Fatalf("缺失部署应报错")
	}
	if _, err := engine.ListDeploymentResources(context.Background(), "missing"); err == nil {
		t.Fatalf("缺失资源列表应报错")
	}
	if _, err := engine.GetDeploymentResource(context.Background(), "missing", "x"); err == nil {
		t.Fatalf("缺失资源应报错")
	}
	if err := engine.DeleteDeployment(context.Background(), "missing", true); err == nil {
		t.Fatalf("缺失部署删除应报错")
	}
	deployXML(t, engine, simpleProcess)
	deployment := mustDeployment(t, engine)
	if _, err := engine.GetDeploymentResource(context.Background(), deployment.ID, "missing"); err == nil {
		t.Fatalf("缺失资源内容应报错")
	}
}

// mustDeployment 返回唯一部署。
// @param t 测试上下文
// @param engine 引擎
// @return 部署
func mustDeployment(t *testing.T, engine *Engine) api.Deployment {
	t.Helper()
	deployments, err := engine.ListDeployments(context.Background())
	if err != nil || len(deployments) == 0 {
		t.Fatalf("查询部署失败: %v", err)
	}
	return deployments[0]
}

// TestDeployment_NoDefinitions 验证无流程定义的部署删除。
// @param t 测试上下文
func TestDeployment_NoDefinitions(t *testing.T) {
	engine := newTestEngine(t)
	_, err := engine.Deploy(context.Background(), api.DeploymentRequest{
		Name:      "resource-only",
		Resources: map[string][]byte{"readme.txt": []byte("hello")},
	})
	if err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	deployment := mustDeployment(t, engine)
	if err := engine.DeleteDeployment(context.Background(), deployment.ID, false); err != nil {
		t.Fatalf("无定义部署应可直接删除: %v", err)
	}
}

// TestDeployment_VersionRecalc 验证删除后版本重算。
// @param t 测试上下文
func TestDeployment_VersionRecalc(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	first := mustDeployment(t, engine)
	deployXML(t, engine, simpleProcess)
	deployments, _ := engine.ListDeployments(context.Background())
	second := deployments[len(deployments)-1]
	if err := engine.DeleteDeployment(context.Background(), second.ID, true); err != nil {
		t.Fatalf("删除最新部署失败: %v", err)
	}
	definition, err := engine.GetProcessDefinitionByKey(context.Background(), "simple")
	if err != nil || definition.Version != 1 {
		t.Fatalf("删除后应保留 v1: %v %+v", err, definition)
	}
	if err := engine.DeleteDeployment(context.Background(), first.ID, true); err != nil {
		t.Fatalf("删除首个部署失败: %v", err)
	}
}
