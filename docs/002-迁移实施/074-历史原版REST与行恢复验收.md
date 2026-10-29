# 074 历史原版REST与行恢复验收

验收日期：2026-10-30。对应进度总览 M6 与 M7，覆盖 21 个 `/history/*` 官方方法与 5 个 `/query/historic-*` 查询方法的原版 HTTP 契约，以及顺序流历史活动的原版语义。范围仍为单执行者；生产切换与集群另行验收。

## 原版样本生成

样本 `testdata/compatibility/java/007-history-rest.json` 由固定提交 `f6017e32` 的原版历史控制器与 Spring MockMvc 生成：独立 H2 内存库、`full` 历史、任务日志开启、单调二十位标识、基准时钟 `2026-10-30T01:02:03.123456789Z` 按固定步长推进（发起 +1h 候选与签收、+2h 变量更新、+3h 结束、+4h 启动第二实例），使时间排序在跨语言间确定。

44 个场景覆盖：历史实例/任务/活动/明细/变量的列表、分页、过滤与查询体；评论的创建、单读与删除；实例与任务身份链接；二进制变量数据端点（含 `payload` 字节与非 UTF-8 字节）；任务日志与按任务过滤；缺失资源 404 与运行中实例删除历史 500。两次独立 Java 执行的完整 JSON 一致；来源摘要见 `history-provenance.json`（38 个控制器类 SHA-256），制品摘要绑定 `baseline.json`。

## Go 回放与对齐差异

`TestJavaHistoryRESTReference` 以同一夹具（共享引擎、计数器续号、时钟复位）回放全部场景，逐字段比较完整响应与历史实体计数。对齐过程中确认并修复的原版语义：

- 顺序流按原版记为历史活动（此前仅在迁移标记下记录）；活动历史回填任务标识与处理人。
- 历史实例记录 `startActivityId`、`endActivityId` 与 `endUserId`；变量明细携带修订号（创建 0、第 n 次更新 n）且流程级更新不记活动标识。
- 历史任务记录优先级、类别、描述、所有者、到期等完整 DTO；任务日志事件名映射为 `USER_TASK_*`，完成数据为 `{}`，操作人采用可信主体。
- 身份链接：发起人写运行时 `starter` 链接与历史 `starter` 链接；任务候选与处理人在完成时升级为流程参与者（处理人变更即时升级），候选组保持任务范围；流程实例历史链接不含任务级条目；任务侧链接响应不带流程实例信息。
- 流程身份链接变更写入 `event` 类型空消息评论（原版行为），评论列表包含该事件。
- 历史表单：原版流程引擎未内置表单引擎，历史任务表单按原版返回 400（有表单键 "Form engine is not initialized"，无表单键 "Task has no form defined"）。
- 删除仍存在运行时实例的历史按原版返回 500；评论与身份链接列表返回裸数组；列表外壳携带 `sort/order` 且 `size` 为实际条数；显式 `size=0` 返回空数据。
- 二进制变量在列表 DTO 中值为空并以 `valueUrl` 指向数据端点；数据端点按原版返回字节流与 404 文案。

认证身份经 `engine.WithActor` 进入命令上下文，用于事件评论作者、任务日志操作人与结束操作人；未认证调用回退到请求值或空。

## 双库行恢复

历史行的行映射此前已覆盖（065、073）；本轮新增语义随标准行持久化：顺序流活动行、任务日志事件与数据、身份链接升级流水、变量明细修订号均随行仓储重启恢复，由既有 `TestIntegrationMySQLRows` / `TestIntegrationPostgresRowsServiceMode` 与黄金样本闭环覆盖。

## 回归与未完成项

```bash
go test ./internal/rest -run TestJavaHistoryRESTReference -count=1
go test ./internal/engine -run 'TestHistoryQueries|TestCompleteTask_EndsProcess' -count=1
make check
make test-mysql
```

边界：全量 184 方法的原版行为验收仍未完成（本轮后为 69 个方法 178 个场景）；身份 `TYPE_` 非 `userinfo` 的账户信息不支持行恢复（073）；集群与生产切换边界不变。
