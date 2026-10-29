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
| 全仓语句覆盖率 | 90.81% | <code>coverage.out</code> 加权总量 |
| 已覆盖语句 | 17,081 | 共 18,809，包含命令及工具入口 |
| 核心库覆盖率门禁 | 91.88% | 16,659 / 18,131；各核心包均 ≥ 90% |
| 测试/基准函数 | 1024 | <code>*_test.go</code> 声明 |
| 公开 API 覆盖率 | 100% | <code>api</code> 包历史采样 |
| 双库验收 | MySQL 8.0.35 / PostgreSQL 16.3 | 临时库、显式启用 |
| 文档规模 | 93 篇 / 15,439 行 | <code>docs/</code> |

::: info 覆盖率口径
Go 原生工具衡量语句覆盖率，不是逐物理行覆盖率。90% 总量与逐核心库包阈值是当前门禁；95% 关键包与新增代码覆盖率是后续目标，尚未作为已实现能力宣称。
:::

## M5 验收结果

六级历史写入矩阵、full 变量更新明细、历史单独恢复查询及租户隔离均已通过回归。audit/activity 仅保存变量最终值，逐次明细需显式设置 full。HTTP 审计按可信认证主体和有效租户记录，失败凭据不能冒充操作人；日志仍为内存环形存储。

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

1. 全量 Java REST 原始请求/响应逐字段契约；用户任务 Java 黄金样本的 Go HTTP 业务对照已通过。
2. 真实数据库上的长稳、故障注入与多执行者仲裁。
3. 95% 关键包覆盖率与固定增量基准。
4. 前端可访问性、Lighthouse 与跨浏览器检查。
5. 真实数据库测试由 CI databases 任务在 push/pull_request 执行；分支保护是否要求该检查需由仓库管理员配置。

## 相关阅读

- [迁移工具链进度](/docs/progress/migration)
- [差距与下一步](/docs/progress/gaps)
- [测试与本地调试](/docs/sdk/testing)
