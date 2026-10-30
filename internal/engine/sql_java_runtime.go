package engine

import (
	"github.com/DengQingNian/Flowtide/api"
	flowablerow "github.com/DengQingNian/Flowtide/internal/persistence/flowable"
)

// restoreJavaExecutionTree 补齐无 Go 扩展的简单 Java 根子执行完成计数。
// @param snapshot 本次导入的标准运行时行
// @param controls 已恢复的 Go 控制扩展
// @return 复杂或损坏的原始执行结构错误
func (e *Engine) restoreJavaExecutionTree(snapshot flowablerow.RuntimeSnapshot, controls []runtimeControl) error {
	controlled := make(map[string]bool, len(controls))
	for _, control := range controls {
		controlled[control.Instance.ID] = true
	}
	children := make(map[string][]*executionState)
	instances := make(map[string]bool)
	// 只处理本次导入且没有控制扩展的树，不能覆盖 Go 并行或多实例计数。
	for _, item := range snapshot.Executions {
		if controlled[item.ProcessInstanceID] {
			continue
		}
		instances[item.ProcessInstanceID] = true
		current := e.state.Executions[item.ID]
		if current.ParentID != "" {
			children[current.ParentID] = append(children[current.ParentID], current)
		}
	}
	for instanceID := range instances {
		instance := e.state.Instances[instanceID]
		root := e.state.Executions[instance.RootExecutionID]
		if root == nil || len(children[root.ID]) == 0 {
			continue
		}
		// 首期仅恢复 Java 的空活动流程根与唯一用户任务叶，不推测复杂作用域语义。
		if root.ActivityID != "" || len(children[root.ID]) != 1 {
			return api.NewError("ENGINE-0921", "原始执行树缺少已验证的续跑控制，需要迁移转换", api.ErrBadRequest)
		}
		child := children[root.ID][0]
		if len(children[child.ID]) != 0 || child.ScopeRoot || child.Concurrent || child.EventSubProcessRoot || !child.Waiting {
			return api.NewError("ENGINE-0921", "原始复杂子执行缺少已验证的续跑控制，需要迁移转换", api.ErrBadRequest)
		}
		root.PendingBranches = 1
	}
	return nil
}
