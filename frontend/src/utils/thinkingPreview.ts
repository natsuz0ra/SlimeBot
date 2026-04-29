export function getThinkingPreviewLineIndex(content: string): number {
  const lines = content.replace(/\r\n/g, '\n').split('\n')
  for (let i = lines.length - 1; i >= 0; i--) {
    if (lines[i]!.trim() !== '') return i
  }
  return 0
}

export function getThinkingPreviewLine(content: string): string {
  const lines = content.replace(/\r\n/g, '\n').split('\n')
  return (lines[getThinkingPreviewLineIndex(content)] || '').trim()
}
