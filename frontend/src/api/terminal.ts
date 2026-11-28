import { apiClient } from '@/api/client'
import { getAuthToken } from '@/utils/authStorage'

export interface TerminalOptions {
  chatSessionId?: string
  cwd?: string
  shell?: string
  cols?: number
  rows?: number
}

export interface TerminalSession extends Required<TerminalOptions> {
  id: string
  title: string
  status: 'running' | 'exited'
  exitCode?: number
  createdAt: string
  lastActiveAt: string
}

export interface TerminalSettings {
  enabled: boolean
  defaultShell: string
  lockedByEnv: boolean
}

export interface TerminalStatus extends TerminalSettings {
  detectedShell: string
  maxSessions: number
  sessions: TerminalSession[]
}

export const getTerminalStatus = async () => (await apiClient.get<TerminalStatus>('/api/terminal/status')).data
export const updateTerminalSettings = (settings: Partial<Pick<TerminalSettings, 'enabled' | 'defaultShell'>>) =>
  apiClient.put('/api/terminal/settings', settings)
export const createTerminal = async (options: TerminalOptions) =>
  (await apiClient.post<TerminalSession>('/api/terminal/sessions', options)).data
export const deleteTerminal = (id: string) => apiClient.delete(`/api/terminal/sessions/${encodeURIComponent(id)}`)

type TerminalFrame =
  | { type: 'ready'; session: TerminalSession }
  | { type: 'exit'; exitCode: number }
  | { type: 'error'; message: string }
  | { type: 'pong' }

export function connectTerminal(id: string, handlers: {
  onData: (data: Uint8Array) => void
  onFrame: (frame: TerminalFrame) => void
  onReconnect: (attempt: number) => void
}) {
  const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
  const base = import.meta.env.VITE_WS_URL || `${protocol}//${location.host}`
  const query = new URLSearchParams({ terminalId: id })
  if (!window.slimebotDesktop) query.set('token', getAuthToken())
  const url = `${base}/ws/terminal?${query}`
  let ws: WebSocket | undefined
  let stopped = false
  let attempt = 0
  let retry: ReturnType<typeof setTimeout> | undefined
  let heartbeat: ReturnType<typeof setInterval> | undefined
  function connect() {
    if (stopped) return
    const connection = new WebSocket(url)
    ws = connection
    connection.binaryType = 'arraybuffer'
    connection.onopen = () => {
      heartbeat = setInterval(() => send(JSON.stringify({ type: 'ping' })), 25000)
    }
    connection.onmessage = ({ data }) => {
      if (stopped) return
      if (data instanceof ArrayBuffer) {
        handlers.onData(new Uint8Array(data))
        return
      }
      const frame = JSON.parse(data) as TerminalFrame
      if (frame.type === 'ready') attempt = 0
      if (frame.type === 'exit') stopped = true
      handlers.onFrame(frame)
    }
    connection.onclose = () => {
      clearInterval(heartbeat)
      if (stopped) return
      attempt++
      handlers.onReconnect(attempt)
      if (!stopped) retry = setTimeout(connect, Math.min(500 * 2 ** (attempt - 1), 10000))
    }
  }
  function send(data: string | Uint8Array<ArrayBuffer>) {
    if (stopped || ws?.readyState !== WebSocket.OPEN) return false
    if (ws.bufferedAmount > 1 << 20) return false
    ws.send(data)
    return true
  }
  connect()
  return {
    input: (data: string) => send(new TextEncoder().encode(data)),
    resize: (cols: number, rows: number) => send(JSON.stringify({ type: 'resize', cols, rows })),
    close: () => {
      stopped = true
      clearTimeout(retry)
      clearInterval(heartbeat)
      ws?.close()
    },
  }
}
