# Flowable Engine Go

用**纯 Go** 重构 [Flowable](https://www.flowable.org/) 工作流引擎。目标是提供一个不依赖 JVM、不依赖 CGO、可直接读写 Flowable 8.x 数据库、并对流程 REST API 保持兼容的 BPMN 2.0 引擎。

> 当前状态：**BPMN 内核扩展实现阶段**。已完成纯 Go 的 BPMN 解析、部署、执行树、任务、网关、子流程、多实例、调用活动、消息/信号、定时器、错误、补偿、事件子流程、迁移、身份、历史、JSON 状态持久化与 REST 核心端点。  
> 参考基线：Flowable 8.1.0-SNAPSHOT（Apache-2.0），位于 `/root/src/public/flowable-engine`。

## 为什么做

- **部署更轻**：单二进制、无 JVM、无 CGO，适合容器与边缘环境。
- **资源更省**：Go 的运行时与内存占用显著低于 JVM。
- **可嵌入**：既能作为独立服务，也能作为 Go library 集成进业务系统。
- **数据兼容**：复用 Flowable 的 `ACT_*` Schema，存量数据、报表与运维脚本可继续使用。
- **接口兼容**：流程 REST 以官方 OpenAPI 为契约，降低客户端改造成本。

## 目标与边界

| 层级 | 范围 | 状态 |
| --- | --- | --- |
| T0 内核 | BPMN 模型/解析、PVM、命令、持久化、变量、任务、历史、作业、身份、流程 REST | 计划中 |
| T1 BPMN 增强 | 事件注册、外部工作项、迁移、批处理、DMN 调用、HTTP/Mail | 计划中 |
| T2 多引擎 | CMMN、DMN、Form、App、Content | 计划中 |
| T3 生态 | LDAP、Kafka/Rabbit、多数据库、Helm | 计划中 |

明确不做：Java 类级二方库兼容、Spring/CDI/OSGi/JMX、Flowable 5 兼容层、Designer/Modeler 前端。详见 [001 总览与范围](docs/001-flowable-go-重构计划/001-项目总览与范围界定.md)。

## 开发计划

全部文档位于 [docs/001-flowable-go-重构计划](docs/001-flowable-go-重构计划/)，按以下顺序阅读：

| 编号 | 文档 | 内容 |
| --- | --- | --- |
| 001 | [项目总览与范围界定](docs/001-flowable-go-重构计划/001-项目总览与范围界定.md) | 目标、范围、指标、决策摘要 |
| 002 | [总体架构与核心运行时](docs/001-flowable-go-重构计划/002-总体架构与核心运行时.md) | 分层、Command、Agenda、事务、并发 |
| 003 | [代码结构、模块映射与工程规范](docs/001-flowable-go-重构计划/003-代码结构、模块映射与工程规范.md) | 目录、模块映射、API 风格、规范 |
| 004 | [数据模型与持久化设计](docs/001-flowable-go-重构计划/004-数据模型与持久化设计.md) | 表结构、实体、UoW、方言、迁移 |
| 005 | [BPMN 模型、XML 解析与部署校验](docs/001-flowable-go-重构计划/005-BPMN模型、XML解析与部署校验.md) | 模型、解析、扩展元素、校验、部署 |
| 006 | [流程执行引擎 PVM 设计](docs/001-flowable-go-重构计划/006-流程执行引擎PVM设计.md) | 执行树、操作、行为、多实例、事件 |
| 007 | [变量、表达式与脚本引擎](docs/001-flowable-go-重构计划/007-变量、表达式与脚本引擎.md) | 变量作用域、类型、UEL、脚本 |
| 008 | [任务、身份与表单](docs/001-flowable-go-重构计划/008-任务、身份与表单.md) | 用户任务、查询、IDM、表单 |
| 009 | [作业、定时器与异步执行](docs/001-flowable-go-重构计划/009-作业、定时器与异步执行.md) | 作业状态机、执行器、定时器、集群 |
| 010 | [历史、事件与审计](docs/001-flowable-go-重构计划/010-历史、事件与审计.md) | 历史级别、写入点、事件总线、审计 |
| 011 | [REST API 与兼容契约](docs/001-flowable-go-重构计划/011-REST-API与兼容契约.md) | 端点清单、DTO、错误码、契约测试 |
| 012 | [测试策略与质量门禁](docs/001-flowable-go-重构计划/012-测试策略与质量门禁.md) | 测试金字塔、覆盖率、CI、基准 |
| 013 | [里程碑、排期与资源计划](docs/001-flowable-go-重构计划/013-里程碑、排期与资源计划.md) | 阶段、排期、人力、退出标准 |
| 014 | [架构决策、风险与验收标准](docs/001-flowable-go-重构计划/014-架构决策、风险与验收标准.md) | ADR、风险登记、验收清单 |

## 关键技术决策

| 领域 | 决策 |
| --- | --- |
| 数据库 | 复用 Flowable `ACT_*` Schema；首期 PostgreSQL 16 + MySQL 8.0 |
| 持久化 | `database/sql` + 显式仓储 + 工作单元，不用 ORM |
| 执行模型 | 移植 Command + Agenda + 执行树（PVM） |
| 表达式 | 自研 UEL 兼容子集，白名单方法调用 |
| 脚本 | goja（JavaScript）、Starlark；不支持 Groovy |
| 并发 | 一命令一事务一 goroutine；作业多 worker |
| REST | 官方 OpenAPI 为契约，chi + 手写 Handler + 契约测试 |
| 质量 | TDD，行覆盖率 ≥ 0.90，race/lint/vuln 全绿 |

## 质量承诺

- 每个函数具备中文注释与 `@param/@return`。
- 圈复杂度 ≥ 3 的分支加行注释。
- 测试驱动开发，行覆盖率 ≥ 0.90。
- 所有文档、注释使用中文。
- 阶段性成果按约定格式提交（`feat|fix|perf|chore(模块): 描述`）。

## 目录约定

```text
docs/{三位流水号}-{描述}/
  {三位流水号}-{描述}.md
```

新增文档必须遵守命名与编号规则；范围或架构变更需同步更新 001 与 014。

## 许可证

本项目遵循 Apache License 2.0。参考 Flowable 的代码、测试语料与文档均保留其原始版权与许可声明。

## 快速开始

```bash
# 构建（无 CGO、无第三方依赖）
make build

# 运行
./bin/flowable-server -addr :8080 -history audit -state ./data/engine.json -jobs-interval 1s

# 健康检查
curl http://localhost:8080/healthz

# 部署 BPMN
curl -X POST 'http://localhost:8080/service/repository/deployments?deploymentName=demo' \
  -H 'Content-Type: application/xml' \
  --data-binary @process.bpmn20.xml

# 启动实例
curl -X POST 'http://localhost:8080/service/runtime/process-instances' \
  -H 'Content-Type: application/json' \
  -d '{"processDefinitionKey":"demo","variables":[{"name":"initiator","value":"alice"}]}'
```

当前实现范围、限制与覆盖率见 [MVP 实现说明](docs/002-迁移实施/001-MVP实现说明.md)；整体进度与剩余差距见 [实施进度与差距](docs/002-迁移实施/002-实施进度与差距.md)。

## 已实现能力

- BPMN XML 安全解析：命名空间、DTD 拒绝、深度/大小限制。
- 流程部署、版本管理、按 Key/ID 启动。
- 执行树、Agenda 操作循环、命令级回滚。
- 用户任务：创建、签收、完成、委派、候选人、查询。
- 服务任务：Go 委托注册表与 delegateExpression。
- 排他网关（条件/默认流）、并行网关（分叉/汇聚）、包容网关（多条件/动态汇聚）、事件网关（竞争）。
- 嵌入式子流程（含嵌套与内部并行汇聚）、多实例（并行/串行、集合/基数、完成条件）。
- 边界定时器（中断/非中断）、定时启动、中间定时捕获、定时器作业扫描与持久化。
- 调用活动（子流程启动、变量继承、in/out 映射、调用深度限制）。
- 消息/信号事件（消息启动、信号启动、中间捕获、消息点对点、信号广播）。
- 异步续跑（asyncBefore/asyncAfter 作业、持久化、后台扫描）。
- 作业重试与死信（独立事务、退避、死信恢复、管理 REST）。
- 重复定时器（`R3/PT1M`、无限重复、非中断边界重排）。
- 流程定义/实例挂起与激活（含实例联动、任务与作业限制）。
- 基础身份目录（用户、组、成员关系、候选组任务查询）。
- 错误事件（错误结束、错误边界、作用域传播、未捕获失败）。
- 边界消息/信号（中断/非中断、订阅生命周期与清理）。
- 流程实例迁移（活动映射、任务重映射、目标定义校验）。
- 执行树查询与历史实例详情。
- 任务评论与附件（查询、添加、删除、持久化）。
- 表单属性（启动/任务表单、必填/只读/类型校验、提交完成）。
- 发送/接收任务（消息点对点、信号广播、等待与触发）。
- 任务变量 API 与 REST（查询、创建、更新、删除）。
- 任务身份链接（候选用户/候选组增删查、REST）。
- 实例变量增删改（API 与 REST）。
- 事务子流程（取消结束、取消边界、作用域销毁）。
- Flowable 8.1 的 41 张 ACT_* 表 DDL（MySQL/PostgreSQL）与 `flowable-migrate` CLI。
- 可插拔状态存储（文件/内存/SQL），SQL 桥接写入 `ACT_GE_BYTEARRAY`。
- ACT_* 行映射、导出与导入（仓库、运行时、作业/订阅/死信、历史/评论/附件）。
- 行级 SQL 仓储、`REV_` 乐观锁与事务工作单元。
- 部署管理（列表、详情、资源下载、级联删除、ZIP/BAR 部署包）。
- 事件子流程（消息/信号启动、中断/非中断、作用域取消）。
- 补偿（补偿边界、关联处理器、逆序执行；服务/手动处理器）。
- 脚本任务（expression/juel/javascript 表达式子集、结果变量）。
- 变量作用域、表达式子集、历史记录（流程/活动/任务/变量）。
- JSON 状态文件持久化（原子写入、启动恢复、失败回滚）。
- REST：部署、流程定义、实例、任务、变量、历史、健康检查。
- 纯标准库实现，`CGO_ENABLED=0` 可构建。