export interface ExecOutputPayload {
  stdout: string
  stderr: string
  exit_code: number
  timed_out: boolean
  truncated: boolean
  shell: string
  working_directory: string
  duration_ms: number
}

export interface WebSearchResult {
  title: string
  url: string
  content: string
}

export interface WebSearchPayload {
  query: string
  results: WebSearchResult[]
}

export interface ToolResultDisplay {
  mode: 'text' | 'exec' | 'web_search' | 'ask_questions'
  outputText: string
  exec?: ExecOutputPayload
  webSearch?: WebSearchPayload
}

export interface AskQuestionsAnswer {
  questionId: string
  selectedOption: number
  customAnswer: string
}

export interface AskQuestionsQuestion {
  id: string
  question: string
  options: string[]
}

export function parseAskQuestionsAnswers(raw: string): AskQuestionsAnswer[] | null {
  const parsed = tryParseJSON(raw)
  if (!Array.isArray(parsed)) return null
  const answers: AskQuestionsAnswer[] = []
  for (const item of parsed) {
    if (!isRecord(item)) return null
    const questionId = item.questionId
    const selectedOption = item.selectedOption
    const customAnswer = item.customAnswer
    if (typeof questionId !== 'string' || typeof selectedOption !== 'number' || typeof customAnswer !== 'string') return null
    answers.push({ questionId, selectedOption, customAnswer })
  }
  return answers.length > 0 ? answers : null
}

export function parseAskQuestionsQuestions(raw: string): AskQuestionsQuestion[] | null {
  const parsed = tryParseJSON(raw)
  if (!Array.isArray(parsed)) return null
  const questions: AskQuestionsQuestion[] = []
  for (const item of parsed) {
    if (!isRecord(item)) return null
    const id = item.id
    const question = item.question
    const options = item.options
    if (typeof id !== 'string' || typeof question !== 'string' || !Array.isArray(options)) return null
    questions.push({ id, question, options: options.filter((o): o is string => typeof o === 'string') })
  }
  return questions.length > 0 ? questions : null
}

function tryParseJSON(raw: string): unknown | null {
  const trimmed = raw.trim()
  if (!trimmed) return null
  try {
    return JSON.parse(trimmed)
  } catch {
    return null
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

export function decodeCommonEscapes(raw: string): string {
  if (!raw.includes('\\')) return raw
  return raw
    .replace(/\\r\\n/g, '\n')
    .replace(/\\n/g, '\n')
    .replace(/\\r/g, '\n')
    .replace(/\\t/g, '\t')
    .replace(/\\\\"/g, '"')
    .replace(/\\\\/g, '\\')
}

export function formatDisplayText(raw: string): string {
  const parsed = tryParseJSON(raw)
  if (parsed !== null) {
    if (typeof parsed === 'string') {
      return decodeCommonEscapes(parsed)
    }
    try {
      return JSON.stringify(parsed, null, 2)
    } catch {
      return raw
    }
  }
  const decoded = decodeCommonEscapes(raw)
  // Filter consecutive empty lines in display only
  return decoded.replace(/\n{2,}/g, '\n').trim()
}

export function parseExecOutputPayload(raw: string): ExecOutputPayload | null {
  const parsed = tryParseJSON(raw)
  if (!isRecord(parsed)) return null

  const stdout = parsed.stdout
  const stderr = parsed.stderr
  const exitCode = parsed.exit_code
  const timedOut = parsed.timed_out
  const truncated = parsed.truncated
  const shell = parsed.shell
  const workingDirectory = parsed.working_directory
  const durationMs = parsed.duration_ms

  if (
    typeof stdout !== 'string' ||
    typeof stderr !== 'string' ||
    typeof exitCode !== 'number' ||
    typeof timedOut !== 'boolean' ||
    typeof truncated !== 'boolean' ||
    typeof shell !== 'string' ||
    typeof workingDirectory !== 'string' ||
    typeof durationMs !== 'number'
  ) {
    return null
  }

  return {
    stdout,
    stderr,
    exit_code: exitCode,
    timed_out: timedOut,
    truncated,
    shell,
    working_directory: workingDirectory,
    duration_ms: durationMs,
  }
}

export function parseWebSearchPayload(raw: string): WebSearchPayload | null {
  const parsed = tryParseJSON(raw)
  if (!isRecord(parsed)) return null

  const query = parsed.query
  const results = parsed.results
  if (typeof query !== 'string' || !Array.isArray(results)) return null

  const normalizedResults: WebSearchResult[] = results
    .map((item) => {
      if (!isRecord(item)) return null
      const title = item.title
      const url = item.url
      const content = typeof item.content === 'string'
        ? item.content
        : typeof item.snippet === 'string'
          ? item.snippet
          : ''
      if (typeof title !== 'string' || typeof url !== 'string') return null
      return { title, url, content }
    })
    .filter((item): item is WebSearchResult => item !== null)

  return {
    query,
    results: normalizedResults,
  }
}

export function formatToolParams(params: Record<string, string>): Array<{ key: string; value: string }> {
  return Object.keys(params)
    .sort()
    .map((key) => ({ key, value: formatDisplayText(params[key] ?? '') }))
}

export function buildToolResultDisplay(toolName: string, command: string, output?: string): ToolResultDisplay {
  const raw = output || ''
  if (toolName === 'exec' && command === 'run') {
    const exec = parseExecOutputPayload(raw)
    if (exec) {
      return {
        mode: 'exec',
        outputText: '',
        exec,
      }
    }
  }

  if (toolName === 'web_search') {
    const webSearch = parseWebSearchPayload(raw)
    if (webSearch) {
      return {
        mode: 'web_search',
        outputText: formatDisplayText(raw),
        webSearch,
      }
    }
  }

  if (toolName === 'ask_questions') {
    const answers = parseAskQuestionsAnswers(raw)
    if (answers) {
      return {
        mode: 'ask_questions',
        outputText: raw,
      }
    }
  }

  return {
    mode: 'text',
    outputText: formatDisplayText(raw),
  }
}
