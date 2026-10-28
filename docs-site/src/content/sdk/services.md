---
title: 服务接口
description: 按职责梳理 Repository、Runtime、Task、History、Management、Identity、Form 与 DMN 的常用方法。
order: 30
section: sdk
status: prototype
since: v0.1.0
tags: [SDK, 服务, API]
---

## RepositoryService

| 方法 | 用途 |
| --- | --- |
| <code>Deploy</code> | 部署 BPMN/资源 |
| <code>ListProcessDefinitions</code> / <code>GetProcessDefinition</code> | 查询定义 |
| <code>GetBpmnModel</code> / <code>GetProcessDefinitionXML</code> | 读取模型与 XML |
| <code>ListDeployments</code> / <code>GetDeployment</code> | 部署查询 |
| <code>ListDeploymentResources</code> / <code>GetDeploymentResource</code> | 资源读取 |
| <code>DeleteDeployment</code> | 级联或非级联删除 |
| <code>SuspendProcessDefinition</code> / <code>ActivateProcessDefinition</code> | 挂起/激活 |
| <code>CreateModel</code> / <code>UpdateModel</code> / <code>DeleteModel</code> | 模型仓库 |
| <code>AddDefinitionIdentityLink</code> 等 | 定义候选发起人 |

## RuntimeService

| 方法 | 用途 |
| --- | --- |
| <code>StartProcessInstance</code> | 按 Key/ID 启动，支持瞬态变量 |
| <code>GetProcessInstance</code> / <code>ListProcessInstances</code> | 实例查询 |
| <code>ListExecutions</code> / <code>GetExecution</code> | 执行树 |
| <code>GetVariables</code> / <code>SetVariable</code> / <code>DeleteVariable</code> | 实例变量 |
| <code>GetExecutionVariable</code> 等 | 执行变量 |
| <code>MessageEventReceived</code> / <code>SignalEventReceived</code> | 消息/信号触发 |
| <code>TriggerExecution</code> | 无事件等待触发 |
| <code>SuspendProcessInstance</code> / <code>ActivateProcessInstance</code> | 挂起/激活 |
| <code>MigrateProcessInstance</code> / <code>ValidateMigration</code> | 实例迁移 |
| <code>ChangeActivityState</code> | 活动迁移 |
| <code>ListActivityInstances</code> | 运行中活动 |
| <code>ListEventSubscriptions</code> | 事件订阅 |
| <code>InjectUserTask</code> | 动态注入任务 |

## TaskService

| 方法 | 用途 |
| --- | --- |
| <code>ListTasks</code> / <code>GetTask</code> | 查询 |
| <code>CreateTask</code> / <code>UpdateTask</code> / <code>DeleteTask</code> | 独立任务 |
| <code>Claim</code> / <code>Complete</code> / <code>Delegate</code> / <code>Resolve</code> | 任务动作 |
| <code>GetTaskVariables</code> / <code>SetTaskVariable</code> | 任务变量 |
| <code>AddCandidateUser</code> / <code>AddCandidateGroup</code> | 候选人 |
| <code>AddComment</code> / <code>ListComments</code> | 评论 |
| <code>AddAttachment</code> / <code>GetAttachmentContent</code> | 附件 |
| <code>RegisterTaskListener</code> | 任务监听器 |

## HistoryService

| 方法 | 用途 |
| --- | --- |
| <code>GetHistoricProcessInstance</code> / <code>ListHistoricProcessInstances</code> | 实例历史 |
| <code>ListHistoricActivityInstances</code> | 活动历史 |
| <code>GetHistoricTaskInstance</code> / <code>ListHistoricTaskInstances</code> | 任务历史 |
| <code>ListHistoricVariableInstances</code> | 变量历史 |
| <code>ListHistoricDetails</code> | 明细 |
| <code>ListHistoricTaskLogs</code> | 任务日志 |
| <code>GetHistoricTaskFormData</code> | 历史表单 |

## ManagementService

作业、死信、定时/挂起/历史作业、表元数据与数据、批次、引擎属性。常用：<code>RunDueJobs</code>、<code>ListJobs</code>、<code>SetJobRetries</code>、<code>MoveDeadLetterJobToExecutable</code>、<code>TableCount</code>。

## IdentityService 与 FormService

身份服务提供用户、组、角色、成员关系、密码校验、头像与用户信息键值；表单服务提供启动/任务表单读取与提交。默认身份库存储在引擎状态中，不连接外部 LDAP。

## DMNService

当前只提供决策定义与决策表元数据查询，以及引擎内部的决策表求值。完整 DMN 引擎、DRD 与 BKM 未实现。

## 参数与返回

- 分页使用 <code>Start</code>/<code>Size</code>，返回 <code>api.Page[T]</code>，字段为 <code>data/total/start/size</code>。
- 变量使用 <code>api.Variable{Name, Type, Value, Scope}</code>。
- 错误使用 <code>*api.Error</code>，可用 <code>errors.Is</code> 判断哨兵错误。
- 所有方法接收 <code>context.Context</code>，取消会传递到命令边界。

## 相关阅读

- [委托与监听器](/docs/sdk/delegates)
- [错误处理](/docs/sdk/errors)
- [REST 分域端点](/docs/rest/domains)
