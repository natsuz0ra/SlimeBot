<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { mdiWeatherNight, mdiWeatherSunny } from '@mdi/js'
import MdiIcon from '@/components/MdiIcon.vue'
import SlimeBotLogo from '@/components/ui/SlimeBotLogo.vue'
import AppTextInput from '@/components/ui/AppTextInput.vue'
import AppPasswordInput from '@/components/ui/AppPasswordInput.vue'
import LanguageSwitcher from '@/components/ui/LanguageSwitcher.vue'
import LoginAuroraBackgroundCanvas from '@/components/login/LoginAuroraBackgroundCanvas.vue'
import { useI18n } from 'vue-i18n'
import { useToast } from '@/composables/useToast'
import { authAPI } from '@/api/auth'
import { useLanguagePreference } from '@/composables/useLanguagePreference'
import { useTheme } from '@/composables/useTheme'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const toast = useToast()
const router = useRouter()
const authStore = useAuthStore()
const { isDark, toggleTheme } = useTheme()
const LOGIN_HOME_TRANSITION_TOKEN = 'slimebot:transition:login-home'

const username = ref('')
const password = ref('')
const submitting = ref(false)
const {
  language,
  languageOptions,
  savingLanguage,
  loadLanguage,
  changeLanguage,
  syncLanguageToServer,
} = useLanguagePreference()

const canSubmit = computed(() => !!username.value.trim() && !!password.value.trim())
const languageSelectOptions = computed(() =>
  languageOptions.map((option) => ({
    value: option.value,
    label: t(option.labelKey),
  })),
)

async function onLanguageChange(nextLanguage: (typeof languageOptions)[number]['value']) {
  await changeLanguage(nextLanguage, { allowRemote: false })
}

async function login() {
  if (!canSubmit.value || submitting.value) return
  submitting.value = true
  try {
    const response = await authAPI.login({
      username: username.value.trim(),
      password: password.value,
    })
    authStore.setAuth(response.token, !!response.mustChangePassword)
    await syncLanguageToServer()
    try {
      sessionStorage.setItem(LOGIN_HOME_TRANSITION_TOKEN, '1')
    } catch {
      // Ignore storage access errors and continue navigating.
    }
    await router.replace('/')
  } catch (error: any) {
    toast.error(error?.response?.data?.error || t('loginFailed'))
  } finally {
    submitting.value = false
  }
}

onMounted(async () => {
  await loadLanguage({ allowRemote: false })
  if (!authStore.initialized) {
    authStore.hydrate()
  }
  if (authStore.isAuthenticated) {
    await router.replace('/')
  }
})
</script>

<template>
  <div class="login-page min-h-screen px-4 py-10 sm:py-14">
    <div class="login-bg-layer" aria-hidden="true">
      <LoginAuroraBackgroundCanvas />
      <div class="login-bg-overlay" />
    </div>

    <div class="login-top-actions">
      <div class="login-language-switcher">
        <LanguageSwitcher
          :model-value="language"
          :options="languageSelectOptions"
          :aria-label="t('selectLanguage')"
          :disabled="savingLanguage"
          @update:model-value="onLanguageChange($event as any)"
        />
      </div>
      <div class="login-theme-switcher">
        <button
          type="button"
          class="login-theme-switcher-trigger inline-flex items-center justify-center rounded-xl px-3 py-2 cursor-pointer"
          :aria-label="t('toggleTheme')"
          @click="toggleTheme"
        >
          <MdiIcon :path="isDark ? mdiWeatherSunny : mdiWeatherNight" :size="18" />
        </button>
      </div>
    </div>

    <div class="login-content-layer">
      <div class="login-card w-full max-w-[420px] mx-auto rounded-2xl p-5 sm:p-7">
        <div class="logo-row login-brand-row login-fade-in-up flex items-center justify-center gap-3">
          <SlimeBotLogo :size="88" animated class="login-brand-logo" />
          <span class="brand-tech-font logo-text">SlimeBot</span>
        </div>

        <div class="mt-6 flex flex-col gap-3.5">
          <div class="input-wrap input-animate input-animate-1">
            <AppTextInput
              v-model="username"
              :placeholder="t('username')"
              autocomplete="username"
            />
          </div>
          <div class="input-wrap password-wrap input-animate input-animate-2">
            <AppPasswordInput
              v-model="password"
              :placeholder="t('password')"
              autocomplete="current-password"
              :show-label="t('showPassword')"
              :hide-label="t('hidePassword')"
              @keydown.enter="login"
            />
          </div>
          <button
            type="button"
            class="login-submit input-animate input-animate-3 w-full rounded-xl py-2.5 text-sm font-semibold cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
            :disabled="!canSubmit || submitting"
            @click="login"
          >
            <span v-if="!submitting">{{ t('login') }}</span>
            <span v-else>{{ t('loading') }}...</span>
          </button>
          <p class="login-tip input-animate input-animate-4 text-xs">
            {{ t('loginDefaultTip') }}
          </p>
        </div>
      </div>
    </div>

  </div>
</template>

<style scoped>
@import './login-page.css';
</style>
