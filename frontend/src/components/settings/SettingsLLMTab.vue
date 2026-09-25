<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { mdiChevronLeft, mdiChevronRight, mdiClose, mdiDeleteOutline, mdiMagnify, mdiPencilOutline, mdiPlus, mdiRefresh, mdiServerOutline } from '@mdi/js'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import { formatContextSize } from '@/utils/contextSize'
import type { DiscoveredModel, LLMConfig, LLMProvider } from '@/types/settings'

const props = defineProps<{
  providers: LLMProvider[]
  models: LLMConfig[]
  discoveryProviderId: string
  discoveredModels: DiscoveredModel[]
  visibleDiscovered: DiscoveredModel[]
  discovering: boolean
  discoverySaving: boolean
  discoveryQuery: string
  selectedModelIds: string[]
}>()

const emit = defineEmits<{
  addProvider: []
  editProvider: [provider: LLMProvider]
  deleteProvider: [id: string]
  addModel: [providerId: string]
  editModel: [model: LLMConfig]
  deleteModel: [id: string]
  discover: [providerId: string]
  closeDiscovery: []
  updateQuery: [query: string]
  toggleModel: [id: string]
  toggleAll: [ids: string[]]
  addSelected: []
}>()

const { t } = useI18n()
const selectedProviderId = ref('')
const selectedProvider = computed(() => props.providers.find(provider => provider.id === selectedProviderId.value))
const modelIdsByProvider = computed(() => {
  const entries = new Map<string, Set<string>>()
  for (const model of props.models) {
    if (!entries.has(model.providerId)) entries.set(model.providerId, new Set())
    entries.get(model.providerId)!.add(model.model)
  }
  return entries
})

function modelsFor(providerId: string) {
  return props.models.filter(model => model.providerId === providerId)
}

function availableDiscovered(providerId: string) {
  const current = modelIdsByProvider.value.get(providerId) || new Set<string>()
  return props.visibleDiscovered.slice(0, 100).filter(model => !current.has(model.id))
}

function protocolLabel(protocol: string) {
  return t(protocol === 'anthropic' ? 'providerAnthropic' : protocol === 'deepseek' ? 'providerDeepSeek' : 'providerOpenAI')
}

function backToProviders() {
  selectedProviderId.value = ''
  emit('closeDiscovery')
}
</script>

<template>
  <div class="llm-settings">
    <template v-if="!selectedProvider">
      <div class="flex items-start justify-between gap-3 mb-5">
        <div>
          <p class="section-label mb-1">{{ t('modelProviders') }}</p>
          <p class="llm-description">{{ t('modelProvidersHint') }}</p>
        </div>
        <button type="button" class="btn-primary action-btn llm-main-action flex items-center gap-1.5 px-3 py-2 rounded-xl cursor-pointer settings-action-text" @click="emit('addProvider')">
          <MdiIcon :path="mdiPlus" :size="14" />{{ t('addProvider') }}
        </button>
      </div>

      <div v-if="providers.length === 0" class="settings-card llm-empty rounded-xl text-center px-4 py-10">
        <div class="llm-empty-icon"><MdiIcon :path="mdiServerOutline" :size="24" /></div>
        <p class="settings-item-name mt-3">{{ t('noProviders') }}</p>
        <p class="llm-description mt-1">{{ t('noProvidersHint') }}</p>
      </div>

      <div class="flex flex-col gap-2.5">
        <button v-for="provider in providers" :key="provider.id" type="button" class="settings-card llm-provider-entry w-full rounded-xl px-4 py-3.5" @click="selectedProviderId = provider.id">
          <span class="llm-provider-icon flex-shrink-0"><MdiIcon :path="mdiServerOutline" :size="19" /></span>
          <span class="flex-1 min-w-0 text-left">
            <span class="flex flex-wrap items-center gap-2"><span class="settings-item-name llm-provider-name">{{ provider.name }}</span><span class="llm-protocol-badge">{{ protocolLabel(provider.protocol) }}</span></span>
            <span class="settings-item-sub llm-provider-url truncate block mt-1" :title="provider.baseUrl">{{ provider.baseUrl }}</span>
          </span>
          <span class="llm-provider-count">{{ t('providerModelCount', { count: modelsFor(provider.id).length }) }}</span>
          <MdiIcon :path="mdiChevronRight" :size="17" class="llm-entry-chevron flex-shrink-0" />
        </button>
      </div>
    </template>

    <template v-else>
      <button type="button" class="llm-back-button mb-3" @click="backToProviders"><MdiIcon :path="mdiChevronLeft" :size="16" />{{ t('modelProviders') }}</button>
      <section class="settings-card llm-provider-card rounded-xl overflow-hidden">
        <div class="llm-provider-header flex items-start gap-3 px-4 py-4">
          <div class="llm-provider-icon flex-shrink-0"><MdiIcon :path="mdiServerOutline" :size="19" /></div>
          <div class="flex-1 min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <h3 class="settings-item-name llm-provider-name">{{ selectedProvider.name }}</h3>
              <span class="llm-protocol-badge">{{ protocolLabel(selectedProvider.protocol) }}</span>
            </div>
            <p class="settings-item-sub llm-provider-url truncate mt-1" :title="selectedProvider.baseUrl">{{ selectedProvider.baseUrl }}</p>
          </div>
          <div class="flex items-center gap-1 flex-shrink-0">
            <button type="button" class="llm-icon-button" :title="t('editProvider')" :aria-label="t('editProvider')" @click="emit('editProvider', selectedProvider)"><MdiIcon :path="mdiPencilOutline" :size="16" /></button>
            <button type="button" class="llm-icon-button llm-danger-button" :title="t('deleteProvider')" :aria-label="t('deleteProvider')" @click="emit('deleteProvider', selectedProvider.id)"><MdiIcon :path="mdiDeleteOutline" :size="16" /></button>
          </div>
        </div>

        <div class="llm-model-list">
          <div class="llm-model-list-heading px-4 py-2">{{ t('availableModels') }} <span>{{ modelsFor(selectedProvider.id).length }}</span></div>
          <div v-for="model in modelsFor(selectedProvider.id)" :key="model.id" class="llm-model-row flex items-center gap-3 px-4 py-2.5">
            <div class="llm-model-dot flex-shrink-0" />
            <div class="flex-1 min-w-0">
              <div class="settings-item-name truncate">{{ model.name }}</div>
              <div class="settings-item-sub truncate mt-0.5">{{ model.model }} · {{ t('contextSize') }} {{ formatContextSize(model.contextSize || 1_000_000) }} <span class="llm-context-source">{{ t(model.contextSizeSource === 'detected' ? 'contextSizeDetected' : model.contextSizeSource === 'fallback' ? 'contextSizeEstimated' : 'contextSizeCustom') }}</span></div>
            </div>
            <button type="button" class="llm-icon-button" :title="t('editModel')" :aria-label="t('editModel')" @click="emit('editModel', model)"><MdiIcon :path="mdiPencilOutline" :size="15" /></button>
            <button type="button" class="llm-icon-button llm-danger-button" :title="t('delete')" :aria-label="t('delete')" @click="emit('deleteModel', model.id)"><MdiIcon :path="mdiDeleteOutline" :size="15" /></button>
          </div>
          <p v-if="modelsFor(selectedProvider.id).length === 0" class="llm-no-models px-4 py-5">{{ t('noModelsInProvider') }}</p>
        </div>

        <div class="llm-card-actions flex flex-wrap gap-2 px-4 py-3">
          <button type="button" class="llm-secondary-button" :disabled="discovering && discoveryProviderId === selectedProvider.id" @click="emit('discover', selectedProvider.id)">
            <LoadingSpinner v-if="discovering && discoveryProviderId === selectedProvider.id" size-class="w-3.5 h-3.5" />
            <MdiIcon v-else :path="mdiRefresh" :size="15" />{{ t('discoverModels') }}
          </button>
          <button type="button" class="llm-secondary-button" @click="emit('addModel', selectedProvider.id)"><MdiIcon :path="mdiPlus" :size="15" />{{ t('addModelManually') }}</button>
        </div>

        <div v-if="discoveryProviderId === selectedProvider.id" class="llm-discovery px-4 py-4">
          <div class="flex items-center justify-between gap-2 mb-3">
            <div>
              <p class="settings-item-name">{{ t('discoveredModels') }}</p>
              <p class="llm-description mt-0.5">{{ t('discoveredModelsHint') }}</p>
            </div>
            <button type="button" class="llm-icon-button" :title="t('close')" :aria-label="t('close')" @click="emit('closeDiscovery')"><MdiIcon :path="mdiClose" :size="17" /></button>
          </div>
          <div v-if="!discovering" class="llm-discovery-search">
            <MdiIcon :path="mdiMagnify" :size="17" />
            <input :value="discoveryQuery" type="search" :placeholder="t('searchModels')" :aria-label="t('searchModels')" @input="emit('updateQuery', ($event.target as HTMLInputElement).value)" />
          </div>
          <div v-if="discovering" class="llm-discovery-status"><LoadingSpinner size-class="w-4 h-4" />{{ t('discoveringModels') }}</div>
          <template v-else>
            <div class="llm-discovery-toolbar flex items-center justify-between gap-2 py-2">
              <span>{{ t('modelsFound', { count: discoveredModels.length }) }}</span>
              <button v-if="availableDiscovered(selectedProvider.id).length > 0" type="button" class="llm-link-button" @click="emit('toggleAll', availableDiscovered(selectedProvider.id).map(model => model.id))">{{ t('selectVisibleModels') }}</button>
            </div>
            <div class="llm-discovery-list">
              <label v-for="model in visibleDiscovered.slice(0, 100)" :key="model.id" class="llm-discovery-row flex items-center gap-2.5">
                <input type="checkbox" :checked="selectedModelIds.includes(model.id) || modelIdsByProvider.get(selectedProvider.id)?.has(model.id)" :disabled="modelIdsByProvider.get(selectedProvider.id)?.has(model.id) || discoverySaving" @change="emit('toggleModel', model.id)" />
                <span class="flex-1 min-w-0"><strong class="truncate block">{{ model.name }}</strong><small class="truncate block">{{ model.id }}</small></span>
                <span v-if="model.contextSize && !modelIdsByProvider.get(selectedProvider.id)?.has(model.id)" class="llm-discovery-context">{{ formatContextSize(model.contextSize) }}</span>
                <span v-if="modelIdsByProvider.get(selectedProvider.id)?.has(model.id)" class="llm-added-label">{{ t('alreadyAdded') }}</span>
              </label>
              <p v-if="visibleDiscovered.length === 0" class="llm-no-models px-3 py-4">{{ t('noDiscoveredModels') }}</p>
            </div>
            <p v-if="visibleDiscovered.length > 100" class="llm-description mt-2">{{ t('searchMoreModels') }}</p>
            <div class="flex justify-end mt-3"><button type="button" class="btn-primary action-btn llm-main-action px-3 py-2 rounded-lg cursor-pointer settings-action-text" :disabled="selectedModelIds.length === 0 || discoverySaving" @click="emit('addSelected')">{{ t('addSelectedModels', { count: selectedModelIds.length }) }}</button></div>
          </template>
        </div>
      </section>
    </template>
  </div>
</template>

<style scoped>
.llm-description { color: var(--text-secondary); font-size: 12px; line-height: 1.5; }
.llm-main-action { white-space: nowrap; flex-shrink: 0; }
.llm-empty-icon, .llm-provider-icon { display: inline-flex; align-items: center; justify-content: center; color: var(--sb-brand); background: var(--primary-alpha-10); border-radius: 10px; }
.llm-empty-icon { width: 42px; height: 42px; margin: auto; }
.llm-provider-icon { width: 38px; height: 38px; }
.llm-provider-entry { display: flex; align-items: center; gap: 12px; text-align: left; cursor: pointer; transition: border-color .18s, background .18s; }
.llm-provider-entry:hover { border-color: var(--sb-brand); background: var(--primary-alpha-05); }
.llm-provider-entry:focus-visible, .llm-back-button:focus-visible { outline: 2px solid var(--sb-brand); outline-offset: 2px; }
.llm-provider-count { flex-shrink: 0; color: var(--text-secondary); font-size: 11px; }
.llm-entry-chevron { color: var(--text-muted); transition: color .18s, transform .18s; }
.llm-provider-entry:hover .llm-entry-chevron { color: var(--sb-brand); transform: translateX(2px); }
.llm-back-button { display: inline-flex; align-items: center; gap: 4px; padding: 5px 8px; border-radius: 7px; color: var(--text-secondary); font-size: 12px; cursor: pointer; transition: color .18s, background .18s; }
.llm-back-button:hover { color: var(--sb-brand); background: var(--primary-alpha-08); }
.llm-provider-name { font-size: 15px; font-weight: 600; }
.llm-provider-url { color: var(--text-secondary); }
.llm-protocol-badge { color: var(--sb-brand); background: var(--primary-alpha-10); border-radius: 6px; padding: 3px 7px; font-size: 11px; font-weight: 600; }
.llm-context-source { color: var(--text-muted); margin-left: 3px; }
.llm-discovery-context { flex-shrink: 0; color: var(--sb-brand); background: var(--primary-alpha-10); border-radius: 6px; padding: 2px 6px; font-size: 11px; font-weight: 600; }
.llm-icon-button { width: 30px; height: 30px; display: inline-flex; align-items: center; justify-content: center; border-radius: 8px; color: var(--text-muted); cursor: pointer; transition: background .15s, color .15s; }
.llm-icon-button:hover { background: var(--primary-alpha-10); color: var(--sb-brand); }
.llm-danger-button:hover { background: var(--danger-alpha-10); color: var(--color-danger); }
.llm-icon-button:focus-visible, .llm-secondary-button:focus-visible, .llm-link-button:focus-visible { outline: 2px solid var(--sb-brand); outline-offset: 2px; }
.llm-model-list { border-top: 1px solid var(--card-border); }
.llm-model-list-heading { background: var(--input-bg); color: var(--text-secondary); font-size: 11px; font-weight: 600; letter-spacing: .02em; }
.llm-model-list-heading span { margin-left: 4px; color: var(--sb-brand); }
.llm-model-row + .llm-model-row { border-top: 1px solid var(--card-border); }
.llm-model-row .settings-item-name { font-size: 13px; }
.llm-model-row .settings-item-sub { color: var(--text-secondary); }
.llm-model-dot { width: 6px; height: 6px; border-radius: 50%; background: var(--sb-brand); }
.llm-no-models { color: var(--text-secondary); font-size: 12px; }
.llm-card-actions { border-top: 1px solid var(--card-border); }
.llm-secondary-button { display: inline-flex; align-items: center; gap: 6px; padding: 6px 10px; border: 1px solid var(--input-border); border-radius: 8px; color: var(--text-secondary); background: var(--input-bg); font-size: 12px; cursor: pointer; transition: border-color .15s, color .15s; }
.llm-secondary-button:hover { border-color: var(--sb-brand); color: var(--sb-brand); }
.llm-secondary-button:disabled { opacity: .5; cursor: default; }
.llm-discovery { border-top: 1px solid var(--card-border); background: var(--input-bg); }
.llm-discovery-search { display: flex; align-items: center; gap: 8px; height: 36px; padding: 0 10px; border: 1px solid var(--input-border); border-radius: 9px; color: var(--text-muted); background: var(--card-bg); }
.llm-discovery-search:focus-within { border-color: var(--sb-brand); box-shadow: var(--focus-ring-shadow); }
.llm-discovery-search input { flex: 1; min-width: 0; outline: none; color: var(--text-primary); background: transparent; font-size: 12px; }
.llm-discovery-toolbar, .llm-added-label { color: var(--text-secondary); font-size: 11px; }
.llm-link-button { color: var(--sb-brand); cursor: pointer; }
.llm-discovery-list { max-height: 270px; overflow-y: auto; border: 1px solid var(--card-border); border-radius: 9px; background: var(--card-bg); }
.llm-discovery-row { min-height: 46px; padding: 7px 10px; cursor: pointer; color: var(--text-primary); font-size: 12px; }
.llm-discovery-row + .llm-discovery-row { border-top: 1px solid var(--card-border); }
.llm-discovery-row:hover { background: var(--primary-alpha-10); }
.llm-discovery-row:has(input:disabled) { cursor: default; opacity: .6; }
.llm-discovery-row input { accent-color: var(--sb-brand); }
.llm-discovery-row strong { font-weight: 500; }
.llm-discovery-row small { color: var(--text-secondary); font-size: 11px; }
.llm-discovery-status { display: flex; align-items: center; gap: 8px; color: var(--text-muted); font-size: 12px; padding: 20px 0; }
@media (max-width: 480px) { .llm-provider-header { display: grid; grid-template-columns: 38px minmax(0, 1fr) auto; align-items: start; } .llm-provider-header > .flex-1 { grid-column: 2; min-width: 0; } .llm-provider-header > .flex-shrink-0:last-child { grid-column: 3; grid-row: 1; } }
:global(.dark) .llm-protocol-badge, :global(.dark) .llm-link-button { color: var(--sb-brand-soft); }
</style>
