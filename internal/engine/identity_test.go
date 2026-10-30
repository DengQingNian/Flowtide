package engine

import (
	"context"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/internal/clock"

	"github.com/DengQingNian/Flowtide/api"
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

// TestIdentityPassword 验证密码哈希与校验。
// @param t 测试上下文
func TestIdentityPassword(t *testing.T) {
	engine := newTestEngine(t)
	user, err := engine.CreateUser(context.Background(), api.User{ID: "alice", Password: "secret", Roles: []string{"reader"}})
	if err != nil || user.PasswordHash == "" || user.Password != "" {
		t.Fatalf("创建密码用户失败: %v %+v", err, user)
	}
	if err := engine.CheckPassword(context.Background(), "alice", "secret"); err != nil {
		t.Fatalf("密码校验失败: %v", err)
	}
	if err := engine.CheckPassword(context.Background(), "alice", "bad"); err == nil {
		t.Fatalf("错误密码应失败")
	}
	if err := engine.CheckPassword(context.Background(), "missing", "x"); err == nil {
		t.Fatalf("缺失用户应失败")
	}
	if _, err := engine.CreateUser(context.Background(), api.User{ID: "bob"}); err != nil {
		t.Fatalf("创建无密码用户失败: %v", err)
	}
	if err := engine.CheckPassword(context.Background(), "bob", "x"); err == nil {
		t.Fatalf("无密码用户应失败")
	}
}

// TestPasswordLockout 验证登录失败锁定。
// @param t 测试上下文
func TestPasswordLockout(t *testing.T) {
	fake := clock.NewFake(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	engine, _ := New(Config{Clock: fake})
	if _, err := engine.CreateUser(context.Background(), api.User{ID: "alice", Password: "secret"}); err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	for i := 0; i < 5; i++ {
		if err := engine.CheckPassword(context.Background(), "alice", "bad"); err == nil {
			t.Fatalf("错误密码应失败")
		}
	}
	if err := engine.CheckPassword(context.Background(), "alice", "secret"); err == nil {
		t.Fatalf("锁定后正确密码也应失败")
	}
	fake.Advance(6 * time.Minute)
	if err := engine.CheckPassword(context.Background(), "alice", "secret"); err != nil {
		t.Fatalf("锁定过期后应可登录: %v", err)
	}
}

// TestPasswordChange 验证密码修改与重置。
// @param t 测试上下文
func TestPasswordChange(t *testing.T) {
	engine := newTestEngine(t)
	if _, err := engine.CreateUser(context.Background(), api.User{ID: "alice", Password: "secret"}); err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	if err := engine.ChangePassword(context.Background(), "alice", "bad", "new"); err == nil {
		t.Fatalf("旧密码错误应失败")
	}
	if err := engine.ChangePassword(context.Background(), "alice", "secret", "new"); err != nil {
		t.Fatalf("修改密码失败: %v", err)
	}
	if err := engine.CheckPassword(context.Background(), "alice", "new"); err != nil {
		t.Fatalf("新密码应可登录: %v", err)
	}
	if err := engine.UpdatePassword(context.Background(), "alice", "reset"); err != nil {
		t.Fatalf("重置密码失败: %v", err)
	}
	if err := engine.CheckPassword(context.Background(), "alice", "reset"); err != nil {
		t.Fatalf("重置后密码应可登录: %v", err)
	}
	if err := engine.UpdatePassword(context.Background(), "missing", "x"); err == nil {
		t.Fatalf("缺失用户重置应失败")
	}
}

// TestUpdateUser 验证用户更新与角色。
// @param t 测试上下文
func TestUpdateUser(t *testing.T) {
	engine := newTestEngine(t)
	if _, err := engine.CreateUser(context.Background(), api.User{ID: "alice", DisplayName: "A", Password: "secret"}); err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	updated, err := engine.UpdateUser(context.Background(), api.User{
		ID: "alice", DisplayName: "Alice", Email: "a@example.com", Roles: []string{"reader"},
	})
	if err != nil || updated.DisplayName != "Alice" || len(updated.Roles) != 1 {
		t.Fatalf("更新用户失败: %v %+v", err, updated)
	}
	if err := engine.CheckPassword(context.Background(), "alice", "secret"); err != nil {
		t.Fatalf("更新不应清除密码: %v", err)
	}
	if err := engine.SetUserRoles(context.Background(), "alice", []string{"admin"}); err != nil {
		t.Fatalf("设置角色失败: %v", err)
	}
	user, _ := engine.GetUser(context.Background(), "alice")
	if len(user.Roles) != 1 || user.Roles[0] != "admin" {
		t.Fatalf("角色设置错误: %+v", user.Roles)
	}
	if _, err := engine.UpdateUser(context.Background(), api.User{ID: "missing"}); err == nil {
		t.Fatalf("缺失用户更新应失败")
	}
	if err := engine.SetUserRoles(context.Background(), "missing", nil); err == nil {
		t.Fatalf("缺失用户设置角色应失败")
	}
	if _, err := engine.UpdateUser(context.Background(), api.User{ID: "alice", Disabled: true}); err != nil {
		t.Fatalf("禁用用户失败: %v", err)
	}
	if err := engine.CheckPassword(context.Background(), "alice", "secret"); err == nil {
		t.Fatalf("禁用用户登录应失败")
	}
}

// TestRoleCRUD 验证角色管理。
// @param t 测试上下文
func TestRoleCRUD(t *testing.T) {
	engine := newTestEngine(t)
	role, err := engine.CreateRole(context.Background(), api.Role{ID: "reader", Name: "Reader", Permissions: []string{"read"}})
	if err != nil || role.ID != "reader" {
		t.Fatalf("创建角色失败: %v", err)
	}
	if _, err := engine.CreateRole(context.Background(), api.Role{ID: "reader"}); err == nil {
		t.Fatalf("重复角色应失败")
	}
	if _, err := engine.CreateRole(context.Background(), api.Role{}); err == nil {
		t.Fatalf("空角色 ID 应失败")
	}
	if got, err := engine.GetRole(context.Background(), "reader"); err != nil || got.Name != "Reader" {
		t.Fatalf("查询角色失败: %v", err)
	}
	if roles, err := engine.ListRoles(context.Background()); err != nil || len(roles) != 1 {
		t.Fatalf("角色列表错误: %v %d", err, len(roles))
	}
	if updated, err := engine.UpdateRole(context.Background(), api.Role{ID: "reader", Name: "R2", Permissions: []string{"read", "write"}}); err != nil || updated.Name != "R2" {
		t.Fatalf("更新角色失败: %v", err)
	}
	if _, err := engine.UpdateRole(context.Background(), api.Role{ID: "missing"}); err == nil {
		t.Fatalf("缺失角色更新应失败")
	}
	if err := engine.DeleteRole(context.Background(), "reader"); err != nil {
		t.Fatalf("删除角色失败: %v", err)
	}
	if _, err := engine.GetRole(context.Background(), "reader"); err == nil {
		t.Fatalf("删除后角色应不存在")
	}
	if err := engine.DeleteRole(context.Background(), "missing"); err == nil {
		t.Fatalf("删除缺失角色应失败")
	}
}

// TestListUserGroups 验证用户所属组。
// @param t 测试上下文
func TestListUserGroups(t *testing.T) {
	engine := newTestEngine(t)
	_, _ = engine.CreateUser(context.Background(), api.User{ID: "alice"})
	_, _ = engine.CreateGroup(context.Background(), api.Group{ID: "dev"})
	_, _ = engine.CreateGroup(context.Background(), api.Group{ID: "ops"})
	_ = engine.AddMembership(context.Background(), "alice", "dev")
	groups, err := engine.ListUserGroups(context.Background(), "alice")
	if err != nil || len(groups) != 1 || groups[0].ID != "dev" {
		t.Fatalf("用户组错误: %v %+v", err, groups)
	}
	if groups, _ := engine.ListUserGroups(context.Background(), "missing"); len(groups) != 0 {
		t.Fatalf("缺失用户应返回空组")
	}
}
