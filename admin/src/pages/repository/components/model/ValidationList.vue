<script setup lang="ts">
// 校验结果列表：按错误/警告分级展示，带元素 ID 的问题可点击定位。
import { computed } from 'vue'
import type { ValidationIssue } from '@/design/validation'

const props = withDefaults(
  defineProps<{
    /** 问题列表 */
    issues: ValidationIssue[]
    /** 是否展示空结果提示 */
    showEmpty?: boolean
  }>(),
  { showEmpty: false },
)

const emit = defineEmits<{ focus: [string] }>()

/** 错误数 */
const errorCount = computed(() => props.issues.filter((item) => item.level === 'error').length)
/** 警告数 */
const warningCount = computed(() => props.issues.filter((item) => item.level === 'warning').length)

// 定位到问题元素。
// @param issue 问题
// @return 无
function focus(issue: ValidationIssue): void {
  if (issue.elementId) emit('focus', issue.elementId)
}
</script>

<template>
  <section v-if="issues.length > 0" class="validation-list" data-testid="validation-list" aria-label="校验结果">
    <header class="validation-list__header">
      <span class="validation-list__title">校验结果</span>
      <span class="validation-list__count validation-list__count--error">错误 {{ errorCount }}</span>
      <span class="validation-list__count validation-list__count--warning">警告 {{ warningCount }}</span>
    </header>
    <ul class="validation-list__items">
      <li v-for="(issue, index) in issues" :key="`${issue.level}-${issue.elementId ?? ''}-${index}`" class="validation-list__item" :class="`validation-list__item--${issue.level}`">
        <span class="validation-list__badge">{{ issue.level === 'error' ? '错误' : '警告' }}</span>
        <button v-if="issue.elementId" type="button" class="validation-list__message validation-list__message--link" @click="focus(issue)">
          {{ issue.message }}
        </button>
        <span v-else class="validation-list__message">{{ issue.message }}</span>
        <code v-if="issue.elementId" class="validation-list__id">{{ issue.elementId }}</code>
      </li>
    </ul>
  </section>
  <p v-else-if="showEmpty" class="muted" data-testid="validation-empty">未发现问题。</p>
</template>

<style scoped>
.validation-list { border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--surface); margin-bottom: 10px; }
.validation-list__header { display: flex; align-items: center; gap: 10px; padding: 8px 12px; border-bottom: 1px solid var(--border); }
.validation-list__title { font-weight: 600; font-size: 13px; }
.validation-list__count { font-size: 12px; }
.validation-list__count--error { color: var(--danger); }
.validation-list__count--warning { color: var(--warning); }
.validation-list__items { list-style: none; margin: 0; padding: 6px 0; max-height: 180px; overflow-y: auto; }
.validation-list__item { display: flex; align-items: baseline; gap: 8px; padding: 4px 12px; font-size: 13px; }
.validation-list__badge { flex-shrink: 0; font-size: 11px; padding: 0 5px; border-radius: 4px; border: 1px solid transparent; }
.validation-list__item--error .validation-list__badge { color: var(--danger); background: var(--danger-soft); }
.validation-list__item--warning .validation-list__badge { color: var(--warning); background: var(--warning-soft); }
.validation-list__message { border: 0; background: transparent; padding: 0; color: var(--text); text-align: left; font: inherit; }
.validation-list__message--link { cursor: pointer; text-decoration: underline; text-underline-offset: 2px; }
.validation-list__message--link:hover { color: var(--blue); }
.validation-list__id { flex-shrink: 0; margin-left: auto; color: var(--muted); background: var(--surface-2); }
</style>
