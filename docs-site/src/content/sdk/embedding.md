---
title: 宿主嵌入 SDK
description: 在业务进程内装配完整运行时：MySQL 行仓储、workflow 门面、HTTP 处理器、身份装饰与作业驱动。
order: 12
section: sdk
status: prototype
since: v0.1.1
tags: [SDK, 嵌入, workflow, MySQL]
---

## 适用边界

<code>NewEmbedded</code> 面向“宿主进程内跑完整审批能力”的接入：引擎内核、workflow 业务门面、<code>/workflow/**</code> 与 <code>/service/**</code> 处理器一次装配。只需要原生引擎服务、自行搭 HTTP 的场景仍可用 <code>flowtide.New</code>。

| 场景 | 入口 | 持久化 |
| --- | --- | --- |
| 独立服务 | <code>cmd/flowtide-server</code> | 状态文件、SQL 快照或 MySQL 行仓储 |
| 宿主嵌入（生产） | <code>flowtide.NewEmbedded</code> + <code>DB</code> | 宿主连接池上的 MySQL 行仓储 |
| 宿主嵌入（原型） | <code>flowtide.NewEmbedded</code> + <code>InMemory</code> | 内存 |
| 轻量 SDK | <code>flowtide.New</code> | 内存或状态文件 |

生产嵌入只接受已完成迁移的 MySQL 库：SDK 校验结构、版本完成标记与快照互斥，但不代替接入方做存量迁移。存量库的预检、转换、停写与回退仍在接入方工程验收。

## 单写者约束

嵌入运行时启动时对数据库名加 MySQL 命名锁（<code>GET_LOCK</code>，非阻塞），与独立服务的行仓储模式共用同一锁命名空间。同一数据库已有快照模式或行模式写者时直接启动失败，不会排队等待。

- 一个数据库同时只允许一个写者；集群、多副本与故障转移未实现。
- 持锁会话占用连接池中的一个连接，因此连接池上限不能为一。
- 进程异常退出时 MySQL 会话断开，锁随会话释放；重启后重新取锁。

## 装配步骤

1. 打开宿主连接池并设置 <code>MaxOpenConns</code>（至少 2，建议 10 左右）。
2. 首次接入空库时离线执行 <code>flowtide-migrate -dialect mysql -apply</code>；也可在本进程内显式调用 <code>flowtide.EnsureMySQLSchema(ctx, db, true)</code>。
3. 常规启动调用 <code>flowtide.EnsureMySQLSchema(ctx, db, false)</code> 做只读校验，或直接交给 <code>NewEmbedded</code>（它总会做只读校验）。
4. 调用 <code>flowtide.NewEmbedded</code> 构造运行时；失败时已获取的资源由 SDK 内部释放。
5. 把 <code>app.Handler()</code> 挂到宿主路由，并在前面加宿主自己的认证、授权与 TLS。
6. 由宿主循环调用 <code>app.RunDueJobs</code> 驱动定时器与异步作业。
7. 停机时先停止接收流量，再调用 <code>app.Close</code>；最后关闭宿主的 <code>sql.DB</code>。

~~~go
package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"time"

	"github.com/DengQingNian/Flowtide"
	_ "github.com/go-sql-driver/mysql"
)

func main() {
	ctx := context.Background()
	db, err := sql.Open("mysql", "user:pass@tcp(127.0.0.1:3306)/flowtide?parseTime=true")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(10)

	app, err := flowtide.NewEmbedded(ctx, flowtide.EmbeddedConfig{
		DB:           db,
		Database:     "flowtide",
		TenantID:     "oa",
		HistoryLevel: "audit",
	})
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close(ctx)

	go func() {
		for range time.Tick(5 * time.Second) {
			if _, err := app.RunDueJobs(ctx); err != nil {
				log.Println("job", err)
			}
		}
	}()

	mux := http.NewServeMux()
	// Handler 的路径以 / 开头，挂到子路径前先剥离前缀。
	mux.Handle("/flowtide/", http.StripPrefix("/flowtide", hostAuth(app.Handler())))
	log.Fatal(http.ListenAndServe(":8099", mux))
}
~~~

完整可运行示例见仓库 <code>examples/host-embedding</code>，包含内存模式、MySQL 模式、宿主认证与停机顺序。

## 配置字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| <code>DB</code> | <code>*sql.DB</code> | 宿主拥有的连接池；SDK 不关闭它 |
| <code>Database</code> | string | 库名，必须等于连接上的 <code>SELECT DATABASE()</code> |
| <code>TenantID</code> | string | 业务流程租户，宿主负责与数据库租户绑定 |
| <code>HistoryLevel</code> | string | 历史级别，默认 <code>audit</code> |
| <code>InMemory</code> | bool | 仅原型与测试；与数据库字段互斥 |
| <code>DecorateIdentity</code> | func | 可选身份读取适配，返回值不能为空 |

<code>InMemory</code> 与数据库配置必须二选一：两者同时出现或同时缺失都会被拒绝，不存在“连不上库就退回内存”的降级路径。

初始化上下文会传到状态加载与行恢复（<code>Config.StartupContext</code>），恢复阶段不会用 <code>Background</code> 绕过宿主超时；初始化完成后该上下文不再保留。

## 公开入口

| 方法 | 返回 | 用途 |
| --- | --- | --- |
| <code>Engine()</code> | <code>api.Engine</code> | 原生服务、事件钩子与会签监听器注册 |
| <code>Workflow()</code> | <code>api.WorkflowFacade</code> | Java 业务语义门面 |
| <code>Handler()</code> | <code>http.Handler</code> | <code>/service/**</code>、<code>/workflow/**</code>、健康与指标 |
| <code>RunDueJobs(ctx)</code> | <code>int, error</code> | 执行到期作业，返回执行数量 |
| <code>Close(ctx)</code> | <code>error</code> | 先关闭内核再释放写者锁，幂等且保留首次错误 |

## HTTP 接入

<code>Handler()</code> 已注册：

- <code>/service/**</code>：Flowable 兼容 REST，共 242 个方法端点。
- <code>/workflow/**</code>：workflow 业务门面，95 条路由。
- <code>/healthz</code>、<code>/readyz</code>、<code>/metrics</code>：健康与指标。
- <code>/adminpage/</code>：内嵌管理台静态资源。

处理器自带租户解析（<code>X-Tenant-Id</code> 头或 <code>tenantId</code> 查询参数）、审计、指标与限流中间件。认证与授权默认不开启：嵌入场景应由宿主在前置层完成，未加保护时端点等价于开放。

挂到子路径时必须 <code>http.StripPrefix</code>，路径匹配按根路径编写。管理台与宿主页面同源时会复用宿主的登录态或反向代理规则。

## 身份与租户

宿主认证完成后，用 <code>flowtide.WithExecutionContext</code> 把可信租户与办理人写进上下文：

~~~go
ctx = flowtide.WithExecutionContext(ctx, "oa", "101")
result, err := app.Workflow().IsProcessDeployed(ctx, "oa", "leave")
~~~

它只注入上下文，不执行认证，也不校验租户与身份的真实性。HTTP 请求仍需要 <code>X-Tenant-Id</code> 参与租户解析。

<code>DecorateIdentity</code> 用于把宿主用户目录接进引擎：接收内核原身份服务，返回装饰后的 <code>api.IdentityService</code>。公共引擎、workflow 门面与 HTTP 共用这份身份视图；返回 <code>nil</code> 会终止启动。需要会签监听器等具体引擎扩展方法时使用 <code>Engine()</code> 返回的具体实现，接口包装会丢掉这些方法。

## 作业与调度

- <code>RunDueJobs</code> 只负责“执行现有的到期作业”，调度频率、并发度与生命周期全部由宿主决定。
- SDK 不启动常驻调度器；独立服务里的 workflow 后台扫描（自动办结、超时提醒、催办、僵尸监控）没有公共入口，嵌入模式下这些扫描不会自动运行。
- 需要这些扫描能力时，先评估改用独立服务，或在宿主侧按业务约定显式触发。

## 失败语义

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| <code>嵌入模式必须显式选择内存或完整 MySQL 连接配置</code> | 内存与数据库字段混配或都缺失 | 二选一并补齐 |
| <code>嵌入数据库名与连接实际数据库不一致</code> | <code>Database</code> 与 DSN 库名不同 | 改为 <code>SELECT DATABASE()</code> 的结果 |
| <code>嵌入连接池至少需要两个连接</code> | <code>MaxOpenConns</code> 为 1 | 提高上限 |
| <code>MySQL Schema 尚未初始化</code> | 空库且未初始化 | 离线 <code>-apply</code> 或显式 <code>initialize=true</code> |
| <code>MySQL Schema 不完整</code> | 部分表已存在 | 人工检查迁移状态，禁止补建后当作可用 |
| <code>无法独占数据库，可能已有快照或行模式服务实例</code> | 锁被占用 | 确认没有其他写者 |
| <code>数据库包含 flowtide-state 快照数据</code> | 快照与行仓储混用 | 选定唯一权威来源 |
| <code>宿主身份装饰器返回空服务</code> | 装饰器返回 <code>nil</code> | 返回有效实现 |
| <code>嵌入 workflow 钩子装配失败</code> | 三组钩子预热失败 | 按错误定位，不要带缺失审批语义启动 |

## 接入检查清单

- [ ] 库名、连接池上限、单写者已确认。
- [ ] 结构校验与版本完成标记通过，存量库已完成迁移验收。
- [ ] 宿主认证、授权、TLS 覆盖 <code>/flowtide/**</code>。
- [ ] <code>RunDueJobs</code> 有宿主调度，停机时随上下文停止。
- [ ] 停机顺序为“停止接流量 → <code>Close</code> → 关闭连接池”。
- [ ] 已确认不需要 workflow 后台扫描，或已有替代方案。

## 相关阅读

- [Go SDK 概览](/docs/sdk)
- [workflow 业务门面](/docs/sdk/workflow)
- [嵌入式快速开始](/docs/getting-started/quickstart-embed)
- [SDK 持久化](/docs/sdk/persistence)
- [SDK 限制](/docs/sdk/limits)
