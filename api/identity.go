package api

import (
	"context"
	"time"
)

// User 是身份用户。
type User struct {
	ID             string     `json:"id"`                    // 用户 ID
	FirstName      string     `json:"firstName,omitempty"`   // 名
	LastName       string     `json:"lastName,omitempty"`    // 姓
	DisplayName    string     `json:"displayName,omitempty"` // 显示名
	Email          string     `json:"email,omitempty"`       // 邮箱
	Password       string     `json:"password,omitempty"`    // 明文密码（仅写入）
	PasswordHash   string     `json:"-"`                     // 密码哈希
	Roles          []string   `json:"roles,omitempty"`       // 角色
	Disabled       bool       `json:"disabled,omitempty"`    // 是否禁用
	FailedAttempts int        `json:"-"`                     // 连续失败次数
	LockedUntil    *time.Time `json:"-"`                     // 锁定截止时间
	TenantID       string     `json:"tenantId,omitempty"`    // 租户
}

// Role 是角色与权限。
type Role struct {
	ID          string   `json:"id"`                    // 角色 ID
	Name        string   `json:"name,omitempty"`        // 名称
	Permissions []string `json:"permissions,omitempty"` // 权限
	TenantID    string   `json:"tenantId,omitempty"`    // 租户
}

// Group 是身份组。
type Group struct {
	ID       string `json:"id"`                 // 组 ID
	Name     string `json:"name,omitempty"`     // 组名
	Type     string `json:"type,omitempty"`     // 组类型
	TenantID string `json:"tenantId,omitempty"` // 租户
}

// IdentityService 提供用户、组与成员关系管理。
type IdentityService interface {
	// CreateUser 创建用户。
	// @param ctx 上下文
	// @param user 用户
	// @return 用户
	// @return 创建错误
	CreateUser(ctx context.Context, user User) (User, error)
	// GetUser 查询用户。
	// @param ctx 上下文
	// @param userID 用户 ID
	// @return 用户
	// @return 查询错误
	GetUser(ctx context.Context, userID string) (User, error)
	// ListUsers 返回全部用户。
	// @param ctx 上下文
	// @return 用户列表
	// @return 查询错误
	ListUsers(ctx context.Context) ([]User, error)
	// UpdateUser 更新用户资料与角色。
	// @param ctx 上下文
	// @param user 用户
	// @return 用户
	// @return 更新错误
	UpdateUser(ctx context.Context, user User) (User, error)
	// GetUserInfo 返回用户信息键值。
	// @param ctx 上下文
	// @param userID 用户 ID
	// @return 信息映射
	// @return 查询错误
	GetUserInfo(ctx context.Context, userID string) (map[string]string, error)
	// GetUserInfoEntries 返回包含原版空值键的用户信息，并复制可空值。
	// @param ctx 上下文
	// @param userID 用户标识
	// @return 包含空值键的信息映射
	// @return 查询错误
	GetUserInfoEntries(ctx context.Context, userID string) (map[string]*string, error)
	// GetUserInfoValue 返回单个用户信息。
	// @param ctx 上下文
	// @param userID 用户 ID
	// @param key 键
	// @return 值
	// @return 查询错误
	GetUserInfoValue(ctx context.Context, userID, key string) (string, error)
	// SetUserInfo 设置用户信息。
	// @param ctx 上下文
	// @param userID 用户 ID
	// @param key 键
	// @param value 值
	// @return 设置错误
	SetUserInfo(ctx context.Context, userID, key, value string) error
	// DeleteUserInfo 删除用户信息。
	// @param ctx 上下文
	// @param userID 用户 ID
	// @param key 键
	// @return 删除错误
	DeleteUserInfo(ctx context.Context, userID, key string) error
	// GetUserPicture 返回用户头像。
	// @param ctx 上下文
	// @param userID 用户 ID
	// @return 图片数据
	// @return MIME 类型
	// @return 查询错误
	GetUserPicture(ctx context.Context, userID string) ([]byte, string, error)
	// SetUserPicture 设置用户头像。
	// @param ctx 上下文
	// @param userID 用户 ID
	// @param data 图片数据
	// @param mimeType MIME 类型
	// @return 设置错误
	SetUserPicture(ctx context.Context, userID string, data []byte, mimeType string) error
	// SetUserRoles 设置用户角色。
	// @param ctx 上下文
	// @param userID 用户 ID
	// @param roles 角色列表
	// @return 设置错误
	SetUserRoles(ctx context.Context, userID string, roles []string) error
	// DeleteUser 删除用户。
	// @param ctx 上下文
	// @param userID 用户 ID
	// @return 删除错误
	DeleteUser(ctx context.Context, userID string) error
	// CreateRole 创建角色。
	// @param ctx 上下文
	// @param role 角色
	// @return 角色
	// @return 创建错误
	CreateRole(ctx context.Context, role Role) (Role, error)
	// GetRole 查询角色。
	// @param ctx 上下文
	// @param roleID 角色 ID
	// @return 角色
	// @return 查询错误
	GetRole(ctx context.Context, roleID string) (Role, error)
	// ListRoles 返回全部角色。
	// @param ctx 上下文
	// @return 角色列表
	// @return 查询错误
	ListRoles(ctx context.Context) ([]Role, error)
	// UpdateRole 更新角色。
	// @param ctx 上下文
	// @param role 角色
	// @return 角色
	// @return 更新错误
	UpdateRole(ctx context.Context, role Role) (Role, error)
	// DeleteRole 删除角色。
	// @param ctx 上下文
	// @param roleID 角色 ID
	// @return 删除错误
	DeleteRole(ctx context.Context, roleID string) error
	// CreateGroup 创建组。
	// @param ctx 上下文
	// @param group 组
	// @return 组
	// @return 创建错误
	CreateGroup(ctx context.Context, group Group) (Group, error)
	// UpdateGroup 更新组。
	// @param ctx 上下文
	// @param group 组
	// @return 组
	// @return 更新错误
	UpdateGroup(ctx context.Context, group Group) (Group, error)
	// GetGroup 查询组。
	// @param ctx 上下文
	// @param groupID 组 ID
	// @return 组
	// @return 查询错误
	GetGroup(ctx context.Context, groupID string) (Group, error)
	// ListGroups 返回全部组。
	// @param ctx 上下文
	// @return 组列表
	// @return 查询错误
	ListGroups(ctx context.Context) ([]Group, error)
	// DeleteGroup 删除组。
	// @param ctx 上下文
	// @param groupID 组 ID
	// @return 删除错误
	DeleteGroup(ctx context.Context, groupID string) error
	// AddMembership 添加成员关系。
	// @param ctx 上下文
	// @param userID 用户 ID
	// @param groupID 组 ID
	// @return 添加错误
	AddMembership(ctx context.Context, userID, groupID string) error
	// RemoveMembership 移除成员关系。
	// @param ctx 上下文
	// @param userID 用户 ID
	// @param groupID 组 ID
	// @return 移除错误
	RemoveMembership(ctx context.Context, userID, groupID string) error
	// UpdatePassword 重置用户密码。
	// @param ctx 上下文
	// @param userID 用户 ID
	// @param newPassword 新密码
	// @return 更新错误
	UpdatePassword(ctx context.Context, userID, newPassword string) error
	// ChangePassword 修改用户密码。
	// @param ctx 上下文
	// @param userID 用户 ID
	// @param oldPassword 旧密码
	// @param newPassword 新密码
	// @return 修改错误
	ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error
	// CheckPassword 校验用户密码。
	// @param ctx 上下文
	// @param userID 用户 ID
	// @param password 密码
	// @return 校验错误
	CheckPassword(ctx context.Context, userID, password string) error
	// ListUserGroups 返回用户所属组。
	// @param ctx 上下文
	// @param userID 用户 ID
	// @return 组列表
	// @return 查询错误
	ListUserGroups(ctx context.Context, userID string) ([]Group, error)
	// ListMembers 返回组成员。
	// @param ctx 上下文
	// @param groupID 组 ID
	// @return 用户列表
	// @return 查询错误
	ListMembers(ctx context.Context, groupID string) ([]User, error)
}
