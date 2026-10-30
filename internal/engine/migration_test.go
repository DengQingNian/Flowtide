package engine

import (
	"context"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

const migrationV1 = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="migrate" name="V1" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="oldTask"/>
    <userTask id="oldTask" name="Old"/>
    <sequenceFlow id="f2" sourceRef="oldTask" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

const migrationV2 = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="migrate" name="V2" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="newTask"/>
    <userTask id="newTask" name="New"/>
    <sequenceFlow id="f2" sourceRef="newTask" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestMigrateProcessInstance 验证活动映射迁移。
// @param t 测试上下文
func TestMigrateProcessInstance(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, migrationV1)
	v1, _ := engine.GetProcessDefinitionByKey(context.Background(), "migrate")
	deployXML(t, engine, migrationV2)
	v2, _ := engine.GetProcessDefinitionByKey(context.Background(), "migrate")
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionID: v1.ID})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if err := engine.MigrateProcessInstance(context.Background(), api.MigrationRequest{
		ProcessInstanceID:         instance.ID,
		TargetProcessDefinitionID: v2.ID,
		ActivityMappings:          map[string]string{"oldTask": "newTask"},
	}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "newTask" {
		t.Fatalf("迁移后任务错误: %+v", tasks)
	}
	if tasks[0].ProcessDefinitionID != v2.ID {
		t.Fatalf("任务定义未更新")
	}
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatalf("迁移后完成任务失败: %v", err)
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended || got.ProcessDefinitionID != v2.ID {
		t.Fatalf("迁移后实例状态错误: %+v", got)
	}
}

// TestMigrate_SameActivityID 验证同名活动迁移。
// @param t 测试上下文
func TestMigrate_SameActivityID(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, migrationV1)
	v1, _ := engine.GetProcessDefinitionByKey(context.Background(), "migrate")
	deployXML(t, engine, migrationV1)
	v2, _ := engine.GetProcessDefinitionByKey(context.Background(), "migrate")
	instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionID: v1.ID})
	if err := engine.MigrateProcessInstance(context.Background(), api.MigrationRequest{
		ProcessInstanceID:         instance.ID,
		TargetProcessDefinitionID: v2.ID,
	}); err != nil {
		t.Fatalf("同名活动迁移失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].ProcessDefinitionID != v2.ID {
		t.Fatalf("同名迁移任务错误")
	}
}

// TestMigrate_Errors 验证迁移错误。
// @param t 测试上下文
func TestMigrate_Errors(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, migrationV1)
	v1, _ := engine.GetProcessDefinitionByKey(context.Background(), "migrate")
	deployXML(t, engine, migrationV2)
	v2, _ := engine.GetProcessDefinitionByKey(context.Background(), "migrate")
	instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionID: v1.ID})

	if err := engine.MigrateProcessInstance(context.Background(), api.MigrationRequest{ProcessInstanceID: "missing", TargetProcessDefinitionID: v2.ID}); err == nil {
		t.Fatalf("缺失实例应报错")
	}
	if err := engine.MigrateProcessInstance(context.Background(), api.MigrationRequest{ProcessInstanceID: instance.ID, TargetProcessDefinitionID: "missing"}); err == nil {
		t.Fatalf("缺失目标应报错")
	}
	if err := engine.MigrateProcessInstance(context.Background(), api.MigrationRequest{
		ProcessInstanceID:         instance.ID,
		TargetProcessDefinitionID: v2.ID,
		ActivityMappings:          map[string]string{"oldTask": "missing"},
	}); err == nil {
		t.Fatalf("目标活动缺失应报错")
	}
	if err := engine.MigrateProcessInstance(context.Background(), api.MigrationRequest{
		ProcessInstanceID:         instance.ID,
		TargetProcessDefinitionID: v2.ID,
		ActivityMappings:          map[string]string{"oldTask": "e"},
	}); err == nil {
		t.Fatalf("活动类型不兼容应报错")
	}
	engine.state.Definitions[v2.ID].Info.Suspended = true
	if err := engine.MigrateProcessInstance(context.Background(), api.MigrationRequest{ProcessInstanceID: instance.ID, TargetProcessDefinitionID: v2.ID}); err == nil {
		t.Fatalf("挂起目标应报错")
	}
}
