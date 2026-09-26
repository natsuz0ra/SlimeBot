import type { SessionItem } from '../api/chat'

export const MESSAGE_PLATFORM_SESSION_ID = 'im-platform-session'
const PREFIX = `${MESSAGE_PLATFORM_SESSION_ID}:`

export function platformFromSessionId(id: string | undefined): string | undefined {
  if (id === MESSAGE_PLATFORM_SESSION_ID) return 'telegram'
  if (!id?.startsWith(PREFIX)) return undefined
  try {
    return decodeURIComponent(id.slice(PREFIX.length)) || undefined
  } catch {
    return undefined
  }
}

export function isMessagePlatformSessionId(id: string | undefined): boolean {
  return platformFromSessionId(id) !== undefined
}

export function platformSessionName(id: string, telegramLabel: string): string {
  const platform = platformFromSessionId(id)
  return platform === 'telegram' ? telegramLabel : platform || id
}

export function listPlatformSessions(sessions: SessionItem[]): SessionItem[] {
  const platformSessions = sessions.filter((session) => isMessagePlatformSessionId(session.id))
  if (!platformSessions.some((session) => session.id === MESSAGE_PLATFORM_SESSION_ID)) {
    platformSessions.unshift({ id: MESSAGE_PLATFORM_SESSION_ID, name: 'telegram', updatedAt: '' })
  }
  return platformSessions
}
