import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const here = path.dirname(fileURLToPath(import.meta.url))
const facts = JSON.parse(fs.readFileSync(path.resolve(here, '../src/generated/facts.json'), 'utf8'))
const content = JSON.parse(fs.readFileSync(path.resolve(here, '../src/generated/content.json'), 'utf8'))

const problems = []

// check 记录不满足条件的事实。
// @param condition 断言条件
// @param message 失败说明
// @return 无
function check(condition, message) {
  if (!condition) problems.push(message)
}

check(facts.commit.count > 100, '提交数异常')
check(facts.stats.goFiles > 300, 'Go 文件数异常')
check(facts.coverage.percent >= 90 && facts.coverage.percent <= 100, '覆盖率不在预期区间')
check(facts.rest.total > facts.rest.officialCount, '端点总数应大于官方端点')
check(facts.rest.endpoints.length === facts.rest.total, '端点数组长度不一致')
check(facts.rest.officialCount === 184, '官方端点应为 184')
check(facts.rest.total === 242, '当前实现端点应为 242')
check(facts.capabilities.rows.length >= 15, '能力矩阵行数过少')
check(facts.stats.tables === 41, 'ACT 表数量应为 41')
check(content.pageCount >= 20, '文档页数少于 20')
check(content.pages.every((page) => page.route.startsWith('/')), '存在非法路由')
check(new Set(content.pages.map((page) => page.route)).size === content.pages.length, '存在重复路由')
check(content.pages.every((page) => page.html.length > 200), '存在过短页面')
check(content.pages.every((page) => page.headings.length > 0), '存在没有目录的页面')
check(content.search.length === content.pageCount, '搜索索引与页面数不一致')

if (problems.length > 0) {
  console.error('事实校验失败：')
  for (const problem of problems) console.error(' - ' + problem)
  process.exit(1)
}
console.log(`facts-check: ${content.pageCount} 页 / ${facts.rest.total} 端点 / 覆盖率 ${facts.coverage.percent}% 通过`)
