package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/flowable-go/flowable/api"
)

// recordTaskIdentityLinks 记录任务身份链接。
// @param cc 命令上下文
// @param task 任务
func (e *Engine) recordTaskIdentityLinks(cc *commandContext, task *taskState) {
	if !e.historyAtLeast(HistoryActivity) {
		return
	}
	e.removeHistoricLinks(task.Task.ID)
	for _, link := range taskIdentityLinks(task.Task, cc.now) {
		e.state.HistoricLinks[link.ID] = link
	}
}

// removeHistoricLinks 删除任务已有历史链接。
// @param taskID 任务 ID
func (e *Engine) removeHistoricLinks(taskID string) {
	for id, link := range e.state.HistoricLinks {
		if link.TaskID == taskID {
			delete(e.state.HistoricLinks, id)
		}
	}
}

// taskIdentityLinks 构造任务身份链接。
// @param task 任务
// @param now 当前时间
// @return 链接列表
func taskIdentityLinks(task api.Task, now time.Time) []*api.IdentityLink {
	links := make([]*api.IdentityLink, 0, len(task.CandidateUsers)+len(task.CandidateGroups)+2)
	appendLink := func(kind, userID, groupID string) {
		id := historicLinkID(task.ID, kind, userID+groupID)
		links = append(links, &api.IdentityLink{
			ID: id, Type: kind, UserID: userID, GroupID: groupID, TaskID: task.ID,
			ProcessInstanceID: task.ProcessInstanceID, CreateTime: now,
		})
	}
	if task.Assignee != "" {
		appendLink("assignee", task.Assignee, "")
	}
	if task.Owner != "" {
		appendLink("owner", task.Owner, "")
	}
	for _, userID := range task.CandidateUsers {
		appendLink("candidate", userID, "")
	}
	for _, groupID := range task.CandidateGroups {
		appendLink("candidate", "", groupID)
	}
	return links
}

// historicLinkID 生成稳定历史链接 ID。
// @param taskID 任务 ID
// @param kind 类型
// @param identity 身份
// @return 链接 ID
func historicLinkID(taskID, kind, identity string) string {
	sum := sha256.Sum256([]byte(taskID + "|" + kind + "|" + identity))
	return "hil-" + hex.EncodeToString(sum[:])[:32]
}
