import { apiClient } from './client'
import type { SkillItem } from '@/types/settings'

export const skillsAPI = {
  list: async () => (await apiClient.get<SkillItem[]>('/api/skills')).data,
  upload: async (files: File[]) => {
    const formData = new FormData()
    files.forEach((file) => formData.append('files', file))
    return (await apiClient.post('/api/skills/upload', formData, { headers: { 'Content-Type': 'multipart/form-data' } })).data
  },
  remove: async (id: string) => apiClient.delete(`/api/skills/${id}`),
}
