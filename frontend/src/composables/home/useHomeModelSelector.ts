import { computed, ref } from 'vue'
import { llmAPI } from '@/api/llm'
import type { LLMConfig } from '@/types/settings'
import { useI18n } from 'vue-i18n'

const MODEL_STORAGE_KEY = 'slimebot:selectedModelId'
const THINKING_STORAGE_KEY = 'slimebot:thinkingLevel'
const SUBAGENT_MODEL_STORAGE_KEY = 'slimebot:subagentModelId'

export function useHomeModelSelector() {
  const { t } = useI18n()
  const modelOptions = ref<LLMConfig[]>([])
  const selectedModelId = ref('')
  const thinkingLevel = ref(localStorage.getItem(THINKING_STORAGE_KEY) || 'off')
  const subagentModelId = ref(localStorage.getItem(SUBAGENT_MODEL_STORAGE_KEY) || '')

  const modelSelectOptions = computed(() => modelOptions.value.map((m) => ({ value: m.id, label: m.name })))
  const hasModel = computed(() => modelOptions.value.length > 0)
  const thinkingSelectOptions = computed(() => [
    { value: 'off', label: t('thinkingOff') as string },
    { value: 'low', label: t('thinkingLow') as string },
    { value: 'medium', label: t('thinkingMedium') as string },
    { value: 'high', label: t('thinkingHigh') as string },
  ])

  function syncModelToLocal(modelId: string) {
    if (!modelId) {
      localStorage.removeItem(MODEL_STORAGE_KEY)
      return
    }
    localStorage.setItem(MODEL_STORAGE_KEY, modelId)
  }

  function resolveInitialModelId(items: LLMConfig[]) {
    const first = items[0]
    if (!first) return ''
    const remembered = localStorage.getItem(MODEL_STORAGE_KEY)
    const matched = remembered ? items.find((item) => item.id === remembered) : undefined
    return matched?.id || first.id
  }

  async function refreshModelOptions(useRemembered = false) {
    const latestModels = await llmAPI.list()
    modelOptions.value = latestModels

    let nextModelId = ''
    if (latestModels.length > 0) {
      const hasCurrent = selectedModelId.value && latestModels.some((item) => item.id === selectedModelId.value)
      if (hasCurrent) {
        nextModelId = selectedModelId.value
      } else if (useRemembered) {
        nextModelId = resolveInitialModelId(latestModels)
      } else {
        const firstModel = latestModels[0]
        nextModelId = firstModel ? firstModel.id : ''
      }
    }

    selectedModelId.value = nextModelId
    syncModelToLocal(nextModelId)
  }

  async function onModelChange(modelId: string) {
    selectedModelId.value = modelId
    syncModelToLocal(modelId)
  }

  function onThinkingLevelChange(level: string) {
    thinkingLevel.value = level
    localStorage.setItem(THINKING_STORAGE_KEY, level)
  }

  const subagentModelSelectOptions = computed(() => [
    { value: '', label: t('subagentModelFollow') as string },
    ...modelOptions.value.map((m) => ({ value: m.id, label: m.name })),
  ])

  function onSubagentModelChange(modelId: string) {
    subagentModelId.value = modelId
    if (!modelId) {
      localStorage.removeItem(SUBAGENT_MODEL_STORAGE_KEY)
    } else {
      localStorage.setItem(SUBAGENT_MODEL_STORAGE_KEY, modelId)
    }
  }

  return {
    modelOptions,
    selectedModelId,
    modelSelectOptions,
    hasModel,
    refreshModelOptions,
    onModelChange,
    thinkingLevel,
    thinkingSelectOptions,
    onThinkingLevelChange,
    subagentModelId,
    subagentModelSelectOptions,
    onSubagentModelChange,
  }
}
