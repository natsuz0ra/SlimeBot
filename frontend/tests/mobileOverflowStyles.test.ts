import { existsSync, readFileSync } from 'node:fs'
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

test('mobile user message actions are visible without hover', () => {
  const itemSource = readFileSync(resolve(projectRoot, 'src/components/chat/ChatMessageItem.vue'), 'utf8')

  assert.match(itemSource, /\.user-message-shell--actions::after\s*\{[\s\S]*?content:\s*'';[\s\S]*?position:\s*absolute;[\s\S]*?top:\s*100%;[\s\S]*?height:\s*26px;[\s\S]*?pointer-events:\s*auto;/)
  assert.match(itemSource, /\.user-message-shell:hover \.user-message-actions,[\s\S]*?\.user-message-shell:focus-within \.user-message-actions\s*\{[\s\S]*?opacity:\s*1;[\s\S]*?pointer-events:\s*auto;/)
  assert.match(itemSource, /@media\s*\(hover:\s*none\),\s*\(pointer:\s*coarse\)\s*\{[\s\S]*?\.user-message-actions\s*\{[\s\S]*?opacity:\s*1;[\s\S]*?pointer-events:\s*auto;/)
  assert.match(itemSource, /@media\s*\(hover:\s*none\),\s*\(pointer:\s*coarse\)\s*\{[\s\S]*?\.user-message-actions--hidden\s*\{[\s\S]*?opacity:\s*0;[\s\S]*?pointer-events:\s*none;/)
})

test('settings typography and narrow tabs use semantic styles', () => {
  const panelSource = readFileSync(resolve(projectRoot, 'src/components/settings/SettingsPanel.vue'), 'utf8')
  const basicSource = readFileSync(resolve(projectRoot, 'src/components/settings/SettingsBasicTab.vue'), 'utf8')
  const llmSource = readFileSync(resolve(projectRoot, 'src/components/settings/SettingsLLMTab.vue'), 'utf8')
  const settingsStyles = readFileSync(resolve(projectRoot, 'src/components/settings/settings-panel.css'), 'utf8')

  assert.match(panelSource, /settings-tab-label/)
  assert.match(basicSource, /settings-action-text/)
  assert.match(llmSource, /llm-protocol-badge/)
  assert.match(llmSource, /@media \(max-width: 480px\)/)
  assert.match(settingsStyles, /\.section-label\s*\{[\s\S]*?font-size:\s*12px;/)
  assert.match(settingsStyles, /\.settings-field-label\s*\{[\s\S]*?font-size:\s*14px;[\s\S]*?font-weight:\s*500;/)
  assert.match(settingsStyles, /\.settings-item-sub\s*\{[\s\S]*?font-size:\s*13px;/)
  assert.match(settingsStyles, /\.settings-action-text\s*\{[\s\S]*?font-size:\s*12px;/)
  assert.match(llmSource, /\.llm-protocol-badge\s*\{[\s\S]*?font-size:\s*11px;/)
  assert.match(settingsStyles, /@media\s*\(max-width:\s*720px\)\s*\{[\s\S]*?\.settings-body\s*\{[\s\S]*?flex-direction:\s*column;/)
  assert.match(settingsStyles, /@media\s*\(max-width:\s*720px\)\s*\{[\s\S]*?\.settings-sidebar\s*\{[\s\S]*?overflow-x:\s*auto;/)
})

test('Agent Team block is semantic, responsive, and disables active motion when requested', () => {
  const source = readFileSync(resolve(projectRoot, 'src/components/chat/AgentTeamBlock.vue'), 'utf8')

  assert.match(source, /<section[^>]*class="agent-team"/)
  assert.match(source, /<button[\s\S]*aria-haspopup="dialog"/)
  assert.match(source, /overflow-wrap:\s*anywhere;/)
  assert.match(source, /@media\s*\(max-width:\s*480px\)/)
  assert.match(source, /@media\s*\(prefers-reduced-motion:\s*reduce\)/)
  assert.doesNotMatch(source, /overflow-x:\s*(auto|scroll)/)
})

test('Agent Team detail dialog is height constrained and becomes a single column on mobile', () => {
  const path = resolve(projectRoot, 'src/components/chat/AgentTeamDetailDialog.vue')
  assert.ok(existsSync(path), 'AgentTeamDetailDialog.vue should exist')
  const source = readFileSync(path, 'utf8')

  assert.match(source, /max-height:\s*80vh;/)
  assert.match(source, /grid-template-columns:\s*minmax\(0,\s*220px\)\s+minmax\(0,\s*1fr\);/)
  assert.match(source, /@media\s*\(max-width:\s*640px\)[\s\S]*grid-template-columns:\s*minmax\(0,\s*1fr\);/)
  assert.match(source, /overflow-wrap:\s*anywhere;/)
  assert.doesNotMatch(source, /overflow-x:\s*(auto|scroll)/)
})
