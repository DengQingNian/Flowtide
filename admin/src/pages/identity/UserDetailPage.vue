<script setup lang="ts">
// 用户详情：资料编辑、删除、角色分配、所属组、信息键值、头像与密码重置。
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NSpin, NTabPane, NTabs } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import ErrorBlock from '@/components/ErrorBlock.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import UserForm from './components/UserForm.vue'
import UserAvatar from './components/UserAvatar.vue'
import UserRolesPanel from './components/UserRolesPanel.vue'
import UserGroupsPanel from './components/UserGroupsPanel.vue'
import UserInfoPanel from './components/UserInfoPanel.vue'
import UserPicturePanel from './components/UserPicturePanel.vue'
import UserPasswordPanel from './components/UserPasswordPanel.vue'
import { deleteUser, getUser, updateUser } from '@/api/identity'
import type { User, UserUpdatePayload } from '@/api/identity'
import { isAuthError } from '@/api/client'
import { markAuthError } from '@/stores/ui'
import { textOrDash } from '@/lib/format'

const route = useRoute()
const router = useRouter()

/** 路径中的用户 ID */
const userId = computed(() => String(route.params.id ?? ''))
/** 用户对象 */
const user = ref<User | null>(null)
/** 加载态 */
const loading = ref(false)
/** 加载错误 */
const error = ref<unknown>(null)
/** 当前 Tab */
const activeTab = ref('profile')
/** 是否编辑资料 */
const editing = ref(false)
/** 保存中 */
const saving = ref(false)
/** 表单错误 */
const formError = ref<unknown>(null)
/** 成功提示 */
const success = ref('')
/** 删除中 */
const deleting = ref(false)
/** 删除错误 */
const actionError = ref<unknown>(null)

// 加载用户详情。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    user.value = await getUser(userId.value)
  } catch (cause) {
    error.value = cause
    if (isAuthError(cause)) markAuthError(cause)
  } finally {
    loading.value = false
  }
}

// 打开编辑表单。
// @return 无
function openEdit(): void {
  formError.value = null
  editing.value = true
}

// 关闭编辑表单。
// @return 无
function closeEdit(): void {
  editing.value = false
  formError.value = null
}

// 保存用户资料。
// @param payload 更新载荷
// @return 无
async function save(payload: UserUpdatePayload): Promise<void> {
  saving.value = true
  formError.value = null
  success.value = ''
  try {
    user.value = await updateUser(userId.value, payload)
    success.value = '用户资料已更新。'
    editing.value = false
  } catch (cause) {
    formError.value = cause
    markAuthError(cause)
  } finally {
    saving.value = false
  }
}

// 删除用户并返回列表。
// @return 无
async function remove(): Promise<void> {
  deleting.value = true
  actionError.value = null
  success.value = ''
  try {
    await deleteUser(userId.value)
    await router.push('/identity/users')
  } catch (cause) {
    actionError.value = cause
    markAuthError(cause)
  } finally {
    deleting.value = false
  }
}

// 返回用户列表。
// @return 无
function back(): void {
  void router.push('/identity/users')
}

onMounted(() => void load())
</script>

<template>
  <div>
    <PageHeader
      :title="user?.displayName || user?.id || '用户详情'"
      :description="`${textOrDash(userId)} · 资料、角色、组、信息与头像`"
    >
      <template #actions>
        <n-button size="small" quaternary @click="back">返回列表</n-button>
        <n-button v-if="user" size="small" @click="openEdit">编辑资料</n-button>
        <ConfirmButton
          v-if="user"
          label="删除用户"
          title="删除该用户？"
          :description="`用户 ${userId} 的组关系、信息键值与头像将一并删除，且不可恢复。`"
          :loading="deleting"
          @confirm="remove"
        />
      </template>
    </PageHeader>

    <ErrorBlock v-if="error" :error="error" context="GET /service/identity/users/{id}" @retry="load" />
    <div v-if="error" class="row">
      <n-button size="small" quaternary @click="back">返回用户列表</n-button>
    </div>

    <n-spin :show="loading">
      <div class="stack">
        <div v-if="success" class="banner" role="status">{{ success }}</div>
        <ErrorBlock v-if="actionError" :error="actionError" @retry="load" />

        <UserForm
          v-if="editing && user"
          mode="edit"
          :initial="user"
          :submitting="saving"
          :error="formError"
          @submit="save"
          @cancel="closeEdit"
        />

        <section v-if="user" class="panel">
          <div class="panel__header">
            <span class="panel__title">基本信息</span>
            <span class="muted" style="font-size: 12px">GET /service/identity/users/{{ userId }}</span>
          </div>
          <div class="panel__body">
            <div class="row" style="align-items: flex-start; gap: 20px">
              <UserAvatar :user="user" :size="64" />
              <div class="detail-grid grow">
                <div class="detail-item"><span class="detail-item__label">用户 ID</span><span class="detail-item__value mono">{{ textOrDash(user.id) }}</span></div>
                <div class="detail-item"><span class="detail-item__label">显示名</span><span class="detail-item__value">{{ textOrDash(user.displayName) }}</span></div>
                <div class="detail-item"><span class="detail-item__label">名</span><span class="detail-item__value">{{ textOrDash(user.firstName) }}</span></div>
                <div class="detail-item"><span class="detail-item__label">姓</span><span class="detail-item__value">{{ textOrDash(user.lastName) }}</span></div>
                <div class="detail-item"><span class="detail-item__label">邮箱</span><span class="detail-item__value">{{ textOrDash(user.email) }}</span></div>
                <div class="detail-item"><span class="detail-item__label">租户</span><span class="detail-item__value">{{ textOrDash(user.tenantId) }}</span></div>
              </div>
            </div>
            <div class="muted" style="font-size: 12px; margin-top: 10px">
              服务端不会返回密码、角色或禁用状态；密码只能重置，不会回显。
            </div>
          </div>
        </section>

        <n-tabs v-if="user" v-model:value="activeTab" type="line" animated>
          <n-tab-pane name="profile" tab="资料与角色" display-directive="if">
            <UserRolesPanel :user-id="userId" />
          </n-tab-pane>
          <n-tab-pane name="groups" tab="所属组" display-directive="if">
            <UserGroupsPanel :user-id="userId" />
          </n-tab-pane>
          <n-tab-pane name="info" tab="用户信息" display-directive="if">
            <UserInfoPanel :user-id="userId" />
          </n-tab-pane>
          <n-tab-pane name="picture" tab="头像与密码" display-directive="if">
            <div class="stack">
              <UserPicturePanel :user="user" />
              <UserPasswordPanel :user-id="userId" />
            </div>
          </n-tab-pane>
        </n-tabs>
      </div>
    </n-spin>
  </div>
</template>
