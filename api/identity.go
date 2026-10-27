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
	FailedAttempts int        `json:"-"`                     // 连续失败次数
	LockedUntil    *time.Time `json:"-"`                     // 锁定截止时间
	TenantID       string     `json:"tenantId,omitempty"`    // 租户
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
	// DeleteUser 删除用户。
	// @param ctx 上下文
	// @param userID 用户 ID
	// @return 删除错误
	DeleteUser(ctx context.Context, userID string) error
	// CreateGroup 创建组。
	// @param ctx 上下文
	// @param group 组
	// @return 组
	// @return 创建错误
	CreateGroup(ctx context.Context, group Group) (Group, error)
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
	// ListMembers 返回组成员。
	// @param ctx 上下文
	// @param groupID 组 ID
	// @return 用户列表
	// @return 查询错误
	ListMembers(ctx context.Context, groupID string) ([]User, error)
}
