import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import {
  clearAuthStorage,
  getAuthToken,
  getMustChangePassword,
  setAuthToken,
  setMustChangePassword,
} from '@/utils/authStorage'

export const useAuthStore = defineStore('auth', () => {
  const token = ref('')
  const mustChangePassword = ref(false)
  const initialized = ref(false)

  const isAuthenticated = computed(() => !!token.value)

  function hydrate() {
    token.value = getAuthToken()
    mustChangePassword.value = getMustChangePassword()
    initialized.value = true
  }

  function setAuth(nextToken: string, nextMustChangePassword: boolean) {
    token.value = nextToken
    mustChangePassword.value = nextMustChangePassword
    setAuthToken(nextToken)
    setMustChangePassword(nextMustChangePassword)
    initialized.value = true
  }

  function markPasswordChanged() {
    mustChangePassword.value = false
    setMustChangePassword(false)
  }

  function clearAuth() {
    token.value = ''
    mustChangePassword.value = false
    clearAuthStorage()
    initialized.value = true
  }

  return {
    token,
    mustChangePassword,
    initialized,
    isAuthenticated,
    hydrate,
    setAuth,
    markPasswordChanged,
    clearAuth,
  }
})
