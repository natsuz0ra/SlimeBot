// Return a number for localization in the UI; missing/invalid end times have no duration.
export function taskRunDurationSeconds(startedAt: string, finishedAt?: string): number | null {
  if (!finishedAt) return null
  const duration = new Date(finishedAt).getTime() - new Date(startedAt).getTime()
  return Number.isFinite(duration) && duration >= 0 ? Math.round(duration / 1000) : null
}

export function taskRunPreview(text: string): string {
  return text
    .replace(/^ {0,3}(?:#{1,6}\s+|[-*+]\s+|\d+\.\s+)/gm, '')
    .replace(/\*\*([^*]+)\*\*/g, '$1')
    .replace(/\s+/g, ' ')
    .trim()
}
