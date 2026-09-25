import { computed, ref, type MaybeRefOrGetter, toValue, type Ref } from 'vue'
import { settingAPI } from '@/api/settings'
import { messagePlatformAPI } from '@/api/messagePlatform'
import type { ApprovalMode, LLMConfig, MessagePlatformConfig, ThinkingLevel } from '@/types/settings'
import { getApprovalModeOptions } from '@/utils/approvalMode'
import { createMessagePlatformThinkingOptions, shouldSaveMessagePlatformDefaultModel } from '@/utils/messagePlatformSettings'
import { groupedModelOptions } from '@/utils/modelSelectOptions'

type ToastLike = {
  error(message: string): void
}

type Translate = (key: string) => string

export function useSettingsMessagePlatform(options: {
  messagePlatformList: Ref<MessagePlatformConfig[]>
  messagePlatformDialogVisible: Ref<boolean>
  messagePlatformSubmitting: Ref<boolean>
  messagePlatformDefaultModel: Ref<string>
  messagePlatformThinkingLevel: Ref<ThinkingLevel>
  messagePlatformApprovalMode: Ref<ApprovalMode>
  llmRows: MaybeRefOrGetter<LLMConfig[]>
  toast: ToastLike
  t: Translate
}) {
  const {
    messagePlatformList,
    messagePlatformDialogVisible,
    messagePlatformSubmitting,
    messagePlatformDefaultModel,
    messagePlatformThinkingLevel,
    messagePlatformApprovalMode,
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

  const telegramConfig = computed(() => messagePlatformList.value.find((item) => item.platform === 'telegram'))
  const messagePlatformModelOptions = computed(() => {
    const base = groupedModelOptions(toValue(llmRows) || [])
    return [{ value: '', label: t('messagePlatformModelUnset') }, ...base]
  })
  const messagePlatformThinkingOptions = computed(() => createMessagePlatformThinkingOptions(t))
  const messagePlatformApprovalOptions = computed(() =>
    getApprovalModeOptions().map((item) => ({ value: item.value, label: t(item.labelKey) })),
  )

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
    if (!shouldSaveMessagePlatformDefaultModel(modelId)) return
    await settingAPI.update({ messagePlatformDefaultModel: modelId })
  }

  async function saveMessagePlatformThinkingLevel(level: ThinkingLevel) {
    messagePlatformThinkingLevel.value = level
    await settingAPI.update({ messagePlatformThinkingLevel: level })
  }

  async function saveMessagePlatformApprovalMode(mode: ApprovalMode) {
    messagePlatformApprovalMode.value = mode
    await settingAPI.update({ messagePlatformApprovalMode: mode })
  }

  return {
    messagePlatformForm,
    telegramConfig,
    messagePlatformModelOptions,
    messagePlatformThinkingOptions,
    messagePlatformApprovalOptions,
    openMessagePlatformDialog,
    saveMessagePlatformConfig,
    toggleTelegramEnabled,
    saveMessagePlatformDefaultModel,
    saveMessagePlatformThinkingLevel,
    saveMessagePlatformApprovalMode,
  }
}
