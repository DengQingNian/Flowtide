# 004 REST 端点对照

> 自动生成：`make contract-update`；校验：`make contract`。以方法及规范化路径为主键。
> 官方方法端点 184；实现方法端点 242。已编码表示路由存在，尚未证明响应模式和 Java 行为兼容。

| 方法与规范化路径 | 官方路径 | 实现路径 | 状态与限制 |
| --- | --- | --- | --- |
| `DELETE /history/historic-detail/{}` | `` | `/history/historic-detail/{id}` | 实现扩展；不计入官方覆盖 |
| `DELETE /history/historic-process-instances/{}` | `/history/historic-process-instances/{processInstanceId}` | `/history/historic-process-instances/{id}` | 已编码；待契约与 Java 行为验收 |
| `DELETE /history/historic-process-instances/{}/comments/{}` | `/history/historic-process-instances/{processInstanceId}/comments/{commentId}` | `/history/historic-process-instances/{id}/comments/{commentId}` | 已编码；待契约与 Java 行为验收 |
| `DELETE /history/historic-task-instances/{}` | `/history/historic-task-instances/{taskId}` | `/history/historic-task-instances/{id}` | 已编码；待契约与 Java 行为验收 |
| `DELETE /history/historic-task-log-entries/{}` | `` | `/history/historic-task-log-entries/{id}` | 实现扩展；不计入官方覆盖 |
| `DELETE /history/historic-variable-instances/{}` | `` | `/history/historic-variable-instances/{id}` | 实现扩展；不计入官方覆盖 |
| `DELETE /identity/groups/{}` | `/identity/groups/{groupId}` | `/identity/groups/{id}` | 已编码；待契约与 Java 行为验收 |
| `DELETE /identity/groups/{}/members/{}` | `/identity/groups/{groupId}/members/{userId}` | `/identity/groups/{id}/members/{userId}` | 已编码；待契约与 Java 行为验收 |
| `DELETE /identity/roles/{}` | `` | `/identity/roles/{id}` | 实现扩展；不计入官方覆盖 |
| `DELETE /identity/users/{}` | `/identity/users/{userId}` | `/identity/users/{id}` | 已编码；待契约与 Java 行为验收 |
| `DELETE /identity/users/{}/groups/{}` | `` | `/identity/users/{id}/groups/{groupId}` | 实现扩展；不计入官方覆盖 |
| `DELETE /identity/users/{}/info/{}` | `/identity/users/{userId}/info/{key}` | `/identity/users/{id}/info/{key}` | 已编码；待契约与 Java 行为验收 |
| `DELETE /management/batches/{}` | `/management/batches/{batchId}` | `/management/batches/{id}` | 已编码；待契约与 Java 行为验收 |
| `DELETE /management/deadletter-jobs/{}` | `/management/deadletter-jobs/{jobId}` | `/management/deadletter-jobs/{id}` | 已编码；待契约与 Java 行为验收 |
| `DELETE /management/engine-properties/{}` | `` | `/management/engine-properties/{name}` | 实现扩展；不计入官方覆盖 |
| `DELETE /management/jobs/{}` | `/management/jobs/{jobId}` | `/management/jobs/{id}` | 已编码；待契约与 Java 行为验收 |
| `DELETE /management/suspended-jobs/{}` | `/management/suspended-jobs/{jobId}` | `/management/suspended-jobs/{id}` | 已编码；待契约与 Java 行为验收 |
| `DELETE /management/timer-jobs/{}` | `/management/timer-jobs/{jobId}` | `/management/timer-jobs/{id}` | 已编码；待契约与 Java 行为验收 |
| `DELETE /repository/deployments/{}` | `/repository/deployments/{deploymentId}` | `/repository/deployments/{id}` | 已编码；待契约与 Java 行为验收 |
| `DELETE /repository/models/{}` | `/repository/models/{modelId}` | `/repository/models/{id}` | 已编码；待契约与 Java 行为验收 |
| `DELETE /repository/process-definitions/{}/identitylinks/{}/{}` | `/repository/process-definitions/{processDefinitionId}/identitylinks/{family}/{identityId}` | `/repository/process-definitions/{id}/identitylinks/{family}/{identityId}` | 已编码；待契约与 Java 行为验收 |
| `DELETE /runtime/executions/{}/variables` | `/runtime/executions/{executionId}/variables` | `/runtime/executions/{id}/variables` | 已编码；待契约与 Java 行为验收 |
| `DELETE /runtime/executions/{}/variables/{}` | `/runtime/executions/{executionId}/variables/{variableName}` | `/runtime/executions/{id}/variables/{name}` | 已编码；待契约与 Java 行为验收 |
| `DELETE /runtime/process-instances/{}` | `/runtime/process-instances/{processInstanceId}` | `/runtime/process-instances/{id}` | 原版实例与执行HTTP场景通过：完整DTO、分页、启动及动作副作用；完整字段组合待验收 |
| `DELETE /runtime/process-instances/{}/attachments/{}` | `` | `/runtime/process-instances/{id}/attachments/{attachmentId}` | 实现扩展；不计入官方覆盖 |
| `DELETE /runtime/process-instances/{}/comments/{}` | `` | `/runtime/process-instances/{id}/comments/{commentId}` | 实现扩展；不计入官方覆盖 |
| `DELETE /runtime/process-instances/{}/identitylinks/users/{}/{}` | `/runtime/process-instances/{processInstanceId}/identitylinks/users/{identityId}/{type}` | `/runtime/process-instances/{id}/identitylinks/users/{identityId}/{type}` | 已编码；待契约与 Java 行为验收 |
| `DELETE /runtime/process-instances/{}/identitylinks/{}/{}/{}` | `` | `/runtime/process-instances/{id}/identitylinks/{family}/{identityId}/{type}` | 实现扩展；不计入官方覆盖 |
| `DELETE /runtime/process-instances/{}/variables` | `/runtime/process-instances/{processInstanceId}/variables` | `/runtime/process-instances/{id}/variables` | 已编码；待契约与 Java 行为验收 |
| `DELETE /runtime/process-instances/{}/variables/{}` | `/runtime/process-instances/{processInstanceId}/variables/{variableName}` | `/runtime/process-instances/{id}/variables/{name}` | 已编码；待契约与 Java 行为验收 |
| `DELETE /runtime/tasks/{}` | `/runtime/tasks/{taskId}` | `/runtime/tasks/{id}` | 原版任务HTTP场景通过：完整DTO、分页与动作副作用；完整字段组合待验收 |
| `DELETE /runtime/tasks/{}/attachments/{}` | `/runtime/tasks/{taskId}/attachments/{attachmentId}` | `/runtime/tasks/{id}/attachments/{attachmentId}` | 已编码；待契约与 Java 行为验收 |
| `DELETE /runtime/tasks/{}/comments/{}` | `/runtime/tasks/{taskId}/comments/{commentId}` | `/runtime/tasks/{id}/comments/{commentId}` | 已编码；待契约与 Java 行为验收 |
| `DELETE /runtime/tasks/{}/events/{}` | `/runtime/tasks/{taskId}/events/{eventId}` | `/runtime/tasks/{id}/events/{eventId}` | 已编码；待契约与 Java 行为验收 |
| `DELETE /runtime/tasks/{}/identitylinks/{}/{}` | `` | `/runtime/tasks/{id}/identitylinks/{family}/{identityId}` | 实现扩展；不计入官方覆盖 |
| `DELETE /runtime/tasks/{}/identitylinks/{}/{}/{}` | `/runtime/tasks/{taskId}/identitylinks/{family}/{identityId}/{type}` | `/runtime/tasks/{id}/identitylinks/{family}/{identityId}/{type}` | 已编码；待契约与 Java 行为验收 |
| `DELETE /runtime/tasks/{}/variables` | `/runtime/tasks/{taskId}/variables` | `/runtime/tasks/{id}/variables` | 已编码；待契约与 Java 行为验收 |
| `DELETE /runtime/tasks/{}/variables/{}` | `/runtime/tasks/{taskId}/variables/{variableName}` | `/runtime/tasks/{id}/variables/{name}` | 已编码；待契约与 Java 行为验收 |
| `GET /form/form-data` | `/form/form-data` | `/form/form-data` | 已编码；待契约与 Java 行为验收 |
| `GET /history/historic-activity-instances` | `/history/historic-activity-instances` | `/history/historic-activity-instances` | 已编码；待契约与 Java 行为验收 |
| `GET /history/historic-activity-instances/{}` | `` | `/history/historic-activity-instances/{id}` | 实现扩展；不计入官方覆盖 |
| `GET /history/historic-detail` | `/history/historic-detail` | `/history/historic-detail` | 已编码；待契约与 Java 行为验收 |
| `GET /history/historic-detail/{}/data` | `/history/historic-detail/{detailId}/data` | `/history/historic-detail/{id}/data` | 已编码；待契约与 Java 行为验收 |
| `GET /history/historic-identitylinks` | `` | `/history/historic-identitylinks` | 实现扩展；不计入官方覆盖 |
| `GET /history/historic-process-instances` | `/history/historic-process-instances` | `/history/historic-process-instances` | 已编码；待契约与 Java 行为验收 |
| `GET /history/historic-process-instances/{}` | `/history/historic-process-instances/{processInstanceId}` | `/history/historic-process-instances/{id}` | 已编码；待契约与 Java 行为验收 |
| `GET /history/historic-process-instances/{}/comments` | `/history/historic-process-instances/{processInstanceId}/comments` | `/history/historic-process-instances/{id}/comments` | 已编码；待契约与 Java 行为验收 |
| `GET /history/historic-process-instances/{}/comments/{}` | `/history/historic-process-instances/{processInstanceId}/comments/{commentId}` | `/history/historic-process-instances/{id}/comments/{commentId}` | 已编码；待契约与 Java 行为验收 |
| `GET /history/historic-process-instances/{}/identitylinks` | `/history/historic-process-instances/{processInstanceId}/identitylinks` | `/history/historic-process-instances/{id}/identitylinks` | 已编码；待契约与 Java 行为验收 |
| `GET /history/historic-process-instances/{}/variables` | `` | `/history/historic-process-instances/{id}/variables` | 实现扩展；不计入官方覆盖 |
| `GET /history/historic-process-instances/{}/variables/{}/data` | `/history/historic-process-instances/{processInstanceId}/variables/{variableName}/data` | `/history/historic-process-instances/{id}/variables/{name}/data` | 已编码；待契约与 Java 行为验收 |
| `GET /history/historic-task-instances` | `/history/historic-task-instances` | `/history/historic-task-instances` | 已编码；待契约与 Java 行为验收 |
| `GET /history/historic-task-instances/{}` | `/history/historic-task-instances/{taskId}` | `/history/historic-task-instances/{id}` | 已编码；待契约与 Java 行为验收 |
| `GET /history/historic-task-instances/{}/form` | `/history/historic-task-instances/{taskId}/form` | `/history/historic-task-instances/{id}/form` | 已编码；待契约与 Java 行为验收 |
| `GET /history/historic-task-instances/{}/identitylinks` | `/history/historic-task-instances/{taskId}/identitylinks` | `/history/historic-task-instances/{id}/identitylinks` | 已编码；待契约与 Java 行为验收 |
| `GET /history/historic-task-instances/{}/variables/{}/data` | `/history/historic-task-instances/{taskId}/variables/{variableName}/data` | `/history/historic-task-instances/{id}/variables/{name}/data` | 已编码；待契约与 Java 行为验收 |
| `GET /history/historic-task-log-entries` | `/history/historic-task-log-entries` | `/history/historic-task-log-entries` | 已编码；待契约与 Java 行为验收 |
| `GET /history/historic-variable-instances` | `/history/historic-variable-instances` | `/history/historic-variable-instances` | 已编码；待契约与 Java 行为验收 |
| `GET /history/historic-variable-instances/{}` | `` | `/history/historic-variable-instances/{id}` | 实现扩展；不计入官方覆盖 |
| `GET /history/historic-variable-instances/{}/data` | `/history/historic-variable-instances/{varInstanceId}/data` | `/history/historic-variable-instances/{id}/data` | 已编码；待契约与 Java 行为验收 |
| `GET /identity/groups` | `/identity/groups` | `/identity/groups` | 已编码；待契约与 Java 行为验收 |
| `GET /identity/groups/{}` | `/identity/groups/{groupId}` | `/identity/groups/{id}` | 已编码；待契约与 Java 行为验收 |
| `GET /identity/groups/{}/members` | `` | `/identity/groups/{id}/members` | 实现扩展；不计入官方覆盖 |
| `GET /identity/roles` | `` | `/identity/roles` | 实现扩展；不计入官方覆盖 |
| `GET /identity/roles/{}` | `` | `/identity/roles/{id}` | 实现扩展；不计入官方覆盖 |
| `GET /identity/users` | `/identity/users` | `/identity/users` | 已编码；待契约与 Java 行为验收 |
| `GET /identity/users/{}` | `/identity/users/{userId}` | `/identity/users/{id}` | 已编码；待契约与 Java 行为验收 |
| `GET /identity/users/{}/groups` | `` | `/identity/users/{id}/groups` | 实现扩展；不计入官方覆盖 |
| `GET /identity/users/{}/info` | `/identity/users/{userId}/info` | `/identity/users/{id}/info` | 已编码；待契约与 Java 行为验收 |
| `GET /identity/users/{}/info/{}` | `/identity/users/{userId}/info/{key}` | `/identity/users/{id}/info/{key}` | 已编码；待契约与 Java 行为验收 |
| `GET /identity/users/{}/picture` | `/identity/users/{userId}/picture` | `/identity/users/{id}/picture` | 已编码；待契约与 Java 行为验收 |
| `GET /management/audit-logs` | `` | `/management/audit-logs` | 实现扩展；不计入官方覆盖 |
| `GET /management/batch-parts/{}` | `/management/batch-parts/{batchPartId}` | `/management/batch-parts/{id}` | 已编码；待契约与 Java 行为验收 |
| `GET /management/batch-parts/{}/batch-part-document` | `/management/batch-parts/{batchPartId}/batch-part-document` | `/management/batch-parts/{id}/batch-part-document` | 已编码；待契约与 Java 行为验收 |
| `GET /management/batches` | `/management/batches` | `/management/batches` | 已编码；待契约与 Java 行为验收 |
| `GET /management/batches/{}` | `/management/batches/{batchId}` | `/management/batches/{id}` | 已编码；待契约与 Java 行为验收 |
| `GET /management/batches/{}/batch-document` | `/management/batches/{batchId}/batch-document` | `/management/batches/{id}/batch-document` | 已编码；待契约与 Java 行为验收 |
| `GET /management/batches/{}/batch-parts` | `/management/batches/{batchId}/batch-parts` | `/management/batches/{id}/batch-parts` | 已编码；待契约与 Java 行为验收 |
| `GET /management/batches/{}/summary` | `` | `/management/batches/{id}/summary` | 实现扩展；不计入官方覆盖 |
| `GET /management/deadletter-jobs` | `/management/deadletter-jobs` | `/management/deadletter-jobs` | 已编码；待契约与 Java 行为验收 |
| `GET /management/deadletter-jobs/{}` | `/management/deadletter-jobs/{jobId}` | `/management/deadletter-jobs/{id}` | 已编码；待契约与 Java 行为验收 |
| `GET /management/deadletter-jobs/{}/exception-stacktrace` | `/management/deadletter-jobs/{jobId}/exception-stacktrace` | `/management/deadletter-jobs/{id}/exception-stacktrace` | 已编码；待契约与 Java 行为验收 |
| `GET /management/engine` | `/management/engine` | `/management/engine` | 已编码；待契约与 Java 行为验收 |
| `GET /management/engine-properties` | `` | `/management/engine-properties` | 实现扩展；不计入官方覆盖 |
| `GET /management/history-jobs` | `` | `/management/history-jobs` | 实现扩展；不计入官方覆盖 |
| `GET /management/history-jobs/{}` | `` | `/management/history-jobs/{id}` | 实现扩展；不计入官方覆盖 |
| `GET /management/jobs` | `/management/jobs` | `/management/jobs` | 已编码；待契约与 Java 行为验收 |
| `GET /management/jobs/{}` | `/management/jobs/{jobId}` | `/management/jobs/{id}` | 已编码；待契约与 Java 行为验收 |
| `GET /management/jobs/{}/exception-stacktrace` | `/management/jobs/{jobId}/exception-stacktrace` | `/management/jobs/{id}/exception-stacktrace` | 已编码；待契约与 Java 行为验收 |
| `GET /management/properties` | `/management/properties` | `/management/properties` | 已编码；待契约与 Java 行为验收 |
| `GET /management/suspended-jobs` | `/management/suspended-jobs` | `/management/suspended-jobs` | 已编码；待契约与 Java 行为验收 |
| `GET /management/suspended-jobs/{}` | `/management/suspended-jobs/{jobId}` | `/management/suspended-jobs/{id}` | 已编码；待契约与 Java 行为验收 |
| `GET /management/suspended-jobs/{}/exception-stacktrace` | `/management/suspended-jobs/{jobId}/exception-stacktrace` | `/management/suspended-jobs/{id}/exception-stacktrace` | 已编码；待契约与 Java 行为验收 |
| `GET /management/tables` | `/management/tables` | `/management/tables` | 已编码；待契约与 Java 行为验收 |
| `GET /management/tables/{}` | `/management/tables/{tableName}` | `/management/tables/{name}` | 已编码；待契约与 Java 行为验收 |
| `GET /management/tables/{}/columns` | `/management/tables/{tableName}/columns` | `/management/tables/{name}/columns` | 已编码；待契约与 Java 行为验收 |
| `GET /management/tables/{}/data` | `/management/tables/{tableName}/data` | `/management/tables/{name}/data` | 已编码；待契约与 Java 行为验收 |
| `GET /management/timer-jobs` | `/management/timer-jobs` | `/management/timer-jobs` | 已编码；待契约与 Java 行为验收 |
| `GET /management/timer-jobs/{}` | `/management/timer-jobs/{jobId}` | `/management/timer-jobs/{id}` | 已编码；待契约与 Java 行为验收 |
| `GET /management/timer-jobs/{}/exception-stacktrace` | `/management/timer-jobs/{jobId}/exception-stacktrace` | `/management/timer-jobs/{id}/exception-stacktrace` | 已编码；待契约与 Java 行为验收 |
| `GET /repository/deployments` | `/repository/deployments` | `/repository/deployments` | 已编码；待契约与 Java 行为验收 |
| `GET /repository/deployments/{}` | `/repository/deployments/{deploymentId}` | `/repository/deployments/{id}` | 已编码；待契约与 Java 行为验收 |
| `GET /repository/deployments/{}/resourcedata/{}` | `/repository/deployments/{deploymentId}/resourcedata/{resourceName}` | `/repository/deployments/{id}/resourcedata/{resourceName}` | 已编码；待契约与 Java 行为验收 |
| `GET /repository/deployments/{}/resources` | `/repository/deployments/{deploymentId}/resources` | `/repository/deployments/{id}/resources` | 已编码；待契约与 Java 行为验收 |
| `GET /repository/deployments/{}/resources/{**}` | `/repository/deployments/{deploymentId}/resources/**` | `/repository/deployments/{id}/resources/{name...}` | 已编码；待契约与 Java 行为验收 |
| `GET /repository/models` | `/repository/models` | `/repository/models` | 已编码；待契约与 Java 行为验收 |
| `GET /repository/models/{}` | `/repository/models/{modelId}` | `/repository/models/{id}` | 已编码；待契约与 Java 行为验收 |
| `GET /repository/models/{}/source` | `/repository/models/{modelId}/source` | `/repository/models/{id}/source` | 已编码；待契约与 Java 行为验收 |
| `GET /repository/models/{}/source-extra` | `/repository/models/{modelId}/source-extra` | `/repository/models/{id}/source-extra` | 已编码；待契约与 Java 行为验收 |
| `GET /repository/process-definitions` | `/repository/process-definitions` | `/repository/process-definitions` | 已编码；待契约与 Java 行为验收 |
| `GET /repository/process-definitions/{}` | `/repository/process-definitions/{processDefinitionId}` | `/repository/process-definitions/{id}` | 已编码；待契约与 Java 行为验收 |
| `GET /repository/process-definitions/{}/decision-tables` | `/repository/process-definitions/{processDefinitionId}/decision-tables` | `/repository/process-definitions/{id}/decision-tables` | 已编码；待契约与 Java 行为验收 |
| `GET /repository/process-definitions/{}/decisions` | `` | `/repository/process-definitions/{id}/decisions` | 实现扩展；不计入官方覆盖 |
| `GET /repository/process-definitions/{}/form-definitions` | `/repository/process-definitions/{processDefinitionId}/form-definitions` | `/repository/process-definitions/{id}/form-definitions` | 已编码；待契约与 Java 行为验收 |
| `GET /repository/process-definitions/{}/identitylinks` | `/repository/process-definitions/{processDefinitionId}/identitylinks` | `/repository/process-definitions/{id}/identitylinks` | 已编码；待契约与 Java 行为验收 |
| `GET /repository/process-definitions/{}/identitylinks/{}/{}` | `/repository/process-definitions/{processDefinitionId}/identitylinks/{family}/{identityId}` | `/repository/process-definitions/{id}/identitylinks/{family}/{identityId}` | 已编码；待契约与 Java 行为验收 |
| `GET /repository/process-definitions/{}/image` | `/repository/process-definitions/{processDefinitionId}/image` | `/repository/process-definitions/{id}/image` | 已编码；待契约与 Java 行为验收 |
| `GET /repository/process-definitions/{}/model` | `/repository/process-definitions/{processDefinitionId}/model` | `/repository/process-definitions/{id}/model` | 已编码；待契约与 Java 行为验收 |
| `GET /repository/process-definitions/{}/resourcedata` | `/repository/process-definitions/{processDefinitionId}/resourcedata` | `/repository/process-definitions/{id}/resourcedata` | 已编码；待契约与 Java 行为验收 |
| `GET /repository/process-definitions/{}/start-form` | `/repository/process-definitions/{processDefinitionId}/start-form` | `/repository/process-definitions/{id}/start-form` | 已编码；待契约与 Java 行为验收 |
| `GET /repository/process-definitions/{}/xml` | `` | `/repository/process-definitions/{id}/xml` | 实现扩展；不计入官方覆盖 |
| `GET /runtime/activity-instances` | `` | `/runtime/activity-instances` | 实现扩展；不计入官方覆盖 |
| `GET /runtime/event-subscriptions` | `/runtime/event-subscriptions` | `/runtime/event-subscriptions` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/event-subscriptions/{}` | `/runtime/event-subscriptions/{eventSubscriptionId}` | `/runtime/event-subscriptions/{id}` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/executions` | `/runtime/executions` | `/runtime/executions` | 原版实例与执行HTTP场景通过：完整DTO、分页、启动及动作副作用；完整字段组合待验收 |
| `GET /runtime/executions/{}` | `/runtime/executions/{executionId}` | `/runtime/executions/{id}` | 原版实例与执行HTTP场景通过：完整DTO、分页、启动及动作副作用；完整字段组合待验收 |
| `GET /runtime/executions/{}/activities` | `/runtime/executions/{executionId}/activities` | `/runtime/executions/{id}/activities` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/executions/{}/variables` | `/runtime/executions/{executionId}/variables` | `/runtime/executions/{id}/variables` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/executions/{}/variables/{}` | `/runtime/executions/{executionId}/variables/{variableName}` | `/runtime/executions/{id}/variables/{name}` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/executions/{}/variables/{}/data` | `/runtime/executions/{executionId}/variables/{variableName}/data` | `/runtime/executions/{id}/variables/{name}/data` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/process-instances` | `/runtime/process-instances` | `/runtime/process-instances` | 原版实例与执行HTTP场景通过：完整DTO、分页、启动及动作副作用；完整字段组合待验收 |
| `GET /runtime/process-instances/{}` | `/runtime/process-instances/{processInstanceId}` | `/runtime/process-instances/{id}` | 原版实例与执行HTTP场景通过：完整DTO、分页、启动及动作副作用；完整字段组合待验收 |
| `GET /runtime/process-instances/{}/attachments` | `` | `/runtime/process-instances/{id}/attachments` | 实现扩展；不计入官方覆盖 |
| `GET /runtime/process-instances/{}/comments` | `` | `/runtime/process-instances/{id}/comments` | 实现扩展；不计入官方覆盖 |
| `GET /runtime/process-instances/{}/diagram` | `/runtime/process-instances/{processInstanceId}/diagram` | `/runtime/process-instances/{id}/diagram` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/process-instances/{}/identitylinks` | `/runtime/process-instances/{processInstanceId}/identitylinks` | `/runtime/process-instances/{id}/identitylinks` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/process-instances/{}/identitylinks/users/{}/{}` | `/runtime/process-instances/{processInstanceId}/identitylinks/users/{identityId}/{type}` | `/runtime/process-instances/{id}/identitylinks/users/{identityId}/{type}` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/process-instances/{}/identitylinks/{}/{}` | `` | `/runtime/process-instances/{id}/identitylinks/{family}/{identityId}` | 实现扩展；不计入官方覆盖 |
| `GET /runtime/process-instances/{}/identitylinks/{}/{}/{}` | `` | `/runtime/process-instances/{id}/identitylinks/{family}/{identityId}/{type}` | 实现扩展；不计入官方覆盖 |
| `GET /runtime/process-instances/{}/tasks` | `` | `/runtime/process-instances/{id}/tasks` | 实现扩展；不计入官方覆盖 |
| `GET /runtime/process-instances/{}/variables` | `/runtime/process-instances/{processInstanceId}/variables` | `/runtime/process-instances/{id}/variables` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/process-instances/{}/variables/{}` | `/runtime/process-instances/{processInstanceId}/variables/{variableName}` | `/runtime/process-instances/{id}/variables/{name}` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/process-instances/{}/variables/{}/data` | `/runtime/process-instances/{processInstanceId}/variables/{variableName}/data` | `/runtime/process-instances/{id}/variables/{name}/data` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/tasks` | `/runtime/tasks` | `/runtime/tasks` | 原版任务HTTP场景通过：完整DTO、分页与动作副作用；完整字段组合待验收 |
| `GET /runtime/tasks/{}` | `/runtime/tasks/{taskId}` | `/runtime/tasks/{id}` | 原版任务HTTP场景通过：完整DTO、分页与动作副作用；完整字段组合待验收 |
| `GET /runtime/tasks/{}/attachments` | `/runtime/tasks/{taskId}/attachments` | `/runtime/tasks/{id}/attachments` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/tasks/{}/attachments/{}` | `/runtime/tasks/{taskId}/attachments/{attachmentId}` | `/runtime/tasks/{id}/attachments/{attachmentId}` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/tasks/{}/attachments/{}/content` | `/runtime/tasks/{taskId}/attachments/{attachmentId}/content` | `/runtime/tasks/{id}/attachments/{attachmentId}/content` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/tasks/{}/comments` | `/runtime/tasks/{taskId}/comments` | `/runtime/tasks/{id}/comments` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/tasks/{}/comments/{}` | `/runtime/tasks/{taskId}/comments/{commentId}` | `/runtime/tasks/{id}/comments/{commentId}` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/tasks/{}/events` | `/runtime/tasks/{taskId}/events` | `/runtime/tasks/{id}/events` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/tasks/{}/events/{}` | `/runtime/tasks/{taskId}/events/{eventId}` | `/runtime/tasks/{id}/events/{eventId}` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/tasks/{}/form` | `/runtime/tasks/{taskId}/form` | `/runtime/tasks/{id}/form` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/tasks/{}/identitylinks` | `/runtime/tasks/{taskId}/identitylinks` | `/runtime/tasks/{id}/identitylinks` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/tasks/{}/identitylinks/{}` | `/runtime/tasks/{taskId}/identitylinks/{family}` | `/runtime/tasks/{id}/identitylinks/{family}` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/tasks/{}/identitylinks/{}/{}` | `` | `/runtime/tasks/{id}/identitylinks/{family}/{identityId}` | 实现扩展；不计入官方覆盖 |
| `GET /runtime/tasks/{}/identitylinks/{}/{}/{}` | `/runtime/tasks/{taskId}/identitylinks/{family}/{identityId}/{type}` | `/runtime/tasks/{id}/identitylinks/{family}/{identityId}/{type}` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/tasks/{}/subtasks` | `/runtime/tasks/{taskId}/subtasks` | `/runtime/tasks/{id}/subtasks` | 原版任务HTTP场景通过：完整DTO、分页与动作副作用；完整字段组合待验收 |
| `GET /runtime/tasks/{}/variables` | `/runtime/tasks/{taskId}/variables` | `/runtime/tasks/{id}/variables` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/tasks/{}/variables/{}` | `/runtime/tasks/{taskId}/variables/{variableName}` | `/runtime/tasks/{id}/variables/{name}` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/tasks/{}/variables/{}/data` | `/runtime/tasks/{taskId}/variables/{variableName}/data` | `/runtime/tasks/{id}/variables/{name}/data` | 已编码；待契约与 Java 行为验收 |
| `GET /runtime/variable-instances` | `` | `/runtime/variable-instances` | 实现扩展；不计入官方覆盖 |
| `GET /runtime/variable-instances/{}/data` | `` | `/runtime/variable-instances/{id}/data` | 实现扩展；不计入官方覆盖 |
| `POST /form/form-data` | `/form/form-data` | `/form/form-data` | 已编码；待契约与 Java 行为验收 |
| `POST /history/historic-process-instances/delete` | `` | `/history/historic-process-instances/delete` | 实现扩展；不计入官方覆盖 |
| `POST /history/historic-process-instances/{}/comments` | `/history/historic-process-instances/{processInstanceId}/comments` | `/history/historic-process-instances/{id}/comments` | 已编码；待契约与 Java 行为验收 |
| `POST /identity/groups` | `/identity/groups` | `/identity/groups` | 已编码；待契约与 Java 行为验收 |
| `POST /identity/groups/{}/members` | `/identity/groups/{groupId}/members` | `/identity/groups/{id}/members` | 已编码；待契约与 Java 行为验收 |
| `POST /identity/roles` | `` | `/identity/roles` | 实现扩展；不计入官方覆盖 |
| `POST /identity/users` | `/identity/users` | `/identity/users` | 已编码；待契约与 Java 行为验收 |
| `POST /identity/users/{}/groups` | `` | `/identity/users/{id}/groups` | 实现扩展；不计入官方覆盖 |
| `POST /identity/users/{}/info` | `/identity/users/{userId}/info` | `/identity/users/{id}/info` | 已编码；待契约与 Java 行为验收 |
| `POST /management/deadletter-jobs/{}` | `/management/deadletter-jobs/{jobId}` | `/management/deadletter-jobs/{id}` | 已编码；待契约与 Java 行为验收 |
| `POST /management/engine-properties` | `` | `/management/engine-properties` | 实现扩展；不计入官方覆盖 |
| `POST /management/jobs/run` | `` | `/management/jobs/run` | 实现扩展；不计入官方覆盖 |
| `POST /management/jobs/{}` | `/management/jobs/{jobId}` | `/management/jobs/{id}` | 已编码；待契约与 Java 行为验收 |
| `POST /management/timer-jobs/{}` | `/management/timer-jobs/{jobId}` | `/management/timer-jobs/{id}` | 已编码；待契约与 Java 行为验收 |
| `POST /query/activity-instances` | `` | `/query/activity-instances` | 实现扩展；不计入官方覆盖 |
| `POST /query/executions` | `/query/executions` | `/query/executions` | 原版实例与执行HTTP场景通过：完整DTO、分页、启动及动作副作用；完整字段组合待验收 |
| `POST /query/historic-activity-instances` | `/query/historic-activity-instances` | `/query/historic-activity-instances` | 已编码；待契约与 Java 行为验收 |
| `POST /query/historic-detail` | `/query/historic-detail` | `/query/historic-detail` | 已编码；待契约与 Java 行为验收 |
| `POST /query/historic-process-instances` | `/query/historic-process-instances` | `/query/historic-process-instances` | 已编码；待契约与 Java 行为验收 |
| `POST /query/historic-task-instances` | `/query/historic-task-instances` | `/query/historic-task-instances` | 已编码；待契约与 Java 行为验收 |
| `POST /query/historic-variable-instances` | `/query/historic-variable-instances` | `/query/historic-variable-instances` | 已编码；待契约与 Java 行为验收 |
| `POST /query/process-instances` | `/query/process-instances` | `/query/process-instances` | 原版实例与执行HTTP场景通过：完整DTO、分页、启动及动作副作用；完整字段组合待验收 |
| `POST /query/tasks` | `/query/tasks` | `/query/tasks` | 原版任务HTTP场景通过：完整DTO、分页与动作副作用；完整字段组合待验收 |
| `POST /query/variable-instances` | `` | `/query/variable-instances` | 实现扩展；不计入官方覆盖 |
| `POST /repository/deployments` | `/repository/deployments` | `/repository/deployments` | 已编码；待契约与 Java 行为验收 |
| `POST /repository/models` | `/repository/models` | `/repository/models` | 已编码；待契约与 Java 行为验收 |
| `POST /repository/models/{}/deployment` | `` | `/repository/models/{id}/deployment` | 实现扩展；不计入官方覆盖 |
| `POST /repository/process-definitions/{}/batch-migrate` | `/repository/process-definitions/{processDefinitionId}/batch-migrate` | `/repository/process-definitions/{id}/batch-migrate` | 已编码；待契约与 Java 行为验收 |
| `POST /repository/process-definitions/{}/identitylinks` | `/repository/process-definitions/{processDefinitionId}/identitylinks` | `/repository/process-definitions/{id}/identitylinks` | 已编码；待契约与 Java 行为验收 |
| `POST /repository/process-definitions/{}/migrate` | `/repository/process-definitions/{processDefinitionId}/migrate` | `/repository/process-definitions/{id}/migrate` | 已编码；待契约与 Java 行为验收 |
| `POST /runtime/executions/{}/change-state` | `/runtime/executions/{executionId}/change-state` | `/runtime/executions/{id}/change-state` | 已编码；待契约与 Java 行为验收 |
| `POST /runtime/executions/{}/variables` | `/runtime/executions/{executionId}/variables` | `/runtime/executions/{id}/variables` | 已编码；待契约与 Java 行为验收 |
| `POST /runtime/executions/{}/variables-async` | `` | `/runtime/executions/{id}/variables-async` | 持久异步已验收：单执行者重试与恢复；Java原版HTTP场景通过；原版204与OpenAPI标记201不同 |
| `POST /runtime/messages` | `` | `/runtime/messages` | 实现扩展；不计入官方覆盖 |
| `POST /runtime/process-instances` | `/runtime/process-instances` | `/runtime/process-instances` | 原版实例与执行HTTP场景通过：完整DTO、分页、启动及动作副作用；完整字段组合待验收 |
| `POST /runtime/process-instances/delete` | `` | `/runtime/process-instances/delete` | 实现扩展；不计入官方覆盖 |
| `POST /runtime/process-instances/{}/attachments` | `` | `/runtime/process-instances/{id}/attachments` | 实现扩展；不计入官方覆盖 |
| `POST /runtime/process-instances/{}/change-state` | `/runtime/process-instances/{processInstanceId}/change-state` | `/runtime/process-instances/{id}/change-state` | 已编码；待契约与 Java 行为验收 |
| `POST /runtime/process-instances/{}/comments` | `` | `/runtime/process-instances/{id}/comments` | 实现扩展；不计入官方覆盖 |
| `POST /runtime/process-instances/{}/evaluate-conditions` | `/runtime/process-instances/{processInstanceId}/evaluate-conditions` | `/runtime/process-instances/{id}/evaluate-conditions` | 已编码；待契约与 Java 行为验收 |
| `POST /runtime/process-instances/{}/identitylinks` | `/runtime/process-instances/{processInstanceId}/identitylinks` | `/runtime/process-instances/{id}/identitylinks` | 已编码；待契约与 Java 行为验收 |
| `POST /runtime/process-instances/{}/inject` | `/runtime/process-instances/{processInstanceId}/inject` | `/runtime/process-instances/{id}/inject` | 已编码；待契约与 Java 行为验收 |
| `POST /runtime/process-instances/{}/migrate` | `/runtime/process-instances/{processInstanceId}/migrate` | `/runtime/process-instances/{id}/migrate` | 已编码；待契约与 Java 行为验收 |
| `POST /runtime/process-instances/{}/validate-migration` | `` | `/runtime/process-instances/{id}/validate-migration` | 实现扩展；不计入官方覆盖 |
| `POST /runtime/process-instances/{}/variables` | `/runtime/process-instances/{processInstanceId}/variables` | `/runtime/process-instances/{id}/variables` | 已编码；待契约与 Java 行为验收 |
| `POST /runtime/process-instances/{}/variables-async` | `` | `/runtime/process-instances/{id}/variables-async` | 持久异步已验收：单执行者重试与恢复；Java原版HTTP场景通过 |
| `POST /runtime/signals` | `/runtime/signals` | `/runtime/signals` | 已编码；待契约与 Java 行为验收 |
| `POST /runtime/tasks` | `/runtime/tasks` | `/runtime/tasks` | 原版任务HTTP场景通过：完整DTO、分页与动作副作用；完整字段组合待验收 |
| `POST /runtime/tasks/{}` | `/runtime/tasks/{taskId}` | `/runtime/tasks/{id}` | 原版任务HTTP场景通过：完整DTO、分页与动作副作用；完整字段组合待验收 |
| `POST /runtime/tasks/{}/attachments` | `/runtime/tasks/{taskId}/attachments` | `/runtime/tasks/{id}/attachments` | 已编码；待契约与 Java 行为验收 |
| `POST /runtime/tasks/{}/comments` | `/runtime/tasks/{taskId}/comments` | `/runtime/tasks/{id}/comments` | 已编码；待契约与 Java 行为验收 |
| `POST /runtime/tasks/{}/identitylinks` | `/runtime/tasks/{taskId}/identitylinks` | `/runtime/tasks/{id}/identitylinks` | 已编码；待契约与 Java 行为验收 |
| `POST /runtime/tasks/{}/variables` | `/runtime/tasks/{taskId}/variables` | `/runtime/tasks/{id}/variables` | 已编码；待契约与 Java 行为验收 |
| `PUT /identity/groups/{}` | `/identity/groups/{groupId}` | `/identity/groups/{id}` | 已编码；待契约与 Java 行为验收 |
| `PUT /identity/roles/{}` | `` | `/identity/roles/{id}` | 实现扩展；不计入官方覆盖 |
| `PUT /identity/users/{}` | `/identity/users/{userId}` | `/identity/users/{id}` | 已编码；待契约与 Java 行为验收 |
| `PUT /identity/users/{}/info/{}` | `/identity/users/{userId}/info/{key}` | `/identity/users/{id}/info/{key}` | 已编码；待契约与 Java 行为验收 |
| `PUT /identity/users/{}/password` | `` | `/identity/users/{id}/password` | 实现扩展；不计入官方覆盖 |
| `PUT /identity/users/{}/picture` | `/identity/users/{userId}/picture` | `/identity/users/{id}/picture` | 已编码；待契约与 Java 行为验收 |
| `PUT /identity/users/{}/roles` | `` | `/identity/users/{id}/roles` | 实现扩展；不计入官方覆盖 |
| `PUT /management/engine-properties/{}` | `` | `/management/engine-properties/{name}` | 实现扩展；不计入官方覆盖 |
| `PUT /repository/models/{}` | `/repository/models/{modelId}` | `/repository/models/{id}` | 已编码；待契约与 Java 行为验收 |
| `PUT /repository/models/{}/source` | `/repository/models/{modelId}/source` | `/repository/models/{id}/source` | 已编码；待契约与 Java 行为验收 |
| `PUT /repository/models/{}/source-extra` | `/repository/models/{modelId}/source-extra` | `/repository/models/{id}/source-extra` | 已编码；待契约与 Java 行为验收 |
| `PUT /repository/process-definitions/{}` | `/repository/process-definitions/{processDefinitionId}` | `/repository/process-definitions/{id}` | 已编码；待契约与 Java 行为验收 |
| `PUT /runtime/executions` | `/runtime/executions` | `/runtime/executions` | 已编码；待契约与 Java 行为验收 |
| `PUT /runtime/executions/{}` | `/runtime/executions/{executionId}` | `/runtime/executions/{id}` | 原版实例与执行HTTP场景通过：完整DTO、分页、启动及动作副作用；完整字段组合待验收 |
| `PUT /runtime/executions/{}/variables` | `/runtime/executions/{executionId}/variables` | `/runtime/executions/{id}/variables` | 已编码；待契约与 Java 行为验收 |
| `PUT /runtime/executions/{}/variables-async` | `` | `/runtime/executions/{id}/variables-async` | 持久异步已验收：单执行者重试与恢复；Java原版HTTP场景通过；原版204与OpenAPI标记201不同 |
| `PUT /runtime/executions/{}/variables-async/{}` | `` | `/runtime/executions/{id}/variables-async/{name}` | 持久异步已验收：单执行者重试与恢复；Java原版HTTP场景通过 |
| `PUT /runtime/executions/{}/variables/{}` | `/runtime/executions/{executionId}/variables/{variableName}` | `/runtime/executions/{id}/variables/{name}` | 已编码；待契约与 Java 行为验收 |
| `PUT /runtime/process-instances/{}` | `/runtime/process-instances/{processInstanceId}` | `/runtime/process-instances/{id}` | 原版实例与执行HTTP场景通过：完整DTO、分页、启动及动作副作用；完整字段组合待验收 |
| `PUT /runtime/process-instances/{}/variables` | `/runtime/process-instances/{processInstanceId}/variables` | `/runtime/process-instances/{id}/variables` | 已编码；待契约与 Java 行为验收 |
| `PUT /runtime/process-instances/{}/variables-async` | `` | `/runtime/process-instances/{id}/variables-async` | 持久异步已验收：单执行者重试与恢复；Java原版HTTP场景通过 |
| `PUT /runtime/process-instances/{}/variables-async/{}` | `` | `/runtime/process-instances/{id}/variables-async/{name}` | 持久异步已验收：单执行者重试与恢复；Java原版HTTP场景通过 |
| `PUT /runtime/process-instances/{}/variables/{}` | `/runtime/process-instances/{processInstanceId}/variables/{variableName}` | `/runtime/process-instances/{id}/variables/{name}` | 已编码；待契约与 Java 行为验收 |
| `PUT /runtime/tasks` | `` | `/runtime/tasks` | 实现扩展；不计入官方覆盖 |
| `PUT /runtime/tasks/{}` | `/runtime/tasks/{taskId}` | `/runtime/tasks/{id}` | 原版任务HTTP场景通过：完整DTO、分页与动作副作用；完整字段组合待验收 |
| `PUT /runtime/tasks/{}/variables/{}` | `/runtime/tasks/{taskId}/variables/{variableName}` | `/runtime/tasks/{id}/variables/{name}` | 已编码；待契约与 Java 行为验收 |
