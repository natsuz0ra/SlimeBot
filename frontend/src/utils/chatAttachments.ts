export const MAX_CHAT_UPLOAD_FILES = 5
export const MAX_CHAT_UPLOAD_BYTES = 10 * 1024 * 1024

const GENERIC_CLIPBOARD_NAMES = new Set([
  '',
  'blob',
  'image.png',
  'image.jpg',
  'image.jpeg',
  'image.webp',
  'image.gif',
  'image.bmp',
  'pasted-image.png',
])

const MIME_EXTENSIONS: Record<string, string> = {
  'image/png': 'png',
  'image/jpeg': 'jpg',
  'image/jpg': 'jpg',
  'image/webp': 'webp',
  'image/gif': 'gif',
  'image/bmp': 'bmp',
  'image/svg+xml': 'svg',
  'application/pdf': 'pdf',
  'text/plain': 'txt',
}

export interface MergeFilesResult {
  files: File[]
  addedCount: number
  skippedTooLarge: string[]
  skippedOverflow: number
}

export interface ClipboardDataLike {
  items?: ArrayLike<{ kind: string; getAsFile: () => File | null }> | null
  files?: ArrayLike<File> | null
  getData?: (format: string) => string
}

function pad2(value: number): string {
  return String(value).padStart(2, '0')
}

function formatTimestamp(now: Date): string {
  return [
    now.getFullYear(),
    pad2(now.getMonth() + 1),
    pad2(now.getDate()),
  ].join('') + '-' + [
    pad2(now.getHours()),
    pad2(now.getMinutes()),
    pad2(now.getSeconds()),
  ].join('')
}

export function isImageFile(file: Pick<File, 'type' | 'name'>): boolean {
  if (file.type && file.type.toLowerCase().startsWith('image/')) return true
  return /\.(png|jpe?g|gif|webp|bmp|svg)$/i.test(file.name || '')
}

export function normalizePastedFile(file: File, now: Date = new Date(), index = 0): File {
  const normalizedName = (file.name || '').trim().toLowerCase()
  if (!GENERIC_CLIPBOARD_NAMES.has(normalizedName)) {
    return file
  }
  const mime = (file.type || '').toLowerCase()
  const ext = MIME_EXTENSIONS[mime] || (mime.startsWith('image/') ? mime.slice('image/'.length).replace(/[^a-z0-9]/g, '') || 'png' : 'bin')
  const prefix = mime.startsWith('image/') || normalizedName.startsWith('image.') ? 'screenshot' : 'pasted-file'
  const suffix = index > 0 ? `-${index + 1}` : ''
  const nextName = `${prefix}-${formatTimestamp(now)}${suffix}.${ext}`
  return new File([file], nextName, {
    type: file.type,
    lastModified: file.lastModified || now.getTime(),
  })
}

export function extractPastedFiles(clipboardData: ClipboardDataLike | null | undefined, now: Date = new Date()): File[] {
  if (!clipboardData) return []
  const rawFiles: File[] = []
  if (clipboardData.items && clipboardData.items.length > 0) {
    for (let i = 0; i < clipboardData.items.length; i += 1) {
      const item = clipboardData.items[i]
      if (item && item.kind === 'file') {
        const file = item.getAsFile()
        if (file) rawFiles.push(file)
      }
    }
  }
  if (rawFiles.length === 0 && clipboardData.files && clipboardData.files.length > 0) {
    for (let i = 0; i < clipboardData.files.length; i += 1) {
      const file = clipboardData.files[i]
      if (file) rawFiles.push(file)
    }
  }
  if (rawFiles.length === 0) return []

  // Avoid hijacking rich HTML+text copy from Word/Excel/browsers where a rendered image is attached alongside text.
  const textPlain = clipboardData.getData?.('text/plain') || ''
  const textHtml = clipboardData.getData?.('text/html') || ''
  if (textHtml.trim() !== '' && textPlain.trim() !== '') {
    return []
  }

  return rawFiles.map((file, idx) => normalizePastedFile(file, now, idx))
}

export function mergePendingFiles(
  existing: File[],
  incoming: File[],
  limits: { maxFiles?: number; maxBytes?: number } = {},
): MergeFilesResult {
  const maxFiles = limits.maxFiles ?? MAX_CHAT_UPLOAD_FILES
  const maxBytes = limits.maxBytes ?? MAX_CHAT_UPLOAD_BYTES
  const files = [...existing]
  const skippedTooLarge: string[] = []
  let skippedOverflow = 0
  let addedCount = 0

  for (const file of incoming) {
    if (file.size > maxBytes) {
      skippedTooLarge.push(file.name)
      continue
    }
    if (files.length >= maxFiles) {
      skippedOverflow += 1
      continue
    }
    files.push(file)
    addedCount += 1
  }

  return {
    files,
    addedCount,
    skippedTooLarge,
    skippedOverflow,
  }
}
