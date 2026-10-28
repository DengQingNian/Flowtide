---
title: SDK 限制
description: 版本稳定性、数据库配置、委托生命周期、脚本与集群能力的使用边界。
order: 60
section: sdk
status: prototype
since: v0.1.0
tags: [SDK, 限制, 版本]
---

## 必须在选型前知道的限制

1. **API 尚未承诺 semver 稳定性**：当前为原型阶段，接口与字段可能调整。
2. **仓库外无法配置数据库**：<code>Config</code> 引用内部包类型，外部模块只能用内存或 <code>StatePath</code>。
3. **委托/监听器不持久化**：重启后必须重新注册。
4. **单执行者**：没有集群租约、作业抢占与故障转移。
5. **脚本仅表达式子集**：不支持 Groovy、Starlark 与完整 JavaScript 语句。
6. **变量类型有限**：<code>serializable</code> 与未知类型被拒绝。
7. **异步历史、外部工作项、升级事件未实现**。
8. **REST 路由覆盖不等于行为兼容**：SDK 同样没有逐方法 Java 对照。

## 兼容性承诺

| 项 | 当前承诺 |
| --- | --- |
| Go 版本 | 1.27+ |
| 公共 API | 原型，可能破坏性调整 |
| 状态文件格式 | 有版本号，但跨版本迁移未承诺 |
| 数据库 Schema | 跟随 Flowable 8.1.0.3 |
| REST 契约 | 路由齐全，行为待验收 |
| 安全 | 有条件试用，权限审计未完成 |

## 何时不应使用

- 需要多节点高可用或自动故障转移。
- 必须运行现有 Java 委托/监听器而不改写。
- 依赖 Groovy/Starlark 或完整脚本引擎。
- 需要完整 CMMN/DMN/Form/App/Content 引擎。
- 要求生产级 SLA、容量承诺或长期兼容保证。
- 无法接受单执行者与人工回退。

## 依赖与构建

- 第三方依赖保持最小：数据库驱动、配置加载与测试替身。
- <code>CGO_ENABLED=0</code> 可构建，便于静态镜像与边缘部署。
- 独立模块验收在 <code>testdata/sdk</code>，每次 <code>make check</code> 执行。

## 相关阅读

- [已知问题](/docs/compatibility/known-issues)
- [差距与下一步](/docs/progress/gaps)
- [安装](/docs/getting-started/installation)
