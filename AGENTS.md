# Flowtide

- 保持通用引擎定位：代码、注释、测试、文档和提交说明不得包含接入方的公司名、项目名、数据库名、租户标识、内部路径或业务数据；接入记录保存在接入方工程。

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

- 阶段性任务完成commit代码

```
{fix/feat/perf/chore}({模块}): xxxxx

1、xxxx
2、yyyy
...
```

- 可用开发环境在.env
