// Prometheus 文本指标解析：只处理展示所需的样本与标签。

/** 单个指标样本 */
export interface MetricSample {
  /** 指标名 */
  name: string
  /** 标签键值 */
  labels: Record<string, string>
  /** 数值 */
  value: number
}

/** 指标分组 */
export interface MetricGroup {
  /** 指标名 */
  name: string
  /** 同指标样本 */
  samples: MetricSample[]
}

// parseLabels 解析 {k="v",k2="v2"} 标签串。
// @param text 标签串
// @return 标签键值
export function parseLabels(text: string): Record<string, string> {
  const labels: Record<string, string> = {}
  const pattern = /([a-zA-Z_][a-zA-Z0-9_]*)\s*=\s*"((?:[^"\\]|\\.)*)"/g
  let match: RegExpExecArray | null
  while ((match = pattern.exec(text)) !== null) {
    labels[match[1]] = match[2].replace(/\\(.)/g, '$1')
  }
  return labels
}

// parseMetrics 解析 Prometheus 文本格式。
// @param text 指标文本
// @return 样本列表，无法解析的行忽略
export function parseMetrics(text: string): MetricSample[] {
  const samples: MetricSample[] = []
  for (const rawLine of text.split('\n')) {
    const line = rawLine.trim()
    if (!line || line.startsWith('#')) continue
    const match = /^([a-zA-Z_:][a-zA-Z0-9_:]*)(\{[^}]*\})?\s+(-?(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?)/.exec(line)
    if (!match) continue
    const value = Number(match[3])
    if (!Number.isFinite(value)) continue
    samples.push({ name: match[1], labels: match[2] ? parseLabels(match[2]) : {}, value })
  }
  return samples
}

// groupMetrics 按指标名分组。
// @param samples 样本列表
// @return 分组列表，按名称排序
export function groupMetrics(samples: MetricSample[]): MetricGroup[] {
  const map = new Map<string, MetricSample[]>()
  for (const sample of samples) {
    const list = map.get(sample.name)
    if (list) list.push(sample)
    else map.set(sample.name, [sample])
  }
  return [...map.entries()]
    .map(([name, list]) => ({ name, samples: list }))
    .sort((a, b) => a.name.localeCompare(b.name))
}

// metricLabelText 将标签渲染为 k=v 文本。
// @param labels 标签键值
// @return 标签文本
export function metricLabelText(labels: Record<string, string>): string {
  const entries = Object.entries(labels)
  if (entries.length === 0) return '—'
  return entries.map(([key, value]) => `${key}=${value}`).join(', ')
}

// sumByPrefix 汇总指定前缀的样本值。
// @param samples 样本列表
// @param prefix 指标名前缀
// @return 汇总值
export function sumByPrefix(samples: MetricSample[], prefix: string): number {
  return samples.filter((sample) => sample.name.startsWith(prefix)).reduce((sum, sample) => sum + sample.value, 0)
}
