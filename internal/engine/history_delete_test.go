package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
)

// TestDeleteHistoric 验证历史记录删除。
// @param t 测试上下文
func TestDeleteHistoric(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	task := listTasks(t, engine)[0]
	if _, err := engine.AddProcessComment(context.Background(), instance.ID, "u", "p"); err != nil {
		t.Fatalf("添加流程评论失败: %v", err)
	}
	if _, err := engine.AddProcessAttachment(context.Background(), instance.ID, api.Attachment{Name: "p"}); err != nil {
		t.Fatalf("添加流程附件失败: %v", err)
	}
	if _, err := engine.AddComment(context.Background(), task.ID, "u", "t"); err != nil {
		t.Fatalf("添加任务评论失败: %v", err)
	}
	if _, err := engine.AddAttachment(context.Background(), task.ID, api.Attachment{Name: "t"}); err != nil {
		t.Fatalf("添加任务附件失败: %v", err)
	}
	if err := engine.Complete(context.Background(), task.ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	if err := engine.DeleteHistoricProcessInstance(context.Background(), instance.ID); err != nil {
		t.Fatalf("删除历史流程失败: %v", err)
	}
	if _, err := engine.GetHistoricProcessInstance(context.Background(), instance.ID); err == nil {
		t.Fatalf("历史流程应删除")
	}
	if len(engine.state.HistoricTasks) != 0 || len(engine.state.HistoricActivities) != 0 || len(engine.state.HistoricVariables) != 0 ||
		len(engine.state.HistoricLinks) != 0 || len(engine.state.Comments) != 0 || len(engine.state.Attachments) != 0 || len(engine.state.TaskLogs) != 0 {
		t.Fatalf("历史关联记录未清理")
	}
	if err := engine.DeleteHistoricProcessInstance(context.Background(), "missing"); err == nil {
		t.Fatalf("缺失历史流程删除应失败")
	}
}

// TestDeleteHistoricDetailAndLog 验证明细与日志删除。
// @param t 测试上下文
func TestDeleteHistoricDetailAndLog(t *testing.T) {
	engine := newTestEngine(t)
	engine.cfg.HistoryLevel = HistoryFull
	deployXML(t, engine, simpleProcess)
	instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	details, _ := engine.ListHistoricDetails(context.Background(), instance.ID)
	if len(details) == 0 {
		t.Fatalf("明细为空")
	}
	if err := engine.DeleteHistoricDetail(context.Background(), details[0].ID); err != nil {
		t.Fatalf("删除明细失败: %v", err)
	}
	logs, _ := engine.ListHistoricTaskLogs(context.Background(), "")
	if len(logs) == 0 {
		t.Fatalf("日志为空")
	}
	if err := engine.DeleteHistoricTaskLog(context.Background(), logs[0].ID); err != nil {
		t.Fatalf("删除日志失败: %v", err)
	}
	if err := engine.DeleteHistoricDetail(context.Background(), "missing"); err == nil {
		t.Fatalf("缺失明细删除应失败")
	}
	if err := engine.DeleteHistoricTaskLog(context.Background(), 9999); err == nil {
		t.Fatalf("缺失日志删除应失败")
	}
}

// TestDeleteHistoricTask 验证历史任务删除。
// @param t 测试上下文
func TestDeleteHistoricTask(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	_, _ = engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	task := listTasks(t, engine)[0]
	if err := engine.Complete(context.Background(), task.ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	if err := engine.DeleteHistoricTaskInstance(context.Background(), task.ID); err != nil {
		t.Fatalf("删除历史任务失败: %v", err)
	}
	if _, err := engine.GetHistoricTaskInstance(context.Background(), task.ID); err == nil {
		t.Fatalf("历史任务应删除")
	}
	if err := engine.DeleteHistoricTaskInstance(context.Background(), "missing"); err == nil {
		t.Fatalf("缺失历史任务删除应失败")
	}
}

// TestHistoricDetailLookup 验证历史活动与变量详情。
// @param t 测试上下文
func TestHistoricDetailLookup(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	activities, _ := engine.ListHistoricActivityInstances(context.Background(), instance.ID)
	if len(activities) == 0 {
		t.Fatalf("历史活动为空")
	}
	if activity, err := engine.GetHistoricActivityInstance(context.Background(), activities[0].ID); err != nil || activity.ID != activities[0].ID {
		t.Fatalf("历史活动查询失败: %v", err)
	}
	if _, err := engine.GetHistoricActivityInstance(context.Background(), "missing"); err == nil {
		t.Fatalf("缺失历史活动应失败")
	}
	variables, _ := engine.ListHistoricVariableInstances(context.Background(), instance.ID)
	if len(variables) == 0 {
		t.Fatalf("历史变量为空")
	}
	if variable, err := engine.GetHistoricVariableInstance(context.Background(), variables[0].ID); err != nil || variable.ID != variables[0].ID {
		t.Fatalf("历史变量查询失败: %v", err)
	}
	if err := engine.DeleteHistoricVariableInstance(context.Background(), variables[0].ID); err != nil {
		t.Fatalf("删除历史变量失败: %v", err)
	}
	if _, err := engine.GetHistoricVariableInstance(context.Background(), variables[0].ID); err == nil {
		t.Fatalf("删除后历史变量应不存在")
	}
	if err := engine.DeleteHistoricVariableInstance(context.Background(), "missing"); err == nil {
		t.Fatalf("缺失历史变量删除应失败")
	}
}
