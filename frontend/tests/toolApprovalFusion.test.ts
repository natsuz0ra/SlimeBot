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
  assert.match(chatStoreSource, /onPlanBody: \(content, sessionId\) => \{[\s\S]*appendPlanBodyToBatch\(batch, content\)[\s\S]*planGenerating\.value = false/s)
})

test('chat socket done event forwards plan metadata while plan_body stays separate', () => {
  const chatSocketSource = readFileSync(resolve(import.meta.dirname, '../src/api/chatSocket.ts'), 'utf8')

  assert.match(chatSocketSource, /if \(data\.type === 'done'\) \{[\s\S]*planId: data\.planId,[\s\S]*planBody: data\.planBody,/s)
  assert.match(chatSocketSource, /if \(data\.type === 'plan_body'\) this\.handlers\?\.onPlanBody\?\.\(data\.content \|\| '', data\.sessionId\)/)
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

test('PlanBlock uses the shared chevron size and right-closed/down-open direction', () => {
  const planBlockSource = readFileSync(resolve(import.meta.dirname, '../src/components/chat/PlanBlock.vue'), 'utf8')

  assert.match(planBlockSource, /<svg[\s\S]*class="plan-block-chevron"[\s\S]*viewBox="0 0 16 16"[\s\S]*width="14"[\s\S]*height="14"/)
  assert.match(planBlockSource, /<path d="M4 6l4 4 4-4" \/>/)
  assert.match(planBlockSource, /\.plan-block-chevron\s*\{[\s\S]*width:\s*14px[\s\S]*height:\s*14px[\s\S]*rotate\(-90deg\)/)
  assert.match(planBlockSource, /\.plan-block-chevron--open\s*\{[\s\S]*rotate\(0deg\)/)
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
})
