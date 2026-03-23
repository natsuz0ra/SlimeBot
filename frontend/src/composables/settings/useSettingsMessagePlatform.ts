import { computed, ref, type MaybeRefOrGetter, toValue, type Ref } from 'vue'
import { settingAPI } from '@/api/settings'
import { messagePlatformAPI } from '@/api/messagePlatform'

type ToastLike = {
  error(message: string): void
}

type Translate = (key: string) => string

type PlatformItem = any
type LLMItem = any

export function useSettingsMessagePlatform(options: {
  messagePlatformList: Ref<PlatformItem[]>
  messagePlatformDialogVisible: Ref<boolean>
  messagePlatformSubmitting: Ref<boolean>
  messagePlatformDefaultModel: Ref<string>
  llmRows: MaybeRefOrGetter<LLMItem[]>
  toast: ToastLike
  t: Translate
}) {
  const {
    messagePlatformList,
    messagePlatformDialogVisible,
    messagePlatformSubmitting,
    messagePlatformDefaultModel,
    llmRows,
    toast,
    t,
  } = options

  const messagePlatformForm = ref({
    id: '',
    platform: 'telegram',
    displayName: 'Telegram',
    botToken: '',
    isEnabled: true,
  })

  const telegramConfig = computed(() => messagePlatformList.value.find((item: PlatformItem) => item.platform === 'telegram'))
  const messagePlatformModelOptions = computed(() => {
    const base = (toValue(llmRows) || []).map((item: LLMItem) => ({ value: item.id, label: item.name }))
    return [{ value: '', label: t('messagePlatformModelUnset') }, ...base]
  })

  function getBotTokenFromAuthConfig(raw: string) {
    try {
      const parsed = JSON.parse(raw || '{}')
      return String(parsed?.botToken || '')
    } catch {
      return ''
    }
  }

  function buildPlatformAuthConfigJson(botToken: string) {
    return JSON.stringify({ botToken: botToken.trim() })
  }

  async function refreshPlatforms() {
    messagePlatformList.value = await messagePlatformAPI.list()
  }

  function openMessagePlatformDialog() {
    const row = telegramConfig.value
    if (!row) {
      messagePlatformForm.value = {
        id: '',
        platform: 'telegram',
        displayName: 'Telegram',
        botToken: '',
        isEnabled: true,
      }
    } else {
      messagePlatformForm.value = {
        id: row.id,
        platform: row.platform,
        displayName: row.displayName,
        botToken: getBotTokenFromAuthConfig(row.authConfigJson),
        isEnabled: !!row.isEnabled,
      }
    }
    messagePlatformDialogVisible.value = true
  }

  async function saveMessagePlatformConfig() {
    if (!messagePlatformForm.value.botToken.trim()) {
      toast.error(t('botTokenRequired'))
      return
    }
    messagePlatformSubmitting.value = true
    try {
      const payload = {
        platform: messagePlatformForm.value.platform,
        displayName: messagePlatformForm.value.displayName,
        authConfigJson: buildPlatformAuthConfigJson(messagePlatformForm.value.botToken),
        isEnabled: messagePlatformForm.value.isEnabled,
      }
      if (messagePlatformForm.value.id) {
        await messagePlatformAPI.update(messagePlatformForm.value.id, payload)
      } else {
        await messagePlatformAPI.create(payload)
      }
      await refreshPlatforms()
      messagePlatformDialogVisible.value = false
    } finally {
      messagePlatformSubmitting.value = false
    }
  }

  async function toggleTelegramEnabled() {
    const row = telegramConfig.value
    if (!row) return
    await messagePlatformAPI.update(row.id, {
      platform: row.platform,
      displayName: row.displayName,
      authConfigJson: row.authConfigJson,
      isEnabled: !row.isEnabled,
    })
    await refreshPlatforms()
  }

  async function saveMessagePlatformDefaultModel(modelId: string) {
    messagePlatformDefaultModel.value = modelId
    if (!modelId) return
    await settingAPI.update({ messagePlatformDefaultModel: modelId } as any)
  }

  return {
    messagePlatformForm,
    telegramConfig,
    messagePlatformModelOptions,
    openMessagePlatformDialog,
    saveMessagePlatformConfig,
    toggleTelegramEnabled,
    saveMessagePlatformDefaultModel,
  }
}
