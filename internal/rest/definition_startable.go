package rest

import (
	"context"

	"github.com/DengQingNian/Flowtide/api"
)

// filterStartableDefinitions 过滤用户可发起的流程定义。
// 无候选发起人的定义对所有人开放；存在候选时按用户或其组匹配。
// @param ctx 上下文
// @param definitions 流程定义
// @param userID 用户 ID
// @return 过滤后的定义
// @return 查询错误
func (s *Server) filterStartableDefinitions(ctx context.Context, definitions []api.ProcessDefinition, userID string) ([]api.ProcessDefinition, error) {
	groups, err := s.engine.IdentityService().ListUserGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	groupSet := make(map[string]bool, len(groups))
	for _, group := range groups {
		groupSet[group.ID] = true
	}
	filtered := make([]api.ProcessDefinition, 0, len(definitions))
	for _, definition := range definitions {
		links, linkErr := s.engine.RepositoryService().ListDefinitionIdentityLinks(ctx, definition.ID)
		if linkErr != nil {
			return nil, linkErr
		}
		if len(links) == 0 {
			filtered = append(filtered, definition)
			continue
		}
		startable := false
		for _, link := range links {
			if link.UserID == userID {
				startable = true
				break
			}
			if link.GroupID != "" && groupSet[link.GroupID] {
				startable = true
				break
			}
		}
		if startable {
			filtered = append(filtered, definition)
		}
	}
	return filtered, nil
}
