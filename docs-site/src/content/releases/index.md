---
title: 版本与变更
description: 当前源码版本、模块 tag、最近能力增量与发布前检查清单。
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
| 源码版本常量 | <code>0.1.0</code>（仅用于引擎信息展示） |
| 模块 tag | <code>v0.1.1</code>（2026-11-01） |
| 分支 | <code>master</code> |
| 提交 | 见页脚“生成于”提交 |
| 正式制品仓库 | 无 |
| API 稳定性 | 未承诺 semver |

源码版本常量没有随模块 tag 同步递增，判断实际能力请看模块 tag 与提交历史。本站不自动生成 changelog。

## v0.1.1

- 根包提供完整嵌入 SDK：<code>NewEmbedded</code>、<code>EmbeddedConfig</code>、<code>EnsureMySQLSchema</code>、<code>WithExecutionContext</code>，复用宿主连接池并持有单写者锁。
- workflow 业务门面与 <code>/workflow/**</code> 路由接入同一服务；管理台新增工作流九页。
- 新增严格门面装配与可取消初始化：装配失败清理资源，关闭保留首次错误且不关闭宿主连接池。
- 修复八个含顿号的文档文件名，标准 Go 模块打包与校验恢复。

已知限制：workflow 调度器既有用例 <code>TestSchedulerTriggerFunctions</code> 因固定假时钟与真实计时混用未通过，本版未修改调度器实现，因此不宣称全仓测试通过。

## 最近能力增量

| 提交 | 内容 |
| --- | --- |
| <code>8a0f2e4</code> | 宿主嵌入 SDK、MySQL 结构检查入口与外部模块验收 |
| <code>6e1f3c8</code> | 服务端注入 workflow 门面与调度器，内嵌新版管理台 |
| <code>3c8aa6f</code> | 管理台工作流九页 |
| <code>548c4c7</code> | 第三批设计器链路端点接线（84/95） |
| <code>723a2a4</code> | v0.1.1 发布与模块文档文件名修复 |

完整提交历史见仓库 Git 日志。REST 与方法级端点的当前口径见 [REST 覆盖与差异](/docs/rest/coverage) 与 [能力矩阵](/compatibility)。

## 升级注意

- 状态文件格式有版本号，但跨版本迁移未承诺；升级前备份。
- 快照行与行仓储控制行前缀为 <code>flowtide-*</code>，旧原型数据不会被自动识别。
- REST 路由与 Go API 均可能调整；模块 tag 不等于 API 冻结。
- 数据库 Schema 跟随 Flowable 8.1.0.3；升级前备份。

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
- [宿主嵌入 SDK](/docs/sdk/embedding)
