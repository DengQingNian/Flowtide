---
title: 30 分钟嵌入 Go 应用
description: 在独立 Go 模块中创建引擎，注册委托，部署流程并查询历史。
order: 30
section: getting-started
status: verified
since: v0.1.0
verifiedCommit: 2925fc7
tags: [SDK, Go, 快速开始]
---

## 目标

这个示例在独立 Go 模块中嵌入引擎。运行后会注册服务任务委托、部署 BPMN、启动实例、完成任务，再读取历史。示例已在本机编译运行，输出 <code>started: ...</code> 与 <code>ended: true state: completed</code>。

## 步骤 1：创建模块

~~~bash
mkdir flowtide-demo && cd flowtide-demo
go mod init flowtide-demo
~~~

仓库内开发时，使用 <code>replace</code> 指向本地仓库：

~~~text title="go.mod"
module flowtide-demo

go 1.27

require github.com/DengQingNian/Flowtide v0.0.0

replace github.com/DengQingNian/Flowtide => /path/to/Flowtide
~~~

模块已发布 <code>v0.1.1</code> 供依赖引用；API 仍未承诺 semver，升级时需重新编译验证。

## 步骤 2：准备 BPMN

保存为 <code>process.bpmn20.xml</code>：

~~~xml title="process.bpmn20.xml"
<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="onboarding" name="Onboarding" isExecutable="true">
    <startEvent id="start" name="Start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="notify"/>
    <serviceTask id="notify" name="Notify" flowable:delegateExpression="${notify}"/>
    <sequenceFlow id="f2" sourceRef="notify" targetRef="approve"/>
    <userTask id="approve" name="Approve" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f3" sourceRef="approve" targetRef="end"/>
    <endEvent id="end" name="End"/>
  </process>
</definitions>
~~~

## 步骤 3：编写程序

~~~go title="main.go"
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/DengQingNian/Flowtide"
	"github.com/DengQingNian/Flowtide/api"
)

func main() {
	ctx := context.Background()
	app, err := flowtide.New(flowtide.Config{StatePath: "./data/engine.json"})
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close(ctx)

	app.RegisterDelegate("notify", func(ctx context.Context, execution *flowtide.Execution) error {
		execution.SetVariable("preparedBy", "system")
		return nil
	})

	xml, err := os.ReadFile("process.bpmn20.xml")
	if err != nil {
		log.Fatal(err)
	}
	if _, err := app.RepositoryService().Deploy(ctx, api.DeploymentRequest{
		Name:      "onboarding",
		Resources: map[string][]byte{"process.bpmn20.xml": xml},
	}); err != nil {
		log.Fatal(err)
	}

	pi, err := app.RuntimeService().StartProcessInstance(ctx, api.StartRequest{
		ProcessDefinitionKey: "onboarding",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("started:", pi.ID)

	page, err := app.TaskService().ListTasks(ctx, api.TaskFilter{Assignee: "alice"})
	if err != nil {
		log.Fatal(err)
	}
	if len(page.Data) != 1 {
		log.Fatalf("期望 1 条任务，实际 %d", len(page.Data))
	}
	if err := app.TaskService().Complete(ctx, page.Data[0].ID, api.CompleteTaskRequest{
		UserID:    "alice",
		Variables: []api.Variable{{Name: "approved", Value: true}},
	}); err != nil {
		log.Fatal(err)
	}

	hist, err := app.HistoryService().GetHistoricProcessInstance(ctx, pi.ID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("ended:", hist.Ended, "state:", hist.State)
}
~~~

## 步骤 4：运行

~~~bash
mkdir -p data
HOME=/root go mod tidy
HOME=/root go run .
~~~

预期输出：

~~~text
started: 01a1835d-9a30-7302-8302-6a277bcdd26d
ended: true state: completed
~~~

## 关键点

1. **委托是进程内注册的**：引擎重启后必须重新调用 <code>RegisterDelegate</code>，否则服务任务会报 <code>ENGINE-0510</code>。
2. **状态文件可恢复**：<code>StatePath</code> 保存定义、实例、任务与历史；重开引擎后可继续查询与操作。
3. **委托内可读写变量**：<code>execution.SetVariable</code> 写入流程作用域，<code>SetVariableLocal</code> 只写当前作用域。
4. **关闭时刷盘**：<code>app.Close(ctx)</code> 负责最终持久化，不要省略。
5. **需要数据库时改用嵌入入口**：<code>Config</code> 仍引用内部包类型，生产接入用 <code>NewEmbedded</code>（MySQL 行仓储），见 [30 分钟接入宿主应用](/docs/getting-started/quickstart-embed)。

## 常见问题

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| <code>cannot find module</code> | 未配置 <code>replace</code> 或网络不可用 | 指向本地仓库路径 |
| 服务任务等待不推进 | 委托名与 <code>delegateExpression</code> 不一致 | 名称需精确匹配 |
| <code>state conflict</code> | 同一状态文件被两个引擎同时打开 | 单进程独占 |
| 任务查询为空 | assignee 与变量不一致 | 检查 <code>flowable:assignee</code> |

## 下一步

- [30 分钟接入宿主应用](/docs/getting-started/quickstart-embed)
- [SDK 概览](/docs/sdk)
- [引擎配置](/docs/sdk/config)
- [委托与监听器](/docs/sdk/delegates)
