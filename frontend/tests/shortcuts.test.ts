import test from 'node:test'
import assert from 'node:assert/strict'

import {
  getShortcutHelpItems,
  resolveHomeShortcut,
  type ShortcutContext,
} from '../src/utils/shortcuts'

function baseContext(overrides: Partial<ShortcutContext> = {}): ShortcutContext {
  return {
    isMac: false,
    isDesktop: false,
    hasModalOpen: false,
    searchVisible: false,
    settingsVisible: false,
    shortcutsHelpVisible: false,
    hasPopoverOrMenuOpen: false,
    editingMessage: false,
    pendingPlanConfirmation: false,
    isTasksWorkspace: false,
    isStreaming: false,
    stopDisabled: true,
    isMessagePlatformSession: false,
    inComposerTextarea: false,
    inEditableElement: false,
    ...overrides,
  }
}

test('resolveHomeShortcut matches platform modifiers for search, sidebar, settings, help and new chat', () => {
  assert.equal(resolveHomeShortcut({ key: 'k', ctrlKey: true }, baseContext()), 'toggle-search')
  assert.equal(resolveHomeShortcut({ key: 'k', metaKey: true }, baseContext({ isMac: true })), 'toggle-search')
  assert.equal(resolveHomeShortcut({ key: 'k', metaKey: true }, baseContext({ isMac: false })), null)

  assert.equal(resolveHomeShortcut({ key: 'b', ctrlKey: true }, baseContext()), 'toggle-sidebar')
  assert.equal(resolveHomeShortcut({ key: ',', ctrlKey: true }, baseContext()), 'toggle-settings')
  assert.equal(resolveHomeShortcut({ key: '/', ctrlKey: true }, baseContext()), 'toggle-shortcuts-help')

  assert.equal(resolveHomeShortcut({ key: 'O', ctrlKey: true, shiftKey: true }, baseContext()), 'new-session')
  // Browser Web ignores Ctrl+N, Desktop enables it
  assert.equal(resolveHomeShortcut({ key: 'n', ctrlKey: true }, baseContext({ isDesktop: false })), null)
  assert.equal(resolveHomeShortcut({ key: 'n', ctrlKey: true }, baseContext({ isDesktop: true })), 'new-session')
})

test('resolveHomeShortcut ignores IME composition events', () => {
  assert.equal(resolveHomeShortcut({ key: 'k', ctrlKey: true, isComposing: true }, baseContext()), null)
  assert.equal(resolveHomeShortcut({ key: 'Escape', keyCode: 229 }, baseContext({ isStreaming: true, stopDisabled: false })), null)
})

test('resolveHomeShortcut enforces modal precedence', () => {
  assert.equal(resolveHomeShortcut({ key: 'k', ctrlKey: true }, baseContext({ hasModalOpen: true, searchVisible: true })), 'toggle-search')
  assert.equal(resolveHomeShortcut({ key: 'b', ctrlKey: true }, baseContext({ hasModalOpen: true, searchVisible: true })), null)

  assert.equal(resolveHomeShortcut({ key: ',', ctrlKey: true }, baseContext({ hasModalOpen: true, settingsVisible: true })), 'toggle-settings')
  assert.equal(resolveHomeShortcut({ key: 'k', ctrlKey: true }, baseContext({ hasModalOpen: true, settingsVisible: true })), null)

  assert.equal(resolveHomeShortcut({ key: '/', ctrlKey: true }, baseContext({ hasModalOpen: true, shortcutsHelpVisible: true })), 'toggle-shortcuts-help')
  assert.equal(resolveHomeShortcut({ key: 'Escape' }, baseContext({ hasModalOpen: true, isStreaming: true, stopDisabled: false })), null)
})

test('resolveHomeShortcut scopes Shift+Tab to composer and guards Esc stop-stream', () => {
  assert.equal(resolveHomeShortcut({ key: 'Tab', shiftKey: true }, baseContext({ inComposerTextarea: false })), null)
  assert.equal(resolveHomeShortcut({ key: 'Tab', shiftKey: true }, baseContext({ inComposerTextarea: true })), 'toggle-plan-mode')
  assert.equal(resolveHomeShortcut({ key: 'Tab', shiftKey: true }, baseContext({ inComposerTextarea: true, isTasksWorkspace: true })), null)

  assert.equal(resolveHomeShortcut({ key: 'Escape' }, baseContext({ isStreaming: true, stopDisabled: false })), 'stop-stream')
  assert.equal(resolveHomeShortcut({ key: 'Escape' }, baseContext({ isStreaming: true, stopDisabled: false, hasPopoverOrMenuOpen: true })), null)
  assert.equal(resolveHomeShortcut({ key: 'Escape' }, baseContext({ isStreaming: true, stopDisabled: false, editingMessage: true })), null)
  assert.equal(resolveHomeShortcut({ key: 'Escape' }, baseContext({ isStreaming: true, stopDisabled: false, pendingPlanConfirmation: true })), null)
})

test('getShortcutHelpItems adapts labels to macOS and Desktop', () => {
  const macDesktop = getShortcutHelpItems(true, true)
  assert.equal(macDesktop.find((item) => item.action === 'toggle-search')?.keys, '⌘K')
  assert.equal(macDesktop.find((item) => item.action === 'new-session')?.keys, '⌘N / ⌘⇧O')

  const winWeb = getShortcutHelpItems(false, false)
  assert.equal(winWeb.find((item) => item.action === 'toggle-search')?.keys, 'Ctrl+K')
  assert.equal(winWeb.find((item) => item.action === 'new-session')?.keys, 'Ctrl+Shift+O')
})
