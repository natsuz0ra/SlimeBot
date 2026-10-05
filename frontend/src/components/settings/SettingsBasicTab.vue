<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import LanguageSwitcher from '@/components/ui/LanguageSwitcher.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import ToggleSwitch from '@/components/ui/ToggleSwitch.vue'
import AppTextInput from '@/components/ui/AppTextInput.vue'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import { mdiAlertCircleOutline, mdiCheckCircleOutline, mdiLanConnect } from '@mdi/js'
import type { SelectOption } from '@/components/ui/AppSelect.vue'
import type { LanguageCode } from '@/composables/useLanguagePreference'
import type { ProxyTestResult, SandboxMode } from '@/types/settings'

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
  testingProxy: boolean
  proxyTestResult: ProxyTestResult | null
  proxyTestError: string
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
  testProxy: []
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
            aria-describedby="settings-proxy-hint settings-proxy-test-hint"
            class="min-w-0 flex-1"
            @update:model-value="emit('proxyUrlChange', $event)"
            @keydown.enter="proxyDirty && !savingProxy && emit('saveProxy')"
          />
          <button type="button" class="proxy-save min-w-[72px] flex-shrink-0 px-4 rounded-xl cursor-pointer transition-colors duration-200" :disabled="!proxyDirty || savingProxy" @click="emit('saveProxy')">
            {{ t(savingProxy ? 'saving' : 'save') }}
          </button>
        </div>
        <p id="settings-proxy-hint" class="proxy-help settings-item-sub mt-2">{{ t('proxyHint') }}</p>
        <div class="flex items-center justify-between gap-3 mt-3">
          <p id="settings-proxy-test-hint" class="proxy-help settings-item-sub min-w-0">{{ t('proxyTestHint') }}</p>
          <button
            type="button"
            class="proxy-test flex-shrink-0 flex items-center justify-center gap-2 px-3 py-2 rounded-lg cursor-pointer account-edit-btn settings-action-text"
            :disabled="testingProxy"
            :aria-busy="testingProxy"
            @click="emit('testProxy')"
          >
            <LoadingSpinner v-if="testingProxy" size-class="w-3.5 h-3.5" aria-hidden="true" />
            {{ t(testingProxy ? 'proxyTesting' : 'proxyTestConnection') }}
          </button>
        </div>
        <div role="status" aria-live="polite" aria-atomic="true">
          <div v-if="proxyTestResult || proxyTestError" class="proxy-result rounded-lg px-3 py-3 mt-3" :class="{ 'proxy-result-error': proxyTestError || !proxyTestResult?.success }">
            <div class="flex items-start gap-2.5">
              <MdiIcon :path="proxyTestResult?.success ? mdiCheckCircleOutline : mdiAlertCircleOutline" :size="18" class="proxy-result-icon flex-shrink-0 mt-0.5" aria-hidden="true" />
              <div class="min-w-0 flex-1">
                <div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
                  <span class="settings-field-label">{{ t(proxyTestResult?.success ? 'proxyTestSuccess' : 'proxyTestFailure') }}</span>
                  <span v-if="proxyTestResult" class="proxy-help settings-item-sub tabular-nums">{{ proxyTestResult.latencyMs }} ms<span v-if="proxyTestResult.statusCode"> · HTTP {{ proxyTestResult.statusCode }}</span></span>
                </div>
                <p v-if="proxyTestError" class="proxy-help settings-item-sub mt-1">{{ t(proxyTestError) }}</p>
                <template v-else-if="proxyTestResult">
                  <p class="proxy-help settings-item-sub mt-1">{{ t(`proxyTestRoute_${proxyTestResult.route}`) }}</p>
                  <p v-if="proxyTestResult.errorCode" class="proxy-help settings-item-sub mt-1">{{ t(`proxyTestError_${proxyTestResult.errorCode}`) }}</p>
                  <p class="proxy-help settings-item-sub mt-1 break-all">{{ t('proxyTestTarget', { target: proxyTestResult.targetUrl }) }}</p>
                </template>
              </div>
            </div>
          </div>
        </div>
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
.proxy-test { min-height: 40px; transition: background-color 0.2s ease; }
.proxy-test:focus-visible { outline: 2px solid var(--sb-brand); outline-offset: 2px; }
.proxy-test:disabled { cursor: wait; opacity: .65; }
.proxy-result { border: 1px solid var(--primary-alpha-20); background: var(--primary-alpha-05); }
.proxy-result-icon { color: var(--sb-brand); }
.proxy-result-error { border-color: var(--danger-alpha-20); background: var(--danger-alpha-04); }
.proxy-result-error .proxy-result-icon { color: var(--color-danger); }
</style>
