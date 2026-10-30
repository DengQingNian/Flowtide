// identity_test.go 验证身份门面（D10）：引擎内置身份库的树构建/降级平铺/搜索/角色组列表、外部适配器委托与 nil 引擎守卫。
package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
	"github.com/DengQingNian/Flowtide/internal/engine"
	"github.com/DengQingNian/Flowtide/internal/idgen"
)

// newIdentityTestEngine 创建预置身份数据的测试引擎（用户/组/角色/成员关系）。
// @param t 测试上下文
// @return 引擎
func newIdentityTestEngine(t *testing.T) *engine.Engine {
	t.Helper()
	eng, err := engine.New(engine.Config{
		HistoryLevel: engine.HistoryAudit,
		Clock:        clock.NewFake(time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)),
		IDs:          idgen.NewUUIDv7(),
	})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	ctx := context.Background()
	// 用户：张三（入研发组）、李四（入研发组）、王五（未入组、禁用）
	for _, user := range []api.User{
		{ID: "u-alice", DisplayName: "张三", FirstName: "三", LastName: "张", Email: "Alice@Example.com", Roles: []string{"r-admin"}},
		{ID: "u-bob", DisplayName: "李四", Email: "bob@example.com"},
		{ID: "u-carol", DisplayName: "王五", Disabled: true},
	} {
		if _, err := eng.CreateUser(ctx, user); err != nil {
			t.Fatalf("预置用户失败: %v", err)
		}
	}
	// 组与成员关系
	for _, group := range []api.Group{{ID: "g-dev", Name: "研发组", Type: "org"}, {ID: "g-qa", Name: "测试组", Type: "org"}} {
		if _, err := eng.CreateGroup(ctx, group); err != nil {
			t.Fatalf("预置组失败: %v", err)
		}
	}
	for _, pair := range [][2]string{{"u-alice", "g-dev"}, {"u-bob", "g-dev"}} {
		if err := eng.AddMembership(ctx, pair[0], pair[1]); err != nil {
			t.Fatalf("预置成员关系失败: %v", err)
		}
	}
	// 角色
	for _, role := range []api.Role{{ID: "r-admin", Name: "管理员"}, {ID: "r-user", Name: "普通用户"}} {
		if _, err := eng.CreateRole(ctx, role); err != nil {
			t.Fatalf("预置角色失败: %v", err)
		}
	}
	return eng
}

// findNode 按类型在根节点列表的一层子节点中查找节点。
// @param nodes 根节点列表
// @param nodeType 节点类型
// @return 命中节点
// @return 是否命中
func findNode(nodes []IdentityNode, nodeType string) (IdentityNode, bool) {
	for _, node := range nodes {
		if node.Type == nodeType {
			return node, true
		}
	}
	return IdentityNode{}, false
}

// findChild 在节点子层按 ID 查找。
// @param node 父节点
// @param id 子节点 ID
// @return 命中节点
// @return 是否命中
func findChild(node IdentityNode, id string) (IdentityNode, bool) {
	for _, child := range node.Children {
		if child.ID == id {
			return child, true
		}
	}
	return IdentityNode{}, false
}

// TestIdentityFacadeUserTree 验证用户树按组组织与未分组兜底。
// @param t 测试上下文
func TestIdentityFacadeUserTree(t *testing.T) {
	facade := NewIdentityFacade(newIdentityTestEngine(t))
	tree, err := facade.UserTree(context.Background())
	if err != nil {
		t.Fatalf("用户树失败: %v", err)
	}
	if len(tree) != 1 || tree[0].Type != IdentityNodeRoot || tree[0].ID != "users" {
		t.Fatalf("应返回单根用户树: %+v", tree)
	}
	root := tree[0]
	// 组节点：研发组 2 名成员，测试组 0 名成员
	dev, ok := findChild(root, "g-dev")
	if !ok || dev.Type != IdentityNodeGroup || dev.Name != "研发组" || len(dev.Children) != 2 {
		t.Fatalf("研发组节点不符: %+v", dev)
	}
	if dev.Children[0].ID != "u-alice" || dev.Children[0].Name != "张三" || dev.Children[0].Type != IdentityNodeUser {
		t.Errorf("研发组成员不符: %+v", dev.Children[0])
	}
	qa, ok := findChild(root, "g-qa")
	if !ok || len(qa.Children) != 0 {
		t.Errorf("测试组应为空组节点: %+v", qa)
	}
	// 未分组节点兜底王五
	ungrouped, ok := findChild(root, "ungrouped")
	if !ok || len(ungrouped.Children) != 1 || ungrouped.Children[0].ID != "u-carol" {
		t.Errorf("未分组节点应含王五: %+v", ungrouped)
	}
}

// TestIdentityFacadeUserTreeFlatFallback 验证缺组数据时用户树退化为平铺列表。
// @param t 测试上下文
func TestIdentityFacadeUserTreeFlatFallback(t *testing.T) {
	eng, err := engine.New(engine.Config{
		HistoryLevel: engine.HistoryAudit,
		Clock:        clock.NewFake(time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)),
		IDs:          idgen.NewUUIDv7(),
	})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	ctx := context.Background()
	if _, err := eng.CreateUser(ctx, api.User{ID: "u-solo", DisplayName: "独行者"}); err != nil {
		t.Fatalf("预置用户失败: %v", err)
	}
	facade := NewIdentityFacade(eng)
	tree, err := facade.UserTree(ctx)
	if err != nil {
		t.Fatalf("用户树失败: %v", err)
	}
	// 无组数据：用户节点直接平铺在根下（降级路径）
	if len(tree) != 1 || len(tree[0].Children) != 1 || tree[0].Children[0].ID != "u-solo" || tree[0].Children[0].Type != IdentityNodeUser {
		t.Errorf("缺组数据应平铺用户列表: %+v", tree)
	}
}

// TestIdentityFacadeOrgTree 验证组织树平铺（引擎无组层级，D10 降级）。
// @param t 测试上下文
func TestIdentityFacadeOrgTree(t *testing.T) {
	facade := NewIdentityFacade(newIdentityTestEngine(t))
	tree, err := facade.OrgTree(context.Background())
	if err != nil {
		t.Fatalf("组织树失败: %v", err)
	}
	if len(tree) != 1 || tree[0].ID != "org" || tree[0].Type != IdentityNodeRoot {
		t.Fatalf("应返回单根组织树: %+v", tree)
	}
	if len(tree[0].Children) != 2 {
		t.Fatalf("组织树应平铺 2 个组: %+v", tree[0])
	}
	for _, child := range tree[0].Children {
		// 引擎组无层级：全部组直接挂根，无孙节点
		if child.Type != IdentityNodeGroup || child.ParentID != "org" || len(child.Children) != 0 {
			t.Errorf("组织树应平铺组节点: %+v", child)
		}
	}
}

// TestIdentityFacadeRoleListGroupList 验证角色与组列表透传。
// @param t 测试上下文
func TestIdentityFacadeRoleListGroupList(t *testing.T) {
	facade := NewIdentityFacade(newIdentityTestEngine(t))
	ctx := context.Background()
	roles, err := facade.RoleList(ctx)
	if err != nil || len(roles) != 2 || roles[0].ID != "r-admin" || roles[0].Name != "管理员" {
		t.Errorf("角色列表不符: %v %v", roles, err)
	}
	groups, err := facade.GroupList(ctx)
	if err != nil || len(groups) != 2 || groups[0].ID != "g-dev" {
		t.Errorf("组列表不符: %v %v", groups, err)
	}
}

// TestIdentityFacadeAccountSearch 验证账号搜索（姓名/ID/邮箱、大小写不敏感、空关键字全量）。
// @param t 测试上下文
func TestIdentityFacadeAccountSearch(t *testing.T) {
	facade := NewIdentityFacade(newIdentityTestEngine(t))
	ctx := context.Background()
	// 按显示名
	got, err := facade.AccountSearch(ctx, "张三")
	if err != nil || len(got) != 1 || got[0].ID != "u-alice" {
		t.Fatalf("按姓名搜索不符: %v %v", got, err)
	}
	// 角色与所属组回填
	if len(got[0].Roles) != 1 || got[0].Roles[0] != "r-admin" {
		t.Errorf("账号应携带角色: %+v", got[0])
	}
	if len(got[0].Groups) != 1 || got[0].Groups[0] != "g-dev" {
		t.Errorf("账号应携带所属组: %+v", got[0])
	}
	// 按 ID 子串
	if got, err := facade.AccountSearch(ctx, "u-b"); err != nil || len(got) != 1 || got[0].ID != "u-bob" {
		t.Errorf("按 ID 搜索不符: %v %v", got, err)
	}
	// 邮箱大小写不敏感
	if got, err := facade.AccountSearch(ctx, "ALICE@"); err != nil || len(got) != 1 || got[0].ID != "u-alice" {
		t.Errorf("邮箱搜索应不区分大小写: %v %v", got, err)
	}
	// 空关键字返回全量（含禁用账号）
	if got, err := facade.AccountSearch(ctx, ""); err != nil || len(got) != 3 {
		t.Errorf("空关键字应返回全量: %v %v", got, err)
	}
	// 无命中
	if got, err := facade.AccountSearch(ctx, "不存在"); err != nil || len(got) != 0 {
		t.Errorf("无命中应返回空列表: %v %v", got, err)
	}
}

// TestIdentityFacadeResourceTreeEmpty 验证资源树本阶段返回空树（D10 预留）。
// @param t 测试上下文
func TestIdentityFacadeResourceTreeEmpty(t *testing.T) {
	facade := NewIdentityFacade(newIdentityTestEngine(t))
	tree, err := facade.ResourceTree(context.Background())
	if err != nil {
		t.Fatalf("资源树失败: %v", err)
	}
	// 引擎无资源模型：单根空树占位
	if len(tree) != 1 || tree[0].ID != "resources" || tree[0].Type != IdentityNodeRoot || len(tree[0].Children) != 0 {
		t.Errorf("资源树应为单根空树: %+v", tree)
	}
}

// fakeIdentityAdapter 捕获委托调用的外部适配器桩。
type fakeIdentityAdapter struct {
	userTreeCalled bool              // UserTree 是否被调用
	orgTreeCalled  bool              // OrgTree 是否被调用
	resourceCalled bool              // ResourceTree 是否被调用
	searchCalled   bool              // SearchAccount 是否被调用
	keyword        string            // 收到的搜索关键字
	nodes          []IdentityNode    // 桩返回的树
	accounts       []IdentityAccount // 桩返回的账号
}

// UserTree 桩实现。
// @param ctx 操作上下文
// @return 预置树
// @return 恒 nil
func (f *fakeIdentityAdapter) UserTree(ctx context.Context) ([]IdentityNode, error) {
	f.userTreeCalled = true
	return f.nodes, nil
}

// OrgTree 桩实现。
// @param ctx 操作上下文
// @return 预置树
// @return 恒 nil
func (f *fakeIdentityAdapter) OrgTree(ctx context.Context) ([]IdentityNode, error) {
	f.orgTreeCalled = true
	return f.nodes, nil
}

// ResourceTree 桩实现。
// @param ctx 操作上下文
// @return 预置树
// @return 恒 nil
func (f *fakeIdentityAdapter) ResourceTree(ctx context.Context) ([]IdentityNode, error) {
	f.resourceCalled = true
	return f.nodes, nil
}

// SearchAccount 桩实现。
// @param ctx 操作上下文
// @param keyword 关键字
// @return 预置账号
// @return 恒 nil
func (f *fakeIdentityAdapter) SearchAccount(ctx context.Context, keyword string) ([]IdentityAccount, error) {
	f.searchCalled = true
	f.keyword = keyword
	return f.accounts, nil
}

// TestIdentityFacadeAdapterDelegation 验证外部适配器注入后全部方法委托（D10 预留路径）。
// @param t 测试上下文
func TestIdentityFacadeAdapterDelegation(t *testing.T) {
	adapter := &fakeIdentityAdapter{
		nodes:    []IdentityNode{{ID: "ext-root", Name: "外部", Type: IdentityNodeRoot}},
		accounts: []IdentityAccount{{ID: "ext-user", Name: "外部账号"}},
	}
	// 传入 nil 引擎也能工作：适配器完全接管数据源
	facade := NewIdentityFacade(nil, WithIdentityAdapter(adapter))
	ctx := context.Background()
	tree, err := facade.UserTree(ctx)
	if err != nil || !adapter.userTreeCalled || tree[0].ID != "ext-root" {
		t.Errorf("UserTree 应委托适配器: %v %v", tree, err)
	}
	if tree, err = facade.OrgTree(ctx); err != nil || !adapter.orgTreeCalled || tree[0].ID != "ext-root" {
		t.Errorf("OrgTree 应委托适配器: %v %v", tree, err)
	}
	if tree, err = facade.ResourceTree(ctx); err != nil || !adapter.resourceCalled || tree[0].ID != "ext-root" {
		t.Errorf("ResourceTree 应委托适配器: %v %v", tree, err)
	}
	accounts, err := facade.AccountSearch(ctx, "kw")
	if err != nil || !adapter.searchCalled || adapter.keyword != "kw" || accounts[0].ID != "ext-user" {
		t.Errorf("AccountSearch 应委托适配器: %v %v", accounts, err)
	}
}

// TestIdentityFacadeNilEngine 验证 nil 引擎守卫与 nil 适配器忽略。
// @param t 测试上下文
func TestIdentityFacadeNilEngine(t *testing.T) {
	// WithIdentityAdapter(nil) 应被忽略（保持内置数据源路径）
	facade := NewIdentityFacade(nil, WithIdentityAdapter(nil))
	ctx := context.Background()
	for name, call := range map[string]func() error{
		"UserTree":      func() error { _, err := facade.UserTree(ctx); return err },
		"OrgTree":       func() error { _, err := facade.OrgTree(ctx); return err },
		"RoleList":      func() error { _, err := facade.RoleList(ctx); return err },
		"GroupList":     func() error { _, err := facade.GroupList(ctx); return err },
		"AccountSearch": func() error { _, err := facade.AccountSearch(ctx, "kw"); return err },
	} {
		err := call()
		var apiErr *api.Error
		if !errors.As(err, &apiErr) || apiErr.Code != errCodeIdentityUnavailable {
			t.Errorf("%s 在 nil 引擎下应返回 %s，实际 %v", name, errCodeIdentityUnavailable, err)
		}
	}
	// ResourceTree 不依赖身份服务：nil 引擎仍返回空树
	tree, err := facade.ResourceTree(ctx)
	if err != nil || len(tree) != 1 || tree[0].ID != "resources" {
		t.Errorf("nil 引擎资源树应返回空树: %v %v", tree, err)
	}
}

// TestIdentityHelperNaming 验证用户/组显示名回退规则。
// @param t 测试上下文
func TestIdentityHelperNaming(t *testing.T) {
	// 显示名优先
	if got := userName(api.User{ID: "u1", DisplayName: "显示", LastName: "姓", FirstName: "名"}); got != "显示" {
		t.Errorf("显示名应优先: %s", got)
	}
	// 姓+名回退
	if got := userName(api.User{ID: "u1", LastName: "张", FirstName: "三"}); got != "张三" {
		t.Errorf("应回退姓+名: %s", got)
	}
	// ID 兜底
	if got := userName(api.User{ID: "u1"}); got != "u1" {
		t.Errorf("应回退 ID: %s", got)
	}
	// 组名回退
	if got := groupName(api.Group{ID: "g1"}); got != "g1" {
		t.Errorf("组名应回退 ID: %s", got)
	}
	if got := groupName(api.Group{ID: "g1", Name: "组名"}); got != "组名" {
		t.Errorf("组名应优先: %s", got)
	}
}
