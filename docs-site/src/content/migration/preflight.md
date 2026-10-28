---
title: 存量库只读预检
description: 用 -preflight 扫描 Java Flowable 库，理解报告字段、阻断规则、退出码与适用范围。
order: 20
section: migration
status: prototype
since: v0.1.0
tags: [迁移, 预检, 只读]
---

## 作用

<code>-preflight</code> 以只读事务扫描既有 Java 库，输出 Schema 版本、表行数、租户/实例/任务/变量统计、逐实体问题与限制。它不初始化、不修改源库，也不生成 Go 状态。

## 命令

~~~bash
# MySQL：连接配置沿用 .env
./bin/flowable-migrate -preflight -dialect mysql -env .env -out preflight.json

# 只检查默认租户（显式空值）
./bin/flowable-migrate -preflight -dialect mysql -tenant="" -out default-tenant.json

# 不传 -tenant 时分析全部租户
./bin/flowable-migrate -preflight -dialect mysql -out all-tenants.json

# PostgreSQL：从 FLOWABLE_POSTGRES_DSN 读取连接
./bin/flowable-migrate -preflight -dialect postgres -timeout 5m -out preflight-pg.json
~~~

## 退出码

| 退出码 | 含义 | 后续动作 |
| ---: | --- | --- |
| 0 | 当前分析范围未发现阻断 | 继续准备转换与 Java 对照，不能直接切换 |
| 2 | 报告包含阻断 | 按问题码、表、实体与实例归属整改 |
| 1 | 参数、连接、扫描或输出失败 | 修复后重跑，不用旧报告代替 |

使用编译后的二进制获取退出码；<code>go run</code> 会包装非零状态。

## 报告内容

- 兼容基线与 Schema 版本。
- 全库表行数与已知表清单。
- 分析范围、请求租户标识。
- 租户实例/执行/任务/变量统计。
- 逐实体问题：表、实体 ID、问题码、固定中文说明。
- 限制说明：未知表、非空未分析表、运行时变化等。

报告不包含变量值、脚本内容或 Java 类实现。

## 阻断规则

- Schema 版本不符合基线或缺少必需表。
- 变量为 <code>serializable</code>、未知类型、超范围整数或缺失外置字节。
- 执行树/任务引用不一致，定义 Key 与模型不匹配。
- 多实例、并发、调用活动与无法确认的复杂结构。
- Java 委托、监听器、脚本与复杂模型。
- Java 作业与订阅（尚无受验收转换器）。
- 非空未知表。

## 边界

- 全量内存扫描；未实现分页、流式分析、容量上限。
- 只支持无表名前缀的 MySQL/PostgreSQL <code>ACT_*</code> 布局。
- <code>-tenant</code> 是分析范围过滤，不是查询下推。
- 需要目标 Schema 整库读取与元数据权限。
- 扫描期间源库继续写入时，报告只代表事务快照。
- 不做全量模型语义分析、委托注册表核对、转换或回退。

## 相关阅读

- [离线转换](/docs/migration/convert)
- [停写切换](/docs/migration/cutover)
- [迁移限制与 FAQ](/docs/migration/limits)
