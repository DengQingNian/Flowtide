<script setup lang="ts">
// 用户新建/编辑表单：编辑时隐藏密码字段，密码重置走用户详情的独立入口。
import { ref, watch } from 'vue'
import { NButton, NInput } from 'naive-ui'
import ErrorBlock from '@/components/ErrorBlock.vue'
import type { User, UserCreatePayload, UserUpdatePayload } from '@/api/identity'

const props = withDefaults(
  defineProps<{
    /** 表单模式 */
    mode: 'create' | 'edit'
    /** 编辑时的初始用户 */
    initial?: User | null
    /** 提交中 */
    submitting?: boolean
    /** 提交错误 */
    error?: unknown
  }>(),
  { initial: null, submitting: false },
)

const emit = defineEmits<{
  submit: [payload: UserCreatePayload | UserUpdatePayload]
  cancel: []
}>()

/** 用户 ID */
const id = ref('')
/** 名 */
const firstName = ref('')
/** 姓 */
const lastName = ref('')
/** 显示名 */
const displayName = ref('')
/** 邮箱 */
const email = ref('')
/** 初始密码（仅新建） */
const password = ref('')
/** 本地校验提示 */
const validation = ref('')

// 用初始用户填充表单；新建时清空。
// @param value 初始用户
// @return 无
function applyInitial(value: User | null | undefined): void {
  id.value = value?.id ?? ''
  firstName.value = value?.firstName ?? ''
  lastName.value = value?.lastName ?? ''
  displayName.value = value?.displayName ?? ''
  email.value = value?.email ?? ''
  password.value = ''
  validation.value = ''
}

watch(() => props.initial, applyInitial, { immediate: true })

// 校验并提交表单。
// @return 无
function submit(): void {
  validation.value = ''
  const userId = id.value.trim()
  // 新建必须提供 ID；编辑沿用路径 ID，不允许修改。
  if (props.mode === 'create' && !userId) {
    validation.value = '用户 ID 不能为空。'
    return
  }
  const common = {
    firstName: firstName.value.trim(),
    lastName: lastName.value.trim(),
    displayName: displayName.value.trim(),
    email: email.value.trim(),
  }
  if (props.mode === 'create') {
    const payload: UserCreatePayload = { id: userId, ...common }
    // 空密码等同于不设置初始密码，避免把空串写入凭据。
    if (password.value) payload.password = password.value
    emit('submit', payload)
    return
  }
  const payload: UserUpdatePayload = common
  emit('submit', payload)
}
</script>

<template>
  <section class="panel identity-form">
    <div class="panel__header">
      <span class="panel__title">{{ mode === 'create' ? '新建用户' : '编辑用户' }}</span>
      <span class="muted" style="font-size: 12px">密码不会回显；需要改密请到用户详情的密码面板。</span>
    </div>
    <div class="panel__body">
      <ErrorBlock v-if="error" :error="error" context="PUT/POST /service/identity/users" />
      <div v-if="validation" class="banner banner--warning" role="alert" style="margin: 0 0 12px">{{ validation }}</div>
      <div class="identity-form__grid">
        <label class="identity-form__item">
          <span class="identity-form__label">用户 ID</span>
          <n-input v-model:value="id" :disabled="mode === 'edit'" placeholder="登录标识，创建后不可修改" data-test="user-id" />
        </label>
        <label class="identity-form__item">
          <span class="identity-form__label">显示名</span>
          <n-input v-model:value="displayName" placeholder="如：张三" data-test="user-display-name" />
        </label>
        <label class="identity-form__item">
          <span class="identity-form__label">名</span>
          <n-input v-model:value="firstName" placeholder="First name" data-test="user-first-name" />
        </label>
        <label class="identity-form__item">
          <span class="identity-form__label">姓</span>
          <n-input v-model:value="lastName" placeholder="Last name" data-test="user-last-name" />
        </label>
        <label class="identity-form__item">
          <span class="identity-form__label">邮箱</span>
          <n-input v-model:value="email" placeholder="name@example.com" data-test="user-email" />
        </label>
        <label v-if="mode === 'create'" class="identity-form__item">
          <span class="identity-form__label">初始密码（可选）</span>
          <n-input v-model:value="password" type="password" show-password-on="click" placeholder="留空表示不设置密码" data-test="user-password" />
        </label>
      </div>
      <div class="row" style="margin-top: 12px">
        <n-button type="primary" size="small" :loading="submitting" @click="submit">
          {{ mode === 'create' ? '创建用户' : '保存资料' }}
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
