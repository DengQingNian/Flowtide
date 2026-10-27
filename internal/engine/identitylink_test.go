package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
)

const identityLinkTaskXML = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="identityLinkTask" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="task"/><userTask id="task" flowable:candidateUsers="alice" flowable:candidateGroups="dev"/><sequenceFlow id="f2" sourceRef="task" targetRef="e"/><endEvent id="e"/></process></definitions>`

// TestIdentityLinks 验证任务候选人与候选组管理。
// @param t 测试上下文
func TestIdentityLinks(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, identityLinkTaskXML)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "identityLinkTask"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	task := listTasks(t, engine)[0]
	links, err := engine.ListIdentityLinks(context.Background(), task.ID)
	if err != nil || len(links) != 2 {
		t.Fatalf("身份链接列表错误: %v %+v", err, links)
	}
	if err := engine.AddCandidateUser(context.Background(), task.ID, "bob"); err != nil {
		t.Fatalf("添加候选用户失败: %v", err)
	}
	if err := engine.AddCandidateUser(context.Background(), task.ID, "bob"); err != nil {
		t.Fatalf("重复添加候选用户失败: %v", err)
	}
	links, _ = engine.ListIdentityLinks(context.Background(), task.ID)
	if len(links) != 3 {
		t.Fatalf("候选用户重复添加: %+v", links)
	}
	if err := engine.DeleteCandidateUser(context.Background(), task.ID, "bob"); err != nil {
		t.Fatalf("删除候选用户失败: %v", err)
	}
	if err := engine.AddCandidateGroup(context.Background(), task.ID, "ops"); err != nil {
		t.Fatalf("添加候选组失败: %v", err)
	}
	if err := engine.DeleteCandidateGroup(context.Background(), task.ID, "ops"); err != nil {
		t.Fatalf("删除候选组失败: %v", err)
	}
	page, _ := engine.ListTasks(context.Background(), api.TaskFilter{CandidateUser: "alice"})
	if page.Total != 1 {
		t.Fatalf("候选用户查询失败")
	}
}

// TestIdentityLinks_Errors 验证身份链接错误。
// @param t 测试上下文
func TestIdentityLinks_Errors(t *testing.T) {
	engine := newTestEngine(t)
	if _, err := engine.ListIdentityLinks(context.Background(), "missing"); err == nil {
		t.Fatalf("缺失任务身份链接应报错")
	}
	if err := engine.AddCandidateUser(context.Background(), "missing", "alice"); err == nil {
		t.Fatalf("缺失任务添加候选应报错")
	}
	if err := engine.DeleteCandidateGroup(context.Background(), "missing", "dev"); err == nil {
		t.Fatalf("缺失任务删除候选组应报错")
	}
}

// TestHistoricIdentityLinks 验证历史身份链接记录。
// @param t 测试上下文
func TestHistoricIdentityLinks(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, identityLinkTaskXML)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "identityLinkTask"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	task := listTasks(t, engine)[0]
	links, err := engine.ListHistoricIdentityLinks(context.Background(), task.ID)
	if err != nil || len(links) != 2 {
		t.Fatalf("历史链接数量错误: %v %+v", err, links)
	}
	if err := engine.AddCandidateUser(context.Background(), task.ID, "bob"); err != nil {
		t.Fatalf("添加候选人失败: %v", err)
	}
	links, _ = engine.ListHistoricIdentityLinks(context.Background(), task.ID)
	if len(links) != 3 {
		t.Fatalf("候选人变更后历史链接错误: %+v", links)
	}
	if err := engine.Complete(context.Background(), task.ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	links, _ = engine.ListHistoricIdentityLinks(context.Background(), task.ID)
	if len(links) != 3 {
		t.Fatalf("完成后历史链接错误: %+v", links)
	}
}
