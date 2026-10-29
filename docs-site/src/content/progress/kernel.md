---
title: BPMN 内核与执行进度
description: 解析、执行树、任务、网关、事件、子流程与多实例的当前实现状态和边界。
order: 20
section: progress
status: prototype
since: v0.1.0
tags: [BPMN, 内核, 执行树]
---

## 实现范围

内核以 Flowable 的 Command + Agenda + 执行树（PVM）模型为参照，但只实现当前测试与样例覆盖到的语义。所有命令在单一进程内串行执行，命令失败回滚内存状态；数据库行写入与引擎状态更新仍在同一临界区内完成。

| 能力组 | 已实现 | 状态 |
| --- | --- | --- |
| 模型解析 | 命名空间、DTD 拒绝、深度/大小限制、ZIP 路径穿越防护 | 已验证 |
| 流程部署 | 资源解析、版本递增、重复过滤、资源下载、ETag/304 | 已验证 |
| 执行树 | 根与活动执行分离、变量继承、活动迁移、删除级联；Java 两层拓扑及旧 Go 单根续跑通过 | 原型可用 |
| 用户任务 | 创建、签收、完成、委派/处理、候选人/组、监听器 | 原型可用 |
| 服务任务 | Go 委托注册表、<code>delegateExpression</code>、表达式结果变量 | 原型可用 |
| 网关 | 排他、并行、包容、事件网关、默认流 | 原型可用 |
| 子流程 | 嵌入式、嵌套、事件子流程、Ad-hoc、事务子流程 | 原型可用 |
| 多实例 | 并行/串行、集合/基数、完成条件、提前取消 | 原型可用 |
| 调用活动 | 变量继承、in/out 映射、调用深度限制 | 原型可用 |
| 事件 | 消息/信号启动与捕获、边界、链接、条件事件 | 原型可用 |
| 定时器 | 边界、启动、中间捕获、重复周期、作业扫描 | 原型可用 |
| 错误与补偿 | 错误结束/边界、作用域传播、补偿处理器逆序执行 | 原型可用 |
| 脚本任务 | expression/juel/javascript 表达式子集 | 仅表达式子集 |
| 流程迁移 | 活动映射、任务重映射、目标定义校验 | 原型可用 |

## 执行模型

一次外部调用对应一个命令：进入引擎临界区、建立命令上下文、克隆/恢复状态、推进 Agenda，最后统一写入历史与持久化。命令内的时间、ID 与事务顺序保持一致；执行监听器返回错误时整条命令回滚。

~~~text
REST / SDK
   │
   ▼
Command ──► Agenda ──► Operation ──► Behavior
   │                                  │
   │                                  ├─ 执行树/作用域
   │                                  ├─ 任务/作业
   │                                  └─ 历史/变量
   ▼
状态提交（内存 / JSON / SQL 快照 / 行仓储）
~~~

这套模型的直接限制是：状态复制是 O(state) 的，实例和任务规模增长后单命令成本会上升。首轮本机基准中，启动并完成一个用户任务约 23 ms、内存分配约 7.6 MB；这是 QEMU 虚拟环境下的观察值，不是生产容量。

## 已覆盖的 BPMN 元素

<code>startEvent</code>、<code>endEvent</code>、<code>userTask</code>、<code>serviceTask</code>、<code>scriptTask</code>、<code>manualTask</code>、<code>callActivity</code>、<code>subProcess</code>、<code>adHocSubProcess</code>、事件子流程、<code>boundaryEvent</code>、中间捕获/抛出事件、排他/并行/包容/事件网关、<code>sequenceFlow</code>、<code>association</code>。

扩展元素包括消息、信号、错误定义、定时器、条件事件、多实例、in/out 参数、补偿、<code>asyncBefore/asyncAfter</code>、<code>formKey</code>、候选人/候选组。

::: info 与 Java 的差异
事件子流程、补偿、事务子流程和迁移做了简化；复杂边界语义仍需与 Java 参考实现做双向路径对照。当前没有 Java 路径回放流水线。
:::

## 明确限制

- 不支持完整 JavaScript 语句、Groovy、Starlark；脚本任务只接受表达式子集。
- 不提供外部工作项（External Worker）、CMMN 与完整 DMN 引擎。
- 复杂网关、动态 BPMN 与部分定时器周期语义未覆盖。
- 集群作业抢占、租约、幂等与故障恢复未实现。
- 执行语义的“已编码”不等于“Java 兼容已验收”。

## 验证入口

~~~bash
HOME=/root make check
HOME=/root make race
~~~

<code>make check</code> 覆盖格式、中文函数注释、vet、普通测试、覆盖率、基线摘要、端点清单、独立 SDK 模块与无 CGO 构建；<code>make race</code> 单独运行竞争检测。真实数据库路径使用 <code>make test-mysql</code> 与 <code>make test-postgres</code>。

## 相关阅读

- [运行时架构](/docs/architecture/runtime)
- [BPMN 元素支持](/docs/compatibility/bpmn)
- [核心概念](/docs/getting-started/concepts)
