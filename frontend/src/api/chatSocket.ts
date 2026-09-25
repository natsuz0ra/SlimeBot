import { getAuthToken } from '../utils/authStorage'
import type { ToolCallStatus } from '@/types/chat'
import type { TeamMemberRunItem, TeamRunItem } from '@/api/chat'

export type ChatSocketHandlers = {
  onSession: (sessionId: string) => void
  onStart: (sessionId?: string, meta?: { startedAt?: string }) => void
  onChunk: (chunk: string, sessionId?: string) => void
  onSessionTitle: (title: string, sessionId?: string) => void
  onDone: (sessionId?: string, answer?: string, meta?: { isInterrupted?: boolean; isStopPlaceholder?: boolean; planId?: string; planBody?: string; finishedAt?: string; durationMs?: number }) => void
  onError: (error: string, sessionId?: string) => void
  onMessageEdited?: (data: MessageEditedData, sessionId?: string) => void
  onToolCallStart?: (data: ToolCallStartData, sessionId?: string) => void
  onToolCallReview?: (data: ToolCallReviewData, sessionId?: string) => void
  onToolApprovalRequired?: (data: ToolApprovalRequiredData, sessionId?: string) => void
  onToolCallResult?: (data: ToolCallResultData, sessionId?: string) => void
  onTeamStart?: (data: TeamRunItem, sessionId?: string) => void
  onTeamMemberQueued?: (data: TeamMemberRunItem, sessionId?: string) => void
  onTeamDone?: (data: TeamRunItem, sessionId?: string) => void
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
  onOpen?: () => void
  onClose?: () => void
  onSocketError?: (error: string) => void
  onStatusChange?: (status: ConnectionStatus, error?: string) => void
}

export type ConnectionStatus = 'connected' | 'reconnecting' | 'disconnected'

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

export type RuntimeTodoStatus = 'pending' | 'in_progress' | 'completed'

export interface RuntimeTodoItem {
  id: string
  content: string
  status: RuntimeTodoStatus
}

export interface TodoUpdateData {
  items: RuntimeTodoItem[]
  note?: string
  updatedAt?: string
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
}

type WSIncoming = {
  type: string
  id?: string
  sessionId?: string
  messageId?: string
  content?: string
  answer?: string
  title?: string
  error?: string
  toolCallId?: string
  toolName?: string
  command?: string
  params?: Record<string, unknown>
  requiresApproval?: boolean
  reviewStatus?: string
  reviewRisk?: string
  reviewReason?: string
  status?: ToolCallStatus
  preamble?: string
  output?: string
  metadata?: unknown
  createdAt?: string
  startedAt?: string
  finishedAt?: string
  updatedAt?: string
  durationMs?: number
  isInterrupted?: boolean
  isStopPlaceholder?: boolean
  parentToolCallId?: string
  subagentRunId?: string
  teamRunId?: string
  memberRunId?: string
  requestId?: string
  maxMembers?: number
  maxParallel?: number
  lastError?: string
  task?: string
  planId?: string
  planBody?: string
  items?: RuntimeTodoItem[]
  note?: string
  usage?: ContextUsageData
  modelConfigId?: string
  usedTokens?: number
  totalTokens?: number
  usedPercent?: number
  availablePercent?: number
  isCompacted?: boolean
  compactedAt?: string
}

function normalizeTeamRun(data: WSIncoming): TeamRunItem {
  return {
    id: data.teamRunId || data.id || '',
    sessionId: data.sessionId || '',
    requestId: data.requestId || '',
    status: data.status === 'completed' ? 'succeeded' : (data.status || 'running') as TeamRunItem['status'],
    maxMembers: Number(data.maxMembers) || 8,
    maxParallel: Number(data.maxParallel) || 4,
    lastError: data.lastError,
    startedAt: data.startedAt || data.createdAt || '',
    finishedAt: data.finishedAt,
    createdAt: data.createdAt || data.startedAt,
    updatedAt: data.updatedAt || data.finishedAt,
  }
}

function normalizeTeamMember(data: WSIncoming): TeamMemberRunItem {
  return {
    id: data.memberRunId || '',
    teamRunId: data.teamRunId || '',
    toolCallId: data.toolCallId || '',
    subagentRunId: data.subagentRunId,
    title: data.title || '',
    task: data.task || '',
    modelConfigId: data.modelConfigId,
    status: (data.status || 'queued') as TeamMemberRunItem['status'],
    answer: data.answer,
    error: data.error,
    startedAt: data.startedAt,
    finishedAt: data.finishedAt,
    createdAt: data.createdAt,
    updatedAt: data.updatedAt,
  }
}

function normalizeContextUsage(data: WSIncoming | ContextUsageData | undefined, fallbackSessionId?: string): ContextUsageData | null {
  if (!data) return null
  const source = 'usage' in data && data.usage ? data.usage : data
  const totalTokens = Number(source.totalTokens)
  const usedTokens = Number(source.usedTokens)
  if (!Number.isFinite(totalTokens) || totalTokens <= 0 || !Number.isFinite(usedTokens)) return null
  const usedPercent = Number.isFinite(Number(source.usedPercent))
    ? Number(source.usedPercent)
    : Math.round((usedTokens / totalTokens) * 100)
  const availablePercent = Number.isFinite(Number(source.availablePercent))
    ? Number(source.availablePercent)
    : Math.max(0, 100 - usedPercent)
  return {
    sessionId: String(source.sessionId || fallbackSessionId || ''),
    modelConfigId: String(source.modelConfigId || ''),
    usedTokens: Math.max(0, Math.round(usedTokens)),
    totalTokens: Math.max(1, Math.round(totalTokens)),
    usedPercent: Math.max(0, Math.min(100, Math.round(usedPercent))),
    availablePercent: Math.max(0, Math.min(100, Math.round(availablePercent))),
    isCompacted: !!source.isCompacted,
    compactedAt: source.compactedAt,
  }
}

export function dispatchChatSocketMessage(raw: string, handlers: ChatSocketHandlers | null): boolean {
  let data: WSIncoming
  try {
    data = JSON.parse(raw) as WSIncoming
  } catch {
    return false
  }

  if (data.type === 'pong') return true

  if (data.type === 'session' && data.sessionId) handlers?.onSession(data.sessionId)
  if (data.type === 'start') handlers?.onStart(data.sessionId, { startedAt: data.startedAt })
  if (data.type === 'chunk') handlers?.onChunk(data.content || '', data.sessionId)
  if (data.type === 'session_title') handlers?.onSessionTitle(data.title || '', data.sessionId)
  if (data.type === 'message_edited') {
    handlers?.onMessageEdited?.({
      messageId: data.messageId || '',
      content: data.content || '',
      createdAt: data.createdAt,
    }, data.sessionId)
  }
  if (data.type === 'done') {
    handlers?.onDone(data.sessionId, data.answer, {
      isInterrupted: data.isInterrupted,
      isStopPlaceholder: data.isStopPlaceholder,
      planId: data.planId,
      planBody: data.planBody,
      finishedAt: data.finishedAt,
      durationMs: data.durationMs,
    })
  }
  if (data.type === 'error') handlers?.onError(data.error || 'unknown error', data.sessionId)
  if (data.type === 'team_start') handlers?.onTeamStart?.(normalizeTeamRun(data), data.sessionId)
  if (data.type === 'team_member_queued') handlers?.onTeamMemberQueued?.(normalizeTeamMember(data), data.sessionId)
  if (data.type === 'team_done') handlers?.onTeamDone?.(normalizeTeamRun(data), data.sessionId)

  if (data.type === 'tool_call_start') {
    handlers?.onToolCallStart?.({
      toolCallId: data.toolCallId || '',
      toolName: data.toolName || '',
      command: data.command || '',
      params: data.params || {},
      requiresApproval: !!data.requiresApproval,
      reviewStatus: data.reviewStatus || '',
      reviewRisk: data.reviewRisk || '',
      reviewReason: data.reviewReason || '',
      preamble: data.preamble || '',
      startedAt: data.startedAt,
      parentToolCallId: data.parentToolCallId,
      subagentRunId: data.subagentRunId,
      teamRunId: data.teamRunId,
      memberRunId: data.memberRunId,
    }, data.sessionId)
  }

  if (data.type === 'tool_call_review') {
    handlers?.onToolCallReview?.({
      toolCallId: data.toolCallId || '',
      toolName: data.toolName || '',
      command: data.command || '',
      reviewStatus: data.reviewStatus || '',
      reviewRisk: data.reviewRisk || '',
      reviewReason: data.reviewReason || '',
      parentToolCallId: data.parentToolCallId,
      subagentRunId: data.subagentRunId,
      teamRunId: data.teamRunId,
      memberRunId: data.memberRunId,
    }, data.sessionId)
  }

  if (data.type === 'tool_call_approval_required') {
    handlers?.onToolApprovalRequired?.({
      toolCallId: data.toolCallId || '',
      toolName: data.toolName || '',
      command: data.command || '',
      params: data.params || {},
      requiresApproval: !!data.requiresApproval,
      reviewStatus: data.reviewStatus || '',
      reviewRisk: data.reviewRisk || '',
      reviewReason: data.reviewReason || '',
      parentToolCallId: data.parentToolCallId,
      subagentRunId: data.subagentRunId,
      teamRunId: data.teamRunId,
      memberRunId: data.memberRunId,
    }, data.sessionId)
  }

  if (data.type === 'tool_call_result') {
    handlers?.onToolCallResult?.({
      toolCallId: data.toolCallId || '',
      toolName: data.toolName || '',
      command: data.command || '',
      requiresApproval: !!data.requiresApproval,
      status: data.status || 'completed',
      output: data.output || '',
      error: data.error || '',
      metadata: data.metadata,
      finishedAt: data.finishedAt,
      parentToolCallId: data.parentToolCallId,
      subagentRunId: data.subagentRunId,
      teamRunId: data.teamRunId,
      memberRunId: data.memberRunId,
    }, data.sessionId)
  }

  if (data.type === 'subagent_start') {
    handlers?.onSubagentStart?.({
      parentToolCallId: data.parentToolCallId || '',
      subagentRunId: data.subagentRunId || '',
      title: data.title || '',
      task: data.task || '',
      teamRunId: data.teamRunId,
      memberRunId: data.memberRunId,
    }, data.sessionId)
  }

  if (data.type === 'subagent_chunk') {
    handlers?.onSubagentChunk?.({
      parentToolCallId: data.parentToolCallId || '',
      subagentRunId: data.subagentRunId || '',
      content: data.content || '',
      teamRunId: data.teamRunId,
      memberRunId: data.memberRunId,
    }, data.sessionId)
  }

  if (data.type === 'subagent_done') {
    handlers?.onSubagentDone?.({
      parentToolCallId: data.parentToolCallId || '',
      subagentRunId: data.subagentRunId || '',
      error: data.error,
      teamRunId: data.teamRunId,
      memberRunId: data.memberRunId,
    }, data.sessionId)
  }

  if (data.type === 'thinking_start') handlers?.onThinkingStart?.({
    startedAt: data.startedAt,
    parentToolCallId: data.parentToolCallId,
    subagentRunId: data.subagentRunId,
    teamRunId: data.teamRunId,
    memberRunId: data.memberRunId,
  }, data.sessionId)
  if (data.type === 'thinking_chunk') handlers?.onThinkingChunk?.({
    content: data.content || '',
    startedAt: data.startedAt,
    parentToolCallId: data.parentToolCallId,
    subagentRunId: data.subagentRunId,
    teamRunId: data.teamRunId,
    memberRunId: data.memberRunId,
  }, data.sessionId)
  if (data.type === 'thinking_done') handlers?.onThinkingDone?.({
    finishedAt: data.finishedAt,
    parentToolCallId: data.parentToolCallId,
    subagentRunId: data.subagentRunId,
    teamRunId: data.teamRunId,
    memberRunId: data.memberRunId,
  }, data.sessionId)
  if (data.type === 'todo_update') handlers?.onTodoUpdate?.({
    items: Array.isArray(data.items) ? data.items : [],
    note: data.note,
    updatedAt: data.updatedAt,
  }, data.sessionId)
  if (data.type === 'plan_start') handlers?.onPlanStart?.(data.sessionId)
  if (data.type === 'plan_chunk') handlers?.onPlanChunk?.(data.content || '', data.sessionId)
  if (data.type === 'plan_body') handlers?.onPlanBody?.(data.content || '', data.sessionId)
  if (data.type === 'context_usage') {
    const usage = normalizeContextUsage(data, data.sessionId)
    if (usage) handlers?.onContextUsage?.(usage, data.sessionId)
  }
  if (data.type === 'context_compacted') {
    const usage = normalizeContextUsage(data.usage, data.sessionId)
    if (usage) handlers?.onContextCompacted?.(usage, data.sessionId)
  }

  return true
}

export class ChatSocket {
  private ws: WebSocket | null = null
  private handlers: ChatSocketHandlers | null = null
  private reconnectTimer: number | null = null
  private heartbeatTimer: number | null = null
  private heartbeatTimeoutTimer: number | null = null
  private reconnectAttempt = 0
  private manualClose = false

  private readonly HEARTBEAT_INTERVAL_MS = 60_000
  private readonly HEARTBEAT_TIMEOUT_MS = 10_000
  private readonly RECONNECT_BASE_DELAY_MS = 1_000
  private readonly RECONNECT_MAX_DELAY_MS = 15_000

  connect(handlers: ChatSocketHandlers) {
    this.handlers = handlers
    this.manualClose = false
    this.clearReconnectTimer()

    if (this.ws) {
      this.teardownSocket()
    }
    this.openSocket()
  }

  send(content: string, sessionId: string, modelId: string, attachmentIds?: string[], thinkingLevel?: string, planMode?: boolean, subagentModelId?: string) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      this.handlers?.onSocketError?.('socket is not connected')
      return false
    }
    this.ws.send(JSON.stringify({ type: 'chat', content, sessionId, modelId, attachmentIds: attachmentIds || [], thinkingLevel: thinkingLevel || 'off', planMode: !!planMode, subagentModelId: subagentModelId || '' }))
    return true
  }

  sendEdit(messageId: string, content: string, sessionId: string, modelId: string, thinkingLevel?: string, planMode?: boolean, subagentModelId?: string) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      this.handlers?.onSocketError?.('socket is not connected')
      return false
    }
    this.ws.send(JSON.stringify({ type: 'chat_edit', messageId, content, sessionId, modelId, thinkingLevel: thinkingLevel || 'off', planMode: !!planMode, subagentModelId: subagentModelId || '' }))
    return true
  }

  sendStop(sessionId: string) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      this.handlers?.onSocketError?.('socket is not connected')
      return false
    }
    this.ws.send(JSON.stringify({ type: 'stop', sessionId }))
    return true
  }

  sendToolApproval(toolCallId: string, approved: boolean, answers?: string) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      this.handlers?.onSocketError?.('socket is not connected')
      return false
    }
    const payload: Record<string, unknown> = { type: 'tool_approve', toolCallId, approved }
    if (answers) payload.answers = answers
    this.ws.send(JSON.stringify(payload))
    return true
  }

  sendPlanApprove(planId: string, sessionId: string, modelId: string, displayContent?: string) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      this.handlers?.onSocketError?.('socket is not connected')
      return false
    }
    this.ws.send(JSON.stringify({ type: 'plan_approve', planId, sessionId, modelId, displayContent: displayContent || '' }))
    return true
  }

  sendPlanReject(planId: string, sessionId: string) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      this.handlers?.onSocketError?.('socket is not connected')
      return false
    }
    this.ws.send(JSON.stringify({ type: 'plan_reject', planId, sessionId }))
    return true
  }

  close() {
    this.manualClose = true
    this.clearReconnectTimer()
    this.clearHeartbeat()
    if (this.ws) {
      this.ws.close()
      this.ws = null
    }
    this.emitStatus('disconnected')
  }

  private openSocket() {
    const token = getAuthToken()
    if (!token && !window.slimebotDesktop) {
      this.emitStatus('disconnected', 'missing auth token')
      this.handlers?.onSocketError?.('missing auth token')
      return
    }

    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsBase = import.meta.env.VITE_WS_URL || `${protocol}//${location.host}`
    const url = window.slimebotDesktop
      ? `${wsBase}/ws/chat`
      : `${wsBase}/ws/chat?${new URLSearchParams({ token }).toString()}`
    this.emitStatus('reconnecting')
    this.ws = new WebSocket(url)

    this.ws.onopen = () => {
      this.reconnectAttempt = 0
      this.emitStatus('connected')
      this.handlers?.onOpen?.()
      this.startHeartbeat()
    }

    this.ws.onmessage = (event) => {
      try {
        const heartbeat = JSON.parse(event.data) as WSIncoming
        if (heartbeat.type === 'pong') {
          this.clearHeartbeatTimeout()
          return
        }
      } catch {
        // Let the shared dispatcher report malformed payloads below.
      }
      if (!dispatchChatSocketMessage(event.data, this.handlers)) {
        this.handlers?.onSocketError?.('invalid websocket payload')
      }
    }

    this.ws.onerror = () => {
      this.handlers?.onSocketError?.('websocket error')
    }

    this.ws.onclose = () => {
      this.clearHeartbeat()
      this.ws = null
      this.handlers?.onClose?.()
      if (this.manualClose) return

      this.emitStatus('reconnecting', 'socket closed unexpectedly')
      this.scheduleReconnect()
    }
  }

  private startHeartbeat() {
    this.clearHeartbeat()
    this.heartbeatTimer = window.setInterval(() => {
      this.sendHeartbeat()
    }, this.HEARTBEAT_INTERVAL_MS)
  }

  private sendHeartbeat() {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return
    try {
      this.ws.send(JSON.stringify({ type: 'ping' }))
      this.clearHeartbeatTimeout()
      this.heartbeatTimeoutTimer = window.setTimeout(() => {
        this.handlers?.onSocketError?.('heartbeat timeout')
        this.ws?.close()
      }, this.HEARTBEAT_TIMEOUT_MS)
    } catch {
      this.handlers?.onSocketError?.('heartbeat send failed')
      this.ws.close()
    }
  }

  private scheduleReconnect() {
    if (this.manualClose) return
    this.clearReconnectTimer()
    const delay = Math.min(this.RECONNECT_BASE_DELAY_MS * 2 ** this.reconnectAttempt, this.RECONNECT_MAX_DELAY_MS)
    this.reconnectAttempt += 1
    this.reconnectTimer = window.setTimeout(() => {
      this.emitStatus('reconnecting')
      this.openSocket()
    }, delay)
  }

  private clearReconnectTimer() {
    if (this.reconnectTimer !== null) {
      window.clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
  }

  private clearHeartbeatTimeout() {
    if (this.heartbeatTimeoutTimer !== null) {
      window.clearTimeout(this.heartbeatTimeoutTimer)
      this.heartbeatTimeoutTimer = null
    }
  }

  private clearHeartbeat() {
    if (this.heartbeatTimer !== null) {
      window.clearInterval(this.heartbeatTimer)
      this.heartbeatTimer = null
    }
    this.clearHeartbeatTimeout()
  }

  private teardownSocket() {
    if (!this.ws) return
    this.ws.onopen = null
    this.ws.onmessage = null
    this.ws.onclose = null
    this.ws.onerror = null
    this.ws.close()
    this.ws = null
    this.clearHeartbeat()
  }

  private emitStatus(status: ConnectionStatus, error?: string) {
    this.handlers?.onStatusChange?.(status, error)
  }
}
