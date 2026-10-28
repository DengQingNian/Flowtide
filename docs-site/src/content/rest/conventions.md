---
title: REST 约定
description: 分页、过滤、日期、错误体、状态码、变量与租户头的统一规则。
order: 20
section: rest
status: prototype
since: v0.1.0
tags: [REST, 分页, 错误, 约定]
---

## 分页

列表端点接受 <code>start</code> 与 <code>size</code>，返回统一分页结构：

~~~json
{
  "data": [],
  "total": 0,
  "start": 0,
  "size": 10,
  "sort": "createTime",
  "order": "desc"
}
~~~

<code>sort</code>/<code>order</code> 只在支持的端点返回。<code>size</code> 为 0 时使用端点默认值；非法值按参数错误处理。不同端点的默认每页数量并不完全一致。

## 过滤

常见过滤参数：

| 参数 | 适用 | 说明 |
| --- | --- | --- |
| <code>assignee</code> / <code>owner</code> | 任务 | 处理人/所有者 |
| <code>processInstanceId</code> | 任务、变量、历史 | 实例范围 |
| <code>processDefinitionKey</code> | 任务、实例 | 定义 Key |
| <code>candidateUser</code> / <code>candidateGroup</code> | 任务 | 候选人/组 |
| <code>tenantId</code> | 多数列表 | 租户过滤 |
| <code>variables</code> | 组合查询 | 变量等值/比较/模糊 |
| <code>startedBefore</code> / <code>startedAfter</code> | 历史 | 时间范围 |

组合查询使用 <code>POST /service/query/...</code>，请求体包含过滤、排序与分页。

## 错误体

统一错误响应的核心字段：

~~~json
{
  "code": "ENGINE-0404",
  "message": "流程实例不存在",
  "exception": "可选底层原因"
}
~~~

- 404：资源不存在。
- 400：参数或状态非法。
- 401：未认证。
- 403：无权限或跨租户。
- 409：并发/状态冲突。
- 429：触发限流。
- 500：未预期错误。

错误码前缀包括 <code>ENGINE-</code>、<code>REST-</code>、<code>AUTH-</code> 等；它们尚未按 Java 错误码逐一映射。

## 状态码与响应

| 操作 | 常见响应 |
| --- | --- |
| 创建部署/实例/任务 | <code>201</code> + 资源 |
| 查询 | <code>200</code> + JSON |
| 完成/删除/动作 | <code>204</code> |
| 条件缓存命中 | <code>304</code> |
| 参数错误 | <code>400</code> + 错误体 |

## 日期与时区

时间字段使用 RFC 3339，服务端按本地时区输出（例如 <code>+08:00</code>）；迁移报告中的扫描时间使用 UTC <code>RFC3339Nano</code>。解析时不要假设固定时区，按带偏移的时间处理。

## 变量

变量 JSON 形状：

~~~json
{"name": "amount", "type": "integer", "value": 100, "scope": "global"}
~~~

类型可省略，由引擎推断。二进制变量使用 multipart 上传，读取原始字节走 <code>.../variables/{name}/data</code>。支持的类型与拒绝规则见 [变量类型](/docs/reference/variables)。

## 租户

租户优先级：认证绑定的租户 > <code>X-Tenant-Id</code> 头 > <code>tenantId</code> 查询参数。绑定租户时请求其他租户返回 403；未绑定且未指定时使用默认租户。

## 条件缓存

部署资源支持 <code>ETag</code>、<code>Last-Modified</code>、<code>If-None-Match</code> 与 <code>If-Modified-Since</code>。部署不可变，命中时返回 <code>304</code> 且无响应体。

## 相关阅读

- [认证与租户](/docs/rest/auth)
- [错误码](/docs/reference/errors)
- [REST 分域端点](/docs/rest/domains)
