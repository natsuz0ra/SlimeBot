import { apiClient } from './client'

export type ExportFormat = 'markdown' | 'json'

export function exportFilename(title: string, format: ExportFormat, now = new Date()) {
  const clean = title.replace(/[^\p{L}\p{N}_-]+/gu, '-').replace(/^[-_]+|[-_]+$/g, '') || 'chat'
  let stem = ''
  for (const char of clean) {
    if (new TextEncoder().encode(stem + char).length > 80) break
    stem += char
  }
  const date = now.toISOString().slice(0, 10).replace(/-/g, '')
  return `slimebot-${stem}-${date}.${format === 'markdown' ? 'md' : 'json'}`
}

export async function downloadSessionExport(id: string, params: {
  format: ExportFormat; includeTools: boolean; includeThinking: boolean
}, fallback: string) {
  const response = await apiClient.get<Blob>(`/api/sessions/${encodeURIComponent(id)}/export`, {
    params, responseType: 'blob', timeout: 60000,
  })
  const disposition = String(response.headers['content-disposition'] || '')
  const encoded = /filename\*=utf-8''([^;]+)/i.exec(disposition)?.[1]
  const plain = /filename=(?:"([^"]+)"|([^;\s]+))/i.exec(disposition)
  const name = encoded ? decodeURIComponent(encoded) : plain?.[1] || plain?.[2] || fallback
  const url = URL.createObjectURL(response.data)
  const link = document.createElement('a')
  link.href = url
  link.download = name
  link.click()
  window.setTimeout(() => URL.revokeObjectURL(url), 1000)
}
