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
