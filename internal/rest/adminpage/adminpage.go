// Package adminpage 内嵌并挂载管理台前端静态资源。
package adminpage

import (
	"bytes"
	"embed"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

// Prefix 是管理台占用的 URL 前缀。
const Prefix = "/adminpage"

// contentSecurityPolicy 是管理台统一内容安全策略。
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'"

// pageNotBuilt 是构建产物缺失时展示的占位页。
const pageNotBuilt = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>管理台未构建</title>
<style>body{font-family:system-ui,sans-serif;max-width:42rem;margin:4rem auto;padding:0 1rem;line-height:1.7;color:#222}code{background:#f2f2f2;padding:.15rem .35rem;border-radius:.25rem}</style>
</head>
<body>
<h1>管理台未构建</h1>
<p>当前二进制只包含占位文件，还没有嵌入管理台前端产物。</p>
<p>请在仓库根目录执行 <code>make admin-build</code> 构建并复制前端资源，然后重新编译服务端。</p>
<p>构建完成后访问 <a href="/adminpage/">/adminpage/</a> 即可打开管理台。</p>
</body>
</html>
`

// pageNotFound 是管理台资源缺失时的友好错误页。
const pageNotFound = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>管理台资源不存在</title>
<style>body{font-family:system-ui,sans-serif;max-width:42rem;margin:4rem auto;padding:0 1rem;line-height:1.7;color:#222}</style>
</head>
<body>
<h1>管理台资源不存在</h1>
<p>请求的静态资源不存在或已被新的构建产物替换，请检查地址是否正确。</p>
<p>如果管理台刚升级，请刷新页面；开发环境请重新执行 <code>make admin-build</code>。</p>
<p><a href="/adminpage/">返回管理台首页</a></p>
</body>
</html>
`

// pageMethodNotAllowed 是不支持的方法返回的提示页。
const pageMethodNotAllowed = `<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><title>请求方法不受支持</title></head>
<body><h1>请求方法不受支持</h1><p>管理台静态资源只接受 GET 与 HEAD 请求。</p><p><a href="/adminpage/">返回管理台首页</a></p></body>
</html>
`

// Assets 是编译期嵌入的管理台构建产物，仅包含 dist 目录。
//
//go:embed all:dist
var Assets embed.FS

// RedirectRoot 将精确根路径重定向到管理台首页。
// @param w 响应写入器
// @param r 请求
func RedirectRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, Prefix+"/", http.StatusFound)
}

// RedirectSlash 将缺少斜杠的管理台前缀永久重定向到管理台首页，并保留查询串。
// @param w 响应写入器
// @param r 请求
func RedirectSlash(w http.ResponseWriter, r *http.Request) {
	target := Prefix + "/"
	// 保留查询串，避免深链参数在补斜杠时丢失
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

// handler 提供管理台静态资源、SPA 回退与占位页。
type handler struct {
	fsys fs.FS // 静态资源根文件系统
}

// NewHandler 基于给定文件系统创建管理台处理器，便于测试注入内存文件系统。
// 当文件系统包含 dist/index.html 时自动以 dist 为根，兼容 go:embed all:dist 的目录布局。
// @param fsys 静态资源文件系统，通常为 Assets 或 fstest.MapFS
// @return 管理台 HTTP 处理器
func NewHandler(fsys fs.FS) http.Handler {
	// 嵌入文件系统的根是包目录，构建产物实际位于 dist 子目录
	if root, ok := distRoot(fsys); ok {
		return &handler{fsys: root}
	}
	return &handler{fsys: fsys}
}

// distRoot 判断文件系统是否包含 dist/index.html 并返回对应的子树。
// @param fsys 静态资源文件系统
// @return 以 dist 为根的文件系统
// @return 是否命中构建产物目录
func distRoot(fsys fs.FS) (fs.FS, bool) {
	root, err := fs.Sub(fsys, "dist")
	// 没有 dist 目录说明调用方直接传入了资源根
	if err != nil {
		return nil, false
	}
	// 只有 index.html 存在才视为已构建，避免把占位文件当成产物根
	if _, err := fs.Stat(root, "index.html"); err != nil {
		return nil, false
	}
	return root, true
}

// ServeHTTP 按管理台规则处理静态资源、SPA 回退与缺失资源。
// @param w 响应写入器
// @param r 请求
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w)
	// 路由层只注册 GET，独立使用时同样拒绝其他方法
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeHTML(w, r, http.StatusMethodNotAllowed, pageMethodNotAllowed)
		return
	}
	// ServeMux 会自动补斜杠，独立挂载时保持同样的跳转行为
	if r.URL.Path == Prefix {
		RedirectSlash(w, r)
		return
	}
	relative, ok := relativePath(r.URL.Path)
	// 前缀不匹配、目录遍历或非法分隔符一律按缺失页处理，避免泄露文件系统信息
	if !ok {
		writeHTML(w, r, http.StatusNotFound, pageNotFound)
		return
	}
	// 根路径与无扩展名路径都交给前端哈希路由处理
	if relative == "" || path.Ext(relative) == "" {
		h.serveIndex(w, r)
		return
	}
	h.serveFile(w, r, relative)
}

// serveIndex 返回 index.html；产物缺失时返回可操作的占位页。
// @param w 响应写入器
// @param r 请求
func (h *handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	content, err := fs.ReadFile(h.fsys, "index.html")
	// 只有 .keep 或 index.html 缺失说明尚未构建，返回 200 占位页而不是错误
	if err != nil {
		writeHTML(w, r, http.StatusOK, pageNotBuilt)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	serveBytes(w, r, content)
}

// serveFile 返回带扩展名的静态资源，缺失时返回中文 404 页面。
// @param w 响应写入器
// @param r 请求
// @param name 相对资源路径
func (h *handler) serveFile(w http.ResponseWriter, r *http.Request, name string) {
	content, err := fs.ReadFile(h.fsys, name)
	// 读取失败包含不存在、是目录等情况，统一按缺失资源处理
	if err != nil {
		writeHTML(w, r, http.StatusNotFound, pageNotFound)
		return
	}
	contentType := mime.TypeByExtension(path.Ext(name))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	// assets 下的文件名带内容指纹，可以长期缓存；其他文件保持不缓存
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	serveBytes(w, r, content)
}

// setSecurityHeaders 写入管理台统一安全响应头。
// @param w 响应写入器
func setSecurityHeaders(w http.ResponseWriter) {
	header := w.Header()
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Referrer-Policy", "same-origin")
	header.Set("Content-Security-Policy", contentSecurityPolicy)
}

// writeHTML 输出指定状态码的 HTML 页面，HEAD 请求只返回头部。
// @param w 响应写入器
// @param r 请求
// @param status HTTP 状态码
// @param page HTML 内容
func writeHTML(w http.ResponseWriter, r *http.Request, status int, page string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	// HEAD 请求不写响应体，保持与 GET 相同的头部
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.WriteString(w, page)
}

// serveBytes 输出内存中的响应内容，兼容 HEAD 与范围请求语义。
// @param w 响应写入器
// @param r 请求
// @param content 响应内容
func serveBytes(w http.ResponseWriter, r *http.Request, content []byte) {
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(content))
}

// relativePath 校验并规范化管理台相对路径，阻止目录遍历与跨平台绝对路径。
// @param requestPath 请求路径
// @return 相对资源路径
// @return 路径是否合法
func relativePath(requestPath string) (string, bool) {
	if !strings.HasPrefix(requestPath, Prefix+"/") {
		return "", false
	}
	rest := strings.TrimPrefix(requestPath, Prefix+"/")
	// 管理台根路径对应 index.html
	if rest == "" {
		return "", true
	}
	// 双斜杠、反斜杠与 NUL 字节可能绕过路径校验或造成跨平台逃逸
	if strings.HasPrefix(rest, "/") || strings.ContainsAny(rest, "\\\x00") {
		return "", false
	}
	// Windows 盘符写法属于绝对路径，直接拒绝
	if len(rest) >= 2 && rest[1] == ':' {
		return "", false
	}
	// 任一 .. 段都视为目录遍历尝试，即使规范化后会落在根目录内
	for _, segment := range strings.Split(rest, "/") {
		if segment == ".." {
			return "", false
		}
	}
	cleaned := strings.TrimPrefix(path.Clean("/"+rest), "/")
	// fs.ValidPath 再次保证结果是相对、干净且不含点段或空段
	if cleaned == "." || !fs.ValidPath(cleaned) {
		return "", false
	}
	return cleaned, true
}
