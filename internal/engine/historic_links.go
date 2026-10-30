package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/DengQingNian/Flowtide/api"
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

// promoteTaskParticipants 在任务完成时把候选用户与处理人升级为流程参与者历史链接。
// @param cc 命令上下文
// @param task 任务
func (e *Engine) promoteTaskParticipants(cc *commandContext, task *taskState) {
	if !e.historyAtLeast(HistoryActivity) || task.Task.ProcessInstanceID == "" {
		return
	}
	// 原版在完成时升级候选用户与完成操作人；处理人已在变更时升级。
	actor := actorFromContext(cc.ctx)
	for _, userID := range append([]string{actor}, task.CandidateUsers...) {
		if userID == "" {
			continue
		}
		e.promoteProcessParticipant(cc, task.Task.ProcessInstanceID, userID, cc.now)
	}
}

// promoteProcessParticipant 记录流程参与者历史链接；历史链接是变更流水，允许重复。
// @param cc 命令上下文
// @param processInstanceID 实例 ID
// @param userID 用户 ID
// @param now 记录时间
func (e *Engine) promoteProcessParticipant(cc *commandContext, processInstanceID, userID string, now time.Time) {
	promoted := &api.IdentityLink{ID: e.newID(), Type: "participant", UserID: userID, ProcessInstanceID: processInstanceID, CreateTime: now}
	e.state.HistoricLinks[promoted.ID] = promoted
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
