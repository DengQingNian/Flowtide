---
title: REST API 概览
description: Base path、242 个方法端点的分域规模、认证入口与交互式浏览器。
order: 10
section: rest
status: prototype
since: v0.1.0
tags: [REST, API, 端点]
---

## Base path 与内容类型

- 业务端点前缀：<code>/service</code>
- 健康与指标：<code>/healthz</code>、<code>/readyz</code>、<code>/metrics</code>
- JSON 请求：<code>Content-Type: application/json</code>
- 部署资源：<code>application/xml</code>、<code>application/zip</code> 或 multipart
- 日期：RFC 3339 字符串
- 删除/完成类操作：通常返回 <code>204 No Content</code>

~~~bash
curl -sS http://localhost:8080/service/management/engine
curl -sS http://localhost:8080/service/repository/process-definitions
~~~

## 分域规模

| 域 | 端点 | 官方 | 扩展 | 典型用途 |
| --- | ---: | ---: | ---: | --- |
| runtime | 96 | 71 | 25 | 实例、执行、任务、变量、消息 |
| management | 41 | 32 | 9 | 作业、表、批次、属性 |
| repository | 34 | 31 | 3 | 部署、定义、模型、资源 |
| identity | 30 | 19 | 11 | 用户、组、角色、成员 |
| history | 29 | 21 | 8 | 历史实例、活动、任务、变量 |
| query | 10 | 8 | 2 | POST 组合查询 |
| form | 2 | 2 | 0 | 启动/任务表单 |

总计 242 个方法端点：官方 184，扩展 58。完整清单可按方法、域与状态筛选：[REST 浏览器](/rest-explorer)。

## 最小调用链

~~~bash
# 部署
curl -sS -X POST 'http://localhost:8080/service/repository/deployments?deploymentName=demo' \
  -H 'Content-Type: application/xml' --data-binary @process.bpmn20.xml

# 启动
curl -sS -X POST 'http://localhost:8080/service/runtime/process-instances' \
  -H 'Content-Type: application/json' \
  -d '{"processDefinitionKey":"demo"}'

# 查询任务
curl -sS 'http://localhost:8080/service/runtime/tasks?assignee=alice'
~~~

完整闭环见 [REST 快速开始](/docs/getting-started/quickstart-rest)。

## 兼容性口径

路由存在只代表“已编码”。每个端点还可能有四类差异：

1. 请求体字段名、必填性与类型；
2. 响应字段、空值与嵌套结构；
3. 分页、排序、过滤语义；
4. 错误码、状态码与副作用。

当前没有逐字段 Java 回放，因此不应把 184/184 的路由覆盖表述为“完全兼容”。详见 [REST 覆盖与差异](/docs/compatibility/rest)。

## 相关阅读

- [REST 约定](/docs/rest/conventions)
- [认证与租户](/docs/rest/auth)
- [分域端点](/docs/rest/domains)
