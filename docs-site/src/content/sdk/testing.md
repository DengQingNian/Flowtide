---
title: 测试与本地调试
description: 用内存引擎、临时状态文件和固定数据编写可重复的 SDK 测试。
order: 70
section: sdk
status: prototype
since: v0.1.0
tags: [SDK, 测试, 调试]
---

## 内存引擎

~~~go
func newTestEngine(t *testing.T) *flowable.Engine {
	t.Helper()
	app, err := flowable.New(flowable.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close(context.Background()) })
	return app
}
~~~

每个测试独立构造引擎，避免状态串扰。内存模式无持久化，适合快速反馈。

## 临时状态文件

~~~go
dir := t.TempDir()
app, err := flowable.New(flowable.Config{StatePath: filepath.Join(dir, "engine.json")})
~~~

<code>t.TempDir()</code> 自动清理。需要验证恢复时，关闭引擎后用同一路径重新构造。

## 固定 BPMN 与委托

- 把 BPMN 放在 <code>testdata/</code>，用相对路径读取。
- 委托用闭包捕获测试状态；注册名与 BPMN 精确匹配。
- 用表驱动覆盖正常、边界与错误路径。
- 断言任务、变量、历史与作业，不只断言无错误。

## 时间与 ID

仓库内测试可以注入内部时钟与 ID 生成器，获得确定性时间/ID。仓库外模块当前无法命名这些内部类型，因此：

- 尽量断言相对时间/非空，而不是固定 ID。
- 需要固定时间时，把时间逻辑放到委托或业务层。
- 不依赖 UUID 的具体值。

## 调试技巧

| 问题 | 方法 |
| --- | --- |
| 流程不推进 | 查执行树、活动实例、事件订阅与作业 |
| 任务不存在 | 查 assignee/候选人/租户与流程定义版本 |
| 变量丢失 | 查作用域、瞬态变量与历史级别 |
| 历史为空 | 检查 <code>HistoryLevel</code> |
| 重启后失败 | 委托/监听器是否重新注册 |

## 门禁

~~~bash
HOME=/root go test ./...
HOME=/root go test -race ./...
HOME=/root make check
~~~

<code>make check</code> 包含覆盖率、注释、vet、端点清单与独立 SDK 模块验收。新增行为应先写失败测试。

## 相关阅读

- [质量门禁](/docs/progress/quality)
- [SDK 持久化](/docs/sdk/persistence)
- [委托与监听器](/docs/sdk/delegates)
