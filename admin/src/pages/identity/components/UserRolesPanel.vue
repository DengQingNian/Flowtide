<script setup lang="ts">
// 用户角色分配：服务端没有角色查询接口，只能全量提交并提示无法回显当前角色。
import { onMounted, ref } from 'vue'
import { NButton, NCheckbox, NCheckboxGroup } from 'naive-ui'
import ErrorBlock from '@/components/ErrorBlock.vue'
import { listRoles, setUserRoles } from '@/api/identity'
import type { Role } from '@/api/identity'
import { isAuthError } from '@/api/client'
import { markAuthError } from '@/stores/ui'
import { textOrDash } from '@/lib/format'

const props = defineProps<{
  /** 用户 ID */
  userId: string
}>()

/** 全部角色 */
const roles = ref<Role[]>([])
/** 待提交的角色 ID */
const selected = ref<string[]>([])
/** 加载态 */
const loading = ref(false)
/** 加载错误 */
const loadError = ref<unknown>(null)
/** 提交中 */
const saving = ref(false)
/** 提交错误 */
const saveError = ref<unknown>(null)
/** 成功提示 */
const success = ref('')

// 加载可选角色。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  loadError.value = null
  try {
    const page = await listRoles({ size: 1000 })
    roles.value = page.data ?? []
  } catch (cause) {
    loadError.value = cause
    if (isAuthError(cause)) markAuthError(cause)
  } finally {
    loading.value = false
  }
}

// 提交全量角色集合。
// @return 无
async function save(): Promise<void> {
  saving.value = true
  saveError.value = null
  success.value = ''
  try {
    await setUserRoles(props.userId, [...selected.value])
    success.value = selected.value.length ? `已设置角色：${selected.value.join('、')}` : '已清空该用户的角色。'
  } catch (cause) {
    saveError.value = cause
    markAuthError(cause)
    // 认证/权限错误由全局横幅承接，同时保留错误原文在面板内。
    if (isAuthError(cause)) success.value = ''
  } finally {
    saving.value = false
  }
}

// 权限标签的展示文本。
// @param role 角色
// @return 逗号分隔的权限
function permissionText(role: Role): string {
  return role.permissions?.length ? role.permissions.join(' / ') : '—'
}

onMounted(() => void load())
</script>

<template>
  <section class="panel">
    <div class="panel__header">
      <span class="panel__title">角色分配</span>
      <span class="muted" style="font-size: 12px">PUT /identity/users/{{ userId }}/roles</span>
    </div>
    <div class="panel__body">
      <div class="banner banner--warning" role="note" style="margin: 0 0 12px">
        服务端未提供“查询用户角色”的 GET 接口，这里无法回显当前角色。保存会将所选角色全量替换现有角色，请谨慎操作。
      </div>
      <ErrorBlock v-if="loadError" :error="loadError" context="GET /service/identity/roles" @retry="load" />
      <ErrorBlock v-if="saveError" :error="saveError" context="PUT /service/identity/users/{id}/roles" @retry="save" />
      <div v-if="success" class="banner" role="status" style="margin: 0 0 12px">{{ success }}</div>
      <div v-if="loading" class="muted">正在加载角色…</div>
      <template v-else-if="!loadError">
        <n-checkbox-group v-model:value="selected">
          <div class="role-list">
            <label v-for="role in roles" :key="role.id" class="role-list__item">
              <n-checkbox :value="role.id" />
              <span class="role-list__id mono">{{ textOrDash(role.id) }}</span>
              <span class="grow">{{ textOrDash(role.name) }}</span>
              <span class="muted" style="font-size: 12px">{{ permissionText(role) }}</span>
            </label>
          </div>
        </n-checkbox-group>
        <div v-if="!roles.length" class="muted">暂无角色，请先在角色页创建。</div>
        <div class="row" style="margin-top: 12px">
          <n-button type="primary" size="small" :loading="saving" @click="save">保存角色</n-button>
          <n-button size="small" quaternary @click="selected = []">清空选择</n-button>
        </div>
      </template>
    </div>
  </section>
</template>

<style scoped>
.role-list { display: flex; flex-direction: column; gap: 2px; max-height: 320px; overflow: auto; }
.role-list__item { display: flex; align-items: center; gap: 10px; padding: 6px 8px; border-radius: var(--radius-sm); }
.role-list__item:hover { background: var(--surface-2); }
.role-list__id { min-width: 110px; }
</style>
