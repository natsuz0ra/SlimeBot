import { apiClient } from './client'
import type { PromptTemplate } from '@/utils/slashCommands'

export const promptTemplatesAPI = {
  list: async () => (await apiClient.get<PromptTemplate[]>('/api/prompt-templates')).data,
  save: async (templates: PromptTemplate[]) => (await apiClient.put<PromptTemplate[]>('/api/prompt-templates', templates)).data,
}
