<script setup lang="ts">
// 组新建/编辑表单：ID 仅在创建时填写，编辑只更新名称与类型。
import { ref, watch } from 'vue'
import { NButton, NInput } from 'naive-ui'
import ErrorBlock from '@/components/ErrorBlock.vue'
import type { Group, GroupCreatePayload, GroupUpdatePayload } from '@/api/identity'

const props = withDefaults(
  defineProps<{
    /** 表单模式 */
    mode: 'create' | 'edit'
    /** 编辑时的初始组 */
    initial?: Group | null
    /** 提交中 */
    submitting?: boolean
    /** 提交错误 */
    error?: unknown
  }>(),
  { initial: null, submitting: false },
)

const emit = defineEmits<{
  submit: [payload: GroupCreatePayload | GroupUpdatePayload]
  cancel: []
}>()

/** 组 ID */
const id = ref('')
/** 组名 */
const name = ref('')
/** 组类型 */
const type = ref('')
/** 本地校验提示 */
const validation = ref('')

// 用初始组填充表单；新建时清空。
// @param value 初始组
// @return 无
function applyInitial(value: Group | null | undefined): void {
  id.value = value?.id ?? ''
  name.value = value?.name ?? ''
  type.value = value?.type ?? ''
  validation.value = ''
}

watch(() => props.initial, applyInitial, { immediate: true })

// 校验并提交表单。
// @return 无
function submit(): void {
  validation.value = ''
  const groupId = id.value.trim()
  // 新建必须提供组 ID，编辑时 ID 来自路由/列表行。
  if (props.mode === 'create' && !groupId) {
    validation.value = '组 ID 不能为空。'
    return
  }
  const common = { name: name.value.trim(), type: type.value.trim() }
  if (props.mode === 'create') {
    emit('submit', { id: groupId, ...common })
    return
  }
  emit('submit', common)
}
</script>

<template>
  <section class="panel identity-form">
    <div class="panel__header">
      <span class="panel__title">{{ mode === 'create' ? '新建组' : '编辑组' }}</span>
      <span class="muted" style="font-size: 12px">组 ID 创建后不可修改。</span>
    </div>
    <div class="panel__body">
      <ErrorBlock v-if="error" :error="error" context="PUT/POST /service/identity/groups" />
      <div v-if="validation" class="banner banner--warning" role="alert" style="margin: 0 0 12px">{{ validation }}</div>
      <div class="identity-form__grid">
        <label class="identity-form__item">
          <span class="identity-form__label">组 ID</span>
          <n-input v-model:value="id" :disabled="mode === 'edit'" placeholder="组的唯一标识" />
        </label>
        <label class="identity-form__item">
          <span class="identity-form__label">名称</span>
          <n-input v-model:value="name" placeholder="如：审批组" />
        </label>
        <label class="identity-form__item">
          <span class="identity-form__label">类型</span>
          <n-input v-model:value="type" placeholder="如：assignment，可留空" />
        </label>
      </div>
      <div class="row" style="margin-top: 12px">
        <n-button type="primary" size="small" :loading="submitting" @click="submit">
          {{ mode === 'create' ? '创建组' : '保存组' }}
        </n-button>
        <n-button size="small" quaternary @click="emit('cancel')">取消</n-button>
      </div>
    </div>
  </section>
</template>

<style scoped>
.identity-form { margin-bottom: 12px; }
.identity-form__grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: 10px 14px; }
.identity-form__item { display: flex; flex-direction: column; gap: 4px; }
.identity-form__label { font-size: 12px; color: var(--muted); }
</style>
