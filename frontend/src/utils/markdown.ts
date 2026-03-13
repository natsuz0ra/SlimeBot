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

/**
 * Inserts zero-width spaces between CJK/fullwidth characters and emphasis markers
 * to work around markdown-it's CommonMark emphasis parsing limitations with CJK punctuation.
 */
function preprocessCJKEmphasis(text: string): string {
  return text
    .replace(/([\u2e80-\u9fff\uff00-\uffef])(\*{1,3})/g, '$1\u200B$2')
    .replace(/(\*{1,3})([\u2e80-\u9fff\uff00-\uffef])/g, '$1\u200B$2')
}

export function renderMarkdown(content: string): string {
  const raw = md.render(preprocessCJKEmphasis(content || ''))
  return DOMPurify.sanitize(raw, {
    USE_PROFILES: { html: true },
  })
}
