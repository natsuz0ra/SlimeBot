import { computed, ref, type Ref } from 'vue'
import { llmAPI } from '@/api/llm'
import type { LLMConfig } from '@/types/settings'
import {
  CONTEXT_SIZE_DEFAULT,
  clampContextSize,
  contextSizeToSlider,
  formatContextSize,
  sliderToContextSize,
} from '@/utils/contextSize'

type ToastLike = {
  error(message: string): void
}

type Translate = (key: string) => string

type LLMProvider = 'openai' | 'anthropic' | 'deepseek'

function emptyLLMForm() {
  return { name: '', provider: 'openai' as LLMProvider, baseUrl: '', apiKey: '', model: '', contextSize: CONTEXT_SIZE_DEFAULT }
}

export function useSettingsLLM(options: {
  llmList: Ref<LLMConfig[]>
  llmDialogVisible: Ref<boolean>
  llmSubmitting: Ref<boolean>
  toast: ToastLike
  t: Translate
  onChanged?: () => void
}) {
  const { llmList, llmDialogVisible, llmSubmitting, toast, t, onChanged } = options
  const llmForm = ref(emptyLLMForm())
  const llmEditingId = ref('')
  const llmRows = computed(() => llmList.value || [])
  const llmDialogTitleKey = computed(() => (llmEditingId.value ? 'editModel' : 'addModel'))
  const llmContextSizeDisplay = computed(() => formatContextSize(llmForm.value.contextSize))
  const llmContextSizeSlider = computed({
    get: () => contextSizeToSlider(llmForm.value.contextSize),
    set: (value: number | string) => {
      llmForm.value.contextSize = sliderToContextSize(value)
    },
  })

  async function refreshLLM() {
    llmList.value = await llmAPI.list()
  }

  function openLLMDialog() {
    llmForm.value = emptyLLMForm()
    llmEditingId.value = ''
    llmDialogVisible.value = true
  }

  function openLLMEditDialog(item: LLMConfig) {
    llmForm.value = {
      name: item.name || '',
      provider: (item.provider || 'openai') as LLMProvider,
      baseUrl: item.baseUrl || '',
      apiKey: item.apiKey || '',
      model: item.model || '',
      contextSize: clampContextSize(item.contextSize),
    }
    llmEditingId.value = item.id
    llmDialogVisible.value = true
  }

  async function saveLLM() {
    if (!llmForm.value.name || !llmForm.value.baseUrl || !llmForm.value.apiKey || !llmForm.value.model) {
      toast.error(t('llmFormIncomplete'))
      return
    }
    llmSubmitting.value = true
    try {
      const payload = {
        ...llmForm.value,
        contextSize: clampContextSize(llmForm.value.contextSize),
      }
      if (llmEditingId.value) {
        await llmAPI.update(llmEditingId.value, payload)
      } else {
        await llmAPI.create(payload)
      }
      llmForm.value = emptyLLMForm()
      llmEditingId.value = ''
      await refreshLLM()
      onChanged?.()
      llmDialogVisible.value = false
    } finally {
      llmSubmitting.value = false
    }
  }

  async function deleteLLM(id: string) {
    await llmAPI.remove(id)
    await refreshLLM()
    onChanged?.()
  }

  return {
    llmForm,
    llmEditingId,
    llmRows,
    llmDialogTitleKey,
    llmContextSizeDisplay,
    llmContextSizeSlider,
    openLLMDialog,
    openLLMEditDialog,
    saveLLM,
    deleteLLM,
  }
}
