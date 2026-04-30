import { ref } from 'vue'
import { settingAPI } from '@/api/settings'
import type { ApprovalMode } from '@/types/settings'

type ToastLike = {
  error(message: string): void
  success(message: string): void
}

type Translate = (key: string) => string

export function useSettingsApprovalMode(options: {
  toast: ToastLike
  t: Translate
}) {
  const { toast, t } = options
  const approvalMode = ref<ApprovalMode>('standard')
  const savingApprovalMode = ref(false)

  async function onApprovalModeChange(mode: ApprovalMode) {
    const previous = approvalMode.value
    approvalMode.value = mode
    savingApprovalMode.value = true
    try {
      await settingAPI.update({ approvalMode: mode })
      toast.success(t('saveSuccess'))
    } catch {
      approvalMode.value = previous
      toast.error(t('approvalModeSaveFailed'))
    } finally {
      savingApprovalMode.value = false
    }
  }

  return {
    approvalMode,
    savingApprovalMode,
    onApprovalModeChange,
  }
}
