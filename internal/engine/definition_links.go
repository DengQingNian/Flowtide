package engine

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
)

// buildStarterLinks 根据 BPMN 配置构建候选发起人链接。
// @param definitionID 流程定义 ID
// @param process 流程模型
// @param now 创建时间
// @return 链接列表
func (e *Engine) buildStarterLinks(definitionID string, process *model.Process, now time.Time) []*api.IdentityLink {
	links := make([]*api.IdentityLink, 0, len(process.CandidateStarterUsers)+len(process.CandidateStarterGroups))
	for _, userID := range process.CandidateStarterUsers {
		links = append(links, &api.IdentityLink{
			ID: e.newID(), Type: "candidate", UserID: userID,
			ProcessDefinitionID: definitionID, CreateTime: now,
		})
	}
	for _, groupID := range process.CandidateStarterGroups {
		links = append(links, &api.IdentityLink{
			ID: e.newID(), Type: "candidate", GroupID: groupID,
			ProcessDefinitionID: definitionID, CreateTime: now,
		})
	}
	return links
}

// ListDefinitionIdentityLinks 返回流程定义候选发起人。
// @param ctx 上下文
// @param definitionID 流程定义 ID
// @return 链接列表
// @return 查询错误
func (e *Engine) ListDefinitionIdentityLinks(ctx context.Context, definitionID string) ([]api.IdentityLink, error) {
	result, err := e.read(func() (any, error) {
		definition, ok := e.findDefinition(definitionID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(ctx, definition.Info.TenantID); err != nil {
			return nil, err
		}
		out := make([]api.IdentityLink, 0, len(definition.StarterLinks))
		for _, link := range definition.StarterLinks {
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

// GetDefinitionIdentityLink 查询单个候选发起人。
// @param ctx 上下文
// @param definitionID 流程定义 ID
// @param family users 或 groups
// @param identityID 用户或组 ID
// @return 链接
// @return 查询错误
func (e *Engine) GetDefinitionIdentityLink(ctx context.Context, definitionID, family, identityID string) (api.IdentityLink, error) {
	links, err := e.ListDefinitionIdentityLinks(ctx, definitionID)
	if err != nil {
		return api.IdentityLink{}, err
	}
	for _, link := range links {
		if family == "users" && link.UserID == identityID {
			return link, nil
		}
		if family == "groups" && link.GroupID == identityID {
			return link, nil
		}
	}
	return api.IdentityLink{}, api.NewError("ENGINE-1304", "身份链接不存在", api.ErrNotFound)
}

// AddDefinitionIdentityLink 添加候选发起人。
// @param ctx 上下文
// @param definitionID 流程定义 ID
// @param link 链接
// @return 创建后的链接
// @return 创建错误
func (e *Engine) AddDefinitionIdentityLink(ctx context.Context, definitionID string, link api.IdentityLink) (api.IdentityLink, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		definition, ok := e.findDefinition(definitionID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(cc.ctx, definition.Info.TenantID); err != nil {
			return nil, err
		}
		userID := strings.TrimSpace(link.UserID)
		groupID := strings.TrimSpace(link.GroupID)
		if (userID == "") == (groupID == "") {
			return nil, api.NewError("ENGINE-1302", "必须且只能指定 userId 或 groupId", api.ErrBadRequest)
		}
		// 已存在时幂等返回
		for _, existing := range definition.StarterLinks {
			if existing.UserID == userID && existing.GroupID == groupID {
				return *existing, nil
			}
		}
		created := &api.IdentityLink{
			ID:                  e.newID(),
			Type:                "candidate",
			UserID:              userID,
			GroupID:             groupID,
			ProcessDefinitionID: definitionID,
			CreateTime:          cc.now,
		}
		definition.StarterLinks = append(definition.StarterLinks, created)
		return *created, nil
	})
	if err != nil {
		return api.IdentityLink{}, err
	}
	return result.(api.IdentityLink), nil
}

// DeleteDefinitionIdentityLink 删除候选发起人。
// @param ctx 上下文
// @param definitionID 流程定义 ID
// @param family users 或 groups
// @param identityID 用户或组 ID
// @return 删除错误
func (e *Engine) DeleteDefinitionIdentityLink(ctx context.Context, definitionID, family, identityID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		definition, ok := e.findDefinition(definitionID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(cc.ctx, definition.Info.TenantID); err != nil {
			return nil, err
		}
		if family != "users" && family != "groups" {
			return nil, api.NewError("ENGINE-1303", "参与者类型非法: "+family, api.ErrBadRequest)
		}
		removed := false
		links := make([]*api.IdentityLink, 0, len(definition.StarterLinks))
		for _, link := range definition.StarterLinks {
			match := (family == "users" && link.UserID == identityID) || (family == "groups" && link.GroupID == identityID)
			if match {
				removed = true
				continue
			}
			links = append(links, link)
		}
		if !removed {
			return nil, api.NewError("ENGINE-1304", "身份链接不存在", api.ErrNotFound)
		}
		definition.StarterLinks = links
		return nil, nil
	})
	return err
}
