type Validator<T> = (value: unknown) => value is T
type Shape = Record<string, Validator<unknown>>
type ValueOf<T> = T extends Validator<infer Value> ? Value : never
type OptionalKeys<S extends Shape> = { [K in keyof S]: undefined extends ValueOf<S[K]> ? K : never }[keyof S]
type ShapeValue<S extends Shape> = {
  [K in Exclude<keyof S, OptionalKeys<S>>]: ValueOf<S[K]>
} & {
  [K in OptionalKeys<S>]?: Exclude<ValueOf<S[K]>, undefined>
}

const string: Validator<string> = (value): value is string => typeof value === 'string'
const id: Validator<string> = (value): value is string => string(value) && value.trim() !== ''
const number: Validator<number> = (value): value is number => typeof value === 'number' && Number.isFinite(value)
const boolean: Validator<boolean> = (value): value is boolean => typeof value === 'boolean'
const metadata: Validator<unknown> = (_value): _value is unknown => true

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function optional<T>(validate: Validator<T>): Validator<T | undefined> {
  return (value): value is T | undefined => value === undefined || validate(value)
}

function nullable<T>(validate: Validator<T>): Validator<T | null> {
  return (value): value is T | null => value === null || validate(value)
}

function array<T>(validate: Validator<T>): Validator<T[]> {
  return (value): value is T[] => Array.isArray(value) && value.every(validate)
}

function oneOf<const T extends readonly string[]>(...allowed: T): Validator<T[number]> {
  return (value): value is T[number] => string(value) && allowed.includes(value)
}

function invalidField(value: Record<string, unknown>, shape: Shape): string | undefined {
  return Object.keys(shape).find(field => !shape[field]!(value[field]))
}

function object<S extends Shape>(shape: S): Validator<ShapeValue<S>> {
  return (value): value is ShapeValue<S> => record(value) && invalidField(value, shape) === undefined
}

export const isToolCallStatus = oneOf('pending', 'reviewing', 'rejected', 'executing', 'completed', 'error')
export type ToolCallStatus = ValueOf<typeof isToolCallStatus>

const todoItem = object({ id, content: string, status: oneOf('pending', 'in_progress', 'completed') })
const backgroundTask = object({ taskId: id, processId: string, status: string })
const skillLearning = object({ skillId: optional(string), status: string, message: string })
const contextLayer = object({ layer: string, tokens: number })

// Legacy payloads and unscoped control frames may omit a session or timing metadata.
const session = { sessionId: optional(string) }
const runIdentity = { teamRunId: optional(string), memberRunId: optional(string) }
const agent = { ...runIdentity, parentToolCallId: optional(string), subagentRunId: optional(string) }
const subagent = { ...runIdentity, parentToolCallId: string, subagentRunId: string }
const tool = { ...agent, toolCallId: id, toolName: optional(string), command: optional(string) }
const toolReview = { ...tool, reviewStatus: optional(string), reviewRisk: optional(string), reviewReason: optional(string) }
const toolApproval = { ...toolReview, params: optional(nullable(record)), requiresApproval: optional(boolean) }
const dates = { startedAt: optional(string), finishedAt: optional(string), createdAt: optional(string), updatedAt: optional(string) }
const team = {
  ...dates, teamRunId: optional(id), id: optional(id), requestId: optional(string),
  status: optional(oneOf('', 'running', 'completed', 'succeeded', 'partial_failed', 'failed', 'canceled', 'interrupted')),
  maxMembers: optional(number), maxParallel: optional(number), lastError: optional(string),
}
const contextUsage = {
  ...session, modelConfigId: optional(string), usedTokens: number, totalTokens: number,
  usedPercent: optional(number), availablePercent: optional(number), isCompacted: optional(boolean),
  compactedAt: optional(string), inputBudget: optional(number), outputReserve: optional(number),
  compactionBeforeTokens: optional(number), compactionAfterTokens: optional(number),
  compactionReason: optional(string), compactionDurationMs: optional(number), compactionFailure: optional(string),
  layers: optional(nullable(array(contextLayer))), source: optional(oneOf('', 'estimated', 'provider-reported')),
  state: optional(oneOf('', 'compacting', 'ready', 'cooldown')),
  outputBudgetTokens: optional(number), outputBudgetUsed: optional(number),
}
const steeringChange = object({
  submissionId: optional(string), messageId: optional(string), turnId: optional(string), content: string, createdAt: optional(string),
})

const eventShapes = {
  pong: {},
  session: { sessionId: id },
  start: { turnId: optional(id), startedAt: optional(string), background: optional(boolean) },
  stopping: {},
  chunk: { content: string },
  done: {
    answer: optional(string), isInterrupted: optional(boolean), isStopPlaceholder: optional(boolean),
    planId: optional(string), planBody: optional(string), narration: optional(string),
    finishedAt: optional(string), durationMs: optional(number), skillLearning: optional(skillLearning),
  },
  error: { error: string },
  message_edited: { messageId: id, content: string, createdAt: optional(string) },
  session_title: { title: string },
  thinking_start: { ...agent, startedAt: optional(string) },
  thinking_chunk: { ...agent, content: string, startedAt: optional(string) },
  thinking_done: { ...agent, finishedAt: optional(string) },
  tool_call_start: { ...toolApproval, preamble: optional(string), startedAt: optional(string) },
  tool_call_review: { ...toolReview, reviewedAt: optional(string) },
  tool_call_approval_required: { ...toolApproval, requiredAt: optional(string) },
  tool_call_result: {
    ...tool, requiresApproval: optional(boolean), status: optional(oneOf('', 'pending', 'reviewing', 'rejected', 'executing', 'completed', 'error')),
    output: optional(string), error: optional(string), metadata, finishedAt: optional(string),
  },
  team_start: team,
  team_done: team,
  team_member_queued: {
    ...dates, teamRunId: id, memberRunId: id, toolCallId: optional(string), subagentRunId: optional(string),
    title: optional(string), task: optional(string), modelConfigId: optional(string),
    status: optional(oneOf('', 'queued', 'running', 'succeeded', 'failed', 'canceled', 'interrupted')),
    answer: optional(string), error: optional(string),
  },
  subagent_start: { ...subagent, title: string, task: string },
  subagent_chunk: { ...subagent, content: string },
  subagent_done: { ...subagent, error: optional(string) },
  plan_start: {},
  plan_chunk: { content: string },
  plan_body: { content: string },
  plan_status: { planId: id, status: string },
  todo_update: { items: nullable(array(todoItem)), note: optional(string), updatedAt: optional(string) },
  context_usage: contextUsage,
  context_compacted: { usage: object(contextUsage) },
  background_tasks: { tasks: array(backgroundTask) },
  background_task: { taskId: id, processId: optional(string), status: string },
  background_error: { error: string },
  steer_accepted: { change: steeringChange },
  steer_rejected: { submissionId: optional(string), content: optional(string), error: string },
  privacy: { masked: number, warned: number, blocked: boolean },
} satisfies Record<string, Shape>

export type ChatWSEventType = keyof typeof eventShapes
export type ChatWSEvent = {
  [K in ChatWSEventType]: { type: K } & ShapeValue<typeof session> & ShapeValue<typeof eventShapes[K]>
}[ChatWSEventType]

export type ContextUsageWire = ShapeValue<typeof contextUsage>
export type RuntimeTodoItem = ValueOf<typeof todoItem>
export type RuntimeTodoStatus = RuntimeTodoItem['status']
export type BackgroundTaskData = ValueOf<typeof backgroundTask>
export type SkillLearningData = ValueOf<typeof skillLearning>

export type ChatWSDiagnostic =
  | { code: 'invalid_json' | 'invalid_message' }
  | { code: 'unknown_type'; eventType: string }
  | { code: 'invalid_field'; eventType: ChatWSEventType; field: string }

export type ChatWSDecodeResult = { ok: true; event: ChatWSEvent } | { ok: false; diagnostic: ChatWSDiagnostic }

function isEventType(type: string): type is ChatWSEventType {
  return Object.prototype.hasOwnProperty.call(eventShapes, type)
}

export function decodeChatWSEvent(raw: unknown): ChatWSDecodeResult {
  let value = raw
  if (typeof raw === 'string') {
    try {
      value = JSON.parse(raw)
    } catch {
      return { ok: false, diagnostic: { code: 'invalid_json' } }
    }
  }
  if (!record(value) || !id(value.type)) {
    return { ok: false, diagnostic: { code: 'invalid_message' } }
  }
  const eventType = value.type
  if (!isEventType(eventType)) {
    return { ok: false, diagnostic: { code: 'unknown_type', eventType } }
  }
  const field = invalidField(value, session) ?? invalidField(value, eventShapes[eventType])
    ?? ((eventType === 'team_start' || eventType === 'team_done') && !value.teamRunId && !value.id ? 'teamRunId' : undefined)
    ?? (eventType === 'start' && !value.turnId && !value.startedAt ? 'turnId' : undefined)
  if (field) {
    return { ok: false, diagnostic: { code: 'invalid_field', eventType, field } }
  }
  // The discriminant selects the shape that was just validated.
  return { ok: true, event: value as ChatWSEvent }
}

export function describeChatWSDiagnostic(diagnostic: ChatWSDiagnostic): string {
  switch (diagnostic.code) {
    case 'invalid_json': return 'Invalid WebSocket JSON.'
    case 'invalid_message': return 'WebSocket event must be an object with a type.'
    case 'unknown_type': return 'Unknown WebSocket event type: ' + diagnostic.eventType
    case 'invalid_field': return 'Invalid WebSocket event field: ' + diagnostic.eventType + '.' + diagnostic.field
  }
}
