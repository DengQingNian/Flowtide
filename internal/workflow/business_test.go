// business_test.go 验证业务系统注册的内存存储与注册服务（P1.1）。
package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/idgen"
)

// newFixedTime 返回测试固定时间。
// @return 固定时间
func newFixedTime() time.Time {
	return time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC)
}

// TestMemoryBusinessSystemStoreCRUD 验证内存存储的 upsert/查询/删除全流程。
// @param t 测试上下文
func TestMemoryBusinessSystemStoreCRUD(t *testing.T) {
	store := NewMemoryBusinessSystemStore()
	ctx := context.Background()
	now := newFixedTime()

	// 空入参与空代号校验
	if err := store.Save(ctx, nil); err == nil {
		t.Error("nil 注册信息应报错")
	}
	if err := store.Save(ctx, &BusinessSystem{ID: "id1", CreateTime: now, UpdateTime: now}); err == nil {
		t.Error("空系统代号应报错")
	}
	// 新增必须带 ID
	if err := store.Save(ctx, &BusinessSystem{SystemSymbol: "oa", CreateTime: now, UpdateTime: now}); err == nil {
		t.Error("新增缺 ID 应报错")
	}
	// 正常新增
	if err := store.Save(ctx, &BusinessSystem{ID: "id1", SystemSymbol: "oa", SystemName: "OA 系统", TaskCallAddr: "http://oa/task", Secret: "s1", TenantID: "oa", CreateTime: now, UpdateTime: now}); err != nil {
		t.Fatalf("新增失败: %v", err)
	}
	// upsert：同代号覆盖业务列并保留原 ID
	if err := store.Save(ctx, &BusinessSystem{ID: "should-be-replaced", SystemSymbol: "oa", SystemName: "OA 二期", ExecutionCallAddr: "http://oa/exec", UpdateTime: now}); err != nil {
		t.Fatalf("upsert 失败: %v", err)
	}
	system, found, err := store.GetBySymbol(ctx, "oa")
	if err != nil || !found {
		t.Fatalf("按代号查询失败: %v", err)
	}
	if system.ID != "id1" {
		t.Errorf("upsert 应保留原 ID，实际 %s", system.ID)
	}
	if system.SystemName != "OA 二期" || system.ExecutionCallAddr != "http://oa/exec" || system.TaskCallAddr != "" {
		t.Errorf("upsert 覆盖结果不符: %+v", system)
	}
	// 查无
	if _, found, _ := store.GetBySymbol(ctx, "nope"); found {
		t.Error("查无代号不应命中")
	}
	// 按租户查询
	if _, found, _ := store.GetByTenant(ctx, "nope"); found {
		t.Error("查无租户不应命中")
	}
	// 追加一条不同租户的注册，验证按租户查询与列表排序
	if err := store.Save(ctx, &BusinessSystem{ID: "id2", SystemSymbol: "crm", TenantID: "crm-tenant", CreateTime: now, UpdateTime: now}); err != nil {
		t.Fatalf("新增第二条失败: %v", err)
	}
	byTenant, found, err := store.GetByTenant(ctx, "crm-tenant")
	if err != nil || !found || byTenant.SystemSymbol != "crm" {
		t.Fatalf("按租户查询失败: %v found=%v", err, found)
	}
	all, err := store.List(ctx)
	if err != nil {
		t.Fatalf("列表失败: %v", err)
	}
	if len(all) != 2 || all[0].SystemSymbol != "crm" || all[1].SystemSymbol != "oa" {
		t.Errorf("列表应按代号升序: %+v", all)
	}
	// 删除与删除不存在
	if err := store.Delete(ctx, "oa"); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if err := store.Delete(ctx, "oa"); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("删除不存在应报 not found，实际 %v", err)
	}
}

// TestBusinessSystemServiceRegister 验证注册流程：唯一校验、ID/密钥生成、租户缺省与注入依赖。
// @param t 测试上下文
func TestBusinessSystemServiceRegister(t *testing.T) {
	store := NewMemoryBusinessSystemStore()
	now := newFixedTime()
	svc := NewBusinessSystemService(store,
		WithBusinessSystemIDs(idgen.NewUUIDv7()),
		WithBusinessSystemNow(func() time.Time { return now }),
	)
	ctx := context.Background()

	// 空入参与空代号
	if _, err := svc.Register(ctx, nil); err == nil {
		t.Error("nil 入参应报错")
	}
	if _, err := svc.Register(ctx, &BusinessSystem{}); err == nil {
		t.Error("空代号应报错")
	}
	// 正常注册：ID/密钥自动生成，租户缺省取代号
	system, err := svc.Register(ctx, &BusinessSystem{SystemSymbol: "oa", SystemName: "OA 系统", TaskCallAddr: "http://oa/task", Creator: "admin", CreatorName: "管理员"})
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	if system.ID == "" || system.Secret == "" {
		t.Errorf("注册应自动生成 ID 与密钥: %+v", system)
	}
	if system.TenantID != "oa" {
		t.Errorf("租户缺省应为系统代号，实际 %s", system.TenantID)
	}
	if !system.CreateTime.Equal(now) || system.UpdateTime != system.CreateTime || system.Updater != "admin" {
		t.Errorf("时间与审计字段不符: %+v", system)
	}
	// 重复代号注册应冲突
	if _, err := svc.Register(ctx, &BusinessSystem{SystemSymbol: "oa"}); !errors.Is(err, api.ErrConflict) {
		t.Errorf("重复注册应报 conflict，实际 %v", err)
	}
	// 显式指定租户与密钥时尊重原值
	custom, err := svc.Register(ctx, &BusinessSystem{SystemSymbol: "crm", TenantID: "crm-tenant", Secret: "preset-secret"})
	if err != nil {
		t.Fatalf("注册 crm 失败: %v", err)
	}
	if custom.TenantID != "crm-tenant" || custom.Secret != "preset-secret" {
		t.Errorf("显式租户/密钥应被尊重: %+v", custom)
	}
}

// TestBusinessSystemServiceSecretInjection 验证密钥生成器注入与失败路径。
// @param t 测试上下文
func TestBusinessSystemServiceSecretInjection(t *testing.T) {
	store := NewMemoryBusinessSystemStore()
	ctx := context.Background()
	// 注入固定密钥生成器
	svc := NewBusinessSystemService(store, WithBusinessSystemSecrets(func() (string, error) { return "fixed-secret", nil }))
	system, err := svc.Register(ctx, &BusinessSystem{SystemSymbol: "oa"})
	if err != nil || system.Secret != "fixed-secret" {
		t.Fatalf("注入密钥生成器未生效: %v %+v", err, system)
	}
	// 注入失败的生成器：注册失败且不落库
	svc = NewBusinessSystemService(store, WithBusinessSystemSecrets(func() (string, error) { return "", errors.New("boom") }))
	if _, err := svc.Register(ctx, &BusinessSystem{SystemSymbol: "crm"}); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("密钥生成失败应上抛，实际 %v", err)
	}
	if _, found, _ := store.GetBySymbol(ctx, "crm"); found {
		t.Error("密钥生成失败的注册不应落库")
	}
	// nil 选项不覆盖默认值（默认密钥 32 位 hex）
	svc = NewBusinessSystemService(store, WithBusinessSystemSecrets(nil), WithBusinessSystemIDs(nil), WithBusinessSystemNow(nil))
	system, err = svc.Register(ctx, &BusinessSystem{SystemSymbol: "erp"})
	if err != nil || len(system.Secret) != 32 {
		t.Fatalf("默认密钥应为 32 位 hex: %v %+v", err, system)
	}
}

// TestDefaultSecretGenerator 验证默认密钥生成长度与随机性。
// @param t 测试上下文
func TestDefaultSecretGenerator(t *testing.T) {
	first, err := defaultSecretGenerator()
	if err != nil {
		t.Fatalf("默认密钥生成失败: %v", err)
	}
	second, _ := defaultSecretGenerator()
	if len(first) != 32 || first == second {
		t.Errorf("默认密钥应为 32 位且随机: %s / %s", first, second)
	}
}

// TestBusinessSystemServiceUpdate 验证更新流程：定位、非空覆盖、审计字段。
// @param t 测试上下文
func TestBusinessSystemServiceUpdate(t *testing.T) {
	store := NewMemoryBusinessSystemStore()
	now := newFixedTime()
	later := now.Add(time.Hour)
	svc := NewBusinessSystemService(store, WithBusinessSystemNow(func() time.Time { return now }))
	ctx := context.Background()
	original, err := svc.Register(ctx, &BusinessSystem{SystemSymbol: "oa", SystemName: "旧名", TaskCallAddr: "http://old/task", ExecutionCallAddr: "http://old/exec", Creator: "admin"})
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	// 更新入参校验
	if _, err := svc.Update(ctx, nil); err == nil {
		t.Error("nil 更新入参应报错")
	}
	if _, err := svc.Update(ctx, &BusinessSystem{}); err == nil {
		t.Error("空代号应报错")
	}
	if _, err := svc.Update(ctx, &BusinessSystem{SystemSymbol: "nope"}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("更新不存在应报 not found，实际 %v", err)
	}
	// 更新：仅覆盖非空字段（回调地址置空语义为"不修改"）
	svc2 := NewBusinessSystemService(store, WithBusinessSystemNow(func() time.Time { return later }))
	updated, err := svc2.Update(ctx, &BusinessSystem{SystemSymbol: "oa", SystemName: "新名", TaskCallAddr: "http://new/task", Updater: "editor"})
	if err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if updated.SystemName != "新名" || updated.TaskCallAddr != "http://new/task" {
		t.Errorf("非空字段应被覆盖: %+v", updated)
	}
	if updated.ExecutionCallAddr != "http://old/exec" {
		t.Errorf("空字段不应覆盖原值: %+v", updated)
	}
	if updated.ID != original.ID || updated.CreateTime != original.CreateTime {
		t.Errorf("ID 与创建时间应保持不变: %+v", updated)
	}
	if updated.UpdateTime != later || updated.Updater != "editor" {
		t.Errorf("更新时间与更新人不符: %+v", updated)
	}
}

// TestBusinessSystemServiceGetListRemove 验证查询/列表/注销。
// @param t 测试上下文
func TestBusinessSystemServiceGetListRemove(t *testing.T) {
	svc := NewBusinessSystemService(NewMemoryBusinessSystemStore())
	ctx := context.Background()
	if _, err := svc.Get(ctx, "oa"); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("查询不存在应报 not found，实际 %v", err)
	}
	registered, err := svc.Register(ctx, &BusinessSystem{SystemSymbol: "oa", SystemName: "OA"})
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	got, err := svc.Get(ctx, "oa")
	if err != nil || got.ID != registered.ID {
		t.Fatalf("查询失败: %v", err)
	}
	all, err := svc.List(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("列表失败: %v", err)
	}
	if err := svc.Remove(ctx, "oa"); err != nil {
		t.Fatalf("注销失败: %v", err)
	}
	if err := svc.Remove(ctx, "oa"); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("注销不存在应报 not found，实际 %v", err)
	}
}
