package engine

import (
	"context"
	"sort"
	"strings"

	"github.com/flowable-go/flowable/api"
)

// ListInstanceIdentityLinks 返回流程实例身份链接。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @return 链接列表
// @return 查询错误
func (e *Engine) ListInstanceIdentityLinks(ctx context.Context, processInstanceID string) ([]api.IdentityLink, error) {
	result, err := e.read(func() (any, error) {
		if _, ok := e.findInstance(processInstanceID); !ok {
			return nil, api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
		}
		if err := e.allowInstance(ctx, processInstanceID); err != nil {
			return nil, err
		}
		out := make([]api.IdentityLink, 0)
		for _, link := range e.state.InstanceLinks {
			if link.ProcessInstanceID != processInstanceID {
				continue
			}
			out = append(out, *link)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].CreateTime.Equal(out[j].CreateTime) {
				return out[i].ID < out[j].ID
			}
			return out[i].CreateTime.Before(out[j].CreateTime)
		})
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.IdentityLink), nil
}

// AddInstanceIdentityLink 添加流程实例身份链接。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @param link 链接
// @return 创建后的链接
// @return 创建错误
func (e *Engine) AddInstanceIdentityLink(ctx context.Context, processInstanceID string, link api.IdentityLink) (api.IdentityLink, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		if _, ok := e.findInstance(processInstanceID); !ok {
			return nil, api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
		}
		if err := e.allowInstance(cc.ctx, processInstanceID); err != nil {
			return nil, err
		}
		if strings.TrimSpace(link.Type) == "" {
			return nil, api.NewError("ENGINE-1301", "身份链接类型不能为空", api.ErrBadRequest)
		}
		// 用户与组必须二选一
		if (strings.TrimSpace(link.UserID) == "") == (strings.TrimSpace(link.GroupID) == "") {
			return nil, api.NewError("ENGINE-1302", "必须且只能指定 userId 或 groupId", api.ErrBadRequest)
		}
		created := api.IdentityLink{
			ID:                e.newID(),
			Type:              link.Type,
			UserID:            strings.TrimSpace(link.UserID),
			GroupID:           strings.TrimSpace(link.GroupID),
			ProcessInstanceID: processInstanceID,
			CreateTime:        cc.now,
		}
		e.state.InstanceLinks[created.ID] = &created
		e.recordInstanceHistoricLink(cc, &created)
		return created, nil
	})
	if err != nil {
		return api.IdentityLink{}, err
	}
	return result.(api.IdentityLink), nil
}

// DeleteInstanceIdentityLink 删除流程实例身份链接。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @param family users 或 groups
// @param identityID 用户或组 ID
// @param linkType 链接类型
// @return 删除错误
func (e *Engine) DeleteInstanceIdentityLink(ctx context.Context, processInstanceID, family, identityID, linkType string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		if _, ok := e.findInstance(processInstanceID); !ok {
			return nil, api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
		}
		if err := e.allowInstance(cc.ctx, processInstanceID); err != nil {
			return nil, err
		}
		if family != "users" && family != "groups" {
			return nil, api.NewError("ENGINE-1303", "参与者类型非法: "+family, api.ErrBadRequest)
		}
		removed := 0
		for id, link := range e.state.InstanceLinks {
			if link.ProcessInstanceID != processInstanceID || link.Type != linkType {
				continue
			}
			if family == "users" && link.UserID != identityID {
				continue
			}
			if family == "groups" && link.GroupID != identityID {
				continue
			}
			delete(e.state.InstanceLinks, id)
			delete(e.state.HistoricLinks, id)
			removed++
		}
		if removed == 0 {
			return nil, api.NewError("ENGINE-1304", "身份链接不存在", api.ErrNotFound)
		}
		return nil, nil
	})
	return err
}

// recordInstanceHistoricLink 记录实例身份链接历史。
// @param cc 命令上下文
// @param link 运行时链接
func (e *Engine) recordInstanceHistoricLink(cc *commandContext, link *api.IdentityLink) {
	if !e.historyAtLeast(HistoryActivity) {
		return
	}
	copied := *link
	e.state.HistoricLinks[copied.ID] = &copied
}
