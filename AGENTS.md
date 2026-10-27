# Flowable-go

- 每个函数需要注释说明，包括参数、返回值

```
// FuncName xxxxxx
// @param xxx yyyyyyy
// @param xxx yyyyyyy
// @return xxxxxxxx
```

- 复杂度>=3的地方加个行注释

```
// xxxx
```

- 在docs/下用 {三位流水号}-{描述}.md 的方式存放文档；可以根据业务创建子目录，命名规则相同。

```
./docs/001-xxx/001-xxxxxx.md
```

- 测试驱动开发，并且要求0.9的行覆盖率

- 注释、文档全部用中文

