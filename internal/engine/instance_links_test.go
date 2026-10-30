package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// TestInstanceIdentityLinks 验证实例身份链接增删查。
// @param t 测试上下文
func TestInstanceIdentityLinks(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	ctx := context.Background()
	instance, err := engine.StartProcessInstance(ctx, api.StartRequest{
		ProcessDefinitionKey: "simple",
		UserID:               "alice",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	links, err := engine.ListInstanceIdentityLinks(ctx, instance.ID)
	// 原版发起人在运行时身份链接中类型为 starter。
	if err != nil || len(links) != 1 || links[0].Type != "starter" || links[0].UserID != "alice" {
		t.Fatalf("发起人链接错误: %v %+v", err, links)
	}
	created, err := engine.AddInstanceIdentityLink(ctx, instance.ID, api.IdentityLink{Type: "candidate", GroupID: "g1"})
	if err != nil || created.ID == "" || created.ProcessInstanceID != instance.ID || created.GroupID != "g1" {
		t.Fatalf("添加身份链接失败: %v %+v", err, created)
	}
	if _, err := engine.AddInstanceIdentityLink(ctx, instance.ID, api.IdentityLink{Type: "candidate", UserID: "a", GroupID: "g"}); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("同时指定用户与组应 400: %v", err)
	}
	if _, err := engine.AddInstanceIdentityLink(ctx, instance.ID, api.IdentityLink{Type: "candidate"}); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("缺少用户与组应 400: %v", err)
	}
	if _, err := engine.AddInstanceIdentityLink(ctx, instance.ID, api.IdentityLink{UserID: "a"}); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("缺少类型应 400: %v", err)
	}
	if links, _ := engine.ListInstanceIdentityLinks(ctx, instance.ID); len(links) != 2 {
		t.Fatalf("链接数量错误: %+v", links)
	}
	if err := engine.DeleteInstanceIdentityLink(ctx, instance.ID, "groups", "g1", "candidate"); err != nil {
		t.Fatalf("删除身份链接失败: %v", err)
	}
	if links, _ := engine.ListInstanceIdentityLinks(ctx, instance.ID); len(links) != 1 {
		t.Fatalf("删除后链接数量错误: %+v", links)
	}
	if err := engine.DeleteInstanceIdentityLink(ctx, instance.ID, "groups", "g1", "candidate"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("重复删除应 404: %v", err)
	}
	if err := engine.DeleteInstanceIdentityLink(ctx, instance.ID, "teams", "g1", "candidate"); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("非法参与者类型应 400: %v", err)
	}
	if err := engine.DeleteInstanceIdentityLink(ctx, instance.ID, "users", "alice", "candidate"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("类型不匹配应 404: %v", err)
	}
	historic, err := engine.ListHistoricProcessIdentityLinks(ctx, instance.ID)
	if err != nil || len(historic) == 0 {
		t.Fatalf("历史身份链接为空: %v %+v", err, historic)
	}
	if _, err := engine.ListInstanceIdentityLinks(ctx, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失实例应 404: %v", err)
	}
	if _, err := engine.AddInstanceIdentityLink(ctx, "missing", api.IdentityLink{Type: "candidate", UserID: "a"}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失实例添加应 404: %v", err)
	}
	if err := engine.DeleteInstanceIdentityLink(ctx, "missing", "users", "a", "candidate"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失实例删除应 404: %v", err)
	}
}

// TestInstanceIdentityLinkTenantAndPersistence 验证租户与行持久化。
// @param t 测试上下文
func TestInstanceIdentityLinkTenantAndPersistence(t *testing.T) {
	engine := newTestEngine(t)
	engine.cfg.TenantID = "t1"
	deployXML(t, engine, simpleProcess)
	ctx := context.Background()
	instance, err := engine.StartProcessInstance(ctx, api.StartRequest{
		ProcessDefinitionKey: "simple",
		TenantID:             "t1",
		UserID:               "alice",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	foreign := tenant.With(ctx, "t2")
	if _, err := engine.ListInstanceIdentityLinks(foreign, instance.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户查询应 404: %v", err)
	}
	if _, err := engine.AddInstanceIdentityLink(foreign, instance.ID, api.IdentityLink{Type: "candidate", UserID: "bob"}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户添加应 404: %v", err)
	}
	if err := engine.DeleteInstanceIdentityLink(foreign, instance.ID, "users", "alice", "participant"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户删除应 404: %v", err)
	}

	rows, err := engine.SnapshotRows(ctx)
	if err != nil {
		t.Fatalf("导出行失败: %v", err)
	}
	found := false
	for _, row := range rows {
		if row.Table == "ACT_RU_IDENTITYLINK" {
			found = true
			if row.Values["USER_ID_"] != "alice" || row.Values["TYPE_"] != "starter" {
				t.Fatalf("身份链接行错误: %+v", row.Values)
			}
		}
	}
	if !found {
		t.Fatalf("未导出身份链接行")
	}
	restored := newTestEngine(t)
	restored.cfg.TenantID = "t1"
	if err := restored.RestoreRows(ctx, rows); err != nil {
		t.Fatalf("导入行失败: %v", err)
	}
	links, err := restored.ListInstanceIdentityLinks(ctx, instance.ID)
	if err != nil || len(links) != 1 || links[0].UserID != "alice" {
		t.Fatalf("恢复身份链接错误: %v %+v", err, links)
	}
}
