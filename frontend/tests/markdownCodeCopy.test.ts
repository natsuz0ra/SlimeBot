import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import test from 'node:test'
import { renderMarkdown } from '../src/utils/markdown'

const projectRoot = resolve(import.meta.dirname, '..')

test('renderMarkdown wraps fenced code blocks with a copy button', () => {
  const html = renderMarkdown('```ts\nconst value = 1\n```', { codeCopyButton: true })

  assert.match(html, /class="markdown-code-block markdown-code-block--single-line"/)
  assert.match(html, /class="markdown-code-copy-btn"/)
  assert.match(html, /aria-label="Copy code"/)
  assert.match(html, /<pre><code class="language-ts">/)
  assert.match(html, /hljs-keyword">const<\/span> value = <span class="hljs-number">1<\/span>/)
})

test('renderMarkdown keeps multiline copy buttons anchored as multiline blocks', () => {
  const html = renderMarkdown('```ts\nconst value = 1\nconsole.log(value)\n```', { codeCopyButton: true })

  assert.match(html, /class="markdown-code-block"/)
  assert.doesNotMatch(html, /markdown-code-block--single-line/)
})

test('renderMarkdown keeps fenced code blocks plain by default', () => {
  const html = renderMarkdown('```ts\nconst value = 1\n```')

  assert.doesNotMatch(html, /markdown-code-block/)
  assert.doesNotMatch(html, /markdown-code-copy-btn/)
  assert.match(html, /<pre><code class="language-ts">/)
})

test('renderMarkdown does not add copy controls for inline code', () => {
  const html = renderMarkdown('Use `const value = 1` inline.', { codeCopyButton: true })

  assert.doesNotMatch(html, /markdown-code-block/)
  assert.doesNotMatch(html, /markdown-code-copy-btn/)
})

test('PlanBlock enables copy controls for fenced code blocks', () => {
  const source = readFileSync(resolve(projectRoot, 'src/components/chat/PlanBlock.vue'), 'utf8')

  assert.match(source, /renderMarkdown\(content,\s*\{\s*codeCopyButton:\s*true\b/)
  assert.match(source, /@click="handleMarkdownClick"/)
})

test('markdown code actions share a keyboard-accessible toolbar', () => {
  const copySource = readFileSync(resolve(projectRoot, 'src/composables/chat/useMarkdownCodeCopy.ts'), 'utf8')
  const markdownStyles = readFileSync(resolve(projectRoot, 'src/styles/markdown.css'), 'utf8')

  assert.match(copySource, /replace\(\s*\/\\n\+\$\/,\s*''\s*\)/)
  assert.match(markdownStyles, /\.markdown-code-actions button\s*\{[^}]*width:\s*28px;[^}]*height:\s*28px;/)
  assert.match(markdownStyles, /\.markdown-code-copy-btn::before\s*\{[\s\S]*?width:\s*14px;[\s\S]*?height:\s*14px;/)
  assert.match(markdownStyles, /\.markdown-code-block:focus-within \.markdown-code-actions/)
})

test('terminal controls are limited to shell fences and escape their label', () => {
  const options = { codeCopyButton: true, terminalLabel: '"Send"' }
  const shell = renderMarkdown('\x60\x60\x60bash\necho hello\n\x60\x60\x60', options)
  const source = renderMarkdown('\x60\x60\x60ts\nconst x = 1\n\x60\x60\x60', options)
  assert.match(shell, /markdown-code-terminal-btn" data-code-lang="bash" title="&quot;Send&quot;"/)
  assert.doesNotMatch(source, /markdown-code-terminal-btn/)
})
