# 004 GitHub Pages 自动发布

更新日期：2026-10-30

## 1. 发布入口

工作流位于 `.github/workflows/docs-pages.yml`。向 GitHub 仓库 `master` 分支推送后，自动构建 `docs-site` 并发布到 GitHub Pages；也可从 Actions 页面手动运行。手动选择其他分支时仅检查构建，不发布。

当前 GitHub 远端名称为 `gh`，地址为 `git@github.com:DengQingNian/Flowtide.git`。自建 Git 服务的 `origin` 推送不会触发 GitHub Actions；需要将提交推送至 `gh`。

仓库管理员首次需要在 **Settings → Pages → Build and deployment → Source** 选择 **GitHub Actions**。若 `github-pages` 环境设置了分支限制或审批规则，也需允许 `master` 发布并完成所需审批。

默认项目站点地址为 `https://dengqingnian.github.io/Flowtide/`；最终地址以部署任务输出为准。工作流读取 Pages 的 `base_path`，不固定仓库名称，使用自定义域名时也会随配置调整。

## 2. 构建和权限

流水线依次执行：

1. 检出全部 Git 历史，确保事实生成器的提交总数和最近提交准确。
2. 根据 `go.mod` 安装 Go，执行 `make cover`，生成未纳入版本控制的 `coverage.out` 并执行现有的 90% 覆盖率门禁。
3. 安装 Node.js 22，根据 `docs-site/package-lock.json` 执行 `npm ci`，缓存 npm 下载内容。
4. 执行文档站单元测试、新增部署路径处理代码的 90% 行覆盖率门禁。
5. 设置 `DOCS_BASE_PATH`，执行生产构建；构建前自动生成事实、页面、搜索索引并检查内部链接。
6. 复制 `dist/index.html` 为 `dist/404.html`，上传 `docs-site/dist`。
7. 构建成功后，由独立部署任务发布到 `github-pages` 环境。

构建任务只有仓库与 Pages 配置读取权限；部署任务使用 `pages: write` 和 `id-token: write`。工作流使用自带的 `GITHUB_TOKEN`，无需个人令牌。同一时间只允许一个发布任务运行，不取消已经开始的部署。

## 3. 路径与刷新

Vite 通过 `DOCS_BASE_PATH` 配置资源路径，Vue Router 通过 `import.meta.env.BASE_URL` 配置 history 基路径。Markdown 的正文由普通 HTML 链接组成，渲染时为站内绝对链接添加相同的部署目录，外链、页内锚点和协议相对链接不受影响。

GitHub Pages 不提供通用的单页应用重写规则，因此使用 `404.html` 加载应用并解析原始 URL。直接访问或刷新 `/Flowtide/docs/sdk/config` 能显示对应文档，但服务器仍返回 HTTP 404；此方式适合文档浏览，对搜索引擎收录有局限。如果需要深层链接始终返回 HTTP 200，后续应增加逐路由静态页面生成或改用支持重写规则的托管服务。

## 4. 本地验证

~~~bash
make cover
cd docs-site
npm ci
npm test
npm run test:deployment
npm run build
DOCS_BASE_PATH=/Flowtide/ npm run build
DOCS_BASE_PATH=/Flowtide/ npm run preview
~~~

通过 `http://localhost:4173/Flowtide/` 检查首页、导航、搜索、Markdown 正文链接及带锚点的跳转。本地 Vite 预览提供单页应用回退，不能用它的 HTTP 状态判断 GitHub Pages 的 404 行为。

部署路径单元测试覆盖根目录、仓库子目录、多层子目录、查询参数、中文锚点、首页链接、外链和空正文，以及路由生成的地址。

2026-10-30 本地验收结果：

- `make cover` 通过总量和核心包 90% 门禁。
- 文档站 5 个测试文件、21 个用例全部通过；新增部署路径处理函数行覆盖率为 100%。
- 根目录与 `/Flowtide/` 子目录生产构建均通过。
- 现有 Playwright 冒烟通过 9 个路由、搜索、主题、筛选、移动端与未知页面检查。
- 使用模拟 GitHub Pages 的静态服务验证子目录首页、资源、带查询参数和锚点的文档直达、正文链接、刷新后的 404 回退以及未知页面，均通过。
- `actionlint v1.7.12` 工作流静态检查通过。

以上为本地验证；GitHub 托管运行器上的首次部署仍需在仓库启用 Pages 并推送工作流后确认。

## 5. 参考

- [GitHub Pages 自定义工作流](https://docs.github.com/en/pages/getting-started-with-github-pages/using-custom-workflows-with-github-pages)
- [Vite 部署到 GitHub Pages](https://vite.dev/guide/static-deploy.html#github-pages)
