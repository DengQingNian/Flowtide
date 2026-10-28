---
title: 质量门禁与测试进度
description: 覆盖率、测试规模、make 门禁、双库集成与仍需补齐的质量证据。
order: 60
section: progress
status: verified
since: v0.1.0
tags: [测试, 覆盖率, CI, 门禁]
---

## 当前质量快照

| 指标 | 数值 | 口径 |
| --- | ---: | --- |
| 语句覆盖率 | 90.86% | <code>coverage.out</code> 加权总量 |
| 已覆盖语句 | 15,904 | 共 17,504 |
| 测试/基准函数 | 965 | <code>*_test.go</code> 声明 |
| 公开 API 覆盖率 | 100% | <code>api</code> 包历史采样 |
| 双库验收 | MySQL 8.0.35 / PostgreSQL 16.3 | 临时库、显式启用 |
| 文档规模 | 84 篇 / 约 14,800 行 | <code>docs/</code> |

::: info 覆盖率口径
Go 原生工具衡量语句覆盖率，不是逐物理行覆盖率。90% 总量与逐核心库包阈值是当前门禁；95% 关键包与新增代码覆盖率是后续目标，尚未作为已实现能力宣称。
:::

## 门禁命令

| 命令 | 内容 |
| --- | --- |
| <code>make check</code> | 格式、中文函数注释、vet、普通测试、覆盖率、基线摘要、端点清单、独立 SDK、无 CGO 构建 |
| <code>make race</code> | 独立竞争检测；环境不可用必须失败 |
| <code>make test-mysql</code> | 读取 <code>.env</code>，创建并删除临时库 |
| <code>make test-postgres</code> | 通过 <code>FLOWABLE_POSTGRES_DSN</code> 注入连接 |
| <code>make bench</code> | 引擎与行仓储基准，不进入默认门禁 |
| <code>make contract</code> | 校验兼容基线摘要与方法级端点清单 |

~~~bash
HOME=/root make check
HOME=/root make race
HOME=/root make test-mysql
FLOWABLE_POSTGRES_DSN='postgres://...' HOME=/root make test-postgres
~~~

## 测试策略

- 测试驱动开发：新增行为先写失败测试，再实现。
- 表驱动覆盖正常、边界、错误与回滚路径。
- 真实数据库测试默认跳过，显式启用后任何连接或执行错误都必须失败。
- 独立 SDK 模块测试验证仓库外模块可构造引擎并调用服务。
- 迁移工具使用只读事务、临时库与确定性样本，避免污染开发库。
- 测试与基准函数数量仅作规模参考，不替代覆盖率与场景覆盖。

## 文档与代码一致性

- 函数注释要求中文，并包含 <code>@param/@return</code>。
- 圈复杂度 ≥ 3 的分支要求行注释。
- 兼容基线、DDL、OpenAPI 与 BPMN 语料由 <code>make contract</code> 校验摘要。
- 文档站构建前重新生成仓库事实、页面 HTML 与搜索索引，并校验站内链接与锚点。

## 仍需补齐的证据

1. Java 路径回放与逐字段请求/响应契约测试。
2. 真实数据库上的长稳、故障注入与多执行者仲裁。
3. 95% 关键包覆盖率与固定增量基准。
4. 前端可访问性、Lighthouse 与跨浏览器检查。
5. CI 中的真实数据库任务目前为显式启用，尚未成为每次合并的强制项。

## 相关阅读

- [迁移工具链进度](/docs/progress/migration)
- [差距与下一步](/docs/progress/gaps)
- [测试与本地调试](/docs/sdk/testing)
