---
title: 变量类型
description: 支持的变量类型、作用域、类型推断、拒绝规则与二进制上传方式。
order: 30
section: reference
status: verified
since: v0.1.0
tags: [参考, 变量, 类型]
---

## 支持的类型

| 类型 | JSON 表示 | 说明 |
| --- | --- | --- |
| <code>null</code> | <code>null</code> | 空值 |
| <code>string</code> | 字符串 | 普通文本 |
| <code>longString</code> | 字符串 | 长文本，存储优化 |
| <code>boolean</code> | true/false | 布尔 |
| <code>short</code> | 数字 | 16 位整数 |
| <code>integer</code> | 数字 | 32 位整数 |
| <code>long</code> | 数字 | 64 位整数 |
| <code>double</code> | 数字 | 浮点 |
| <code>date</code> | RFC 3339 字符串 | 时间 |
| <code>json</code> | 对象/数组 | 结构化数据 |
| <code>longJson</code> | 对象/数组 | 大 JSON |
| <code>bytes</code> | Base64 或 multipart | 二进制 |

## 拒绝的类型

- <code>serializable</code>：Java 序列化格式，Go 无法安全读取。
- 未知类型名。
- 缺失外置字节数据的二进制变量。
- 超出有符号 64 位范围的整数。

## 作用域

| 作用域 | 写入方式 | 可见范围 | 持久化 |
| --- | --- | --- | --- |
| 流程实例 | <code>SetVariable</code> | 整棵执行树 | 是 |
| 执行 | <code>SetVariableLocal</code> | 当前执行及子执行 | 是 |
| 任务局部 | 完成时 LocalVariables | 当前任务 | 是 |
| 瞬态 | 请求中的 transientVariables | 当前命令 | 否 |

变量查找沿执行树向上，直到找到同名变量。删除变量只影响指定作用域。

## 类型推断

REST 请求可以省略 <code>type</code>，由引擎根据 JSON 值推断。为避免整数/浮点歧义，建议显式指定类型。迁移或跨语言场景应使用明确类型。

## 二进制变量

REST 使用 multipart 上传：

~~~bash
curl -sS -X POST 'http://localhost:8080/service/runtime/process-instances/<id>/variables' \
  -F 'name=document' \
  -F 'type=bytes' \
  -F 'file=@report.pdf'
~~~

原始字节通过 <code>.../variables/{name}/data</code> 下载。历史变量明细与原值下载遵循相同模式。

## 与 Java 的差异

Java <code>serializable</code> 变量必须迁移为 JSON 或 bytes，否则预检阻断。日期/JSON 的精度与格式需要在客户端侧明确约定。

## 相关阅读

- [核心概念](/docs/getting-started/concepts)
- [迁移限制](/docs/migration/limits)
- [REST 约定](/docs/rest/conventions)
