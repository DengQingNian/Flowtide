# Flowtide 管理台

Flowtide 单二进制内嵌的管理界面，构建产物由 `internal/rest/adminpage` 通过 `go:embed` 打包，访问路径为 `/adminpage/#/`。

## 技术栈

- Vue 3 + TypeScript + Vite 7
- Naive UI 2.45（Notion 风格主题覆盖）
- vue-router 4（hash 模式，刷新与深链不依赖服务端回退）
- bpmn-js 18 + bpmn-js-properties-panel 5（设计器与只读查看器，独立懒加载）
- Vitest + @vue/test-utils + jsdom（单测）
- Playwright（e2e 冒烟）

## 开发

```bash
cd admin
npm ci
npm run dev        # http://localhost:5174，/service 代理到 8080
npm run build      # 类型检查 + 生产构建到 admin/dist
npm test           # Vitest + 覆盖率门禁
npm run test:e2e   # Playwright（需先构建服务端与管理台）
```

在仓库根目录：

```bash
make admin-build   # npm ci + 构建并复制到 internal/rest/adminpage/dist
make admin-test    # 前端单测与覆盖率
make admin-e2e     # 构建服务端后运行 Playwright 冒烟
make build         # 先构建管理台，再编译单二进制
```

## 目录

```text
src/
  api/          REST 客户端与按域拆分的接口封装
  components/   通用布局、表格、变量编辑器、表单、BPMN 图等
  design/       bpmn-js 设计器/查看器、中文翻译、最小 BPMN
  pages/        按业务域划分的页面
  router/       hash 路由与标题同步
  stores/       侧栏、主题、服务健康、认证提示
  lib/          格式化、下载、Prometheus、主题与 Naive 覆盖
  styles/       Notion 令牌与全局布局
tests/          Vitest 用例与 fetch 桩
e2e/            Playwright 冒烟
```

## 约定

- 界面只是 REST 客户端，不新增业务规则；错误以服务端 `{code,message,exception}` 原文为准。
- 不注入 `Authorization` 与 `X-Tenant-Id`，不保存任何凭据。
- 列表默认每页 50，最大 1000；筛选条件写入路由 query。
- 401/403 使用全局提示说明原因，页面与已填内容保留。
- 事故与删除操作必须二次确认；危险按钮采用低饱和红色。
- 样式只使用 `src/styles/main.css` 的 CSS 变量；圆角 6/8px，无渐变与重阴影。
- 函数使用中文注释与 `@param/@return`，复杂分支加行注释。

## 构建与嵌入

`make admin-build` 会把 `admin/dist` 复制到 `internal/rest/adminpage/dist`，Go 侧使用：

```go
//go:embed all:dist
var Assets embed.FS
```

干净仓库只有 `.keep`，此时访问 `/adminpage/` 会显示“管理台未构建”的占位页；正式发布必须执行 `make build`。

## 访问边界

本期不做登录、会话、RBAC 菜单与租户切换。服务端启用 Basic/JWT/mTLS 时管理台仍可打开，但数据请求返回 401，界面会给出说明；推荐仅部署在内网或前置反向代理注入认证。
