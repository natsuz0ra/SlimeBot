import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import test from 'node:test'
import assert from 'node:assert/strict'

const projectRoot = resolve(import.meta.dirname, '..')

test('chat message containers prevent page-level horizontal overflow', () => {
  const listSource = readFileSync(resolve(projectRoot, 'src/components/chat/ChatMessageList.vue'), 'utf8')
  const itemSource = readFileSync(resolve(projectRoot, 'src/components/chat/ChatMessageItem.vue'), 'utf8')
  const assistantBodySource = readFileSync(resolve(projectRoot, 'src/components/chat/AssistantMessageBody.vue'), 'utf8')
  const homeStyles = readFileSync(resolve(projectRoot, 'src/pages/home-page.css'), 'utf8')

  assert.match(listSource, /messages-section[^"]*\bmin-w-0\b[^"]*\boverflow-x-hidden\b/)
  assert.match(listSource, /class="[^"]*\bflex\b[^"]*\bmin-w-0\b[^"]*"/)
  assert.match(itemSource, /class="[^"]*\bflex\b[^"]*\bmin-w-0\b[^"]*message-animate/)
  assert.match(itemSource, /class="[^"]*\bmin-w-0\b[^"]*text-sm/)
  assert.match(assistantBodySource, /assistant-reply-body[^"]*\bmin-w-0\b/)
  assert.match(homeStyles, /\.chat-content-shell\s*\{[\s\S]*?min-width:\s*0;/)
})

test('chat text wraps while unwrappable markdown blocks scroll internally', () => {
  const markdownStyles = readFileSync(resolve(projectRoot, 'src/styles/markdown.css'), 'utf8')
  const homeStyles = readFileSync(resolve(projectRoot, 'src/pages/home-page.css'), 'utf8')

  assert.match(homeStyles, /\.user-message-content\s*\{[\s\S]*?overflow-wrap:\s*anywhere;/)
  assert.match(markdownStyles, /\.bubble-markdown\s*\{[\s\S]*?overflow-wrap:\s*anywhere;/)
  assert.match(markdownStyles, /\.bubble-markdown pre\s*\{[\s\S]*?max-width:\s*100%;[\s\S]*?overflow-x:\s*auto;/)
  assert.match(markdownStyles, /\.bubble-markdown table\s*\{[\s\S]*?display:\s*block;[\s\S]*?max-width:\s*100%;[\s\S]*?overflow-x:\s*auto;/)
})

test('assistant waiting placeholder centers typing dots with the avatar', () => {
  const itemSource = readFileSync(resolve(projectRoot, 'src/components/chat/ChatMessageItem.vue'), 'utf8')
  const assistantBodySource = readFileSync(resolve(projectRoot, 'src/components/chat/AssistantMessageBody.vue'), 'utf8')

  assert.match(assistantBodySource, /const isTypingPlaceholder = computed\(\(\) => ctx\.isEmptyPlaceholder\(props\.item\.id\) && ctx\.waiting\)/)
  assert.match(assistantBodySource, /<TransitionGroup[\s\S]*v-if="renderedTimeline\.length > 0"[\s\S]*class="assistant-reply-timeline"/)
  assert.match(assistantBodySource, /<div v-if="isTypingPlaceholder" class="assistant-typing-placeholder">[\s\S]*<TypingDots \/>[\s\S]*<\/div>/)
  assert.match(assistantBodySource, /\.assistant-typing-placeholder\s*\{[\s\S]*min-height:\s*40px;[\s\S]*display:\s*flex;[\s\S]*align-items:\s*center;/)
  assert.doesNotMatch(assistantBodySource, /<TypingDots v-if="ctx\.isEmptyPlaceholder\(item\.id\) && ctx\.waiting" \/>/)
  assert.doesNotMatch(itemSource, /ctx\.isEmptyPlaceholder\(item\.id\) && ctx\.waiting[\s\S]*\?\s*'items-center'/)
})
