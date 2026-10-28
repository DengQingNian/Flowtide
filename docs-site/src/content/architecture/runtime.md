---
title: 运行时与执行树
description: Command、Agenda、执行树、事务回滚、监听器与单执行者并发模型的实现方式。
order: 20
section: architecture
status: prototype
since: v0.1.0
tags: [运行时, PVM, Agenda, 事务]
---

## 命令边界

每个外部操作对应一个命令。命令开始后，引擎持有进程内互斥锁，建立统一的时间、ID 与事务顺序；命令结束前不会把中间状态暴露给其他请求。命令内任何错误都会回滚内存变更，历史与行写入也随事务边界处理。

~~~go
// 概念示意，非公开 API
func (e *Engine) command(ctx context.Context, fn func(*commandContext) error) error {
    e.mu.Lock()
    defer e.mu.Unlock()
    cc := newCommandContext(ctx)
    if err := fn(cc); err != nil {
        return err
    }
    return e.persist(cc)
}
~~~

## Agenda

Agenda 保存待执行操作。流程推进不是递归调用，而是把“执行活动”“离开活动”“触发事件”等操作放入队列，由循环依次取出。这样可以控制最大步数（默认 10,000），避免模型错误导致无限循环。

~~~text
take(sequenceFlow)
  └─ execute(activity)
       ├─ userTask  → 创建任务，等待
       ├─ serviceTask → 调用委托
       ├─ gateway → 选择后继
       └─ endEvent → 结束执行
~~~

## 执行树与作用域

执行树由父子执行组成。流程实例持有一个根执行；并行网关或多实例会创建子执行。变量查找沿父链向上，直到找到同名变量；写入默认写当前作用域，<code>SetVariable</code> 写流程实例作用域。

| 作用域 | 生命周期 | 持久化 |
| --- | --- | --- |
| 流程实例 | 从启动到结束 | 是 |
| 执行 | 随执行树创建/删除 | 是 |
| 任务局部 | 随任务 | 是 |
| 瞬态 | 单条命令 | 否 |

## 事件与监听器

- 执行监听器：start/end 等节点事件，可读写变量；错误会使命令回滚。
- 任务监听器：create/complete 事件，支持 class/delegateExpression 的 Go 注册。
- 事件订阅：消息/信号/条件在等待点创建，触发后删除；持久化后可在重启时恢复。

监听器是进程内 Go 函数。引擎重启后需要重新注册；没有跨进程类加载或反射实例化。

## 并发与隔离

| 维度 | 当前实现 |
| --- | --- |
| 进程内并发 | 单写锁，命令串行 |
| 多进程 | 数据库独占锁，单执行者 |
| 作业执行 | 后台单扫描器，按间隔执行到期作业 |
| 集群 | 无租约、抢占、故障转移 |
| 事务 | 命令级内存事务；行仓储命令后增量写 |

::: danger 不要共享状态源
两个进程同时打开同一个 JSON 状态文件或同一个行仓储库会造成状态分叉。数据库锁只能阻止第二写者，不能替代集群协调。
:::

## 失败与恢复

- 命令失败：内存回滚，不写入历史。
- JSON 快照失败：原子写临时文件失败则不替换旧文件。
- 行仓储失败：命令不提交，连接丢失时拒绝继续写入。
- 作业失败：按退避重试，耗尽后进入死信。
- 外部副作用（HTTP、消息）不随内存回滚，需要业务幂等。

## 性能特征

首轮本机基准（QEMU、4 vCPU、内存状态）显示：启动并完成用户任务约 23 ms，完成任务约 15 ms，1000 任务分页查询约 5 ms，1000 行全量快照约 275 ms。绝对值不能外推到生产硬件；状态复制、历史写入和全量导出是主要成本。

## 相关阅读

- [BPMN 内核进度](/docs/progress/kernel)
- [持久化架构](/docs/architecture/persistence)
- [容量基线](/docs/operations/storage)
