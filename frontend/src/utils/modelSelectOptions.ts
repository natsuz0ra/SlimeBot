import type { SelectOption } from '@/components/ui/AppSelect.vue'
import type { LLMConfig } from '@/types/settings'

export function groupedModelOptions(models: LLMConfig[]): SelectOption[] {
  return [...models]
    .sort((a, b) => (a.providerName || a.provider).localeCompare(b.providerName || b.provider) || a.name.localeCompare(b.name))
    .map(model => ({ value: model.id, label: model.name, group: model.providerName || model.provider }))
}
