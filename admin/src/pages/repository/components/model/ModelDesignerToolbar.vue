<script setup lang="ts">
// 设计器工具栏：保存、部署、导入导出、撤销重做、缩放与校验入口。
import { NButton } from 'naive-ui'
import { Download, Maximize2, Redo2, Rocket, Save, Undo2, Upload, ZoomIn, ZoomOut } from '@lucide/vue'

withDefaults(
  defineProps<{
    /** 是否有未保存修改 */
    dirty?: boolean
    /** 保存中 */
    saving?: boolean
    /** 部署中 */
    deploying?: boolean
  }>(),
  { dirty: false, saving: false, deploying: false },
)

const emit = defineEmits<{
  save: []
  deploy: []
  import: []
  export: ['bpmn' | 'bpmn20.xml']
  undo: []
  redo: []
  zoomIn: []
  zoomOut: []
  fit: []
  validate: []
}>()
</script>

<template>
  <div class="designer-toolbar">
    <span v-if="dirty" class="designer-toolbar__dirty" data-testid="dirty-indicator">未保存</span>
    <n-button size="small" type="primary" :loading="saving" data-testid="save" @click="emit('save')">
      <template #icon><Save :size="14" /></template>
      保存
    </n-button>
    <n-button size="small" type="primary" secondary :loading="deploying" data-testid="deploy" @click="emit('deploy')">
      <template #icon><Rocket :size="14" /></template>
      部署
    </n-button>
    <n-button size="small" quaternary data-testid="import" @click="emit('import')">
      <template #icon><Upload :size="14" /></template>
      导入
    </n-button>
    <n-button size="small" quaternary data-testid="export-bpmn" @click="emit('export', 'bpmn')">
      <template #icon><Download :size="14" /></template>
      导出 .bpmn
    </n-button>
    <n-button size="small" quaternary data-testid="export-bpmn20" @click="emit('export', 'bpmn20.xml')">
      导出 .bpmn20.xml
    </n-button>
    <n-button size="small" quaternary title="撤销" data-testid="undo" @click="emit('undo')">
      <template #icon><Undo2 :size="14" /></template>
    </n-button>
    <n-button size="small" quaternary title="重做" data-testid="redo" @click="emit('redo')">
      <template #icon><Redo2 :size="14" /></template>
    </n-button>
    <n-button size="small" quaternary title="放大" data-testid="zoom-in" @click="emit('zoomIn')">
      <template #icon><ZoomIn :size="14" /></template>
    </n-button>
    <n-button size="small" quaternary title="缩小" data-testid="zoom-out" @click="emit('zoomOut')">
      <template #icon><ZoomOut :size="14" /></template>
    </n-button>
    <n-button size="small" quaternary title="适应窗口" data-testid="fit" @click="emit('fit')">
      <template #icon><Maximize2 :size="14" /></template>
    </n-button>
    <n-button size="small" quaternary data-testid="validate" @click="emit('validate')">校验</n-button>
  </div>
</template>

<style scoped>
.designer-toolbar { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; justify-content: flex-end; }
.designer-toolbar__dirty { color: var(--danger); font-size: 12px; margin-right: 2px; }
</style>
