package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
)

// TestIdentity_CRUD 验证用户与组管理。
// @param t 测试上下文
func TestIdentity_CRUD(t *testing.T) {
	engine := newTestEngine(t)
	user, err := engine.CreateUser(context.Background(), api.User{ID: "alice", DisplayName: "Alice"})
	if err != nil || user.ID != "alice" {
		t.Fatalf("创建用户失败: %v", err)
	}
	if _, err := engine.CreateUser(context.Background(), api.User{ID: "alice"}); err == nil {
		t.Fatalf("重复用户应失败")
	}
	if got, err := engine.GetUser(context.Background(), "alice"); err != nil || got.DisplayName != "Alice" {
		t.Fatalf("查询用户失败: %v", err)
	}
	if users, err := engine.ListUsers(context.Background()); err != nil || len(users) != 1 {
		t.Fatalf("用户列表错误: %v %d", err, len(users))
	}
	group, err := engine.CreateGroup(context.Background(), api.Group{ID: "dev", Name: "Dev"})
	if err != nil || group.ID != "dev" {
		t.Fatalf("创建组失败: %v", err)
	}
	if _, err := engine.CreateGroup(context.Background(), api.Group{ID: "dev"}); err == nil {
		t.Fatalf("重复组应失败")
	}
	if got, err := engine.GetGroup(context.Background(), "dev"); err != nil || got.Name != "Dev" {
		t.Fatalf("查询组失败: %v", err)
	}
	if groups, err := engine.ListGroups(context.Background()); err != nil || len(groups) != 1 {
		t.Fatalf("组列表错误: %v %d", err, len(groups))
	}
	if err := engine.AddMembership(context.Background(), "alice", "dev"); err != nil {
		t.Fatalf("添加成员失败: %v", err)
	}
	members, err := engine.ListMembers(context.Background(), "dev")
	if err != nil || len(members) != 1 || members[0].ID != "alice" {
		t.Fatalf("成员列表错误: %v %+v", err, members)
	}
	if err := engine.RemoveMembership(context.Background(), "alice", "dev"); err != nil {
		t.Fatalf("移除成员失败: %v", err)
	}
	if members, _ := engine.ListMembers(context.Background(), "dev"); len(members) != 0 {
		t.Fatalf("成员应移除")
	}
	if err := engine.DeleteGroup(context.Background(), "dev"); err != nil {
		t.Fatalf("删除组失败: %v", err)
	}
	if err := engine.DeleteUser(context.Background(), "alice"); err != nil {
		t.Fatalf("删除用户失败: %v", err)
	}
	if _, err := engine.GetUser(context.Background(), "alice"); err == nil {
		t.Fatalf("删除后用户应不存在")
	}
}

// TestIdentity_Errors 验证身份错误。
// @param t 测试上下文
func TestIdentity_Errors(t *testing.T) {
	engine := newTestEngine(t)
	if _, err := engine.CreateUser(context.Background(), api.User{}); err == nil {
		t.Fatalf("空用户 ID 应失败")
	}
	if _, err := engine.CreateGroup(context.Background(), api.Group{}); err == nil {
		t.Fatalf("空组 ID 应失败")
	}
	if _, err := engine.GetUser(context.Background(), "missing"); err == nil {
		t.Fatalf("缺失用户应失败")
	}
	if _, err := engine.GetGroup(context.Background(), "missing"); err == nil {
		t.Fatalf("缺失组应失败")
	}
	if err := engine.DeleteUser(context.Background(), "missing"); err == nil {
		t.Fatalf("删除缺失用户应失败")
	}
	if err := engine.DeleteGroup(context.Background(), "missing"); err == nil {
		t.Fatalf("删除缺失组应失败")
	}
	if err := engine.AddMembership(context.Background(), "missing", "missing"); err == nil {
		t.Fatalf("缺失用户/组应失败")
	}
}

// TestIdentity_CandidateGroup 验证候选组任务查询。
// @param t 测试上下文
func TestIdentity_CandidateGroup(t *testing.T) {
	engine := newTestEngine(t)
	_, _ = engine.CreateUser(context.Background(), api.User{ID: "alice"})
	_, _ = engine.CreateGroup(context.Background(), api.Group{ID: "dev"})
	_ = engine.AddMembership(context.Background(), "alice", "dev")
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="groupTask" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="task"/><userTask id="task" flowable:candidateGroups="dev"/><sequenceFlow id="f2" sourceRef="task" targetRef="e"/><endEvent id="e"/></process></definitions>`
	deployXML(t, engine, xml)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "groupTask"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	page, _ := engine.ListTasks(context.Background(), api.TaskFilter{CandidateUser: "alice"})
	if page.Total != 1 {
		t.Fatalf("候选组查询失败: %+v", page)
	}
	_ = engine.RemoveMembership(context.Background(), "alice", "dev")
	page, _ = engine.ListTasks(context.Background(), api.TaskFilter{CandidateUser: "alice"})
	if page.Total != 0 {
		t.Fatalf("移除成员后不应匹配")
	}
}
