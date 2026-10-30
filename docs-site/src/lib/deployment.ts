// withBaseLinks 为 Markdown 正文中的站内绝对链接添加部署目录。
// @param html 生成器渲染的 Markdown 正文
// @param base Vite 提供的部署目录，包含首尾斜杠
// @return 已适配部署目录的正文，外链与页内锚点保持原样
export function withBaseLinks(html: string, base: string): string {
  return html.replace(/href="\/(?!\/)([^"]*)"/g, `href="${base}$1"`)
}
