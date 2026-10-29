package engine

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/flowable-go/flowable/api"
	statestore "github.com/flowable-go/flowable/internal/persistence/state"
	"testing"
)

// TestIdentityInfoNullSnapshot 验证原版删除用户信息保留空值键，且旧字符串快照仍可兼容恢复。
// @param t 测试上下文
// @return 无
func TestIdentityInfoNullSnapshot(t *testing.T) {
	ctx := context.Background()
	app := newTestEngine(t)
	if _, err := app.CreateUser(ctx, api.User{ID: "alice"}); err != nil {
		t.Fatal(err)
	}
	if err := app.SetUserInfo(ctx, "alice", "team", "原团队"); err != nil {
		t.Fatal(err)
	}
	entries, err := app.GetUserInfoEntries(ctx, "alice")
	if err != nil || entries["team"] == nil {
		t.Fatalf("非空信息读取失败: %+v %v", entries, err)
	}
	*entries["team"] = "外部篡改"
	if value, _ := app.GetUserInfoValue(ctx, "alice", "team"); value != "原团队" {
		t.Fatal("可空信息接口暴露内部指针")
	}
	if err := app.DeleteUserInfo(ctx, "alice", "team"); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(app.toSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	var stored struct {
		UserInfo map[string]map[string]any `json:"userInfo"`
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	value, exists := stored.UserInfo["alice"]["team"]
	if !exists || value != nil {
		t.Fatalf("原版空值键在快照中丢失: %+v", stored.UserInfo)
	}
	store := statestore.NewMemory()
	app.cfg.StateStore = store
	if err := app.saveState(ctx); err != nil {
		t.Fatal(err)
	}
	restored, err := New(Config{StateStore: store})
	if err != nil {
		t.Fatal(err)
	}
	entries, err = restored.GetUserInfoEntries(ctx, "alice")
	if value, exists := entries["team"]; err != nil || !exists || value != nil {
		t.Fatalf("可空键重启恢复失败: %+v %v", entries, err)
	}
	if _, err := restored.GetUserInfoEntries(ctx, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失用户未拒绝: %v", err)
	}
	// 旧字符串值与新指针值使用同一 JSON 形式，旧快照不需要升级版本号。
	var legacy snapshot
	if err := json.Unmarshal([]byte(`{"userInfo":{"alice":{"team":"旧字符串"}}}`), &legacy); err != nil || legacy.UserInfo["alice"]["team"] == nil || *legacy.UserInfo["alice"]["team"] != "旧字符串" {
		t.Fatalf("旧字符串格式不可读: %+v %v", legacy.UserInfo, err)
	}
}
