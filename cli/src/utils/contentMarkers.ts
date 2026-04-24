export interface ContentSegment {
  type: 'text' | 'tool_call_marker' | 'thinking_marker' | 'plan_start' | 'plan_end'
  content: string
  toolCallId?: string
  thinkingId?: string
}

const MARKER_RE = /\n?<!-- (TOOL_CALL:(.+?)|THINKING:(.+?)|PLAN_START|PLAN_END) -->\n?/g

export function hasContentMarkers(content: string): boolean {
  return content.includes('<!-- TOOL_CALL:') || content.includes('<!-- THINKING:') || content.includes('<!-- PLAN_START -->')
}

export function parseContentMarkers(content: string): ContentSegment[] {
  const segments: ContentSegment[] = []
  let lastIndex = 0
  const regex = new RegExp(MARKER_RE.source, 'g')
  let match: RegExpExecArray | null

  while ((match = regex.exec(content)) !== null) {
    if (match.index > lastIndex) {
      const text = content.slice(lastIndex, match.index)
      if (text.trim() !== '') {
        segments.push({ type: 'text', content: text })
      }
    }
    const full = match[1] ?? ''
    if (full.startsWith('TOOL_CALL:')) {
      segments.push({ type: 'tool_call_marker', content: '', toolCallId: match[2] })
    } else if (full.startsWith('THINKING:')) {
      segments.push({ type: 'thinking_marker', content: '', thinkingId: match[3] })
    } else if (full === 'PLAN_START') {
      segments.push({ type: 'plan_start', content: '' })
    } else if (full === 'PLAN_END') {
      segments.push({ type: 'plan_end', content: '' })
    }
    lastIndex = regex.lastIndex
  }
  if (lastIndex < content.length) {
    const text = content.slice(lastIndex)
    if (text.trim() !== '') {
      segments.push({ type: 'text', content: text })
    }
  }
  return segments
}
