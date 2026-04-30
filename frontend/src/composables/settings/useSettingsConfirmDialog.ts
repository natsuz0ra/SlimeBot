import { ref } from 'vue'

export function useSettingsConfirmDialog() {
  const confirmDialogVisible = ref(false)
  const confirmDialogCallback = ref<(() => Promise<void>) | null>(null)

  function openConfirmDialog(callback: () => Promise<void>) {
    confirmDialogCallback.value = callback
    confirmDialogVisible.value = true
  }

  async function runConfirmDialog() {
    if (confirmDialogCallback.value) await confirmDialogCallback.value()
    confirmDialogVisible.value = false
    confirmDialogCallback.value = null
  }

  return {
    confirmDialogVisible,
    openConfirmDialog,
    runConfirmDialog,
  }
}
