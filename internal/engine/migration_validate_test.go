package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// TestValidateMigration 验证迁移预检。
// @param t 测试上下文
func TestValidateMigration(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, migrationV1)
	v1, _ := engine.GetProcessDefinitionByKey(context.Background(), "migrate")
	deployXML(t, engine, migrationV2)
	v2, _ := engine.GetProcessDefinitionByKey(context.Background(), "migrate")
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionID: v1.ID})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	messages, err := engine.ValidateMigration(context.Background(), api.MigrationRequest{
		ProcessInstanceID:         instance.ID,
		TargetProcessDefinitionID: v2.ID,
		ActivityMappings:          map[string]string{"oldTask": "newTask"},
	})
	if err != nil || len(messages) != 0 {
		t.Fatalf("合法迁移预检失败: %v %+v", err, messages)
	}
	if tasks := listTasks(t, engine); len(tasks) != 1 || tasks[0].TaskDefinitionKey != "oldTask" {
		t.Fatalf("预检不应修改状态: %+v", tasks)
	}
	messages, err = engine.ValidateMigration(context.Background(), api.MigrationRequest{
		ProcessInstanceID:         instance.ID,
		TargetProcessDefinitionID: v2.ID,
		ActivityMappings:          map[string]string{"oldTask": "missing"},
	})
	if err != nil || len(messages) == 0 {
		t.Fatalf("非法映射应返回消息: %v %+v", err, messages)
	}
	if _, err := engine.ValidateMigration(context.Background(), api.MigrationRequest{
		ProcessInstanceID:         "missing",
		TargetProcessDefinitionID: v2.ID,
	}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失实例应 404: %v", err)
	}
	if _, err := engine.ValidateMigration(context.Background(), api.MigrationRequest{
		ProcessInstanceID:         instance.ID,
		TargetProcessDefinitionID: "missing",
	}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失目标应 404: %v", err)
	}

	if err := engine.MigrateProcessInstance(context.Background(), api.MigrationRequest{
		ProcessInstanceID:         instance.ID,
		TargetProcessDefinitionID: v2.ID,
		ActivityMappings:          map[string]string{"oldTask": "newTask"},
	}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	if tasks := listTasks(t, engine); len(tasks) != 1 || tasks[0].TaskDefinitionKey != "newTask" {
		t.Fatalf("迁移后任务错误: %+v", tasks)
	}
}

// TestValidateMigrationWithJob 验证存在作业时的预检消息。
// @param t 测试上下文
func TestValidateMigrationWithJob(t *testing.T) {
	engine := newTestEngine(t)
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="jobMigrate" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="task"/><userTask id="task" name="Task" flowable:assignee="alice"/><boundaryEvent id="timer" attachedToRef="task"><timerEventDefinition><timeDuration>PT1H</timeDuration></timerEventDefinition></boundaryEvent><sequenceFlow id="f2" sourceRef="task" targetRef="e"/><sequenceFlow id="f3" sourceRef="timer" targetRef="e"/><endEvent id="e"/></process></definitions>`
	deployXML(t, engine, xml)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "jobMigrate"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	target, _ := engine.GetProcessDefinitionByKey(context.Background(), "jobMigrate")
	messages, err := engine.ValidateMigration(context.Background(), api.MigrationRequest{
		ProcessInstanceID:         instance.ID,
		TargetProcessDefinitionID: target.ID,
	})
	if err != nil || len(messages) == 0 {
		t.Fatalf("存在作业应返回消息: %v %+v", err, messages)
	}
	if err := engine.MigrateProcessInstance(context.Background(), api.MigrationRequest{
		ProcessInstanceID:         instance.ID,
		TargetProcessDefinitionID: target.ID,
	}); err == nil {
		t.Fatalf("存在作业时迁移应失败")
	}
}
