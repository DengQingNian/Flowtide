package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/flowable-go/flowable/api"
)

// TestIdentityRowsRestart 验证身份用户、组、成员、头像及可空信息在完整行快照中保存并重启恢复。
// @param t 测试上下文
// @return 无
func TestIdentityRowsRestart(t *testing.T) {
	ctx := context.Background()
	app := newTestEngine(t)
	if _, err := app.CreateUser(ctx, api.User{ID: "alice", FirstName: "原名", Password: "reference-password"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.CreateGroup(ctx, api.Group{ID: "review", Name: "审批组", Type: "assignment", TenantID: "tenant-a"}); err != nil {
		t.Fatal(err)
	}
	if err := app.AddMembership(ctx, "alice", "review"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.CreateRole(ctx, api.Role{ID: "reader", Name: "读取者", Permissions: []string{"read"}}); err != nil {
		t.Fatal(err)
	}
	if err := app.SetUserRoles(ctx, "alice", []string{"reader"}); err != nil {
		t.Fatal(err)
	}
	if err := app.SetUserInfo(ctx, "alice", "team", "原团队"); err != nil {
		t.Fatal(err)
	}
	if err := app.SetUserInfo(ctx, "alice", "cleared", "待清空"); err != nil {
		t.Fatal(err)
	}
	if err := app.DeleteUserInfo(ctx, "alice", "cleared"); err != nil {
		t.Fatal(err)
	}
	if err := app.SetUserPicture(ctx, "alice", []byte{1, 2, 0, 255}, "image/png"); err != nil {
		t.Fatal(err)
	}
	rows, err := app.SnapshotRows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	// 标准用户行必须是资料的真实来源，不能被控制扩展中的旧副本覆盖。
	for _, row := range rows {
		found[row.Table] = true
		if row.Table == "ACT_ID_USER" && row.Values["ID_"] == "alice" {
			_ = row.Set("FIRST_", "标准行名称")
		}
	}
	for _, table := range []string{"ACT_ID_USER", "ACT_ID_GROUP", "ACT_ID_MEMBERSHIP", "ACT_ID_INFO", "ACT_ID_BYTEARRAY"} {
		if !found[table] {
			t.Fatalf("身份行快照缺少%s", table)
		}
	}
	restored := newTestEngine(t)
	if err := restored.RestoreRows(ctx, rows); err != nil {
		t.Fatal(err)
	}
	user, err := restored.GetUser(ctx, "alice")
	if err != nil || user.FirstName != "标准行名称" {
		t.Fatalf("标准用户行没有恢复: %+v %v", user, err)
	}
	if len(user.Roles) != 1 || user.Roles[0] != "reader" {
		t.Fatalf("身份角色扩展未恢复: %+v", user.Roles)
	}
	if role, err := restored.GetRole(ctx, "reader"); err != nil || len(role.Permissions) != 1 || role.Permissions[0] != "read" {
		t.Fatalf("角色权限未恢复: %+v %v", role, err)
	}
	if err := restored.CheckPassword(ctx, "alice", "reference-password"); err != nil {
		t.Fatalf("重启后密码失效: %v", err)
	}
	groups, err := restored.ListUserGroups(ctx, "alice")
	if err != nil || len(groups) != 1 || groups[0].ID != "review" || groups[0].TenantID != "tenant-a" {
		t.Fatalf("成员关系恢复失败: %+v %v", groups, err)
	}
	entries, err := restored.GetUserInfoEntries(ctx, "alice")
	if err != nil || entries["team"] == nil || *entries["team"] != "原团队" {
		t.Fatalf("用户信息恢复失败: %+v %v", entries, err)
	}
	if value, exists := entries["cleared"]; !exists || value != nil {
		t.Fatal("标准身份信息行丢失空值键")
	}
	if _, err := restored.GetUserInfoValue(ctx, "alice", "cleared"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("空值单键没有保持不存在语义: %v", err)
	}
	data, mime, err := restored.GetUserPicture(ctx, "alice")
	if err != nil || string(data) != string([]byte{1, 2, 0, 255}) || mime != "image/png" {
		t.Fatalf("头像恢复失败: %v %s %v", data, mime, err)
	}
}

// TestIdentityMembershipDelimiter 验证成员标识包含竖线时仍为两个独立关系，并可从旧格式快照读取普通标识。
// @param t 测试上下文
// @return 无
func TestIdentityMembershipDelimiter(t *testing.T) {
	ctx := context.Background()
	app := newTestEngine(t)
	for _, pair := range [][2]string{{"a|b", "c"}, {"a", "b|c"}} {
		if _, err := app.CreateUser(ctx, api.User{ID: pair[0]}); err != nil {
			t.Fatal(err)
		}
		if _, err := app.CreateGroup(ctx, api.Group{ID: pair[1]}); err != nil {
			t.Fatal(err)
		}
		if err := app.AddMembership(ctx, pair[0], pair[1]); err != nil {
			t.Fatal(err)
		}
	}
	// 两个字符串拼接后的文本相同，仍必须分别返回真实的用户和组。
	for _, pair := range [][2]string{{"a|b", "c"}, {"a", "b|c"}} {
		groups, err := app.ListUserGroups(ctx, pair[0])
		if err != nil || len(groups) != 1 || groups[0].ID != pair[1] {
			t.Fatalf("成员分隔符碰撞: %+v %v", groups, err)
		}
	}
	if err := app.DeleteGroup(ctx, "c"); err != nil {
		t.Fatal(err)
	}
	groups, err := app.ListUserGroups(ctx, "a")
	if err != nil || len(groups) != 1 || groups[0].ID != "b|c" {
		t.Fatal("删除一个关系误删了另一个关系")
	}
}
