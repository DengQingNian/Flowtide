// 设计器文件工具：本地导入读取、导出下载与离开提示。
import { downloadText } from '@/lib/download'

// 读取本地文件文本。
// @param file 本地文件
// @return 文本内容
export function readFileText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result ?? ''))
    reader.onerror = () => reject(reader.error ?? new Error('读取文件失败'))
    reader.readAsText(file)
  })
}

// 导出 BPMN XML 并返回文件名。
// @param xml XML 文本
// @param key 模型 Key，空时使用 process
// @param format 扩展名
// @return 下载文件名
export function exportBpmnXml(xml: string, key: string, format: 'bpmn' | 'bpmn20.xml'): string {
  const base = key.trim() || 'process'
  const filename = `${base}.${format}`
  downloadText(filename, xml, 'application/xml;charset=utf-8')
  return filename
}

// 创建 beforeunload 守卫，仅在存在未保存修改时拦截。
// @param isDirty 是否未保存
// @return 事件处理函数
export function createBeforeUnloadGuard(isDirty: () => boolean): (event: BeforeUnloadEvent) => void {
  return (event: BeforeUnloadEvent) => {
    if (!isDirty()) return
    event.preventDefault()
    event.returnValue = ''
  }
}
