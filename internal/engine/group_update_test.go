package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/flowable-go/flowable/api"
)

// TestUpdateGroup 验证组更新。
// @param t 测试上下文
func TestUpdateGroup(t *testing.T) {
	engine := newTestEngine(t)
	ctx := context.Background()
	created, err := engine.CreateGroup(ctx, api.Group{ID: "team", Name: "Team", Type: "assignment"})
	if err != nil || created.ID != "team" {
		t.Fatalf("创建组失败: %v %+v", err, created)
	}
	updated, err := engine.UpdateGroup(ctx, api.Group{ID: "team", Name: "New Team", Type: "security"})
	if err != nil || updated.Name != "New Team" || updated.Type != "security" {
		t.Fatalf("更新组失败: %v %+v", err, updated)
	}
	loaded, _ := engine.GetGroup(ctx, "team")
	if loaded.Name != "New Team" || loaded.Type != "security" {
		t.Fatalf("组未更新: %+v", loaded)
	}
	if _, err := engine.UpdateGroup(ctx, api.Group{ID: "missing"}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失组更新应 404: %v", err)
	}
	if _, err := engine.UpdateGroup(ctx, api.Group{ID: " "}); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("空组 ID 应 400: %v", err)
	}
}
