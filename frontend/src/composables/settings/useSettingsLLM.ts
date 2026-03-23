import { computed, ref, type Ref } from 'vue'
import { llmAPI } from '@/api/llm'

type ToastLike = {
  error(message: string): void
}

type Translate = (key: string) => string

type LLMItem = any

export function useSettingsLLM(options: {
  llmList: Ref<LLMItem[]>
  llmDialogVisible: Ref<boolean>
  llmSubmitting: Ref<boolean>
  toast: ToastLike
  t: Translate
  onChanged?: () => void
}) {
  const { llmList, llmDialogVisible, llmSubmitting, toast, t, onChanged } = options
  const llmForm = ref({ name: '', baseUrl: '', apiKey: '', model: '' })
  const llmRows = computed(() => llmList.value || [])

  async function refreshLLM() {
    llmList.value = await llmAPI.list()
  }

  function openLLMDialog() {
    llmForm.value = { name: '', baseUrl: '', apiKey: '', model: '' }
    llmDialogVisible.value = true
  }

  async function addLLM() {
    if (!llmForm.value.name || !llmForm.value.baseUrl || !llmForm.value.apiKey || !llmForm.value.model) {
      toast.error(t('llmFormIncomplete'))
      return
    }
    llmSubmitting.value = true
    try {
      await llmAPI.create(llmForm.value)
      llmForm.value = { name: '', baseUrl: '', apiKey: '', model: '' }
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
    llmRows,
    openLLMDialog,
    addLLM,
    deleteLLM,
  }
}
