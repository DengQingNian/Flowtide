---
title: 10 分钟跑通第一个流程（REST）
description: 从部署 BPMN、启动实例、查询并完成用户任务到读取历史，全部命令已在当前提交验证。
order: 20
section: getting-started
status: verified
since: v0.1.0
verifiedCommit: 2925fc7
tags: [REST, 快速开始, 用户任务]
---

## 你将完成什么

部署一个「开始 → 用户任务 → 结束」的流程，启动实例，以 <code>alice</code> 身份查询并完成待办，最后确认实例已结束。所有命令在 2026-10-29 的提交 <code>2925fc7</code> 上实际执行通过。

## 前置条件

- 已按 [安装](/docs/getting-started/installation) 构建 <code>bin/flowtide-server</code>。
- 8080 端口空闲。本文命令假设服务监听 <code>localhost:8080</code>。
- 已安装 <code>curl</code> 与 <code>jq</code>（可选，便于提取字段）。

## 步骤 1：启动服务

~~~bash
mkdir -p ./data
./bin/flowtide-server -addr :8080 -state ./data/engine.json -jobs-interval 0
~~~

另开终端验证：

~~~bash
curl -sS http://localhost:8080/healthz
# {"status":"UP"}
~~~

## 步骤 2：准备流程文件

保存为 <code>hello.bpmn20.xml</code>：

~~~xml title="hello.bpmn20.xml"
<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="helloWorld" name="Hello World" isExecutable="true">
    <startEvent id="start" name="Start"/>
    <sequenceFlow id="flow1" sourceRef="start" targetRef="usertask"/>
    <userTask id="usertask" name="Hello" flowable:assignee="${initiator}"/>
    <sequenceFlow id="flow2" sourceRef="usertask" targetRef="end"/>
    <endEvent id="end" name="End"/>
  </process>
</definitions>
~~~

## 步骤 3：部署

~~~bash
curl -sS -X POST 'http://localhost:8080/service/repository/deployments?deploymentName=hello' \
  -H 'Content-Type: application/xml' \
  --data-binary @hello.bpmn20.xml
~~~

预期返回 <code>201</code>，响应包含部署 ID：

~~~json
{
  "id": "01a1835c-43b0-714a-8157-2257a711c04e",
  "name": "hello",
  "tenantId": "",
  "deployTime": "2026-10-29T00:33:44.368908479+08:00"
}
~~~

::: info 为什么路径有 /service
所有业务流程端点都以 <code>/service</code> 为前缀。健康检查、指标与就绪探针是例外，位于根路径。
:::

## 步骤 4：启动实例

~~~bash
curl -sS -X POST 'http://localhost:8080/service/runtime/process-instances' \
  -H 'Content-Type: application/json' \
  -d '{
    "processDefinitionKey": "helloWorld",
    "variables": [{"name": "initiator", "value": "alice"}]
  }'
~~~

预期 <code>201</code>，关键字段：

~~~json
{
  "id": "01a1835c-43b9-7376-83af-8ba0a8b63d5a",
  "processDefinitionId": "helloWorld:1:01a1835c-43b1-7257-821e-bb0d345d1d9a",
  "processDefinitionKey": "helloWorld",
  "ended": false,
  "suspended": false,
  "variables": {"initiator": "alice"}
}
~~~

记录响应中的 <code>id</code> 作为 <code>&lt;processInstanceId&gt;</code>。

## 步骤 5：查询待办

~~~bash
curl -sS 'http://localhost:8080/service/runtime/tasks?assignee=alice'
~~~

预期 <code>data</code> 中恰好一条任务：

~~~json
{
  "data": [
    {
      "id": "01a1835c-43b9-78f7-886d-6ba8e0524243",
      "name": "Hello",
      "processInstanceId": "01a1835c-43b9-7376-83af-8ba0a8b63d5a",
      "taskDefinitionKey": "usertask",
      "assignee": "alice",
      "priority": 50,
      "variables": {"initiator": "alice"}
    }
  ],
  "total": 1,
  "start": 0,
  "size": 10
}
~~~

记录 <code>id</code> 作为 <code>&lt;taskId&gt;</code>。

## 步骤 6：完成任务

~~~bash
curl -sS -i -X POST 'http://localhost:8080/service/runtime/tasks/&lt;taskId&gt;' \
  -H 'Content-Type: application/json' \
  -d '{
    "action": "complete",
    "variables": [{"name": "approved", "value": true}]
  }'
~~~

预期 <code>204 No Content</code>。使用 <code>POST</code> 加 <code>action</code> 的形式，与 Flowable 客户端习惯一致。

## 步骤 7：读取历史

~~~bash
curl -sS 'http://localhost:8080/service/history/historic-process-instances/&lt;processInstanceId&gt;'
~~~

预期：

~~~json
{
  "id": "01a1835c-43b9-7376-83af-8ba0a8b63d5a",
  "processDefinitionKey": "helloWorld",
  "startTime": "2026-10-29T00:33:44.377685693+08:00",
  "endTime": "2026-10-29T00:33:44.490389055+08:00",
  "durationMillis": 112,
  "state": "completed",
  "ended": true
}
~~~

## 验证清单

- [ ] 部署返回 <code>201</code>，部署 ID 非空。
- [ ] 实例 <code>ended=false</code>，变量 <code>initiator=alice</code>。
- [ ] 任务查询返回 1 条，<code>assignee=alice</code>。
- [ ] 完成请求返回 <code>204</code>。
- [ ] 历史实例 <code>ended=true</code>、<code>state=completed</code>。

## 排错

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| 404 <code>/runtime/...</code> | 漏掉 <code>/service</code> 前缀 | 使用 <code>/service/runtime/...</code> |
| 启动报流程定义不存在 | Key 不一致或部署失败 | 查询 <code>/service/repository/process-definitions</code> |
| 任务列表为空 | assignee 与变量不一致 | 检查 <code>flowable:assignee</code> 与启动变量 |
| 完成报 400 | 未传 <code>action</code> 或任务已结束 | 先查任务详情，再按 action 调用 |
| 服务任务报 <code>ENGINE-0510</code> | 委托未注册 | 嵌入模式下先 <code>RegisterDelegate</code> |

## 清理

停止服务后删除 <code>./data/engine.json</code> 即可重置。状态文件包含业务数据，不要提交。

## 下一步

- [30 分钟嵌入 Go 应用](/docs/getting-started/quickstart-go)
- [REST 约定](/docs/rest/conventions)
- [用户任务与身份](/docs/compatibility/bpmn)
