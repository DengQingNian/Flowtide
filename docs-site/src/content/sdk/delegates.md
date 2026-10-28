---
title: 委托与监听器
description: 注册服务任务委托、任务监听器与执行监听器，理解生命周期、变量访问和错误传播。
order: 40
section: sdk
status: prototype
since: v0.1.0
tags: [SDK, 委托, 监听器]
---

## 服务任务委托

<code>serviceTask</code> 通过 <code>flowable:delegateExpression</code> 或委托名绑定到 Go 函数。注册发生在引擎实例上：

~~~go
app.RegisterDelegate("notify", func(ctx context.Context, execution *flowable.Execution) error {
	orderID, ok := execution.GetVariable("orderId")
	if !ok {
		return errors.New("缺少 orderId")
	}
	execution.SetVariable("notified", true)
	execution.SetVariableLocal("attempt", 1)
	log.Printf("通知订单 %v", orderID)
	return nil
})
~~~

<code>ExecutionContext</code> 提供 <code>GetVariable</code>、<code>SetVariable</code>、<code>SetVariableLocal</code>、<code>ProcessInstanceID</code> 与 <code>ActivityID</code>。

## 任务监听器

任务监听器在 create/complete 等事件触发，注册名与 BPMN 的 <code>flowable:taskListener</code> 匹配：

~~~go
app.TaskService().RegisterTaskListener("auditTask", func(ctx context.Context, task *api.Task) error {
	log.Printf("task %s event assignee=%s", task.ID, task.Assignee)
	return nil
})
~~~

任务监听器返回错误会使当前命令失败并回滚。

## 执行监听器

执行监听器在节点 start/end 触发，可读取当前活动与变量。适合做审计、指标或轻量校验；不要在其中执行不可回滚的外部副作用。

## 生命周期

| 事项 | 行为 |
| --- | --- |
| 注册时机 | 引擎构造后、命令执行前 |
| 作用域 | 单个引擎实例 |
| 重启 | 委托与监听器不会持久化，必须重新注册 |
| 同名 | 后注册覆盖先注册 |
| 并发 | 命令串行，回调不并行执行 |
| 类加载 | 无；不存在从 BPMN 反射实例化 Go 类型 |

::: warning 重启陷阱
状态文件或数据库恢复的是流程状态，不包含 Go 函数。服务重启后若忘记注册委托，服务任务会报 <code>ENGINE-0510</code> 并阻塞流程。
:::

## 错误与重试

- 返回普通 error：命令失败，内存状态回滚；如果是异步作业，按退避重试。
- 返回 <code>*api.Error</code>：错误码会进入 REST 错误体与作业异常栈。
- 外部调用（HTTP、消息）不会随内存回滚，业务实现必须幂等或使用补偿。
- 死信作业可通过管理 API 重试或移动回可执行队列。

## 与 Java 的差异

| Java | Go |
| --- | --- |
| 实现 <code>JavaDelegate</code> 类 | 注册 <code>func(context.Context, *Execution) error</code> |
| 类加载与 Spring 注入 | 进程内函数，依赖需闭包捕获 |
| <code>DelegateExecution</code> | <code>*flowable.Execution</code> |
| 任务监听器类 | <code>func(context.Context, *api.Task) error</code> |
| 表达式实例化 | 仅按注册名查找 |

## 相关阅读

- [SDK 服务接口](/docs/sdk/services)
- [BPMN 元素支持](/docs/compatibility/bpmn)
- [作业与定时器](/docs/operations/troubleshooting)
