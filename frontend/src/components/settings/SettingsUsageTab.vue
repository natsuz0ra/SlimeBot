<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { isAxiosError } from 'axios'
import { useI18n } from 'vue-i18n'
import { usageAPI, type UsageGroup, type UsageSummary } from '@/api/usage'
import { settingAPI } from '@/api/settings'
import { useToast } from '@/composables/useToast'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import { isMessagePlatformSessionId } from '@/utils/messagePlatformSessions'
import {
  canJumpToUsageSession,
  formatUsageRowLabel,
  resolveUsageTimeRange,
  type UsageTimeRange,
} from '@/utils/usageRange'

const emit = defineEmits<{
  openSession: [sessionId: string]
  budgetSaved: []
}>()

const { t } = useI18n()
const toast = useToast()
const loading = ref(false)
const saving = ref(false)
const group = ref<UsageGroup>('session')
const range = ref<UsageTimeRange>('all')
const rows = ref<UsageSummary[]>([])
const budget = ref(0)
let activeController: AbortController | null = null

const timeRanges: UsageTimeRange[] = ['all', 'today', '7d', '30d']

const totals = computed(() => rows.value.reduce(
  (acc, row) => ({
    requests: acc.requests + row.requestCount,
    input: acc.input + row.inputTokens,
    output: acc.output + row.outputTokens,
    cacheRead: acc.cacheRead + row.cacheReadInputTokens,
  }),
  { requests: 0, input: 0, output: 0, cacheRead: 0 },
))

function label(row: UsageSummary) {
  return formatUsageRowLabel(group.value, row, {
    unclassified: t('usageUnclassified'),
    telegram: t('telegram'),
  })
}

function rowTitle(row: UsageSummary) {
  const base = label(row)
  if (group.value === 'session' && row.rootSessionId && base !== row.rootSessionId) {
    return `${base} (${row.rootSessionId})`
  }
  if (group.value === 'model' && row.modelConfigId) {
    return `${base} (${row.modelConfigId})`
  }
  return base
}

function isPlatformRow(row: UsageSummary) {
  return group.value === 'session' && isMessagePlatformSessionId(row.rootSessionId)
}

function number(value: number) {
  return new Intl.NumberFormat().format(value)
}

function isCanceledError(error: unknown) {
  if (isAxiosError(error) && error.code === 'ERR_CANCELED') return true
  return error instanceof Error && error.name === 'AbortError'
}

async function load() {
  activeController?.abort()
  const controller = new AbortController()
  activeController = controller
  loading.value = true
  try {
    const windowRange = resolveUsageTimeRange(range.value)
    rows.value = await usageAPI.summary(group.value, {
      ...windowRange,
      signal: controller.signal,
    })
  } catch (error) {
    if (isCanceledError(error)) return
    toast.error(t('usageLoadFailed'))
  } finally {
    if (activeController === controller) {
      loading.value = false
    }
  }
}

async function save() {
  saving.value = true
  try {
    await settingAPI.update({ sessionOutputTokenBudget: Math.max(0, Math.floor(budget.value || 0)) })
    toast.success(t('saveSuccess'))
    emit('budgetSaved')
  } catch {
    toast.error(t('usageBudgetSaveFailed'))
  } finally {
    saving.value = false
  }
}

watch([group, range], load)

onMounted(async () => {
  try {
    budget.value = (await settingAPI.get()).sessionOutputTokenBudget || 0
  } catch {
    toast.error(t('usageLoadFailed'))
  }
  await load()
})

onUnmounted(() => {
  activeController?.abort()
  activeController = null
})
</script>

<template>
  <div class="usage-tab">
    <p class="usage-description">{{ t('usageDescription') }}</p>
    <section class="usage-budget">
      <label for="usage-budget">{{ t('usageBudget') }}</label>
      <div>
        <input id="usage-budget" v-model.number="budget" type="number" min="0" max="1000000000" step="1024" />
        <button type="button" :disabled="saving" @click="save">{{ t('save') }}</button>
      </div>
      <p>{{ t('usageBudgetHint') }}</p>
    </section>

    <div class="usage-toolbar">
      <div class="usage-btn-group" role="group" :aria-label="t('usageSettings')">
        <button
          v-for="item in ['session', 'model', 'project'] as UsageGroup[]"
          :key="item"
          type="button"
          :class="{ active: group === item }"
          @click="group = item"
        >
          {{ t(`usageGroup_${item}`) }}
        </button>
      </div>
      <div class="usage-btn-group" role="group" :aria-label="t('usageRangeLabel')">
        <button
          v-for="item in timeRanges"
          :key="item"
          type="button"
          :class="{ active: range === item }"
          @click="range = item"
        >
          {{ t(`usageRange_${item}`) }}
        </button>
      </div>
    </div>

    <div v-if="!loading && rows.length" class="usage-summary-strip">
      <div class="usage-stat">
        <span>{{ t('usageRequests') }}</span>
        <strong>{{ number(totals.requests) }}</strong>
      </div>
      <div class="usage-stat">
        <span>{{ t('usageInput') }}</span>
        <strong>{{ number(totals.input) }}</strong>
      </div>
      <div class="usage-stat">
        <span>{{ t('usageOutput') }}</span>
        <strong>{{ number(totals.output) }}</strong>
      </div>
      <div class="usage-stat">
        <span>{{ t('usageCacheRead') }}</span>
        <strong>{{ number(totals.cacheRead) }}</strong>
      </div>
    </div>

    <div v-if="loading" class="usage-empty"><LoadingSpinner /></div>
    <div v-else-if="!rows.length" class="usage-empty">{{ t('usageEmpty') }}</div>
    <div v-else class="usage-table">
      <div class="usage-row usage-head">
        <span>{{ t(`usageGroup_${group}`) }}</span>
        <span>{{ t('usageRequests') }}</span>
        <span>{{ t('usageInput') }}</span>
        <span>{{ t('usageOutput') }}</span>
        <span>{{ t('usageCacheRead') }}</span>
      </div>
      <div v-for="row in rows" :key="row.groupKey" class="usage-row">
        <div class="usage-name-cell">
          <button
            v-if="group === 'session' && canJumpToUsageSession(row)"
            type="button"
            class="usage-session-link"
            :title="rowTitle(row)"
            @click="emit('openSession', row.rootSessionId!)"
          >
            {{ label(row) }}
          </button>
          <strong v-else :title="rowTitle(row)">{{ label(row) }}</strong>
          <span v-if="group === 'session' && row.sessionKind === 'task_run'" class="usage-badge usage-badge--accent">
            {{ t('usageSessionTaskRun') }}
          </span>
          <span v-else-if="isPlatformRow(row)" class="usage-badge usage-badge--accent">IM</span>
          <span v-if="group === 'session' && !row.sessionExists" class="usage-badge usage-badge--muted">
            {{ t('usageSessionDeleted') }}
          </span>
        </div>
        <span>{{ number(row.requestCount) }}</span>
        <span>{{ number(row.inputTokens) }}</span>
        <span>{{ number(row.outputTokens) }}</span>
        <span>{{ number(row.cacheReadInputTokens) }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.usage-tab { display: flex; flex-direction: column; gap: 16px; color: var(--text-primary); }
.usage-description, .usage-budget p { margin: 0; font-size: 12px; color: var(--text-muted); line-height: 1.7; }
.usage-budget { padding: 16px; border: 1px solid var(--card-border); border-radius: 10px; background: var(--primary-alpha-05); }
.usage-budget label { display: block; font-size: 13px; font-weight: 600; margin-bottom: 10px; }
.usage-budget > div { display: flex; gap: 8px; }
.usage-budget input { flex: 1; min-width: 0; padding: 8px 10px; border: 1px solid var(--input-border); border-radius: 8px; background: var(--input-bg); color: var(--text-primary); }
button { padding: 6px 11px; border: 1px solid var(--input-border); border-radius: 8px; cursor: pointer; color: var(--text-secondary); background: var(--input-bg); font-size: 12px; transition: border-color 140ms ease, color 140ms ease, background 140ms ease; }
button:hover:not(:disabled) { border-color: var(--primary-alpha-25); color: var(--text-primary); }
button.active { color: var(--sb-brand); border-color: var(--sb-brand); background: var(--primary-alpha-08); }
.usage-budget p { margin-top: 8px; }
.usage-toolbar { display: flex; justify-content: space-between; gap: 10px; align-items: center; flex-wrap: wrap; font-size: 12px; }
.usage-btn-group { display: flex; gap: 6px; flex-wrap: wrap; }
.usage-summary-strip { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px; }
.usage-stat { padding: 10px 12px; border: 1px solid var(--card-border); border-radius: 10px; background: var(--primary-alpha-05); display: flex; flex-direction: column; gap: 4px; font-variant-numeric: tabular-nums; }
.usage-stat span { font-size: 11px; color: var(--text-muted); }
.usage-stat strong { font-size: 15px; font-weight: 650; color: var(--text-primary); }
.usage-table { border: 1px solid var(--card-border); border-radius: 10px; overflow-x: auto; }
.usage-row { display: grid; grid-template-columns: minmax(190px, 1fr) repeat(4, minmax(84px, auto)); gap: 12px; padding: 11px 12px; align-items: center; font-size: 12px; font-variant-numeric: tabular-nums; }
.usage-row + .usage-row { border-top: 1px solid var(--card-border); }
.usage-name-cell { display: flex; align-items: center; gap: 6px; min-width: 0; }
.usage-name-cell strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.usage-session-link { padding: 0; border: 0; background: transparent; color: var(--text-primary); font-weight: 600; text-align: left; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 100%; }
.usage-session-link:hover { color: var(--sb-brand); text-decoration: underline; }
.usage-badge { flex-shrink: 0; padding: 1px 6px; border-radius: 999px; font-size: 10px; font-weight: 600; line-height: 1.5; }
.usage-badge--muted { color: var(--text-muted); background: var(--primary-alpha-08); }
.usage-badge--accent { color: var(--sb-brand); background: var(--primary-alpha-12); }
.usage-head { color: var(--text-muted); background: var(--primary-alpha-05); }
.usage-empty { padding: 30px; display: flex; justify-content: center; color: var(--text-muted); font-size: 12px; }
@media (max-width: 640px) {
  .usage-summary-strip { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
</style>
