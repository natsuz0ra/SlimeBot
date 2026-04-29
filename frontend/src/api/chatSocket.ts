import { getAuthToken } from '@/utils/authStorage'
import type { ToolCallStatus } from '@/types/chat'

type Handlers = {
  onSession: (sessionId: string) => void
  onStart: (sessionId?: string, meta?: { startedAt?: string }) => void
  onChunk: (chunk: string, sessionId?: string) => void
  onSessionTitle: (title: string, sessionId?: string) => void
  onDone: (sessionId?: string, answer?: string, meta?: { isInterrupted?: boolean; isStopPlaceholder?: boolean; planId?: string; planBody?: string; finishedAt?: string; durationMs?: number }) => void
  onError: (error: string, sessionId?: string) => void
  onToolCallStart?: (data: ToolCallStartData, sessionId?: string) => void
  onToolCallResult?: (data: ToolCallResultData, sessionId?: string) => void
  onSubagentStart?: (data: SubagentStartData, sessionId?: string) => void
  onSubagentChunk?: (data: SubagentChunkData, sessionId?: string) => void
  onSubagentDone?: (data: SubagentDoneData, sessionId?: string) => void
  onThinkingStart?: (data: ThinkingEventData, sessionId?: string) => void
  onThinkingChunk?: (data: ThinkingEventData, sessionId?: string) => void
  onThinkingDone?: (data: ThinkingEventData, sessionId?: string) => void
  onPlanStart?: (sessionId?: string) => void
  onPlanChunk?: (chunk: string, sessionId?: string) => void
  onPlanBody?: (content: string, sessionId?: string) => void
  onOpen?: () => void
  onClose?: () => void
  onSocketError?: (error: string) => void
  onStatusChange?: (status: ConnectionStatus, error?: string) => void
}

export type ConnectionStatus = 'connected' | 'reconnecting' | 'disconnected'

export interface ToolCallStartData {
  toolCallId: string
  toolName: string
  command: string
  params: Record<string, string>
  requiresApproval: boolean
  preamble?: string
  startedAt?: string
  parentToolCallId?: string
  subagentRunId?: string
}

export interface ToolCallResultData {
  toolCallId: string
  toolName: string
  command: string
  requiresApproval: boolean
  status: ToolCallStatus
  output: string
  error: string
  finishedAt?: string
  parentToolCallId?: string
  subagentRunId?: string
}

export interface SubagentStartData {
  parentToolCallId: string
  subagentRunId: string
  task: string
}

export interface SubagentChunkData {
  parentToolCallId: string
  subagentRunId: string
  content: string
}

export interface SubagentDoneData {
  parentToolCallId: string
  subagentRunId: string
  error?: string
}

export interface ThinkingEventData {
  content?: string
  startedAt?: string
  finishedAt?: string
  parentToolCallId?: string
  subagentRunId?: string
}

type WSIncoming = {
  type: string
  sessionId?: string
  content?: string
  answer?: string
  title?: string
  error?: string
  toolCallId?: string
  toolName?: string
  command?: string
  params?: Record<string, string>
  requiresApproval?: boolean
  status?: ToolCallStatus
  preamble?: string
  output?: string
  startedAt?: string
  finishedAt?: string
  durationMs?: number
  isInterrupted?: boolean
  isStopPlaceholder?: boolean
  parentToolCallId?: string
  subagentRunId?: string
  task?: string
  planId?: string
  planBody?: string
}

export class ChatSocket {
  private ws: WebSocket | null = null
  private handlers: Handlers | null = null
  private reconnectTimer: number | null = null
  private heartbeatTimer: number | null = null
  private heartbeatTimeoutTimer: number | null = null
  private reconnectAttempt = 0
  private manualClose = false

  private readonly HEARTBEAT_INTERVAL_MS = 60_000
  private readonly HEARTBEAT_TIMEOUT_MS = 10_000
  private readonly RECONNECT_BASE_DELAY_MS = 1_000
  private readonly RECONNECT_MAX_DELAY_MS = 15_000

  connect(handlers: Handlers) {
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

  sendPlanModify(planId: string, sessionId: string, modelId: string, content: string, thinkingLevel: string) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      this.handlers?.onSocketError?.('socket is not connected')
      return false
    }
    this.ws.send(JSON.stringify({ type: 'plan_modify', planId, sessionId, modelId, content, thinkingLevel }))
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
    if (!token) {
      this.emitStatus('disconnected', 'missing auth token')
      this.handlers?.onSocketError?.('missing auth token')
      return
    }

    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsBase = import.meta.env.VITE_WS_URL || `${protocol}//${location.host}`
    const query = new URLSearchParams({ token })
    const url = `${wsBase}/ws/chat?${query.toString()}`
    this.emitStatus('reconnecting')
    this.ws = new WebSocket(url)

    this.ws.onopen = () => {
      this.reconnectAttempt = 0
      this.emitStatus('connected')
      this.handlers?.onOpen?.()
      this.startHeartbeat()
    }

    this.ws.onmessage = (event) => {
      let data: WSIncoming
      try {
        data = JSON.parse(event.data) as WSIncoming
      } catch {
        this.handlers?.onSocketError?.('invalid websocket payload')
        return
      }

      if (data.type === 'pong') {
        this.clearHeartbeatTimeout()
        return
      }

      if (data.type === 'session' && data.sessionId) this.handlers?.onSession(data.sessionId)
      if (data.type === 'start') this.handlers?.onStart(data.sessionId, { startedAt: data.startedAt })
      if (data.type === 'chunk') this.handlers?.onChunk(data.content || '', data.sessionId)
      if (data.type === 'session_title') this.handlers?.onSessionTitle(data.title || '', data.sessionId)
      if (data.type === 'done') {
        this.handlers?.onDone(data.sessionId, data.answer, {
          isInterrupted: data.isInterrupted,
          isStopPlaceholder: data.isStopPlaceholder,
          planId: data.planId,
          planBody: data.planBody,
          finishedAt: data.finishedAt,
          durationMs: data.durationMs,
        })
      }
      if (data.type === 'error') this.handlers?.onError(data.error || 'unknown error', data.sessionId)

      if (data.type === 'tool_call_start') {
        this.handlers?.onToolCallStart?.({
          toolCallId: data.toolCallId || '',
          toolName: data.toolName || '',
          command: data.command || '',
          params: data.params || {},
          requiresApproval: !!data.requiresApproval,
          preamble: data.preamble || '',
          startedAt: data.startedAt,
          parentToolCallId: data.parentToolCallId,
          subagentRunId: data.subagentRunId,
        }, data.sessionId)
      }

      if (data.type === 'tool_call_result') {
        this.handlers?.onToolCallResult?.({
          toolCallId: data.toolCallId || '',
          toolName: data.toolName || '',
          command: data.command || '',
          requiresApproval: !!data.requiresApproval,
          status: data.status || 'completed',
          output: data.output || '',
          error: data.error || '',
          finishedAt: data.finishedAt,
          parentToolCallId: data.parentToolCallId,
          subagentRunId: data.subagentRunId,
        }, data.sessionId)
      }

      if (data.type === 'subagent_start') {
        this.handlers?.onSubagentStart?.({
          parentToolCallId: data.parentToolCallId || '',
          subagentRunId: data.subagentRunId || '',
          task: data.task || '',
        }, data.sessionId)
      }

      if (data.type === 'subagent_chunk') {
        this.handlers?.onSubagentChunk?.({
          parentToolCallId: data.parentToolCallId || '',
          subagentRunId: data.subagentRunId || '',
          content: data.content || '',
        }, data.sessionId)
      }

      if (data.type === 'subagent_done') {
        this.handlers?.onSubagentDone?.({
          parentToolCallId: data.parentToolCallId || '',
          subagentRunId: data.subagentRunId || '',
          error: data.error,
        }, data.sessionId)
      }

      if (data.type === 'thinking_start') this.handlers?.onThinkingStart?.({
        startedAt: data.startedAt,
        parentToolCallId: data.parentToolCallId,
        subagentRunId: data.subagentRunId,
      }, data.sessionId)
      if (data.type === 'thinking_chunk') this.handlers?.onThinkingChunk?.({
        content: data.content || '',
        startedAt: data.startedAt,
        parentToolCallId: data.parentToolCallId,
        subagentRunId: data.subagentRunId,
      }, data.sessionId)
      if (data.type === 'thinking_done') this.handlers?.onThinkingDone?.({
        finishedAt: data.finishedAt,
        parentToolCallId: data.parentToolCallId,
        subagentRunId: data.subagentRunId,
      }, data.sessionId)
      if (data.type === 'plan_start') this.handlers?.onPlanStart?.(data.sessionId)
      if (data.type === 'plan_chunk') this.handlers?.onPlanChunk?.(data.content || '', data.sessionId)
      if (data.type === 'plan_body') this.handlers?.onPlanBody?.(data.content || '', data.sessionId)
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
