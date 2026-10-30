<script setup lang="ts">
// 实例迁移：选择同 Key 目标定义、校验活动映射并执行。
import { computed, onMounted, ref } from 'vue'
import { NButton, NInput, NSelect } from 'naive-ui'
import {
  getProcessDefinition,
  listProcessDefinitions,
  migrateInstance,
  validateInstanceMigration,
  type MigrationPayload,
  type ProcessDefinitionSummary,
} from '@/api/runtime'
import { errorMessage } from '@/api/client'
import { markAuthError } from '@/stores/ui'
import ErrorBlock from '@/components/ErrorBlock.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'

const props = defineProps<{
  /** 实例 ID */
  instanceId: string
  /** 当前定义 ID */
  definitionId: string
}>()

const emit = defineEmits<{ migrated: [] }>()

const current = ref<ProcessDefinitionSummary | null>(null)
const targets = ref<ProcessDefinitionSummary[]>([])
const targetId = ref('')
const mappingsText = ref('{}')
const loading = ref(false)
const error = ref<unknown>(null)
const actionError = ref('')
const notice = ref('')
const validating = ref(false)
const migrating = ref(false)
const validationMessages = ref<string[] | null>(null)

/** 目标定义选项 */
const options = computed(() =>
  targets.value.map((item) => ({ label: `${item.name || item.key} · v${item.version ?? '—'}`, value: item.id })),
)

/** 是否已通过校验 */
const canMigrate = computed(() => validationMessages.value !== null && validationMessages.value.length === 0 && Boolean(targetId.value))

// 加载当前定义与同 Key 的目标版本。
// @return 无
async function load(): Promise<void> {
  loading.value = true
  error.value = null
  validationMessages.value = null
  try {
    const definition = await getProcessDefinition(props.definitionId)
    current.value = definition
    const page = await listProcessDefinitions({ key: definition.key, start: 0, size: 1000 })
    targets.value = (page.data ?? []).filter((item) => item.id !== props.definitionId)
    targetId.value = targets.value[0]?.id ?? ''
  } catch (cause) {
    error.value = cause
    markAuthError(cause)
  } finally {
    loading.value = false
  }
}

// 解析活动映射 JSON。
// @return 映射对象，非法时返回 null
function parseMappings(): Record<string, string> | null {
  actionError.value = ''
  const text = mappingsText.value.trim()
  if (!text) return {}
  try {
    const parsed = JSON.parse(text) as unknown
    // 映射必须是字符串到字符串的对象，数组或标量直接拒绝。
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      actionError.value = '活动映射必须是 JSON 对象，例如 {"taskA":"taskB"}'
      return null
    }
    const result: Record<string, string> = {}
    for (const [key, value] of Object.entries(parsed as Record<string, unknown>)) {
      // 目标活动 ID 必须是字符串，避免提交后被引擎按非法参数拒绝。
      if (typeof value !== 'string') {
        actionError.value = `活动映射 ${key} 的值必须是字符串`
        return null
      }
      result[key] = value
    }
    return result
  } catch {
    actionError.value = '活动映射 JSON 格式非法，请修正后再校验'
    return null
  }
}

// 组装迁移载荷。
// @return 载荷或 null
function buildPayload(): MigrationPayload | null {
  if (!targetId.value) {
    actionError.value = '请选择目标流程定义'
    return null
  }
  const activityMappings = parseMappings()
  if (!activityMappings) return null
  return { targetProcessDefinitionId: targetId.value, activityMappings }
}

// 校验迁移。
// @return 无
async function validate(): Promise<void> {
  const payload = buildPayload()
  if (!payload) return
  validating.value = true
  notice.value = ''
  validationMessages.value = null
  try {
    const result = await validateInstanceMigration(props.instanceId, payload)
    validationMessages.value = result.validationMessages ?? []
    notice.value = validationMessages.value.length === 0 ? '校验通过，可以执行迁移' : '校验发现阻断项'
  } catch (cause) {
    actionError.value = errorMessage(cause)
    markAuthError(cause)
  } finally {
    validating.value = false
  }
}

// 执行迁移。
// @return 无
async function migrate(): Promise<void> {
  const payload = buildPayload()
  if (!payload) return
  if (!canMigrate.value) {
    actionError.value = '请先完成迁移校验并确认没有阻断项'
    return
  }
  migrating.value = true
  actionError.value = ''
  notice.value = ''
  try {
    await migrateInstance(props.instanceId, payload)
    notice.value = '迁移成功，正在刷新实例'
    emit('migrated')
  } catch (cause) {
    actionError.value = errorMessage(cause)
    markAuthError(cause)
  } finally {
    migrating.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="stack">
    <ErrorBlock v-if="error" :error="error" @retry="load" />
    <div v-else class="panel">
      <div class="panel__header">
        <span class="panel__title">实例迁移</span>
        <span class="muted" style="margin-left: auto; font-size: 12px">
          当前定义：{{ current?.key ?? '—' }} v{{ current?.version ?? '—' }}
        </span>
      </div>
      <div class="panel__body">
        <div class="row" style="align-items: flex-end">
          <div style="flex: 1; min-width: 260px">
            <div class="filter-bar__label">目标流程定义（同 Key，排除当前版本）</div>
            <n-select v-model:value="targetId" :options="options" :loading="loading" filterable placeholder="选择目标定义" @update:value="validationMessages = null" />
          </div>
          <n-button size="small" :loading="validating" :disabled="!targetId" @click="validate">校验迁移</n-button>
        </div>
        <div style="margin-top: 10px">
          <div class="filter-bar__label">活动映射（可选，JSON 对象：源活动 ID → 目标活动 ID）</div>
          <n-input v-model:value="mappingsText" type="textarea" :rows="4" placeholder="{&quot;taskA&quot;:&quot;taskB&quot;}" />
        </div>

        <div v-if="validationMessages" style="margin-top: 10px">
          <div v-if="validationMessages.length === 0" class="text-success">校验通过，没有阻断项。</div>
          <div v-else>
            <div class="text-danger">存在 {{ validationMessages.length }} 条阻断信息：</div>
            <ul style="margin: 6px 0 0; padding-left: 20px">
              <li v-for="(item, index) in validationMessages" :key="index">{{ item }}</li>
            </ul>
          </div>
        </div>

        <div class="row" style="margin-top: 12px">
          <span v-if="notice" class="text-success">{{ notice }}</span>
          <span v-if="actionError" class="text-danger" role="alert">{{ actionError }}</span>
          <span style="margin-left: auto">
            <ConfirmButton
              label="执行迁移"
              title="确认执行实例迁移？"
              description="实例的当前活动将被映射到目标定义版本，未映射且无法自动匹配的活动可能导致实例停滞。"
              :loading="migrating"
              :disabled="!canMigrate"
              @confirm="migrate"
            />
          </span>
        </div>
      </div>
    </div>

    <div class="banner" style="margin: 0">
      <span class="grow">执行树、活动注入与条件求值将在后续版本提供；本期请通过作业与历史页面完成排障。</span>
    </div>
  </div>
</template>
