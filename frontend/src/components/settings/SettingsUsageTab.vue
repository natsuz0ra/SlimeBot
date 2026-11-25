<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { usageAPI, type UsageGroup, type UsageSummary } from '@/api/usage'
import { settingAPI } from '@/api/settings'
import { useToast } from '@/composables/useToast'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'

const { t } = useI18n()
const toast = useToast()
const loading = ref(false)
const saving = ref(false)
const group = ref<UsageGroup>('session')
const rows = ref<UsageSummary[]>([])
const budget = ref(0)
const total = computed(() => rows.value.reduce((sum, row) => sum + row.outputTokens, 0))
function label(row: UsageSummary) { return row.groupKey || row.rootSessionId || row.model || row.workingDirectory || t('usageUnclassified') }
function number(value: number) { return new Intl.NumberFormat().format(value) }
async function load() {
  loading.value = true
  try { rows.value = await usageAPI.summary(group.value) }
  catch { toast.error(t('usageLoadFailed')) }
  finally { loading.value = false }
}
async function save() {
  saving.value = true
  try { await settingAPI.update({ sessionOutputTokenBudget: Math.max(0, Math.floor(budget.value)) }); toast.success(t('saveSuccess')) }
  catch { toast.error(t('usageBudgetSaveFailed')) }
  finally { saving.value = false }
}
watch(group, load)
onMounted(async () => {
  try { budget.value = (await settingAPI.get()).sessionOutputTokenBudget || 0 } catch { toast.error(t('usageLoadFailed')) }
  await load()
})
</script>

<template>
  <div class="usage-tab">
    <p class="usage-description">{{ t('usageDescription') }}</p>
    <section class="usage-budget"><label for="usage-budget">{{ t('usageBudget') }}</label><div><input id="usage-budget" v-model.number="budget" type="number" min="0" max="1000000000" step="1024" /><button type="button" :disabled="saving" @click="save">{{ t('save') }}</button></div><p>{{ t('usageBudgetHint') }}</p></section>
    <div class="usage-toolbar"><div role="group"><button v-for="item in ['session', 'model', 'project'] as UsageGroup[]" :key="item" type="button" :class="{ active: group === item }" @click="group = item">{{ t(`usageGroup_${item}`) }}</button></div><span>{{ t('usageOutputTotal', { count: number(total) }) }}</span></div>
    <div v-if="loading" class="usage-empty"><LoadingSpinner /></div>
    <div v-else-if="!rows.length" class="usage-empty">{{ t('usageEmpty') }}</div>
    <div v-else class="usage-table"><div class="usage-row usage-head"><span>{{ t(`usageGroup_${group}`) }}</span><span>{{ t('usageRequests') }}</span><span>{{ t('usageInput') }}</span><span>{{ t('usageOutput') }}</span><span>{{ t('usageCacheRead') }}</span></div><div v-for="row in rows" :key="row.groupKey" class="usage-row"><strong :title="label(row)">{{ label(row) }}</strong><span>{{ number(row.requestCount) }}</span><span>{{ number(row.inputTokens) }}</span><span>{{ number(row.outputTokens) }}</span><span>{{ number(row.cacheReadInputTokens) }}</span></div></div>
  </div>
</template>

<style scoped>
.usage-tab { display: flex; flex-direction: column; gap: 18px; color: var(--text-primary); }
.usage-description, .usage-budget p { margin: 0; font-size: 12px; color: var(--text-muted); line-height: 1.7; }
.usage-budget { padding: 16px; border: 1px solid var(--card-border); border-radius: 10px; background: var(--primary-alpha-05); }
.usage-budget label { display: block; font-size: 13px; font-weight: 600; margin-bottom: 10px; }
.usage-budget > div { display: flex; gap: 8px; }
.usage-budget input { flex: 1; min-width: 0; padding: 8px 10px; border: 1px solid var(--input-border); border-radius: 8px; background: var(--input-bg); color: var(--text-primary); }
button { padding: 7px 12px; border: 1px solid var(--input-border); border-radius: 8px; cursor: pointer; color: var(--text-secondary); background: var(--input-bg); }
button.active { color: var(--sb-brand); border-color: var(--sb-brand); }
.usage-budget p { margin-top: 8px; }
.usage-toolbar { display: flex; justify-content: space-between; gap: 10px; align-items: center; flex-wrap: wrap; font-size: 12px; }
.usage-toolbar > div { display: flex; gap: 6px; }
.usage-table { border: 1px solid var(--card-border); border-radius: 10px; overflow-x: auto; }
.usage-row { display: grid; grid-template-columns: minmax(180px, 1fr) repeat(4, minmax(90px, auto)); gap: 12px; padding: 12px; align-items: center; font-size: 12px; font-variant-numeric: tabular-nums; }
.usage-row + .usage-row { border-top: 1px solid var(--card-border); }
.usage-row strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.usage-head { color: var(--text-muted); background: var(--primary-alpha-05); }
.usage-empty { padding: 30px; display: flex; justify-content: center; color: var(--text-muted); font-size: 12px; }
</style>
