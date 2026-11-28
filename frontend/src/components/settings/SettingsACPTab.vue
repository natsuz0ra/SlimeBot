<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Copy, Plus } from 'lucide-vue-next'
import { acpAPI, type ACPConfig, type ACPConfigInput, type ACPConnectionTest } from '@/api/acp'
import AppDialog from '@/components/ui/AppDialog.vue'
import AppTextInput from '@/components/ui/AppTextInput.vue'
import ToggleSwitch from '@/components/ui/ToggleSwitch.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import { useToast } from '@/composables/useToast'
import { parseACPArguments } from '@/utils/acp'

const emit = defineEmits<{ changed: [] }>()
const { t } = useI18n()
const toast = useToast()
const launchCommand = 'slimebot acp'
const example = JSON.stringify({ agent_servers: { SlimeBot: { type: 'custom', command: 'slimebot', args: ['acp'], env: {} } } }, null, 2)
const items = ref<ACPConfig[]>([])
const loading = ref(true)
const listError = ref(false)
const results = reactive(new Map<string, ACPConnectionTest>())
const testing = reactive(new Set<string>())
const changing = reactive(new Set<string>())
const deleting = ref<ACPConfig>()
const saving = ref(false)
const dialogError = ref('')
let environmentRowId = 0

interface EnvironmentRow { id: number; key: string; value: string; storedKey?: string }
interface Draft { agent?: ACPConfig; name: string; command: string; args: string; cwd: string; timeout: number; env: EnvironmentRow[] }
const draft = ref<Draft>()
const formTitle = computed(() => t(draft.value?.agent ? 'acpEdit' : 'acpAdd'))

async function load() {
  loading.value = true
  listError.value = false
  try { items.value = await acpAPI.list() }
  catch { listError.value = true }
  finally { loading.value = false }
}
onMounted(load)

async function copy(value: string) {
  try {
    await navigator.clipboard.writeText(value)
    toast.success(t('messageCopySuccess'))
  } catch { toast.error(t('messageCopyFailed')) }
}

function edit(agent?: ACPConfig) {
  dialogError.value = ''
  draft.value = {
    agent, name: agent?.name || '', command: agent?.command || '',
    args: agent?.args?.length ? JSON.stringify(agent.args, null, 2) : '',
    cwd: agent?.defaultCwd || '', timeout: agent?.timeoutSec ?? 300,
    env: (agent?.envKeys ?? []).map(key => ({ id: ++environmentRowId, key, value: '', storedKey: key })),
  }
}

function addEnvironmentRow() {
  draft.value?.env.push({ id: ++environmentRowId, key: '', value: '' })
}

async function save() {
  const row = draft.value
  if (!row || saving.value) return
  dialogError.value = ''
  if (!row.name.trim() || !row.command.trim() || !Number.isInteger(row.timeout) || row.timeout < 1 || row.timeout > 3600) {
    dialogError.value = t('acpRequiredFields')
    return
  }
  let args: string[]
  try { args = parseACPArguments(row.args) }
  catch { dialogError.value = t('acpInvalidArgs'); return }
  const changes = new Map<string, string>()
  const seen = new Set<string>()
  for (const entry of row.env) {
    const key = entry.key.trim()
    if (!key && !entry.value && !entry.storedKey) continue
    if (!/^[A-Za-z_][A-Za-z0-9_]{0,127}$/.test(key) || seen.has(key)) {
      dialogError.value = t('acpInvalidEnv')
      return
    }
    if (entry.value || key !== entry.storedKey) changes.set(key, entry.value)
    seen.add(key)
  }
  const input: ACPConfigInput = {
    name: row.name.trim(), command: row.command.trim(), args, defaultCwd: row.cwd.trim(), timeoutSec: row.timeout,
    env: Object.fromEntries(changes), removeEnvKeys: row.agent?.envKeys?.filter(key => !seen.has(key)),
  }
  saving.value = true
  try {
    const config = row.agent ? await acpAPI.update(row.agent.id, input) : await acpAPI.create(input)
    if (row.agent) {
      items.value = items.value.map(agent => agent.id === config.id ? config : agent)
      results.delete(config.id)
    } else items.value = [...items.value, config]
    draft.value = undefined
    emit('changed')
  } catch { dialogError.value = t('acpSaveFailed') }
  finally { saving.value = false }
}

async function toggle(agent: ACPConfig, enabled: boolean) {
  changing.add(agent.id)
  try {
    const updated = await acpAPI.update(agent.id, { isEnabled: enabled })
    items.value = items.value.map(item => item.id === agent.id ? updated : item)
    emit('changed')
  } catch { toast.error(t('acpSaveFailed')) }
  finally { changing.delete(agent.id) }
}

async function testAgent(agent: ACPConfig) {
  testing.add(agent.id)
  try { results.set(agent.id, await acpAPI.test(agent.id)) }
  catch { results.set(agent.id, { ok: false, latencyMs: 0, error: t('acpTestFailed') }) }
  finally { testing.delete(agent.id) }
}

function showDelete(agent: ACPConfig) {
  deleting.value = agent
  dialogError.value = ''
}

async function remove() {
  const agent = deleting.value
  if (!agent || saving.value) return
  saving.value = true
  try {
    await acpAPI.remove(agent.id)
    items.value = items.value.filter(item => item.id !== agent.id)
    results.delete(agent.id)
    deleting.value = undefined
    emit('changed')
  } catch { dialogError.value = t('acpDeleteFailed') }
  finally { saving.value = false }
}

function capabilities(result?: ACPConnectionTest) {
  const value = result?.capabilities
  return [
    value?.loadSession && t('acpLoadSession'),
    value?.promptCapabilities?.image && t('acpImages'),
    value?.promptCapabilities?.embeddedContext && t('acpEmbedded'),
    value?.promptCapabilities?.audio && t('acpAudio'),
  ].filter(Boolean).join(' · ')
}
</script>

<template>
  <div class="space-y-3">
    <section class="settings-card p-4 rounded-xl">
      <h3 class="settings-field-label">{{ t('acpLocalAgent') }}</h3>
      <p class="settings-item-sub mt-1">{{ t('acpLocalDescription') }}</p>
      <div class="acp-code-bar mt-3"><code>{{ launchCommand }}</code><button type="button" :aria-label="t('acpCopyCommand')" :title="t('acpCopyCommand')" @click="copy(launchCommand)"><Copy :size="16" :stroke-width="1.75" aria-hidden="true" /></button></div>
      <details class="mt-3 text-sm sb-text-secondary">
        <summary class="cursor-pointer">{{ t('acpClientExample') }}</summary>
        <div class="acp-code-bar mt-2"><pre>{{ example }}</pre><button type="button" :aria-label="t('acpCopyExample')" :title="t('acpCopyExample')" @click="copy(example)"><Copy :size="16" :stroke-width="1.75" aria-hidden="true" /></button></div>
      </details>
    </section>

    <section class="settings-card p-4 rounded-xl">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <h3 class="settings-field-label">{{ t('acpExternalAgents') }}</h3>
        <button type="button" class="acp-primary inline-flex items-center gap-1.5" :disabled="loading || listError" @click="edit()"><Plus :size="15" :stroke-width="1.75" aria-hidden="true" />{{ t('acpAdd') }}</button>
      </div>
      <p v-if="loading" class="settings-item-sub mt-3">{{ t('loading') }}</p>
      <p v-else-if="listError" class="settings-item-sub mt-3" role="alert">{{ t('acpLoadFailed') }} <button type="button" class="acp-action" @click="load">{{ t('templateRetry') }}</button></p>
      <p v-else-if="!items.length" class="settings-item-sub mt-3">{{ t('acpEmpty') }}</p>
      <ul v-else class="mt-2 divide-y divide-[var(--border-subtle)]">
        <li v-for="item in items" :key="item.id" class="py-3">
          <div class="flex flex-wrap items-center gap-2">
            <h4 class="text-sm font-semibold sb-text-primary min-w-0 break-all">{{ item.name }}</h4>
            <span class="acp-test-status" :class="{ failed: results.get(item.id)?.ok === false, passed: results.get(item.id)?.ok }">
              {{ testing.has(item.id) ? t('acpTesting') : results.get(item.id)?.ok
                ? t('acpConnected', { ms: results.get(item.id)?.latencyMs, version: results.get(item.id)?.protocolVersion })
                : results.has(item.id) ? t('acpDisconnected') : t('acpUntested') }}
            </span>
            <ToggleSwitch :model-value="item.isEnabled" :disabled="changing.has(item.id) || testing.has(item.id)" role="switch" :aria-checked="item.isEnabled" :aria-label="t('acpToggle', { name: item.name })" class="ml-auto" @update:model-value="toggle(item, $event)" />
          </div>
          <div class="acp-command mt-2 truncate" :title="[item.command, ...item.args].join(' ')">{{ [item.command, ...item.args].join(' ') }}</div>
          <p v-if="item.defaultCwd" class="settings-item-sub truncate" :title="item.defaultCwd">{{ item.defaultCwd }}</p>
          <p v-if="results.get(item.id)?.ok" class="settings-item-sub mt-1" role="status">
            {{ [results.get(item.id)?.agentInfo?.title || results.get(item.id)?.agentInfo?.name, capabilities(results.get(item.id))].filter(Boolean).join(' · ') }}
          </p>
          <p v-else-if="results.get(item.id)?.error" class="acp-error mt-1 break-words" role="alert">{{ results.get(item.id)?.error }}</p>
          <div class="flex gap-3 items-center mt-2">
            <button type="button" class="acp-action" :disabled="testing.has(item.id) || changing.has(item.id)" @click="testAgent(item)"><LoadingSpinner v-if="testing.has(item.id)" size-class="w-3 h-3" />{{ t('acpTest') }}</button>
            <button type="button" class="acp-action" :disabled="testing.has(item.id) || changing.has(item.id)" @click="edit(item)">{{ t('edit') }}</button>
            <button type="button" class="acp-action acp-error" :disabled="testing.has(item.id) || changing.has(item.id)" @click="showDelete(item)">{{ t('delete') }}</button>
          </div>
        </li>
      </ul>
    </section>
  </div>

  <AppDialog v-if="draft" :visible="true" :title="formTitle" width="520px" :confirm-text="t('save')" :confirm-loading="saving" :close-on-esc="!saving" :close-on-mask="!saving" :show-close="!saving" :show-cancel="!saving" @confirm="save" @cancel="draft = undefined">
    <div class="acp-form">
      <label for="acp-name">{{ t('acpName') }}</label><AppTextInput id="acp-name" v-model="draft.name" maxlength="128" />
      <label for="acp-command">{{ t('acpCommand') }}</label><AppTextInput id="acp-command" v-model="draft.command" maxlength="512" placeholder="npx" spellcheck="false" />
      <label for="acp-args">{{ t('acpArguments') }}</label><textarea id="acp-args" v-model="draft.args" rows="3" :placeholder="t('acpArgumentsHint')" spellcheck="false" />
      <label for="acp-cwd">{{ t('acpDefaultCwd') }}</label><AppTextInput id="acp-cwd" v-model="draft.cwd" maxlength="2048" spellcheck="false" />
      <label for="acp-timeout">{{ t('acpTimeout') }}</label><input id="acp-timeout" v-model.number="draft.timeout" type="number" min="1" max="3600" />
      <div class="flex items-center justify-between gap-2 mt-2"><span>{{ t('acpEnv') }}</span><button type="button" class="acp-action" @click="addEnvironmentRow">{{ t('acpAddEnv') }}</button></div>
      <div v-for="entry in draft.env" :key="entry.id" class="acp-env-row">
        <AppTextInput v-model="entry.key" :aria-label="t('acpEnvKey')" :placeholder="t('acpEnvKey')" spellcheck="false" />
        <AppTextInput v-model="entry.value" type="password" autocomplete="new-password" :aria-label="t('acpEnvValue')" :placeholder="t(entry.storedKey ? 'acpEnvStored' : 'acpEnvValue')" />
        <button type="button" class="acp-action acp-error" @click="draft.env = draft.env.filter(row => row.id !== entry.id)">{{ t('delete') }}</button>
      </div>
      <p v-if="dialogError" class="acp-error" role="alert">{{ dialogError }}</p>
    </div>
  </AppDialog>
  <AppDialog v-if="deleting" :visible="true" :title="t('acpDelete')" :confirm-text="t('delete')" confirm-danger :confirm-loading="saving" :close-on-esc="!saving" :close-on-mask="!saving" :show-close="!saving" :show-cancel="!saving" @confirm="remove" @cancel="deleting = undefined">
    <p class="sb-text-primary text-sm">{{ t('acpDeleteQuestion', { name: deleting.name }) }}</p>
    <p v-if="dialogError" class="acp-error mt-2" role="alert">{{ dialogError }}</p>
  </AppDialog>
</template>

<style scoped>
.acp-code-bar { display: flex; align-items: flex-start; justify-content: space-between; gap: 8px; padding: 8px 10px; border: 1px solid var(--border-subtle); border-radius: 8px; background: var(--bg-subtle); color: var(--text-primary); font: 12px var(--font-mono); }
.acp-code-bar pre { margin: 0; min-width: 0; overflow: auto; font: inherit; }
.acp-code-bar button { display: grid; flex: 0 0 26px; height: 26px; place-items: center; border-radius: 6px; color: var(--text-secondary); cursor: pointer; }
.acp-code-bar button:hover { background: var(--bg-hover); }
.acp-code-bar code { align-self: center; }
.acp-primary { border-radius: 8px; background: var(--color-primary); color: white; padding: 6px 10px; font-size: 13px; cursor: pointer; }
.acp-primary:hover { background: var(--color-primary-hover); }
.acp-primary:disabled, .acp-action:disabled { opacity: .5; cursor: default; }
.acp-action { display: inline-flex; align-items: center; gap: 4px; color: var(--sb-brand); font-size: 12px; cursor: pointer; }
.acp-action:hover { text-decoration: underline; }
.acp-error { color: var(--color-danger); font-size: 12px; }
.acp-command { color: var(--text-secondary); font: 12px var(--font-mono); }
.acp-test-status { padding: 2px 5px; border: 1px solid var(--border-subtle); border-radius: 6px; font-size: 11px; color: var(--text-secondary); background: var(--bg-subtle); }
.acp-test-status.passed { color: var(--tool-success-text); border-color: var(--tool-success-border); }
.acp-test-status.failed { color: var(--tool-error-text); border-color: var(--tool-error-border); }
.acp-form { display: grid; gap: 6px; color: var(--text-primary); font-size: 13px; }
.acp-form label:not(:first-child) { margin-top: 6px; }
.acp-form textarea, .acp-form input[type="number"] { min-width: 0; padding: 7px 9px; border: 1px solid var(--border-default); border-radius: 8px; background: var(--bg-subtle); outline-color: var(--focus-ring); }
.acp-form textarea { resize: vertical; font-family: var(--font-mono); }
.acp-env-row { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1.4fr) auto; align-items: center; gap: 6px; }
</style>
