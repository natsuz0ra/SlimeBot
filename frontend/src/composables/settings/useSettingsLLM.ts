import { computed, ref, type Ref } from 'vue'
import { llmAPI } from '@/api/llm'
import type { DiscoveredModel, LLMConfig, LLMProvider } from '@/types/settings'
import { CONTEXT_SIZE_DEFAULT, clampContextSize, contextSizeToSlider, formatContextSize, sliderToContextSize } from '@/utils/contextSize'

type ToastLike = { error(message: string): void }
type Translate = (key: string) => string

function emptyProviderForm() {
  return { name: '', protocol: 'openai' as LLMProvider['protocol'], baseUrl: '', apiKey: '', clearApiKey: false }
}

function emptyModelForm(providerId = '') {
  return { providerId, name: '', model: '', contextSize: CONTEXT_SIZE_DEFAULT }
}

function errorMessage(error: unknown, fallback: string) {
  const response = error as { response?: { data?: { error?: string } }; message?: string }
  return response.response?.data?.error || response.message || fallback
}

export function useSettingsLLM(options: {
  llmList: Ref<LLMConfig[]>
  providerList: Ref<LLMProvider[]>
  providerDialogVisible: Ref<boolean>
  modelDialogVisible: Ref<boolean>
  llmSubmitting: Ref<boolean>
  providerSubmitting: Ref<boolean>
  toast: ToastLike
  t: Translate
  onChanged?: () => void
}) {
  const { llmList, providerList, providerDialogVisible, modelDialogVisible, llmSubmitting, providerSubmitting, toast, t, onChanged } = options
  const providerForm = ref(emptyProviderForm())
  const providerEditingId = ref('')
  const modelForm = ref(emptyModelForm())
  const modelEditingId = ref('')
  const discoveryProviderId = ref('')
  const discoveredModels = ref<DiscoveredModel[]>([])
  const discovering = ref(false)
  const discoverySaving = ref(false)
  const discoveryQuery = ref('')
  const selectedModelIds = ref<string[]>([])
  let discoveryRequest = 0
  const llmRows = computed(() => llmList.value || [])
  const providerRows = computed(() => providerList.value || [])
  const providerDialogTitleKey = computed(() => providerEditingId.value ? 'editProvider' : 'addProvider')
  const modelDialogTitleKey = computed(() => modelEditingId.value ? 'editModel' : 'addModel')
  const contextSizeDisplay = computed(() => formatContextSize(modelForm.value.contextSize))
  const contextSizeSlider = computed({
    get: () => contextSizeToSlider(modelForm.value.contextSize),
    set: (value: number | string) => { modelForm.value.contextSize = sliderToContextSize(value) },
  })
  const visibleDiscovered = computed(() => {
    const query = discoveryQuery.value.trim().toLowerCase()
    return discoveredModels.value.filter(item => !query || item.id.toLowerCase().includes(query) || item.name.toLowerCase().includes(query))
  })

  async function refresh() {
    const [providers, models] = await Promise.all([llmAPI.providers(), llmAPI.list()])
    providerList.value = providers
    llmList.value = models
  }

  function openProviderDialog() {
    providerForm.value = emptyProviderForm()
    providerEditingId.value = ''
    providerDialogVisible.value = true
  }

  function openProviderEditDialog(item: LLMProvider) {
    providerForm.value = { name: item.name, protocol: item.protocol, baseUrl: item.baseUrl, apiKey: '', clearApiKey: false }
    providerEditingId.value = item.id
    providerDialogVisible.value = true
  }

  async function saveProvider() {
    if (!providerForm.value.name.trim() || !providerForm.value.baseUrl.trim()) {
      toast.error(t('providerFormIncomplete'))
      return
    }
    providerSubmitting.value = true
    try {
      if (providerEditingId.value) await llmAPI.updateProvider(providerEditingId.value, providerForm.value)
      else await llmAPI.createProvider(providerForm.value)
      await refresh()
      onChanged?.()
      providerDialogVisible.value = false
    } catch (error) {
      toast.error(errorMessage(error, t('providerSaveFailed')))
    } finally {
      providerSubmitting.value = false
    }
  }

  async function deleteProvider(id: string) {
    await llmAPI.removeProvider(id)
    if (discoveryProviderId.value === id) discoveryProviderId.value = ''
    await refresh()
    onChanged?.()
  }

  function openModelDialog(providerId: string) {
    modelForm.value = emptyModelForm(providerId)
    modelEditingId.value = ''
    modelDialogVisible.value = true
  }

  function openModelEditDialog(item: LLMConfig) {
    modelForm.value = { providerId: item.providerId, name: item.name, model: item.model, contextSize: clampContextSize(item.contextSize) }
    modelEditingId.value = item.id
    modelDialogVisible.value = true
  }

  async function saveModel() {
    if (!modelForm.value.name.trim() || !modelForm.value.model.trim()) {
      toast.error(t('llmFormIncomplete'))
      return
    }
    llmSubmitting.value = true
    try {
      const payload = { ...modelForm.value, contextSize: clampContextSize(modelForm.value.contextSize) }
      if (modelEditingId.value) await llmAPI.update(modelEditingId.value, payload)
      else await llmAPI.create(payload)
      await refresh()
      onChanged?.()
      modelDialogVisible.value = false
    } catch (error) {
      toast.error(errorMessage(error, t('modelSaveFailed')))
    } finally {
      llmSubmitting.value = false
    }
  }

  async function deleteModel(id: string) {
    await llmAPI.remove(id)
    await refresh()
    onChanged?.()
  }

  async function discover(providerId: string) {
    const request = ++discoveryRequest
    discoveryProviderId.value = providerId
    discoveredModels.value = []
    discoveryQuery.value = ''
    selectedModelIds.value = []
    discovering.value = true
    try {
      const found = await llmAPI.discover(providerId)
      if (request === discoveryRequest) discoveredModels.value = found
    } catch (error) {
      if (request === discoveryRequest) toast.error(errorMessage(error, t('modelDiscoverFailed')))
    } finally {
      if (request === discoveryRequest) discovering.value = false
    }
  }

  function closeDiscovery() {
    discoveryRequest++
    discoveryProviderId.value = ''
    discovering.value = false
  }

  async function addDiscovered() {
    const existing = new Set(llmRows.value.filter(m => m.providerId === discoveryProviderId.value).map(m => m.model))
    const selected = discoveredModels.value.filter(m => selectedModelIds.value.includes(m.id) && !existing.has(m.id))
    if (selected.length === 0) return
    discoverySaving.value = true
    try {
      for (const item of selected) {
        await llmAPI.create({ providerId: discoveryProviderId.value, name: item.name, model: item.id, contextSize: clampContextSize(item.contextSize || CONTEXT_SIZE_DEFAULT) })
      }
      selectedModelIds.value = []
      await refresh()
      onChanged?.()
    } catch (error) {
      await refresh()
      toast.error(errorMessage(error, t('modelSaveFailed')))
    } finally {
      discoverySaving.value = false
    }
  }

  return {
    llmRows, providerRows, providerForm, providerEditingId, modelForm, providerDialogTitleKey, modelDialogTitleKey,
    contextSizeDisplay, contextSizeSlider, discoveryProviderId, discoveredModels, visibleDiscovered,
    discovering, discoverySaving, discoveryQuery, selectedModelIds,
    openProviderDialog, openProviderEditDialog, saveProvider, deleteProvider,
    openModelDialog, openModelEditDialog, saveModel, deleteModel,
    discover, closeDiscovery, addDiscovered,
  }
}
