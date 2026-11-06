package engine

import (
	"context"
	"strconv"
	"strings"

	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// IsMultiInstanceLastPerson 判断任务是否为其节点的最后一名待办成员（对位 Java ProcessesUtil#isMultiInstanceLastPerson:10437-10480）。
// 语义：
//  1. 任务不存在 → false（Java task==null 口径，不报错）；
//  2. 节点非用户任务 → false；
//  3. 用户任务无多实例配置 → true（非会签节点当前处理人即最后一个处理人员）；
//  4. 多实例带完成条件 → 按比例判定：Java 口径 split("=") 取比例值（去掉 "}"），
//     (completed+1)/all 为整数除法，结果 >= proportion 时为 true；
//  5. 多实例无完成条件 → false（Java fallthrough）。
//
// @param ctx 上下文
// @param taskID 任务 ID
// @return 是否为最后一名待办成员
// @return 查询错误
func (e *Engine) IsMultiInstanceLastPerson(ctx context.Context, taskID string) (bool, error) {
	result, err := e.read(func() (any, error) {
		task, ok := e.findTask(taskID)
		if !ok {
			return false, nil
		}
		if err := e.allowTask(ctx, task.Task.ID); err != nil {
			return nil, err
		}
		definition, ok := e.findDefinition(task.Task.ProcessDefinitionID)
		if !ok {
			return false, nil
		}
		node, found := findElement(processOfDefinition(definition), task.Task.TaskDefinitionKey)
		if !found {
			return false, nil
		}
		userTask, ok := node.(*model.UserTask)
		if !ok {
			return false, nil
		}
		loop := userTask.LoopCharacteristics
		if loop == nil {
			// 非会签节点当前处理人即最后一个处理人员（Java 直返 true）
			return true, nil
		}
		root := e.miRootOfExecution(task.ExecutionID)
		if root == nil {
			return false, nil
		}
		proportion, ok := multiInstanceProportion(loop.CompletionCondition)
		if !ok {
			return false, nil
		}
		completed := e.counter(root, "nrOfCompletedInstances")
		all := e.counter(root, "nrOfInstances")
		// Java 口径：(completed+1)/all 整数除法后再与比例比较；分母为 0 直接 false 防御。
		if all <= 0 {
			return false, nil
		}
		if float64((completed+1)/all) >= proportion {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return false, err
	}
	return result.(bool), nil
}

// miRootOfExecution 沿执行父链查找任务所属的多实例根执行（含任务执行本身即根的情况）。
// @param executionID 任务所属执行 ID
// @return 多实例根执行，不在多实例结构内返回 nil
func (e *Engine) miRootOfExecution(executionID string) *executionState {
	current, ok := e.findExecution(executionID)
	if !ok {
		return nil
	}
	// 圈复杂度 >= 3：沿父链上溯，当前执行本身为根时直接命中。
	for current != nil {
		if current.MultiInstanceRoot {
			return current
		}
		if current.ParentID == "" {
			return nil
		}
		parent, ok := e.findExecution(current.ParentID)
		if !ok {
			return nil
		}
		current = parent
	}
	return nil
}

// multiInstanceProportion 从完成条件表达式解析比例值（Java 口径）。
// Java 用 completionCondition.split("=") 取 split[1] 并去掉 "}"，例如
// `${nrOfCompletedInstances/nrOfInstances >=0.8}` → "0.8}" → 0.8。
// @param condition 完成条件表达式原文
// @return 比例值
// @return 是否可解析
func multiInstanceProportion(condition string) (float64, bool) {
	parts := strings.Split(condition, "=")
	// Java split[1] 一定存在时才走比例分支；不含 "=" 或其后为空白则跳过（fallthrough → false）
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		return 0, false
	}
	raw := strings.TrimSpace(strings.ReplaceAll(parts[1], "}", ""))
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}
