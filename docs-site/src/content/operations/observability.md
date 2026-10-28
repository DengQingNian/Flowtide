---
title: 指标、健康检查与审计
description: healthz/readyz/metrics、指标名称、审计日志与当前 CLI 接入边界。
order: 40
section: operations
status: prototype
since: v0.1.0
tags: [运维, 监控, 指标, 审计]
---

## 探针

| 端点 | 用途 | 响应 |
| --- | --- | --- |
| <code>GET /healthz</code> | 进程存活 | <code>{"status":"UP"}</code> |
| <code>GET /readyz</code> | 引擎可接受请求 | 就绪状态 |
| <code>GET /metrics</code> | Prometheus 文本 | 指标 |

<code>/healthz</code> 不检查数据库锁或作业积压；编排系统应同时使用 <code>/readyz</code> 与业务检查。

## 指标

HTTP 指标（启用 <code>WithMetrics</code> 时）：

- <code>flowable_http_requests_total{method,status}</code>
- <code>flowable_http_request_duration_seconds_sum</code>
- <code>flowable_http_request_duration_seconds_count</code>
- <code>flowable_http_requests_in_flight</code>

引擎 gauge：

- 运行实例、任务、作业等计数（按实现输出）。
- 指标为进程内聚合，重启后重置。

即使未启用 HTTP 指标，<code>/metrics</code> 仍会输出引擎 gauge。标准 CLI 没有 <code>WithMetrics</code> flag，需要嵌入服务端才能采集 HTTP 指标。

## 日志

- 使用 <code>log/slog</code> 输出结构化日志。
- 启动、作业执行、关闭、错误会记录关键字段。
- 请求级审计日志需 <code>WithAuditLog</code> 启用，标准 CLI 未暴露。
- 不要记录密码、Token、数据库连接串与变量值。

## 审计

- 引擎历史（audit 级别）记录流程实例、活动、任务与变量。
- HTTP 审计日志为内存环形缓冲，重启丢失。
- <code>GET /service/management/audit-logs</code> 可查询已启用审计的请求。
- 全量审计、异步历史与定期清理未实现。

## 建议告警

| 指标/现象 | 建议 |
| --- | --- |
| 5xx 比例 | 阈值告警 |
| 请求延迟 | P95/P99 告警 |
| 作业积压/死信 | 数量与年龄告警 |
| 数据库锁持有 | 进程重启/异常时告警 |
| 状态文件写入失败 | 立即告警 |
| 任务积压 | 按业务 SLA |

## 相关阅读

- [存储模式与容量](/docs/operations/storage)
- [排障手册](/docs/operations/troubleshooting)
- [质量门禁](/docs/progress/quality)
