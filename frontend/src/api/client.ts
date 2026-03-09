import axios from 'axios'

const baseURL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080'

export const apiClient = axios.create({
  baseURL,
  timeout: 15000,
})
