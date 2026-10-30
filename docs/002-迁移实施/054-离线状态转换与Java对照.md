# 054 离线状态转换与 Java 用户任务对照

> 实施日期：2026-10-28。参考 Java 提交为 `f6017e32a4bdd3e59a8881f58b0ee181652d1d9e`，Schema 基线为 `8.1.0.3`。本阶段交付简单用户任务执行树的离线状态准备，以及固定 Java 引擎真实写库样本的只读对照；不交付停写切换、生产接管或回退演练。

## 1. 问题与结果

053 的只读预检只能说明来源是否存在已知阻断，不能生成 Go 可读取的状态，也没有真实 Java 引擎写入的样本证明恢复后的业务行为。本阶段新增：

- `flowtide-migrate -convert`：预检通过后把源行恢复到隔离内存引擎，序列化为带显式变量类型的 Go 状态文件；只创建新文件，不覆盖已有状态。
- Java H2 黄金样本：从固定提交编译真实 Flowable 8.1.0-SNAPSHOT，以 audit 历史级别运行 `start → approve → review → end`，保存两个任务断点和完成后的原始 `ACT_*` 行及 Java 公共服务结果。
- Java 列保留：`ACT_HI_PROCINST`、`ACT_HI_TASKINST`、`ACT_HI_ACTINST`、`ACT_HI_COMMENT` 的实际业务列进入 Go 状态，并在再次写出时恢复。
- 类型保真：状态格式版本 2 使用侧表保存整数、浮点、时间、字节、JSON 数字和嵌套容器，避免经过 `float64` 丢失大整数精度。
- 续跑审计形态：转换后的状态记录 `sequenceFlow` 历史，与 Java audit 历史一致；普通 Go 新建实例保持原有历史行为。

## 2. 使用方式

```bash
make build

# 先做只读预检；存在阻断时不会生成状态文件。
./bin/flowtide-migrate -preflight -dialect mysql -env .env -out preflight.json

# 预检通过后准备离线状态；-out 必须是当前不存在的路径。
./bin/flowtide-migrate -convert -dialect mysql -env .env -out prepared-state.json

# 服务端使用同一状态文件启动；切换前必须停止 Java 执行者与后台作业。
./bin/flowtide-server -state prepared-state.json

# PostgreSQL 源库沿用 FLOWABLE_POSTGRES_DSN。
./bin/flowtide-migrate -convert -dialect postgres -timeout 10m -out prepared-postgres.json
```

`-convert` 必须与 `-preflight`、`-apply`、`-tables` 互斥，`-out` 必填；`-tenant` 和 `-timeout` 适用于预检与转换。报告写到标准输出，状态只写入 `-out` 指定的新文件，权限为 `0600`。已有文件、悬空链接、配置文件和目录均拒绝；发布使用同目录硬链接，避免检查与创建之间的覆盖竞争。

| 退出码 | 含义 | 后续动作 |
| --- | --- | --- |
| 0 | 预检无阻断且状态已创建 | 核对报告摘要，进入停写与切换演练；不能直接双写 |
| 2 | 报告包含阻断 | 按问题码整改，不产生状态文件 |
| 1 | 参数、连接、扫描、转换或输出失败 | 修复后重新执行，不采用旧报告或旧状态 |

状态文件包含业务变量和历史内容，不得提交到版本库或当作可公开日志。报告中的 `preparedState` 只记录格式、摘要、字节数和源/转换行数，不包含变量值。

## 3. 转换范围与规则

转换复用预检分析器；只要报告存在任一阻断，就不进入恢复阶段。通过后按预定租户过滤源行，在隔离引擎中执行标准行恢复，最后从状态存储读取完整字节：

1. 校验 Schema 版本、必需表、未知非空表、运行时/历史/身份链接归属、变量类型和值编码。
2. 仅在逐列证明 `ACT_RU_ACTINST` 与同标识 `ACT_HI_ACTINST` 等价时排除运行时活动容器；否则阻断，避免丢弃独有列。
3. 使用与生产相同的 `RestoreRows` 入口恢复仓库、运行时和历史；恢复失败不产生状态。
4. 以状态格式版本 2 序列化，变量类型写入侧表；历史活动、任务、流程和评论保存 Java 原始业务列。
5. 状态内标记本次实例来自 Java 迁移，续跑时补记 `sequenceFlow` 历史并保持 audit 语义。

当前只对已验证的简单用户任务执行树放行。作业、事件订阅、复杂作用域、多实例、调用活动、脚本/委托映射和未分析的非空表仍按预检规则阻断。

## 4. Java 参考样本

`tools/java-reference` 是本地验收生成器，按本阶段约定不纳入版本库；干净检出不能直接重新生成 Java 样本。生成器从固定源码提交编译 Flowable，在 JDK 17、H2 2.4.240 下使用固定时钟运行：

- 流程为 `start → approve → review → end`，租户 `tenant-a`，业务键 `java-business`。
- `approve` 设置候选用户/组并由 `alice` 签收；`review` 固定处理人 `carol`。
- 流程变量包含中文、`9007199254740993` 大整数、布尔、字节、日期和 JSON；任务局部变量各自保存。
- 保存 `approve`、`review` 两个等待断点及 `completed` 完成状态，每个断点包含 JDBC 原始行和 Java 公共服务期望。

固定制品为 [001-user-task-audit.json](../../testdata/compatibility/java/001-user-task-audit.json)，来源摘要见 [provenance.json](../../testdata/compatibility/java/provenance.json)，摘要已登记到 [兼容基线](../../testdata/compatibility/baseline.json)。Go 测试直接读取该制品，执行预检、离线转换、独立引擎恢复和任务完成，并对照 Java 任务、候选人、变量、历史活动/任务/变量及原始标识；不使用 Go 输出覆盖 Java 期望。

## 5. 验证记录

2026-10-28 本阶段验收结果：

| 检查 | 结果 |
| --- | --- |
| `make check` | 通过：中文函数注释、格式、vet、覆盖率、基线摘要、端点清单、独立 SDK 与无 CGO 构建 |
| 核心库加权总语句覆盖率 | `14662 / 16006 = 91.6031%`，逐核心库包达到 90% |
| `internal/migration` | 96.1% 语句覆盖率 |
| `internal/engine` / `internal/persistence/flowable` | 91.3% / 91.2% 语句覆盖率 |
| `make race` | 全仓竞争检测通过 |
| `make test-mysql` | MySQL 8.0.35，原持久化、行仓储与预检/转换测试通过 |
| `make test-postgres` | PostgreSQL 16.3，同上 |
| Java 黄金样本 | `approve`、`review`、`completed` 三组恢复与续跑对照通过 |
| 纯 Go 构建 | `CGO_ENABLED=0` 通过 |

覆盖率沿用仓库既有的 Go 语句覆盖率口径；命令入口按 `.coverignore` 豁免核心库门禁。MySQL/PostgreSQL 测试只创建和删除专用临时库，Java 样本使用自身 H2 内存库，均不读取业务库。

## 6. 适用边界

- H2 黄金样本证明固定提交的 Java 引擎确实产生这些行和业务结果，不替代 MySQL/PostgreSQL 接管验收，也不证明停写切换、回退或生产容量。
- 转换只覆盖简单用户任务执行树；作业、订阅、复杂作用域、多实例、调用活动、历史明细/附件/模型等仍阻断或未分析。
- 序列流历史补记只对转换状态启用，普通 Go 新建实例没有该行为；这是当前已知差异，后续若统一历史语义需另行验收。
- 预检和转换都是只读扫描加离线产物；若 Java 仍在运行，状态只代表扫描事务快照。启用 Go 前必须停止 Java 执行者和后台作业，禁止双写。
- 未实现反向转换、还原演练、实例归属切换和回退窗口；转换成功不能推导出可回退。
- 生成器依赖本地 Java 源码和 Maven 缓存，不在自动门禁内；固定制品的哈希和来源摘要由 `make contract` 校验。
