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
    if (event.defaultPrevented) return
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

  onMounted(() => {
    window.addEventListener('keydown', onKeydown)
  })

  onUnmounted(() => {
    window.removeEventListener('keydown', onKeydown)
  })

  return {
    isMac,
    isDesktop,
  }
}
