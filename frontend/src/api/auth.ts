import { apiClient } from './client'

export interface LoginPayload {
  username: string
  password: string
}

export interface LoginResponse {
  token: string
  tokenType: 'Bearer'
  expiresInMinutes: number
  mustChangePassword: boolean
}

export interface UpdateAccountPayload {
  username?: string
  oldPassword?: string
  newPassword?: string
}

export const authAPI = {
  login: async (payload: LoginPayload) => (await apiClient.post<LoginResponse>('/api/login', payload)).data,
  updateAccount: async (payload: UpdateAccountPayload) => apiClient.put('/api/account', payload),
}
