---
title: 版本与变更
description: 当前源码版本、最近提交、能力增量与“没有正式发布”的说明。
route: /releases
order: 10
section: releases
status: prototype
since: v0.1.0
tags: [发布, 变更, 版本]
---

## 当前版本

| 项 | 值 |
| --- | --- |
| 源码版本常量 | <code>0.1.0</code> |
| 分支 | <code>master</code> |
| 提交 | <code>2925fc7</code> |
| 日期 | 2026-10-29 |
| 正式 tag | 无 |
| API 稳定性 | 未承诺 semver |

当前没有发布 tag、变更日志文件或制品仓库。版本常量只用于引擎信息展示，不代表已发布版本。

## 最近能力增量

| 提交 | 内容 |
| --- | --- |
| <code>2925fc7</code> | 文档站规划与内容清单 |
| <code>a8866c0</code> | 任务查询下推与状态恢复校验 |
| <code>515c79f</code> | MySQL 行仓储服务模式、迁移与安全工具 |
| <code>66e97ce</code> | 官方 REST 端点、DMN 命中策略、mTLS |
| <code>ba0c6d0</code> | Java 存量离线转换与用户任务对照 |
| <code>b4d66b4</code> | 既有 Java 库只读迁移预检 |
| <code>5bd33c4</code> | 兼容基线质量门禁与迁移退出条件 |

完整提交历史见仓库 Git 日志。本站不自动生成 changelog；发布流程建立后再生成。

## 升级注意

- 状态文件格式有版本号，但跨版本迁移未承诺。
- REST 路由与 Go API 均可能调整。
- 数据库 Schema 跟随 Flowable 8.1.0.3；升级前备份。
- 没有旧版本文档快照；当前只有 <code>next</code> 内容。

## 发布前检查

- [ ] <code>make check</code> 与 <code>make race</code> 通过。
- [ ] 双库测试通过并保存日志。
- [ ] 文档事实与链接校验通过。
- [ ] 版本号、tag、变更日志更新。
- [ ] 升级/回退说明完成。
- [ ] 已知问题与限制更新。

## 相关阅读

- [路线图](/docs/releases/roadmap)
- [质量门禁](/docs/progress/quality)
- [差距与下一步](/docs/progress/gaps)
