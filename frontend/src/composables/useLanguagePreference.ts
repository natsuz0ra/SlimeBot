import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { settingAPI } from '@/api/settings'
import { useAuthStore } from '@/stores/auth'
import { useToast } from '@/composables/useToast'

export type LanguageCode = 'zh-CN' | 'en-US'

interface LoadLanguageOptions {
  allowRemote?: boolean
}

interface ChangeLanguageOptions {
  allowRemote?: boolean
  showSuccessToast?: boolean
}

interface SyncLanguageOptions {
  showSuccessToast?: boolean
  silentOnError?: boolean
}

const LANGUAGE_STORAGE_KEY = 'slimebot.language'
const FALLBACK_LANGUAGE: LanguageCode = 'zh-CN'
const LANGUAGE_VALUES: LanguageCode[] = ['zh-CN', 'en-US']

function normalizeLanguage(value: string | null | undefined): LanguageCode | null {
  return LANGUAGE_VALUES.includes(value as LanguageCode) ? (value as LanguageCode) : null
}

function readLocalLanguage(): LanguageCode | null {
  return normalizeLanguage(window.localStorage.getItem(LANGUAGE_STORAGE_KEY))
}

function writeLocalLanguage(language: LanguageCode) {
  window.localStorage.setItem(LANGUAGE_STORAGE_KEY, language)
}

export function useLanguagePreference() {
  const { t, locale } = useI18n()
  const toast = useToast()
  const authStore = useAuthStore()

  const language = ref<LanguageCode>(normalizeLanguage(locale.value as string) || FALLBACK_LANGUAGE)
  const savingLanguage = ref(false)

  const languageOptions: Array<{ value: LanguageCode; labelKey: 'chinese' | 'english' }> = [
    { value: 'zh-CN', labelKey: 'chinese' },
    { value: 'en-US', labelKey: 'english' },
  ]

  const currentLanguageLabel = computed(() => t(language.value === 'zh-CN' ? 'chinese' : 'english'))

  function ensureAuthHydrated() {
    if (!authStore.initialized) authStore.hydrate()
  }

  function applyLanguage(nextLanguage: LanguageCode) {
    language.value = nextLanguage
    locale.value = nextLanguage
    writeLocalLanguage(nextLanguage)
  }

  function canUseRemote(allowRemote: boolean) {
    if (!allowRemote) return false
    ensureAuthHydrated()
    return !!authStore.isAuthenticated
  }

  async function loadLanguage(options?: LoadLanguageOptions) {
    const allowRemote = options?.allowRemote ?? true
    const localLanguage = readLocalLanguage()
    if (localLanguage) {
      applyLanguage(localLanguage)
    } else {
      applyLanguage(normalizeLanguage(locale.value as string) || FALLBACK_LANGUAGE)
    }

    if (!canUseRemote(allowRemote)) return language.value

    try {
      const settings = await settingAPI.get()
      const remoteLanguage = normalizeLanguage(settings.language) || FALLBACK_LANGUAGE
      applyLanguage(remoteLanguage)
      return remoteLanguage
    } catch {
      return language.value
    }
  }

  async function changeLanguage(nextLanguage: LanguageCode, options?: ChangeLanguageOptions) {
    if (savingLanguage.value) return false
    if (nextLanguage === language.value) return true

    const previousLanguage = language.value
    const allowRemote = options?.allowRemote ?? true
    const showSuccessToast = options?.showSuccessToast ?? false

    applyLanguage(nextLanguage)

    if (!canUseRemote(allowRemote)) return true

    savingLanguage.value = true
    try {
      await settingAPI.update({ language: nextLanguage })
      if (showSuccessToast) toast.success(t('saveSuccess'))
      return true
    } catch {
      applyLanguage(previousLanguage)
      toast.error(t('languageSaveFailed'))
      return false
    } finally {
      savingLanguage.value = false
    }
  }

  async function syncLanguageToServer(options?: SyncLanguageOptions) {
    if (savingLanguage.value) return false
    const showSuccessToast = options?.showSuccessToast ?? false
    const silentOnError = options?.silentOnError ?? true
    if (!canUseRemote(true)) return false

    savingLanguage.value = true
    try {
      await settingAPI.update({ language: language.value })
      if (showSuccessToast) toast.success(t('saveSuccess'))
      return true
    } catch {
      if (!silentOnError) toast.error(t('languageSaveFailed'))
      return false
    } finally {
      savingLanguage.value = false
    }
  }

  return {
    language,
    languageOptions,
    currentLanguageLabel,
    savingLanguage,
    loadLanguage,
    changeLanguage,
    syncLanguageToServer,
  }
}
