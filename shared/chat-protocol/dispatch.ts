import type { BackgroundTaskData, ChatWSDiagnostic, ChatWSEvent, ContextUsageWire, RuntimeTodoItem, SkillLearningData, ToolCallStatus } from './events.js'

export type { BackgroundTaskData, RuntimeTodoItem, RuntimeTodoStatus, SkillLearningData } from './events.js'

export type ChatStartData = { startedAt?: string; turnId?: string; background?: boolean }
export type ChatDoneData = {
  isInterrupted?: boolean
  isStopPlaceholder?: boolean
  planId?: string
  planBody?: string
  narration?: string
  finishedAt?: string
  durationMs?: number
  skillLearning?: SkillLearningData
}
export type SteeringData = {
  submissionId: string
  messageId?: string
  turnId?: string
  content?: string
  createdAt?: string
  error?: string
}

export type ChatEventHandlers = {
  onBackgroundError?: (error: string, sessionId?: string) => void
  onBackgroundTask?: (task: BackgroundTaskData, sessionId?: string) => void
  onBackgroundTasks?: (tasks: BackgroundTaskData[], sessionId?: string) => void
  onSteering?: (data: SteeringData, sessionId?: string) => void

  onPrivacy?: (data: { masked: number; warned: number; blocked: boolean }, sessionId?: string) => void
  onSession?: (sessionId: string) => void
  onStart?: (sessionId?: string, meta?: ChatStartData) => void
  onChunk?: (chunk: string, sessionId?: string) => void
  onSessionTitle?: (title: string, sessionId?: string) => void
  onDone?: (sessionId?: string, answer?: string, meta?: ChatDoneData) => void
  onError?: (error: string, sessionId?: string) => void
  onMessageEdited?: (data: MessageEditedData, sessionId?: string) => void
  onToolCallStart?: (data: ToolCallStartData, sessionId?: string) => void
  onToolCallReview?: (data: ToolCallReviewData, sessionId?: string) => void
  onToolApprovalRequired?: (data: ToolApprovalRequiredData, sessionId?: string) => void
  onToolCallResult?: (data: ToolCallResultData, sessionId?: string) => void
  onTeamStart?: (data: TeamRunData, sessionId?: string) => void
  onTeamMemberQueued?: (data: TeamMemberData, sessionId?: string) => void
  onTeamDone?: (data: TeamRunData, sessionId?: string) => void
  onSubagentStart?: (data: SubagentStartData, sessionId?: string) => void
  onSubagentChunk?: (data: SubagentChunkData, sessionId?: string) => void
  onSubagentDone?: (data: SubagentDoneData, sessionId?: string) => void
  onThinkingStart?: (data: ThinkingEventData, sessionId?: string) => void
  onThinkingChunk?: (data: ThinkingEventData, sessionId?: string) => void
  onThinkingDone?: (data: ThinkingEventData, sessionId?: string) => void
  onPlanStart?: (sessionId?: string) => void
  onPlanChunk?: (chunk: string, sessionId?: string) => void
  onPlanBody?: (content: string, sessionId?: string) => void
  onTodoUpdate?: (data: TodoUpdateData, sessionId?: string) => void
  onContextUsage?: (data: ContextUsageData, sessionId?: string) => void
  onContextCompacted?: (data: ContextUsageData, sessionId?: string) => void
  onPlanStatus?: (planId: string, status: string) => void
  onStopping?: (sessionId?: string) => void
  onProtocolError?: (diagnostic: ChatWSDiagnostic) => void
  acceptSession?: (sessionId: string) => boolean
}

export interface MessageEditedData {
  messageId: string
  content: string
  createdAt?: string
}

export interface ToolCallStartData {
  toolCallId: string
  toolName: string
  command: string
  params: Record<string, unknown>
  requiresApproval: boolean
  reviewStatus?: string
  reviewRisk?: string
  reviewReason?: string
  preamble?: string
  startedAt?: string
  parentToolCallId?: string
  subagentRunId?: string
  teamRunId?: string
  memberRunId?: string
}

export interface ToolCallReviewData {
  toolCallId: string
  toolName: string
  command: string
  reviewStatus: string
  reviewRisk?: string
  reviewReason?: string
  parentToolCallId?: string
  subagentRunId?: string
  teamRunId?: string
  memberRunId?: string
}

export interface ToolApprovalRequiredData extends ToolCallReviewData {
  params: Record<string, unknown>
  requiresApproval: boolean
}

export interface ToolCallResultData {
  toolCallId: string
  toolName: string
  command: string
  requiresApproval: boolean
  status: ToolCallStatus
  output: string
  error: string
  metadata?: unknown
  finishedAt?: string
  parentToolCallId?: string
  subagentRunId?: string
  teamRunId?: string
  memberRunId?: string
}

export interface SubagentStartData {
  parentToolCallId: string
  subagentRunId: string
  title: string
  task: string
  teamRunId?: string
  memberRunId?: string
}

export interface SubagentChunkData {
  parentToolCallId: string
  subagentRunId: string
  content: string
  teamRunId?: string
  memberRunId?: string
}

export interface SubagentDoneData {
  parentToolCallId: string
  subagentRunId: string
  error?: string
  teamRunId?: string
  memberRunId?: string
}

export interface ThinkingEventData {
  content?: string
  startedAt?: string
  finishedAt?: string
  parentToolCallId?: string
  subagentRunId?: string
  teamRunId?: string
  memberRunId?: string
}

export interface TodoUpdateData {
  items: RuntimeTodoItem[]
  note?: string
  updatedAt?: string
}

export interface ContextLayerUsage {
  layer: string
  tokens: number
}

export interface ContextUsageData {
  sessionId: string
  modelConfigId: string
  usedTokens: number
  totalTokens: number
  usedPercent: number
  availablePercent: number
  isCompacted: boolean
  compactedAt?: string
  inputBudget?: number
  outputReserve?: number
  compactionBeforeTokens?: number
  compactionAfterTokens?: number
  compactionReason?: string
  compactionDurationMs?: number
  compactionFailure?: string
  layers?: ContextLayerUsage[]
  source?: 'estimated' | 'provider-reported'
  state?: 'compacting' | 'ready' | 'cooldown'
  outputBudgetTokens?: number
  outputBudgetUsed?: number
}

type TeamRunStatus = 'running' | 'succeeded' | 'partial_failed' | 'failed' | 'canceled' | 'interrupted'
type TeamMemberRunStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'

export interface TeamRunData {
  id: string
  sessionId: string
  requestId: string
  assistantMessageId?: string
  status: TeamRunStatus
  maxMembers: number
  maxParallel: number
  lastError?: string
  startedAt: string
  finishedAt?: string
  createdAt?: string
  updatedAt?: string
}

export interface TeamMemberData {
  id: string
  teamRunId: string
  toolCallId: string
  subagentRunId?: string
  title: string
  task: string
  modelConfigId?: string
  status: TeamMemberRunStatus
  answer?: string
  error?: string
  startedAt?: string
  finishedAt?: string
  createdAt?: string
  updatedAt?: string
}

function normalizeTeamRun(data: Extract<ChatWSEvent, { type: 'team_start' | 'team_done' }>): TeamRunData {
  return {
    id: data.teamRunId ?? data.id!,
    sessionId: data.sessionId || '',
    requestId: data.requestId || '',
    status: data.status === 'completed' ? 'succeeded' : (data.status || 'running'),
    maxMembers: data.maxMembers || 8,
    maxParallel: data.maxParallel || 4,
    lastError: data.lastError,
    startedAt: data.startedAt || data.createdAt || '',
    finishedAt: data.finishedAt,
    createdAt: data.createdAt || data.startedAt,
    updatedAt: data.updatedAt || data.finishedAt,
  }
}

function normalizeTeamMember(data: Extract<ChatWSEvent, { type: 'team_member_queued' }>): TeamMemberData {
  return {
    id: data.memberRunId,
    teamRunId: data.teamRunId,
    toolCallId: data.toolCallId || '',
    subagentRunId: data.subagentRunId,
    title: data.title || '',
    task: data.task || '',
    modelConfigId: data.modelConfigId,
    status: (data.status || 'queued'),
    answer: data.answer,
    error: data.error,
    startedAt: data.startedAt,
    finishedAt: data.finishedAt,
    createdAt: data.createdAt,
    updatedAt: data.updatedAt,
  }
}

function normalizeContextUsage(source: ContextUsageWire, fallbackSessionId?: string): ContextUsageData | null {
  const totalTokens = source.totalTokens
  const usedTokens = source.usedTokens
  if (totalTokens <= 0) return null
  const usedPercent = source.usedPercent ?? Math.round((usedTokens / totalTokens) * 100)
  const availablePercent = source.availablePercent ?? Math.max(0, 100 - usedPercent)
  return {
    sessionId: source.sessionId || fallbackSessionId || '',
    modelConfigId: source.modelConfigId || '',
    usedTokens: Math.max(0, Math.round(usedTokens)),
    totalTokens: Math.max(1, Math.round(totalTokens)),
    usedPercent: Math.max(0, Math.min(100, Math.round(usedPercent))),
    availablePercent: Math.max(0, Math.min(100, Math.round(availablePercent))),
    isCompacted: !!source.isCompacted,
    compactedAt: source.compactedAt,
    ...(source.inputBudget !== undefined ? { inputBudget: Math.max(0, source.inputBudget) } : {}),
    ...(source.outputReserve !== undefined ? { outputReserve: Math.max(0, source.outputReserve) } : {}),
    ...(source.compactionBeforeTokens !== undefined ? { compactionBeforeTokens: Math.max(0, source.compactionBeforeTokens) } : {}),
    ...(source.compactionAfterTokens !== undefined ? { compactionAfterTokens: Math.max(0, source.compactionAfterTokens) } : {}),
    ...(source.compactionReason ? { compactionReason: source.compactionReason } : {}),
    ...(source.compactionDurationMs !== undefined ? { compactionDurationMs: Math.max(0, source.compactionDurationMs) } : {}),
    ...(source.compactionFailure ? { compactionFailure: source.compactionFailure } : {}),
    ...(Array.isArray(source.layers) ? { layers: source.layers.map(({ layer, tokens }) => ({ layer, tokens: Math.max(0, Math.round(tokens)) })) } : {}),
    ...(source.source ? { source: source.source } : {}),
    ...(source.state ? { state: source.state } : {}),
    ...(source.outputBudgetTokens !== undefined ? { outputBudgetTokens: Math.max(0, source.outputBudgetTokens) } : {}),
    ...(source.outputBudgetUsed !== undefined ? { outputBudgetUsed: Math.max(0, source.outputBudgetUsed) } : {}),
  }
}

function agentIdentity(data: Pick<ThinkingEventData, 'parentToolCallId' | 'subagentRunId' | 'teamRunId' | 'memberRunId'>) {
  const { parentToolCallId, subagentRunId, teamRunId, memberRunId } = data
  return {
    parentToolCallId, subagentRunId,
    ...(teamRunId ? { teamRunId } : {}),
    ...(memberRunId ? { memberRunId } : {}),
  }
}

type ToolWire = Extract<ChatWSEvent, { type: 'tool_call_start' | 'tool_call_review' | 'tool_call_approval_required' | 'tool_call_result' }>

function normalizeToolIdentity(data: ToolWire) {
  return { ...agentIdentity(data), toolCallId: data.toolCallId, toolName: data.toolName || '', command: data.command || '' }
}

function normalizeToolReview(data: Exclude<ToolWire, { type: 'tool_call_result' }>): ToolCallReviewData {
  return {
    ...normalizeToolIdentity(data), reviewStatus: data.reviewStatus || '',
    reviewRisk: data.reviewRisk || '', reviewReason: data.reviewReason || '',
  }
}

export function dispatchChatWSEvent(data: ChatWSEvent, handlers: ChatEventHandlers): void {
  if (data.type === 'pong') return
  if (data.type === 'session') {
    handlers.onSession?.(data.sessionId)
    return
  }
  if (data.sessionId && handlers.acceptSession && !handlers.acceptSession(data.sessionId)) return
  const sid = data.sessionId

  switch (data.type) {
    case 'background_error':
      handlers.onBackgroundError?.(data.error || 'Background continuation failed.', sid)
      break
    case 'background_tasks':
      handlers.onBackgroundTasks?.(data.tasks, sid)
      break
    case 'background_task':
      handlers.onBackgroundTask?.({ taskId: data.taskId, processId: data.processId || '', status: data.status }, sid)
      break
    case 'steer_accepted':
      handlers.onSteering?.({ ...data.change, submissionId: data.change.submissionId || '' }, sid)
      break
    case 'steer_rejected':
      handlers.onSteering?.({ submissionId: data.submissionId || '', error: data.error || 'Change rejected.', ...(data.content ? { content: data.content } : {}) }, sid)
      break
    case 'start':
      handlers.onStart?.(sid, { startedAt: data.startedAt, turnId: data.turnId, background: data.background })
      break
    case 'stopping':
      handlers.onStopping?.(sid)
      break
    case 'chunk':
      handlers.onChunk?.(data.content, sid)
      break
    case 'session_title':
      handlers.onSessionTitle?.(data.title, sid)
      break
    case 'message_edited':
      handlers.onMessageEdited?.({ messageId: data.messageId, content: data.content, createdAt: data.createdAt }, sid)
      break
    case 'done':
      handlers.onDone?.(sid, data.answer, {
        isInterrupted: data.isInterrupted, isStopPlaceholder: data.isStopPlaceholder,
        planId: data.planId, planBody: data.planBody, narration: data.narration,
        finishedAt: data.finishedAt, durationMs: data.durationMs, skillLearning: data.skillLearning,
      })
      break
    case 'error':
      handlers.onError?.(data.error || 'unknown error', sid)
      break
    case 'team_start':
      handlers.onTeamStart?.(normalizeTeamRun(data), sid)
      break
    case 'team_done':
      handlers.onTeamDone?.(normalizeTeamRun(data), sid)
      break
    case 'team_member_queued':
      handlers.onTeamMemberQueued?.(normalizeTeamMember(data), sid)
      break
    case 'tool_call_start':
      handlers.onToolCallStart?.({
        ...normalizeToolReview(data), params: data.params ?? {}, requiresApproval: data.requiresApproval ?? false,
        preamble: data.preamble || '', startedAt: data.startedAt,
      }, sid)
      break
    case 'tool_call_review':
      handlers.onToolCallReview?.(normalizeToolReview(data), sid)
      break
    case 'tool_call_approval_required':
      handlers.onToolApprovalRequired?.({
        ...normalizeToolReview(data), params: data.params ?? {}, requiresApproval: data.requiresApproval ?? false,
      }, sid)
      break
    case 'tool_call_result':
      handlers.onToolCallResult?.({
        ...normalizeToolIdentity(data), requiresApproval: data.requiresApproval ?? false, status: data.status || 'completed',
        output: data.output || '', error: data.error || '', metadata: data.metadata, finishedAt: data.finishedAt,
      }, sid)
      break
    case 'subagent_start':
      handlers.onSubagentStart?.({
        ...agentIdentity(data), parentToolCallId: data.parentToolCallId, subagentRunId: data.subagentRunId, title: data.title, task: data.task,
      }, sid)
      break
    case 'subagent_chunk':
      handlers.onSubagentChunk?.({
        ...agentIdentity(data), parentToolCallId: data.parentToolCallId, subagentRunId: data.subagentRunId, content: data.content,
      }, sid)
      break
    case 'subagent_done':
      handlers.onSubagentDone?.({
        ...agentIdentity(data), parentToolCallId: data.parentToolCallId, subagentRunId: data.subagentRunId, error: data.error,
      }, sid)
      break
    case 'thinking_start':
      handlers.onThinkingStart?.({ ...agentIdentity(data), startedAt: data.startedAt }, sid)
      break
    case 'thinking_chunk':
      handlers.onThinkingChunk?.({ ...agentIdentity(data), content: data.content, startedAt: data.startedAt }, sid)
      break
    case 'thinking_done':
      handlers.onThinkingDone?.({ ...agentIdentity(data), finishedAt: data.finishedAt }, sid)
      break
    case 'todo_update':
      handlers.onTodoUpdate?.({ items: data.items ?? [], note: data.note, updatedAt: data.updatedAt }, sid)
      break
    case 'plan_start':
      handlers.onPlanStart?.(sid)
      break
    case 'plan_chunk':
      handlers.onPlanChunk?.(data.content, sid)
      break
    case 'plan_body':
      handlers.onPlanBody?.(data.content, sid)
      break
    case 'plan_status':
      handlers.onPlanStatus?.(data.planId, data.status)
      break
    case 'context_usage': {
      const usage = normalizeContextUsage(data, sid)
      if (usage) handlers.onContextUsage?.(usage, sid)
      break
    }
    case 'context_compacted': {
      const usage = normalizeContextUsage(data.usage, sid)
      if (usage) handlers.onContextCompacted?.(usage, sid)
      break
    }
    case 'privacy':
      handlers.onPrivacy?.({ masked: Math.max(0, data.masked), warned: Math.max(0, data.warned), blocked: data.blocked }, sid)
      break
  }
}
