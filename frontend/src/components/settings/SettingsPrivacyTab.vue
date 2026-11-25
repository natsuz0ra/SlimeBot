<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, useId } from 'vue'
import { useI18n } from 'vue-i18n'
import { privacyAPI } from '@/api/privacy'
import { llmAPI } from '@/api/llm'
import AppSelect from '@/components/ui/AppSelect.vue'
import ToggleSwitch from '@/components/ui/ToggleSwitch.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import type { PrivacyConfig, PrivacyMappingStatus, PrivacyModelStatus, PrivacyPreview, PrivacyRecord } from '@/types/privacy'
import type { LLMProvider } from '@/types/settings'
import { buildPrivacyConfig, privacyCoreKinds, privacyExtraKinds, privacyKindEnabled, privacyTotals } from '@/utils/privacy'

const { t } = useI18n()
const config = ref<PrivacyConfig | null>(null)
const model = ref<PrivacyModelStatus | null>(null)
const mappings = ref<PrivacyMappingStatus | null>(null)
const mappingTTL = ref<number | string>('')
const mappingTTLHintId = useId()
const maxMappingTTL = 43_200
const records = ref<PrivacyRecord[]>([])
const providers = ref<LLMProvider[]>([])
const words = ref('')
const allow = ref('')
const saved = ref('')
const loading = ref(true)
const saving = ref(false)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const previewInput = ref('')
const preview = ref<PrivacyPreview | null>(null)
const testing = ref(false)
let timer: ReturnType<typeof setInterval> | undefined
let disposed = false
const mappingTTLValid = computed(() => typeof mappingTTL.value === 'number' && Number.isInteger(mappingTTL.value) && mappingTTL.value >= 1 && mappingTTL.value <= maxMappingTTL)
const mappingModeLabel = computed(() => t(mappings.value?.mode === 'encrypted'
  ? 'privacyMappingEncrypted'
  : mappings.value?.mode === 'unavailable' ? 'privacyMappingUnavailable' : 'privacyMappingMemory'))
const draft = computed(() => config.value ? buildPrivacyConfig({
  ...config.value,
  mappingTtlMinutes: mappingTTLValid.value ? Number(mappingTTL.value) : config.value.mappingTtlMinutes,
}, words.value, allow.value) : null)
const dirty = computed(() => draft.value && (!mappingTTLValid.value || JSON.stringify(draft.value) !== saved.value))
const totals = computed(() => privacyTotals(records.value))
const actionOptions = computed(() => [
  { value: 'mask', label: t('privacyMask') },
  { value: 'warn', label: t('privacyWarn') },
  { value: 'block', label: t('privacyBlock') },
])
const progress = computed(() => model.value?.totalBytes ? Math.min(100, Math.round(model.value.downloadedBytes * 100 / model.value.totalBytes)) : 0)

function message(err: unknown) {
  const candidate = err as { response?: { data?: { error?: string } }; message?: string }
  return candidate.response?.data?.error || candidate.message || t('privacyFailed')
}

async function load(initial = false) {
  try {
    const snapshot = await privacyAPI.get()
    if (disposed) return
    model.value = snapshot.model
    mappings.value = snapshot.mappings
    records.value = snapshot.records || []
    if (initial) {
      config.value = snapshot.config
      mappingTTL.value = snapshot.config.mappingTtlMinutes
      words.value = snapshot.config.words?.join('\n') || ''
      allow.value = snapshot.config.allow?.join('\n') || ''
      saved.value = JSON.stringify(draft.value)
      error.value = ''
    }
  } catch (err) {
    if (!disposed) error.value = message(err)
  } finally {
    if (!disposed && initial) loading.value = false
  }
}

async function save() {
  if (!draft.value || !mappingTTLValid.value) return
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    const payload = draft.value
    await privacyAPI.update(payload)
    saved.value = JSON.stringify(payload)
    notice.value = t('privacySaved')
    await load()
  } catch (err) { error.value = message(err) }
  finally { saving.value = false }
}

async function tryPreview() {
  if (!draft.value || !previewInput.value.trim()) return
  testing.value = true
  error.value = ''
  preview.value = null
  try { preview.value = await privacyAPI.preview(previewInput.value, draft.value) }
  catch (err) { error.value = message(err) }
  finally { testing.value = false }
}

async function modelAction(action: 'install' | 'cancel' | 'remove') {
  busy.value = true
  error.value = ''
  try {
    if (action === 'install') model.value = await privacyAPI.installModel()
    if (action === 'cancel') model.value = await privacyAPI.cancelModel()
    if (action === 'remove') {
      await privacyAPI.removeModel()
      if (config.value) config.value.modelEnabled = false
      // Removing the model also disables it in saved settings.
      if (saved.value) { const previous = JSON.parse(saved.value) as PrivacyConfig; previous.modelEnabled = false; saved.value = JSON.stringify(previous) }
    }
    await load()
  } catch (err) { error.value = message(err) }
  finally { busy.value = false }
}

async function clearRecords() {
  try { await privacyAPI.clearRecords(); records.value = [] }
  catch (err) { error.value = message(err) }
}

function toggleKind(kind: string, enabled: boolean) { if (config.value) config.value.kinds[kind] = enabled }
function setAction(kind: string, action: string) {
  if (config.value && (action === 'mask' || action === 'warn' || action === 'block')) {
    config.value.actions[kind] = action
  }
}
function toggleProvider(id: string, enabled: boolean) {
  if (!config.value) return
  config.value.bypassProviderIds = enabled ? [...config.value.bypassProviderIds, id] : config.value.bypassProviderIds.filter(value => value !== id)
}

onMounted(async () => {
  await load(true)
  try { const list = await llmAPI.providers(); if (!disposed) providers.value = list }
  catch (err) { if (!disposed) error.value = message(err) }
  if (!disposed) timer = setInterval(() => { void load() }, 4000)
})
onUnmounted(() => { disposed = true; if (timer) clearInterval(timer) })
</script>

<template>
  <div class="privacy-settings">
    <div class="privacy-heading">
      <p class="settings-item-sub">{{ t('privacyIntro') }}</p>
      <button v-if="config" type="button" class="privacy-button privacy-primary" :disabled="saving || !dirty || !mappingTTLValid" @click="save">{{ saving ? t('privacySaving') : t('privacySave') }}</button>
    </div>
    <p v-if="error" class="privacy-alert" role="alert">{{ error }} <button v-if="!config" type="button" @click="load(true)">{{ t('privacyRetry') }}</button></p>
    <p v-if="notice" class="privacy-notice" role="status">{{ notice }}</p>
    <p v-if="dirty" class="settings-item-sub" role="status">{{ t('privacyUnsaved') }}</p>
    <LoadingSpinner v-if="loading" />
    <template v-else-if="config">
      <section class="settings-card privacy-card">
        <div class="privacy-row">
          <div><h3>{{ t('privacyEnable') }}</h3><p class="settings-item-sub">{{ t('privacyEnableHint') }}</p></div>
          <ToggleSwitch v-model="config.enabled" :aria-label="t('privacyEnable')" />
        </div>
        <div class="privacy-row"><div><h3>{{ t('privacySecrets') }}</h3><p class="settings-item-sub">{{ t('privacySecretsHint') }}</p></div><ToggleSwitch v-model="config.secrets" :aria-label="t('privacySecrets')" /></div>
        <div class="privacy-row"><div><h3>{{ t('privacyPersonal') }}</h3><p class="settings-item-sub">{{ t('privacyPersonalHint') }}</p></div><ToggleSwitch v-model="config.personal" :aria-label="t('privacyPersonal')" /></div>
      </section>

      <section class="settings-card privacy-card" :aria-label="t('privacyMappingTitle')">
        <div class="privacy-mapping-heading">
          <div>
            <h3>{{ t('privacyMappingTitle') }}</h3>
            <p class="settings-item-sub">{{ t('privacyMappingHint') }}</p>
          </div>
          <span class="privacy-badge privacy-mapping-status" role="status">{{ mappingModeLabel }}</span>
        </div>
        <div class="privacy-mapping-fields">
          <label class="privacy-field">
            <span>{{ t('privacyMappingStorage') }}</span>
            <select v-model="config.mappingPersistence">
              <option :value="false">{{ t('privacyMappingMemory') }}</option>
              <option :value="true">{{ t('privacyMappingEncrypted') }}</option>
            </select>
          </label>
          <label class="privacy-field">
            <span>{{ t('privacyMappingTTL') }}</span>
            <input v-model.number="mappingTTL" type="number" min="1" :max="maxMappingTTL" step="1" required
              :aria-invalid="!mappingTTLValid || undefined" :aria-describedby="!mappingTTLValid ? mappingTTLHintId : undefined" />
          </label>
        </div>
        <p v-if="!mappingTTLValid" :id="mappingTTLHintId" class="settings-item-sub" role="status">{{ t('privacyMappingTTLRange') }}</p>
        <div class="privacy-mapping-hints">
          <p v-if="mappings?.keyStatus === 'mismatch'" class="settings-item-sub">{{ t('privacyMappingKeyMismatch') }}</p>
          <p v-else class="settings-item-sub">{{ t('privacyMappingKeyHint', { key: 'SLIMEBOT_PRIVACY_MAPPING_KEY' }) }}</p>
          <p v-if="mappings?.storageError" class="settings-item-sub">{{ t('privacyMappingStorageError') }}</p>
          <p v-if="mappings?.mode !== 'memory' || config.mappingPersistence" class="settings-item-sub">{{ t('privacyMappingDisableHint') }}</p>
        </div>
      </section>

      <section class="settings-card privacy-card">
        <h3>{{ t('privacyKinds') }}</h3><p class="settings-item-sub">{{ t('privacyActionHint') }}</p>
        <div class="privacy-kind-grid">
          <div v-for="kind in privacyCoreKinds" :key="kind" class="privacy-kind-row">
            <label><input type="checkbox" :checked="privacyKindEnabled(config, kind)" @change="toggleKind(kind, ($event.target as HTMLInputElement).checked)">{{ t(`privacyKind_${kind}`) }}</label>
            <AppSelect
              class="privacy-action-select"
              :aria-label="`${t(`privacyKind_${kind}`)} ${t('privacyAction')}`"
              :model-value="config.actions[kind] || 'mask'"
              :options="actionOptions"
              @update:model-value="setAction(kind, $event)"
            />
          </div>
        </div>
        <details class="privacy-details"><summary>{{ t('privacyExtraKinds') }}</summary>
          <div class="privacy-kind-grid"><div v-for="kind in privacyExtraKinds" :key="kind" class="privacy-kind-row">
            <label><input type="checkbox" :checked="privacyKindEnabled(config, kind)" @change="toggleKind(kind, ($event.target as HTMLInputElement).checked)">{{ t(`privacyKind_${kind}`) }}</label>
            <AppSelect
              class="privacy-action-select"
              :aria-label="`${t(`privacyKind_${kind}`)} ${t('privacyAction')}`"
              :model-value="config.actions[kind] || 'mask'"
              :options="actionOptions"
              @update:model-value="setAction(kind, $event)"
            />
          </div></div>
        </details>
      </section>

      <section class="settings-card privacy-card">
        <div class="privacy-two-columns">
          <label class="privacy-field"><span>{{ t('privacyWords') }}</span><textarea v-model="words" rows="5" spellcheck="false" :placeholder="t('privacyWordsPlaceholder')" /><span class="settings-item-sub">{{ t('privacyOnePerLine') }}</span></label>
          <label class="privacy-field"><span>{{ t('privacyAllow') }}</span><textarea v-model="allow" rows="5" spellcheck="false" :placeholder="t('privacyAllowPlaceholder')" /><span class="settings-item-sub">{{ t('privacyAllowHint') }}</span></label>
        </div>
        <div class="privacy-inline"><label><input v-model="config.ignoreCase" type="checkbox">{{ t('privacyIgnoreCase') }}</label><label><input v-model="config.wholeWords" type="checkbox">{{ t('privacyWholeWords') }}</label></div>
        <details class="privacy-details"><summary>{{ t('privacyCustomRules') }}</summary>
          <p class="settings-item-sub">{{ t('privacyRulesHint') }}</p>
          <div v-for="(rule, index) in config.rules" :key="index" class="privacy-rule">
            <label class="privacy-field"><span>{{ t('privacyRuleKind') }}</span><input v-model="rule.kind" maxlength="24" placeholder="CUSTOM" /></label>
            <label class="privacy-field"><span>{{ t('privacyRulePrefix') }}</span><input v-model="rule.prefix" placeholder="company_key_" /></label>
            <label class="privacy-field"><span>{{ t('privacyRulePattern') }}</span><input v-model="rule.pattern" placeholder="[A-Z]{3}-[0-9]{8}" spellcheck="false" /></label>
            <button type="button" class="privacy-button" :aria-label="`${t('privacyRemoveRule')} ${index + 1}`" @click="config.rules.splice(index, 1)">{{ t('privacyRemove') }}</button>
          </div>
          <button type="button" class="privacy-button" :disabled="config.rules.length >= 32" @click="config.rules.push({ kind: 'CUSTOM', prefix: '', pattern: '' })">{{ t('privacyAddRule') }}</button>
        </details>
      </section>

      <section class="settings-card privacy-card">
        <div class="privacy-row"><div><h3>{{ t('privacyLocalModel') }}</h3><p class="settings-item-sub">{{ t('privacyModelHint') }}</p></div><ToggleSwitch v-model="config.modelEnabled" :aria-label="t('privacyLocalModel')" /></div>
        <div class="privacy-model-status" role="status">
          <span class="privacy-badge">{{ model?.installing ? t('privacyModelInstalling') : model?.installed ? t('privacyModelReady') : t('privacyModelMissing') }}</span>
          <span v-if="model?.running" class="settings-item-sub">{{ t('privacyModelRunning') }}</span>
          <span v-if="model?.installing" class="settings-item-sub">{{ progress }}%</span>
          <button v-if="!model?.installed && !model?.installing" type="button" class="privacy-button" :disabled="busy" @click="modelAction('install')">{{ t('privacyModelInstall') }}</button>
          <button v-if="model?.installing" type="button" class="privacy-button" :disabled="busy" @click="modelAction('cancel')">{{ t('privacyModelCancel') }}</button>
          <button v-if="model?.installed && !model?.installing" type="button" class="privacy-button" :disabled="busy" @click="modelAction('remove')">{{ t('privacyModelRemove') }}</button>
        </div>
        <progress v-if="model?.installing" :value="progress" max="100" :aria-label="t('privacyModelInstalling')" />
        <p v-if="model?.error" class="privacy-alert" role="alert">{{ model.error }}</p>
        <div class="privacy-two-columns privacy-model-options">
          <label class="privacy-field">
            <span>{{ t('privacyThreshold') }} · {{ config.modelThreshold.toFixed(2) }}</span>
            <input
              v-model.number="config.modelThreshold"
              class="privacy-threshold"
              type="range"
              min="0.5"
              max="1"
              step="0.01"
              :style="{ '--threshold-progress': `${Math.round((config.modelThreshold - 0.5) * 200)}%` }"
            />
          </label>
          <label class="privacy-field"><span>{{ t('privacyTimeout') }}</span><input v-model.number="config.modelTimeoutSeconds" type="number" min="1" max="120" /></label>
        </div>
        <p class="settings-item-sub">{{ t('privacyStrictModel') }}</p>
      </section>

      <section class="settings-card privacy-card">
        <h3>{{ t('privacyProviderExceptions') }}</h3><p class="settings-item-sub">{{ t('privacyProviderHint') }}</p>
        <div v-if="providers.length" class="privacy-provider-list"><label v-for="provider in providers" :key="provider.id"><input type="checkbox" :checked="config.bypassProviderIds.includes(provider.id)" @change="toggleProvider(provider.id, ($event.target as HTMLInputElement).checked)">{{ provider.name }}</label></div>
        <p v-else class="settings-item-sub">{{ t('privacyNoProviders') }}</p>
      </section>

      <section class="settings-card privacy-card">
        <h3>{{ t('privacyTry') }}</h3><p class="settings-item-sub">{{ t('privacyTryHint') }}</p>
        <label class="privacy-field"><span class="sr-only">{{ t('privacyTryInput') }}</span><textarea v-model="previewInput" rows="4" spellcheck="false" :placeholder="t('privacyTryPlaceholder')" /></label>
        <button type="button" class="privacy-button" :disabled="testing || !previewInput.trim()" @click="tryPreview">{{ testing ? t('privacyTesting') : t('privacyPreview') }}</button>
        <div v-if="preview" class="privacy-preview" aria-live="polite"><p class="settings-item-sub">{{ t('privacyPreviewCount', { count: preview.hits.length }) }}<span v-if="preview.blocked"> · {{ t('privacyBlocked') }}</span></p><pre>{{ preview.text }}</pre><div class="privacy-inline"><span v-for="(hit, index) in preview.hits" :key="index" class="privacy-badge">{{ t(`privacyKind_${hit.kind}`, hit.kind) }} · {{ t(`privacyAction_${hit.action}`) }}</span></div></div>
      </section>

      <section class="settings-card privacy-card">
        <div class="privacy-row"><div><h3>{{ t('privacyRecords') }}</h3><p class="settings-item-sub">{{ t('privacyRecordsHint') }}</p></div><button type="button" class="privacy-button" :disabled="!records.length" @click="clearRecords">{{ t('privacyClearRecords') }}</button></div>
        <div class="privacy-stat-grid"><div><strong>{{ totals.masked }}</strong><span>{{ t('privacyMaskedValues') }}</span></div><div><strong>{{ totals.warned }}</strong><span>{{ t('privacyWarnings') }}</span></div><div><strong>{{ totals.blocked }}</strong><span>{{ t('privacyBlockedRequests') }}</span></div></div>
        <p v-if="!records.length" class="settings-item-sub">{{ t('privacyNoRecords') }}</p>
        <div v-else class="privacy-records"><details v-for="record in records.slice(0, 30)" :key="record.id" class="privacy-record"><summary><time>{{ new Date(record.time).toLocaleTimeString() }}</time><span>{{ record.source || 'LLM' }}</span><span>{{ record.blocked ? t('privacyBlocked') : record.failed ? t('privacyFailed') : t('privacyRecordMasked', { count: record.masked }) }}</span><span class="settings-item-sub">{{ record.durationMs }} ms</span></summary><div class="privacy-record-detail"><span>{{ t('privacyRestored', { count: record.restored }) }}</span><span v-for="(count, kind) in record.kinds" :key="kind">{{ t(`privacyKind_${kind}`, String(kind)) }} × {{ count }}</span><code v-if="record.sessionId">{{ record.sessionId }}</code></div></details></div>
      </section>
      <p class="privacy-boundary settings-item-sub">{{ t('privacyBoundary') }}</p>
      <div class="privacy-bottom-save"><span class="settings-item-sub">{{ dirty ? t('privacyUnsaved') : t('privacySavedState') }}</span><button type="button" class="privacy-button privacy-primary" :disabled="saving || !dirty || !mappingTTLValid" @click="save">{{ saving ? t('privacySaving') : t('privacySave') }}</button></div>
    </template>
  </div>
</template>

<style scoped>
.privacy-settings { display: flex; flex-direction: column; gap: 18px; max-width: 1000px; margin: 0 auto; color: var(--text-primary); text-wrap: pretty; }
.privacy-heading, .privacy-row { display: flex; align-items: center; justify-content: space-between; gap: 18px; }
.privacy-heading p, .privacy-card p { margin-top: 4px; }
.privacy-card { border-radius: 14px; padding: 20px; }
.privacy-card h3 { font-size: 14px; line-height: 22px; font-weight: 600; margin: 0; }
.privacy-card .privacy-row + .privacy-row { margin-top: 18px; padding-top: 18px; border-top: 1px solid var(--card-border); }
.privacy-button { border: 1px solid var(--input-border); background: var(--input-bg); color: var(--text-primary); padding: 7px 12px; min-height: 34px; border-radius: 8px; font-size: 13px; cursor: pointer; white-space: nowrap; transition: background .15s; }
.privacy-button:hover:not(:disabled) { background: var(--primary-alpha-08); }
.privacy-primary { background: var(--sb-brand); border-color: var(--sb-brand); color: #fff; }
.privacy-primary:hover:not(:disabled) { background: var(--sb-brand); filter: brightness(.95); }
.privacy-button:disabled { opacity: .45; cursor: default; }
.privacy-button:focus-visible, input:focus-visible, textarea:focus-visible, select:focus-visible, summary:focus-visible { outline: 2px solid var(--sb-brand); outline-offset: 3px; }
.privacy-kind-grid { display: grid; grid-template-columns: repeat(2, minmax(0,1fr)); gap: 12px 24px; margin-top: 18px; }
.privacy-kind-row { display: flex; justify-content: space-between; align-items: center; gap: 8px; min-width: 0; }
.privacy-kind-row label, .privacy-inline label, .privacy-provider-list label { display: flex; align-items: center; gap: 8px; font-size: 13px; cursor: pointer; }
.privacy-action-select { flex: 0 0 104px; }
.privacy-action-select :deep(.select-trigger) {
  width: 100%;
  min-height: 34px;
  justify-content: space-between;
}
input[type=checkbox] { accent-color: var(--sb-brand); width: 15px; height: 15px; flex-shrink: 0; }
.privacy-threshold { width: 100%; accent-color: var(--sb-brand); }
@media (forced-colors: none) {
  .privacy-threshold {
    --threshold-track: linear-gradient(to right, var(--sb-brand) var(--threshold-progress), var(--input-border) var(--threshold-progress));
    appearance: none;
    background: transparent;
    cursor: pointer;
  }
  .privacy-threshold::-webkit-slider-runnable-track {
    height: 6px;
    border-radius: 999px;
    background: var(--threshold-track);
  }
  .privacy-threshold::-moz-range-track {
    height: 6px;
    border-radius: 999px;
    background: var(--threshold-track);
  }
  .privacy-threshold::-webkit-slider-thumb {
    appearance: none;
    width: 16px;
    height: 16px;
    margin-top: -5px;
    border-radius: 50%;
    background: var(--sb-brand);
  }
  .privacy-threshold::-moz-range-thumb {
    width: 16px;
    height: 16px;
    border: 0;
    border-radius: 50%;
    background: var(--sb-brand);
  }
}
.privacy-two-columns { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 20px; }
.privacy-field { display: flex; flex-direction: column; gap: 8px; min-width: 0; font-size: 13px; }
.privacy-field textarea, .privacy-field input:not([type=range]), .privacy-field select { width: 100%; padding: 10px 12px; border: 1px solid var(--input-border); border-radius: 8px; background: var(--input-bg); color: var(--text-primary); font: inherit; line-height: 1.6; }
.privacy-mapping-heading { display: flex; align-items: flex-start; justify-content: space-between; flex-wrap: wrap; column-gap: 18px; row-gap: 6px; }
.privacy-mapping-fields { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 200px), 1fr)); gap: 12px 20px; margin: 14px 0 8px; }
.privacy-mapping-fields input, .privacy-mapping-fields select { min-height: 40px; box-sizing: border-box; }
.privacy-mapping-status { max-width: 100%; overflow-wrap: anywhere; }
.privacy-mapping-hints { border-top: 1px solid var(--card-border); margin-top: 12px; padding-top: 8px; overflow-wrap: anywhere; }
.privacy-field textarea { resize: vertical; min-height: 90px; }
.privacy-inline, .privacy-model-status, .privacy-provider-list { display: flex; align-items: center; flex-wrap: wrap; gap: 12px; margin-top: 14px; }
.privacy-details { margin-top: 20px; padding-top: 16px; border-top: 1px solid var(--card-border); font-size: 13px; }
summary { cursor: pointer; }
.privacy-details > summary { color: var(--text-secondary); }
.privacy-rule { display: grid; grid-template-columns: 1fr 1fr 1.6fr auto; gap: 10px; align-items: end; margin: 16px 0; }
.privacy-model-options { margin: 18px 0 12px; }
.privacy-model-options .privacy-field > input {
  height: 40px;
}
.privacy-badge { display: inline-flex; padding: 4px 8px; border-radius: 6px; background: var(--primary-alpha-08); color: var(--text-secondary); font-size: 12px; }
.privacy-model-status + progress { width: 100%; height: 6px; accent-color: var(--sb-brand); margin-top: 12px; }
.privacy-alert { border: 1px solid var(--card-border); border-radius: 8px; background: var(--input-bg); color: var(--text-primary); padding: 12px; font-size: 13px; white-space: pre-wrap; overflow-wrap: anywhere; }
.privacy-alert button { text-decoration: underline; margin-left: 8px; }
.privacy-notice { font-size: 13px; color: var(--sb-brand); }
.privacy-field + .privacy-button { margin-top: 12px; }
.privacy-preview { margin-top: 16px; }
.privacy-preview pre { border-radius: 8px; padding: 14px; background: var(--input-bg); font-size: 12px; line-height: 1.8; white-space: pre-wrap; overflow-wrap: anywhere; user-select: text; }
.privacy-stat-grid { display: grid; grid-template-columns: repeat(3,1fr); gap: 16px; margin: 20px 0; }
.privacy-stat-grid > div { display: flex; flex-direction: column; gap: 4px; }
.privacy-stat-grid strong { font-size: 24px; line-height: 32px; font-weight: 600; font-variant-numeric: tabular-nums; }
.privacy-stat-grid span { font-size: 12px; color: var(--text-muted); }
.privacy-record { border-top: 1px solid var(--card-border); padding: 10px 0; font-size: 12px; }
.privacy-record summary { display: grid; grid-template-columns: 82px 1fr 1.4fr 72px; align-items: center; gap: 10px; }
.privacy-record-detail { display: flex; flex-wrap: wrap; gap: 8px 16px; padding: 12px 0 4px; color: var(--text-muted); overflow-wrap: anywhere; }
.privacy-record-detail code { width: 100%; user-select: text; }
.privacy-bottom-save { position: sticky; bottom: -24px; display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 12px 0; background: var(--bg-main); border-top: 1px solid var(--card-border); }
.privacy-boundary { line-height: 1.7; }
@media (max-width: 850px) { .privacy-kind-grid, .privacy-two-columns { grid-template-columns: 1fr; } .privacy-rule { grid-template-columns: 1fr 1fr; } .privacy-card { padding: 16px; } .privacy-record summary { grid-template-columns: 76px 1fr 1.2fr; } .privacy-record summary > :last-child { display: none; } }
@media (prefers-reduced-motion: reduce) { .privacy-button { transition: none; } }
</style>
