import { onMounted, onUnmounted } from 'vue'
import { detectMacPlatform, resolveHomeShortcut, type ShortcutAction } from '@/utils/shortcuts'

export interface UseHomeShortcutsOptions {
  hasModalOpen: () => boolean
  searchVisible: () => boolean
  settingsVisible: () => boolean
  shortcutsHelpVisible: () => boolean
  hasPopoverOrMenuOpen: () => boolean
  editingMessage: () => boolean
  pendingPlanConfirmation: () => boolean
  isTasksWorkspace: () => boolean
  isStreaming: () => boolean
  stopDisabled: () => boolean
  isMessagePlatformSession: () => boolean
  onAction: (action: Exclude<ShortcutAction, null>) => void
}

function isEditableElement(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  if (target.isContentEditable) return true
  const tag = target.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true
  return Boolean(target.closest('.cm-editor'))
}

function isComposerTextarea(target: EventTarget | null): boolean {
  return target instanceof HTMLTextAreaElement && target.dataset.chatComposerInput === 'true'
}

function hasOpenDomPopoverOrMenu(): boolean {
  return Boolean(
    document.querySelector('.context-usage-panel, .composer-settings-panel, .floating-session-menu, .header-dropdown'),
  )
}

function hasActiveMessageEditor(): boolean {
  return Boolean(document.querySelector('.message-edit-box, [data-message-edit-input="true"], .message-edit-textarea'))
}

export function useHomeShortcuts(options: UseHomeShortcutsOptions) {
  const isMac = detectMacPlatform()
  const isDesktop = typeof window !== 'undefined' && Boolean(window.slimebotDesktop)

  function onKeydown(event: KeyboardEvent) {
    const inTerminal = event.target instanceof Element && Boolean(event.target.closest('.xterm'))
    if (event.defaultPrevented || inTerminal) return
    const action = resolveHomeShortcut(event, {
      isMac,
      isDesktop,
      hasModalOpen: options.hasModalOpen(),
      searchVisible: options.searchVisible(),
      settingsVisible: options.settingsVisible(),
      shortcutsHelpVisible: options.shortcutsHelpVisible(),
      hasPopoverOrMenuOpen: options.hasPopoverOrMenuOpen() || hasOpenDomPopoverOrMenu(),
      editingMessage: options.editingMessage() || hasActiveMessageEditor(),
      pendingPlanConfirmation: options.pendingPlanConfirmation(),
      isTasksWorkspace: options.isTasksWorkspace(),
      isStreaming: options.isStreaming(),
      stopDisabled: options.stopDisabled(),
      isMessagePlatformSession: options.isMessagePlatformSession(),
      inComposerTextarea: isComposerTextarea(event.target),
      inEditableElement: isEditableElement(event.target),
    })
    if (!action) return
    event.preventDefault()
    event.stopPropagation()
    options.onAction(action)
  }

  function onTerminalKeydown(event: KeyboardEvent) {
    if (!(event.target instanceof Element) || !event.target.closest('.xterm')) return
    if (event.isComposing || event.keyCode === 229 || event.code !== 'Backquote' || event.altKey || event.shiftKey) return
    if (!(isMac ? event.metaKey && !event.ctrlKey : event.ctrlKey && !event.metaKey)) return
    if (options.hasModalOpen() || options.isTasksWorkspace()) return
    event.preventDefault()
    event.stopPropagation()
    options.onAction('toggle-terminal')
  }

  onMounted(() => {
    window.addEventListener('keydown', onKeydown)
    window.addEventListener('keydown', onTerminalKeydown, true)
  })

  onUnmounted(() => {
    window.removeEventListener('keydown', onKeydown)
    window.removeEventListener('keydown', onTerminalKeydown, true)
  })

  return {
    isMac,
    isDesktop,
  }
}
