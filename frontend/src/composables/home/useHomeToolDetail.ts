import { computed, ref } from 'vue'
import type { ToolCallItem } from '@/api/chat'
import { useChatStore } from '@/stores/chat'

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

  function getReplyToolCount(messageId: string) {
    return findReplyBatchByMessageId(messageId)?.toolCalls.length || 0
  }

  function getToolCallDesc(toolCall: ToolCallItem) {
    const params = toolCall.params || {}
    const nonEmptyEntries = Object.entries(params).filter(([, value]) => String(value ?? '').trim() !== '')

    if (toolCall.toolName === 'web_search') {
      const query = String(params.query ?? '').trim()
      if (query !== '') return `query: ${query}`
    }

    if (nonEmptyEntries.length === 0) return toolCall.command || ''
    return nonEmptyEntries
      .map(([key, value]) => `${key}: ${String(value)}`)
      .join(' | ')
  }

  function getReplyToolSummary(messageId: string) {
    const batch = findReplyBatchByMessageId(messageId)
    if (!batch) return ''

    const count = batch.toolCalls.length
    if (count === 0) return ''
    if (batch.collapsed) return t('toolExecutionCount', { count })

    const runningCall = [...batch.toolCalls].reverse().find((item) => item.status === 'pending' || item.status === 'executing')
    if (runningCall) {
      const desc = getToolCallDesc(runningCall).trim()
      if (desc !== '') {
        return t('toolExecutionRunning', { command: runningCall.toolName, desc })
      }
      return t('toolExecutionRunningNoDesc', { command: runningCall.toolName })
    }

    const latest = batch.toolCalls[count - 1]
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

  function getReplyToolItem(messageId: string, toolCallId: string) {
    return getReplyToolCalls(messageId).find((item) => item.toolCallId === toolCallId)
  }

  function shouldShowInlineToolCall(messageId: string, toolCallId: string) {
    const item = getReplyToolItem(messageId, toolCallId)
    if (!item) return false
    return item.requiresApproval
  }

  function isReplyToolCollapsed(messageId: string) {
    return findReplyBatchByMessageId(messageId)?.collapsed ?? false
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
    return toolDetailTimeline.value.filter((entry) => entry.kind !== 'text')
  })

  return {
    toolDetailVisible,
    toolDetailDialogWidth,
    getReplyToolCount,
    getReplyToolSummary,
    getReplyTimeline,
    getReplyToolItem,
    shouldShowInlineToolCall,
    isReplyToolCollapsed,
    isEmptyPlaceholder,
    openToolDetail,
    toolDetailItems,
    toolDetailToolTimeline,
  }
}
