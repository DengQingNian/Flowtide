package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

const starterProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <process id="starter" name="Starter" isExecutable="true"
           flowable:candidateStarterUsers="alice" flowable:candidateStarterGroups="g1">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="task"/>
    <userTask id="task" name="Task"/>
    <sequenceFlow id="f2" sourceRef="task" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestDefinitionIdentityLinks 验证候选发起人增删查。
// @param t 测试上下文
func TestDefinitionIdentityLinks(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, starterProcess)
	ctx := context.Background()
	definitions, err := engine.ListProcessDefinitions(ctx)
	if err != nil || len(definitions) != 1 {
		t.Fatalf("定义列表错误: %v %+v", err, definitions)
	}
	definitionID := definitions[0].ID

	links, err := engine.ListDefinitionIdentityLinks(ctx, definitionID)
	if err != nil || len(links) != 2 || links[0].Type != "candidate" {
		t.Fatalf("候选发起人列表错误: %v %+v", err, links)
	}
	userLink, err := engine.GetDefinitionIdentityLink(ctx, definitionID, "users", "alice")
	if err != nil || userLink.UserID != "alice" || userLink.ProcessDefinitionID != definitionID {
		t.Fatalf("候选用户查询失败: %v %+v", err, userLink)
	}
	groupLink, err := engine.GetDefinitionIdentityLink(ctx, definitionID, "groups", "g1")
	if err != nil || groupLink.GroupID != "g1" {
		t.Fatalf("候选组查询失败: %v %+v", err, groupLink)
	}
	if _, err := engine.GetDefinitionIdentityLink(ctx, definitionID, "users", "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失候选用户应 404: %v", err)
	}

	created, err := engine.AddDefinitionIdentityLink(ctx, definitionID, api.IdentityLink{UserID: "carol", Type: "participant"})
	if err != nil || created.UserID != "carol" || created.Type != "candidate" {
		t.Fatalf("添加候选发起人失败: %v %+v", err, created)
	}
	duplicate, err := engine.AddDefinitionIdentityLink(ctx, definitionID, api.IdentityLink{UserID: "carol"})
	if err != nil || duplicate.ID != created.ID {
		t.Fatalf("重复添加应幂等: %v %+v", err, duplicate)
	}
	if links, _ := engine.ListDefinitionIdentityLinks(ctx, definitionID); len(links) != 3 {
		t.Fatalf("添加后链接数量错误: %+v", links)
	}
	if _, err := engine.AddDefinitionIdentityLink(ctx, definitionID, api.IdentityLink{}); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("缺少身份应 400: %v", err)
	}
	if _, err := engine.AddDefinitionIdentityLink(ctx, definitionID, api.IdentityLink{UserID: "a", GroupID: "g"}); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("同时指定用户与组应 400: %v", err)
	}
	if err := engine.DeleteDefinitionIdentityLink(ctx, definitionID, "users", "carol"); err != nil {
		t.Fatalf("删除候选发起人失败: %v", err)
	}
	if err := engine.DeleteDefinitionIdentityLink(ctx, definitionID, "users", "carol"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("重复删除应 404: %v", err)
	}
	if err := engine.DeleteDefinitionIdentityLink(ctx, definitionID, "teams", "carol"); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("非法参与者类型应 400: %v", err)
	}

	foreign := tenant.With(ctx, "t2")
	if _, err := engine.ListDefinitionIdentityLinks(foreign, definitionID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户查询应 404: %v", err)
	}
	if _, err := engine.AddDefinitionIdentityLink(foreign, definitionID, api.IdentityLink{UserID: "x"}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户添加应 404: %v", err)
	}
	if err := engine.DeleteDefinitionIdentityLink(foreign, definitionID, "users", "alice"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户删除应 404: %v", err)
	}
	if _, err := engine.ListDefinitionIdentityLinks(ctx, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失定义应 404: %v", err)
	}
}

// TestDefinitionIdentityLinkPersistence 验证候选发起人行持久化。
// @param t 测试上下文
func TestDefinitionIdentityLinkPersistence(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, starterProcess)
	ctx := context.Background()
	definitions, _ := engine.ListProcessDefinitions(ctx)
	definitionID := definitions[0].ID
	created, err := engine.AddDefinitionIdentityLink(ctx, definitionID, api.IdentityLink{UserID: "carol"})
	if err != nil {
		t.Fatalf("添加候选发起人失败: %v", err)
	}
	rows, err := engine.SnapshotRows(ctx)
	if err != nil {
		t.Fatalf("导出行失败: %v", err)
	}
	linkRows := 0
	for _, row := range rows {
		if row.Table == "ACT_RU_IDENTITYLINK" && row.Values["PROC_DEF_ID_"] == definitionID {
			linkRows++
		}
	}
	if linkRows != 3 {
		t.Fatalf("候选发起人行数量错误: %d", linkRows)
	}
	restored := newTestEngine(t)
	if err := restored.RestoreRows(ctx, rows); err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	links, err := restored.ListDefinitionIdentityLinks(ctx, definitionID)
	if err != nil || len(links) != 3 {
		t.Fatalf("恢复候选发起人错误: %v %+v", err, links)
	}
	found := false
	for _, link := range links {
		if link.ID == created.ID || link.UserID == "carol" {
			found = true
		}
	}
	if !found {
		t.Fatalf("恢复后新增候选用户丢失: %+v", links)
	}
}
