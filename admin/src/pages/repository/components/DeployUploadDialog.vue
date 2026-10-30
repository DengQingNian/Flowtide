<script setup lang="ts">
// 部署上传弹窗：收集文件、名称、租户与重复过滤选项，校验后交给页面提交。
import { computed, ref, watch } from 'vue'
import { NButton, NCheckbox, NForm, NFormItem, NInput, NModal } from 'naive-ui'
import ErrorBlock from '@/components/ErrorBlock.vue'
import { formatBytes } from '@/lib/format'

/** 上传提交载荷 */
export interface DeployUploadPayload {
  /** 待上传文件 */
  files: File[]
  /** 部署名称，空时由服务端取默认值 */
  name: string
  /** 租户，空时使用服务端默认租户 */
  tenantId: string
  /** 是否复用同租户同内容的已有部署 */
  enableDuplicateFiltering: boolean
}

const props = withDefaults(
  defineProps<{
    /** 是否显示 */
    show: boolean
    /** 提交中 */
    submitting?: boolean
    /** 提交错误 */
    error?: unknown
  }>(),
  { submitting: false },
)

const emit = defineEmits<{
  'update:show': [boolean]
  submit: [DeployUploadPayload]
}>()

const fileInput = ref<HTMLInputElement | null>(null)
const files = ref<File[]>([])
const name = ref('')
const tenantId = ref('')
const duplicateFiltering = ref(false)
const localError = ref('')

/** 文件总大小 */
const totalSize = computed(() => files.value.reduce((sum, file) => sum + file.size, 0))

// 重置弹窗内的全部输入。
// @return 无
function reset(): void {
  files.value = []
  name.value = ''
  tenantId.value = ''
  duplicateFiltering.value = false
  localError.value = ''
  if (fileInput.value) fileInput.value.value = ''
}

watch(
  () => props.show,
  (visible) => {
    if (visible) reset()
  },
)

// 读取用户选择的文件。
// @param event 文件输入事件
// @return 无
function onFilesChange(event: Event): void {
  const input = event.target as HTMLInputElement
  files.value = Array.from(input.files ?? [])
  localError.value = ''
}

// 校验并提交上传请求。
// @return 无
function submit(): void {
  localError.value = ''
  if (files.value.length === 0) {
    localError.value = '请选择至少一个 BPMN、XML、ZIP 或 BAR 文件'
    return
  }
  emit('submit', {
    files: files.value,
    name: name.value.trim(),
    tenantId: tenantId.value.trim(),
    enableDuplicateFiltering: duplicateFiltering.value,
  })
}
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    title="上传部署"
    style="width: 620px; max-width: 94vw"
    @update:show="emit('update:show', $event)"
  >
    <n-form label-placement="top">
      <n-form-item label="部署文件">
        <input
          ref="fileInput"
          class="deploy-upload__file"
          type="file"
          multiple
          accept=".bpmn,.xml,.zip,.bar,application/xml,application/zip"
          @change="onFilesChange"
        />
        <div class="muted" style="font-size: 12px">
          支持单个 BPMN/XML，也支持 ZIP/BAR 或多文件；资源名取文件名，最大 64MB。
        </div>
        <div v-if="files.length > 0" class="deploy-upload__summary">
          已选择 {{ files.length }} 个文件，共 {{ formatBytes(totalSize) }}
        </div>
        <ul v-if="files.length > 0" class="deploy-upload__list">
          <li v-for="file in files" :key="file.name">{{ file.name }}（{{ formatBytes(file.size) }}）</li>
        </ul>
      </n-form-item>
      <n-form-item label="部署名称">
        <n-input v-model:value="name" placeholder="可选，缺省为 deployment" />
      </n-form-item>
      <n-form-item label="租户">
        <n-input v-model:value="tenantId" placeholder="可选，留空使用服务端默认租户" />
      </n-form-item>
      <n-checkbox v-model:checked="duplicateFiltering">
        相同租户与相同资源内容时复用已有部署
      </n-checkbox>
      <div v-if="localError" class="text-danger" role="alert" style="margin-top: 8px">{{ localError }}</div>
      <ErrorBlock v-if="error" :error="error" context="POST /service/repository/deployments" style="margin-top: 12px" @retry="submit" />
    </n-form>
    <template #footer>
      <div class="row" style="justify-content: flex-end">
        <n-button size="small" quaternary :disabled="submitting" @click="emit('update:show', false)">取消</n-button>
        <n-button size="small" type="primary" :loading="submitting" @click="submit">上传并部署</n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.deploy-upload__file {
  width: 100%;
  padding: 6px;
  border: 1px dashed var(--border-strong);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
  color: var(--text-2);
}
.deploy-upload__summary {
  margin-top: 6px;
  font-size: 12px;
  color: var(--text-2);
}
.deploy-upload__list {
  margin: 6px 0 0;
  padding-left: 18px;
  font-size: 12px;
  color: var(--muted);
}
</style>
