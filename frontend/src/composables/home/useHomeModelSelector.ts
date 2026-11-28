import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { llmAPI } from '@/api/llm'
import { acpAPI, type ACPConfig } from '@/api/acp'
import type { LLMConfig } from '@/types/settings'
import { groupedModelOptions } from '@/utils/modelSelectOptions'
import { acpAgentIdFromEngine, acpEngineId, chooseEngine, composeEngineOptions, isACPEngine } from '@/utils/acp'

const MODEL_STORAGE_KEY = 'slimebot:selectedModelId'
const THINKING_STORAGE_KEY = 'slimebot:thinkingLevel'
const SUBAGENT_MODEL_STORAGE_KEY = 'slimebot:subagentModelId'

export function useHomeModelSelector() {
  const { t } = useI18n()
  const modelOptions = ref<LLMConfig[]>([])
  const acpAgents = ref<ACPConfig[]>([])
  const boundAgentId = ref<string>()
  const selectedModelId = ref('')
  const thinkingLevel = ref(localStorage.getItem(THINKING_STORAGE_KEY) || 'off')
  const subagentModelId = ref(localStorage.getItem(SUBAGENT_MODEL_STORAGE_KEY) || '')

  const modelSelectOptions = computed(() => groupedModelOptions(modelOptions.value))
  const engineSelectOptions = computed(() => composeEngineOptions(
    modelSelectOptions.value,
    acpAgents.value,
    t('acpLocalGroup') as string,
    t('acpModelGroup') as string,
  ))
  const engineCount = computed(() => engineSelectOptions.value.length)
  const hasModel = computed(() => engineCount.value > 0)
  const enabledACPAgents = computed(() => acpAgents.value.filter(agent => agent.isEnabled))
  const selectedACPConfig = computed(() => {
    const agentId = acpAgentIdFromEngine(selectedModelId.value)
    return agentId ? acpAgents.value.find(agent => agent.id === agentId) : undefined
  })
  const externalEngine = computed(() => isACPEngine(selectedModelId.value))
  const thinkingSelectOptions = computed(() => [
    { value: 'off', label: t('thinkingOff') as string },
    { value: 'low', label: t('thinkingLow') as string },
    { value: 'medium', label: t('thinkingMedium') as string },
    { value: 'high', label: t('thinkingHigh') as string },
    { value: 'max', label: t('thinkingMax') as string },
  ])

  function syncModelToLocal(modelId: string) {
    if (!modelId) {
      localStorage.removeItem(MODEL_STORAGE_KEY)
      return
    }
    localStorage.setItem(MODEL_STORAGE_KEY, modelId)
  }

  async function loadACPConfigs() {
    try {
      acpAgents.value = await acpAPI.list()
    } catch {
      // Keep the previous list so a transient settings failure does not drop the picker.
    }
  }

  function isSelectableEngine(value: string | null | undefined) {
    return !!value && engineSelectOptions.value.some(option => option.value === value)
  }

  async function refreshModelOptions(useRemembered = false) {
    const latestModels = await llmAPI.list().catch(() => modelOptions.value)
    await loadACPConfigs()
    modelOptions.value = latestModels
    const remembered = localStorage.getItem(MODEL_STORAGE_KEY)
    const bound = boundAgentId.value && enabledACPAgents.value.some(agent => agent.id === boundAgentId.value)
      ? acpEngineId(boundAgentId.value!)
      : undefined
    const next = chooseEngine(
      engineSelectOptions.value,
      isSelectableEngine(selectedModelId.value) ? selectedModelId.value : undefined,
      bound,
      useRemembered && isSelectableEngine(remembered) ? remembered : null,
    )
    selectedModelId.value = next
    syncModelToLocal(next)
  }

  async function onModelChange(modelId: string) {
    selectedModelId.value = modelId
    syncModelToLocal(modelId)
  }

  function applySessionAgent(agentId?: string) {
    boundAgentId.value = agentId
    if (agentId && enabledACPAgents.value.some(agent => agent.id === agentId)) {
      selectedModelId.value = acpEngineId(agentId)
      syncModelToLocal(selectedModelId.value)
    }
  }

  function onThinkingLevelChange(level: string) {
    thinkingLevel.value = level
    localStorage.setItem(THINKING_STORAGE_KEY, level)
  }

  const subagentModelSelectOptions = computed(() => [
    { value: '', label: t('subagentModelFollow') as string },
    ...modelSelectOptions.value,
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
    acpAgents,
    enabledACPAgents,
    selectedACPConfig,
    externalEngine,
    selectedModelId,
    modelSelectOptions,
    engineSelectOptions,
    engineCount,
    hasModel,
    refreshModelOptions,
    loadACPConfigs,
    onModelChange,
    applySessionAgent,
    thinkingLevel,
    thinkingSelectOptions,
    onThinkingLevelChange,
    subagentModelId,
    subagentModelSelectOptions,
    onSubagentModelChange,
  }
}
