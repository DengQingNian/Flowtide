import { describe, expect, it } from 'vitest'
import { groupMetrics, metricLabelText, parseLabels, parseMetrics, sumByPrefix } from '@/lib/prometheus'

describe('Prometheus 解析', () => {
  it('解析标签与转义', () => {
    expect(parseLabels('{a="1",b="x\\"y"}')).toEqual({ a: '1', b: 'x"y' })
    expect(parseLabels('')).toEqual({})
  })

  it('解析样本并忽略注释与非法行', () => {
    const text = [
      '# HELP flowable_jobs 作业',
      'flowable_jobs 3',
      'flowable_http_requests_total{method="GET",path="/x"} 12',
      'bad line',
      'NaN_metric not-a-number',
      'flowable_x 1.5e3',
    ].join('\n')
    const samples = parseMetrics(text)
    expect(samples).toHaveLength(3)
    expect(samples[0]).toEqual({ name: 'flowable_jobs', labels: {}, value: 3 })
    expect(samples[1].labels).toEqual({ method: 'GET', path: '/x' })
    expect(samples[2].value).toBe(1500)
  })

  it('按名称分组并排序', () => {
    const groups = groupMetrics(parseMetrics('b 1\na{x="1"} 2\na{x="2"} 3'))
    expect(groups.map((group) => group.name)).toEqual(['a', 'b'])
    expect(groups[0].samples).toHaveLength(2)
  })

  it('渲染标签与按前缀汇总', () => {
    expect(metricLabelText({})).toBe('—')
    expect(metricLabelText({ a: '1', b: '2' })).toBe('a=1, b=2')
    const samples = parseMetrics('x 1\nx 2\ny 5')
    expect(sumByPrefix(samples, 'x')).toBe(3)
    expect(sumByPrefix(samples, 'z')).toBe(0)
  })
})
