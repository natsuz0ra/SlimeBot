import { onMounted, onUnmounted, ref } from 'vue'

export function useHomeUiState() {
  const desktopLayout = window.matchMedia('(min-width: 768px)')
  const drawerOpen = ref(desktopLayout.matches)
  const renameVisible = ref(false)
  const renameValue = ref('')
  const renameTargetId = ref('')
  const inputValue = ref('')
  const pendingFiles = ref<File[]>([])
  const loading = ref(false)
  const settingsVisible = ref(false)
  const activeSessionMenu = ref<{ id: string; x: number; y: number } | null>(null)
  const topMenuVisible = ref(false)
  const deleteConfirmVisible = ref(false)
  const deleteTargetId = ref('')

  function onGlobalClick() {
    activeSessionMenu.value = null
    topMenuVisible.value = false
  }

  function toggleSidebar() {
    drawerOpen.value = !drawerOpen.value
  }

  function onLayoutChange(event: MediaQueryListEvent) {
    drawerOpen.value = event.matches
  }

  function toggleSessionMenu(sessionId: string, event: MouseEvent) {
    const target = event.currentTarget as HTMLElement | null
    if (!target) return
    if (activeSessionMenu.value?.id === sessionId) {
      activeSessionMenu.value = null
      return
    }
    const rect = target.getBoundingClientRect()
    activeSessionMenu.value = { id: sessionId, x: rect.right + 6, y: rect.top }
  }

  onMounted(() => {
    document.addEventListener('click', onGlobalClick)
    desktopLayout.addEventListener('change', onLayoutChange)
  })

  onUnmounted(() => {
    document.removeEventListener('click', onGlobalClick)
    desktopLayout.removeEventListener('change', onLayoutChange)
  })

  return {
    drawerOpen,
    renameVisible,
    renameValue,
    renameTargetId,
    inputValue,
    pendingFiles,
    loading,
    settingsVisible,
    activeSessionMenu,
    topMenuVisible,
    deleteConfirmVisible,
    deleteTargetId,
    toggleSidebar,
    toggleSessionMenu,
  }
}
