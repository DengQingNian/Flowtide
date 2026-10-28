import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import matter from 'gray-matter'
import MarkdownIt from 'markdown-it'
import anchor from 'markdown-it-anchor'
import container from 'markdown-it-container'
import { createHighlighter } from 'shiki'

const here = path.dirname(fileURLToPath(import.meta.url))
const contentDir = path.resolve(here, '../src/content')
const outFile = path.resolve(here, '../src/generated/content.json')
const factsFile = path.resolve(here, '../src/generated/facts.json')

const specialRoutes = new Set(['/', '/docs', '/progress', '/compatibility', '/rest-explorer', '/releases', '/404'])

const languageLabels = {
  go: 'Go',
  bash: 'Shell',
  sh: 'Shell',
  shell: 'Shell',
  json: 'JSON',
  jsonc: 'JSON',
  xml: 'BPMN XML',
  yaml: 'YAML',
  yml: 'YAML',
  sql: 'SQL',
  http: 'HTTP',
  diff: 'Diff',
  text: 'Text',
  ini: 'INI',
}

const calloutTitles = {
  tip: '提示',
  info: '说明',
  warning: '边界',
  danger: '警告',
}

// walk 递归列出 Markdown 内容文件。
// @param dir 绝对目录
// @param out 收集结果
// @return 文件绝对路径数组
function walk(dir, out = []) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name)
    if (entry.isDirectory()) walk(full, out)
    else if (entry.name.endsWith('.md')) out.push(full)
  }
  return out
}

// slugify 生成标题锚点，保留中文并移除标点。
// @param value 标题文本
// @return 锚点 ID
function slugify(value) {
  return value
    .trim()
    .toLowerCase()
    .replace(/[\s]+/g, '-')
    .replace(/[^\p{L}\p{N}_-]/gu, '')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '')
}

// escapeHtml 转义无法语法高亮的代码。
// @param value 原始文本
// @return 转义后的 HTML
function escapeHtml(value) {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

// parseFenceInfo 解析代码围栏语言与标题。
// @param info 围栏信息串
// @return 语言与可选标题
function parseFenceInfo(info) {
  const trimmed = info.trim()
  const match = trimmed.match(/^(\S+)?\s*(?:title="([^"]*)")?\s*(?:\{([^}]*)\})?/)
  const lang = (match?.[1] || 'text').toLowerCase()
  const title = match?.[2] || ''
  return { lang, title }
}

// stripHtml 将渲染结果压缩为可搜索文本。
// @param html 渲染后的 HTML
// @return 纯文本
function stripHtml(html) {
  return html
    .replace(/<script[\s\S]*?<\/script>/gi, ' ')
    .replace(/<style[\s\S]*?<\/style>/gi, ' ')
    .replace(/<[^>]+>/g, ' ')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    .replace(/&amp;/g, '&')
    .replace(/\s+/g, ' ')
    .trim()
}

// renderMarkdown 配置 Markdown 渲染器：锚点、提示块、表格包裹与双主题高亮。
// @param highlighter Shiki 高亮器
// @return MarkdownIt 实例
function renderMarkdown(highlighter) {
  const md = new MarkdownIt({
    html: true,
    linkify: false,
    typographer: false,
    breaks: false,
  })

  md.use(anchor, {
    slugify,
    level: [2, 3],
    tabIndex: false,
  })

  for (const [type, title] of Object.entries(calloutTitles)) {
    md.use(container, type, {
      validate: (params) => params.trim().split(/\s+/)[0] === type,
      render(tokens, idx) {
        const token = tokens[idx]
        if (token.nesting === 1) {
          const custom = token.info.trim().slice(type.length).trim()
          return `<div class="callout callout-${type}"><div class="callout-title">${escapeHtml(custom || title)}</div><div class="callout-body">\n`
        }
        return '</div></div>\n'
      },
    })
  }

  md.renderer.rules.fence = (tokens, idx) => {
    const token = tokens[idx]
    const { lang, title } = parseFenceInfo(token.info)
    const label = languageLabels[lang] || lang.toUpperCase()
    const code = token.content
    let highlighted
    try {
      highlighted = highlighter.codeToHtml(code, {
        lang: lang === 'sh' || lang === 'shell' ? 'bash' : lang,
        themes: { light: 'github-light', dark: 'github-dark' },
        defaultColor: false,
      })
    } catch {
      highlighted = `<pre class="shiki"><code>${escapeHtml(code)}</code></pre>`
    }
    return `<div class="code-block" data-lang="${escapeHtml(lang)}">
<div class="code-head"><span class="code-lang">${escapeHtml(label)}</span>${title ? `<span class="code-title">${escapeHtml(title)}</span>` : ''}<button class="copy-btn" type="button" aria-label="复制代码">复制</button></div>
${highlighted}
</div>\n`
  }

  md.renderer.rules.table_open = () => '<div class="table-wrap"><table>\n'
  md.renderer.rules.table_close = () => '</table></div>\n'

  const defaultLinkOpen = md.renderer.rules.link_open || ((tokens, idx, options, _env, self) => self.renderToken(tokens, idx, options))
  md.renderer.rules.link_open = (tokens, idx, options, env, self) => {
    const href = tokens[idx].attrGet('href') || ''
    if (/^https?:\/\//.test(href)) {
      tokens[idx].attrSet('target', '_blank')
      tokens[idx].attrSet('rel', 'noopener noreferrer')
    }
    return defaultLinkOpen(tokens, idx, options, env, self)
  }

  return md
}

// validateFrontmatter 校验页面元数据。
// @param data 解析后的 frontmatter
// @param file 源文件路径
// @return 无；不合法时抛错
function validateFrontmatter(data, file) {
  const relative = path.relative(contentDir, file)
  if (!data.title || typeof data.title !== 'string') throw new Error(`${relative}: 缺少 title`)
  if (!data.description || typeof data.description !== 'string') throw new Error(`${relative}: 缺少 description`)
  if (typeof data.order !== 'number') throw new Error(`${relative}: order 必须是数字`)
  if (!['verified', 'prototype', 'planned'].includes(data.status)) {
    throw new Error(`${relative}: status 必须是 verified/prototype/planned`)
  }
}

// collectHeadings 从解析后的 token 中提取二三级标题。
// @param tokens Markdown token 数组
// @return 标题数组
function collectHeadings(tokens) {
  const headings = []
  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index]
    if (token.type !== 'heading_open') continue
    const level = Number(token.tag.slice(1))
    if (level !== 2 && level !== 3) continue
    const inline = tokens[index + 1]
    const text = inline ? inline.content.trim() : ''
    const id = token.attrGet('id') || slugify(text)
    headings.push({ level, text, id })
  }
  return headings
}

// checkInternalLinks 校验站内路由与锚点。
// @param pages 页面数组
// @return 无；发现断链时抛错
function checkInternalLinks(pages) {
  const routeMap = new Map(pages.map((page) => [page.route, page]))
  const errors = []
  for (const page of pages) {
    for (const match of page.html.matchAll(/href="([^"]+)"/g)) {
      const href = match[1]
      if (!href.startsWith('/') || href.startsWith('//')) continue
      const [pathname, hash] = href.split('#')
      const target = routeMap.get(pathname) || (specialRoutes.has(pathname) ? null : undefined)
      if (target === undefined) {
        errors.push(`${page.route}: 断链 ${href}`)
        continue
      }
      if (hash && target) {
        const ids = new Set(target.headings.map((heading) => heading.id))
        if (!ids.has(hash)) errors.push(`${page.route}: 锚点不存在 ${href}`)
      }
    }
  }
  if (errors.length > 0) {
    throw new Error('站内链接校验失败：\n' + errors.map((item) => ' - ' + item).join('\n'))
  }
}

// main 生成页面 HTML、目录与搜索索引。
// @return 无
async function main() {
  if (!fs.existsSync(contentDir)) throw new Error('缺少 src/content 目录')
  const facts = JSON.parse(fs.readFileSync(factsFile, 'utf8'))
  const highlighter = await createHighlighter({
    themes: ['github-light', 'github-dark'],
    langs: ['go', 'bash', 'json', 'xml', 'yaml', 'sql', 'http', 'diff', 'ini'],
  })
  const md = renderMarkdown(highlighter)
  const pages = []

  for (const file of walk(contentDir).sort()) {
    const relative = path.relative(contentDir, file)
    const raw = fs.readFileSync(file, 'utf8')
    const parsed = matter(raw)
    validateFrontmatter(parsed.data, file)

    const parts = relative.split(path.sep)
    const section = parsed.data.section || (parts.length > 1 ? parts[0] : 'docs')
    const slug = path.basename(file, '.md')
    const route = parsed.data.route || (slug === 'index' ? `/docs/${section}` : `/docs/${section}/${slug}`)
    if (pages.some((page) => page.route === route)) throw new Error(`重复路由：${route}`)

    const env = {}
    const tokens = md.parse(parsed.content, env)
    const html = md.renderer.render(tokens, md.options, env)
    const headings = collectHeadings(tokens)
    const text = stripHtml(html)
    const readingMinutes = Math.max(1, Math.round(text.length / 500 + (html.match(/<code/g) || []).length / 10))

    pages.push({
      key: relative.replace(/\.md$/, ''),
      route,
      section,
      slug,
      title: parsed.data.title,
      description: parsed.data.description,
      order: parsed.data.order,
      status: parsed.data.status,
      since: parsed.data.since || 'v0.1.0',
      tags: Array.isArray(parsed.data.tags) ? parsed.data.tags : [],
      verifiedCommit: parsed.data.verifiedCommit || facts.commit.short,
      sourcePath: `docs-site/src/content/${relative.split(path.sep).join('/')}`,
      html,
      headings,
      text,
      readingMinutes,
    })
  }

  if (pages.length === 0) throw new Error('没有找到任何文档页面')
  checkInternalLinks(pages)

  pages.sort((a, b) => a.section.localeCompare(b.section) || a.order - b.order || a.title.localeCompare(b.title))

  const output = {
    generatedAt: new Date().toISOString(),
    commit: facts.commit.short,
    pageCount: pages.length,
    pages: pages.map(({ text, ...page }) => page),
    search: pages.map((page) => ({
      id: page.route,
      route: page.route,
      section: page.section,
      title: page.title,
      description: page.description,
      headings: page.headings.map((heading) => heading.text),
      text: page.text.slice(0, 6000),
    })),
  }

  fs.mkdirSync(path.dirname(outFile), { recursive: true })
  fs.writeFileSync(outFile, JSON.stringify(output) + '\n')
  console.log(`content: ${pages.length} 页 · ${output.search.reduce((sum, page) => sum + page.text.length, 0)} 字索引`)
  await highlighter.dispose()
}

main().catch((error) => {
  console.error(error instanceof Error ? error.message : error)
  process.exit(1)
})
