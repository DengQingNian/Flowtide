package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
)

// newMigrationFixture 创建迁移测试环境。
// @param t 测试上下文
// @return 引擎
// @return 实例
// @return 目标定义 ID
func newMigrationFixture(t *testing.T) (*Engine, api.ProcessInstance, string) {
	t.Helper()
	engine := newTestEngine(t)
	deployXML(t, engine, migrationV1)
	v1, _ := engine.GetProcessDefinitionByKey(context.Background(), "migrate")
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionID: v1.ID})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	deployXML(t, engine, migrationV2)
	v2, _ := engine.GetProcessDefinitionByKey(context.Background(), "migrate")
	return engine, instance, v2.ID
}

// TestMigrate_MoreErrors 验证迁移边界错误。
// @param t 测试上下文
func TestMigrate_MoreErrors(t *testing.T) {
	t.Run("已结束实例", func(t *testing.T) {
		engine, instance, targetID := newMigrationFixture(t)
		task := listTasks(t, engine)[0]
		if err := engine.Complete(context.Background(), task.ID, api.CompleteTaskRequest{}); err != nil {
			t.Fatalf("完成任务失败: %v", err)
		}
		if err := engine.MigrateProcessInstance(context.Background(), api.MigrationRequest{ProcessInstanceID: instance.ID, TargetProcessDefinitionID: targetID}); err == nil {
			t.Fatalf("已结束实例应报错")
		}
	})
	t.Run("租户不一致", func(t *testing.T) {
		engine := newTestEngine(t)
		_, _ = engine.Deploy(context.Background(), api.DeploymentRequest{
			Name: "v1", TenantID: "a", Resources: map[string][]byte{"v1.bpmn20.xml": []byte(migrationV1)},
		})
		v1, _ := engine.GetProcessDefinitionByKey(context.Background(), "migrate")
		instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionID: v1.ID, TenantID: "a"})
		_, _ = engine.Deploy(context.Background(), api.DeploymentRequest{
			Name: "v2", TenantID: "b", Resources: map[string][]byte{"v2.bpmn20.xml": []byte(migrationV2)},
		})
		var targetID string
		for id, definition := range engine.state.Definitions {
			if definition.Info.TenantID == "b" {
				targetID = id
			}
		}
		if err := engine.MigrateProcessInstance(context.Background(), api.MigrationRequest{ProcessInstanceID: instance.ID, TargetProcessDefinitionID: targetID}); err == nil {
			t.Fatalf("跨租户迁移应报错")
		}
	})
	t.Run("存在作业", func(t *testing.T) {
		engine, instance, targetID := newMigrationFixture(t)
		engine.state.Jobs["j"] = &jobState{ID: "j", ProcessInstanceID: instance.ID}
		if err := engine.MigrateProcessInstance(context.Background(), api.MigrationRequest{ProcessInstanceID: instance.ID, TargetProcessDefinitionID: targetID}); err == nil {
			t.Fatalf("存在作业应报错")
		}
	})
	t.Run("存在订阅", func(t *testing.T) {
		engine, instance, targetID := newMigrationFixture(t)
		engine.state.Subscriptions["s"] = &subscriptionState{ID: "s", ProcessInstanceID: instance.ID}
		if err := engine.MigrateProcessInstance(context.Background(), api.MigrationRequest{ProcessInstanceID: instance.ID, TargetProcessDefinitionID: targetID}); err == nil {
			t.Fatalf("存在订阅应报错")
		}
	})
	t.Run("源定义缺失", func(t *testing.T) {
		engine, instance, targetID := newMigrationFixture(t)
		delete(engine.state.Definitions, instance.ProcessDefinitionID)
		if err := engine.MigrateProcessInstance(context.Background(), api.MigrationRequest{ProcessInstanceID: instance.ID, TargetProcessDefinitionID: targetID}); err == nil {
			t.Fatalf("源定义缺失应报错")
		}
	})
	t.Run("目标无流程", func(t *testing.T) {
		engine, instance, targetID := newMigrationFixture(t)
		engine.state.Definitions[targetID].Model = &model.BpmnModel{}
		if err := engine.MigrateProcessInstance(context.Background(), api.MigrationRequest{ProcessInstanceID: instance.ID, TargetProcessDefinitionID: targetID}); err == nil {
			t.Fatalf("目标无流程应报错")
		}
	})
	t.Run("无活跃执行", func(t *testing.T) {
		engine, instance, targetID := newMigrationFixture(t)
		for _, execution := range engine.state.Executions {
			if execution.ProcessInstanceID == instance.ID {
				execution.Ended = true
			}
		}
		if err := engine.MigrateProcessInstance(context.Background(), api.MigrationRequest{ProcessInstanceID: instance.ID, TargetProcessDefinitionID: targetID}); err == nil {
			t.Fatalf("无活跃执行应报错")
		}
	})
	t.Run("源活动缺失", func(t *testing.T) {
		engine, instance, targetID := newMigrationFixture(t)
		for _, execution := range engine.state.Executions {
			if execution.ProcessInstanceID == instance.ID {
				execution.ActivityID = "missing"
			}
		}
		if err := engine.MigrateProcessInstance(context.Background(), api.MigrationRequest{ProcessInstanceID: instance.ID, TargetProcessDefinitionID: targetID}); err == nil {
			t.Fatalf("源活动缺失应报错")
		}
	})
}
