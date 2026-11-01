# 宿主嵌入示例

演示业务宿主如何内嵌 Flowtide：复用宿主连接池、挂载 HTTP 处理器、由宿主驱动到期作业。

## 内存模式

```bash
go run ./examples/host-embedding
```

- 引擎入口：`http://127.0.0.1:8099/flowtide/`（路径前缀由宿主提供，需带 `X-Actor` 头）
- 宿主自有路由：`GET /host/deployed?key={流程定义Key}`，演示 Go 门面调用

## MySQL 模式

```bash
go run ./examples/host-embedding \
  -dsn 'user:pass@tcp(127.0.0.1:3306)/flowtide?parseTime=true' \
  -database flowtide -tenant oa
```

首次接入空库时加 `-init-schema` 初始化引擎与 workflow 结构；之后去掉该参数，只做只读校验。

## 相关文档

- [宿主嵌入 SDK](docs-site/src/content/sdk/embedding.md)
- [嵌入式快速开始](docs-site/src/content/getting-started/quickstart-embed.md)
