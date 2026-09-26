import type { SessionItem } from '@/api/chat'

export interface SessionGroup {
  path: string
  name: string
  sessions: SessionItem[]
}

export function groupSessionsByDirectory(sessions: SessionItem[], unclassifiedLabel: string, excludedId = ''): SessionGroup[] {
  const groups = new Map<string, SessionGroup>()
  for (const session of sessions) {
    if (session.id === excludedId) continue
    const path = session.workingDirectory || ''
    if (!groups.has(path)) {
      const clean = path.replace(/[\\/]+$/, '') || path
      const name = path ? clean.split(/[\\/]/).filter(Boolean).pop() || clean : unclassifiedLabel
      groups.set(path, { path, name, sessions: [] })
    }
    groups.get(path)!.sessions.push(session)
  }
  return [...groups.values()].sort((a, b) => a.path === '' ? 1 : b.path === '' ? -1 : 0)
}
