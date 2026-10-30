---
title: 安装
description: 从源码构建 flowtide-server 与 flowtide-migrate，验证二进制与健康检查，并了解可选数据库依赖。
order: 10
section: getting-started
status: verified
since: v0.1.0
tags: [安装, 构建, 快速开始]
---

## 前置条件

| 依赖 | 版本 | 是否必需 |
| --- | --- | --- |
| Go | 1.27+ | 构建必需 |
| MySQL | 8.0 | 可选，行仓储/快照模式需要 |
| PostgreSQL | 16 | 可选，迁移预检与行仓储需要 |
| Docker | 未发布镜像 | 当前不可用 |

服务端默认使用内存或 JSON 状态文件，不连接数据库也能完成快速开始。

## 从源码构建

~~~bash
git clone https://github.com/DengQingNian/Flowtide.git
cd Flowtide
HOME=/root make build
ls -lh bin/
~~~

构建产物：

| 文件 | 用途 |
| --- | --- |
| <code>bin/flowtide-server</code> | REST 服务与内嵌引擎 |
| <code>bin/flowtide-migrate</code> | Schema 初始化、预检、转换、静默检查与状态校验 |

<code>make build</code> 使用 <code>CGO_ENABLED=0</code>，不依赖 C 工具链。

## 启动

~~~bash
./bin/flowtide-server \
  -addr :8080 \
  -history audit \
  -state ./data/engine.json \
  -jobs-interval 1s
~~~

首次启动时 <code>./data</code> 目录需存在，或改用已有目录。状态文件以原子方式写入，权限包含业务数据，不应提交到版本库。

## 验证

~~~bash
curl -sS http://localhost:8080/healthz
# {"status":"UP"}

curl -sS http://localhost:8080/readyz
# 就绪状态

./bin/flowtide-server -h
~~~

<code>/healthz</code> 只表示进程存活；<code>/readyz</code> 用于检查引擎是否可接受请求。标准二进制还提供 <code>/metrics</code>，即使未启用 HTTP 指标也会输出引擎 gauge。

## 可选：启用数据库

~~~bash
# 建表与 Schema 校验；重复执行会校验版本与完成标记
go run ./cmd/flowtide-migrate -dialect mysql -apply -env .env

# SQL 快照模式
go run ./cmd/flowtide-server -storage mysql -env .env

# 行仓储模式（与快照互斥，单执行者独占）
go run ./cmd/flowtide-server -storage mysql-rows -env .env
~~~

<code>.env</code> 中的开发连接参数见仓库示例文件。数据库模式要求先完成 Schema 初始化；服务端启动时只校验，不执行 DDL。

::: warning 尚未发布 Docker 镜像
文档中的容器部署属于规划内容。当前没有官方镜像、Helm Chart 或 Kubernetes 清单。
:::

## 下一步

- [10 分钟跑通第一个流程（REST）](/docs/getting-started/quickstart-rest)
- [30 分钟嵌入 Go 应用](/docs/getting-started/quickstart-go)
- [核心概念](/docs/getting-started/concepts)

## 常见问题

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| <code>go: module cache not found</code> | 环境缺少 <code>HOME</code> 或 Go 缓存目录 | 显式设置 <code>HOME=/root</code> 后重试 |
| 端口被占用 | 8080 已有服务 | 改用 <code>-addr :18080</code> |
| 状态文件写入失败 | 目录不存在或权限不足 | 创建父目录并检查权限 |
| 数据库启动即失败 | Schema 未初始化或版本不符 | 先执行 <code>-apply</code>，再启动服务 |
