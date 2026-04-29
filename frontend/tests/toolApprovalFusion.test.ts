import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import test from 'node:test'
import type { ToolCallItem } from '../src/api/chat'
import { shouldAutoExpandToolCall } from '../src/utils/toolApprovalExpansion'

test('HomePage no longer mounts the standalone approval drawer', () => {
  const homePage = readFileSync(resolve(import.meta.dirname, '../src/pages/HomePage.vue'), 'utf8')

  assert.doesNotMatch(homePage, /ApprovalDrawer/)
  assert.doesNotMatch(homePage, /pendingApproval/)
})

test('parent tool call auto-expands when a nested child is awaiting approval', () => {
  const parent: ToolCallItem = {
    toolCallId: 'parent-tool',
    toolName: 'run_subagent',
    command: 'run',
    params: {},
    requiresApproval: false,
    status: 'executing',
  }
  const nestedTools: ToolCallItem[] = [
    {
      toolCallId: 'nested-tool',
      toolName: 'exec',
      command: 'shell_command',
      params: { command: 'npm test' },
      requiresApproval: true,
      status: 'pending',
      parentToolCallId: parent.toolCallId,
    },
  ]

  assert.equal(shouldAutoExpandToolCall(parent, nestedTools), true)
})

test('parent tool call does not auto-expand after nested approval is resolved', () => {
  const parent: ToolCallItem = {
    toolCallId: 'parent-tool',
    toolName: 'run_subagent',
    command: 'run',
    params: {},
    requiresApproval: false,
    status: 'executing',
  }
  const nestedTools: ToolCallItem[] = [
    {
      toolCallId: 'nested-tool',
      toolName: 'exec',
      command: 'shell_command',
      params: { command: 'npm test' },
      requiresApproval: true,
      status: 'executing',
      parentToolCallId: parent.toolCallId,
    },
  ]

  assert.equal(shouldAutoExpandToolCall(parent, nestedTools), false)
})

test('chat store only promotes done.planId to pending plan confirmation', () => {
  const chatStoreSource = readFileSync(resolve(import.meta.dirname, '../src/stores/chat.ts'), 'utf8')

  assert.match(chatStoreSource, /if \(meta\?\.planId\) \{/)
  assert.match(chatStoreSource, /pendingPlanConfirmation\.value = \{[\s\S]*planId: meta\.planId/s)
  assert.match(chatStoreSource, /pendingPlanConfirmation\.value = \{[\s\S]*sessionId,/s)
  assert.match(chatStoreSource, /onPlanBody: \(content, sessionId\) => \{[\s\S]*appendPlanBodyToBatch\(batch, content\)[\s\S]*planGenerating\.value = false/s)
})

test('plan confirmation is only visible for the owning session', () => {
  const homePageSource = readFileSync(resolve(import.meta.dirname, '../src/pages/HomePage.vue'), 'utf8')
  const homeChatPageSource = readFileSync(resolve(import.meta.dirname, '../src/composables/home/useHomeChatPage.ts'), 'utf8')

  assert.doesNotMatch(homePageSource, /:plan-confirmation-visible="!!store\.pendingPlanConfirmation"/)
  assert.match(homePageSource, /:plan-confirmation-visible="currentSessionPlanConfirmationVisible"/)
  assert.match(
    homeChatPageSource,
    /currentSessionPlanConfirmationVisible[\s\S]*pendingPlanConfirmation[\s\S]*sessionId === store\.currentSessionId/s,
  )
})

test('plan approval actions ignore confirmations from another session', () => {
  const chatStoreSource = readFileSync(resolve(import.meta.dirname, '../src/stores/chat.ts'), 'utf8')

  assert.match(
    chatStoreSource,
    /function approvePlan[\s\S]*if \(pendingPlanConfirmation\.value\.sessionId !== sessionId\) return/s,
  )
  assert.match(
    chatStoreSource,
    /function rejectPlan[\s\S]*if \(pendingPlanConfirmation\.value\.sessionId !== sessionId\) return/s,
  )
  assert.match(
    chatStoreSource,
    /function modifyPlan[\s\S]*if \(pendingPlanConfirmation\.value\.sessionId !== sessionId\) return/s,
  )
})

test('approval prompts force the message list to scroll to bottom', () => {
  const homeScrollSource = readFileSync(resolve(import.meta.dirname, '../src/composables/home/useHomeScroll.ts'), 'utf8')

  assert.match(homeScrollSource, /store\.pendingApprovalToolCallIds\.join\('\|'\)/)
  assert.match(homeScrollSource, /store\.pendingQuestions\?\.toolCallId/)
  assert.match(homeScrollSource, /pendingPlanConfirmation[\s\S]*sessionId === store\.currentSessionId/s)
  assert.match(homeScrollSource, /queueScrollMessagesToBottom\(true\)/)
})

test('chat socket done event forwards plan metadata while plan_body stays separate', () => {
  const chatSocketSource = readFileSync(resolve(import.meta.dirname, '../src/api/chatSocket.ts'), 'utf8')

  assert.match(chatSocketSource, /if \(data\.type === 'done'\) \{[\s\S]*planId: data\.planId,[\s\S]*planBody: data\.planBody,/s)
  assert.match(chatSocketSource, /if \(data\.type === 'plan_body'\) handlers\?\.onPlanBody\?\.\(data\.content \|\| '', data\.sessionId\)/)
})

test('chat socket and store forward subagent title from start event', () => {
  const chatSocketSource = readFileSync(resolve(import.meta.dirname, '../src/api/chatSocket.ts'), 'utf8')
  const chatStoreSource = readFileSync(resolve(import.meta.dirname, '../src/stores/chat.ts'), 'utf8')

  assert.match(chatSocketSource, /interface SubagentStartData[\s\S]*title: string/s)
  assert.match(chatSocketSource, /title: data\.title \|\| ''/)
  assert.match(chatStoreSource, /parent\.subagentTitle = data\.title/)
})

test('chat socket forwards backend event timestamps for tool and thinking ordering', () => {
  const chatSocketSource = readFileSync(resolve(import.meta.dirname, '../src/api/chatSocket.ts'), 'utf8')
  const chatStoreSource = readFileSync(resolve(import.meta.dirname, '../src/stores/chat.ts'), 'utf8')

  assert.match(chatSocketSource, /startedAt\?: string/)
  assert.match(chatSocketSource, /finishedAt\?: string/)
  assert.match(chatSocketSource, /startedAt: data\.startedAt/)
  assert.match(chatSocketSource, /finishedAt: data\.finishedAt/)
  assert.match(chatStoreSource, /function parseSocketTimestamp/)
  assert.match(chatStoreSource, /startedAt: parseSocketTimestamp\(data\.startedAt\)/)
  assert.match(chatStoreSource, /startSubagentThinking\(batch, data\.parentToolCallId, parseSocketTimestamp\(data\.startedAt\)\)/)
  assert.match(chatStoreSource, /finishSubagentThinking\(batch, data\.parentToolCallId, parseSocketTimestamp\(data\.finishedAt\)\)/)
})

test('chat socket and store forward backend reply timing for live batches', () => {
  const chatSocketSource = readFileSync(resolve(import.meta.dirname, '../src/api/chatSocket.ts'), 'utf8')
  const chatStoreSource = readFileSync(resolve(import.meta.dirname, '../src/stores/chat.ts'), 'utf8')

  assert.match(chatSocketSource, /onStart: \(sessionId\?: string, meta\?: \{ startedAt\?: string \}\) => void/)
  assert.match(chatSocketSource, /durationMs\?: number/)
  assert.match(chatSocketSource, /handlers\?\.onStart\(data\.sessionId, \{ startedAt: data\.startedAt \}\)/)
  assert.match(chatSocketSource, /durationMs: data\.durationMs/)
  assert.match(chatStoreSource, /onStart: \(sessionId, meta\) => \{[\s\S]*startedAt: parseSocketTimestamp\(meta\?\.startedAt\),/s)
  assert.match(chatStoreSource, /finalizeReplyBatchTiming\(batch, parseSocketTimestamp\(meta\?\.finishedAt\), meta\?\.durationMs\)/)
})

test('ThinkingBlock supports live subagent reasoning content before completion', () => {
  const thinkingBlockSource = readFileSync(resolve(import.meta.dirname, '../src/components/chat/ThinkingBlock.vue'), 'utf8')

  assert.match(thinkingBlockSource, /variant\?: 'default' \| 'subagent'/)
  assert.match(thinkingBlockSource, /thinking-preview/)
  assert.match(thinkingBlockSource, /v-if="hasVisibleContent && done && expanded"/)
  assert.match(thinkingBlockSource, /subagentThinkingLabel/)
  assert.match(thinkingBlockSource, /subagentThoughtFor/)
  assert.match(thinkingBlockSource, /\.thinking-chevron\s*\{[\s\S]*margin-left:\s*auto/)
  assert.match(thinkingBlockSource, /class="thinking-chevron"/)
  assert.match(thinkingBlockSource, /<svg[\s\S]*class="thinking-chevron"[\s\S]*viewBox="0 0 16 16"/)
  assert.match(thinkingBlockSource, /<path d="M4 6l4 4 4-4" \/>/)
  assert.match(thinkingBlockSource, /\.thinking-chevron\s*\{[\s\S]*width:\s*14px[\s\S]*height:\s*14px[\s\S]*rotate\(-90deg\)/)
  assert.match(thinkingBlockSource, /\.thinking-chevron--open\s*\{[\s\S]*rotate\(0deg\)/)
  assert.match(thinkingBlockSource, /\.thinking-expand-enter-active\s*\{[\s\S]*opacity 180ms ease, max-height 250ms ease/)
  assert.match(thinkingBlockSource, /\.thinking-expand-leave-active\s*\{[\s\S]*opacity 120ms ease, max-height 180ms ease/)
  assert.doesNotMatch(thinkingBlockSource, /Sub-agent thought/)
})

test('ToolCallCard renders subagent thinking entries with the shared ThinkingBlock', () => {
  const toolCallCardSource = readFileSync(resolve(import.meta.dirname, '../src/components/chat/ToolCallCard.vue'), 'utf8')

  assert.match(toolCallCardSource, /subagentThinkingItems/)
  assert.match(toolCallCardSource, /subagentTimelineItems/)
  assert.match(toolCallCardSource, /subagentTimelineExpanded = ref\(false\)/)
  assert.match(toolCallCardSource, /showSubagentToolCallsThinking/)
  assert.match(toolCallCardSource, /subagentToolCallsThinkingTitle/)
  assert.match(toolCallCardSource, /v-for="timelineItem in subagentTimelineItems"/)
  assert.match(toolCallCardSource, /variant="subagent"/)
  assert.match(toolCallCardSource, /tool-result-summary--button/)
  assert.match(toolCallCardSource, /<Transition name="tool-subagent-expand">/)
  assert.match(toolCallCardSource, /<svg[\s\S]*class="tool-collapse-arrow"[\s\S]*viewBox="0 0 16 16"/)
  assert.match(toolCallCardSource, /<svg[\s\S]*class="tool-result-arrow"[\s\S]*viewBox="0 0 16 16"/)
  assert.match(toolCallCardSource, /<path d="M4 6l4 4 4-4" \/>/)
  assert.match(toolCallCardSource, /\.tool-collapse-arrow\s*\{[\s\S]*width:\s*14px[\s\S]*height:\s*14px[\s\S]*rotate\(-90deg\)/)
  assert.match(toolCallCardSource, /\.tool-collapse-arrow--open\s*\{[\s\S]*rotate\(0deg\)/)
  assert.match(toolCallCardSource, /\.tool-result-arrow--open\s*\{[\s\S]*rotate\(0deg\)/)
  assert.match(toolCallCardSource, /\.tool-subagent-expand-enter-active\s*\{[\s\S]*opacity 180ms ease, max-height 250ms ease/)
})

test('ToolCallCard collapses subagent tool calls and thinking when the outer card collapses', () => {
  const toolCallCardSource = readFileSync(resolve(import.meta.dirname, '../src/components/chat/ToolCallCard.vue'), 'utf8')

  assert.match(
    toolCallCardSource,
    /function toggleCollapse\(\) \{[\s\S]*isCollapsed\.value = !isCollapsed\.value[\s\S]*if \(isCollapsed\.value\) subagentTimelineExpanded\.value = false[\s\S]*\}/,
  )
})

test('ToolCallInline keeps subagent tool calls collapsed by default with the shared chevron style', () => {
  const toolCallInlineSource = readFileSync(resolve(import.meta.dirname, '../src/components/chat/ToolCallInline.vue'), 'utf8')

  assert.match(toolCallInlineSource, /subagentTimelineExpanded = ref\(false\)/)
  assert.match(toolCallInlineSource, /inline-subagent-tool-calls-thinking/)
  assert.match(toolCallInlineSource, /<Transition name="inline-expand">/)
  assert.match(toolCallInlineSource, /<svg[\s\S]*class="inline-tool-chevron"[\s\S]*viewBox="0 0 16 16"[\s\S]*width="14"[\s\S]*height="14"/)
  assert.match(toolCallInlineSource, /<svg[\s\S]*class="inline-subagent-chevron"[\s\S]*viewBox="0 0 16 16"/)
  assert.match(toolCallInlineSource, /<path d="M4 6l4 4 4-4" \/>/)
  assert.match(toolCallInlineSource, /\.inline-tool-chevron\s*\{[\s\S]*width:\s*14px[\s\S]*height:\s*14px[\s\S]*rotate\(-90deg\)/)
  assert.match(toolCallInlineSource, /\.inline-tool-chevron--open\s*\{[\s\S]*rotate\(0deg\)/)
  assert.match(toolCallInlineSource, /\.inline-subagent-chevron--open\s*\{[\s\S]*rotate\(0deg\)/)
})

test('ToolCallInline collapses subagent tool calls and thinking when the outer card collapses', () => {
  const toolCallInlineSource = readFileSync(resolve(import.meta.dirname, '../src/components/chat/ToolCallInline.vue'), 'utf8')

  assert.match(
    toolCallInlineSource,
    /function toggleExpanded\(\) \{[\s\S]*expanded\.value = shouldAutoExpand\.value \? true : !expanded\.value[\s\S]*if \(!expanded\.value\) subagentTimelineExpanded\.value = false[\s\S]*\}/,
  )
})

test('ToolCallInline collapses when approval-driven auto expansion ends', () => {
  const toolCallInlineSource = readFileSync(resolve(import.meta.dirname, '../src/components/chat/ToolCallInline.vue'), 'utf8')

  assert.match(
    toolCallInlineSource,
    /watch\(\s*shouldAutoExpand,\s*\(value\) => \{[\s\S]*if \(value\) \{[\s\S]*expanded\.value = true[\s\S]*return[\s\S]*\}[\s\S]*expanded\.value = false[\s\S]*subagentTimelineExpanded\.value = false[\s\S]*\}/,
  )
})

test('PlanBlock uses the shared chevron size and right-closed/down-open direction', () => {
  const planBlockSource = readFileSync(resolve(import.meta.dirname, '../src/components/chat/PlanBlock.vue'), 'utf8')

  assert.match(planBlockSource, /<svg[\s\S]*class="plan-block-chevron"[\s\S]*viewBox="0 0 16 16"[\s\S]*width="14"[\s\S]*height="14"/)
  assert.match(planBlockSource, /<path d="M4 6l4 4 4-4" \/>/)
  assert.match(planBlockSource, /\.plan-block-chevron\s*\{[\s\S]*width:\s*14px[\s\S]*height:\s*14px[\s\S]*rotate\(-90deg\)/)
  assert.match(planBlockSource, /\.plan-block-chevron--open\s*\{[\s\S]*rotate\(0deg\)/)
})

test('AssistantMessageBody renders the parent reply collapse bar and visible timeline', () => {
  const assistantBodySource = readFileSync(resolve(import.meta.dirname, '../src/components/chat/AssistantMessageBody.vue'), 'utf8')

  assert.match(assistantBodySource, /reply-collapse-bar/)
  assert.match(assistantBodySource, /<TransitionGroup[\s\S]*name="reply-segment"[\s\S]*tag="div"[\s\S]*class="assistant-reply-timeline"/)
  assert.match(assistantBodySource, /getCollapsedReplyTimeline/)
  assert.match(assistantBodySource, /v-for="\((entry, index|entry,\s*index)\) in renderedTimeline"/)
  assert.match(assistantBodySource, /assistant-reply-segment--first-visible/)
  assert.match(assistantBodySource, /assistant-reply-segment-inner/)
  assert.match(assistantBodySource, /ctx\.toggleReplyCollapsed\(item\.id\)/)
  assert.match(assistantBodySource, /ctx\.getReplyElapsedMs\((props\.)?item\.id\)/)
  assert.match(assistantBodySource, /reply-collapse-arrow/)
  assert.match(assistantBodySource, /\.reply-collapse-arrow\s*\{[\s\S]*rotate\(-90deg\)/)
  assert.match(assistantBodySource, /\.reply-collapse-arrow--open\s*\{[\s\S]*rotate\(0deg\)/)
  assert.match(assistantBodySource, /\.assistant-reply-segment\s*\{[\s\S]*grid-template-rows:\s*1fr/)
  assert.match(assistantBodySource, /\.assistant-reply-segment\s*\{[\s\S]*margin-top:\s*10px/)
  assert.match(assistantBodySource, /\.assistant-reply-segment:first-child,\s*\.assistant-reply-segment--first-visible\s*\{[\s\S]*margin-top:\s*0/)
  assert.match(assistantBodySource, /\.reply-segment-enter-active\s*,[\s\S]*grid-template-rows 300ms cubic-bezier\(0\.22, 1, 0\.36, 1\)/)
  assert.match(assistantBodySource, /\.reply-segment-enter-active\s*,[\s\S]*margin-top 300ms cubic-bezier\(0\.22, 1, 0\.36, 1\)/)
  assert.match(assistantBodySource, /\.reply-segment-enter-from,\s*\.reply-segment-leave-to[\s\S]*grid-template-rows:\s*0fr[\s\S]*margin-top:\s*0/)
  assert.match(assistantBodySource, /\.reply-segment-move\s*\{[\s\S]*transform 300ms cubic-bezier\(0\.22, 1, 0\.36, 1\)/)
})

test('i18n includes subagent thinking and combined tool-call labels', () => {
  const i18nSource = readFileSync(resolve(import.meta.dirname, '../src/i18n.ts'), 'utf8')

  assert.match(i18nSource, /subagentThinkingLabel:\s*'思考中\.\.\.'/)
  assert.match(i18nSource, /subagentThoughtFor:\s*'思考了 \{duration\}'/)
  assert.match(i18nSource, /subagentContextLabel:\s*'上下文'/)
  assert.match(i18nSource, /subagentTaskLabel:\s*'任务'/)
  assert.match(i18nSource, /subagentToolCallsThinkingTitle:\s*'工具调用 & 思考'/)
  assert.match(i18nSource, /subagentResultLabel:\s*'执行结果'/)
  assert.match(i18nSource, /subagentThinkingLabel:\s*'Thinking\.\.\.'/)
  assert.match(i18nSource, /subagentThoughtFor:\s*'Thought for \{duration\}'/)
  assert.match(i18nSource, /subagentToolCallsThinkingTitle:\s*'Tool calls & thinking'/)
  assert.match(i18nSource, /subagentResultLabel:\s*'Execution result'/)
  assert.match(i18nSource, /replyElapsed:\s*'.*\{duration\}/)
})
