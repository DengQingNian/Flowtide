package engine

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/auth"
)

// IdentityService 返回身份服务。
// @return 身份服务
func (e *Engine) IdentityService() api.IdentityService { return e }

// CreateUser 创建用户。
// @param ctx 上下文
// @param user 用户
// @return 用户
// @return 创建错误
func (e *Engine) CreateUser(ctx context.Context, user api.User) (api.User, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		if strings.TrimSpace(user.ID) == "" {
			return nil, api.NewError("ENGINE-1001", "用户 ID 不能为空", api.ErrBadRequest)
		}
		if _, exists := e.state.Users[user.ID]; exists {
			return nil, api.NewError("ENGINE-1002", "用户已存在", api.ErrConflict)
		}
		copied := user
		if copied.Password != "" {
			hashed, hashErr := auth.Hash(copied.Password)
			if hashErr != nil {
				return nil, hashErr
			}
			copied.PasswordHash = hashed
			copied.Password = ""
		}
		e.state.Users[user.ID] = &copied
		return copied, nil
	})
	if err != nil {
		return api.User{}, err
	}
	return result.(api.User), nil
}

// GetUser 查询用户。
// @param ctx 上下文
// @param userID 用户 ID
// @return 用户
// @return 查询错误
func (e *Engine) GetUser(ctx context.Context, userID string) (api.User, error) {
	result, err := e.read(func() (any, error) {
		user, ok := e.state.Users[userID]
		if !ok {
			return nil, api.NewError("ENGINE-0404", "用户不存在", api.ErrNotFound)
		}
		return *user, nil
	})
	if err != nil {
		return api.User{}, err
	}
	return result.(api.User), nil
}

// ListUsers 返回全部用户。
// @param ctx 上下文
// @return 用户列表
// @return 查询错误
func (e *Engine) ListUsers(ctx context.Context) ([]api.User, error) {
	result, err := e.read(func() (any, error) {
		out := make([]api.User, 0, len(e.state.Users))
		for _, user := range e.state.Users {
			out = append(out, *user)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.User), nil
}

// DeleteUser 删除用户。
// @param ctx 上下文
// @param userID 用户 ID
// @return 删除错误
func (e *Engine) DeleteUser(ctx context.Context, userID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		if _, ok := e.state.Users[userID]; !ok {
			return nil, api.NewError("ENGINE-0404", "用户不存在", api.ErrNotFound)
		}
		delete(e.state.Users, userID)
		delete(e.state.UserInfo, userID)
		delete(e.state.UserPictures, userID)
		for key := range e.state.Memberships {
			if strings.HasPrefix(key, userID+"|") {
				delete(e.state.Memberships, key)
			}
		}
		return nil, nil
	})
	return err
}

// GetUserInfo 返回用户信息键值。
// @param ctx 上下文
// @param userID 用户 ID
// @return 信息映射
// @return 查询错误
func (e *Engine) GetUserInfo(ctx context.Context, userID string) (map[string]string, error) {
	result, err := e.read(func() (any, error) {
		if _, ok := e.state.Users[userID]; !ok {
			return nil, api.NewError("ENGINE-0404", "用户不存在", api.ErrNotFound)
		}
		out := make(map[string]string)
		// 既有字符串接口只返回非空值，原版键集合由可空接口读取。
		for key, value := range e.state.UserInfo[userID] {
			if value != nil {
				out[key] = *value
			}
		}
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(map[string]string), nil
}

// GetUserInfoEntries 返回可空信息集合，调用方修改返回指针不会污染状态。
// @param ctx 上下文
// @param userID 用户标识
// @return 包含空值键的独立映射
// @return 查询错误
func (e *Engine) GetUserInfoEntries(ctx context.Context, userID string) (map[string]*string, error) {
	result, err := e.read(func() (any, error) {
		if _, ok := e.state.Users[userID]; !ok {
			return nil, api.NewError("ENGINE-0404", "用户不存在", api.ErrNotFound)
		}
		out := make(map[string]*string)
		// 保留已清空的键，非空字符串则逐项复制，不能暴露可写状态指针。
		for key, value := range e.state.UserInfo[userID] {
			out[key] = nil
			if value != nil {
				text := *value
				out[key] = &text
			}
		}
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(map[string]*string), nil
}

// GetUserInfoValue 返回单个用户信息。
// @param ctx 上下文
// @param userID 用户 ID
// @param key 键
// @return 值
// @return 查询错误
func (e *Engine) GetUserInfoValue(ctx context.Context, userID, key string) (string, error) {
	result, err := e.read(func() (any, error) {
		if _, ok := e.state.Users[userID]; !ok {
			return nil, api.NewError("ENGINE-0404", "用户不存在", api.ErrNotFound)
		}
		value, ok := e.state.UserInfo[userID][key]
		if !ok || value == nil {
			return nil, api.NewError("ENGINE-1701", "用户信息不存在", api.ErrNotFound)
		}
		return *value, nil
	})
	if err != nil {
		return "", err
	}
	return result.(string), nil
}

// SetUserInfo 设置用户信息。
// @param ctx 上下文
// @param userID 用户 ID
// @param key 键
// @param value 值
// @return 设置错误
func (e *Engine) SetUserInfo(ctx context.Context, userID, key, value string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		if _, ok := e.state.Users[userID]; !ok {
			return nil, api.NewError("ENGINE-0404", "用户不存在", api.ErrNotFound)
		}
		if strings.TrimSpace(key) == "" {
			return nil, api.NewError("ENGINE-1702", "用户信息键不能为空", api.ErrBadRequest)
		}
		if e.state.UserInfo[userID] == nil {
			e.state.UserInfo[userID] = make(map[string]*string)
		}
		e.state.UserInfo[userID][key] = &value
		return nil, nil
	})
	return err
}

// DeleteUserInfo 删除用户信息。
// @param ctx 上下文
// @param userID 用户 ID
// @param key 键
// @return 删除错误
func (e *Engine) DeleteUserInfo(ctx context.Context, userID, key string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		if _, ok := e.state.Users[userID]; !ok {
			return nil, api.NewError("ENGINE-0404", "用户不存在", api.ErrNotFound)
		}
		if value, ok := e.state.UserInfo[userID][key]; !ok || value == nil {
			return nil, api.NewError("ENGINE-1701", "用户信息不存在", api.ErrNotFound)
		}
		// 原版删除仅把值设为空，列表仍能列出该键，但单值查询按不存在处理。
		e.state.UserInfo[userID][key] = nil
		return nil, nil
	})
	return err
}

// GetUserPicture 返回用户头像。
// @param ctx 上下文
// @param userID 用户 ID
// @return 图片数据
// @return MIME 类型
// @return 查询错误
func (e *Engine) GetUserPicture(ctx context.Context, userID string) ([]byte, string, error) {
	result, err := e.read(func() (any, error) {
		if _, ok := e.state.Users[userID]; !ok {
			return nil, api.NewError("ENGINE-0404", "用户不存在", api.ErrNotFound)
		}
		picture, ok := e.state.UserPictures[userID]
		if !ok {
			return nil, api.NewError("ENGINE-1703", "用户头像不存在", api.ErrNotFound)
		}
		return *picture, nil
	})
	if err != nil {
		return nil, "", err
	}
	picture := result.(userPictureState)
	return append([]byte(nil), picture.Data...), picture.MimeType, nil
}

// SetUserPicture 设置用户头像。
// @param ctx 上下文
// @param userID 用户 ID
// @param data 图片数据
// @param mimeType MIME 类型
// @return 设置错误
func (e *Engine) SetUserPicture(ctx context.Context, userID string, data []byte, mimeType string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		if _, ok := e.state.Users[userID]; !ok {
			return nil, api.NewError("ENGINE-0404", "用户不存在", api.ErrNotFound)
		}
		if len(data) == 0 {
			return nil, api.NewError("ENGINE-1704", "用户头像不能为空", api.ErrBadRequest)
		}
		if strings.TrimSpace(mimeType) == "" {
			mimeType = "image/jpeg"
		}
		e.state.UserPictures[userID] = &userPictureState{
			Data:     append([]byte(nil), data...),
			MimeType: mimeType,
		}
		return nil, nil
	})
	return err
}

// CreateGroup 创建组。
// @param ctx 上下文
// @param group 组
// @return 组
// @return 创建错误
func (e *Engine) CreateGroup(ctx context.Context, group api.Group) (api.Group, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		if strings.TrimSpace(group.ID) == "" {
			return nil, api.NewError("ENGINE-1003", "组 ID 不能为空", api.ErrBadRequest)
		}
		if _, exists := e.state.Groups[group.ID]; exists {
			return nil, api.NewError("ENGINE-1004", "组已存在", api.ErrConflict)
		}
		copied := group
		e.state.Groups[group.ID] = &copied
		return copied, nil
	})
	if err != nil {
		return api.Group{}, err
	}
	return result.(api.Group), nil
}

// UpdateGroup 更新组。
// @param ctx 上下文
// @param group 组
// @return 组
// @return 更新错误
func (e *Engine) UpdateGroup(ctx context.Context, group api.Group) (api.Group, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		if strings.TrimSpace(group.ID) == "" {
			return nil, api.NewError("ENGINE-1003", "组 ID 不能为空", api.ErrBadRequest)
		}
		existing, ok := e.state.Groups[group.ID]
		if !ok {
			return nil, api.NewError("ENGINE-0404", "组不存在", api.ErrNotFound)
		}
		existing.Name = group.Name
		existing.Type = group.Type
		existing.TenantID = group.TenantID
		return *existing, nil
	})
	if err != nil {
		return api.Group{}, err
	}
	return result.(api.Group), nil
}

// GetGroup 查询组。
// @param ctx 上下文
// @param groupID 组 ID
// @return 组
// @return 查询错误
func (e *Engine) GetGroup(ctx context.Context, groupID string) (api.Group, error) {
	result, err := e.read(func() (any, error) {
		group, ok := e.state.Groups[groupID]
		if !ok {
			return nil, api.NewError("ENGINE-0404", "组不存在", api.ErrNotFound)
		}
		return *group, nil
	})
	if err != nil {
		return api.Group{}, err
	}
	return result.(api.Group), nil
}

// ListGroups 返回全部组。
// @param ctx 上下文
// @return 组列表
// @return 查询错误
func (e *Engine) ListGroups(ctx context.Context) ([]api.Group, error) {
	result, err := e.read(func() (any, error) {
		out := make([]api.Group, 0, len(e.state.Groups))
		for _, group := range e.state.Groups {
			out = append(out, *group)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.Group), nil
}

// DeleteGroup 删除组。
// @param ctx 上下文
// @param groupID 组 ID
// @return 删除错误
func (e *Engine) DeleteGroup(ctx context.Context, groupID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		if _, ok := e.state.Groups[groupID]; !ok {
			return nil, api.NewError("ENGINE-0404", "组不存在", api.ErrNotFound)
		}
		delete(e.state.Groups, groupID)
		for key := range e.state.Memberships {
			if strings.HasSuffix(key, "|"+groupID) {
				delete(e.state.Memberships, key)
			}
		}
		return nil, nil
	})
	return err
}

// AddMembership 添加成员关系。
// @param ctx 上下文
// @param userID 用户 ID
// @param groupID 组 ID
// @return 添加错误
func (e *Engine) AddMembership(ctx context.Context, userID, groupID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		if _, ok := e.state.Users[userID]; !ok {
			return nil, api.NewError("ENGINE-0404", "用户不存在", api.ErrNotFound)
		}
		if _, ok := e.state.Groups[groupID]; !ok {
			return nil, api.NewError("ENGINE-0404", "组不存在", api.ErrNotFound)
		}
		e.state.Memberships[userID+"|"+groupID] = true
		return nil, nil
	})
	return err
}

// RemoveMembership 移除成员关系。
// @param ctx 上下文
// @param userID 用户 ID
// @param groupID 组 ID
// @return 移除错误
func (e *Engine) RemoveMembership(ctx context.Context, userID, groupID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		delete(e.state.Memberships, userID+"|"+groupID)
		return nil, nil
	})
	return err
}

// ListMembers 返回组成员。
// @param ctx 上下文
// @param groupID 组 ID
// @return 用户列表
// @return 查询错误
func (e *Engine) ListMembers(ctx context.Context, groupID string) ([]api.User, error) {
	result, err := e.read(func() (any, error) {
		out := make([]api.User, 0)
		for key := range e.state.Memberships {
			parts := strings.SplitN(key, "|", 2)
			if len(parts) != 2 || parts[1] != groupID {
				continue
			}
			if user, ok := e.state.Users[parts[0]]; ok {
				out = append(out, *user)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.User), nil
}

// CheckPassword 校验用户密码。
// @param ctx 上下文
// @param userID 用户 ID
// @param password 密码
// @return 校验错误
func (e *Engine) CheckPassword(ctx context.Context, userID, password string) error {
	var authErr error
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		user, ok := e.state.Users[userID]
		if !ok {
			authErr = api.NewError("AUTH-0003", "用户名或密码错误", api.ErrUnauthorized)
			return nil, nil
		}
		if user.Disabled {
			authErr = api.NewError("AUTH-0005", "用户已禁用", api.ErrUnauthorized)
			return nil, nil
		}
		if user.LockedUntil != nil && user.LockedUntil.After(cc.now) {
			authErr = api.NewError("AUTH-0004", "用户已锁定", api.ErrUnauthorized)
			return nil, nil
		}
		if user.PasswordHash == "" || !auth.Verify(password, user.PasswordHash) {
			user.FailedAttempts++
			if user.FailedAttempts >= 5 {
				locked := cc.now.Add(5 * time.Minute)
				user.LockedUntil = &locked
			}
			authErr = api.NewError("AUTH-0003", "用户名或密码错误", api.ErrUnauthorized)
			return nil, nil
		}
		user.FailedAttempts = 0
		user.LockedUntil = nil
		return nil, nil
	})
	if err != nil {
		return err
	}
	return authErr
}

// UpdateUser 更新用户资料与角色。
// @param ctx 上下文
// @param user 用户
// @return 用户
// @return 更新错误
func (e *Engine) UpdateUser(ctx context.Context, user api.User) (api.User, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		existing, ok := e.state.Users[user.ID]
		if !ok {
			return nil, api.NewError("ENGINE-0404", "用户不存在", api.ErrNotFound)
		}
		existing.FirstName = user.FirstName
		existing.LastName = user.LastName
		existing.DisplayName = user.DisplayName
		existing.Email = user.Email
		existing.TenantID = user.TenantID
		existing.Disabled = user.Disabled
		if user.Roles != nil {
			existing.Roles = append([]string(nil), user.Roles...)
		}
		if user.Password != "" {
			hashed, hashErr := auth.Hash(user.Password)
			if hashErr != nil {
				return nil, hashErr
			}
			existing.PasswordHash = hashed
			existing.Password = ""
		}
		copied := *existing
		copied.Password = ""
		return copied, nil
	})
	if err != nil {
		return api.User{}, err
	}
	return result.(api.User), nil
}

// SetUserRoles 设置用户角色。
// @param ctx 上下文
// @param userID 用户 ID
// @param roles 角色列表
// @return 设置错误
func (e *Engine) SetUserRoles(ctx context.Context, userID string, roles []string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		user, ok := e.state.Users[userID]
		if !ok {
			return nil, api.NewError("ENGINE-0404", "用户不存在", api.ErrNotFound)
		}
		user.Roles = append([]string(nil), roles...)
		return nil, nil
	})
	return err
}

// UpdatePassword 重置用户密码。
// @param ctx 上下文
// @param userID 用户 ID
// @param newPassword 新密码
// @return 更新错误
func (e *Engine) UpdatePassword(ctx context.Context, userID, newPassword string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		user, ok := e.state.Users[userID]
		if !ok {
			return nil, api.NewError("ENGINE-0404", "用户不存在", api.ErrNotFound)
		}
		hashed, hashErr := auth.Hash(newPassword)
		if hashErr != nil {
			return nil, hashErr
		}
		user.PasswordHash = hashed
		user.FailedAttempts = 0
		user.LockedUntil = nil
		return nil, nil
	})
	return err
}

// ChangePassword 修改用户密码。
// @param ctx 上下文
// @param userID 用户 ID
// @param oldPassword 旧密码
// @param newPassword 新密码
// @return 修改错误
func (e *Engine) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		user, ok := e.state.Users[userID]
		if !ok || user.PasswordHash == "" || !auth.Verify(oldPassword, user.PasswordHash) {
			return nil, api.NewError("AUTH-0003", "用户名或密码错误", api.ErrUnauthorized)
		}
		hashed, hashErr := auth.Hash(newPassword)
		if hashErr != nil {
			return nil, hashErr
		}
		user.PasswordHash = hashed
		user.FailedAttempts = 0
		user.LockedUntil = nil
		return nil, nil
	})
	return err
}

// CreateRole 创建角色。
// @param ctx 上下文
// @param role 角色
// @return 角色
// @return 创建错误
func (e *Engine) CreateRole(ctx context.Context, role api.Role) (api.Role, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		if role.ID == "" {
			return nil, api.NewError("ENGINE-1005", "角色 ID 不能为空", api.ErrBadRequest)
		}
		if _, exists := e.state.Roles[role.ID]; exists {
			return nil, api.NewError("ENGINE-1006", "角色已存在", api.ErrConflict)
		}
		copied := role
		copied.Permissions = append([]string(nil), role.Permissions...)
		e.state.Roles[copied.ID] = &copied
		return copied, nil
	})
	if err != nil {
		return api.Role{}, err
	}
	return result.(api.Role), nil
}

// GetRole 查询角色。
// @param ctx 上下文
// @param roleID 角色 ID
// @return 角色
// @return 查询错误
func (e *Engine) GetRole(ctx context.Context, roleID string) (api.Role, error) {
	result, err := e.read(func() (any, error) {
		role, ok := e.state.Roles[roleID]
		if !ok {
			return nil, api.NewError("ENGINE-0404", "角色不存在", api.ErrNotFound)
		}
		return *role, nil
	})
	if err != nil {
		return api.Role{}, err
	}
	return result.(api.Role), nil
}

// ListRoles 返回全部角色。
// @param ctx 上下文
// @return 角色列表
// @return 查询错误
func (e *Engine) ListRoles(ctx context.Context) ([]api.Role, error) {
	result, err := e.read(func() (any, error) {
		out := make([]api.Role, 0, len(e.state.Roles))
		for _, role := range e.state.Roles {
			out = append(out, *role)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.Role), nil
}

// UpdateRole 更新角色。
// @param ctx 上下文
// @param role 角色
// @return 角色
// @return 更新错误
func (e *Engine) UpdateRole(ctx context.Context, role api.Role) (api.Role, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		existing, ok := e.state.Roles[role.ID]
		if !ok {
			return nil, api.NewError("ENGINE-0404", "角色不存在", api.ErrNotFound)
		}
		existing.Name = role.Name
		existing.TenantID = role.TenantID
		existing.Permissions = append([]string(nil), role.Permissions...)
		return *existing, nil
	})
	if err != nil {
		return api.Role{}, err
	}
	return result.(api.Role), nil
}

// DeleteRole 删除角色。
// @param ctx 上下文
// @param roleID 角色 ID
// @return 删除错误
func (e *Engine) DeleteRole(ctx context.Context, roleID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		if _, ok := e.state.Roles[roleID]; !ok {
			return nil, api.NewError("ENGINE-0404", "角色不存在", api.ErrNotFound)
		}
		delete(e.state.Roles, roleID)
		return nil, nil
	})
	return err
}

// ListUserGroups 返回用户所属组。
// @param ctx 上下文
// @param userID 用户 ID
// @return 组列表
// @return 查询错误
func (e *Engine) ListUserGroups(ctx context.Context, userID string) ([]api.Group, error) {
	result, err := e.read(func() (any, error) {
		out := make([]api.Group, 0)
		prefix := userID + "|"
		for key := range e.state.Memberships {
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			groupID := strings.TrimPrefix(key, prefix)
			if group, ok := e.state.Groups[groupID]; ok {
				out = append(out, *group)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.Group), nil
}
