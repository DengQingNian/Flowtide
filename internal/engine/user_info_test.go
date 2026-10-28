package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/flowable-go/flowable/api"
)

// TestUserInfoAndPicture 验证用户信息与头像。
// @param t 测试上下文
func TestUserInfoAndPicture(t *testing.T) {
	engine := newTestEngine(t)
	ctx := context.Background()
	if _, err := engine.CreateUser(ctx, api.User{ID: "alice"}); err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	info, err := engine.GetUserInfo(ctx, "alice")
	if err != nil || len(info) != 0 {
		t.Fatalf("初始用户信息应为空: %v %+v", err, info)
	}
	if err := engine.SetUserInfo(ctx, "alice", "title", "Engineer"); err != nil {
		t.Fatalf("设置用户信息失败: %v", err)
	}
	if err := engine.SetUserInfo(ctx, "alice", "title", "Senior"); err != nil {
		t.Fatalf("更新用户信息失败: %v", err)
	}
	value, err := engine.GetUserInfoValue(ctx, "alice", "title")
	if err != nil || value != "Senior" {
		t.Fatalf("用户信息查询失败: %v %s", err, value)
	}
	info, err = engine.GetUserInfo(ctx, "alice")
	if err != nil || info["title"] != "Senior" {
		t.Fatalf("用户信息列表错误: %v %+v", err, info)
	}
	if err := engine.DeleteUserInfo(ctx, "alice", "title"); err != nil {
		t.Fatalf("删除用户信息失败: %v", err)
	}
	if _, err := engine.GetUserInfoValue(ctx, "alice", "title"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("删除后查询应 404: %v", err)
	}
	if err := engine.DeleteUserInfo(ctx, "alice", "title"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("重复删除应 404: %v", err)
	}
	if err := engine.SetUserInfo(ctx, "alice", " ", "x"); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("空键应 400: %v", err)
	}
	if _, err := engine.GetUserInfo(ctx, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失用户信息应 404: %v", err)
	}
	if err := engine.SetUserInfo(ctx, "missing", "k", "v"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失用户设置应 404: %v", err)
	}

	picture := []byte{0x89, 0x50, 0x4e, 0x47}
	if err := engine.SetUserPicture(ctx, "alice", picture, "image/png"); err != nil {
		t.Fatalf("设置头像失败: %v", err)
	}
	data, mimeType, err := engine.GetUserPicture(ctx, "alice")
	if err != nil || string(data) != string(picture) || mimeType != "image/png" {
		t.Fatalf("头像查询失败: %v %s %v", err, mimeType, data)
	}
	if err := engine.SetUserPicture(ctx, "alice", []byte("x"), ""); err != nil {
		t.Fatalf("默认 MIME 设置失败: %v", err)
	}
	if _, mimeType, _ := engine.GetUserPicture(ctx, "alice"); mimeType != "image/jpeg" {
		t.Fatalf("默认 MIME 错误: %s", mimeType)
	}
	if err := engine.SetUserPicture(ctx, "alice", nil, "image/png"); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("空头像应 400: %v", err)
	}
	if _, _, err := engine.GetUserPicture(ctx, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失用户头像应 404: %v", err)
	}

	if err := engine.SetUserInfo(ctx, "alice", "dept", "R&D"); err != nil {
		t.Fatalf("设置用户信息失败: %v", err)
	}
	snapshot := engine.toSnapshot()
	if snapshot.UserInfo["alice"]["dept"] != "R&D" {
		t.Fatalf("快照用户信息错误: %+v", snapshot.UserInfo)
	}
	if snapshot.UserPictures["alice"] == nil || len(snapshot.UserPictures["alice"].Data) == 0 {
		t.Fatalf("快照用户头像缺失: %+v", snapshot.UserPictures)
	}
	cloned := cloneState(engine.state)
	if cloned.UserPictures["alice"] == nil || cloned.UserInfo["alice"] == nil {
		t.Fatalf("克隆状态丢失用户信息/头像")
	}

	if err := engine.DeleteUser(ctx, "alice"); err != nil {
		t.Fatalf("删除用户失败: %v", err)
	}
	if _, err := engine.GetUserInfo(ctx, "alice"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("删除用户后信息应 404: %v", err)
	}
	if _, _, err := engine.GetUserPicture(ctx, "alice"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("删除用户后头像应 404: %v", err)
	}
}
