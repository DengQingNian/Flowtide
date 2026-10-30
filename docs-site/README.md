# Flowtide 文档站

Vue 3 + TypeScript + Naive UI 的静态文档站，内容聚焦“截至当前重构提交，引擎能做什么、不能做什么、下一步是什么”。

## 快速开始

~~~bash
cd docs-site
npm install
npm run dev          # http://localhost:5173
~~~

生产构建与预览：

~~~bash
npm run build        # vue-tsc + vite build，输出 dist/
npm run preview      # http://localhost:4173
~~~

构建前会自动运行 `npm run gen`，从仓库读取事实、渲染 Markdown、生成搜索索引并校验站内链接。

## 脚本

| 命令 | 作用 |
| --- | --- |
| `npm run gen` | 生成 `src/generated/facts.json` 与 `content.json` |
| `npm run dev` | 开发服务器（先执行一次 gen） |
| `npm run build` | 类型检查 + 生产构建 |
| `npm test` | Vitest 单元测试（内容、搜索、导航、组件） |
| `npm run test:e2e` | Playwright 冒烟测试；需先启动 preview |
| `npm run check:links` | 重新生成内容并执行站内链接/锚点校验 |
| `npm run check:facts` | 生成事实并校验关键不变量 |

E2E 需要 Chromium：

~~~bash
npx playwright install chromium
# Linux 还需要系统依赖；CI 可用 npx playwright install --with-deps chromium
npm run build
npm run preview &
BASE_URL=http://127.0.0.1:4173 npm run test:e2e
~~~

截图输出到 `screenshots/`，不参与生产构建。

## 目录

~~~text
docs-site/
  scripts/
    gen-facts.mjs      # 提交、代码、覆盖率、端点、能力矩阵、最近提交
    gen-content.mjs    # frontmatter、Markdown、Shiki、目录、搜索、链接校验
    verify-facts.mjs   # 事实不变量
    e2e.mjs            # Playwright 冒烟与截图
  src/
    content/           # Markdown 内容，按分区组织
    generated/         # 构建生成物，不提交
    components/        # 页面组件与 Markdown 周边组件
    layouts/           # LandingLayout / DocsLayout
    pages/             # 落地页、进度看板、能力矩阵、REST 浏览器、文档页
    lib/               # 内容索引、导航、搜索、主题
    styles/main.css    # 主题变量与响应式布局
  tests/               # Vitest
~~~

## 内容模型

每篇 Markdown 使用 frontmatter：

~~~yaml
---
title: 重构进度总览
description: 截至当前提交的规模、阶段与差距
order: 10
section: progress
status: prototype
since: v0.1.0
tags: [进度, 重构]
---
~~~

- `section` 决定侧栏分组与路由 `/docs/{section}/{slug}`；`index.md` 映射到 `/docs/{section}`。
- `status` 只允许 `verified`、`prototype`、`planned`。
- `order` 控制分组内顺序；`route` 可覆盖生成路由（如 `/releases`）。
- 站内链接统一使用绝对站点路径，例如 `/docs/getting-started/installation`。生成器会校验路由与锚点。

支持的 Markdown 扩展：表格横向滚动、`tip/info/warning/danger` 容器、双主题 Shiki 代码高亮、代码复制按钮、标题锚点。

## 事实来源

| 数据 | 来源 |
| --- | --- |
| 提交数、提交日期、最近提交 | `git` |
| Go 文件/行数、测试函数数 | 仓库扫描 |
| 覆盖率 | 根目录 `coverage.out`（先运行 `make cover` 可刷新） |
| 端点总数、分域、方法 | `docs/003-迁移审计/004-REST端点对照.md` |
| 能力矩阵 | `docs/003-迁移审计/003-能力矩阵与迁移退出条件.md` |
| ACT 表数 | `internal/persistence/schema/sql/mysql/001-flowable-8.1.sql` |

`npm run gen` 不修改仓库源码，只写 `src/generated/`。

## 站点信息架构

- `/`：落地页，定位、规模、阶段、能力、边界、代码入口。
- `/progress`：进度看板，规模、阶段、能力分布、迁移状态、最近提交、差距。
- `/compatibility`：可筛选能力矩阵，展开限制与责任职责。
- `/rest-explorer`：242 个方法端点的搜索、域/方法/来源筛选与分页。
- `/docs`：文档中心与全部页面索引。
- `/docs/...`：Markdown 文档。
- `/releases`：版本与路线图。

## 部署

`dist/` 是纯静态产物，可部署到任意静态托管。使用 history 路由，需要把未知路径回退到 `index.html`：

- Nginx：`try_files $uri $uri/ /index.html;`
- Netlify / Cloudflare Pages：配置 SPA fallback。
- GitHub/Gitea Pages：需要 404 回退或改为 hash 路由。

当前没有版本快照、英文版或 OpenAPI 浏览器。

## 限制

1. 内容在构建时静态生成，编辑 Markdown 后需要重新运行 `npm run gen` 或重启 dev。
2. 覆盖率取自 `coverage.out`，不是每次构建实时运行 `make cover`。
3. 能力矩阵与端点表来自仓库文档，文档自身漂移不会被站点自动发现。
4. E2E 与截图未纳入 CI；CI 只执行单元测试与生产构建。
5. 未做 Lighthouse、axe、跨浏览器与完整键盘可达性审计。
6. 没有版本化文档、i18n 内容与全文服务端搜索。
