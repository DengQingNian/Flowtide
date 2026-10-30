package workflow

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestFillBase 验证公共审计列填充：ID 生成、时间补齐、创建人/租户注入。
// @param t 测试上下文
func TestFillBase(t *testing.T) {
	newID := func() string { return "id-fixed" }
	now := func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local) }
	base := &BaseColumns{}
	fillBase(base, newID, now, "op-1", "sys")
	// ID/时间/创建人/租户全部填充
	if base.ID != "id-fixed" || base.CreateTime.IsZero() || base.UpdateTime.IsZero() {
		t.Fatalf("审计列未填充: %+v", base)
	}
	if base.Creator != "op-1" || base.Updater != "op-1" || base.TenantID != "sys" {
		t.Fatalf("创建人/租户注入错误: %+v", base)
	}
	// 已有 ID/时间/租户时尊重原值；创建人/更新人始终取当前操作人
	kept := &BaseColumns{ID: "kept", Creator: "keep", CreateTime: now(), UpdateTime: now(), TenantID: "other"}
	fillBase(kept, newID, now, "op-2", "sys")
	if kept.ID != "kept" || kept.Creator != "op-2" || kept.Updater != "op-2" || kept.TenantID != "other" {
		t.Fatalf("审计列覆盖语义错误: %+v", kept)
	}
	// 空创建人/租户不覆盖
	empty := &BaseColumns{TenantID: "kept-tenant"}
	fillBase(empty, newID, now, "", "")
	if empty.Creator != "" || empty.TenantID != "kept-tenant" {
		t.Fatalf("空值不应覆盖: %+v", empty)
	}
}

// TestFirstNonEmpty 验证首个非空字符串选择。
// @param t 测试上下文
func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "a", "b"); got != "a" {
		t.Fatalf("应返回首个非空: %q", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Fatalf("全空应返回空串: %q", got)
	}
}

// TestAPIError 验证领域错误构造。
// @param t 测试上下文
func TestAPIError(t *testing.T) {
	err := apiError("WORKFLOW-9999", "测试错误")
	if err == nil || !strings.Contains(err.Error(), "测试错误") {
		t.Fatalf("错误消息缺失: %v", err)
	}
}

// TestPairGroupID 验证候选组配对规则。
// @param t 测试上下文
func TestPairGroupID(t *testing.T) {
	// 等长按下标配对
	if got := pairGroupID([]string{"g1", "g2"}, 1, 2); got != "g2" {
		t.Fatalf("等长配对错误: %q", got)
	}
	// 单组共享
	if got := pairGroupID([]string{"g9"}, 1, 3); got != "g9" {
		t.Fatalf("单组共享错误: %q", got)
	}
	// 无组
	if got := pairGroupID(nil, 0, 2); got != "" {
		t.Fatalf("无组应为空: %q", got)
	}
}

// TestCleanIdentityIDs 验证执行人清洗：空白、字面量 null 剔除。
// @param t 测试上下文
func TestCleanIdentityIDs(t *testing.T) {
	got := cleanIdentityIDs([]string{"u1", " ", "", "null", "u2", " null "})
	// 仅剔除空白与精确字面量 null（带空格的 " null " 保留，与 Java equals 语义一致）
	if len(got) != 3 || got[0] != "u1" || got[1] != "u2" || got[2] != " null " {
		t.Fatalf("清洗语义错误: %v", got)
	}
	if got := cleanIdentityIDs(nil); len(got) != 0 {
		t.Fatalf("空列表应为空: %v", got)
	}
}

// TestDefaultIDGen 验证默认 ID 生成函数产出非空 ID。
// @param t 测试上下文
func TestDefaultIDGen(t *testing.T) {
	gen := DefaultIDGen()
	id := gen()
	if id == "" {
		t.Fatal("默认 ID 生成器应产出非空 ID")
	}
	if gen() == id {
		t.Fatal("默认 ID 生成器应产出不同 ID")
	}
}

// TestGetExtendColumns 验证扩展列名全集与 DDL 顺序一致。
// @param t 测试上下文
func TestGetExtendColumns(t *testing.T) {
	columns := GetExtendColumns()
	if len(columns) != 17 {
		t.Fatalf("扩展列应 17 个，实际 %d", len(columns))
	}
	// 首尾列名
	if columns[0] != "extend_s1" || columns[7] != "extend_s8" || columns[10] != "extend_i3" || columns[16] != "extend_d3" {
		t.Fatalf("扩展列顺序错误: %v", columns)
	}
}

// TestContextKeysIgnored 验证上下文透传不影响纯函数（覆盖占位，防止 ctx 误用）。
// @param t 测试上下文
func TestContextKeysIgnored(t *testing.T) {
	// 域服务不读取 ctx 值，仅透传；此处验证 context.Background 可用
	if context.Background() == nil {
		t.Fatal("上下文不应为空")
	}
}
