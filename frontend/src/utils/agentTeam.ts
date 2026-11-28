import type { TeamMemberRunItem, TeamRunItem, ToolCallItem } from '@/api/chat'

export interface AgentTeamMember extends TeamMemberRunItem {
  createdAt: string
  updatedAt: string
}

export interface AgentTeamRun extends TeamRunItem {
  createdAt: string
  updatedAt: string
  members: AgentTeamMember[]
}

export interface AgentTeamState {
  runs: AgentTeamRun[]
}

export function createAgentTeamState(runs: AgentTeamRun[] = []): AgentTeamState {
  return { runs: [...runs].sort(compareRuns) }
}

function compareTimeAndID(leftAt: string | undefined, leftID: string, rightAt: string | undefined, rightID: string) {
  const leftTime = Date.parse(leftAt || '')
  const rightTime = Date.parse(rightAt || '')
  const normalizedLeft = Number.isFinite(leftTime) ? leftTime : 0
  const normalizedRight = Number.isFinite(rightTime) ? rightTime : 0
  return normalizedLeft - normalizedRight || leftID.localeCompare(rightID)
}

function compareRuns(left: AgentTeamRun, right: AgentTeamRun) {
  return compareTimeAndID(left.createdAt || left.startedAt, left.id, right.createdAt || right.startedAt, right.id)
}

function compareMembers(left: AgentTeamMember, right: AgentTeamMember) {
  return compareTimeAndID(left.createdAt, left.id, right.createdAt, right.id)
}

function mergeText(previous: string | undefined, next: string | undefined) {
  return next === undefined || next === '' ? previous : next
}

function placeholderTeam(teamRunId: string, member?: TeamMemberRunItem): AgentTeamRun {
  const createdAt = member?.createdAt || member?.startedAt || ''
  return {
    id: teamRunId,
    sessionId: '',
    requestId: '',
    status: 'running',
    maxMembers: 8,
    maxParallel: 4,
    startedAt: createdAt,
    createdAt,
    updatedAt: member?.updatedAt || createdAt,
    members: [],
  }
}

export function mergeAgentTeamRun(state: AgentTeamState, incoming: TeamRunItem | AgentTeamRun) {
  const existing = state.runs.find((run) => run.id === incoming.id)
  if (!existing) {
    const createdAt = incoming.createdAt || incoming.startedAt || ''
    state.runs.push({
      ...incoming,
      createdAt,
      updatedAt: incoming.updatedAt || incoming.finishedAt || createdAt,
      members: 'members' in incoming ? [...incoming.members].sort(compareMembers) : [],
    })
    state.runs.sort(compareRuns)
    return
  }

  const members = existing.members
  Object.assign(existing, incoming, {
    sessionId: mergeText(existing.sessionId, incoming.sessionId) || '',
    requestId: mergeText(existing.requestId, incoming.requestId) || '',
    createdAt: incoming.createdAt || existing.createdAt || incoming.startedAt,
    updatedAt: incoming.updatedAt || incoming.finishedAt || existing.updatedAt,
    members,
  })
  if ('members' in incoming) {
    for (const member of incoming.members) mergeAgentTeamMember(state, member)
  }
  state.runs.sort(compareRuns)
}

export function mergeAgentTeamMember(state: AgentTeamState, incoming: TeamMemberRunItem | AgentTeamMember) {
  let run = state.runs.find((item) => item.id === incoming.teamRunId)
  if (!run) {
    run = placeholderTeam(incoming.teamRunId, incoming)
    state.runs.push(run)
  }
  const existing = run.members.find((member) => member.id === incoming.id)
  if (!existing) {
    const createdAt = incoming.createdAt || incoming.startedAt || ''
    run.members.push({
      ...incoming,
      title: incoming.title || '',
      task: incoming.task || '',
      createdAt,
      updatedAt: incoming.updatedAt || incoming.finishedAt || createdAt,
    })
  } else {
    Object.assign(existing, incoming, {
      title: mergeText(existing.title, incoming.title) || '',
      task: mergeText(existing.task, incoming.task) || '',
      subagentRunId: mergeText(existing.subagentRunId, incoming.subagentRunId),
      createdAt: incoming.createdAt || existing.createdAt || incoming.startedAt || '',
      updatedAt: incoming.updatedAt || incoming.finishedAt || existing.updatedAt,
    })
  }
  run.members.sort(compareMembers)
  state.runs.sort(compareRuns)
}

export function selectAgentTeamMembers(state: AgentTeamState, teamRunId: string) {
  return state.runs.find((run) => run.id === teamRunId)?.members ?? []
}

export function findAgentTeamPendingTool(member: TeamMemberRunItem, tools: ToolCallItem[]) {
  return tools.find((tool) => tool.status === 'pending' && (
    tool.toolCallId === member.toolCallId || tool.parentToolCallId === member.toolCallId
  ))
}

export function selectVisibleAgentTeamMembers(
  members: AgentTeamMember[],
  tools: ToolCallItem[],
  limit: number,
) {
  const statusPriority = {
    failed: 1,
    running: 2,
    queued: 3,
    interrupted: 4,
    canceled: 4,
    succeeded: 5,
  } as const

  return members
    .map((member, index) => ({
      member,
      index,
      priority: findAgentTeamPendingTool(member, tools) ? 0 : statusPriority[member.status],
    }))
    .sort((left, right) => left.priority - right.priority || left.index - right.index)
    .slice(0, Math.max(0, limit))
    .map(({ member }) => member)
}

export function buildAgentTeamResultPreview(value: string | undefined, maxLength = 320) {
  const firstParagraph = (value || '').trim().split(/\n\s*\n/, 1)[0]!.replace(/\s+/g, ' ') || ''
  if (firstParagraph.length <= maxLength) return firstParagraph
  if (maxLength <= 0) return ''
  return `${firstParagraph.slice(0, Math.max(0, maxLength - 1))}…`
}

export function countAgentTeamMemberTools(parentToolCallId: string, tools: ToolCallItem[]) {
  return tools.filter((tool) => tool.parentToolCallId === parentToolCallId).length
}

export function buildAgentTeamsFromHistory(
  teamRuns: TeamRunItem[] | undefined,
  members: TeamMemberRunItem[] | undefined,
  assistantMessageId: string,
) {
  const state = createAgentTeamState()
  for (const run of teamRuns ?? []) {
    if (run.assistantMessageId === assistantMessageId) mergeAgentTeamRun(state, run)
  }
  const visibleRunIDs = new Set(state.runs.map((run) => run.id))
  for (const member of members ?? []) {
    if (visibleRunIDs.has(member.teamRunId)) mergeAgentTeamMember(state, member)
  }
  return state.runs
}

export function getAgentTeamStatusLabel(status: string) {
  return ({
    running: '进行中',
    succeeded: '已完成',
    partial_failed: '部分失败',
    failed: '失败',
    canceled: '已取消',
    interrupted: '已中断',
  } as Record<string, string>)[status] || status
}

export function formatAgentTeamDuration(durationMs: number) {
  const safe = Math.max(0, Math.round(durationMs))
  if (safe < 1000) return `${safe}ms`
  if (safe < 60_000) return `${(safe / 1000).toFixed(safe < 10_000 ? 1 : 1).replace(/\.0$/, '')}s`
  const minutes = Math.floor(safe / 60_000)
  const seconds = Math.floor((safe % 60_000) / 1000)
  return seconds > 0 ? `${minutes}m ${seconds}s` : `${minutes}m`
}

export function getAgentTeamDurationMs(item: Pick<TeamRunItem | TeamMemberRunItem, 'startedAt' | 'finishedAt'>, now = Date.now()) {
  const startedAt = Date.parse(item.startedAt || '')
  if (!Number.isFinite(startedAt)) return 0
  const finishedAt = Date.parse(item.finishedAt || '')
  return Math.max(0, (Number.isFinite(finishedAt) ? finishedAt : now) - startedAt)
}
