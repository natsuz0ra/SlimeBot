<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ContextUsageData } from '@/api/chatSocket'
import { contextUsageTone, formatContextTokenCount } from '@/utils/contextSize'

const props = defineProps<{
  usage: ContextUsageData
  modelLabel: string
  compacting?: boolean
  disabled?: boolean
}>()
const emit = defineEmits<{ compact: [] }>()
const { t } = useI18n()
const open = ref(false)
const root = ref<HTMLElement | null>(null)

const tone = computed(() => contextUsageTone(props.usage.usedPercent))
const circleStyle = computed(() => ({ '--context-usage-deg': `${Math.max(0, Math.min(100, props.usage.usedPercent)) * 3.6}deg` }))
const statusLabel = computed(() => props.compacting || props.usage.state === 'compacting'
  ? t('contextStatusCompacting')
  : props.usage.state === 'cooldown'
    ? t('contextStatusCooldown')
    : props.usage.isCompacted ? t('contextStatusCompacted') : t('contextStatusFull'))
const reasonLabel = computed(() => {
  const keys: Record<string, string> = {
    overflow: 'contextReasonOverflow', pressure: 'contextReasonPressure',
    manual: 'contextReasonManual', idle: 'contextReasonIdle',
  }
  const key = keys[props.usage.compactionReason || '']
  return key ? t(key) : props.usage.compactionReason
})
const savedTokens = computed(() => Math.max(0, (props.usage.compactionBeforeTokens || 0) - (props.usage.compactionAfterTokens || 0)))

function layerLabel(layer: string) {
  const keys: Record<string, string> = {
    system: 'contextLayerSystem', project: 'contextLayerProject', tools: 'contextLayerTools',
    skill_catalog: 'contextLayerSkillCatalog', memory: 'contextLayerMemory', runtime: 'contextLayerRuntime',
    compaction: 'contextLayerCompaction', recent_conversation: 'contextLayerRecentConversation',
    current_turn: 'contextLayerCurrentTurn', runtime_state: 'contextLayerRuntimeState',
  }
  return keys[layer] ? t(keys[layer]!) : layer
}

function onDocumentPointerDown(event: PointerEvent) {
  if (open.value && root.value && !root.value.contains(event.target as Node)) open.value = false
}
function onDocumentKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') open.value = false
}
watch(() => props.usage.sessionId, () => { open.value = false })
onMounted(() => {
  document.addEventListener('pointerdown', onDocumentPointerDown)
  document.addEventListener('keydown', onDocumentKeydown)
})
onUnmounted(() => {
  document.removeEventListener('pointerdown', onDocumentPointerDown)
  document.removeEventListener('keydown', onDocumentKeydown)
})
</script>

<template>
  <div ref="root" class="context-usage relative z-[120]">
    <button
      type="button"
      class="context-usage-indicator"
      :class="`context-usage-indicator--${tone}`"
      :style="circleStyle"
      :aria-label="t('contextUsageLabel')"
      :aria-expanded="open"
      :title="t('contextUsageDetails')"
      @click="open = !open"
    >
      <span class="context-usage-core" />
    </button>
    <Transition name="context-popover">
      <section v-if="open" class="context-usage-panel" :aria-label="t('contextUsageDetails')">
        <header class="context-usage-header">
          <div class="min-w-0">
            <h3>{{ t('contextUsageLabel') }}</h3>
            <p :title="modelLabel">{{ modelLabel }}</p>
          </div>
          <strong>{{ usage.usedPercent }}%</strong>
        </header>

        <dl class="context-usage-grid">
          <dt>{{ t('contextUsedTokens') }}</dt><dd>{{ formatContextTokenCount(usage.usedTokens) }}</dd>
          <dt>{{ t('contextTotalTokens') }}</dt><dd>{{ formatContextTokenCount(usage.totalTokens) }}</dd>
          <dt>{{ t('contextAvailablePercent') }}</dt><dd>{{ usage.availablePercent }}%</dd>
          <dt v-if="usage.inputBudget">{{ t('contextInputBudget') }}</dt><dd v-if="usage.inputBudget">{{ formatContextTokenCount(usage.inputBudget) }}</dd>
          <dt v-if="usage.outputReserve">{{ t('contextOutputReserve') }}</dt><dd v-if="usage.outputReserve">{{ formatContextTokenCount(usage.outputReserve) }}</dd>
          <dt>{{ t('contextMeasurement') }}</dt><dd>{{ t(usage.source === 'provider-reported' ? 'contextProviderReported' : 'contextEstimated') }}</dd>
          <dt>{{ t('contextStatus') }}</dt><dd>{{ statusLabel }}</dd>
        </dl>

        <div v-if="usage.layers?.length" class="context-usage-section">
          <h4>{{ t('contextLayerBreakdown') }}</h4>
          <div v-for="layer in usage.layers" :key="layer.layer" class="context-layer-row">
            <span :title="layerLabel(layer.layer)">{{ layerLabel(layer.layer) }}</span>
            <span>{{ formatContextTokenCount(layer.tokens) }}</span>
          </div>
        </div>

        <div v-if="usage.compactedAt || usage.compactionFailure" class="context-usage-section">
          <h4>{{ t('contextLastCompaction') }}</h4>
          <dl v-if="usage.compactedAt" class="context-usage-grid">
            <dt v-if="usage.compactionBeforeTokens !== undefined && usage.compactionAfterTokens !== undefined">{{ t('contextLastCompaction') }}</dt>
            <dd v-if="usage.compactionBeforeTokens !== undefined && usage.compactionAfterTokens !== undefined">
              {{ formatContextTokenCount(usage.compactionBeforeTokens) }} → {{ formatContextTokenCount(usage.compactionAfterTokens) }}
            </dd>
            <dt v-if="savedTokens">{{ t('contextCompactionSaved') }}</dt><dd v-if="savedTokens">{{ formatContextTokenCount(savedTokens) }}</dd>
            <dt v-if="usage.compactionDurationMs !== undefined">{{ t('contextCompactionDuration') }}</dt><dd v-if="usage.compactionDurationMs !== undefined">{{ usage.compactionDurationMs }} ms</dd>
            <dt v-if="reasonLabel">{{ t('contextCompactionReason') }}</dt><dd v-if="reasonLabel">{{ reasonLabel }}</dd>
            <dt>{{ t('contextCompactedTime') }}</dt><dd>{{ new Date(usage.compactedAt).toLocaleString() }}</dd>
          </dl>
          <p v-if="usage.compactionFailure" class="context-usage-failure">{{ usage.compactionFailure }}</p>
        </div>

        <button
          type="button"
          class="context-compact-button"
          :disabled="disabled || compacting"
          @click="emit('compact')"
        >
          {{ compacting ? t('contextStatusCompacting') : t('contextCompactAction') }}
        </button>
      </section>
    </Transition>
  </div>
</template>

<style scoped>
.context-usage {
  --context-usage-size: 16px;
  --context-usage-core-size: 10px;
  flex-shrink: 0;
}
.context-usage-indicator {
  --context-usage-color: #22c55e;
  width: var(--context-usage-size);
  height: var(--context-usage-size);
  padding: 0;
  border: 0;
  border-radius: 999px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  background: conic-gradient(var(--context-usage-color) var(--context-usage-deg, 0deg), rgba(148, 163, 184, 0.28) 0);
  box-shadow: inset 0 0 0 1px rgba(148, 163, 184, 0.22);
}
.context-usage-indicator--warning { --context-usage-color: #f59e0b; }
.context-usage-indicator--danger { --context-usage-color: #ef4444; }
.context-usage-core {
  width: var(--context-usage-core-size);
  height: var(--context-usage-core-size);
  border-radius: 999px;
  background: var(--input-bg);
}
.context-usage-panel {
  position: absolute;
  right: 0;
  bottom: calc(100% + 10px);
  width: min(340px, calc(100vw - 24px));
  max-height: min(560px, 70vh);
  overflow-y: auto;
  padding: 14px;
  border: 1px solid var(--menu-border);
  border-radius: 12px;
  color: var(--text-primary);
  background: var(--menu-bg);
  box-shadow: var(--menu-shadow);
  backdrop-filter: blur(16px);
}
.context-usage-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
.context-usage-header h3 { margin: 0; font-size: 14px; font-weight: 650; }
.context-usage-header p { margin: 3px 0 0; overflow: hidden; color: var(--text-muted); font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }
.context-usage-header strong { font-size: 18px; }
.context-usage-grid { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 5px 12px; margin: 12px 0 0; font-size: 12px; }
.context-usage-grid dt { color: var(--text-muted); }
.context-usage-grid dd { margin: 0; text-align: right; }
.context-usage-section { margin-top: 12px; padding-top: 10px; border-top: 1px solid var(--menu-border); }
.context-usage-section h4 { margin: 0 0 7px; font-size: 12px; font-weight: 650; }
.context-layer-row { display: flex; justify-content: space-between; gap: 12px; padding: 3px 0; color: var(--text-secondary); font-size: 11px; }
.context-layer-row span:first-child { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.context-usage-failure { margin: 8px 0 0; color: var(--tool-error-text); font-size: 11px; line-height: 1.5; overflow-wrap: anywhere; }
.context-compact-button { width: 100%; margin-top: 14px; padding: 8px 12px; border: 1px solid var(--primary-alpha-20); border-radius: 8px; color: var(--sb-brand); background: var(--primary-alpha-08); font-size: 12px; font-weight: 600; cursor: pointer; }
.context-compact-button:hover:not(:disabled) { background: var(--primary-alpha-12); }
.context-compact-button:disabled { opacity: .5; cursor: not-allowed; }
.context-popover-enter-active, .context-popover-leave-active { transition: opacity 130ms ease, transform 130ms ease; transform-origin: bottom right; }
.context-popover-enter-from, .context-popover-leave-to { opacity: 0; transform: translateY(4px) scale(.98); }
</style>
