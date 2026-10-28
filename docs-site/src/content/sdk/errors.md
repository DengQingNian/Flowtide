---
title: 错误处理
description: api.Error、哨兵错误、错误码包装与委托/作业中的错误传播。
order: 65
section: sdk
status: prototype
since: v0.1.0
tags: [SDK, 错误, 异常]
---

## 错误类型

~~~go
type Error struct {
	Code    string
	Message string
	Cause   error
}
~~~

<code>Error</code> 实现 <code>error</code> 与 <code>Unwrap</code>，可用 <code>errors.As</code> 取错误码、<code>errors.Is</code> 判断哨兵。

## 哨兵错误

| 哨兵 | 含义 |
| --- | --- |
| <code>api.ErrNotFound</code> | 资源不存在 |
| <code>api.ErrConflict</code> | 并发/状态冲突 |
| <code>api.ErrBadRequest</code> | 参数非法 |
| <code>api.ErrUnauthorized</code> | 未认证 |
| <code>api.ErrForbidden</code> | 无权限 |
| <code>api.ErrTooManyRequests</code> | 请求过多 |
| <code>api.ErrEngineClosed</code> | 引擎已关闭 |

## 创建错误

~~~go
return api.NewError("ENGINE-0510", "服务任务委托未注册", nil)
~~~

第三个参数是底层原因，可为空。错误码应稳定，说明可读；不要用错误说明做分支。

## 处理示例

~~~go
_, err := app.RuntimeService().GetProcessInstance(ctx, id)
if err != nil {
	var engineErr *api.Error
	if errors.As(err, &engineErr) {
		log.Printf("code=%s message=%s", engineErr.Code, engineErr.Message)
	}
	if errors.Is(err, api.ErrNotFound) {
		// 返回 404 或业务默认值
	}
	return err
}
~~~

## 委托与作业

- 委托返回 error 会使命令回滚；异步作业按退避重试。
- 返回 <code>*api.Error</code> 时错误码进入作业异常栈与管理响应。
- 外部调用失败不会自动回滚外部系统；使用幂等键或补偿。
- 死信作业可用管理服务重试、移动或删除。

## 相关阅读

- [错误码参考](/docs/reference/errors)
- [委托与监听器](/docs/sdk/delegates)
- [排障手册](/docs/operations/troubleshooting)
