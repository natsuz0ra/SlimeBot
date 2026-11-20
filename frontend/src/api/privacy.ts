import { apiClient } from './client'
import type { PrivacyConfig, PrivacyModelStatus, PrivacyPreview, PrivacySnapshot } from '@/types/privacy'

export const privacyAPI = {
  get: async () => (await apiClient.get<PrivacySnapshot>('/api/privacy')).data,
  update: async (config: PrivacyConfig) => apiClient.put('/api/privacy', config),
  preview: async (text: string, config?: PrivacyConfig) => (await apiClient.post<PrivacyPreview>('/api/privacy/preview', { text, config }, { timeout: 130000 })).data,
  clearRecords: async () => apiClient.delete('/api/privacy/records'),
  installModel: async () => (await apiClient.post<PrivacyModelStatus>('/api/privacy/model/install')).data,
  cancelModel: async () => (await apiClient.post<PrivacyModelStatus>('/api/privacy/model/cancel')).data,
  removeModel: async () => apiClient.delete('/api/privacy/model'),
}
