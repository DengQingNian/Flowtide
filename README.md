# Flowtide

<p align="center">
  <img src="docs-site/public/tide-icon.png" width="160" height="160" alt="Flowtide 项目图标" />
</p>

> 用纯 Go 重构的 BPMN 2.0 工作流引擎，兼容 Flowable 8.x 数据模型与流程 REST 契约。

[![Go](https://img.shields.io/badge/Go-1.27%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![License](https://img.shields.io/badge/License-Apache--2.0-blue.svg)](LICENSE)
[![GitHub](https://img.shields.io/badge/GitHub-DengQingNian%2FFlowtide-181717?logo=github)](https://github.com/DengQingNian/Flowtide)

Flowtide 是一个不依赖 JVM、不依赖 CGO 的 BPMN 2.0 引擎。它复用 Flowable 的 `ACT_*` 数据库 Schema 与流程 REST 契约，提供独立服务、Go 库嵌入和存量库迁移工具三种使用方式。

> **当前状态：BPMN 内核扩展实现阶段。** 已完成 BPMN 解析、部署、执行树、任务、网关、子流程、多实例、调用活动、消息/信号、定时器、错误、补偿、事件子流程、迁移、身份、历史、JSON/SQL 持久化与 REST 核心端点。官方 184 个流程 REST 方法端点均已有路由；当前适合新建库原型、功能验证与迁移预检，尚未通过 Java 存量停写、回退或集群验收。完整边界见[能力矩阵](docs/003-迁移审计/003-能力矩阵与迁移退出条件.md)。

## 重构来源

Flowtide **重构自 [Flowable](https://github.com/flowable/flowable-engine)**（Java 版工作流与决策引擎）。本项目保留其数据模型与接口契约，用纯 Go 重写运行时，以便在无 JVM、无 CGO 的环境中运行。

| 项 | 说明 |
| --- | --- |
| 上游项目 | [flowable/flowable-engine](https://github.com/flowable/flowable-engine) |
| 重构基线 | Java 提交 `f6017e32a4bdd3e59a8881f58b0ee181652d1d9e` |
| 兼容基线 | Flowable 8.1.0.3、`ACT_*` Schema、流程 REST OpenAPI |
| 本仓库 | [DengQingNian/Flowtide](https://github.com/DengQingNian/Flowtide) |
| Go 模块 | `github.com/DengQingNian/Flowtide` |
| 许可证 | Apache License 2.0 |

> Flowtide 是独立的社区重构项目，不是 Flowable 官方发行版，与 Flowable 项目及其维护者没有隶属、赞助或背书关系。Flowable 名称及相关商标归其权利人所有。

### 命名说明

- 项目名、Go 模块、根包、二进制与状态格式统一使用 **Flowtide / flowtide**。
- `ACT_*` 表名、`internal/persistence/flowable` 包与 `FLOWABLE_*` 测试环境变量保留 Flowable 命名：它们描述的是与上游兼容的数据映射和验收层，不是旧项目名残留。
- 本次更名将状态格式标识从 `flowable-go-state` 调整为 `flowtide-state`，SQL 快照行与行仓储控制行前缀同步改为 `flowtide-*`。旧原型的 SQL 快照与行仓储控制行不会被自动识别，请重新生成；JSON 状态文件本身不含格式标识，仍可读取，但建议备份后验证。

## 为什么选择 Flowtide

- **部署更轻**：单二进制、无 JVM、无 CGO，适合容器、边缘与受限环境。
- **资源更省**：Go 运行时与内存占用显著低于同规模 JVM 部署。
- **可嵌入**：既能作为独立 REST 服务，也能作为 Go library 集成进业务进程。
- **数据兼容**：复用 Flowable 的 `ACT_*` Schema，降低存量库评估与迁移成本。
- **接口兼容**：流程 REST 以官方 OpenAPI 为契约，减少客户端改造。
- **可验证**：迁移预检、停写静默检查、状态恢复校验与双库集成测试均有独立命令。

## 快速开始

要求 Go 1.27+。构建与运行：

```bash
# 构建服务端与迁移工具（无 CGO）
make build

# 启动本地服务；-state 为空时使用内存状态
./bin/flowtide-server -addr :8080 -history audit -state ./data/engine.json -jobs-interval 1s

# 健康检查
curl -sS http://localhost:8080/healthz
```

部署并启动第一个流程：

```bash
# 部署 BPMN
curl -sS -X POST 'http://localhost:8080/service/repository/deployments?deploymentName=demo' \
  -H 'Content-Type: application/xml' \
  --data-binary @process.bpmn20.xml

# 启动实例
curl -sS -X POST 'http://localhost:8080/service/runtime/process-instances' \
  -H 'Content-Type: application/json' \
  -d '{"processDefinitionKey":"demo","variables":[{"name":"initiator","value":"alice"}]}'

# 查询待办
curl -sS 'http://localhost:8080/service/runtime/tasks?assignee=alice'
```

作为 Go 库嵌入：

```go
package main

import (
	"context"
	"log"

	"github.com/DengQingNian/Flowtide"
	"github.com/DengQingNian/Flowtide/api"
)

func main() {
	ctx := context.Background()
	app, err := flowtide.New(flowtide.Config{StatePath: "./data/engine.json"})
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close(ctx)

	task, err := app.TaskService().CreateTask(ctx, api.CreateTaskRequest{Name: "独立任务"})
	if err != nil {
		log.Fatal(err)
	}
	log.Println("created", task.ID)
}
```

公共入口为 `flowtide.New(flowtide.Config{})`，服务接口位于 `api`；仓库外模块验收位于 `testdata/sdk`。

## 能力范围

| 层级 | 范围 | 状态 |
| --- | --- | --- |
| T0 内核 | BPMN 模型/解析、PVM、命令、持久化、变量、任务、历史、作业、身份、流程 REST | 按能力矩阵分项验收 |
| T1 BPMN 增强 | 事件注册、外部工作项、迁移、批处理、DMN 调用、HTTP/Mail | 部分实现，持续补齐 |
| T2 多引擎 | CMMN、DMN、Form、App、Content | 计划中 |
| T3 生态 | LDAP、Kafka/Rabbit、多数据库、Helm | 计划中 |

明确不做：Java 类级二方库兼容、Spring/CDI/OSGi/JMX、Flowable 5 兼容层、Designer/Modeler 前端。详见 [001 项目总览与范围](docs/001-flowtide-重构计划/001-项目总览与范围界定.md)。

<details>
<summary><strong>展开完整能力清单</strong></summary>

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
- 执行树查询、实例任务、执行变量、单变量、启动表单、流程/任务评论与附件（增删查）、历史活动/变量详情与删除、作业查询/重试与用户组查询。
- 表单属性（启动/任务表单、必填/只读/类型校验、提交完成）。
- 发送/接收任务（消息点对点、信号广播、等待与触发）。
- 任务变量、任务身份链接、实例变量、事务子流程与可插拔状态存储。
- Flowable 8.1 的 41 张 `ACT_*` 表 DDL（MySQL/PostgreSQL）与 `flowtide-migrate` CLI。
- `ACT_*` 行映射、导出与导入（仓库、运行时、作业/订阅/死信、历史/评论/附件/身份链接/变量明细/任务日志）。
- MySQL 开发配置、连接池、Schema 初始化校验、原子 SQL 快照与真实数据库集成测试（默认跳过）。
- 行级 SQL 仓储、`REV_` 乐观锁、事务工作单元、全量/增量/自动写入、启动恢复与单一状态源。
- MySQL 行仓储服务模式：`-storage mysql-rows`、启动恢复、命令后增量写入、独占锁与快照互斥。
- REST 认证/身份库、PBKDF2、RBAC、租户隔离、限流、TLS、双向 TLS（CN/O/OU 身份映射）、审计与 Prometheus 指标。
- 部署管理（列表、详情、资源下载、级联删除、ZIP/BAR 部署包）。
- 事件子流程、补偿、脚本任务、变量作用域、表达式子集与历史记录。
- JSON 状态文件持久化（原子写入、启动恢复、失败回滚）。
- REST：部署、流程定义、实例、任务、变量、历史、健康检查。
- 管理端点：引擎属性、41 张表的行数、列元数据与表数据分页；批次/分片文档与删除、作业按桶删除及 move/reschedule。
- POST 组合查询：流程实例、执行、任务、活动实例、变量实例、历史实例/任务/活动/变量/明细。
- 流程图图片：BPMN DI 解析、SVG/PNG 渲染、泳池/泳道、任务/事件图标与运行态高亮。
- 链接事件、附件二进制内容、流程模型仓库、候选发起人、用户信息键值与头像。
- 条件事件、动态活动注入、Ad-hoc 子流程、执行动作、瞬态变量、查询变量过滤。
- 部署资源 ETag/Last-Modified 与 304 条件缓存。
- JWT/JWKS Bearer 认证、DMN 决策表求值、BPMN 业务规则任务、任务监听器。
- 官方 184 个流程 REST 方法端点均已有路由；具体响应模式与 Java 行为仍待逐项验收。

</details>

## 数据库与迁移

MySQL 开发库初始化与两种服务模式：

```bash
go run ./cmd/flowtide-migrate -dialect mysql -apply -env .env

# SQL 快照模式
go run ./cmd/flowtide-server -storage mysql -env .env

# 按表行仓储模式（单执行者独占；与快照互斥）
go run ./cmd/flowtide-server -storage mysql-rows -env .env
```

`mysql` 模式使用 SQL 快照保存完整状态；`mysql-rows` 模式把 `ACT_*` 行作为权威状态，启动时从行表恢复、命令后增量写入，并以数据库独占锁限制单执行者。实例/历史查询下推、多租户下推、多实例协调与集群租约仍未实现。详见 [058 MySQL 行仓储服务模式](docs/002-迁移实施/058-MySQL行仓储服务模式.md)。

既有 Java 数据库可先执行只读预检，不初始化或修改源库：

```bash
./bin/flowtide-migrate -preflight -dialect mysql -env .env -out preflight.json
# 不提供 tenant 时检查全部租户；显式空值只检查默认租户。
./bin/flowtide-migrate -preflight -dialect mysql -tenant="" -out default-tenant.json
# PostgreSQL 连接从 FLOWABLE_POSTGRES_DSN 环境变量读取。
./bin/flowtide-migrate -preflight -dialect postgres -timeout 5m -out preflight-postgres.json
```

退出码 `0` 表示未发现阻断，`2` 表示报告存在阻断，`1` 表示参数、连接、扫描或输出失败。预检通过不代表 Java 实例已可由 Go 接管，转换、Java 对照续跑与回退仍须独立验收。

停写切换前可运行只读静默检查：

```bash
./bin/flowtide-migrate -cutover-check -dialect mysql -env .env -quiesce-window 30s -out cutover.json
```

预检通过后可准备离线 Go 状态文件，并在隔离引擎中校验恢复：

```bash
./bin/flowtide-migrate -convert -dialect mysql -env .env -out prepared-state.json
./bin/flowtide-migrate -verify-state -state prepared-state.json -out verify-report.json
./bin/flowtide-server -state prepared-state.json
```

状态文件包含业务值，权限为 `0600`，不得提交。转换只覆盖简单用户任务执行树，复杂作用域、作业和事件订阅仍阻断。范围与验收记录见 [053 预检](docs/002-迁移实施/053-既有Java数据库只读预检.md)、[054 离线转换](docs/002-迁移实施/054-离线状态转换与Java对照.md)、[059 静默检查](docs/002-迁移实施/059-停写切换静默检查.md) 与 [063 恢复校验](docs/002-迁移实施/063-状态文件恢复校验.md)。

## REST API

- 业务端点前缀：`/service`
- 健康与指标：`/healthz`、`/readyz`、`/metrics`
- JSON 请求：`Content-Type: application/json`
- 部署资源：`application/xml`、`application/zip` 或 multipart
- 当前共 242 个方法端点：官方 184 个、扩展 58 个

认证支持 Basic、静态 Bearer、JWT/JWKS、mTLS 客户端证书与内置身份库；授权支持 RBAC 与租户绑定。未配置认证时端点开放，只适合开发环境。详见 [REST 约定](docs-site/src/content/rest/conventions.md)、[认证与租户](docs-site/src/content/rest/auth.md) 与 [REST 覆盖与差异](docs-site/src/content/rest/coverage.md)。

## 文档

- **设计文档**：[docs/001-flowtide-重构计划](docs/001-flowtide-重构计划/)，包含总体架构、PVM、数据模型、REST、质量与里程碑。
- **实施记录**：[docs/002-迁移实施](docs/002-迁移实施/)，按功能记录实现、验证与边界。
- **迁移审计**：[docs/003-迁移审计](docs/003-迁移审计/)，能力矩阵、REST 对照与迁移退出条件。
- **公开文档站**：[docs-site](docs-site/)，Vue 3 + TypeScript + Naive UI。

```bash
cd docs-site
npm install
npm run dev      # 本地预览
npm run build    # 静态产物 dist/
npm test         # 内容、搜索、导航与组件测试
```

文档站事实、页面与搜索索引由 `npm run gen` 从仓库生成，站内链接与锚点在构建期校验。

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

## 质量与验收

- 每个函数具备中文注释与 `@param/@return`。
- 圈复杂度 ≥ 3 的分支加行注释。
- 测试驱动开发，行覆盖率 ≥ 0.90。
- 所有文档、注释使用中文。
- 阶段性成果按约定格式提交（`feat|fix|perf|chore(模块): 描述`）。

```bash
make check        # 格式、注释、vet、测试、覆盖率、契约、独立 SDK 与构建
make race         # 竞争检测
make test-mysql   # 真实 MySQL 临时库验收
make test-postgres # 真实 PostgreSQL 验收（FLOWABLE_POSTGRES_DSN）
make bench        # 引擎与行仓储基准，不作为容量承诺
```

## 项目结构

```text
api/                      公共请求/响应类型与服务接口
bpmn/model/               BPMN 模型
cmd/flowtide-server/      REST 服务入口
cmd/flowtide-migrate/     迁移与数据库工具
internal/engine/          引擎运行时、命令、Agenda、任务与历史
internal/persistence/     状态文件、SQL 快照与 ACT_* 行仓储
internal/persistence/flowable/  Flowable ACT_* 行映射（保留上游命名）
internal/rest/            REST 路由、认证、租户与指标
internal/migration/       预检、转换、静默检查与恢复校验
docs/                     设计与实施文档
docs-site/                Vue 3 文档站
testdata/                 兼容基线、BPMN 语料与独立 SDK 验收
```

文档命名遵循 `docs/{三位流水号}-{描述}/{三位流水号}-{描述}.md`；范围或架构变更需同步更新 001 与 014。

## 许可证与致谢

本项目遵循 [Apache License 2.0](LICENSE)。

Flowtide 重构自 [Flowable](https://github.com/flowable/flowable-engine)。参考的 Flowable 代码、测试语料、SQL Schema 与文档仍遵循其原始 Apache-2.0 许可，相关版权与许可声明归原项目及其贡献者所有。

感谢 Flowable 社区提供的引擎设计、数据模型与兼容基线，使本项目能够在公开契约之上进行独立实现与验证。
