import { ref } from 'vue'
import { settingAPI } from '@/api/settings'

type ToastLike = {
  error(message: string): void
  success(message: string): void
}

type Translate = (key: string) => string

export function useSettingsWebSearch(options: {
  toast: ToastLike
  t: Translate
}) {
  const { toast, t } = options
  const webSearchDialogVisible = ref(false)
  const webSearchKey = ref('')
  const savingWebSearch = ref(false)

  function openWebSearchDialog() {
    webSearchDialogVisible.value = true
  }

  function closeWebSearchDialog() {
    webSearchDialogVisible.value = false
  }

  async function saveWebSearch() {
    savingWebSearch.value = true
    try {
      await settingAPI.update({ webSearchKey: webSearchKey.value })
      closeWebSearchDialog()
      toast.success(t('saveSuccess'))
    } catch (err: unknown) {
      const response = err as { response?: { data?: { error?: string } } }
      toast.error(response.response?.data?.error || t('webSearchSaveFailed'))
    } finally {
      savingWebSearch.value = false
    }
  }

  return {
    webSearchDialogVisible,
    webSearchKey,
    savingWebSearch,
    openWebSearchDialog,
    closeWebSearchDialog,
    saveWebSearch,
  }
}
