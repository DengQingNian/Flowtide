package engine

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/flowable-go/flowable/api"
	bpmnxml "github.com/flowable-go/flowable/internal/bpmn/xml"
)

// deploymentSnapshot 是部署的持久化结构。
type deploymentSnapshot struct {
	Info      api.Deployment    `json:"info"`      // 部署信息
	Resources map[string][]byte `json:"resources"` // 资源内容
}

// definitionSnapshot 是流程定义的持久化结构。
type definitionSnapshot struct {
	Info         api.ProcessDefinition `json:"info"`         // 流程定义信息
	XML          []byte                `json:"xml"`          // 原始 BPMN XML
	StarterLinks []*api.IdentityLink   `json:"starterLinks"` // 候选发起人
}

// batchSnapshot 是批处理作业的持久化结构。
type batchSnapshot struct {
	Info  api.Batch        `json:"info"`  // 批次信息
	Parts []*api.BatchPart `json:"parts"` // 分片
}

// snapshot 是引擎状态的持久化结构。
// snapshotVersion 是当前快照结构版本，版本零表示兼容旧版无标记快照。
const snapshotVersion = 1

type snapshot struct {
	Version            int                                `json:"version"`            // 格式版本
	Deployments        []deploymentSnapshot               `json:"deployments"`        // 部署
	Definitions        []definitionSnapshot               `json:"definitions"`        // 流程定义
	Models             []*modelState                      `json:"models"`             // 流程模型及编辑源
	Instances          []*instanceState                   `json:"instances"`          // 流程实例
	Executions         []*executionState                  `json:"executions"`         // 执行
	Tasks              []*taskState                       `json:"tasks"`              // 任务
	Jobs               []*jobState                        `json:"jobs"`               // 定时器作业
	DeadLetters        []*jobState                        `json:"deadLetters"`        // 死信作业
	Subscriptions      []*subscriptionState               `json:"subscriptions"`      // 事件订阅
	Compensations      map[string][]*compensationState    `json:"compensations"`      // 补偿登记
	Batches            []batchSnapshot                    `json:"batches"`            // 批处理作业
	HistoricProcesses  []*api.HistoricProcessInstance     `json:"historicProcesses"`  // 历史流程
	HistoricActivities []*api.HistoricActivityInstance    `json:"historicActivities"` // 历史活动
	HistoricTasks      []*api.HistoricTaskInstance        `json:"historicTasks"`      // 历史任务
	HistoricVariables  []*api.HistoricVariableInstance    `json:"historicVariables"`  // 历史变量
	InstanceLinks      []*api.IdentityLink                `json:"instanceLinks"`      // 实例身份链接
	HistoricLinks      []*api.IdentityLink                `json:"historicLinks"`      // 历史身份链接
	HistoricDetails    []*api.HistoricDetail              `json:"historicDetails"`    // 历史变量明细
	TaskLogs           []*api.HistoricTaskLog             `json:"taskLogs"`           // 任务日志
	NextTaskLogID      int64                              `json:"nextTaskLogId"`      // 下一个任务日志 ID
	Comments           []*api.Comment                     `json:"comments"`           // 评论
	Attachments        []*api.Attachment                  `json:"attachments"`        // 附件
	AttachmentContents map[string]*attachmentContentState `json:"attachmentContents"` // 附件内容
	Users              []*api.User                        `json:"users"`              // 用户
	UserInfo           map[string]map[string]string       `json:"userInfo"`           // 用户信息
	UserPictures       map[string]*userPictureState       `json:"userPictures"`       // 用户头像
	Groups             []*api.Group                       `json:"groups"`             // 组
	Roles              []*api.Role                        `json:"roles"`              // 角色
	Memberships        map[string]bool                    `json:"memberships"`        // 成员关系
	Properties         map[string]string                  `json:"properties"`         // 自定义属性
	Versions           map[string]int                     `json:"versions"`           // 版本号
}

// toSnapshot 将运行时状态转换为持久化结构。
// @return 快照
func (e *Engine) toSnapshot() *snapshot {
	out := &snapshot{Version: snapshotVersion, Versions: make(map[string]int, len(e.state.Versions))}
	for _, deployment := range e.state.Deployments {
		out.Deployments = append(out.Deployments, deploymentSnapshot{Info: deployment.Info, Resources: deployment.Resources})
	}
	for _, definition := range e.state.Definitions {
		out.Definitions = append(out.Definitions, definitionSnapshot{Info: definition.Info, XML: definition.XML, StarterLinks: definition.StarterLinks})
	}
	for _, model := range e.state.Models {
		out.Models = append(out.Models, model)
	}
	for _, instance := range e.state.Instances {
		out.Instances = append(out.Instances, instance)
	}
	for _, execution := range e.state.Executions {
		out.Executions = append(out.Executions, execution)
	}
	for _, task := range e.state.Tasks {
		out.Tasks = append(out.Tasks, task)
	}
	for _, job := range e.state.Jobs {
		out.Jobs = append(out.Jobs, job)
	}
	for _, job := range e.state.DeadLetters {
		out.DeadLetters = append(out.DeadLetters, job)
	}
	for _, subscription := range e.state.Subscriptions {
		out.Subscriptions = append(out.Subscriptions, subscription)
	}
	out.Compensations = e.state.Compensations
	for _, batch := range e.state.Batches {
		item := batchSnapshot{Info: batch.Info, Parts: make([]*api.BatchPart, 0, len(batch.Parts))}
		for _, part := range batch.Parts {
			item.Parts = append(item.Parts, part)
		}
		// 排序保证快照稳定，便于比对和迁移
		sort.SliceStable(item.Parts, func(i, j int) bool { return item.Parts[i].ID < item.Parts[j].ID })
		out.Batches = append(out.Batches, item)
	}
	for _, historic := range e.state.HistoricProcesses {
		out.HistoricProcesses = append(out.HistoricProcesses, historic)
	}
	for _, historic := range e.state.HistoricActivities {
		out.HistoricActivities = append(out.HistoricActivities, historic)
	}
	for _, historic := range e.state.HistoricTasks {
		out.HistoricTasks = append(out.HistoricTasks, historic)
	}
	for _, historic := range e.state.HistoricVariables {
		out.HistoricVariables = append(out.HistoricVariables, historic)
	}
	for _, link := range e.state.InstanceLinks {
		out.InstanceLinks = append(out.InstanceLinks, link)
	}
	for _, link := range e.state.HistoricLinks {
		out.HistoricLinks = append(out.HistoricLinks, link)
	}
	for _, detail := range e.state.HistoricDetails {
		out.HistoricDetails = append(out.HistoricDetails, detail)
	}
	for _, log := range e.state.TaskLogs {
		out.TaskLogs = append(out.TaskLogs, log)
	}
	out.NextTaskLogID = e.state.NextTaskLogID
	for _, comment := range e.state.Comments {
		out.Comments = append(out.Comments, comment)
	}
	for _, attachment := range e.state.Attachments {
		out.Attachments = append(out.Attachments, attachment)
	}
	out.AttachmentContents = e.state.AttachmentContents
	for _, user := range e.state.Users {
		out.Users = append(out.Users, user)
	}
	out.UserInfo = e.state.UserInfo
	out.UserPictures = e.state.UserPictures
	out.Properties = e.state.Properties
	for _, group := range e.state.Groups {
		out.Groups = append(out.Groups, group)
	}
	for _, role := range e.state.Roles {
		out.Roles = append(out.Roles, role)
	}
	out.Memberships = e.state.Memberships
	for key, version := range e.state.Versions {
		out.Versions[key] = version
	}
	return out
}

// saveState 保存引擎状态。
// @param ctx 上下文
// @return 保存错误
func (e *Engine) saveState(ctx context.Context) error {
	if e.rowSource || e.suppressPersist {
		return nil
	}
	if e.cfg.StateStore == nil {
		return nil
	}
	data, err := json.Marshal(e.toSnapshot())
	if err != nil {
		return api.NewError("ENGINE-0901", "序列化引擎状态失败", err)
	}
	if err := e.cfg.StateStore.Save(ctx, data); err != nil {
		return api.NewError("ENGINE-0908", "保存引擎状态失败", err)
	}
	return nil
}

// loadState 从状态存储恢复。
// @param ctx 上下文
// @return 恢复错误
func (e *Engine) loadState(ctx context.Context) error {
	if e.cfg.StateStore == nil {
		return nil
	}
	data, err := e.cfg.StateStore.Load(ctx)
	if err != nil {
		return api.NewError("ENGINE-0910", "读取状态失败", err)
	}
	if data == nil {
		return nil
	}
	var stored snapshot
	if err := json.Unmarshal(data, &stored); err != nil {
		return api.NewError("ENGINE-0911", "解析状态文件失败", err)
	}
	if stored.Version < 0 || stored.Version > snapshotVersion {
		return api.NewError("ENGINE-0917", "不支持的快照版本", api.ErrBadRequest)
	}
	restored := newState()
	for _, deployment := range stored.Deployments {
		restored.Deployments[deployment.Info.ID] = &deploymentState{Info: deployment.Info, Resources: deployment.Resources}
	}
	for _, definition := range stored.Definitions {
		parsed, err := bpmnxml.Parse(definition.XML)
		if err != nil {
			return api.NewError("ENGINE-0912", "恢复流程定义失败: "+definition.Info.ID, err)
		}
		restored.Definitions[definition.Info.ID] = &definitionState{Info: definition.Info, Model: parsed, XML: definition.XML, StarterLinks: definition.StarterLinks}
	}
	// 旧快照缺少新增字段时得到空列表，保留已有文件和数据库快照的兼容性。
	for _, model := range stored.Models {
		restored.Models[model.Info.ID] = model
	}
	for _, instance := range stored.Instances {
		restored.Instances[instance.ID] = instance
	}
	for _, execution := range stored.Executions {
		restored.Executions[execution.ID] = execution
	}
	for _, task := range stored.Tasks {
		restored.Tasks[task.Task.ID] = task
	}
	for _, job := range stored.Jobs {
		restored.Jobs[job.ID] = job
	}
	for _, job := range stored.DeadLetters {
		restored.DeadLetters[job.ID] = job
	}
	for _, subscription := range stored.Subscriptions {
		restored.Subscriptions[subscription.ID] = subscription
	}
	if stored.Compensations != nil {
		restored.Compensations = stored.Compensations
	}
	for _, batch := range stored.Batches {
		copied := &batchState{Info: batch.Info, Parts: make(map[string]*api.BatchPart, len(batch.Parts))}
		for _, part := range batch.Parts {
			copied.Parts[part.ID] = part
		}
		restored.Batches[batch.Info.ID] = copied
	}
	for _, historic := range stored.HistoricProcesses {
		restored.HistoricProcesses[historic.ID] = historic
	}
	for _, historic := range stored.HistoricActivities {
		restored.HistoricActivities[historic.ID] = historic
	}
	for _, historic := range stored.HistoricTasks {
		restored.HistoricTasks[historic.ID] = historic
	}
	for _, historic := range stored.HistoricVariables {
		restored.HistoricVariables[historic.ID] = historic
	}
	for _, link := range stored.InstanceLinks {
		restored.InstanceLinks[link.ID] = link
	}
	for _, link := range stored.HistoricLinks {
		restored.HistoricLinks[link.ID] = link
	}
	for _, detail := range stored.HistoricDetails {
		restored.HistoricDetails[detail.ID] = detail
	}
	for _, log := range stored.TaskLogs {
		restored.TaskLogs[log.ID] = log
	}
	restored.NextTaskLogID = stored.NextTaskLogID
	for _, comment := range stored.Comments {
		restored.Comments[comment.ID] = comment
	}
	for _, attachment := range stored.Attachments {
		restored.Attachments[attachment.ID] = attachment
	}
	if stored.AttachmentContents != nil {
		restored.AttachmentContents = stored.AttachmentContents
	}
	for _, user := range stored.Users {
		restored.Users[user.ID] = user
	}
	if stored.UserInfo != nil {
		restored.UserInfo = stored.UserInfo
	}
	if stored.UserPictures != nil {
		restored.UserPictures = stored.UserPictures
	}
	if stored.Properties != nil {
		restored.Properties = stored.Properties
	}
	for _, group := range stored.Groups {
		restored.Groups[group.ID] = group
	}
	for _, role := range stored.Roles {
		restored.Roles[role.ID] = role
	}
	if stored.Memberships != nil {
		restored.Memberships = stored.Memberships
	}
	for key, version := range stored.Versions {
		restored.Versions[key] = version
	}
	e.state = restored
	return nil
}
