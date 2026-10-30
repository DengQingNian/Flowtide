# 058 MySQL 行仓储服务模式

> 文档编号：002-058
> 更新日期：2026-10-28
> 对应任务：按表行仓储接入 flowtide-server

## 1. 背景与目标

`internal/engine` 已支持以 `RowRepository` 作为权威状态源，但 `cmd/flowtide-server` 此前只提供 `-storage mysql` 快照模式。本任务新增 `-storage mysql-rows`，让服务直接使用 `internal/persistence/flowable` 的 ACT_* 行仓储：

- 启动时连接 MySQL、校验 Schema，并获取与快照模式同名的数据库独占锁；
- 以 `flowable.Repository` 读写 ACT_* 表，设置 `LoadRows=true` 从行表恢复；
- 不设置 `StateStore` / `StatePath`，不写入 `flowtide-state` 快照；
- 行模式发现数据库已有快照状态时拒绝启动；
- 关闭时先关闭引擎（落行差异），再释放独占锁并关闭连接池。

原有 `-storage mysql` 快照模式的行为、参数和锁规则保持不变。

## 2. 启动方式

### 2.1 参数

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-storage` | 空 | `mysql` 为快照模式，`mysql-rows` 为按表行仓储模式；为空时沿用内存或 `-state` 文件。 |
| `-state` | 空 | 状态文件路径。与 `mysql`、`mysql-rows` 均互斥。 |
| `-env` | `.env` | MySQL 连接配置文件；进程环境变量优先于文件。 |
| `-history` | `audit` | 历史级别，行模式同样适用。 |
| `-addr` | `:8080` | HTTP 监听地址，行模式不改变 REST、mTLS、认证或 jobs 扫描行为。 |

### 2.2 启动示例

使用 `.env` 中的开发库：

```bash
HOME=/root go run ./cmd/flowtide-server \
  -storage mysql-rows \
  -addr :8080
```

显式指定配置文件和监听地址：

```bash
HOME=/root go run ./cmd/flowtide-server \
  -storage mysql-rows \
  -env .env \
  -addr :8080 \
  -history full
```

连接配置仍读取以下键，规则与快照模式一致：`db_mysql_host`、`db_mysql_port`、`db_mysql_user`、`db_mysql_pwd`、`db_mysql_db_name`。启动顺序为：

1. 解析并按互斥规则校验参数；
2. 读取连接配置并创建连接池；
3. 调用 `mysql.EnsureSchema(ctx, db, false)` 校验完整 Schema 和版本标记，不自动建表；
4. 在专用 `*sql.Conn` 上执行 `GET_LOCK(?, 0)`；
5. 持锁后检查 `ACT_GE_BYTEARRAY.NAME_='flowtide-state'`；
6. 构造 `mysql.LockedRows` 包装与 `flowtide.NewRepository(..., "mysql")`；
7. 以 `engine.Config{RowRepository: ..., LoadRows: true}` 创建引擎并加载行表。

任何一步失败都会关闭已取得的连接和锁，进程以非零状态退出。

### 2.3 非法组合

| 配置 | 结果 |
| --- | --- |
| `-storage mysql-rows -state state.json` | 启动失败："-storage mysql-rows 与 -state 不能同时使用" |
| `-storage mysql -state state.json` | 保持原有拒绝行为 |
| `-storage postgres` 等未知值 | 启动失败："不支持的存储类型" |
| `-storage mysql-rows` 但数据库 Schema 未初始化 | 启动失败，提示执行 `flowtide-migrate -dialect mysql -apply` |
| `-storage mysql-rows` 且已有 `flowtide-state` 快照 | 启动失败，提示不能与快照模式混用 |

## 3. 锁与单执行者边界

### 3.1 锁名规则

快照模式和行模式共用 `internal/persistence/sqlstate.LockName`：

```go
fmt.Sprintf("flowtide:%x", sha256.Sum256([]byte(namespace)))[:64]
```

其中 `namespace` 为数据库名 `db_mysql_db_name`。锁名总长 64，前缀为 `flowtide:`，后接 SHA-256 十六进制摘要的前 52 个字符。相同数据库无论以快照还是行模式启动，得到的锁名相同，因此两种模式不能同时运行。

### 3.2 专用连接与所有权校验

- 锁必须由 `NewLockedRows` 内部的专用 `*sql.Conn` 获取，并使用 `GET_LOCK(?, 0)` 非阻塞尝试；已有服务实例时直接拒绝启动。
- 行仓储的读写实际通过连接池执行，但 `LockedRows` 在每次 `ExecContext`、`QueryContext`、`BeginTx` 之前，都会在持锁连接上执行：

```sql
SELECT IS_USED_LOCK(?) = CONNECTION_ID()
```

  只有返回真才继续；返回 NULL、假或查询失败时返回错误，不执行数据库写入。
- 事务由 `lockedTransaction` 包装，`Commit` 前再次校验锁归属；锁丢失时先 `Rollback` 再返回错误，避免提交来源不明的数据。`Rollback` 不依赖锁状态，始终尝试回滚。
- 释放锁使用 `RELEASE_LOCK`，并校验返回值必须为 1。释放失败或锁已不归本连接时返回错误，同时将物理连接标记为坏连接，禁止带锁会话回到连接池。

### 3.3 单执行者语义

该锁只保证“同一个数据库同一时间只有一个 flowtide 服务实例写入”，不提供多主、只读副本或跨库分布式锁。多个实例同时启动时，后启动者立即失败，不会排队等待。若运行期间持锁连接断开，后续行操作和事务提交会失败，服务不会自动重新取锁；应重启并确认数据库状态。

## 4. 与快照模式的互斥

- 两种模式使用相同锁名，正常启动时互斥。
- 行模式在持锁后检查 `ACT_GE_BYTEARRAY` 是否存在 `NAME_='flowtide-state'` 的记录；只要存在就拒绝启动，防止快照与行表形成两个权威来源。
- 行模式不写入该快照记录；关闭引擎时只通过行仓储提交差异。
- `-state` 文件仍与两种 MySQL 模式互斥。
- 本次未修改快照模式的启动检查。如果数据库已有行数据却改用 `-storage mysql`，快照模式不会主动检测行表内容；切换模式前必须确认数据库用途，不能在同一数据库上交替启动两种模式。

## 5. 表数据可查询，但查询下推尚未实现

行模式会把引擎状态写入 Flowable 8.1 兼容的 ACT_* 表，数据可直接用 SQL 查询，便于与 Java 迁移数据对照。但当前实现仍有以下限制：

- 引擎启动时通过 `Repository.LoadAll`（或按租户过滤的 `LoadAllByTenant`）读取全部相关表，恢复为内存状态；
- REST 查询、分页、排序仍在内存状态上执行，不会下推为 SQL `WHERE` / `LIMIT` / `ORDER BY`；
- 表规模越大，启动时间和内存占用越高，不适合直接承载超大规模历史库；
- 不支持通过修改 ACT_* 表热更新运行中的引擎状态，数据库变更需重启后重新加载。

## 6. 失败恢复限制

- 每条引擎命令在内存状态变更后，通过行仓储事务提交增量差异；提交失败会回滚内存状态并返回错误。因此正常返回成功的命令应已落库。
- 进程崩溃时，已提交的行数据保留；事务中的未提交写入由 MySQL 回滚。没有额外的 WAL 或补偿日志。
- `engine.Close` 会再次提交最后差异。若关闭时数据库不可用，关闭函数返回错误，服务日志记录“关闭引擎时保存状态失败”，随后仍尝试释放锁并关闭连接池；未落库的最后变更需要人工核对。
- 若释放锁失败，`storageSetup.Close` 返回错误，日志记录“释放存储资源失败”。连接已被标记为坏连接，不会把持锁会话放回连接池。
- 当前没有自动重连取锁、主备切换或跨实例故障转移；独占权丢失后需要重启服务。
- 行模式与快照模式之间没有自动转换命令。切换前应使用迁移工具或人工流程确认数据来源，并清理另一种介质的残留状态。

## 7. 实现文件

| 文件 | 变更 |
| --- | --- |
| `cmd/flowtide-server/storage.go` | 新增 `parseStorageMode`、`newStorage`、`newRowStorage`、`storageSetup.Close`；选择快照或行仓储并统一管理释放顺序。 |
| `cmd/flowtide-server/main.go` | `-storage` 说明更新；将 `Store` / `Repository` / `LoadRows` 注入 `engine.Config`；关闭失败写日志。 |
| `internal/persistence/mysql/row_lock.go` | 新增 `LockedRows`、`NewLockedRows`、事务包装和 `CheckNoSnapshot`。 |
| `internal/persistence/sqlstate/exclusive.go` | 导出 `LockName`，快照模式复用同一锁名规则。 |
| `cmd/flowtide-server/storage_test.go` | 覆盖模式选择、互斥、连接失败、行仓储初始化、快照拒绝及释放幂等。 |
| `internal/persistence/mysql/row_lock_test.go` | 使用 sqlmock 覆盖锁校验、读写、事务提交/回滚、锁丢失、释放错误和快照检查。 |
| `internal/persistence/mysql/row_integration_test.go` | 新增真实 MySQL 行服务模式集成测试。 |

## 8. 验证命令与结果

### 8.1 默认单测与覆盖率

```bash
HOME=/root go test -count=1 -coverprofile=/tmp/final_cov.out \
  ./cmd/flowtide-server ./internal/persistence/mysql ./internal/persistence/sqlstate
```

本次结果（MySQL 行模式相关默认测试，不依赖真实数据库）：

```text
ok  github.com/DengQingNian/Flowtide/cmd/flowtide-server       45.8% of statements
ok  github.com/DengQingNian/Flowtide/internal/persistence/mysql  97.1% of statements
ok  github.com/DengQingNian/Flowtide/internal/persistence/sqlstate 92.1% of statements
```

核心库包 `internal/persistence/mysql` 与 `internal/persistence/sqlstate` 的语句覆盖率均高于 0.90。`row_lock.go` 中除 `Release` 的 `conn.Close` 失败分支外均达到 100%，`Release` 为 88.9%。

相关包整体回归：

```bash
HOME=/root go test -count=1 ./cmd/... ./internal/persistence/...
HOME=/root go vet ./cmd/flowtide-server ./internal/persistence/mysql ./internal/persistence/sqlstate
```

以上命令均通过，`go vet` 无输出。

### 8.2 真实 MySQL 集成测试

```bash
HOME=/root FLOWABLE_MYSQL_INTEGRATION=1 \
  go test -count=1 -run 'TestIntegration' -v ./internal/persistence/mysql
```

本次针对开发环境 MySQL 8.0.35 执行，每个用例创建独立临时库，结束后删除：

```text
--- PASS: TestIntegrationMySQLSchema
--- PASS: TestIntegrationMySQLSnapshot
--- PASS: TestIntegrationMySQLRows
--- PASS: TestIntegrationMySQLRowsServiceMode
--- PASS: TestIntegrationMySQLExclusive
PASS
ok  github.com/DengQingNian/Flowtide/internal/persistence/mysql  32.234s
```

新增 `TestIntegrationMySQLRowsServiceMode` 覆盖：

1. 临时库初始化 Schema 后以空行表启动行仓储引擎；
2. 部署流程、启动实例、查询并完成用户任务；
3. 关闭引擎后释放独占锁、关闭连接池；
4. 新建连接池与引擎，从 ACT_* 行表恢复部署、流程定义 XML、历史流程、历史任务和历史变量；
5. 首个实例持锁期间，第二个行模式连接立即失败；
6. 写入 `flowtide-state` 快照记录后，`CheckNoSnapshot` 拒绝启动。

集成测试仅在显式设置 `FLOWABLE_MYSQL_INTEGRATION=1` 时运行；显式启用后连接、建库或 Schema 失败都会 `t.Fatal`，不会转为跳过。可通过 `FLOWABLE_MYSQL_ENV` 指定其他配置文件，默认为仓库根目录 `.env`。
