// 模型列表列定义：把行内操作抽离页面，保持单文件体积可控。
import { h } from 'vue'
import { NButton, type DataTableColumns } from 'naive-ui'
import ConfirmButton from '@/components/ConfirmButton.vue'
import StatusTag from '@/components/StatusTag.vue'
import type { Model } from '@/api/models'
import { formatDateTime, textOrDash } from '@/lib/format'

/** 列操作回调 */
export interface ModelTableHandlers {
  /** 打开设计器 */
  onDesign: (model: Model) => void
  /** 编辑元信息 */
  onEdit: (model: Model) => void
  /** 删除模型 */
  onDelete: (model: Model) => void
  /** 是否正在删除该行 */
  isDeleting: (model: Model) => boolean
}

// 构造模型表格列。
// @param handlers 行操作回调
// @return 表格列定义
export function buildModelColumns(handlers: ModelTableHandlers): DataTableColumns<Model> {
  return [
    {
      title: '名称',
      key: 'name',
      minWidth: 180,
      ellipsis: { tooltip: true },
      render: (row) =>
        h(NButton, { text: true, size: 'tiny', onClick: () => handlers.onDesign(row) }, { default: () => row.name }),
    },
    { title: 'Key', key: 'key', minWidth: 150, ellipsis: { tooltip: true } },
    { title: '分类', key: 'category', width: 120, render: (row) => textOrDash(row.category) },
    { title: '版本', key: 'version', width: 80, render: (row) => `v${row.version}` },
    {
      title: '部署状态',
      key: 'deploymentId',
      width: 100,
      render: (row) =>
        h(StatusTag, {
          value: row.deploymentId ? '已部署' : '未部署',
          tone: row.deploymentId ? 'success' : 'default',
        }),
    },
    { title: '更新时间', key: 'lastUpdateTime', width: 170, render: (row) => formatDateTime(row.lastUpdateTime) },
    {
      title: '操作',
      key: 'actions',
      width: 210,
      render: (row) =>
        h('div', { class: 'row' }, [
          h(NButton, { size: 'tiny', quaternary: true, onClick: () => handlers.onDesign(row) }, { default: () => '设计' }),
          h(NButton, { size: 'tiny', quaternary: true, onClick: () => handlers.onEdit(row) }, { default: () => '编辑' }),
          h(ConfirmButton, {
            label: handlers.isDeleting(row) ? '删除中…' : '删除',
            loading: handlers.isDeleting(row),
            title: '确认删除该流程模型？',
            description: '仅删除模型元信息与源码；已生成的部署、流程定义与运行实例不受影响。',
            onConfirm: () => handlers.onDelete(row),
          }),
        ]),
    },
  ]
}
