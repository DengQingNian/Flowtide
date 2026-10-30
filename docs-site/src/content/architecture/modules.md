---
title: 模块与代码结构
description: 仓库目录、包职责、公共 API 边界与新增代码的放置规则。
order: 40
section: architecture
status: verified
since: v0.1.0
tags: [模块, 代码结构, 规范]
---

## 顶层目录

| 目录 | 说明 |
| --- | --- |
| <code>api/</code> | 公共类型、服务接口、错误 |
| <code>bpmn/model/</code> | 对外 BPMN 模型类型 |
| <code>cmd/flowtide-server/</code> | REST 服务入口 |
| <code>cmd/flowtide-migrate/</code> | 迁移与 Schema CLI |
| <code>internal/engine/</code> | 运行时核心 |
| <code>internal/bpmn/</code> | XML 解析与图形 |
| <code>internal/expr/</code> | 表达式 |
| <code>internal/persistence/</code> | 状态、行仓储、SQL、Schema |
| <code>internal/rest/</code> | HTTP 层 |
| <code>internal/migration/</code> | 预检、转换、静默检查、校验 |
| <code>internal/auth/</code> | 认证与授权 |
| <code>internal/quality/</code> | 质量门禁 |
| <code>docs/</code> | 设计、实施、审计文档 |
| <code>docs-site/</code> | 本静态文档站 |
| <code>testdata/</code> | 兼容基线、SDK 验收、BPMN 语料 |
| <code>tools/</code> | 契约与质量工具 |

## 公共 API 边界

外部模块只应依赖：

- <code>github.com/DengQingNian/Flowtide</code>：<code>New</code>、<code>Config</code>、<code>Engine</code>、<code>Delegate</code>、<code>Execution</code>。
- <code>github.com/DengQingNian/Flowtide/api</code>：请求/响应类型与服务接口。
- <code>github.com/DengQingNian/Flowtide/bpmn/model</code>：模型类型（只读场景）。

<code>internal/</code> 不构成公共契约。REST DTO 也不承诺与 Go 结构体字段一一对应。

## 新增代码的放置

| 变更类型 | 放置位置 | 要求 |
| --- | --- | --- |
| 新 BPMN 元素 | <code>internal/bpmn/xml</code> + <code>internal/engine</code> | 解析、校验、执行、历史与测试 |
| 新服务方法 | <code>api</code> 接口 + <code>internal/engine</code> 实现 | 参数/返回注释、REST 映射、SDK 验收 |
| 新 REST 端点 | <code>internal/rest</code> + 契约清单 | 方法级对照、错误码、租户、测试 |
| 新存储形态 | <code>internal/persistence</code> | 恢复、锁、事务、失败路径 |
| 新迁移能力 | <code>internal/migration</code> + CLI | 只读优先、退出码、报告、边界 |
| 新文档 | <code>docs/{三位流水号}-{描述}/</code> | 中文、编号、关联更新 |

## 工程规范

- 每个函数有中文注释，包含参数与返回值说明。
- 圈复杂度 ≥ 3 的分支加行注释。
- 测试驱动开发，语句覆盖率 ≥ 90%。
- 阶段性成果按 <code>feat|fix|perf|chore(模块): 描述</code> 提交。
- 文档与注释使用中文；术语保留英文原名。

## 依赖与构建

- 运行时依赖仅数据库驱动、配置加载与测试替身，版本以 <code>go.mod</code> 为准。
- <code>CGO_ENABLED=0</code> 可构建。
- 独立 SDK 验收位于 <code>testdata/sdk</code>，通过 <code>replace</code> 指向仓库根。
- 文档站是独立 npm 工程，不进入 Go 模块依赖。

## 相关阅读

- [架构总览](/docs/architecture/overview)
- [质量门禁](/docs/progress/quality)
- [文档索引](/docs/reference/docs-index)
