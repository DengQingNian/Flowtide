<script setup lang="ts">
// 管理员重置密码：只提交新密码，成功后清空输入并提示。
import { ref } from 'vue'
import { NButton, NInput } from 'naive-ui'
import ErrorBlock from '@/components/ErrorBlock.vue'
import { resetPassword } from '@/api/identity'
import { markAuthError } from '@/stores/ui'

const props = defineProps<{
  /** 用户 ID */
  userId: string
}>()

/** 新密码 */
const newPassword = ref('')
/** 确认密码 */
const confirmPassword = ref('')
/** 提交中 */
const saving = ref(false)
/** 错误 */
const error = ref<unknown>(null)
/** 成功提示 */
const success = ref('')
/** 本地校验 */
const validation = ref('')

// 校验并提交新密码。
// @return 无
async function submit(): Promise<void> {
  validation.value = ''
  success.value = ''
  error.value = null
  // 前端只做非空与一致性校验，强度规则由服务端决定。
  if (!newPassword.value) {
    validation.value = '新密码不能为空。'
    return
  }
  if (newPassword.value !== confirmPassword.value) {
    validation.value = '两次输入的密码不一致。'
    return
  }
  saving.value = true
  try {
    await resetPassword(props.userId, newPassword.value)
    success.value = '密码已重置，请通知用户使用新密码登录。'
    newPassword.value = ''
    confirmPassword.value = ''
  } catch (cause) {
    error.value = cause
    markAuthError(cause)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <section class="panel">
    <div class="panel__header">
      <span class="panel__title">重置密码</span>
      <span class="muted" style="font-size: 12px">PUT /identity/users/{{ userId }}/password</span>
    </div>
    <div class="panel__body">
      <ErrorBlock v-if="error" :error="error" context="PUT /service/identity/users/{id}/password" @retry="submit" />
      <div v-if="validation" class="banner banner--warning" role="alert" style="margin: 0 0 12px">{{ validation }}</div>
      <div v-if="success" class="banner" role="status" style="margin: 0 0 12px">{{ success }}</div>
      <div class="row" style="align-items: flex-end">
        <label class="password-field">
          <span class="identity-label">新密码</span>
          <n-input v-model:value="newPassword" type="password" show-password-on="click" placeholder="输入新密码" />
        </label>
        <label class="password-field">
          <span class="identity-label">确认新密码</span>
          <n-input v-model:value="confirmPassword" type="password" show-password-on="click" placeholder="再次输入" />
        </label>
        <n-button size="small" type="primary" :loading="saving" @click="submit">重置密码</n-button>
      </div>
      <div class="muted" style="font-size: 12px; margin-top: 8px">
        该操作只提交新密码，不要求旧密码；密码不会在界面回显，也不会保存到本地。
      </div>
    </div>
  </section>
</template>

<style scoped>
.password-field { display: flex; flex-direction: column; gap: 4px; min-width: 200px; }
.identity-label { font-size: 12px; color: var(--muted); }
</style>
