<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { mdiArrowCollapseDown, mdiArrowExpandUp, mdiBroom, mdiChatPlusOutline, mdiChevronDown, mdiChevronUp, mdiClose, mdiMagnify, mdiPlus, mdiRestart } from '@mdi/js'
import { Terminal, type ITheme } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { SearchAddon } from '@xterm/addon-search'
import { WebLinksAddon } from '@xterm/addon-web-links'
import '@xterm/xterm/css/xterm.css'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import { connectTerminal } from '@/api/terminal'
import { useTerminalStore } from '@/stores/terminal'
import { canStageTerminalCommand, formatTerminalOutputForChat } from '@/utils/terminalLink'

const props = defineProps<{ sessionId: string; cwd: string; dark: boolean; canSendOutput: boolean }>()
const emit = defineEmits<{ output: [text: string]; error: []; staged: [] }>()
const { t } = useI18n()
const store = useTerminalStore()
const canvas = ref<HTMLElement>()
const searchBox = ref<HTMLInputElement>()
const searching = ref(false)
const searchValue = ref('')
const selection = ref(false)
const selectedOutput = ref(false)
const reconnecting = ref(false)
const error = ref('')
const pending = ref<{ id: string; command: string }>()
let screen: Terminal | undefined
let fitAddon: FitAddon | undefined
let searchAddon: SearchAddon | undefined
let socket: ReturnType<typeof connectTerminal> | undefined
let observer: ResizeObserver | undefined
let ready = false
let dragY: number | undefined
let dragHeight = 0
let stageTimeout: ReturnType<typeof setTimeout> | undefined

const light: ITheme = {
  background: '#ffffff', foreground: '#18181b', cursor: '#4f46e5', selectionBackground: 'rgba(99,102,241,.20)',
  black: '#27272a', red: '#dc2626', green: '#15803d', yellow: '#b45309', blue: '#2563eb', magenta: '#a21caf',
  cyan: '#0e7490', white: '#52525b', brightBlack: '#71717a', brightRed: '#b91c1c', brightGreen: '#166534',
  brightYellow: '#92400e', brightBlue: '#1d4ed8', brightMagenta: '#86198f', brightCyan: '#155e75', brightWhite: '#18181b',
}
const dark: ITheme = {
  background: '#111113', foreground: '#e4e4e7', cursor: '#818cf8', selectionBackground: 'rgba(129,140,248,.28)',
  black: '#27272a', red: '#f87171', green: '#4ade80', yellow: '#fbbf24', blue: '#60a5fa', magenta: '#e879f9',
  cyan: '#22d3ee', white: '#d4d4d8', brightBlack: '#a1a1aa', brightRed: '#fca5a5', brightGreen: '#86efac',
  brightYellow: '#fcd34d', brightBlue: '#93c5fd', brightMagenta: '#f0abfc', brightCyan: '#67e8f9', brightWhite: '#fafafa',
}
const theme = computed(() => props.dark ? dark : light)

function fit() {
  fitAddon?.fit()
  if (ready && screen) socket?.resize(screen.cols, screen.rows)
}

function dispose() {
  ready = false
  observer?.disconnect()
  socket?.close()
  screen?.dispose()
  observer = socket = screen = fitAddon = searchAddon = undefined
}

function stage(id: string, command: string) {
  clearTimeout(stageTimeout)
  pending.value = { id, command }
  stageTimeout = setTimeout(() => {
    if (pending.value?.id === id) {
      pending.value = undefined
      error.value = t('terminalStageUnavailable')
    }
  }, 5000)
  flushStagedCommand()
}

function flushStagedCommand() {
  const draft = pending.value
  if (!draft || !ready || !screen || draft.id !== store.activeTabId) return
  const bracketed = screen.modes.bracketedPasteMode
  if (draft.command.includes('\n') && !bracketed) return
  clearTimeout(stageTimeout)
  pending.value = undefined
  if (!canStageTerminalCommand(draft.command, bracketed)) {
    error.value = t('terminalUnsafeCommand')
    return
  }
  const text = bracketed ? `\x1b[200~${draft.command.replace(/\n/g, '\r')}\x1b[201~` : draft.command
  if (!socket?.input(text)) {
    error.value = t('terminalStageUnavailable')
    return
  }
  screen.focus()
  emit('staged')
}

defineExpose({ stage })

watch(() => [store.isOpen, store.activeTabId, store.enabled] as const, ([open, id, enabled]) => {
  dispose()
  selection.value = selectedOutput.value = false
  reconnecting.value = false
  error.value = ''
  if (pending.value && (!open || pending.value.id !== id)) pending.value = undefined
  if (!open || !id || !enabled || !canvas.value) return
  const terminal = new Terminal({
    theme: theme.value,
    fontFamily: getComputedStyle(canvas.value).getPropertyValue('--font-mono').trim(),
    fontSize: 13,
    cursorBlink: true,
    scrollback: 2000,
    allowProposedApi: false,
  })
  screen = terminal
  fitAddon = new FitAddon()
  searchAddon = new SearchAddon()
  terminal.loadAddon(fitAddon)
  terminal.loadAddon(searchAddon)
  terminal.loadAddon(new WebLinksAddon())
  terminal.open(canvas.value)
  terminal.onData(data => { if (ready) socket?.input(data) })
  terminal.onSelectionChange(() => { selection.value = terminal.hasSelection() })
  terminal.onWriteParsed(() => {
    selectedOutput.value = terminal.buffer.active.baseY > 0 || Boolean(recentOutput(1))
  })
  observer = new ResizeObserver(fit)
  observer.observe(canvas.value)
  fit()
  reconnecting.value = store.activeTab?.status === 'running'
  socket = connectTerminal(id, {
    onData: data => terminal.write(data, flushStagedCommand),
    onFrame(frame) {
      switch (frame.type) {
        case 'ready':
          terminal.reset()
          store.update(frame.session)
          ready = frame.session.status === 'running'
          reconnecting.value = false
          fit()
          flushStagedCommand()
          if (ready) terminal.focus()
          break
        case 'exit': {
          const tab = store.activeTab
          if (tab?.id === id) store.update({ ...tab, status: 'exited', exitCode: frame.exitCode })
          ready = false
          reconnecting.value = false
          break
        }
        case 'error':
          error.value = frame.message
          break
      }
    },
    onReconnect(attempt) {
      ready = false
      reconnecting.value = true
      if (attempt === 3) void store.refresh().catch(() => {})
    },
  })
}, { flush: 'post' })

watch(theme, value => { if (screen) screen.options.theme = value })
onBeforeUnmount(() => {
  clearTimeout(stageTimeout)
  dispose()
})

function recentOutput(limit = 80) {
  const buffer = screen?.buffer.active
  if (!buffer) return ''
  const lines: string[] = []
  for (let index = buffer.length - 1; index >= 0 && lines.length < limit; index--) {
    const line = buffer.getLine(index)?.translateToString(true) ?? ''
    if (line.trim()) lines.push(line)
  }
  return lines.reverse().join('\n')
}

function sendOutput() {
  if (!store.activeTab || !props.canSendOutput) return
  const output = screen?.getSelection() || recentOutput()
  if (!output.trim()) return
  emit('output', formatTerminalOutputForChat(output, store.activeTab.cwd, store.activeTab.exitCode))
}

async function newTab(restart = false) {
  if (store.busy) return
  store.busy = true
  error.value = ''
  try {
    const old = restart ? store.activeTab : undefined
    const options = old ? { chatSessionId: old.chatSessionId, cwd: old.cwd, shell: old.shell, cols: old.cols, rows: old.rows }
      : { chatSessionId: props.sessionId, cwd: props.cwd }
    if (old) await store.close(old.id)
    await store.create(options)
  } catch {
    emit('error')
  } finally {
    store.busy = false
  }
}

async function closeTab(id: string) {
  try { await store.close(id) } catch { emit('error') }
}

async function toggleSearch() {
  searching.value = !searching.value
  await nextTick()
  if (searching.value) searchBox.value?.focus()
  else screen?.focus()
}

function search(previous = false, incremental = false) {
  if (previous) searchAddon?.findPrevious(searchValue.value)
  else searchAddon?.findNext(searchValue.value, { incremental })
}

function startDrag(event: PointerEvent) {
  dragY = event.clientY
  dragHeight = store.heightPx
  ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
}

function drag(event: PointerEvent) {
  if (dragY !== undefined) store.setHeight(dragHeight + dragY - event.clientY)
}
</script>

<template>
  <section
    v-show="store.isOpen"
    class="terminal-drawer"
    :class="{ maximized: store.isMaximized }"
    :style="{ '--terminal-height': `${store.heightPx}px`, '--terminal-bg': theme.background }"
    :aria-label="t('terminalTitle')"
  >
    <div
      v-if="!store.isMaximized" class="terminal-handle" role="separator" tabindex="0"
      :aria-label="t('terminalResize')" aria-orientation="horizontal"
      @pointerdown.prevent="startDrag" @pointermove="drag" @lostpointercapture="dragY = undefined"
      @keydown.up.prevent="store.setHeight(store.heightPx + 16)" @keydown.down.prevent="store.setHeight(store.heightPx - 16)"
    />
    <header class="terminal-bar">
      <div class="terminal-tabs" role="tablist" :aria-label="t('terminalTitle')">
        <div v-for="tab in store.tabs" :key="tab.id" class="terminal-tab" :class="{ active: tab.id === store.activeTabId }">
          <button type="button" role="tab" :aria-selected="tab.id === store.activeTabId" :title="tab.cwd" @click="store.activeTabId = tab.id">
            <span class="terminal-dot" :class="tab.status === 'running' ? (tab.id === store.activeTabId && reconnecting ? 'reconnecting' : 'running') : tab.exitCode ? 'failed' : ''" />
            <span class="truncate">{{ tab.title }}</span>
          </button>
          <button type="button" class="terminal-icon" :title="t('terminalCloseTab')" :aria-label="t('terminalCloseTab')" @click="closeTab(tab.id)"><MdiIcon :path="mdiClose" :size="14" /></button>
        </div>
        <button type="button" class="terminal-icon" :disabled="!store.enabled || store.busy" :title="t('terminalNew')" :aria-label="t('terminalNew')" @click="newTab()"><MdiIcon :path="mdiPlus" :size="17" /></button>
      </div>
      <div class="terminal-tools">
        <button type="button" class="terminal-icon terminal-send" :class="{ selected: selection }" :disabled="!(selection || selectedOutput) || !canSendOutput" :title="t(selection ? 'terminalSendSelection' : 'terminalSendOutput')" @click="sendOutput"><MdiIcon :path="mdiChatPlusOutline" :size="16" /><span>{{ t(selection ? 'terminalSendSelection' : 'terminalSendOutput') }}</span></button>
        <details v-if="store.activeTab" class="terminal-extra">
          <summary class="terminal-icon" :aria-label="t('terminalActions')">…</summary>
          <div class="terminal-extra-menu">
            <button type="button" class="terminal-icon" :title="t('terminalSearch')" :aria-label="t('terminalSearch')" @click="toggleSearch"><MdiIcon :path="mdiMagnify" :size="16" /></button>
            <button type="button" class="terminal-icon" :title="t('terminalClear')" :aria-label="t('terminalClear')" @click="screen?.clear()"><MdiIcon :path="mdiBroom" :size="16" /></button>
            <button type="button" class="terminal-icon" :disabled="store.busy" :title="t('terminalRestart')" :aria-label="t('terminalRestart')" @click="newTab(true)"><MdiIcon :path="mdiRestart" :size="16" /></button>
          </div>
        </details>
        <button type="button" class="terminal-icon" :aria-pressed="store.isMaximized" :title="t('terminalMaximize')" :aria-label="t('terminalMaximize')" @click="store.isMaximized = !store.isMaximized"><MdiIcon :path="store.isMaximized ? mdiArrowCollapseDown : mdiArrowExpandUp" :size="16" /></button>
        <button type="button" class="terminal-icon" :title="t('terminalHide')" :aria-label="t('terminalHide')" @click="store.isOpen = false"><MdiIcon :path="mdiClose" :size="16" /></button>
      </div>
    </header>
    <div v-if="!store.enabled" class="terminal-empty">
      {{ t('terminalDisabled') }}
      <code v-if="store.lockedByEnv">SLIMEBOT_TERMINAL_ENABLED=false</code>
      <span v-else>{{ t('terminalEnableInSettings') }}</span>
    </div>
    <div v-else-if="!store.activeTab" class="terminal-empty">{{ t(store.busy ? 'loading' : 'terminalEmpty') }}</div>
    <div v-else class="terminal-viewport">
      <div ref="canvas" class="terminal-canvas" />
      <div v-if="searching" class="terminal-search" @keydown.esc.stop.prevent="toggleSearch">
        <input ref="searchBox" v-model="searchValue" :aria-label="t('terminalSearch')" :placeholder="t('terminalSearch')" @input="search(false, true)" @keydown.enter.stop.prevent="search($event.shiftKey)" />
        <button type="button" class="terminal-icon" :aria-label="t('terminalPrevious')" @click="search(true)"><MdiIcon :path="mdiChevronUp" :size="16" /></button>
        <button type="button" class="terminal-icon" :aria-label="t('terminalNext')" @click="search()"><MdiIcon :path="mdiChevronDown" :size="16" /></button>
        <button type="button" class="terminal-icon" :aria-label="t('terminalHide')" @click="toggleSearch"><MdiIcon :path="mdiClose" :size="16" /></button>
      </div>
    </div>
    <div v-if="error || reconnecting || store.activeTab?.status === 'exited'" class="terminal-status" role="status">
      {{ error || (reconnecting ? t('terminalReconnecting') : t('terminalExited', { code: store.activeTab?.exitCode ?? '-' })) }}
      <button v-if="store.activeTab?.status === 'exited'" type="button" class="terminal-icon" :disabled="store.busy" @click="newTab(true)">{{ t('terminalRestart') }}</button>
    </div>
  </section>
</template>

<style scoped>
.terminal-drawer { position: relative; display: flex; flex: 0 0 min(var(--terminal-height), 75dvh); min-height: 0; flex-direction: column; border-top: 1px solid var(--border-default); background: var(--bg-base); }
.terminal-drawer.maximized { flex: 1; }
.terminal-handle { position: absolute; z-index: 2; top: -3px; width: 100%; height: 5px; cursor: row-resize; touch-action: none; }
.terminal-handle:hover, .terminal-handle:focus-visible { border-bottom: 2px solid var(--sb-brand); }
.terminal-bar, .terminal-tools, .terminal-tabs, .terminal-tab, .terminal-tab > button:first-child, .terminal-extra-menu { display: flex; align-items: center; gap: 4px; min-width: 0; }
.terminal-bar { flex: 0 0 36px; justify-content: space-between; padding: 0 8px; border-bottom: 1px solid var(--border-subtle); }
.terminal-tabs { overflow: auto; scrollbar-width: none; }
.terminal-tab { height: 28px; flex: 0 0 auto; max-width: 180px; padding-left: 8px; border: 1px solid transparent; border-radius: 6px; color: var(--text-secondary); font: 12px var(--font-mono); }
.terminal-tab.active { border-color: var(--border-default); background: var(--bg-main); color: var(--text-primary); }
.terminal-tab:hover, .terminal-icon:hover { background: var(--bg-hover); }
.terminal-tab > button { cursor: pointer; }
.terminal-dot { width: 6px; height: 6px; flex: 0 0 6px; border-radius: 50%; background: var(--text-muted); }
.terminal-dot.running { background: var(--tool-success-dot); }
.terminal-dot.failed { background: var(--tool-error-dot); }
.terminal-dot.reconnecting { background: var(--tool-pending-dot); }
.terminal-tools { flex-shrink: 0; }
.terminal-icon { display: inline-flex; min-width: 26px; height: 26px; flex: 0 0 auto; align-items: center; justify-content: center; gap: 4px; padding: 0 4px; border-radius: 6px; color: var(--text-secondary); font-size: 12px; cursor: pointer; }
.terminal-icon:disabled { opacity: .45; cursor: default; }
.terminal-icon:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: -2px; }
.terminal-send.selected { background: var(--primary-alpha-10); color: var(--sb-brand); }
.terminal-extra { position: relative; }
.terminal-extra summary { display: none; }
.terminal-extra:not([open]) .terminal-extra-menu { display: flex; }
.terminal-viewport { position: relative; flex: 1; min-height: 0; padding: 8px 10px 6px; background: var(--terminal-bg); }
.terminal-canvas { height: 100%; }
.terminal-search { position: absolute; z-index: 3; right: 16px; top: 6px; display: flex; padding: 4px; gap: 2px; border: 1px solid var(--border-default); border-radius: 8px; background: var(--bg-elevated); }
.terminal-search input { width: min(36vw, 170px); padding: 0 6px; font-size: 12px; }
.terminal-empty { flex: 1; display: grid; align-content: center; justify-items: center; gap: 6px; color: var(--text-secondary); font-size: 13px; }
.terminal-status { display: flex; align-items: center; gap: 8px; min-height: 30px; padding: 0 10px; background: var(--bg-subtle); border-top: 1px solid var(--border-subtle); color: var(--text-secondary); font-size: 12px; }
@media (max-width: 767px) {
  .terminal-drawer { flex-basis: 45dvh; }
  .terminal-handle, .terminal-send span { display: none; }
  .terminal-extra summary { display: inline-flex; }
  .terminal-extra:not([open]) .terminal-extra-menu { display: none; }
  .terminal-extra-menu { position: absolute; z-index: 4; right: 0; top: 30px; padding: 4px; border: 1px solid var(--menu-border); border-radius: 8px; background: var(--menu-bg); }
}
</style>
