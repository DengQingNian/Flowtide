package engine

import (
	"strings"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
	"github.com/DengQingNian/Flowtide/internal/expr"
)

// executeScriptTask 执行脚本任务。
// 支持 expression/juel/javascript 的表达式子集；不支持 Groovy/Starlark。
// @param cc 命令上下文
// @param execution 执行
// @param task 脚本任务
// @return 执行错误
func (e *Engine) executeScriptTask(cc *commandContext, execution *executionState, task *model.ScriptTask) error {
	format := strings.ToLower(strings.TrimSpace(task.ScriptFormat))
	switch format {
	case "", "expression", "juel", "javascript", "js":
	case "groovy", "starlark":
		return api.NewError("ENGINE-0701", "不支持的脚本格式: "+task.ScriptFormat, nil)
	default:
		return api.NewError("ENGINE-0702", "未知脚本格式: "+task.ScriptFormat, nil)
	}
	source := strings.TrimSpace(task.Script)
	source = strings.TrimSuffix(source, ";")
	source = strings.TrimSpace(strings.TrimPrefix(source, "return"))
	if source == "" {
		return api.NewError("ENGINE-0703", "脚本内容为空", api.ErrBadRequest)
	}
	compiled, err := expr.CompileExpression(source)
	if err != nil {
		return api.NewError("ENGINE-0704", "脚本编译失败", err)
	}
	result, err := compiled.Eval(cc.ctx, &expr.Env{Vars: e.variablesOf(execution)})
	if err != nil {
		return api.NewError("ENGINE-0705", "脚本执行失败", err)
	}
	if task.ResultVariable != "" {
		e.setVariable(execution, task.ResultVariable, result)
	}
	return nil
}
