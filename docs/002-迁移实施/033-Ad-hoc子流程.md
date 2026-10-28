# 033 Ad-hoc 子流程

> 文档编号：002-033
> 更新日期：2026-10-28
> 对应计划：006 流程执行引擎 PVM 设计；005 BPMN 模型、XML 解析与部署校验

## 1. 背景

Flowable 的 Ad-hoc 子流程（Ad-hoc SubProcess）用于“活动集合由执行时决定”的场景：子流程内没有开始事件和顺序流，引擎在进入时按配置并行或串行激活内部活动，全部完成或满足完成条件后离开子流程。

本期实现安全子集：**Parallel / Sequential 两种顺序 + completionCondition**，优先保证“并行 Ad-hoc + 完成条件 + 用户任务”的兼容性。

## 2. 支持范围

| 能力 | 支持情况 |
| --- | --- |
| `<adHocSubProcess>` 元素 | 支持 |
| `<subProcess adHoc="true">` 兼容写法 | 支持 |
| `ordering="Parallel"`（默认） | 支持，每个内部活动一个子执行 |
| `ordering="Sequential"` | 支持，按文档顺序一次激活一个内部活动 |
| 内部用户任务 | 支持，各生成独立任务 |
| 内部服务任务/脚本任务/手动任务 | 支持，完成后按同样规则推进 |
| `<completionCondition>` 子元素 | 支持 |
| `completionCondition` 扩展属性 | 支持（Flowable 兼容） |
| 无内部活动 | 支持，进入后直接沿出线继续 |

## 3. 模型与解析

`model.SubProcess` 新增三个字段：

| 字段 | 说明 |
| --- | --- |
| `AdHoc bool` | 是否 Ad-hoc 子流程 |
| `AdHocOrdering string` | `Parallel`（默认）或 `Sequential`，解析时已规范化 |
| `CompletionCondition string` | 完成条件表达式 |

解析入口为 `internal/bpmn/xml/parse.go`：

```xml
<adHocSubProcess id="reviewAdhoc" name="材料补录" ordering="Parallel">
  <userTask id="contract" name="补录合同"/>
  <userTask id="invoice" name="补录发票"/>
  <completionCondition>${approved == true}</completionCondition>
</adHocSubProcess>
```

兼容写法：

```xml
<subProcess id="reviewAdhoc" adHoc="true" ordering="Sequential">
  <userTask id="contract" name="补录合同"/>
  <userTask id="invoice" name="补录发票"/>
</subProcess>
```

解析规则：

- `ordering` 缺省或为空按 BPMN 规范默认为 `Parallel`；大小写不敏感。
- 非 `Parallel`/`Sequential` 的取值在部署阶段返回 `BPMN-1010`。
- `completionCondition` 优先取子元素文本，其次取扩展属性；均为空表示没有完成条件。
- Ad-hoc 子流程缺少 `id` 返回 `BPMN-1011`。
- Ad-hoc 子流程不校验开始事件；事件子流程（`triggeredByEvent="true"`）仍按原逻辑要求开始事件。
- `adHoc="true"` 与 `triggeredByEvent="true"` 同时出现时按 Ad-hoc 处理，忽略事件子流程标记。

## 4. 引擎语义

实现位于 `internal/engine/adhoc.go`，并复用现有执行/议程/历史机制，不新增执行状态字段，因此不影响持久化表结构。

### 4.1 进入

1. 校验 `ordering`，非法返回 `ENGINE-0513`。
2. 收集内部活动：按文档顺序取子流程内的流程节点，排除开始事件、结束事件、边界事件与事件子流程。
3. 无内部活动：结束子流程活动历史、取消边界定时器/订阅，直接沿子流程出线继续。
4. 有内部活动：创建 `Scope=true, ScopeRoot=true` 的子流程作用域执行，父执行进入等待。
5. 为作用域注册内部事件子流程的启动订阅（保持与普通子流程一致）。
6. Parallel：为每个内部活动创建子执行并调度进入；Sequential：只创建第一个子执行。

### 4.2 内部活动完成

内部活动完成后，`takeOutgoing` 在解析出线前识别“该执行是 Ad-hoc 作用域根的直接子执行”，转交 `finishAdHocActivity`：

1. 结束子执行，取消其边界定时器、边界订阅与事件订阅。
2. 若有 `completionCondition`，使用作用域可见变量求值：
   - 为真：取消作用域内所有剩余执行与任务，立即结束 Ad-hoc 作用域并沿子流程出线继续；
   - 求值失败：返回 `ENGINE-0514`，命令整体回滚。
3. Parallel：仍有未完成内部执行时继续等待；全部完成则离开子流程。
4. Sequential：按文档顺序找到下一个尚未激活的内部活动并创建子执行；没有下一个则离开子流程。
5. 离开子流程复用 `endExecution` 的 `ScopeRoot` 分支：结束子流程活动历史、恢复父执行、沿子流程出线继续，与普通子流程行为一致。

### 4.3 完成条件

- 只在“某个内部活动完成”后求值，进入时与等待期间不求值。
- 变量作用域为 Ad-hoc 作用域可见变量（流程实例变量、父作用域变量、作用域局部变量）。
- 完成条件为真时固定取消剩余实例，不提供 `cancelRemainingInstances="false"` 的保留语义。

## 5. 错误码

| 错误码 | 场景 |
| --- | --- |
| `BPMN-1010` | `ordering` 取值非法 |
| `BPMN-1011` | Ad-hoc 子流程缺少 `id` |
| `ENGINE-0513` | 运行时模型中的 `ordering` 非法（如持久化/外部构造模型） |
| `ENGINE-0514` | 完成条件表达式求值失败 |

## 6. 限制

1. Ad-hoc 子流程的内部活动不通过 `sequenceFlow` 串联；内部节点即使配置了出线，也会在活动完成后交回 Ad-hoc 作用域，不按内部出线路由。
2. 内部活动不支持 `asyncAfter`（完成后异步）；`asyncBefore` 仍由 `continueAt` 的既有异步作业逻辑处理。
3. 不承诺嵌套 Ad-hoc 子流程、Ad-hoc 内事件子流程与边界事件的组合语义；这些组合未纳入本期测试范围。
4. 不提供 Flowable 的 `nrOfInstances`/`nrOfCompletedInstances` 等 Ad-hoc 计数变量；完成条件请使用业务变量。
5. 串行模式只保证“按文档顺序一次一个”，不实现 Flowable 的“运行期任意挑选下一个活动”能力。
6. 取消剩余活动时统一结束任务、执行与订阅并写历史，不触发补偿。
7. Ad-hoc 子流程不修改部署/迁移/流程图生成逻辑，流程图中的元素类型仍按 `subProcess` 输出。

## 7. 测试

`internal/engine/adhoc_test.go` 覆盖：

- 并行两个用户任务全部完成并沿出线继续；
- 未配置 `ordering` 时默认按并行执行；
- Sequential 按文档顺序逐个激活；
- completionCondition 为真时提前结束并取消剩余任务；
- completionCondition 不满足时继续执行剩余活动；
- 无内部活动直接继续；
- Sequential 内部服务任务按文档顺序同步执行；
- `subProcess adHoc="true"` 兼容写法；
- 非法 `ordering` 部署失败与运行时模型校验；
- 缺少 `id` 部署失败；
- 定义中缺少 Ad-hoc 子流程时启动返回 `ENGINE-0501`；
- 完成条件表达式错误回滚。

验证命令：

```bash
GOCACHE=/root/gocache-sub-adhoc go test ./...
GOCACHE=/root/gocache-sub-adhoc go test ./internal/engine -coverprofile=/tmp/cover.out
GOCACHE=/root/gocache-sub-adhoc go tool cover -func=/tmp/cover.out | tail -1
```

`internal/engine` 语句覆盖率为 **90.8%**；新增 `adhoc.go` 中进入子流程、创建子执行、完成条件判定、串行推进与取消剩余活动等核心函数覆盖率为 100%，整体满足不低于 90% 的要求。
