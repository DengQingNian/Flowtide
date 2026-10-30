package workflow

import (
	"context"
	"time"

	"github.com/DengQingNian/Flowtide/api"
)

// fakeCommentTasks 是 TaskCommentEngine 的测试桩（记录调用与预置评论）。
type fakeCommentTasks struct {
	addComments      []api.Comment // AddComment 调用记录
	addProcess       []api.Comment // AddProcessComment 调用记录
	comments         []api.Comment // ListInstanceComments 返回值
	failListInstance bool          // 查询评论时返回错误
}

// AddComment 记录任务级评论调用。
// @param ctx 上下文
// @param taskID 任务 ID
// @param userID 作者
// @param message 内容
// @return 评论
// @return 错误
func (f *fakeCommentTasks) AddComment(ctx context.Context, taskID, userID, message string) (api.Comment, error) {
	comment := api.Comment{ID: "cmt", TaskID: taskID, ProcessInstanceID: "pi", Type: "comment", Message: message, UserID: userID, Time: time.Now()}
	f.addComments = append(f.addComments, comment)
	return comment, nil
}

// AddProcessComment 记录流程级评论调用。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @param userID 作者
// @param message 内容
// @return 评论
// @return 错误
func (f *fakeCommentTasks) AddProcessComment(ctx context.Context, processInstanceID, userID, message string) (api.Comment, error) {
	comment := api.Comment{ID: "cmt", ProcessInstanceID: processInstanceID, Type: "comment", Message: message, UserID: userID, Time: time.Now()}
	f.addProcess = append(f.addProcess, comment)
	return comment, nil
}

// ListInstanceComments 返回预置评论。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @return 评论列表
// @return 错误
func (f *fakeCommentTasks) ListInstanceComments(ctx context.Context, processInstanceID string) ([]api.Comment, error) {
	// 预置开关控制失败路径
	if f.failListInstance {
		return nil, api.NewError("FAKE-001", "查询评论失败", nil)
	}
	return f.comments, nil
}

// fakeTaskHistory 是 TaskHistoryQuery 的测试桩。
type fakeTaskHistory struct {
	tasks map[string]api.HistoricTaskInstance // 任务 ID → 历史任务
	miss  map[string]bool                     // 强制未命中的任务 ID
}

// GetHistoricTaskInstance 查询历史任务。
// @param ctx 上下文
// @param taskID 任务 ID
// @return 历史任务
// @return 错误
func (f *fakeTaskHistory) GetHistoricTaskInstance(ctx context.Context, taskID string) (api.HistoricTaskInstance, error) {
	// 强制未命中集合优先，用于覆盖查询失败路径
	if f.miss[taskID] {
		return api.HistoricTaskInstance{}, api.NewError("FAKE-002", "历史任务不存在", api.ErrNotFound)
	}
	if task, ok := f.tasks[taskID]; ok {
		return task, nil
	}
	return api.HistoricTaskInstance{}, api.NewError("FAKE-002", "历史任务不存在", api.ErrNotFound)
}

// fakeUsers 是 UserQuery 的测试桩。
type fakeUsers struct {
	users  map[string]api.User // 用户 ID → 用户
	miss   map[string]bool     // 强制未命中的用户 ID
	groups map[string][]api.Group
}

// GetUser 查询用户。
// @param ctx 上下文
// @param userID 用户 ID
// @return 用户
// @return 错误
func (f *fakeUsers) GetUser(ctx context.Context, userID string) (api.User, error) {
	// 强制未命中集合优先
	if f.miss[userID] {
		return api.User{}, api.NewError("FAKE-003", "用户不存在", api.ErrNotFound)
	}
	if user, ok := f.users[userID]; ok {
		return user, nil
	}
	return api.User{}, api.NewError("FAKE-003", "用户不存在", api.ErrNotFound)
}

// ListUserGroups 返回用户所属组。
// @param ctx 上下文
// @param userID 用户 ID
// @return 组列表
// @return 错误
func (f *fakeUsers) ListUserGroups(ctx context.Context, userID string) ([]api.Group, error) {
	// 无组记录时返回空列表
	return f.groups[userID], nil
}

// fakeInstances 是 InstanceQuery 的测试桩。
type fakeInstances struct {
	running map[string]api.ProcessInstance         // 运行实例
	history map[string]api.HistoricProcessInstance // 历史实例
}

// GetProcessInstance 查询运行实例。
// @param ctx 上下文
// @param id 实例 ID
// @return 实例
// @return 错误
func (f *fakeInstances) GetProcessInstance(ctx context.Context, id string) (api.ProcessInstance, error) {
	if instance, ok := f.running[id]; ok {
		return instance, nil
	}
	// 运行表查不到即返回不存在（模拟实例已结束）
	return api.ProcessInstance{}, api.NewError("FAKE-004", "流程实例不存在", api.ErrNotFound)
}

// GetHistoricProcessInstance 查询历史实例。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @return 历史实例
// @return 错误
func (f *fakeInstances) GetHistoricProcessInstance(ctx context.Context, processInstanceID string) (api.HistoricProcessInstance, error) {
	if instance, ok := f.history[processInstanceID]; ok {
		return instance, nil
	}
	return api.HistoricProcessInstance{}, api.NewError("FAKE-005", "历史实例不存在", api.ErrNotFound)
}

// fakeDefinitions 是 DefinitionQuery 的测试桩。
type fakeDefinitions struct {
	definitions map[string]api.ProcessDefinition // 定义 ID → 定义
}

// GetProcessDefinition 查询流程定义。
// @param ctx 上下文
// @param id 定义 ID
// @return 定义
// @return 错误
func (f *fakeDefinitions) GetProcessDefinition(ctx context.Context, id string) (api.ProcessDefinition, error) {
	if definition, ok := f.definitions[id]; ok {
		return definition, nil
	}
	return api.ProcessDefinition{}, api.NewError("FAKE-006", "流程定义不存在", api.ErrNotFound)
}
