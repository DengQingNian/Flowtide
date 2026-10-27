package engine

import "fmt"

// operation 是议程中的原子操作。
type operation interface {
	// run 执行操作。
	// @param cc 命令上下文
	// @return 执行错误
	run(cc *commandContext) error
}

// operationFunc 将函数适配为 operation。
type operationFunc func(cc *commandContext) error

// run 执行函数。
// @param cc 命令上下文
// @return 执行错误
func (f operationFunc) run(cc *commandContext) error { return f(cc) }

// agenda 串行执行操作队列。
type agenda struct {
	operations []operation // 待执行操作
	maxSteps   int         // 最大步数
	steps      int         // 已执行步数
}

// newAgenda 创建议程。
// @param maxSteps 最大步数
// @return 议程
func newAgenda(maxSteps int) *agenda {
	return &agenda{maxSteps: maxSteps}
}

// schedule 追加操作。
// @param op 操作
func (a *agenda) schedule(op operation) { a.operations = append(a.operations, op) }

// run 执行直到队列为空。
// @param cc 命令上下文
// @return 执行错误
func (a *agenda) run(cc *commandContext) error {
	for len(a.operations) > 0 {
		op := a.operations[0]
		a.operations = a.operations[1:]
		a.steps++
		// 防止错误模型导致无限循环
		if a.steps > a.maxSteps {
			return fmt.Errorf("议程执行步数超过上限 %d", a.maxSteps)
		}
		if err := op.run(cc); err != nil {
			return err
		}
	}
	return nil
}
