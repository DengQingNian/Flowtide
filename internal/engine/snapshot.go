package engine

import (
	"encoding/json"
	"os"
	"path/filepath"

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
	Info api.ProcessDefinition `json:"info"` // 流程定义信息
	XML  []byte                `json:"xml"`  // 原始 BPMN XML
}

// snapshot 是引擎状态的持久化结构。
type snapshot struct {
	Deployments        []deploymentSnapshot            `json:"deployments"`        // 部署
	Definitions        []definitionSnapshot            `json:"definitions"`        // 流程定义
	Instances          []*instanceState                `json:"instances"`          // 流程实例
	Executions         []*executionState               `json:"executions"`         // 执行
	Tasks              []*taskState                    `json:"tasks"`              // 任务
	Jobs               []*jobState                     `json:"jobs"`               // 定时器作业
	Subscriptions      []*subscriptionState            `json:"subscriptions"`      // 事件订阅
	Compensations      map[string][]*compensationState `json:"compensations"`      // 补偿登记
	HistoricProcesses  []*api.HistoricProcessInstance  `json:"historicProcesses"`  // 历史流程
	HistoricActivities []*api.HistoricActivityInstance `json:"historicActivities"` // 历史活动
	HistoricTasks      []*api.HistoricTaskInstance     `json:"historicTasks"`      // 历史任务
	HistoricVariables  []*api.HistoricVariableInstance `json:"historicVariables"`  // 历史变量
	Users              []*api.User                     `json:"users"`              // 用户
	Groups             []*api.Group                    `json:"groups"`             // 组
	Memberships        map[string]bool                 `json:"memberships"`        // 成员关系
	Versions           map[string]int                  `json:"versions"`           // 版本号
}

// toSnapshot 将运行时状态转换为持久化结构。
// @return 快照
func (e *Engine) toSnapshot() *snapshot {
	out := &snapshot{Versions: make(map[string]int, len(e.state.Versions))}
	for _, deployment := range e.state.Deployments {
		out.Deployments = append(out.Deployments, deploymentSnapshot{Info: deployment.Info, Resources: deployment.Resources})
	}
	for _, definition := range e.state.Definitions {
		out.Definitions = append(out.Definitions, definitionSnapshot{Info: definition.Info, XML: definition.XML})
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
	for _, subscription := range e.state.Subscriptions {
		out.Subscriptions = append(out.Subscriptions, subscription)
	}
	out.Compensations = e.state.Compensations
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
	for _, user := range e.state.Users {
		out.Users = append(out.Users, user)
	}
	for _, group := range e.state.Groups {
		out.Groups = append(out.Groups, group)
	}
	out.Memberships = e.state.Memberships
	for key, version := range e.state.Versions {
		out.Versions[key] = version
	}
	return out
}

// saveState 原子写入状态文件。
// @return 写入错误
func (e *Engine) saveState() error {
	if e.cfg.StatePath == "" {
		return nil
	}
	data, err := json.Marshal(e.toSnapshot())
	if err != nil {
		return api.NewError("ENGINE-0901", "序列化引擎状态失败", err)
	}
	dir := filepath.Dir(e.cfg.StatePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return api.NewError("ENGINE-0902", "创建状态目录失败", err)
	}
	temp, err := os.CreateTemp(dir, ".flowable-state-*")
	if err != nil {
		return api.NewError("ENGINE-0903", "创建临时状态文件失败", err)
	}
	name := temp.Name()
	defer func() {
		// 失败路径清理临时文件
		_ = os.Remove(name)
	}()
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return api.NewError("ENGINE-0904", "写入状态失败", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return api.NewError("ENGINE-0905", "同步状态失败", err)
	}
	if err := temp.Close(); err != nil {
		return api.NewError("ENGINE-0906", "关闭状态文件失败", err)
	}
	if err := os.Rename(name, e.cfg.StatePath); err != nil {
		return api.NewError("ENGINE-0907", "替换状态文件失败", err)
	}
	return nil
}

// loadState 从状态文件恢复。
// @return 恢复错误
func (e *Engine) loadState() error {
	if e.cfg.StatePath == "" {
		return nil
	}
	data, err := os.ReadFile(e.cfg.StatePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return api.NewError("ENGINE-0910", "读取状态文件失败", err)
	}
	var stored snapshot
	if err := json.Unmarshal(data, &stored); err != nil {
		return api.NewError("ENGINE-0911", "解析状态文件失败", err)
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
		restored.Definitions[definition.Info.ID] = &definitionState{Info: definition.Info, Model: parsed, XML: definition.XML}
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
	for _, subscription := range stored.Subscriptions {
		restored.Subscriptions[subscription.ID] = subscription
	}
	if stored.Compensations != nil {
		restored.Compensations = stored.Compensations
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
	for _, user := range stored.Users {
		restored.Users[user.ID] = user
	}
	for _, group := range stored.Groups {
		restored.Groups[group.ID] = group
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
