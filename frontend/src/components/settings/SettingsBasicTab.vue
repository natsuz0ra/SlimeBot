<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import LanguageSwitcher from '@/components/ui/LanguageSwitcher.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import ToggleSwitch from '@/components/ui/ToggleSwitch.vue'
import AppTextInput from '@/components/ui/AppTextInput.vue'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import { mdiLanConnect } from '@mdi/js'
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
  proxyUrl: string
  proxyDirty: boolean
  proxyEnabled: boolean
  savingProxy: boolean
}>()

const emit = defineEmits<{
  openAccount: []
  openWebSearch: []
  logout: []
  languageChange: [value: LanguageCode]
  sandboxModeChange: [value: SandboxMode]
  sandboxNetworkChange: [value: boolean]
  proxyUrlChange: [value: string]
  saveProxy: []
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
    <div class="settings-card px-4 py-4 rounded-xl mt-2">
      <div class="flex items-start justify-between gap-3">
        <div class="flex items-start gap-3 min-w-0">
          <span class="proxy-icon flex-shrink-0 flex items-center justify-center rounded-lg"><MdiIcon :path="mdiLanConnect" :size="18" /></span>
          <div>
            <div class="settings-field-label">{{ t('proxySetting') }}</div>
            <p class="proxy-help settings-item-sub mt-0.5">{{ t('proxyDescription') }}</p>
          </div>
        </div>
        <span class="proxy-status flex-shrink-0" :class="proxyEnabled ? 'proxy-status-on' : ''">{{ t(proxyEnabled ? 'proxyEnabled' : 'proxyDisabled') }}</span>
      </div>
      <div class="mt-4">
        <label for="settings-proxy-url" class="proxy-help settings-dialog-label block mb-1.5">{{ t('proxyAddress') }}</label>
        <div class="flex flex-wrap sm:flex-nowrap gap-2">
          <AppTextInput
            id="settings-proxy-url"
            :model-value="proxyUrl"
            placeholder="http://127.0.0.1:7890"
            autocomplete="off"
            spellcheck="false"
            class="min-w-0 flex-1"
            @update:model-value="emit('proxyUrlChange', $event)"
            @keydown.enter="proxyDirty && !savingProxy && emit('saveProxy')"
          />
          <button type="button" class="proxy-save min-w-[72px] flex-shrink-0 px-4 rounded-xl cursor-pointer transition-colors duration-200" :disabled="!proxyDirty || savingProxy" @click="emit('saveProxy')">
            {{ t(savingProxy ? 'saving' : 'save') }}
          </button>
        </div>
        <p class="proxy-help settings-item-sub mt-2">{{ t('proxyHint') }}</p>
      </div>
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

<style scoped>
.proxy-icon { width: 34px; height: 34px; color: var(--sb-brand); background: var(--primary-alpha-12); }
.proxy-status { padding: 4px 9px; border-radius: 999px; color: var(--text-secondary); background: var(--primary-alpha-07); font-size: 11px; font-weight: 600; }
.proxy-status-on { color: var(--sb-brand); background: var(--primary-alpha-12); }
.proxy-help { color: var(--text-secondary); }
.proxy-save { min-height: 40px; color: white; background: var(--sb-brand); font-size: 13px; font-weight: 600; }
.proxy-save:hover:not(:disabled) { filter: brightness(1.08); }
.proxy-save:focus-visible { outline: 2px solid var(--sb-brand); outline-offset: 2px; }
.proxy-save:disabled { cursor: not-allowed; opacity: .45; }
</style>
