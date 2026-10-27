# 003 数据库 Schema 资产

> 文档编号：002-003  
> 更新日期：2026-10-28  
> 状态：SQL 资产已就绪，运行时仍使用 JSON 快照

## 1. 目的

为下一阶段的 SQL 持久化提供**与 Flowable 8.1 完全一致的 DDL 基线**，避免后续"边写代码边猜表结构"。

SQL 来源：

- 参考仓库：Flowable 8.1.0-SNAPSHOT（Apache-2.0）。
- 拼接模块：`flowable-engine-common`、`flowable-engine`、`flowable-idm-engine`。
- 文件：
  - `internal/persistence/schema/sql/mysql/001-flowable-8.1.sql`
  - `internal/persistence/schema/sql/postgres/001-flowable-8.1.sql`

## 2. 表清单

两种方言均包含 **41 张表**：

| 分组 | 表 |
| --- | --- |
| 公共运行时 | `ACT_GE_PROPERTY`、`ACT_GE_BYTEARRAY`、`ACT_RU_EXECUTION`、`ACT_RU_TASK`、`ACT_RU_VARIABLE`、`ACT_RU_IDENTITYLINK`、`ACT_RU_ENTITYLINK`、`ACT_RU_EVENT_SUBSCR`、`ACT_RU_ACTINST` |
| 作业 | `ACT_RU_JOB`、`ACT_RU_TIMER_JOB`、`ACT_RU_SUSPENDED_JOB`、`ACT_RU_DEADLETTER_JOB`、`ACT_RU_HISTORY_JOB`、`ACT_RU_EXTERNAL_JOB` |
| 仓库 | `ACT_RE_DEPLOYMENT`、`ACT_RE_PROCDEF`、`ACT_RE_MODEL`、`ACT_PROCDEF_INFO` |
| 历史 | `ACT_HI_PROCINST`、`ACT_HI_ACTINST`、`ACT_HI_TASKINST`、`ACT_HI_VARINST`、`ACT_HI_DETAIL`、`ACT_HI_IDENTITYLINK`、`ACT_HI_ENTITYLINK`、`ACT_HI_COMMENT`、`ACT_HI_ATTACHMENT`、`ACT_HI_TSK_LOG` |
| 身份 | `ACT_ID_USER`、`ACT_ID_GROUP`、`ACT_ID_MEMBERSHIP`、`ACT_ID_INFO`、`ACT_ID_BYTEARRAY`、`ACT_ID_PRIV`、`ACT_ID_PRIV_MAPPING`、`ACT_ID_PROPERTY`、`ACT_ID_TOKEN` |
| 其他 | `ACT_EVT_LOG`、`FLW_RU_BATCH`、`FLW_RU_BATCH_PART` |

> 计数以 `create table` 去重统计为准；不同方言的索引/外键语句可能不同。

## 3. Go 资产

`internal/persistence/schema` 通过 `embed.FS` 打包 SQL：

~~~go
content, err := schema.Load("postgres")
tables, err := schema.Tables("postgres")
checksum, err := schema.Checksum("postgres")
~~~

- `Version` 固定为 `8.1.0.3`。
- `Tables` 返回排序、去重后的表名。
- `Checksum` 用于启动时校验 SQL 资产未被篡改。

## 4. 命令行

~~~bash
# 输出 MySQL DDL
go run ./cmd/flowable-migrate -dialect mysql

# 写入文件
go run ./cmd/flowable-migrate -dialect postgres -out schema.sql

# 仅列出表
go run ./cmd/flowable-migrate -dialect mysql -tables
~~~

## 5. 与运行时的关系

当前引擎运行时使用 JSON 状态快照，**尚未使用这些 SQL**。下一阶段计划：

1. 定义 `persistence.Store` 接口，把 JSON 快照替换为可插拔实现。
2. 引入 PostgreSQL/MySQL 驱动（`pgx`、`go-sql-driver/mysql`）。
3. 按实体实现仓储与工作单元，写入上述 41 张表。
4. 使用 `schema.Version` 做启动校验，提供升级脚本。
5. 用参考仓库初始化的数据库做双读对比。

## 6. 许可

SQL 资产来自 Flowable 参考仓库，遵循 Apache-2.0；文件头保留来源说明。分发时需保留 LICENSE 与归属声明。
