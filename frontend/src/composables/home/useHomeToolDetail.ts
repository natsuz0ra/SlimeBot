import { computed, ref } from 'vue'
import type { ToolCallItem } from '@/api/chat'
import { useChatStore } from '@/stores/chat'
import type { ToolTimelineEntry } from '@/types/chat'
import { getCollapsedReplyTimeline, hasCollapsibleReplyContent } from '@/utils/replyBatchBuilder'

export function useHomeToolDetail(options: {
  t: (key: string, params?: Record<string, unknown>) => string
  store: ReturnType<typeof useChatStore>
}) {
  const { t, store } = options

  const toolDetailVisible = ref(false)
  const toolDetailBatchId = ref('')
  const toolDetailDialogWidth = 'min(688px, calc(100vw - 36px))'

  function findReplyBatchByMessageId(messageId: string) {
    return store.replyBatches.find((batch) => batch.assistantMessageId === messageId)
  }

  function topLevelToolCalls(batch: { toolCalls: ToolCallItem[] }) {
    return batch.toolCalls.filter((tc) => !tc.parentToolCallId)
  }

  function getReplyToolCount(messageId: string) {
    const batch = findReplyBatchByMessageId(messageId)
    if (!batch) return 0
    return topLevelToolCalls(batch).length
  }

  function getToolCallDesc(toolCall: ToolCallItem) {
    const params = toolCall.params || {}
    const nonEmptyEntries = Object.entries(params).filter(([, value]) => String(value ?? '').trim() !== '')

    if (toolCall.toolName === 'web_search') {
      const query = String(params.query ?? '').trim()
      if (query !== '') return `query: ${query}`
    }

    if (toolCall.toolName === 'run_subagent') {
      const title = String(toolCall.subagentTitle || params.title || '').trim()
      if (title !== '') return title
      const task = String(toolCall.subagentTask || params.task || '').trim()
      if (task !== '') {
        const short = task.length > 100 ? `${task.slice(0, 100)}…` : task
        return `task: ${short}`
      }
    }

    if (nonEmptyEntries.length === 0) return toolCall.command || ''
    return nonEmptyEntries
      .map(([key, value]) => `${key}: ${String(value)}`)
      .join(' | ')
  }

  function getReplyToolSummary(messageId: string) {
    const batch = findReplyBatchByMessageId(messageId)
    if (!batch) return ''

    const calls = topLevelToolCalls(batch)
    const count = calls.length
    if (count === 0) return ''
    if (batch.collapsed) return t('toolExecutionCount', { count })

    const runningCall = [...calls].reverse().find((item) => item.status === 'pending' || item.status === 'executing')
    if (runningCall) {
      const desc = getToolCallDesc(runningCall).trim()
      if (desc !== '') {
        return t('toolExecutionRunning', { command: runningCall.toolName, desc })
      }
      return t('toolExecutionRunningNoDesc', { command: runningCall.toolName })
    }

    const latest = calls[count - 1]
    if (!latest) return t('toolExecutionCount', { count })
    if (latest.status === 'completed') {
      return t('toolExecutionSuccess', { command: latest.toolName })
    }
    return t('toolExecutionFailed', { command: latest.toolName })
  }

  function getReplyToolCalls(messageId: string): ToolCallItem[] {
    return findReplyBatchByMessageId(messageId)?.toolCalls || []
  }

  function getReplyTimeline(messageId: string) {
    return findReplyBatchByMessageId(messageId)?.timeline || []
  }

  function getVisibleReplyTimeline(messageId: string) {
    const batch = findReplyBatchByMessageId(messageId)
    if (!batch) return []
    return batch.collapsed ? getCollapsedReplyTimeline(batch.timeline) : batch.timeline
  }

  function getReplyToolItem(messageId: string, toolCallId: string) {
    return getReplyToolCalls(messageId).find((item) => item.toolCallId === toolCallId)
  }

  function getSubagentChildTools(messageId: string, parentToolCallId: string) {
    const batch = findReplyBatchByMessageId(messageId)
    if (!batch) return []
    return batch.toolCalls.filter((tc) => tc.parentToolCallId === parentToolCallId)
  }

  function shouldShowInlineToolCall(messageId: string, toolCallId: string) {
    const item = getReplyToolItem(messageId, toolCallId)
    if (!item) return false
    if (item.parentToolCallId) return false
    if (item.requiresApproval) return true
    return item.toolName === 'run_subagent'
  }

  function isReplyToolCollapsed(messageId: string) {
    return findReplyBatchByMessageId(messageId)?.collapsed ?? false
  }

  function toggleReplyCollapsed(messageId: string) {
    const batch = findReplyBatchByMessageId(messageId)
    if (!batch || store.isStreamingMessage(messageId)) return
    batch.collapsed = !batch.collapsed
  }

  function getReplyElapsedMs(messageId: string) {
    const batch = findReplyBatchByMessageId(messageId)
    if (!batch) return undefined
    if (typeof batch.durationMs === 'number') return batch.durationMs
    if (typeof batch.startedAt !== 'number') return undefined
    const end = typeof batch.finishedAt === 'number' ? batch.finishedAt : Date.now()
    return Math.max(0, end - batch.startedAt)
  }

  function shouldShowReplyCollapseBar(messageId: string) {
    const batch = findReplyBatchByMessageId(messageId)
    if (!batch) return false
    return hasCollapsibleReplyContent(batch.timeline, batch.toolCalls)
  }

  function isEmptyPlaceholder(messageId: string) {
    const batch = findReplyBatchByMessageId(messageId)
    if (!batch) return false
    if (batch.collapsed) return false
    const msg = store.messages.find((m) => m.id === messageId)
    return !!msg && msg.content === '' && batch.timeline.length === 0
  }

  function openToolDetail(messageId: string) {
    const batch = findReplyBatchByMessageId(messageId)
    if (!batch || batch.toolCalls.length === 0) return
    toolDetailBatchId.value = batch.id
    toolDetailVisible.value = true
  }

  const toolDetailItems = computed(() => {
    return store.replyBatches.find((batch) => batch.id === toolDetailBatchId.value)?.toolCalls || []
  })
  const toolDetailTimeline = computed(() => {
    return store.replyBatches.find((batch) => batch.id === toolDetailBatchId.value)?.timeline || []
  })
  const toolDetailToolTimeline = computed(() => {
    return toolDetailTimeline.value.filter((entry): entry is ToolTimelineEntry => entry.kind !== 'text' && entry.kind !== 'plan')
  })

  return {
    toolDetailVisible,
    toolDetailDialogWidth,
    getReplyToolCount,
    getReplyToolSummary,
    getReplyTimeline,
    getVisibleReplyTimeline,
    getReplyToolItem,
    getSubagentChildTools,
    shouldShowInlineToolCall,
    isReplyToolCollapsed,
    toggleReplyCollapsed,
    getReplyElapsedMs,
    shouldShowReplyCollapseBar,
    isEmptyPlaceholder,
    openToolDetail,
    toolDetailItems,
    toolDetailToolTimeline,
  }
}
