---
title: 30 分钟接入宿主应用
description: 在业务进程内装配 MySQL 行仓储、workflow 门面与 HTTP 处理器，并用示例进程验证发起与办结。
order: 25
section: getting-started
status: prototype
since: v0.1.1
tags: [SDK, 嵌入, 快速开始, MySQL]
---

## 目标

把 Flowtide 嵌进已有 Go 业务进程：复用宿主的 <code>sql.DB</code>、暴露 <code>/workflow/**</code> 与 <code>/service/**</code>、由宿主驱动到期作业，最后用 curl 验证一次发起与办结。

仓库示例 <code>examples/host-embedding</code> 已包含本文全部接线，可先跑通再替换成自己的连接与租户。

## 步骤 1：准备依赖

~~~bash
mkdir host-demo && cd host-demo
go mod init host-demo
go get github.com/DengQingNian/Flowtide@v0.1.1
go get github.com/go-sql-driver/mysql
~~~

仓库内开发时用 <code>replace</code> 指向本地路径，避免版本等待：

~~~text title="go.mod"
require github.com/DengQingNian/Flowtide v0.1.1

replace github.com/DengQingNian/Flowtide => /path/to/Flowtide
~~~

## 步骤 2：准备数据库

嵌入模式只接受已完成迁移的 MySQL 8.0 库。空库先初始化一次：

~~~bash
go run github.com/DengQingNian/Flowtide/cmd/flowtide-migrate -dialect mysql -apply -env .env
~~~

初始化会创建 41 张 <code>ACT_*</code> 引擎表与 22 张 <code>workflow_*</code> 扩展表，并写入版本完成标记。已有 Java 存量库时先做只读预检，不要直接初始化。

连接串至少要带 <code>parseTime=true</code>；库名必须与 <code>EmbeddedConfig.Database</code> 一致。

## 步骤 3：装配运行时

~~~go title="main.go"
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
	// 持锁会话要独占一个连接，上限不能为 1。
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

	// 宿主认证由前置中间件完成，示例只在入口校验主体。
	mux := http.NewServeMux()
	mux.Handle("/flowtide/", http.StripPrefix("/flowtide", requireActor(app.Handler())))

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if _, err := app.RunDueJobs(ctx); err != nil {
				log.Println("到期作业失败:", err)
			}
		}
	}()

	log.Println("监听 :8099")
	log.Fatal(http.ListenAndServe(":8099", mux))
}

func requireActor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor := r.Header.Get("X-Actor")
		if actor == "" {
			http.Error(w, "缺少认证主体", http.StatusUnauthorized)
			return
		}
		r.Header.Set("X-Tenant-Id", "oa")
		next.ServeHTTP(w, r.WithContext(flowtide.WithExecutionContext(r.Context(), "oa", actor)))
	})
}
~~~

<code>requireActor</code> 只是示例。真实接入应校验会话或令牌，再把可信租户与办理人交给 <code>WithExecutionContext</code>。

## 步骤 4：部署并发起

~~~bash
# 部署 BPMN
curl -sS -X POST 'http://127.0.0.1:8099/flowtide/service/repository/deployments?deploymentName=leave' \
  -H 'X-Actor: 101' -H 'Content-Type: application/xml' \
  --data-binary @leave.bpmn20.xml

# 发起流程（workflow 门面）
curl -sS -X POST 'http://127.0.0.1:8099/flowtide/workflow/process/apply/task' \
  -H 'X-Actor: 101' -H 'Content-Type: application/json' \
  -d '{"processDefinitionKey":"leave","title":"请假申请","operator":"101","tenantId":"oa","businessKey":"L-1"}'
~~~

成功时返回 <code>{"code":"200","success":true,"data":{...}}</code>；业务失败仍是 HTTP 200，但 <code>success=false</code> 并带 <code>ErrorMessage</code>。

## 步骤 5：验证待办与办结

~~~bash
# 待办
curl -sS -H 'X-Actor: 101' \
  'http://127.0.0.1:8099/flowtide/workflow/process/task/all/todoList?tenantId=oa&identityId=101&start=0&size=10'

# 办结
curl -sS -X POST 'http://127.0.0.1:8099/flowtide/workflow/process/complete/task' \
  -H 'X-Actor: 101' -H 'Content-Type: application/json' \
  -d '{"taskId":"<任务ID>","userCode":"101","message":"同意","processInstanceId":"<实例ID>","type":"SP"}'
~~~

路径与方法以路由表为准；<code>/workflow/**</code> 的 95 条路由中 84 条已接线，其余返回 501。Go 侧等价调用：

~~~go
ctx = flowtide.WithExecutionContext(ctx, "oa", "101")
result, err := app.Workflow().GetTodoTaskListByIdentityID(ctx, "101", api.PageQuery{Start: 0, Size: 10})
~~~

## 步骤 6：停机

顺序固定：先停止接流量，再 <code>app.Close</code> 释放引擎与数据库写者锁，最后关闭宿主连接池。

~~~go
shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
_ = server.Shutdown(shutdownCtx)
if err := app.Close(shutdownCtx); err != nil {
	log.Println("关闭失败:", err)
}
~~~

## 常见问题

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| <code>嵌入连接池至少需要两个连接</code> | <code>MaxOpenConns</code> 为 1 | 提高到 2 以上 |
| <code>嵌入数据库名与连接实际数据库不一致</code> | 配置库名与 DSN 不一致 | 用 <code>SELECT DATABASE()</code> 校准 |
| <code>MySQL Schema 尚未初始化</code> | 空库 | 执行 <code>flowtide-migrate -apply</code> |
| <code>无法独占数据库</code> | 同库已有写者 | 停掉其他实例，确认单写者 |
| <code>/workflow/**</code> 返回 501 | 保留端点或未注入门面 | 换用已接线端点，或使用独立服务 |
| 待办查询为空 | 租户或身份不一致 | 检查 <code>X-Tenant-Id</code> 与 <code>identityId</code> |

## 下一步

- [宿主嵌入 SDK](/docs/sdk/embedding)
- [workflow 业务门面](/docs/sdk/workflow)
- [SDK 持久化](/docs/sdk/persistence)
- [存量库预检](/docs/migration/preflight)
