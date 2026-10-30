<script setup lang="ts">
// 任务动作栏：签收、取消签收、完成、委派、解决、改派、到期日、优先级与删除。
import { computed, ref, watch } from 'vue'
import { NButton, NCheckbox, NInput, NInputNumber, NModal } from 'naive-ui'
import type { Task } from '@/api/task'
import ConfirmButton from '@/components/ConfirmButton.vue'

/** 动作类型 */
type ActionKind = 'claim' | 'unclaim' | 'delegate' | 'resolve' | 'assign' | 'due' | 'priority' | 'delete'

/** 动作附加数据 */
interface ActionPayload {
  assignee?: string
  value?: string | number | null
  reason?: string
  cascadeHistory?: boolean
}

const props = withDefaults(
  defineProps<{
    /** 任务 */
    task: Task
    /** 正在执行的动作 */
    busy?: string
  }>(),
  { busy: '' },
)

const emit = defineEmits<{
  /** 请求执行动作 */
  action: [kind: ActionKind, payload: ActionPayload]
  /** 请求打开完成表单 */
  complete: []
}>()

/** 当前弹窗动作 */
const pending = ref<ActionKind | null>(null)
const text = ref('')
const number = ref<number | null>(null)
const reason = ref('')
const cascadeHistory = ref(false)
const localError = ref('')

/** 是否有办理人 */
const hasAssignee = computed(() => Boolean(props.task.assignee))
/** 是否处于委派待处理 */
const delegationPending = computed(() => (props.task.delegationState ?? '').toLowerCase() === 'pending')
/** 弹窗是否显示 */
const showModal = computed({
  get: () => pending.value !== null,
  set: (value: boolean) => {
    if (!value) pending.value = null
  },
})
/** 弹窗标题 */
const modalTitle = computed(() => {
  const titles: Record<string, string> = {
    claim: '签收任务',
    delegate: '委派任务',
    assign: '改派办理人',
    due: '设置到期日',
    priority: '设置优先级',
    delete: '删除任务',
  }
  return pending.value ? titles[pending.value] ?? '任务操作' : ''
})

// 打开动作弹窗并初始化字段。
// @param kind 动作类型
// @return 无
function open(kind: ActionKind): void {
  pending.value = kind
  text.value = kind === 'assign' ? props.task.assignee ?? '' : ''
  number.value = props.task.priority
  reason.value = ''
  cascadeHistory.value = false
  localError.value = ''
}

watch(
  () => props.task.id,
  () => {
    pending.value = null
  },
)

// 提交弹窗中的动作，校验失败保留弹窗。
// @return 无
function submit(): void {
  localError.value = ''
  const kind = pending.value
  if (!kind) return
  if (kind === 'claim' || kind === 'delegate') {
    if (!text.value.trim()) {
      localError.value = kind === 'claim' ? '请输入签收人' : '请输入被委托人'
      return
    }
    emit('action', kind, { assignee: text.value.trim() })
  } else if (kind === 'assign') {
    emit('action', 'assign', { assignee: text.value.trim() })
  } else if (kind === 'due') {
    const raw = text.value.trim()
    // 空值代表清空到期日；非空必须是可解析的时间。
    if (raw && Number.isNaN(Date.parse(raw))) {
      localError.value = '到期时间格式非法，请使用 ISO-8601 或 YYYY-MM-DDTHH:mm:ss'
      return
    }
    emit('action', 'due', { value: raw || null })
  } else if (kind === 'priority') {
    if (number.value === null || !Number.isFinite(number.value)) {
      localError.value = '请输入合法优先级'
      return
    }
    emit('action', 'priority', { value: Math.trunc(number.value) })
  } else if (kind === 'delete') {
    emit('action', 'delete', { reason: reason.value.trim(), cascadeHistory: cascadeHistory.value })
  }
  pending.value = null
}
</script>

<template>
  <div class="panel">
    <div class="panel__body">
      <div class="row">
        <n-button size="small" type="primary" :disabled="hasAssignee" :loading="busy === 'claim'" @click="open('claim')">签收</n-button>
        <confirm-button
          v-if="hasAssignee"
          label="取消签收"
          title="确认取消签收？"
          :description="`任务将回到未签收状态，当前办理人 ${task.assignee} 会被清空。`"
          :loading="busy === 'unclaim'"
          @confirm="emit('action', 'unclaim', {})"
        />
        <n-button size="small" :loading="busy === 'complete'" @click="emit('complete')">完成…</n-button>
        <n-button size="small" :loading="busy === 'delegate'" @click="open('delegate')">委派</n-button>
        <confirm-button
          v-if="delegationPending"
          label="解决委派"
          title="确认解决委派？"
          description="任务办理人将恢复为原所有者，委派状态变为 resolved。"
          :danger="false"
          :loading="busy === 'resolve'"
          @confirm="emit('action', 'resolve', {})"
        />
        <n-button size="small" :loading="busy === 'assign'" @click="open('assign')">改派</n-button>
        <n-button size="small" @click="open('due')">到期日</n-button>
        <n-button size="small" @click="open('priority')">优先级</n-button>
        <confirm-button
          label="删除"
          title="确认删除该任务？"
          description="任务会从运行态移除；流程任务删除还需服务端允许，操作不可撤销。"
          :loading="busy === 'delete'"
          @confirm="open('delete')"
        />
      </div>
      <div v-if="delegationPending" class="muted" style="margin-top: 8px">
        当前为委派待处理：办理人 {{ task.assignee || '—' }}，所有者 {{ task.owner || '—' }}。
      </div>
    </div>

    <n-modal v-model:show="showModal" preset="card" :title="modalTitle" style="width: 480px; max-width: 92vw">
      <div v-if="pending === 'claim' || pending === 'delegate' || pending === 'assign'" class="filter-bar__item">
        <span class="filter-bar__label">{{ pending === 'claim' ? '签收人' : pending === 'delegate' ? '被委托人' : '新办理人（留空清空）' }}</span>
        <n-input v-model:value="text" placeholder="用户 ID" @keyup.enter="submit" />
      </div>
      <div v-else-if="pending === 'due'" class="filter-bar__item">
        <span class="filter-bar__label">到期时间（留空清空）</span>
        <n-input v-model:value="text" placeholder="2026-11-01T00:00:00Z" @keyup.enter="submit" />
      </div>
      <div v-else-if="pending === 'priority'" class="filter-bar__item">
        <span class="filter-bar__label">优先级</span>
        <n-input-number v-model:value="number" :min="0" :max="1000" style="width: 100%" />
      </div>
      <div v-else-if="pending === 'delete'">
        <div class="banner banner--warning" style="margin-bottom: 12px">
          删除后任务从运行态消失；勾选级联历史会同时删除历史任务记录，无法恢复。
        </div>
        <div class="filter-bar__item">
          <span class="filter-bar__label">删除原因</span>
          <n-input v-model:value="reason" placeholder="可选" />
        </div>
        <div class="row" style="margin-top: 10px">
          <n-checkbox v-model:checked="cascadeHistory">同时删除历史记录（cascadeHistory）</n-checkbox>
        </div>
      </div>
      <div v-if="localError" class="text-danger" role="alert" style="margin-top: 8px">{{ localError }}</div>
      <template #footer>
        <div class="row" style="justify-content: flex-end">
          <n-button size="small" quaternary @click="pending = null">取消</n-button>
          <n-button size="small" :type="pending === 'delete' ? 'error' : 'primary'" @click="submit">确认</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>
