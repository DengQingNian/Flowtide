# 028 REST 兼容补充：组更新、批量任务、执行变量与嵌套资源

> 文档编号：002-028  
> 更新日期：2026-10-28  
> 对应计划：011 REST API 与兼容契约；008 任务、身份与表单

## 1. 背景

本期补齐 4 个官方路径/行为：

- `PUT /identity/groups/{id}`
- `PUT /runtime/tasks`（批量更新）
- `PUT /runtime/executions/{id}/variables`（批量更新）
- `GET /repository/deployments/{id}/resources/**`（嵌套资源名）

## 2. 端点

### 2.1 更新组

`PUT /service/identity/groups/{id}`

~~~json
{"name": "New Team", "type": "security", "tenantId": "t1"}
~~~

- 路径 ID 为准；请求体 `id` 非空且不一致返回 400。
- 组不存在返回 404。
- 成功返回更新后的组。

### 2.2 批量更新任务

`PUT /service/runtime/tasks`

~~~json
{
  "taskIds": ["task-1", "task-2"],
  "name": "Bulk",
  "assignee": "alice",
  "priority": 60,
  "dueDate": "2026-11-01T00:00:00Z"
}
~~~

- 与单任务更新共享字段解析（`name/description/assignee/owner/category/formKey/priority/dueDate`）。
- `taskIds` 缺失、为 null、非数组或空数组返回 400。
- 任一任务不存在返回 404，已完成的任务不会被部分更新（按顺序执行，前面已更新的任务不回滚）。
- 成功返回 `{data,total,start,size}` 分页结构，`data` 为更新后的任务。

### 2.3 批量更新执行变量

`PUT /service/runtime/executions/{id}/variables`

- 请求体为变量数组或单对象，语义与实例变量批量更新一致。
- 成功返回 200 与更新后的变量列表。
- 空体/非法 JSON 返回 400；执行不存在返回 404。

### 2.4 嵌套部署资源

`GET /service/repository/deployments/{id}/resources/{name}`

- 路由改用 `{name...}` 通配，支持 `forms/leave.bpmn20.xml` 等含斜杠资源名。
- 资源不存在返回 404。

## 3. 限制

1. 批量任务更新不是原子操作，失败前的任务已更新。
2. 批量任务响应为分页结构，与 Flowable `DataResponse` 一致，但未实现排序参数。
3. 嵌套资源名按原样匹配，不做 URL 解码后的路径清理。
4. 组更新不校验租户（与现有身份端点一致）。

## 4. 测试

- `internal/engine/group_update_test.go`：组更新、缺失、空 ID。
- `internal/rest/parity_extra_test.go`：组更新、批量任务、执行变量、嵌套资源与错误路径。
- 覆盖率：`internal/engine` 90.7%，`internal/rest` 90.7%。
