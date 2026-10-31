// 工作流页面公共工具：列表数据源状态机、字段兼容读取与操作结果文案。
import { ref, onUnmounted } from 'vue'
import type { Ref } from 'vue'
import { extractRows, extractTotal } from '@/api/workflow'
import type { KeyedRow, WorkflowResult } from '@/api/workflow'

/** 列表数据源状态（供列表页模板绑定） */
export interface WorkflowListSource<T extends Record<string, unknown>> {
  /** 行数据（已补 _rid 行键） */
  rows: Ref<(T & KeyedRow)[]>
  /** 总条数 */
  total: Ref<number>
  /** 加载态 */
  loading: Ref<boolean>
  /** 失败错误文案（success=false 或 HTTP 层错误） */
  errorMessage: Ref<string>
  /** 501 降级文案（后端接线中） */
  degraded: Ref<string>
  /** 重新加载 */
  load: () => Promise<void>
}

// useWorkflowListSource 建立一个可刷新的列表数据源。
// 查询结果折叠为三种形态：501 → degraded 空态降级；失败 → errorMessage；成功 → rows/total。
// 通过序号丢弃过期响应，避免快速翻页时旧结果覆盖新结果。
// @param fetcher 列表查询函数（返回 WorkflowResult，不抛错）
// @param emptyHint 501 降级文案，如"实例分页接口接线中"
// @return 数据源状态
export function useWorkflowListSource<T extends Record<string, unknown>>(
  fetcher: () => Promise<WorkflowResult<unknown>>,
  emptyHint = '后端接口接线中，暂无数据',
): WorkflowListSource<T> {
  const rows = ref([]) as Ref<(T & KeyedRow)[]>
  const total = ref(0)
  const loading = ref(false)
  const errorMessage = ref('')
  const degraded = ref('')
  /** 请求序号，仅最新请求的结果会落盘 */
  let sequence = 0

  // load 执行一次查询并按结果形态更新状态。
  // @return 无
  async function load(): Promise<void> {
    const current = ++sequence
    loading.value = true
    errorMessage.value = ''
    try {
      const result = await fetcher()
      if (current !== sequence) return
      if (result.notImplemented) {
        rows.value = []
        total.value = 0
        degraded.value = emptyHint
      } else if (!result.success) {
        rows.value = []
        total.value = 0
        degraded.value = ''
        errorMessage.value = result.errorMessage
      } else {
        degraded.value = ''
        rows.value = extractRows<T>(result.data)
        total.value = extractTotal(result.data, rows.value)
      }
    } finally {
      if (current === sequence) loading.value = false
    }
  }

  onUnmounted(() => {
    // 序号失效即可让在途结果被丢弃
    sequence += 1
  })

  return { rows, total, loading, errorMessage, degraded, load }
}

// actionErrorOf 把一次写操作结果转换为可展示错误文案；成功返回 null。
// @param result 操作结果
// @param notImplementedText 501 时的降级文案（如"部署接口接线中"）
// @return 错误文案或 null
export function actionErrorOf(result: WorkflowResult<unknown>, notImplementedText: string): string | null {
  if (result.notImplemented) return notImplementedText
  if (!result.success) return result.errorMessage || '操作失败'
  return null
}

// pickField 从行数据中按优先级读取字段，兼容驼峰与下划线两种后端形态。
// @param row 行数据
// @param keys 候选字段名（按优先级排列）
// @return 第一个非空字段值，均缺失返回 undefined
export function pickField<T = unknown>(row: Record<string, unknown> | null | undefined, ...keys: string[]): T | undefined {
  if (!row) return undefined
  for (const key of keys) {
    const value = row[key]
    if (value !== undefined && value !== null && value !== '') return value as T
  }
  return undefined
}

// boolField 读取布尔字段，兼容 boolean 与字符串 "true"/"1"。
// @param row 行数据
// @param keys 候选字段名
// @return 布尔值，缺失返回 false
export function boolField(row: Record<string, unknown> | null | undefined, ...keys: string[]): boolean {
  const value = pickField<unknown>(row, ...keys)
  if (typeof value === 'boolean') return value
  if (typeof value === 'number') return value !== 0
  if (typeof value === 'string') return value === 'true' || value === '1'
  return false
}

// yesNoText 布尔值转中文文案。
// @param value 布尔值
// @return 是/否
export function yesNoText(value: boolean | undefined): string {
  return value ? '是' : '否'
}
