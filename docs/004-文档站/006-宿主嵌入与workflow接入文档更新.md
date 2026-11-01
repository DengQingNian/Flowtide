# 宿主嵌入与 workflow 接入文档更新

日期：2026-11-01。基线：`8a0f2e4`（宿主嵌入 SDK）与 `723a2a4`（v0.1.1 模块发布）。状态：文档站、README 与示例已更新并验证；验证过程中发现两处与本轮改动无关的既有测试失败，已记录未修改。

## 一、差距盘点

1. 文档站 SDK 分组仍停留在"仓库外无法配置数据库、等待后续公共持久化抽象"，与 `NewEmbedded` 的实际能力相反，涉及 `sdk/index`、`sdk/config`、`sdk/persistence`、`sdk/limits`、`getting-started/quickstart-go`、`reference/config`、`compatibility/known-issues` 与 `progress/persistence`。
2. 文档站没有任何 workflow 二次封装与管理台页面，而 015/016 已落地 95 条 `/workflow/**` 路由、84 条接线与九页管理台。
3. 没有可运行的宿主嵌入示例；README 只展示轻量入口 `flowtide.New`。
4. 发布页仍写 `0.1.0`、提交 `2925fc7`、无 tag，与 v0.1.1 模块发布不符。

## 二、更新内容

### 新增页面

| 文件 | 内容 |
| --- | --- |
| `sdk/embedding.md` | 生产接入入口、单写者约束、七步装配、配置字段、HTTP 与身份、作业驱动、失败语义表、检查清单 |
| `sdk/workflow.md` | 两条接入路径、95/84/11 路由口径、R 协议、门面方法分组、存储模式差异、三组钩子与调度边界 |
| `getting-started/quickstart-embed.md` | 从迁移初始化到发起、待办、办结与停机的端到端示例 |
| `operations/admin-console.md` | 页面结构、工作流九页、认证边界、开发代理与构建命令 |

### 新增示例

- `examples/host-embedding`：内存与 MySQL 两种模式、宿主认证中间件、`RunDueJobs` 循环、`StripPrefix` 挂载与优雅停机。
- `.coverignore` 增加示例入口排除项，避免示例代码拉低核心库覆盖率门禁。

### 修订页面

- `sdk/index`：入口表补充 `NewEmbedded`，公共包与生命周期说明同步。
- `sdk/config`、`reference/config`：补充 `StartupContext`，把"等公共持久化抽象"改为"用 `NewEmbedded`"。
- `sdk/persistence`：行仓储入口改为 `NewEmbedded` 或 `-storage mysql-rows`，新增嵌入模式小节。
- `sdk/limits`：移除"仓库外无法配置数据库"，补充单写者、嵌入模式无后台扫描、SQL 模式无待办数据源。
- `sdk/testing`、`getting-started/quickstart-go`、`compatibility/known-issues`、`progress/persistence`：同步新入口与已知限制。
- `releases/index`：改为 v0.1.1 模块发布、最近能力增量与已知测试限制。
- `README.md`：嵌入章节增加 `NewEmbedded` 片段与文档链接，项目结构补充 `examples/`。
- `admin/vite.config.ts`：开发代理补充 `/workflow`，此前 `npm run dev` 下工作流页面拿不到数据。

### 规划文档注记

`docs/004-文档站/002` 的页面清单与 SDK 限制条目保留原规划，只加"v0.1.1 起改用 `NewEmbedded`"的注记，避免把历史约束改成事后结论。

## 三、验证

- `go vet ./...` 与 `go run ./tools/quality -doccheck` 通过。
- `examples/host-embedding` 构建通过；内存模式启动后验证 `/flowtide/healthz` 无认证主体返回 401、带 `X-Actor` 返回 200，宿主路由 `/host/deployed` 经门面返回 `{"deployed":false}`。
- 文档站 `npm run gen` 生成 54 页，站内链接与锚点全部有效；`npm test`（含覆盖率门禁）与 `npm run build`（vue-tsc + vite）通过；`npm run check:facts` 输出 `54 页 / 242 端点 / 覆盖率 91.21% 通过`。
- `go test ./...` 在 HEAD 上有两处既有失败，与本轮文档改动无关，未修改实现：
  1. `TestChecksumStable`：001 脚本的校验和基准按 CRLF 内容计算（`b684…`），而仓库 `.gitattributes` 统一为 LF，实际 SHA-256 为 `f8a484…`；PostgreSQL 同理。
  2. `TestSchedulerTriggerFunctions`：固定假时钟与真实计时混用，016 号记录已注明未修。

## 四、边界

- 未改动 001-003 设计文档与能力矩阵口径。
- 未宣称集群、存量库切换、Java 行为兼容或 workflow 多表事务已解决。
- SQL 行仓储缺少待办数据源、嵌入模式无 workflow 后台扫描等限制在页面中显式列出，不隐含为可用能力。

## 五、后续

- SDK 服务级页面（repository、runtime、task、history 等）仍缺，按原规划优先级补齐。
- 校验和基准建议先按 LF 归一后再定值，避免不同检出环境互相冲突。
- 调度器用例改用可控时钟，恢复全仓测试绿灯后再收紧发布检查。
