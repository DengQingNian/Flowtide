<script setup lang="ts">
// 实例概览：状态、时间、发起人、定义与长 ID 复制。
import { computed } from 'vue'
import type { ProcessInstance } from '@/api/runtime'
import { definitionKeyOf, definitionVersionOf } from '@/api/runtime'
import { formatDateTime, textOrDash } from '@/lib/format'
import StatusTag from '@/components/StatusTag.vue'
import CopyText from '@/components/CopyText.vue'

const props = defineProps<{
  /** 实例快照 */
  instance: ProcessInstance
}>()

/** 状态文案 */
const stateLabel = computed(() => {
  if (props.instance.ended) return '已结束'
  return props.instance.suspended ? '已挂起' : '运行中'
})

/** 状态色调 */
const stateTone = computed(() => {
  if (props.instance.ended) return 'default'
  return props.instance.suspended ? 'warning' : 'success'
})
</script>

<template>
  <div class="panel">
    <div class="panel__header"><span class="panel__title">基本信息</span></div>
    <div class="panel__body">
      <div class="detail-grid">
        <div class="detail-item">
          <span class="detail-item__label">实例 ID</span>
          <span class="detail-item__value"><CopyText :value="instance.id" truncate /></span>
        </div>
        <div class="detail-item">
          <span class="detail-item__label">状态</span>
          <span class="detail-item__value"><StatusTag :value="stateLabel" :tone="stateTone" /></span>
        </div>
        <div class="detail-item">
          <span class="detail-item__label">流程定义</span>
          <span class="detail-item__value">{{ textOrDash(instance.processDefinitionName || definitionKeyOf(instance.processDefinitionId)) }}</span>
        </div>
        <div class="detail-item">
          <span class="detail-item__label">定义 ID</span>
          <span class="detail-item__value"><CopyText :value="instance.processDefinitionId" truncate /></span>
        </div>
        <div class="detail-item">
          <span class="detail-item__label">定义 Key / 版本</span>
          <span class="detail-item__value mono">{{ definitionKeyOf(instance.processDefinitionId) || '—' }} / v{{ definitionVersionOf(instance.processDefinitionId) ?? '—' }}</span>
        </div>
        <div class="detail-item">
          <span class="detail-item__label">发起人</span>
          <span class="detail-item__value">{{ textOrDash(instance.startUserId) }}</span>
        </div>
        <div class="detail-item">
          <span class="detail-item__label">开始时间</span>
          <span class="detail-item__value">{{ formatDateTime(instance.startTime) }}</span>
        </div>
        <div class="detail-item">
          <span class="detail-item__label">结束时间</span>
          <span class="detail-item__value">{{ instance.endTime ? formatDateTime(instance.endTime) : '—（运行中）' }}</span>
        </div>
        <div class="detail-item">
          <span class="detail-item__label">业务 Key</span>
          <span class="detail-item__value">
            <CopyText v-if="instance.businessKey" :value="instance.businessKey" truncate />
            <span v-else>—</span>
          </span>
        </div>
        <div class="detail-item">
          <span class="detail-item__label">业务状态</span>
          <span class="detail-item__value">{{ textOrDash(instance.businessStatus) }}</span>
        </div>
        <div class="detail-item">
          <span class="detail-item__label">当前活动</span>
          <span class="detail-item__value mono">{{ textOrDash(instance.activityId) }}</span>
        </div>
        <div class="detail-item">
          <span class="detail-item__label">租户</span>
          <span class="detail-item__value">{{ textOrDash(instance.tenantId) }}</span>
        </div>
        <div class="detail-item">
          <span class="detail-item__label">到期时间</span>
          <span class="detail-item__value">{{ formatDateTime(instance.dueDate) }}</span>
        </div>
        <div class="detail-item">
          <span class="detail-item__label">删除原因</span>
          <span class="detail-item__value">{{ instance.deleteReason ? instance.deleteReason : '—（运行中实例不记录删除原因）' }}</span>
        </div>
      </div>
    </div>
  </div>
</template>
