---
title: 错误码
description: 错误体结构、HTTP 状态映射与 ENGINE/REST/AUTH 前缀的定位方法。
order: 20
section: reference
status: prototype
since: v0.1.0
tags: [参考, 错误, 错误码]
---

## 错误体

~~~json
{
  "code": "ENGINE-0404",
  "message": "流程实例不存在",
  "exception": "可选底层原因"
}
~~~

<code>code</code> 是稳定错误码，<code>message</code> 是中文说明，<code>exception</code> 只在有底层原因时出现。客户端应优先按 HTTP 状态与 <code>code</code> 分支，不要解析 <code>message</code>。

## HTTP 状态映射

| 状态 | 哨兵错误 | 典型场景 |
| --- | --- | --- |
| 400 | <code>ErrBadRequest</code> | 参数非法、状态不允许 |
| 401 | <code>ErrUnauthorized</code> | 未认证、Token 无效 |
| 403 | <code>ErrForbidden</code> | 无权限、跨租户 |
| 404 | <code>ErrNotFound</code> | 资源不存在 |
| 409 | <code>ErrConflict</code> | 并发/状态冲突 |
| 429 | <code>ErrTooManyRequests</code> | 限流 |
| 500 | 其他 | 内部错误 |

## 前缀

| 前缀 | 范围 |
| --- | --- |
| <code>ENGINE-</code> | 引擎、流程、任务、历史、作业 |
| <code>REST-</code> | HTTP 参数、路径与请求体 |
| <code>AUTH-</code> | 认证、密码、Token |
| <code>MIGRATE-</code> | 迁移工具（报告与日志） |

## 常见错误

| 错误码 | 含义 | 处理 |
| --- | --- | --- |
| <code>ENGINE-0404</code> | 资源不存在 | 检查 ID、租户与是否已删除 |
| <code>ENGINE-0510</code> | 服务任务委托未注册 | 重启后重新 <code>RegisterDelegate</code> |
| <code>ENGINE-0918</code> | 表不存在 | 检查 Schema 初始化 |
| <code>REST-0401</code> | 未认证 | 补 Authorization 或证书 |
| <code>REST-0403</code> | 无权限/跨租户 | 检查绑定租户与 RBAC |
| <code>REST-0405</code> | 排序参数非法 | 只指定一个方向 |
| <code>AUTH-0001</code> | 凭据无效 | 检查密码/Token |

> 错误码会随实现增加。完整列表以 <code>api/errors.go</code> 与 <code>internal/rest</code> 中的 <code>NewError</code> 为准。

## 迁移工具退出码

| 退出码 | 含义 |
| ---: | --- |
| 0 | 当前检查未发现阻断/恢复成功 |
| 2 | 预检存在阻断或停写检查发现变化 |
| 1 | 参数、连接、扫描、恢复或输出失败 |

## 相关阅读

- [REST 约定](/docs/rest/conventions)
- [排障手册](/docs/operations/troubleshooting)
- [SDK 错误处理](/docs/sdk/errors)
