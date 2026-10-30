<script setup lang="ts">
// 角色新建/编辑表单：权限固定展示服务端识别的 read/write/admin。
import { ref, watch } from 'vue'
import { NButton, NCheckbox, NCheckboxGroup, NInput } from 'naive-ui'
import ErrorBlock from '@/components/ErrorBlock.vue'
import type { Role, RoleCreatePayload, RoleUpdatePayload } from '@/api/identity'
import { textOrDash } from '@/lib/format'

/** 固定权限选项 */
const PERMISSIONS = ['read', 'write', 'admin'] as const

const props = withDefaults(
  defineProps<{
    /** 表单模式 */
    mode: 'create' | 'edit'
    /** 编辑时的初始角色 */
    initial?: Role | null
    /** 提交中 */
    submitting?: boolean
    /** 提交错误 */
    error?: unknown
  }>(),
  { initial: null, submitting: false },
)

const emit = defineEmits<{
  submit: [payload: RoleCreatePayload | RoleUpdatePayload]
  cancel: []
}>()

/** 角色 ID */
const id = ref('')
/** 角色名 */
const name = ref('')
/** 选中的权限 */
const permissions = ref<string[]>([])
/** 本地校验提示 */
const validation = ref('')

// 用初始角色填充表单；新建时清空。
// @param value 初始角色
// @return 无
function applyInitial(value: Role | null | undefined): void {
  id.value = value?.id ?? ''
  name.value = value?.name ?? ''
  permissions.value = [...(value?.permissions ?? [])]
  validation.value = ''
}

watch(() => props.initial, applyInitial, { immediate: true })

/** 初始角色中不属于 read/write/admin 的权限，保存时会被替换 */
const extraPermissions = ref<string[]>([])

// 记录服务端返回的额外权限，提示保存会全量替换。
// @param value 初始角色
// @return 无
function collectExtra(value: Role | null | undefined): void {
  extraPermissions.value = (value?.permissions ?? []).filter((item) => !PERMISSIONS.includes(item as (typeof PERMISSIONS)[number]))
}

watch(() => props.initial, collectExtra, { immediate: true })

// 校验并提交表单。
// @return 无
function submit(): void {
  validation.value = ''
  const roleId = id.value.trim()
  // 新建必须提供角色 ID。
  if (props.mode === 'create' && !roleId) {
    validation.value = '角色 ID 不能为空。'
    return
  }
  const common = { name: name.value.trim(), permissions: [...permissions.value] }
  if (props.mode === 'create') {
    emit('submit', { id: roleId, ...common })
    return
  }
  emit('submit', common)
}
</script>

<template>
  <section class="panel identity-form">
    <div class="panel__header">
      <span class="panel__title">{{ mode === 'create' ? '新建角色' : '编辑角色' }}</span>
      <span class="muted" style="font-size: 12px">权限选项：read / write / admin；保存为全量替换。</span>
    </div>
    <div class="panel__body">
      <ErrorBlock v-if="error" :error="error" context="PUT/POST /service/identity/roles" />
      <div v-if="validation" class="banner banner--warning" role="alert" style="margin: 0 0 12px">{{ validation }}</div>
      <div class="identity-form__grid">
        <label class="identity-form__item">
          <span class="identity-form__label">角色 ID</span>
          <n-input v-model:value="id" :disabled="mode === 'edit'" placeholder="角色唯一标识" />
        </label>
        <label class="identity-form__item">
          <span class="identity-form__label">名称</span>
          <n-input v-model:value="name" placeholder="如：流程管理员" />
        </label>
      </div>
      <div class="identity-form__item" style="margin-top: 10px">
        <span class="identity-form__label">权限</span>
        <n-checkbox-group v-model:value="permissions">
          <div class="row">
            <n-checkbox v-for="permission in PERMISSIONS" :key="permission" :value="permission" :label="permission" />
          </div>
        </n-checkbox-group>
      </div>
      <div v-if="extraPermissions.length" class="muted" style="font-size: 12px; margin-top: 6px">
        该角色还有额外权限：{{ extraPermissions.map((item) => textOrDash(item)).join('、') }}。保存后仅保留上方勾选项。
      </div>
      <div class="row" style="margin-top: 12px">
        <n-button type="primary" size="small" :loading="submitting" @click="submit">
          {{ mode === 'create' ? '创建角色' : '保存角色' }}
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
