import fs from 'node:fs'
import path from 'node:path'
import { execFileSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'

const here = path.dirname(fileURLToPath(import.meta.url))
const root = path.resolve(here, '../..')
const outFile = path.resolve(here, '../src/generated/facts.json')

// readText 读取相对仓库根目录的文本文件。
// @param rel 相对路径
// @return 文件内容
function readText(rel) {
  return fs.readFileSync(path.join(root, rel), 'utf8')
}

// walk 递归列出目录中的文件，跳过构建产物与版本库。
// @param dir 绝对目录
// @param out 收集结果
// @return 文件绝对路径数组
function walk(dir, out = []) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (['.git', 'node_modules', 'dist', 'playwright-report', 'test-results'].includes(entry.name)) continue
    const full = path.join(dir, entry.name)
    if (entry.isDirectory()) walk(full, out)
    else out.push(full)
  }
  return out
}

// countLines 统计文本行数。
// @param file 绝对文件路径
// @return 行数
function countLines(file) {
  const text = fs.readFileSync(file, 'utf8')
  if (!text) return 0
  const lines = text.split('\n')
  return text.endsWith('\n') ? lines.length - 1 : lines.length
}

// git 执行只读 git 命令。
// @param args 命令参数
// @return 去尾换行的标准输出
function git(...args) {
  return execFileSync('git', args, { cwd: root, encoding: 'utf8' }).trim()
}

// parseCoverage 从 coverage.out 计算语句加权覆盖率。
// @param text coverage.out 内容
// @return 覆盖率、已覆盖语句数与总语句数
function parseCoverage(text) {
  let total = 0
  let covered = 0
  for (const line of text.split('\n')) {
    if (!line || line.startsWith('mode:')) continue
    const match = line.match(/^(.+):(\d+)\.(\d+),(\d+)\.(\d+) (\d+) (\d+)$/)
    if (!match) continue
    const statements = Number(match[6])
    const count = Number(match[7])
    total += statements
    if (count > 0) covered += statements
  }
  return {
    percent: total === 0 ? 0 : Math.round((covered / total) * 10000) / 100,
    covered,
    total,
  }
}

// parseRestTable 解析 004 端点对照表的行。
// @param text 对照 Markdown
// @return 端点、分域统计与方法统计
function parseRestTable(text) {
  const rowPattern = /^\| \`([A-Z]+) ([^\`]*)\` \| \`([^\`]*)\` \| \`([^\`]*)\` \| (.+) \|$/
  const endpoints = []
  for (const line of text.split('\n')) {
    const match = line.match(rowPattern)
    if (!match) continue
    const [, method, normalized, officialPath, implPath, status] = match
    const segments = normalized.split('/').filter(Boolean)
    const domain = segments[0] || 'root'
    const official = officialPath.trim() !== ''
    endpoints.push({
      id: endpoints.length + 1,
      method,
      path: normalized,
      officialPath,
      implPath,
      status: status.trim(),
      official,
      extension: !official,
      domain,
    })
  }
  const domains = {}
  const methods = {}
  for (const endpoint of endpoints) {
    domains[endpoint.domain] ??= { total: 0, official: 0, extension: 0 }
    domains[endpoint.domain].total += 1
    domains[endpoint.domain][endpoint.official ? 'official' : 'extension'] += 1
    methods[endpoint.method] = (methods[endpoint.method] || 0) + 1
  }
  const officialCount = endpoints.filter((item) => item.official).length
  return {
    total: endpoints.length,
    officialCount,
    extensionCount: endpoints.length - officialCount,
    domains,
    methods,
    endpoints,
  }
}

// splitTableRow 拆分 Markdown 表格行并去除首尾空单元。
// @param line 表格行
// @return 单元文本数组
function splitTableRow(line) {
  return line
    .split('|')
    .slice(1, -1)
    .map((cell) => cell.trim())
}

// parseCapabilityTable 解析 003 能力矩阵表。
// @param text 能力矩阵 Markdown
// @return 能力条目与发布状态分组
function parseCapabilityTable(text) {
  const lines = text.split('\n')
  const start = lines.findIndex((line) => line.startsWith('| 能力与计划范围 |'))
  if (start < 0) return { rows: [], levels: {} }
  const rows = []
  for (let index = start + 1; index < lines.length; index += 1) {
    const line = lines[index]
    if (!line.startsWith('|')) break
    if (/^\|\s*-+/.test(line)) continue
    const cells = splitTableRow(line)
    if (cells.length !== 7) continue
    const [capability, encoded, database, java, limits, owner, release] = cells
    let level = 'partial'
    if (release.includes('原型') || release.includes('实验验收') || release.includes('预检工具')) level = 'prototype'
    if (release.includes('有条件试用')) level = 'conditional'
    if (release.includes('最小门禁')) level = 'verified'
    if (release.includes('禁止') || release.includes('不可宣称')) level = 'restricted'
    rows.push({ capability, encoded, database, java, limits, owner, release, level })
  }
  const levels = {}
  for (const row of rows) levels[row.level] = (levels[row.level] || 0) + 1
  return { rows, levels }
}

// main 汇总仓库事实并写入 JSON。
// @return 无
function main() {
  const files = walk(root)
  const goFiles = files.filter((file) => file.endsWith('.go'))
  const goLines = goFiles.reduce((sum, file) => sum + countLines(file), 0)
  const docsFiles = files.filter((file) => file.startsWith(path.join(root, 'docs') + path.sep) && file.endsWith('.md'))
  const docsLines = docsFiles.reduce((sum, file) => sum + countLines(file), 0)
  const tests = goFiles
    .filter((file) => file.endsWith('_test.go'))
    .reduce((sum, file) => sum + (fs.readFileSync(file, 'utf8').match(/^func (?:Test|Benchmark|Fuzz)\w*\(/gm) || []).length, 0)

  const coverage = parseCoverage(readText('coverage.out'))
  const rest = parseRestTable(readText('docs/003-迁移审计/004-REST端点对照.md'))
  const capabilities = parseCapabilityTable(readText('docs/003-迁移审计/003-能力矩阵与迁移退出条件.md'))
  const schema = readText('internal/persistence/schema/sql/mysql/001-flowable-8.1.sql')
  const tables = (schema.match(/create table/gi) || []).length
  const versionMatch = readText('internal/engine/engine.go').match(/const Version = "([^"]+)"/)

  const recentCommits = git('log', '-12', '--date=short', '--pretty=format:%h%x1f%ad%x1f%s')
    .split('\n')
    .filter(Boolean)
    .map((line) => {
      const [hash, date, subject] = line.split('\x1f')
      return { hash, date, subject }
    })

  const facts = {
    generatedAt: new Date().toISOString(),
    version: versionMatch ? versionMatch[1] : '0.1.0',
    commit: {
      hash: git('rev-parse', 'HEAD'),
      short: git('rev-parse', '--short', 'HEAD'),
      date: git('log', '-1', '--date=short', '--pretty=format:%ad'),
      subject: git('log', '-1', '--pretty=format:%s'),
      count: Number(git('rev-list', '--count', 'HEAD')),
      branch: git('rev-parse', '--abbrev-ref', 'HEAD'),
    },
    stats: {
      goFiles: goFiles.length,
      goLines,
      docsFiles: docsFiles.length,
      docsLines,
      tests,
      tables,
      binaries: 2,
    },
    coverage,
    rest,
    capabilities,
    recentCommits,
    status: {
      phase: 'BPMN 内核扩展与迁移工具阶段',
      migration: '新建库原型；停写切换与回退未验收',
      deployment: '单执行者；禁止多节点共享状态源',
      javaBaseline: 'f6017e32a4bdd3e59a8881f58b0ee181652d1d9e',
      schemaVersion: '8.1.0.3',
    },
  }

  fs.mkdirSync(path.dirname(outFile), { recursive: true })
  fs.writeFileSync(outFile, JSON.stringify(facts, null, 2) + '\n')
  console.log(
    `facts: ${facts.commit.short} · ${facts.stats.goFiles} Go 文件 / ${facts.stats.goLines} 行 · ${facts.rest.officialCount}/${facts.rest.total} 官方端点 · 覆盖率 ${facts.coverage.percent}%`,
  )
}

main()
