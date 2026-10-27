package engine

import (
	"context"
	"sort"
	"strings"

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
		for key := range e.state.Memberships {
			if strings.HasPrefix(key, userID+"|") {
				delete(e.state.Memberships, key)
			}
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
	_, err := e.read(func() (any, error) {
		user, ok := e.state.Users[userID]
		if !ok || user.PasswordHash == "" || !auth.Verify(password, user.PasswordHash) {
			return nil, api.NewError("AUTH-0003", "用户名或密码错误", api.ErrUnauthorized)
		}
		return user, nil
	})
	return err
}
