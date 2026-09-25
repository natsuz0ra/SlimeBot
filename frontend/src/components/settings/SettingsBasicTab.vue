<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import LanguageSwitcher from '@/components/ui/LanguageSwitcher.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import ToggleSwitch from '@/components/ui/ToggleSwitch.vue'
import type { SelectOption } from '@/components/ui/AppSelect.vue'
import type { LanguageCode } from '@/composables/useLanguagePreference'
import type { SandboxMode } from '@/types/settings'

const props = defineProps<{
  language: LanguageCode
  languageSelectOptions: { value: LanguageCode; label: string }[]
  savingLanguage: boolean
  sandboxMode: SandboxMode
  sandboxModeOptions: SelectOption[]
  sandboxNetworkEnabled: boolean
}>()

const emit = defineEmits<{
  openAccount: []
  openWebSearch: []
  logout: []
  languageChange: [value: LanguageCode]
  sandboxModeChange: [value: SandboxMode]
  sandboxNetworkChange: [value: boolean]
}>()

const { t } = useI18n()
const isDesktop = Boolean(window.slimebotDesktop)
</script>

<template>
  <div>
    <p class="section-label">{{ t('basicSettings') }}</p>
    <div v-if="!isDesktop" class="settings-card flex items-center justify-between px-4 py-3.5 rounded-xl mb-2">
      <span class="settings-field-label">{{ t('accountEdit') }}</span>
      <button type="button" class="px-3 py-1.5 rounded-lg cursor-pointer account-edit-btn settings-action-text" @click="emit('openAccount')">
        {{ t('accountEditAction') }}
      </button>
    </div>
    <div class="settings-card flex items-center justify-between px-4 py-3.5 rounded-xl">
      <span class="settings-field-label">{{ t('language') }}</span>
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
        <span class="settings-field-label">{{ t('sandboxMode') }}</span>
        <AppSelect
          :model-value="sandboxMode"
          :options="sandboxModeOptions"
          @update:model-value="emit('sandboxModeChange', $event as SandboxMode)"
        />
      </div>
      <div class="mt-3 flex items-center justify-between gap-3 settings-field-label">
        <span>{{ t('sandboxNetwork') }}</span>
        <ToggleSwitch :model-value="sandboxNetworkEnabled" @update:model-value="emit('sandboxNetworkChange', $event)" />
      </div>
    </div>
    <div class="settings-card px-4 py-3.5 rounded-xl mt-2">
      <div class="flex items-center justify-between gap-3">
        <span class="settings-field-label">{{ t('webSearchSetting') }}</span>
        <button
          type="button"
          class="px-3 py-1.5 rounded-lg cursor-pointer account-edit-btn settings-action-text"
          @click="emit('openWebSearch')"
        >
          {{ t('accountEditAction') }}
        </button>
      </div>
    </div>
    <button v-if="!isDesktop" type="button" class="settings-card w-full mt-2 px-4 py-3.5 rounded-xl text-left cursor-pointer logout-btn settings-field-label" @click="emit('logout')">
      {{ t('logout') }}
    </button>
  </div>
</template>
