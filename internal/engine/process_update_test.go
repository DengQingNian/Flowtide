package engine

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
	"github.com/flowable-go/flowable/internal/tenant"
)

// TestProcessMetadataRows 验证实例属性、签收和发起人以标准行为准，不被续跑控制中的旧副本覆盖。
// @param t 测试上下文
// @return 无
func TestProcessMetadataRows(t *testing.T) {
	ctx := context.Background()
	app := newTestEngine(t)
	app.cfg.HistoryLevel = HistoryNone
	deployXML(t, app, simpleProcess)
	instance, err := app.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "simple", Name: "原名", UserID: "starter"})
	if err != nil {
		t.Fatal(err)
	}
	name, key, status, user := "更新名称", "updated", "review", "bob"
	due := time.Date(2026, 12, 1, 8, 0, 0, 123000000, time.FixedZone("验收时区", 8*3600))
	updated, err := app.UpdateProcessInstance(ctx, instance.ID, api.ProcessInstanceUpdate{Name: &name, BusinessKey: &key, BusinessStatus: &status, DueDate: &due, ClaimUserID: &user})
	if err != nil || updated.ClaimTime == nil {
		t.Fatalf("实例更新失败: %+v %v", updated, err)
	}
	rows, err := app.SnapshotRows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, control := range []bool{true, false} {
		t.Run(map[bool]string{true: "含续跑控制", false: "仅标准行"}[control], func(t *testing.T) {
			prepared := make([]flowablerow.Row, 0, len(rows))
			for _, row := range rows {
				if row.Table == "ACT_GE_BYTEARRAY" && strings.HasPrefix(row.Values["NAME_"].(string), "flowable-go-control/") && !control {
					continue
				}
				if row.Table == "ACT_RU_EXECUTION" && row.Values["ID_"] == instance.ID {
					_ = row.Set("NAME_", "标准行名称")
				}
				prepared = append(prepared, row)
			}
			restored := newTestEngine(t)
			if err := restored.RestoreRows(ctx, prepared); err != nil {
				t.Fatal(err)
			}
			actual, err := restored.GetProcessInstance(ctx, instance.ID)
			if err != nil || actual.Name != "标准行名称" || actual.StartUserID != "starter" || actual.BusinessStatus != "review" || actual.BusinessKey != "updated" || actual.DueDate == nil || !actual.DueDate.Equal(due) || actual.ClaimedBy != "bob" || actual.ClaimTime == nil || !actual.ClaimTime.Equal(*updated.ClaimTime) {
				t.Fatalf("标准实例元数据丢失或被扩展覆盖: %+v %v", actual, err)
			}
			links, err := restored.ListInstanceIdentityLinks(ctx, instance.ID)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, link := range links {
				if link.Type == "assignee" && link.UserID == "bob" {
					found = true
				}
			}
			if !found {
				t.Fatal("实例签收身份链接未恢复")
			}
		})
	}
}

// TestProcessUpdateAtomicity 验证签收冲突、跨租户、取消及持久化失败不会部分修改实例或身份链接。
// @param t 测试上下文
// @return 无
func TestProcessUpdateAtomicity(t *testing.T) {
	ctx := context.Background()
	app := newTestEngine(t)
	deployXML(t, app, simpleProcess)
	instance, err := app.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "simple", Name: "原名"})
	if err != nil {
		t.Fatal(err)
	}
	user, replacement := "alice", "禁止部分更新"
	claimed, err := app.UpdateProcessInstance(ctx, instance.ID, api.ProcessInstanceUpdate{ClaimUserID: &user})
	if err != nil {
		t.Fatal(err)
	}
	links, _ := app.ListInstanceIdentityLinks(ctx, instance.ID)
	// 同一用户重复签收也应冲突，名称等属性必须保持提交前的完整状态。
	if _, err := app.UpdateProcessInstance(ctx, instance.ID, api.ProcessInstanceUpdate{Name: &replacement, ClaimUserID: &user}); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("重复实例签收未冲突: %v", err)
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	for _, request := range []struct {
		ctx  context.Context
		id   string
		want error
	}{{tenant.With(ctx, "other"), instance.ID, api.ErrNotFound}, {ctx, "missing", api.ErrNotFound}, {canceled, instance.ID, context.Canceled}} {
		if _, err := app.UpdateProcessInstance(request.ctx, request.id, api.ProcessInstanceUpdate{Name: &replacement}); !errors.Is(err, request.want) {
			t.Fatalf("拒绝无效更新: %v", err)
		}
	}
	app.cfg.StateStore = asyncSaveFailureStore{}
	empty := ""
	if _, err := app.UpdateProcessInstance(ctx, instance.ID, api.ProcessInstanceUpdate{Name: &replacement, ClaimUserID: &empty}); !errors.Is(err, context.Canceled) {
		t.Fatalf("持久化故障未返回: %v", err)
	}
	app.cfg.StateStore = nil
	actual, _ := app.GetProcessInstance(ctx, instance.ID)
	actualLinks, _ := app.ListInstanceIdentityLinks(ctx, instance.ID)
	if !reflect.DeepEqual(claimed, actual) || !reflect.DeepEqual(links, actualLinks) {
		t.Fatalf("失败更新污染实例或签收链接: %+v %+v", actual, actualLinks)
	}
	// 返回的可空时间必须是副本；取消签收应同步清除运行字段与历史字段。
	*actual.ClaimTime = actual.ClaimTime.Add(time.Hour)
	readBack, _ := app.GetProcessInstance(ctx, instance.ID)
	if readBack.ClaimTime == nil || !readBack.ClaimTime.Equal(*claimed.ClaimTime) {
		t.Fatal("调用方修改签收时间污染引擎状态")
	}
	unclaimed, err := app.UpdateProcessInstance(ctx, instance.ID, api.ProcessInstanceUpdate{ClaimUserID: &empty})
	if err != nil || unclaimed.ClaimTime != nil || unclaimed.ClaimedBy != "" {
		t.Fatalf("取消签收失败: %+v %v", unclaimed, err)
	}
	historic, _ := app.GetHistoricProcessInstance(ctx, instance.ID)
	if historic.ClaimTime != nil || historic.ClaimedBy != "" {
		t.Fatalf("历史签收元数据未清除: %+v", historic)
	}
	if err := app.DeleteProcessInstance(ctx, instance.ID, "验收"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.UpdateProcessInstance(ctx, instance.ID, api.ProcessInstanceUpdate{Name: &replacement}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("已结束实例仍可更新: %v", err)
	}
}

// TestMessageStartSelection 验证消息只启动本租户最新定义，旧版本订阅和运行时等待不参与选择。
// @param t 测试上下文
// @return 无
func TestMessageStartSelection(t *testing.T) {
	ctx := context.Background()
	app := newTestEngine(t)
	// 同一消息跨租户、跨版本并存时，每个请求只能看到自身租户的最新版本。
	for _, scope := range []context.Context{ctx, tenant.With(ctx, "other")} {
		for version := 0; version < 2; version++ {
			if _, err := app.Deploy(scope, api.DeploymentRequest{Resources: map[string][]byte{"message.bpmn20.xml": []byte(messageStartXML)}}); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, scope := range []context.Context{ctx, tenant.With(ctx, "other")} {
		instance, err := app.StartProcessInstance(scope, api.StartRequest{MessageName: "orderCreated", UserID: "starter"})
		if err != nil {
			t.Fatal(err)
		}
		definition, err := app.GetProcessDefinition(scope, instance.ProcessDefinitionID)
		if err != nil || definition.Version != 2 || definition.TenantID != tenant.From(scope) {
			t.Fatalf("消息未选择本租户最新定义: %+v %v", definition, err)
		}
	}
	if _, err := app.StartProcessInstance(ctx, api.StartRequest{MessageName: "missing"}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("不存在消息未拒绝: %v", err)
	}
	deployXML(t, app, strings.Replace(messageStartXML, `id="msgStart"`, `id="anotherMessageStart"`, 1))
	before, _ := app.ListProcessInstances(ctx)
	if _, err := app.StartProcessInstance(ctx, api.StartRequest{MessageName: "orderCreated"}); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("歧义消息未拒绝: %v", err)
	}
	after, _ := app.ListProcessInstances(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("歧义启动仍创建了实例")
	}
}
