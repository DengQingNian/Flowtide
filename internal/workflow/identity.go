// identity.go 提供身份门面（D10）：基于引擎 api.IdentityService 封装管理台左树/人员/角色数据源，
// 对位 Java auth 服务（AccountIdentityService/OrgGroupService/RoleService 等）经 /workflow/identity/** 暴露的形态。
// 引擎身份库（ACT_ID_*）无组层级、无资源模型：缺数据处按 D10 决策降级（平铺/空树），
// 并预留 IdentityAdapter 接口供外部 auth 适配器接管（P6+）。
package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
)

// 身份树节点类型常量。
const (
	// IdentityNodeRoot 虚拟根节点。
	IdentityNodeRoot = "root"
	// IdentityNodeGroup 组节点。
	IdentityNodeGroup = "group"
	// IdentityNodeRole 角色节点。
	IdentityNodeRole = "role"
	// IdentityNodeUser 用户节点。
	IdentityNodeUser = "user"
	// IdentityNodeResource 资源节点。
	IdentityNodeResource = "resource"
)

// IdentityNode 是身份树节点（用户树/组织树/资源树通用）。
type IdentityNode struct {
	ID       string         `json:"id"`                 // 节点 ID（组/用户/角色 ID，虚拟根用固定代号）
	ParentID string         `json:"parentId,omitempty"` // 父节点 ID（根为空）
	Name     string         `json:"name"`               // 节点显示名
	Type     string         `json:"type"`               // 节点类型（root/group/role/user/resource）
	Children []IdentityNode `json:"children,omitempty"` // 子节点
}

// IdentityAccount 是账号搜索结果视图。
type IdentityAccount struct {
	ID       string   `json:"id"`                 // 用户 ID
	Name     string   `json:"name"`               // 显示名
	Email    string   `json:"email,omitempty"`    // 邮箱
	Roles    []string `json:"roles,omitempty"`    // 角色 ID 列表
	Groups   []string `json:"groups,omitempty"`   // 所属组 ID 列表
	Disabled bool     `json:"disabled,omitempty"` // 是否禁用
	TenantID string   `json:"tenantId,omitempty"` // 租户
}

// IdentityAdapter 是外部身份适配器接口（D10 预留）：
// 企业已有独立 auth/组织中心时，实现本接口注入 IdentityFacade，即可整体替换内置身份库数据源；
// 本阶段不提供实现，引擎内置身份库为默认数据源。
type IdentityAdapter interface {
	// UserTree 返回外部用户树。
	// @param ctx 操作上下文
	// @return 树节点列表
	// @return 查询错误
	UserTree(ctx context.Context) ([]IdentityNode, error)
	// OrgTree 返回外部组织树。
	// @param ctx 操作上下文
	// @return 树节点列表
	// @return 查询错误
	OrgTree(ctx context.Context) ([]IdentityNode, error)
	// ResourceTree 返回外部资源树。
	// @param ctx 操作上下文
	// @return 树节点列表
	// @return 查询错误
	ResourceTree(ctx context.Context) ([]IdentityNode, error)
	// SearchAccount 按关键字模糊搜索账号。
	// @param ctx 操作上下文
	// @param keyword 关键字（姓名/ID）
	// @return 账号列表
	// @return 查询错误
	SearchAccount(ctx context.Context, keyword string) ([]IdentityAccount, error)
}

// IdentityFacadeOptions 是身份门面的可注入配置。
type IdentityFacadeOptions struct {
	Adapter IdentityAdapter // 外部身份适配器（nil 时用引擎内置身份库）
}

// IdentityOption 是身份门面选项函数。
type IdentityOption func(*IdentityFacadeOptions)

// WithIdentityAdapter 注入外部身份适配器（D10 预留，优先于引擎内置身份库）。
// @param adapter 适配器
// @return 选项
func WithIdentityAdapter(adapter IdentityAdapter) IdentityOption {
	return func(o *IdentityFacadeOptions) {
		if adapter != nil {
			o.Adapter = adapter
		}
	}
}

// IdentityFacade 是身份门面（D10）：用户树/组织树/角色/组/账号搜索/资源树。
type IdentityFacade struct {
	identity api.IdentityService // 引擎身份服务
	adapter  IdentityAdapter     // 外部适配器（可空）
}

// 身份门面错误码。
const errCodeIdentityUnavailable = "WORKFLOW-2301"

// NewIdentityFacade 创建身份门面。
// @param engine 引擎门面（七服务）
// @param options 可选配置（外部适配器注入）
// @return 身份门面
func NewIdentityFacade(engine api.Engine, options ...IdentityOption) *IdentityFacade {
	opts := IdentityFacadeOptions{}
	for _, option := range options {
		option(&opts)
	}
	var identity api.IdentityService
	// engine 为 nil 时 identity 保持 nil，由各方法统一返回"不可用"错误
	if engine != nil {
		identity = engine.IdentityService()
	}
	return &IdentityFacade{identity: identity, adapter: opts.Adapter}
}

// UserTree 返回用户树：按组维度组织（组节点 → 成员用户节点），未入组用户挂"未分组"节点。
// 引擎组无父子层级（ACT_ID_GROUP 无 parent 列），树深固定两层；完全缺组数据时退化为平铺用户列表。
// @param ctx 操作上下文
// @return 树节点列表（单根）
// @return 引擎查询错误
func (f *IdentityFacade) UserTree(ctx context.Context) ([]IdentityNode, error) {
	// 外部适配器优先（D10 预留路径）
	if f.adapter != nil {
		return f.adapter.UserTree(ctx)
	}
	if f.identity == nil {
		return nil, identityUnavailableErr()
	}
	users, err := f.identity.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	groups, err := f.identity.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	root := IdentityNode{ID: "users", Name: "用户", Type: IdentityNodeRoot}
	// 完全缺组数据：退化为平铺用户列表（与 Java 组织树体验的最小可用替代）
	if len(groups) == 0 {
		for _, user := range users {
			root.Children = append(root.Children, userNode(user, root.ID))
		}
		return []IdentityNode{root}, nil
	}
	grouped := make(map[string]bool)
	for _, group := range groups {
		members, err := f.identity.ListMembers(ctx, group.ID)
		if err != nil {
			return nil, err
		}
		groupNode := IdentityNode{ID: group.ID, ParentID: root.ID, Name: groupName(group), Type: IdentityNodeGroup}
		for _, member := range members {
			groupNode.Children = append(groupNode.Children, userNode(member, group.ID))
			grouped[member.ID] = true
		}
		root.Children = append(root.Children, groupNode)
	}
	// 未入组用户归入"未分组"节点，保证全量用户可见
	var orphans []IdentityNode
	for _, user := range users {
		if grouped[user.ID] {
			continue
		}
		orphans = append(orphans, userNode(user, "ungrouped"))
	}
	if len(orphans) > 0 {
		root.Children = append(root.Children, IdentityNode{ID: "ungrouped", ParentID: root.ID, Name: "未分组", Type: IdentityNodeGroup, Children: orphans})
	}
	return []IdentityNode{root}, nil
}

// OrgTree 返回组织树：引擎组无父子层级（D10 降级），平铺为单根 + 一层组节点。
// @param ctx 操作上下文
// @return 树节点列表（单根）
// @return 引擎查询错误
func (f *IdentityFacade) OrgTree(ctx context.Context) ([]IdentityNode, error) {
	// 外部适配器优先（D10 预留路径）
	if f.adapter != nil {
		return f.adapter.OrgTree(ctx)
	}
	if f.identity == nil {
		return nil, identityUnavailableErr()
	}
	groups, err := f.identity.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	root := IdentityNode{ID: "org", Name: "组织", Type: IdentityNodeRoot}
	// 引擎无层级数据：全部组平铺挂在根下，后续引入层级数据源（IdentityAdapter）时再建真实树
	for _, group := range groups {
		root.Children = append(root.Children, IdentityNode{ID: group.ID, ParentID: root.ID, Name: groupName(group), Type: IdentityNodeGroup})
	}
	return []IdentityNode{root}, nil
}

// RoleList 返回角色列表。
// @param ctx 操作上下文
// @return 角色列表
// @return 引擎查询错误
func (f *IdentityFacade) RoleList(ctx context.Context) ([]api.Role, error) {
	if f.identity == nil {
		return nil, identityUnavailableErr()
	}
	return f.identity.ListRoles(ctx)
}

// GroupList 返回组列表。
// @param ctx 操作上下文
// @return 组列表
// @return 引擎查询错误
func (f *IdentityFacade) GroupList(ctx context.Context) ([]api.Group, error) {
	if f.identity == nil {
		return nil, identityUnavailableErr()
	}
	return f.identity.ListGroups(ctx)
}

// AccountSearch 按关键字模糊搜索账号（命中用户 ID/显示名/姓/名/邮箱，不区分大小写）。
// 关键字为空时返回全部用户。
// @param ctx 操作上下文
// @param keyword 关键字（姓名/ID 模糊）
// @return 账号列表
// @return 引擎查询错误
func (f *IdentityFacade) AccountSearch(ctx context.Context, keyword string) ([]IdentityAccount, error) {
	// 外部适配器优先（D10 预留路径）
	if f.adapter != nil {
		return f.adapter.SearchAccount(ctx, keyword)
	}
	if f.identity == nil {
		return nil, identityUnavailableErr()
	}
	users, err := f.identity.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(strings.TrimSpace(keyword))
	out := make([]IdentityAccount, 0, len(users))
	for _, user := range users {
		// 空关键字返回全量；否则按 ID/姓名/邮箱包含匹配
		if needle != "" && !accountMatches(user, needle) {
			continue
		}
		account := IdentityAccount{
			ID:       user.ID,
			Name:     userName(user),
			Email:    user.Email,
			Roles:    user.Roles,
			Disabled: user.Disabled,
			TenantID: user.TenantID,
		}
		// 补充所属组 ID（内存身份库查询代价可接受；外部适配器应自行优化）
		groups, groupErr := f.identity.ListUserGroups(ctx, user.ID)
		if groupErr != nil {
			return nil, groupErr
		}
		for _, group := range groups {
			account.Groups = append(account.Groups, group.ID)
		}
		out = append(out, account)
	}
	return out, nil
}

// ResourceTree 返回资源树：本阶段引擎无资源/菜单模型，返回单根空树（D10 预留，
// 外部 auth 适配器实现 IdentityAdapter.ResourceTree 后即有真实数据）。
// @param ctx 操作上下文
// @return 树节点列表（单根，无子节点）
// @return 引擎查询错误
func (f *IdentityFacade) ResourceTree(ctx context.Context) ([]IdentityNode, error) {
	// 外部适配器优先（D10 预留路径）
	if f.adapter != nil {
		return f.adapter.ResourceTree(ctx)
	}
	// 引擎身份库只有用户/组/角色，无资源模型：空树占位，避免前端左树报错
	return []IdentityNode{{ID: "resources", Name: "资源", Type: IdentityNodeRoot}}, nil
}

// userNode 构造用户树节点。
// @param user 引擎用户
// @param parentID 父节点 ID
// @return 树节点
func userNode(user api.User, parentID string) IdentityNode {
	return IdentityNode{ID: user.ID, ParentID: parentID, Name: userName(user), Type: IdentityNodeUser}
}

// userName 返回用户显示名（显示名 → 姓+名 → ID 逐级回退）。
// @param user 引擎用户
// @return 显示名
func userName(user api.User) string {
	if strings.TrimSpace(user.DisplayName) != "" {
		return user.DisplayName
	}
	// 无显示名时拼接姓名；姓名也为空则回退用户 ID
	full := strings.TrimSpace(user.LastName + user.FirstName)
	if full != "" {
		return full
	}
	return user.ID
}

// groupName 返回组显示名（组名 → ID 回退）。
// @param group 引擎组
// @return 显示名
func groupName(group api.Group) string {
	if strings.TrimSpace(group.Name) != "" {
		return group.Name
	}
	return group.ID
}

// accountMatches 判断用户是否命中搜索关键字（ID/显示名/姓/名/邮箱，不区分大小写）。
// @param user 引擎用户
// @param needle 小写关键字
// @return 是否命中
func accountMatches(user api.User, needle string) bool {
	// 任一字段包含关键字即命中
	for _, field := range []string{user.ID, user.DisplayName, user.FirstName, user.LastName, user.Email} {
		if strings.Contains(strings.ToLower(field), needle) {
			return true
		}
	}
	return false
}

// identityUnavailableErr 返回身份服务不可用错误（测试与调用方共用文案）。
// @return 错误
func identityUnavailableErr() error {
	return api.NewError(errCodeIdentityUnavailable, "引擎身份服务不可用", fmt.Errorf("engine is nil"))
}
