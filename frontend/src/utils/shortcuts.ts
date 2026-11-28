export interface ShortcutEventLike {
  key: string
  code?: string
  metaKey?: boolean
  ctrlKey?: boolean
  altKey?: boolean
  shiftKey?: boolean
  isComposing?: boolean
  keyCode?: number
}

export interface ShortcutContext {
  isMac: boolean
  isDesktop: boolean
  hasModalOpen: boolean
  searchVisible: boolean
  settingsVisible: boolean
  shortcutsHelpVisible: boolean
  hasPopoverOrMenuOpen: boolean
  editingMessage: boolean
  pendingPlanConfirmation: boolean
  isTasksWorkspace: boolean
  isStreaming: boolean
  stopDisabled: boolean
  isMessagePlatformSession: boolean
  inComposerTextarea: boolean
  inEditableElement: boolean
  inTerminal?: boolean
}

export type ShortcutAction =
  | 'toggle-search'
  | 'new-session'
  | 'toggle-sidebar'
  | 'toggle-terminal'
  | 'toggle-settings'
  | 'toggle-plan-mode'
  | 'stop-stream'
  | 'toggle-shortcuts-help'
  | null

export function detectMacPlatform(platform?: string): boolean {
  const value = platform ?? (typeof navigator !== 'undefined' ? (navigator.platform || navigator.userAgent || '') : '')
  return /mac|iphone|ipad|ipod/i.test(value)
}

export function formatShortcutKeys(isMac: boolean, keys: { mac: string; other: string }): string {
  return isMac ? keys.mac : keys.other
}

export interface ShortcutHelpItem {
  action: Exclude<ShortcutAction, null>
  labelKey: string
  keys: string
}

export function getShortcutHelpItems(isMac: boolean, isDesktop: boolean): ShortcutHelpItem[] {
  const newChatKeys = isDesktop
    ? formatShortcutKeys(isMac, { mac: '⌘N / ⌘⇧O', other: 'Ctrl+N / Ctrl+Shift+O' })
    : formatShortcutKeys(isMac, { mac: '⌘⇧O', other: 'Ctrl+Shift+O' })
  return [
    { action: 'toggle-terminal', labelKey: 'terminalTitle', keys: formatShortcutKeys(isMac, { mac: '⌘`', other: 'Ctrl+`' }) },
    { action: 'toggle-search', labelKey: 'shortcutSearch', keys: formatShortcutKeys(isMac, { mac: '⌘K', other: 'Ctrl+K' }) },
    { action: 'new-session', labelKey: 'shortcutNewChat', keys: newChatKeys },
    { action: 'toggle-sidebar', labelKey: 'shortcutToggleSidebar', keys: formatShortcutKeys(isMac, { mac: '⌘B', other: 'Ctrl+B' }) },
    { action: 'toggle-settings', labelKey: 'shortcutOpenSettings', keys: formatShortcutKeys(isMac, { mac: '⌘,', other: 'Ctrl+,' }) },
    { action: 'toggle-plan-mode', labelKey: 'shortcutTogglePlanMode', keys: 'Shift+Tab' },
    { action: 'stop-stream', labelKey: 'shortcutStopStream', keys: 'Esc' },
    { action: 'toggle-shortcuts-help', labelKey: 'shortcutHelp', keys: formatShortcutKeys(isMac, { mac: '⌘/', other: 'Ctrl+/' }) },
  ]
}

export function resolveHomeShortcut(event: ShortcutEventLike, context: ShortcutContext): ShortcutAction {
  if (event.isComposing || event.keyCode === 229) {
    return null
  }

  const key = (event.key || '').toLowerCase()
  const code = event.code || ''
  const mod = context.isMac ? Boolean(event.metaKey && !event.ctrlKey) : Boolean(event.ctrlKey && !event.metaKey)
  const alt = Boolean(event.altKey)
  const shift = Boolean(event.shiftKey)
  const terminalKey = mod && !alt && !shift && code === 'Backquote'
  if (context.inTerminal) return terminalKey && !context.hasModalOpen && !context.isTasksWorkspace ? 'toggle-terminal' : null

  const isSearchKey = mod && !alt && !shift && key === 'k'
  const isSettingsKey = mod && !alt && !shift && (key === ',' || code === 'Comma')
  const isHelpKey = mod && !alt && !shift && (key === '/' || code === 'Slash')

  if (context.searchVisible) {
    return isSearchKey ? 'toggle-search' : null
  }
  if (context.settingsVisible) {
    return isSettingsKey ? 'toggle-settings' : null
  }
  if (context.shortcutsHelpVisible) {
    return isHelpKey ? 'toggle-shortcuts-help' : null
  }
  if (context.hasModalOpen) {
    return null
  }
  if (terminalKey && !context.isTasksWorkspace) return 'toggle-terminal'

  if (isSearchKey) return 'toggle-search'
  if (isSettingsKey) return 'toggle-settings'
  if (isHelpKey) return 'toggle-shortcuts-help'

  if (mod && !alt && !shift && key === 'b') {
    return 'toggle-sidebar'
  }
  if (mod && !alt && shift && key === 'o') {
    return 'new-session'
  }
  if (context.isDesktop && mod && !alt && !shift && key === 'n') {
    return 'new-session'
  }

  if (!mod && !alt && shift && key === 'tab') {
    if (
      context.inComposerTextarea
      && !context.isTasksWorkspace
      && !context.isMessagePlatformSession
      && !context.hasPopoverOrMenuOpen
      && !context.pendingPlanConfirmation
    ) {
      return 'toggle-plan-mode'
    }
    return null
  }

  if (!mod && !alt && !shift && key === 'escape') {
    if (
      context.isStreaming
      && !context.stopDisabled
      && !context.isTasksWorkspace
      && !context.hasPopoverOrMenuOpen
      && !context.editingMessage
      && !context.pendingPlanConfirmation
    ) {
      return 'stop-stream'
    }
    return null
  }

  return null
}
