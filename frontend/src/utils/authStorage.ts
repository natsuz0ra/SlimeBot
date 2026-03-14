const AUTH_TOKEN_KEY = 'slimebot:authToken'
const FORCE_PASSWORD_CHANGE_KEY = 'slimebot:mustChangePassword'

export function getAuthToken() {
  return localStorage.getItem(AUTH_TOKEN_KEY) || ''
}

export function setAuthToken(token: string) {
  if (!token) {
    localStorage.removeItem(AUTH_TOKEN_KEY)
    return
  }
  localStorage.setItem(AUTH_TOKEN_KEY, token)
}

export function getMustChangePassword() {
  return localStorage.getItem(FORCE_PASSWORD_CHANGE_KEY) === 'true'
}

export function setMustChangePassword(value: boolean) {
  localStorage.setItem(FORCE_PASSWORD_CHANGE_KEY, value ? 'true' : 'false')
}

export function clearAuthStorage() {
  localStorage.removeItem(AUTH_TOKEN_KEY)
  localStorage.removeItem(FORCE_PASSWORD_CHANGE_KEY)
}
