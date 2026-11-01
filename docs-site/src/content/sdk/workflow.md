---
title: workflow 业务门面
description: 两路接入 workflow 二次封装：/workflow/** HTTP 门面与 api.WorkflowFacade，含路由接线现状、R 协议与存储差异。
order: 14
section: sdk
status: prototype
since: v0.1.1
tags: [workflow, 门面, REST, 接入]
---

## 定位

workflow 二次封装在引擎之上提供业务语义：发起、办结、退回、撤回、直送、加签减签、会签、待办已办、处理人调整、委托、催办、模型与步骤配置。它复用同一套 <code>ACT_*</code> 数据模型，额外写入 <code>workflow_*</code> 扩展表。

同一份门面实现对外提供两条接入路径：

| 路径 | 入口 | 适用 |
| --- | --- | --- |
| HTTP | <code>/workflow/{模块}/{方法}</code> | 已有前端、管理台或跨语言调用 |
| Go | <code>app.Workflow()</code>（<code>api.WorkflowFacade</code>） | 宿主进程内直接调用 |

两条路径共用 <code>internal/workflow</code> 的同一实例，不复制业务逻辑。与 <code>/service/**</code> 的区别：<code>/service/**</code> 是 Flowable 官方 REST 契约，<code>/workflow/**</code> 是业务封装协议，方法名与路径各自独立。

## 接线现状

路由表共 95 条，已接线 84 条，保留 501 共 11 条：

| 类别 | 数量 | 说明 |
| --- | --- | --- |
| 导入导出聚合 | 3 | <code>import/xml</code>、<code>importProcess</code>、<code>exportProcess</code>（单模型 XML 导出已接） |
| public 变体 | 2 | <code>public/deploymentId</code>、<code>public/processes/id</code> |
| 运行时注入 | 1 | <code>inject/node</code> |
| Java 遗留语义 | 2 | <code>exmple/node/list</code>、<code>wait/node/list</code> |
| 部署快照只读 | 3 | <code>hisstep</code> 增删改 |

保留端点返回 HTTP 501 与 <code>{"code":"500","ErrorMessage":"接口未实现","success":false}</code>；未注入门面服务时所有端点都回落到该桩，不会 404。

## R 协议

所有方法返回统一信封，成功与业务失败都是 HTTP 200：

~~~json
{"code":"200","msg":"","data":{"processInstanceId":"..."},"success":true}
~~~

~~~json
{"code":"500","ErrorMessage":"流程定义未部署","success":false}
~~~

失败时只填 <code>ErrorMessage</code>（首字母大写，兼容 Java 侧字段名）与 <code>success=false</code>，不填 <code>msg</code>。参数解析失败或系统错误才使用 500 状态码。

请求参数以 query 优先、JSON body 兜底；路径参数按路由模板解析。租户来自 <code>X-Tenant-Id</code> 头、<code>tenantId</code> 查询参数或已认证身份。

## 门面方法分组

| 分组 | 代表方法 |
| --- | --- |
| 发起 | <code>StartProcess</code>、<code>StartProcessAndFirstStep</code> |
| 流转 | <code>Complete</code>、<code>CompleteToTargetActivity</code>、<code>BackTask</code>、<code>BackToStepTask</code>、<code>DoRollbackTask</code>、<code>DoResetProcess</code> |
| 待办已办 | <code>GetTodoTaskListByIdentityID</code>、<code>GetDoneTaskListByIdentityID</code> |
| 推演与预存 | <code>GetNextUserTaskAndUserList</code>、<code>GetNextUserTaskAndUserListByKey</code>、<code>SaveAgreeNextActivity</code> |
| 状态与变量 | <code>IsProcessDeployed</code>、<code>IsExistRunningProcess</code>、<code>GetWorkflowStatusByBusinessID</code>、<code>UpdateProcessGlobalVariable</code>、<code>GetProcessGlobalVariableByBusinessID</code> |
| 表单回写 | <code>UpdateWorkflowDocumentByProcessInstanceIDOrTaskID</code> |
| 处理人调整 | <code>DoTaskOwnerAdjustment</code>、<code>DoTaskOwnerAdjustmentByID</code> |
| 会签加签 | <code>AddExecution</code>、<code>DelExecution</code>、<code>AddMultiInstanceMember</code>、<code>RemoveMultiInstanceMember</code>、<code>CompleteSign</code> |
| 生命周期 | <code>DoStopProcessInstance</code>、<code>DoSuspendProcessInstance</code>、<code>DoActivateProcessInstance</code>、<code>DoDeleteProcessInstance</code>、<code>DoReactivateProcess</code> |
| 历史与消息 | <code>GetTaskHistoryListByProcessInstanceID</code>、<code>GetTaskMessageListByBusinessID</code>、<code>GetTaskInfoByTaskID</code> |

完整方法签名见 <code>api/workflow.go</code>。方法返回 <code>(R, error)</code>：<code>error</code> 只表示调用失败，业务结论看 <code>R.Success</code> 与 <code>R.Data</code>。

## 存储模式差异

workflow 依赖 22 张 <code>workflow_*</code> 扩展表，由 <code>EnsureMySQLSchema</code> 与引擎表一起校验（引擎 41 张 <code>ACT_*</code> 表）。

| 模式 | 装配方式 | 业务数据 |
| --- | --- | --- |
| MySQL 行仓储 | <code>NewEmbedded</code> 或独立服务 <code>-storage mysql-rows</code> | <code>workflow_*</code> 行表 |
| 内存 / SQL 快照 | 独立服务默认或内存模式 | 进程内存 |

SQL 行仓储模式下，依赖待办清单的自动办结、超时提醒与催办扫描使用空待办源占位，不会命中数据；内存模式使用内存待办源。接入前需按业务确认这些扫描是否必须。

## 生产装配要点

嵌入 SDK 已封装以下步骤（独立服务在 <code>cmd/flowtide-server</code> 中做同样的事）：

1. 使用严格构造器装配 SQL 存储，SQL 失败直接返回错误，不回退内存。
2. 注册执行人重写钩子：任务创建时回填预存的下一步执行人。
3. 注册会签运行时：会签集合配置、权重监听器与比例规则在此预热，未注册时会签配置不生效。
4. 注册业务回调钩子：任务创建后锁外 HTTP 回调，失败落异常表。

独立服务还会启动 workflow 后台调度（自动办结、超时提醒、催办扫描、僵尸监控）；嵌入 SDK 不暴露该调度入口，需要时按宿主约定自行驱动或改用独立服务。

## 管理台

独立服务的管理台内置“工作流”九个页面：流程分类、流程模型、流程配置、待办任务、已办任务、流程实例、委托配置、催办配置、流程异常。页面数据来自 <code>/workflow/**</code>，501 端点降级为空态而非报错。

## 已知边界

- 引擎命令与 workflow 多表写入不在同一事务内，失败补偿沿用现有实现。
- 导入导出聚合、<code>inject/node</code>、Java 遗留节点清单与 <code>hisstep</code> 增删改未接线。
- <code>workflow_*</code> 表结构与 Java 原库的差异需按接入方数据单独核对，通用库不记录接入方信息。

## 相关阅读

- [宿主嵌入 SDK](/docs/sdk/embedding)
- [管理台](/docs/operations/admin-console)
- [REST 覆盖与差异](/docs/rest/coverage)
- [SDK 限制](/docs/sdk/limits)
