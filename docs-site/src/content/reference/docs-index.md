---
title: 仓库文档索引
description: 设计、实施、审计与文档站四组中文文档的编号规则、阅读顺序与事实源。
order: 50
section: reference
status: verified
since: v0.1.0
tags: [参考, 文档, 索引]
---

## 文档分组

| 分组 | 内容 | 入口 |
| --- | --- | --- |
| <code>docs/001-flowtide-重构计划</code> | 目标、架构、数据模型、PVM、任务、变量、作业、历史、REST、质量、里程碑、ADR | 001 项目总览 |
| <code>docs/002-迁移实施</code> | 分阶段实施记录、MySQL、预检、转换、mTLS、容量、查询下推 | 001 MVP 实现说明 |
| <code>docs/003-迁移审计</code> | 审计证据、整改记录、能力矩阵、REST 对照 | 003 能力矩阵 |
| <code>docs/004-文档站</code> | 站点规划、内容清单、写作规范 | 001 文档站规划 |

## 编号规则

~~~text
docs/{三位流水号}-{描述}/
  {三位流水号}-{描述}.md
~~~

示例：<code>docs/002-迁移实施/058-MySQL行仓储服务模式.md</code>。

## 阅读顺序

1. **判断能否用**：[能力矩阵](/compatibility) → [已知问题](/docs/compatibility/known-issues)。
2. **跑通第一个流程**：[安装](/docs/getting-started/installation) → [REST 快速开始](/docs/getting-started/quickstart-rest)。
3. **理解实现**：[架构总览](/docs/architecture/overview) → 仓库 <code>docs/001</code>。
4. **做迁移评估**：[迁移总览](/docs/migration) → [预检](/docs/migration/preflight) → [限制](/docs/migration/limits)。
5. **查实现证据**：<code>docs/002</code> 与 <code>docs/003</code>，以及每个能力的提交与测试。

## 事实源优先级

| 问题 | 权威来源 |
| --- | --- |
| 能力状态与发布边界 | <code>docs/003-迁移审计/003-能力矩阵与迁移退出条件.md</code> |
| REST 方法清单 | <code>docs/003-迁移审计/004-REST端点对照.md</code> 与 <code>testdata/compatibility/baseline.json</code> |
| 配置与 CLI | <code>cmd/flowtide-server/main.go</code>、<code>internal/engine/engine.go</code> |
| 错误码 | <code>api/errors.go</code> 与 <code>internal/rest</code> |
| 质量门禁 | <code>Makefile</code> 与 <code>.github/</code> |
| 站点内容 | <code>docs-site/src/content</code> |

## 站点如何生成

本站构建前运行两个脚本：

1. <code>gen-facts.mjs</code>：统计提交、代码、覆盖率、端点、能力矩阵与最近提交。
2. <code>gen-content.mjs</code>：解析 frontmatter，渲染 Markdown、代码高亮、目录与搜索索引，并校验站内链接。

生成物位于 <code>docs-site/src/generated</code>，不提交版本库。

## 相关阅读

- [文档站规划与内容清单](https://github.com/DengQingNian/Flowtide)
- [质量门禁](/docs/progress/quality)
- [术语表](/docs/reference/glossary)
