import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { createTerminal, deleteTerminal, getTerminalStatus, type TerminalOptions, type TerminalSession } from '@/api/terminal'

export const useTerminalStore = defineStore('terminal', () => {
  const isOpen = ref(false)
  const isMaximized = ref(false)
  const heightPx = ref(Number(localStorage.getItem('slimebot:terminal-height')) || 280)
  const tabs = ref<TerminalSession[]>([])
  const activeTabId = ref('')
  const enabled = ref(true)
  const lockedByEnv = ref(false)
  const defaultShell = ref('')
  const busy = ref(false)
  const activeTab = computed(() => tabs.value.find(tab => tab.id === activeTabId.value))
  const hasRunning = computed(() => tabs.value.some(tab => tab.status === 'running'))

  function setHeight(height: number) {
    heightPx.value = Math.round(Math.min(Math.max(height, 160), Math.max(160, window.innerHeight * 0.75)))
    localStorage.setItem('slimebot:terminal-height', String(heightPx.value))
  }

  async function refresh() {
    const result = await getTerminalStatus()
    enabled.value = result.enabled
    lockedByEnv.value = result.lockedByEnv
    defaultShell.value = result.defaultShell
    tabs.value = result.sessions
    if (!activeTab.value) activeTabId.value = tabs.value[0]?.id || ''
  }

  function update(tab: TerminalSession) {
    if (tabs.value.some(item => item.id === tab.id)) {
      tabs.value = tabs.value.map(item => item.id === tab.id ? tab : item)
    }
  }

  async function create(options: TerminalOptions) {
    const session = await createTerminal(options)
    tabs.value.push(session)
    activeTabId.value = session.id
    return session
  }

  async function close(id: string) {
    await deleteTerminal(id)
    const index = tabs.value.findIndex(tab => tab.id === id)
    tabs.value = tabs.value.filter(tab => tab.id !== id)
    if (activeTabId.value === id) activeTabId.value = tabs.value[Math.max(0, index - 1)]?.id || ''
  }

  async function openFor(options: TerminalOptions, matchSession = false) {
    if (busy.value) return
    isOpen.value = true
    busy.value = true
    try {
      await refresh()
      if (!enabled.value) return
      if (!matchSession && activeTab.value) return activeTab.value
      const match = tabs.value.find(tab => tab.status === 'running'
        && tab.chatSessionId === (options.chatSessionId || '')
        && (options.chatSessionId || !options.cwd || tab.cwd === options.cwd))
      if (match) {
        activeTabId.value = match.id
        return match
      }
      return await create(options)
    } finally {
      busy.value = false
    }
  }

  return { isOpen, isMaximized, heightPx, tabs, activeTabId, activeTab, enabled, lockedByEnv,
    defaultShell, busy, hasRunning, setHeight, refresh, update, create, close, openFor }
})
