<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import LanguageSwitcher from '@/components/ui/LanguageSwitcher.vue'
import type { LanguageCode } from '@/composables/useLanguagePreference'

const props = defineProps<{
  language: LanguageCode
  languageSelectOptions: { value: LanguageCode; label: string }[]
  savingLanguage: boolean
}>()

const emit = defineEmits<{
  openAccount: []
  openWebSearch: []
  logout: []
  languageChange: [value: LanguageCode]
}>()

const { t } = useI18n()
</script>

<template>
  <div>
    <p class="section-label">{{ t('basicSettings') }}</p>
    <div class="settings-card flex items-center justify-between px-4 py-3.5 rounded-xl mb-2">
      <span class="text-sm settings-field-label">{{ t('accountEdit') }}</span>
      <button type="button" class="px-3 py-1.5 text-xs rounded-lg cursor-pointer account-edit-btn" @click="emit('openAccount')">
        {{ t('accountEditAction') }}
      </button>
    </div>
    <div class="settings-card flex items-center justify-between px-4 py-3.5 rounded-xl">
      <span class="text-sm settings-field-label">{{ t('language') }}</span>
      <LanguageSwitcher
        :model-value="language"
        :options="languageSelectOptions"
        :disabled="savingLanguage"
        shadow-mode="none"
        :aria-label="t('selectLanguage')"
        @update:model-value="emit('languageChange', $event as LanguageCode)"
      />
    </div>
    <div class="settings-card px-4 py-3.5 rounded-xl mt-2">
      <div class="flex items-center justify-between gap-3">
        <span class="text-sm settings-field-label">{{ t('webSearchSetting') }}</span>
        <button
          type="button"
          class="px-3 py-1.5 text-xs rounded-lg cursor-pointer account-edit-btn"
          @click="emit('openWebSearch')"
        >
          {{ t('accountEditAction') }}
        </button>
      </div>
    </div>
    <button type="button" class="settings-card w-full mt-2 px-4 py-3.5 rounded-xl text-left text-sm cursor-pointer logout-btn" @click="emit('logout')">
      {{ t('logout') }}
    </button>
  </div>
</template>
