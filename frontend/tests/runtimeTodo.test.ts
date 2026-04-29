import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import test from 'node:test'

test('chat socket and store wire runtime todo updates outside history', () => {
  const chatSocketSource = readFileSync(resolve(import.meta.dirname, '../src/api/chatSocket.ts'), 'utf8')
  const chatStoreSource = readFileSync(resolve(import.meta.dirname, '../src/stores/chat.ts'), 'utf8')

  assert.match(chatSocketSource, /export interface TodoUpdateData/)
  assert.match(chatSocketSource, /if \(data\.type === 'todo_update'\) handlers\?\.onTodoUpdate\?\.\(/)
  assert.match(chatSocketSource, /items: Array\.isArray\(data\.items\) \? data\.items : \[\]/)

  assert.match(chatStoreSource, /const runtimeTodos = ref<RuntimeTodoItem\[]>\(\[\]\)/)
  assert.match(chatStoreSource, /function applyRuntimeTodoUpdate\(update: TodoUpdateData, sessionId\?: string\)/)
  assert.match(chatStoreSource, /if \(!sessionId \|\| sessionId !== currentSessionId\.value\) return/)
  assert.match(chatStoreSource, /todoPanelOpen\.value = runtimeTodos\.value\.length > 0/)
  assert.match(chatStoreSource, /function clearRuntimeTodos\(\)/)
})

test('HomePage mounts the runtime todo panel outside message history', () => {
  const homeSource = readFileSync(resolve(import.meta.dirname, '../src/pages/HomePage.vue'), 'utf8')
  const panelSource = readFileSync(resolve(import.meta.dirname, '../src/components/chat/TodoPanel.vue'), 'utf8')

  assert.match(homeSource, /import TodoPanel from '@\/components\/chat\/TodoPanel\.vue'/)
  assert.match(homeSource, /<TodoPanel[\s\S]*:items="store\.runtimeTodos"[\s\S]*@toggle="store\.toggleTodoPanel"/)
  assert.match(panelSource, /todo-panel--open/)
  assert.match(panelSource, /symbolFor\(item\.status\)/)
})
