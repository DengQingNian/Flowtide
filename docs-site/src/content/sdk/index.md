---
title: Go SDK 概览
description: 通过 flowable.New 嵌入引擎，理解公共包、服务接口与和 REST 的关系。
order: 10
section: sdk
status: prototype
since: v0.1.0
tags: [SDK, Go, 服务接口]
---

## 定位

引擎既可以作为独立 REST 服务运行，也可以作为 Go 库嵌入业务进程。SDK 与 REST 共用同一个 <code>internal/engine</code> 实现，因此语义一致；差异在传输、认证与生命周期管理。

| 使用方式 | 适用场景 | 入口 |
| --- | --- | --- |
| 独立服务 | 多语言客户端、集中运维 | <code>cmd/flowable-server</code> |
| Go 嵌入 | 单进程业务、测试、工具 | <code>flowable.New</code> + <code>api</code> |
| 迁移 CLI | 存量库预检与状态准备 | <code>cmd/flowable-migrate</code> |

## 公共包

| 包 | 内容 |
| --- | --- |
| <code>github.com/flowable-go/flowable</code> | <code>New</code>、<code>Config</code>、<code>Engine</code>、<code>Delegate</code>、<code>Execution</code> |
| <code>.../api</code> | 请求/响应类型与服务接口 |
| <code>.../bpmn/model</code> | BPMN 模型类型 |

<code>internal/</code> 不属于公共契约，即使通过类型别名出现在 <code>Config</code> 字段中也不应直接依赖。

## 最小程序

~~~go
package main

import (
	"context"
	"log"

	"github.com/flowable-go/flowable"
	"github.com/flowable-go/flowable/api"
)

func main() {
	ctx := context.Background()
	app, err := flowable.New(flowable.Config{StatePath: "./data/engine.json"})
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close(ctx)

	task, err := app.TaskService().CreateTask(ctx, api.CreateTaskRequest{Name: "独立任务"})
	if err != nil {
		log.Fatal(err)
	}
	log.Println("created", task.ID)
}
~~~

## 服务接口

| 接口 | 获取方式 | 主要职责 |
| --- | --- | --- |
| <code>RepositoryService</code> | <code>app.RepositoryService()</code> | 部署、定义、资源、模型、挂起 |
| <code>RuntimeService</code> | <code>app.RuntimeService()</code> | 实例、执行、变量、消息/信号、迁移 |
| <code>TaskService</code> | <code>app.TaskService()</code> | 任务、候选人、委派、评论、附件、监听器 |
| <code>HistoryService</code> | <code>app.HistoryService()</code> | 实例/活动/任务/变量历史 |
| <code>ManagementService</code> | <code>app.ManagementService()</code> | 作业、死信、表、批次、属性 |
| <code>IdentityService</code> | <code>app.IdentityService()</code> | 用户、组、角色、成员、密码 |
| <code>FormService</code> | <code>app.FormService()</code> | 启动/任务表单、提交 |
| <code>DMNService</code> | <code>app.DMNService()</code> | 决策/决策表元数据 |

接口方法与 Java Service 的对应关系见 [从 Java Service API 迁移](/docs/sdk/java-api-mapping)（规划页）。

## 生命周期

1. <code>flowable.New</code> 构造引擎；若配置了状态文件或行仓储，会在此恢复状态。
2. 注册委托、任务监听器、执行监听器。
3. 通过服务接口执行命令；每个命令串行提交。
4. <code>Close</code> 刷盘并释放资源；行仓储模式还会释放数据库锁。

## 与 REST 的关系

SDK 不经过 HTTP，因此不涉及 Basic/JWT/mTLS/租户头；租户通过请求结构体或 <code>tenant.With</code> 上下文传递。REST 的 DTO 字段与 Go 结构体不完全相同，不要假设 JSON 字段名一一对应。

## 相关阅读

- [引擎配置](/docs/sdk/config)
- [服务接口](/docs/sdk/services)
- [委托与监听器](/docs/sdk/delegates)
- [SDK 限制](/docs/sdk/limits)
