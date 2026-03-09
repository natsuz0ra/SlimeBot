import DOMPurify from 'dompurify'
import hljs from 'highlight.js/lib/common'
import MarkdownIt from 'markdown-it'

function escapeHtml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

const md = new MarkdownIt({
  html: false,
  linkify: true,
  breaks: false,
  highlight(code, language) {
    if (language && hljs.getLanguage(language)) {
      try {
        const highlighted = hljs.highlight(code, {
          language,
          ignoreIllegals: true,
        }).value
        return `<span class="hljs">${highlighted}</span>`
      } catch {
        return `<span class="hljs">${escapeHtml(code)}</span>`
      }
    }

    try {
      return `<span class="hljs">${hljs.highlightAuto(code).value}</span>`
    } catch {
      return `<span class="hljs">${escapeHtml(code)}</span>`
    }
  },
})

export function renderMarkdown(content: string): string {
  const raw = md.render(content || '')
  return DOMPurify.sanitize(raw, {
    USE_PROFILES: { html: true },
  })
}
