import DOMPurify from 'dompurify'
import hljs from 'highlight.js/lib/common'
import MarkdownIt from 'markdown-it'
import { isShellLanguage } from './terminalLink'

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

interface MarkdownRenderEnv {
  codeCopyButton?: boolean
  terminalLabel?: string
}

const defaultLinkOpenRule =
  md.renderer.rules.link_open ??
  ((tokens, idx, options, _env, self) => self.renderToken(tokens, idx, options))

md.renderer.rules.link_open = (tokens, idx, options, env, self) => {
  const linkToken = tokens[idx]
  if (linkToken) {
    linkToken.attrSet('target', '_blank')
    linkToken.attrSet('rel', 'noopener noreferrer nofollow')
  }
  return defaultLinkOpenRule(tokens, idx, options, env, self)
}

md.renderer.rules.fence = (tokens, idx, options, env: MarkdownRenderEnv) => {
  const token = tokens[idx]
  const code = token?.content ?? ''
  const info = token?.info ? md.utils.unescapeAll(token.info).trim() : ''
  const language = info ? (info.split(/\s+/g)[0] ?? '') : ''
  const languageClass = language ? ` class="language-${escapeHtml(language)}"` : ''
  const highlighted = options.highlight ? options.highlight(code, language, '') : escapeHtml(code)
  const codeMarkup = `<pre><code${languageClass}>${highlighted}</code></pre>`

  if (!env.codeCopyButton) return codeMarkup

  const blockClass = code.replace(/\n+$/, '').includes('\n')
    ? 'markdown-code-block'
    : 'markdown-code-block markdown-code-block--single-line'
  const terminal = env.terminalLabel && isShellLanguage(language)
    ? `<button type="button" class="markdown-code-terminal-btn" data-code-lang="${escapeHtml(language)}" title="${escapeHtml(env.terminalLabel)}" aria-label="${escapeHtml(env.terminalLabel)}">›_</button>`
    : ''
  return `<div class="${blockClass}">${codeMarkup}<div class="markdown-code-actions">${terminal}<button type="button" class="markdown-code-copy-btn" title="Copy code" aria-label="Copy code"></button></div></div>`
}

/**
 * Inserts zero-width spaces between CJK/fullwidth characters and emphasis markers
 * to work around markdown-it's CommonMark emphasis parsing limitations with CJK punctuation.
 */
function preprocessCJKEmphasis(text: string): string {
  return text
    .replace(/([\u2e80-\u9fff\uff00-\uffef])(\*{1,3})/g, '$1\u200B$2')
    .replace(/(\*{1,3})([\u2e80-\u9fff\uff00-\uffef])/g, '$1\u200B$2')
}

function sanitizeHtml(raw: string): string {
  const purifierModule = DOMPurify as unknown as {
    sanitize?: (html: string, options: Record<string, unknown>) => string
    default?: { sanitize?: (html: string, options: Record<string, unknown>) => string }
  }
  const sanitize = purifierModule.sanitize ?? purifierModule.default?.sanitize
  if (!sanitize) return raw
  return sanitize(raw, {
    USE_PROFILES: { html: true },
    ADD_ATTR: ['target', 'rel', 'type', 'title', 'aria-label', 'data-code-lang'],
  })
}

export function renderMarkdown(content: string, options: MarkdownRenderEnv = {}): string {
  const raw = md.render(preprocessCJKEmphasis(content || ''), options)
  return sanitizeHtml(raw)
}
