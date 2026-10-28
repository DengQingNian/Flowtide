---
title: 从 Java Service API 迁移
description: Java 服务方法与 Go 接口的对应关系，以及参数、异常和生命周期的改写要点。
order: 80
section: sdk
status: prototype
since: v0.1.0
tags: [SDK, Java, 迁移, API]
---

## 总体差异

Java Flowable 通过 <code>ProcessEngine</code> 获取服务，服务方法大量使用查询对象、构建器和异常。Go 版本使用：

- <code>flowable.New</code> 构造引擎。
- 服务接口方法接收 <code>context.Context</code> 与结构体请求。
- 错误通过返回值返回，使用 <code>api.Error</code>。
- 没有 Spring/CDI、类加载、表达式实例化。

## 服务映射

| Java | Go | 说明 |
| --- | --- | --- |
| <code>RepositoryService</code> | <code>RepositoryService</code> | 部署、定义、资源、模型 |
| <code>RuntimeService</code> | <code>RuntimeService</code> | 实例、执行、变量、事件 |
| <code>TaskService</code> | <code>TaskService</code> | 任务、候选人、评论、附件 |
| <code>HistoryService</code> | <code>HistoryService</code> | 历史查询与删除 |
| <code>ManagementService</code> | <code>ManagementService</code> | 作业、表、批次、属性 |
| <code>IdentityService</code> | <code>IdentityService</code> | 用户、组、角色 |
| <code>FormService</code> | <code>FormService</code> | 表单 |
| <code>DynamicBpmnService</code> | 未实现 | 动态 BPMN |

## 常见方法对照

| Java | Go |
| --- | --- |
| <code>deploy().addInputStream(...).deploy()</code> | <code>Deploy(ctx, DeploymentRequest{Resources: ...})</code> |
| <code>startProcessInstanceByKey(key, vars)</code> | <code>StartProcessInstance(ctx, StartRequest{...})</code> |
| <code>createTaskQuery().taskAssignee(user).list()</code> | <code>ListTasks(ctx, TaskFilter{Assignee: user})</code> |
| <code>taskService.complete(taskId, vars)</code> | <code>Complete(ctx, taskID, CompleteTaskRequest{...})</code> |
| <code>historyService.createHistoricProcessInstanceQuery()</code> | <code>GetHistoricProcessInstance</code> / 列表方法 |
| <code>managementService.executeJob(jobId)</code> | <code>RunDueJobs</code> / 作业管理方法 |

## 委托改写

| Java | Go |
| --- | --- |
| <code>implements JavaDelegate</code> | 注册 <code>func(context.Context, *flowable.Execution) error</code> |
| <code>execution.getVariable("x")</code> | <code>execution.GetVariable("x")</code> |
| <code>execution.setVariable("x", v)</code> | <code>execution.SetVariable("x", v)</code> |
| Spring 注入 | 闭包捕获依赖 |
| 类名写在 BPMN | 注册名/表达式匹配 |

## 异常改写

| Java | Go |
| --- | --- |
| <code>FlowableObjectNotFoundException</code> | <code>api.ErrNotFound</code> + <code>api.Error</code> |
| <code>FlowableException</code> | <code>error</code> / <code>*api.Error</code> |
| <code>try/catch</code> | <code>if err != nil</code> + <code>errors.As/Is</code> |

## 迁移步骤

1. 盘点 Java 服务调用与委托，列出使用的方法。
2. 按上表替换为 Go 接口，先用内存引擎跑通单测。
3. 把 Java 委托改写为 Go 函数并补幂等/补偿。
4. 用状态文件或数据库模式验证重启恢复。
5. 对关键路径做 Java/Go 行为对照。
6. 最后再评估存量库迁移，不要反向顺序。

## 相关阅读

- [SDK 服务接口](/docs/sdk/services)
- [委托与监听器](/docs/sdk/delegates)
- [迁移总览](/docs/migration)
