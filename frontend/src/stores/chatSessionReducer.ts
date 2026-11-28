import type { MessageItem, SessionHistoryThinkingItem } from '@/api/chat'
import type {
  ChatDoneData,
  ChatStartData,
  MessageEditedData,
  SteeringData,
  SubagentChunkData,
  SubagentDoneData,
  SubagentStartData,
  TeamMemberData,
  TeamRunData,
  ThinkingEventData,
  ToolApprovalRequiredData,
  ToolCallResultData,
  ToolCallReviewData,
  ToolCallStartData,
} from '../../../shared/chat-protocol/dispatch.js'
import { parseContentMarkers, stripContentMarkers } from '../utils/contentMarkers'
import {
  appendPlanBodyToBatch,
  appendPlanChunkToBatch,
  appendSubagentThinkingChunk,
  appendTextChunkToBatch,
  finalizeOpenReplyRuntimeState,
  finalizeReplyBatchTiming,
  finishOpenThinkingEntries,
  finishSubagentThinking,
  markLastThinkingDone,
  markToolCallError,
  startSubagentThinking,
} from '../utils/liveReplyTimeline'
import { applyEditedUserMessage } from '../utils/messageEditing'
import {
  buildInterleavedTimeline,
  normalizeToolStatus,
  type AssistantReplyBatch,
  type AssistantReplyTimelineItem,
} from '../utils/replyBatchBuilder'
import { createAgentTeamState, mergeAgentTeamMember, mergeAgentTeamRun } from '../utils/agentTeam'

export interface RuntimeQuestionItem {
  id: string
  question: string
  options: string[]
  option_descriptions?: string[]
}

export interface PendingPlanConfirmation {
  sessionId: string
  planId: string
  content: string
}

export interface ChatRuntimeState {
  messages: MessageItem[]
  replyBatches: AssistantReplyBatch[]
  currentBatchId: string
  activeTurnId: string
  waiting: boolean
  streamingStarted: boolean
  planGenerating: boolean
  assistantErrorIds: Set<string>
  failedUserMessageIds: Set<string>
  pendingQuestions: { toolCallId: string; questions: RuntimeQuestionItem[] } | null
  pendingPlanConfirmation: PendingPlanConfirmation | null
  pendingEditMessageId: string
}

export type ChatRuntimeEvent =
  | { type: 'start'; sessionId: string; meta?: ChatStartData; reuseBatchId?: string }
  | { type: 'chunk'; chunk: string }
  | { type: 'steering'; sessionId: string; data: SteeringData }
  | { type: 'message_edited'; data: MessageEditedData }
  | { type: 'done'; sessionId: string; answer?: string; meta?: ChatDoneData }
  | { type: 'error'; sessionId: string; error: string }
  | { type: 'connection_closed'; error?: string }
  | { type: 'tool_call_start'; data: ToolCallStartData }
  | { type: 'tool_call_review'; data: ToolCallReviewData }
  | { type: 'tool_approval_required'; data: ToolApprovalRequiredData }
  | { type: 'tool_call_result'; data: ToolCallResultData }
  | { type: 'subagent_start'; data: SubagentStartData }
  | { type: 'subagent_chunk'; data: SubagentChunkData }
  | { type: 'subagent_done'; data: SubagentDoneData }
  | { type: 'team_start' | 'team_done'; data: TeamRunData }
  | { type: 'team_member_queued'; data: TeamMemberData }
  | { type: 'thinking_start' | 'thinking_chunk' | 'thinking_done'; data: ThinkingEventData }
  | { type: 'plan_start' }
  | { type: 'plan_chunk'; chunk: string }
  | { type: 'plan_body'; content: string }

export interface ChatRuntimeReducerDeps {
  createId: () => string
  now: () => number
  formatAssistantError: (error: string) => string
  stoppedPlaceholder: () => string
}

export function createEmptyChatRuntimeState(): ChatRuntimeState {
  return {
    messages: [],
    replyBatches: [],
    currentBatchId: '',
    activeTurnId: '',
    waiting: false,
    streamingStarted: false,
    planGenerating: false,
    assistantErrorIds: new Set(),
    failedUserMessageIds: new Set(),
    pendingQuestions: null,
    pendingPlanConfirmation: null,
    pendingEditMessageId: '',
  }
}

function timestamp(value: string | undefined, deps: ChatRuntimeReducerDeps): number {
  if (!value) return deps.now()
  const parsed = Date.parse(value)
  return Number.isFinite(parsed) ? parsed : deps.now()
}

function cloneBatch(batch: AssistantReplyBatch): AssistantReplyBatch {
  return {
    ...batch,
    toolCalls: batch.toolCalls.map(tool => ({
      ...tool,
      subagentThinkings: tool.subagentThinkings?.map(thinking => ({ ...thinking })),
      subagentThinking: tool.subagentThinking ? { ...tool.subagentThinking } : undefined,
    })),
    teamRuns: batch.teamRuns.map(run => ({ ...run, members: run.members.map(member => ({ ...member })) })),
    timeline: batch.timeline.map(entry => ({ ...entry })),
  }
}

function mutableState(state: ChatRuntimeState, batchId = state.currentBatchId): ChatRuntimeState {
  return {
    ...state,
    messages: state.messages.map(message => ({ ...message })),
    replyBatches: state.replyBatches.map(batch => batch.id === batchId ? cloneBatch(batch) : batch),
    assistantErrorIds: new Set(state.assistantErrorIds),
    failedUserMessageIds: new Set(state.failedUserMessageIds),
  }
}

function currentBatch(state: ChatRuntimeState): AssistantReplyBatch | undefined {
  return state.replyBatches.find(batch => batch.id === state.currentBatchId)
}

function withCurrentBatch(
  state: ChatRuntimeState,
  reduce: (state: ChatRuntimeState, batch: AssistantReplyBatch) => void,
): ChatRuntimeState {
  if (!currentBatch(state)) return state
  const next = mutableState(state)
  reduce(next, currentBatch(next)!)
  return next
}

function buildLiveThinkingHistory(content: string, timeline: AssistantReplyTimelineItem[]): SessionHistoryThinkingItem[] {
  const thinkingEntries = timeline.filter(entry => entry.kind === 'thinking')
  if (thinkingEntries.length === 0) return []
  const thinkingIds = parseContentMarkers(content)
    .flatMap(segment => segment.type === 'thinking_marker' ? [segment.thinkingId] : [])
  return thinkingEntries.map((entry, index) => ({
    thinkingId: thinkingIds[index] ?? entry.id,
    content: entry.content,
    status: entry.done ? 'completed' : 'streaming',
    startedAt: entry.startedAt === undefined ? undefined : new Date(entry.startedAt).toISOString(),
    durationMs: entry.durationMs,
  }))
}

function reduceStart(state: ChatRuntimeState, event: Extract<ChatRuntimeEvent, { type: 'start' }>, deps: ChatRuntimeReducerDeps): ChatRuntimeState {
  if (event.reuseBatchId) {
    const existing = state.replyBatches.find(batch => batch.id === event.reuseBatchId)
    const assistant = existing && state.messages.find(message => message.id === existing.assistantMessageId)
    if (existing && assistant) {
      const next = mutableState(state, existing.id)
      const batch = next.replyBatches.find(item => item.id === existing.id)!
      const nextAssistant = next.messages.find(message => message.id === assistant.id)!
      nextAssistant.content = ''
      batch.timeline = []
      batch.toolCalls = []
      batch.teamRuns = []
      next.assistantErrorIds.delete(nextAssistant.id)
      next.currentBatchId = batch.id
      next.activeTurnId = event.meta?.turnId || ''
      next.waiting = true
      next.streamingStarted = false
      return next
    }
  }

  const next = mutableState(state)
  const assistantMessageId = deps.createId()
  const batchId = deps.createId()
  next.messages.push({
    id: assistantMessageId,
    sessionId: event.sessionId,
    role: 'assistant',
    content: '',
    createdAt: new Date(deps.now()).toISOString(),
  })
  next.assistantErrorIds.delete(assistantMessageId)
  next.replyBatches.push({
    id: batchId,
    sessionId: event.sessionId,
    assistantMessageId,
    toolCalls: [],
    teamRuns: [],
    timeline: [],
    collapsed: false,
    startedAt: timestamp(event.meta?.startedAt, deps),
  })
  next.currentBatchId = batchId
  next.activeTurnId = event.meta?.turnId || ''
  next.waiting = true
  next.streamingStarted = false
  return next
}

function reduceDone(state: ChatRuntimeState, event: Extract<ChatRuntimeEvent, { type: 'done' }>, deps: ChatRuntimeReducerDeps): ChatRuntimeState {
  const next = mutableState(state)
  next.activeTurnId = ''
  next.waiting = false
  next.streamingStarted = false
  next.planGenerating = false
  const batch = currentBatch(next)
  if (batch) {
    const finishedAt = timestamp(event.meta?.finishedAt, deps)
    finalizeOpenReplyRuntimeState(batch, 'Execution cancelled.', finishedAt)
    const assistant = next.messages.find(message => message.id === batch.assistantMessageId)
    if (assistant) {
      assistant.isInterrupted = !!event.meta?.isInterrupted
      assistant.isStopPlaceholder = !!event.meta?.isStopPlaceholder
    }
    const finalAnswer = typeof event.answer === 'string' && event.answer !== ''
      ? event.answer
      : (event.meta?.isStopPlaceholder ? deps.stoppedPlaceholder() : '')
    if (finalAnswer !== '') {
      if (assistant) {
        assistant.content = stripContentMarkers(finalAnswer)
        next.assistantErrorIds.delete(assistant.id)
      }
      const liveThinking = buildLiveThinkingHistory(finalAnswer, batch.timeline)
      batch.timeline = buildInterleavedTimeline(batch.toolCalls, finalAnswer, liveThinking)
    }
    finalizeReplyBatchTiming(batch, finishedAt, event.meta?.durationMs)
  }
  next.currentBatchId = ''
  if (event.meta?.planId) {
    next.pendingPlanConfirmation = {
      sessionId: event.sessionId,
      planId: event.meta.planId,
      content: event.meta.planBody || (event.answer ? stripContentMarkers(event.answer) : ''),
    }
  }
  return next
}

function reduceError(state: ChatRuntimeState, event: Extract<ChatRuntimeEvent, { type: 'error' }>, deps: ChatRuntimeReducerDeps): ChatRuntimeState {
  const next = mutableState(state)
  next.activeTurnId = ''
  next.waiting = false
  next.streamingStarted = false
  next.pendingEditMessageId = ''
  const errorMessage = deps.formatAssistantError(event.error)
  const batch = currentBatch(next)
  if (!batch) {
    const assistantMessageId = deps.createId()
    next.messages.push({
      id: assistantMessageId,
      sessionId: event.sessionId,
      role: 'assistant',
      content: errorMessage,
      createdAt: new Date(deps.now()).toISOString(),
    })
    next.assistantErrorIds.add(assistantMessageId)
    return next
  }

  finalizeOpenReplyRuntimeState(batch, event.error || 'Execution cancelled.', deps.now())
  const assistant = next.messages.find(message => message.id === batch.assistantMessageId)
  if (assistant) {
    assistant.content = errorMessage
    next.assistantErrorIds.add(assistant.id)
  }
  const textEntry: AssistantReplyTimelineItem = { id: deps.createId(), kind: 'text', content: errorMessage }
  const timeline: AssistantReplyTimelineItem[] = []
  let inserted = false
  for (const entry of batch.timeline) {
    if (entry.kind === 'text') {
      if (!inserted) {
        timeline.push(textEntry)
        inserted = true
      }
    } else {
      timeline.push(entry)
    }
  }
  if (!inserted) timeline.push(textEntry)
  batch.timeline = timeline
  finalizeReplyBatchTiming(batch, deps.now())
  next.currentBatchId = ''
  return next
}

function reduceToolStart(state: ChatRuntimeState, data: ToolCallStartData, deps: ChatRuntimeReducerDeps): ChatRuntimeState {
  return withCurrentBatch(state, (next, batch) => {
    finishOpenThinkingEntries(batch, deps.now())
    if (data.toolName === 'ask_questions' && data.params?.questions) {
      try {
        const questions = JSON.parse(String(data.params.questions)) as RuntimeQuestionItem[]
        if (Array.isArray(questions) && questions.length > 0) {
          next.pendingQuestions = { toolCallId: data.toolCallId, questions }
        }
      } catch {
        // Malformed questions remain owned by the backend timeout path.
      }
    }
    const startedToolCall = {
      toolCallId: data.toolCallId,
      toolName: data.toolName,
      command: data.command,
      params: data.params,
      preamble: data.preamble,
      requiresApproval: data.requiresApproval,
      reviewStatus: data.reviewStatus,
      reviewRisk: data.reviewRisk,
      reviewReason: data.reviewReason,
      status: data.reviewStatus === 'reviewing' ? 'reviewing' as const : data.requiresApproval ? 'pending' as const : 'executing' as const,
      startedAt: timestamp(data.startedAt, deps),
      parentToolCallId: data.parentToolCallId,
      subagentRunId: data.subagentRunId,
      teamRunId: data.teamRunId,
      memberRunId: data.memberRunId,
    }
    const existing = batch.toolCalls.find(tool => tool.toolCallId === data.toolCallId)
    if (existing) Object.assign(existing, startedToolCall)
    else batch.toolCalls.push(startedToolCall)
    if (!data.parentToolCallId) {
      batch.timeline.push({ id: deps.createId(), kind: 'tool_start', toolCallId: data.toolCallId })
    }
  })
}

function reduceToolResult(state: ChatRuntimeState, data: ToolCallResultData, deps: ChatRuntimeReducerDeps): ChatRuntimeState {
  return withCurrentBatch(state, (next, batch) => {
    const item = batch.toolCalls.find(tool => tool.toolCallId === data.toolCallId)
    if (item) {
      item.status = normalizeToolStatus(data.status, data.error)
      item.output = data.output
      item.error = data.error
      item.metadata = data.metadata
      item.requiresApproval = data.requiresApproval
      item.finishedAt = timestamp(data.finishedAt, deps)
      if (data.parentToolCallId) item.parentToolCallId = data.parentToolCallId
      if (data.subagentRunId) item.subagentRunId = data.subagentRunId
      if (data.teamRunId) item.teamRunId = data.teamRunId
      if (data.memberRunId) item.memberRunId = data.memberRunId
      if (item.toolName === 'ask_questions' && (item.status === 'error' || item.status === 'rejected')) {
        next.pendingQuestions = null
      }
    }
    if (!data.parentToolCallId) {
      batch.timeline.push({ id: deps.createId(), kind: 'tool_result', toolCallId: data.toolCallId })
    }
  })
}

function reduceThinking(state: ChatRuntimeState, event: Extract<ChatRuntimeEvent, { type: 'thinking_start' | 'thinking_chunk' | 'thinking_done' }>, deps: ChatRuntimeReducerDeps): ChatRuntimeState {
  return withCurrentBatch(state, (_next, batch) => {
    const data = event.data
    if (event.type === 'thinking_start') {
      if (data.parentToolCallId && data.subagentRunId) {
        startSubagentThinking(batch, data.parentToolCallId, timestamp(data.startedAt, deps))
      } else {
        batch.timeline.push({
          id: deps.createId(), kind: 'thinking', content: '', done: false, startedAt: timestamp(data.startedAt, deps),
        })
      }
      return
    }
    if (event.type === 'thinking_chunk') {
      if (data.parentToolCallId && data.subagentRunId) {
        appendSubagentThinkingChunk(batch, data.parentToolCallId, data.content || '', timestamp(data.startedAt, deps))
        return
      }
      const entries = [...batch.timeline]
      for (let index = entries.length - 1; index >= 0; index--) {
        const entry = entries[index]!
        if (entry.kind !== 'thinking' || entry.done) continue
        entries[index] = { ...entry, content: entry.content + (data.content || '') }
        break
      }
      batch.timeline = entries
      return
    }
    if (data.parentToolCallId && data.subagentRunId) {
      finishSubagentThinking(batch, data.parentToolCallId, timestamp(data.finishedAt, deps))
    } else {
      batch.timeline = markLastThinkingDone(batch.timeline, timestamp(data.finishedAt, deps))
    }
  })
}

export function reduceChatRuntimeEvent(
  state: ChatRuntimeState,
  event: ChatRuntimeEvent,
  deps: ChatRuntimeReducerDeps,
): ChatRuntimeState {
  switch (event.type) {
    case 'start':
      return reduceStart(state, event, deps)
    case 'chunk':
      return withCurrentBatch(state, (next, batch) => {
        const assistant = next.messages.find(message => message.id === batch.assistantMessageId)
        if (!assistant) return
        assistant.content += event.chunk
        appendTextChunkToBatch(batch, event.chunk)
        next.streamingStarted = true
      })
    case 'steering': {
      if (!event.data.messageId || state.messages.some(message => message.id === event.data.messageId)) return state
      const next = mutableState(state)
      const batch = currentBatch(next)
      const index = batch ? next.messages.findIndex(message => message.id === batch.assistantMessageId) : -1
      next.messages.splice(index < 0 ? next.messages.length : index, 0, {
        id: event.data.messageId,
        sessionId: event.sessionId,
        role: 'user',
        content: event.data.content || '',
        createdAt: event.data.createdAt || new Date(deps.now()).toISOString(),
      })
      return next
    }
    case 'message_edited':
      if (!event.data.messageId || (state.pendingEditMessageId && event.data.messageId !== state.pendingEditMessageId)) return state
      return {
        ...mutableState(state),
        ...applyEditedUserMessage(state.messages, state.replyBatches, event.data.messageId, event.data.content, event.data.createdAt),
        currentBatchId: '',
        pendingPlanConfirmation: null,
        pendingEditMessageId: '',
      }
    case 'done':
      return reduceDone(state, event, deps)
    case 'error':
      return reduceError(state, event, deps)
    case 'connection_closed': {
      const next = mutableState(state)
      next.activeTurnId = ''
      next.waiting = false
      next.streamingStarted = false
      next.pendingEditMessageId = ''
      const batch = currentBatch(next)
      if (batch) finalizeOpenReplyRuntimeState(batch, event.error || 'Execution cancelled.', deps.now())
      return next
    }
    case 'tool_call_start':
      return reduceToolStart(state, event.data, deps)
    case 'tool_call_review':
      return withCurrentBatch(state, (_next, batch) => {
        const item = batch.toolCalls.find(tool => tool.toolCallId === event.data.toolCallId)
        if (!item) return
        item.reviewStatus = event.data.reviewStatus
        item.reviewRisk = event.data.reviewRisk
        item.reviewReason = event.data.reviewReason
        if (event.data.reviewStatus === 'reviewing') item.status = 'reviewing'
        if (event.data.reviewStatus === 'approved') item.status = 'executing'
      })
    case 'tool_approval_required':
      return withCurrentBatch(state, (_next, batch) => {
        const item = batch.toolCalls.find(tool => tool.toolCallId === event.data.toolCallId)
        if (!item) return
        item.requiresApproval = event.data.requiresApproval
        item.reviewStatus = event.data.reviewStatus
        item.reviewRisk = event.data.reviewRisk
        item.reviewReason = event.data.reviewReason
        item.status = 'pending'
      })
    case 'tool_call_result':
      return reduceToolResult(state, event.data, deps)
    case 'subagent_start':
      return withCurrentBatch(state, (_next, batch) => {
        const parent = batch.toolCalls.find(tool => tool.toolCallId === event.data.parentToolCallId)
        if (parent) {
          parent.subagentRunId = event.data.subagentRunId
          parent.subagentTitle = event.data.title
          parent.subagentTask = event.data.task
          if (parent.subagentStream === undefined) parent.subagentStream = ''
          if (event.data.teamRunId) parent.teamRunId = event.data.teamRunId
          if (event.data.memberRunId) parent.memberRunId = event.data.memberRunId
        }
        if (event.data.teamRunId && event.data.memberRunId) {
          const teams = createAgentTeamState(batch.teamRuns)
          mergeAgentTeamMember(teams, {
            id: event.data.memberRunId,
            teamRunId: event.data.teamRunId,
            toolCallId: event.data.parentToolCallId,
            subagentRunId: event.data.subagentRunId,
            title: event.data.title,
            task: event.data.task,
            status: 'running',
            startedAt: new Date(deps.now()).toISOString(),
          })
          batch.teamRuns = teams.runs
        }
      })
    case 'subagent_chunk':
      return withCurrentBatch(state, (_next, batch) => {
        const parent = batch.toolCalls.find(tool => tool.toolCallId === event.data.parentToolCallId)
        if (!parent) return
        if (parent.subagentStream === undefined) parent.subagentStream = ''
        parent.subagentStream += event.data.content
      })
    case 'subagent_done':
      return withCurrentBatch(state, (_next, batch) => {
        finishSubagentThinking(batch, event.data.parentToolCallId, deps.now())
        if (event.data.error) {
          const parent = batch.toolCalls.find(tool => tool.toolCallId === event.data.parentToolCallId)
          if (parent && (parent.status === 'pending' || parent.status === 'reviewing' || parent.status === 'executing')) {
            markToolCallError(batch, event.data.parentToolCallId, event.data.error, deps.now())
          }
        }
        if (event.data.teamRunId && event.data.memberRunId) {
          const teams = createAgentTeamState(batch.teamRuns)
          mergeAgentTeamMember(teams, {
            id: event.data.memberRunId,
            teamRunId: event.data.teamRunId,
            toolCallId: event.data.parentToolCallId,
            subagentRunId: event.data.subagentRunId,
            title: '',
            task: '',
            status: event.data.error ? 'failed' : 'succeeded',
            error: event.data.error,
            finishedAt: new Date(deps.now()).toISOString(),
          })
          batch.teamRuns = teams.runs
        }
      })
    case 'team_start':
    case 'team_done':
      return withCurrentBatch(state, (_next, batch) => {
        const teams = createAgentTeamState(batch.teamRuns)
        mergeAgentTeamRun(teams, event.data)
        batch.teamRuns = teams.runs
      })
    case 'team_member_queued':
      return withCurrentBatch(state, (_next, batch) => {
        const teams = createAgentTeamState(batch.teamRuns)
        mergeAgentTeamMember(teams, event.data)
        batch.teamRuns = teams.runs
      })
    case 'thinking_start':
    case 'thinking_chunk':
    case 'thinking_done':
      return reduceThinking(state, event, deps)
    case 'plan_start':
      return withCurrentBatch(state, (next, batch) => {
        finishOpenThinkingEntries(batch, deps.now())
        next.planGenerating = true
      })
    case 'plan_chunk':
      return withCurrentBatch(state, (_next, batch) => appendPlanChunkToBatch(batch, event.chunk))
    case 'plan_body':
      return withCurrentBatch(state, (next, batch) => {
        appendPlanBodyToBatch(batch, event.content)
        next.planGenerating = false
      })
  }
}
