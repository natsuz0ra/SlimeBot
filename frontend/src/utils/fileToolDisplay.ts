import type { ToolCallItem } from '@/api/chat'

export type FileToolName = 'file_read' | 'file_edit' | 'file_write'

export interface FileReadSummary {
  filePath: string
  totalLines: number
  startLine?: number
  endLine?: number
  truncated: boolean
}

export interface FileDiffLine {
  kind: 'context' | 'added' | 'removed'
  oldLine?: number
  newLine?: number
  text: string
}

export interface FileToolDisplay {
  toolName: FileToolName
  filePath: string
  operation: 'Read' | 'Create' | 'Update' | 'Write'
  summary: string
  diffLines: FileDiffLine[]
}

const fileToolNames = new Set(['file_read', 'file_edit', 'file_write'])

export function isFileToolName(toolName?: string): toolName is FileToolName {
  return fileToolNames.has((toolName || '').trim().toLowerCase())
}

export function isFileTool(item: Pick<ToolCallItem, 'toolName'>) {
  return isFileToolName(item.toolName)
}

export function countTextLines(content: string): number {
  if (content === '') return 0
  const parts = content.split('\n')
  return content.endsWith('\n') ? parts.length - 1 : parts.length
}

function splitTextLines(content: string): string[] {
  if (content === '') return []
  const lines = content.replace(/\r\n/g, '\n').split('\n')
  if (content.endsWith('\n')) lines.pop()
  return lines
}

export function parseFileReadOutput(raw: string): FileReadSummary | null {
  const filePath = raw.match(/^File:\s*(.+)$/m)?.[1]?.trim()
  const totalRaw = raw.match(/^Total lines:\s*(\d+)$/m)?.[1]
  if (!filePath || totalRaw === undefined) return null

  const showing = raw.match(/^Showing lines\s+(\d+)-(\d+):$/m)
  return {
    filePath,
    totalLines: Number(totalRaw),
    startLine: showing ? Number(showing[1]) : undefined,
    endLine: showing ? Number(showing[2]) : undefined,
    truncated: raw.includes('[truncated;'),
  }
}

export function formatFileReadSummary(summary: FileReadSummary): string {
  if (
    summary.startLine !== undefined &&
    summary.endLine !== undefined &&
    (summary.startLine !== 1 || summary.endLine !== summary.totalLines)
  ) {
    const readLines = Math.max(0, summary.endLine - summary.startLine + 1)
    return `Read ${readLines} of ${summary.totalLines} lines, showing ${summary.startLine}-${summary.endLine}`
  }
  return `Read ${summary.totalLines} ${summary.totalLines === 1 ? 'line' : 'lines'}`
}

export function buildLineDiff(oldContent: string, newContent: string): FileDiffLine[] {
  const oldLines = splitTextLines(oldContent)
  const newLines = splitTextLines(newContent)
  const table: number[][] = Array.from({ length: oldLines.length + 1 }, () => Array.from({ length: newLines.length + 1 }, () => 0))

  for (let i = oldLines.length - 1; i >= 0; i--) {
    for (let j = newLines.length - 1; j >= 0; j--) {
      table[i]![j] = oldLines[i] === newLines[j]
        ? table[i + 1]![j + 1]! + 1
        : Math.max(table[i + 1]![j]!, table[i]![j + 1]!)
    }
  }

  const result: FileDiffLine[] = []
  let i = 0
  let j = 0
  while (i < oldLines.length && j < newLines.length) {
    if (oldLines[i] === newLines[j]) {
      result.push({ kind: 'context', oldLine: i + 1, newLine: j + 1, text: oldLines[i]! })
      i++
      j++
    } else if (table[i + 1]![j]! >= table[i]![j + 1]!) {
      result.push({ kind: 'removed', oldLine: i + 1, text: oldLines[i]! })
      i++
    } else {
      result.push({ kind: 'added', newLine: j + 1, text: newLines[j]! })
      j++
    }
  }
  while (i < oldLines.length) {
    result.push({ kind: 'removed', oldLine: i + 1, text: oldLines[i]! })
    i++
  }
  while (j < newLines.length) {
    result.push({ kind: 'added', newLine: j + 1, text: newLines[j]! })
    j++
  }
  return result
}

export function buildFileToolDisplay(item: {
  toolName?: string
  params?: Record<string, string>
  output?: string
  content?: string
}): FileToolDisplay | null {
  const toolName = (item.toolName || '').trim().toLowerCase()
  if (!isFileToolName(toolName)) return null

  const params = item.params || {}
  const filePath = String(params.file_path || '').trim()
  if (toolName === 'file_read') {
    const parsed = parseFileReadOutput(item.output || item.content || '')
    const path = parsed?.filePath || filePath
    return {
      toolName,
      filePath: path,
      operation: 'Read',
      summary: parsed ? formatFileReadSummary(parsed) : 'Read file',
      diffLines: [],
    }
  }

  if (toolName === 'file_edit') {
    const oldString = String(params.old_string ?? '')
    const newString = String(params.new_string ?? '')
    const creating = oldString === ''
    return {
      toolName,
      filePath,
      operation: creating ? 'Create' : 'Update',
      summary: creating ? `Created ${filePath}` : `Updated ${filePath}`,
      diffLines: buildLineDiff(oldString, newString),
    }
  }

  const content = String(params.content ?? '')
  const lineCount = countTextLines(content)
  return {
    toolName,
    filePath,
    operation: 'Write',
    summary: `Wrote ${lineCount} ${lineCount === 1 ? 'line' : 'lines'} to ${filePath}`,
    diffLines: buildLineDiff('', content),
  }
}

export function fileToolSummaryFromParams(toolName: string, params?: Record<string, string>): string {
  const name = toolName.trim().toLowerCase()
  if (!isFileToolName(name)) return ''
  const filePath = String(params?.file_path ?? '').trim()
  if (!filePath) return ''
  if (name === 'file_read') return `Read ${filePath}`
  if (name === 'file_edit') return `${String(params?.old_string ?? '') === '' ? 'Create' : 'Update'} ${filePath}`
  return `Write ${filePath}`
}
