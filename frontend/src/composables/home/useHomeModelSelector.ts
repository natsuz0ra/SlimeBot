import { computed, ref } from 'vue'
import { llmAPI, type LLMConfig } from '@/api/settings'

const MODEL_STORAGE_KEY = 'slimebot:selectedModelId'

export function useHomeModelSelector() {
  const modelOptions = ref<LLMConfig[]>([])
  const selectedModelId = ref('')

  const modelSelectOptions = computed(() => modelOptions.value.map((m) => ({ value: m.id, label: m.name })))
  const hasModel = computed(() => modelOptions.value.length > 0)

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

  return {
    modelOptions,
    selectedModelId,
    modelSelectOptions,
    hasModel,
    refreshModelOptions,
    onModelChange,
  }
}
