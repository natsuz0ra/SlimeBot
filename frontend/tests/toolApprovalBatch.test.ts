import assert from 'node:assert/strict'
import test from 'node:test'
import type { ToolCallItem } from '../src/api/chat'
import {
  getBatchApprovalToolCallIds,
  markToolApprovalDecision,
} from '../src/utils/toolApprovals'

function tool(id: string, status: ToolCallItem['status'], toolName = 'exec'): ToolCallItem {
  return {
    toolCallId: id,
    toolName,
    command: 'run',
    params: {},
    requiresApproval: true,
    status,
  }
}

test('getBatchApprovalToolCallIds returns only pending non-question tools', () => {
  const tools = [
    tool('exec-1', 'pending'),
    tool('question-1', 'pending', 'ask_questions'),
    tool('exec-2', 'executing'),
    tool('exec-3', 'pending'),
  ]

  assert.deepEqual(getBatchApprovalToolCallIds(tools), ['exec-1', 'exec-3'])
})

test('markToolApprovalDecision updates the matching pending tool only', () => {
  const tools = [
    tool('exec-1', 'pending'),
    tool('exec-2', 'pending'),
  ]

  markToolApprovalDecision(tools, 'exec-1', true)

  assert.equal(tools[0]!.status, 'executing')
  assert.equal(tools[1]!.status, 'pending')

  markToolApprovalDecision(tools, 'exec-2', false)

  assert.equal(tools[1]!.status, 'rejected')
})
