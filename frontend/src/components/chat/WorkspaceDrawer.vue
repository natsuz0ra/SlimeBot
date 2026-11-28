<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import hljs from 'highlight.js/lib/common'
import { mdiChevronRight, mdiClose, mdiContentCopy, mdiEyeOffOutline, mdiEyeOutline, mdiFileOutline, mdiFolderOutline, mdiRefresh, mdiArrowCollapseUp, mdiChatPlusOutline, mdiSourceBranch } from '@mdi/js'
import { workspaceAPI, type WorkspaceFile, type WorkspaceGitDiff, type WorkspaceGitStatus, type WorkspaceTreeEntry } from '@/api/workspace'
import CheckpointDiff from '@/components/chat/CheckpointDiff.vue'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import { formatSize } from '@/utils/format'
import { modalFocusableElements, ownsModalKeyboard } from '@/utils/dialogFocus'
import { useToast } from '@/composables/useToast'

const props = defineProps<{
  sessionId: string; directory: string; status?: WorkspaceGitStatus; statusLoading: boolean
  statusFailed: boolean; tab: 'git' | 'files'; canQuote: boolean; modal: boolean
}>()
const emit = defineEmits<{ close: []; refresh: []; quote: [path: string]; 'update:tab': [tab: 'git' | 'files'] }>()
const { t } = useI18n()
const toast = useToast()
const panel = ref<HTMLElement>()
const width = ref(420)
const hidden = ref(false)
const folders = reactive(new Map<string, WorkspaceTreeEntry[]>())
const expanded = reactive(new Set<string>())
const loading = reactive(new Set<string>())
const folderErrors = reactive(new Set<string>())
const folderRequests = new Map<string, AbortController>()
const file = ref<WorkspaceFile>()
const diff = ref<WorkspaceGitDiff>()
const activePath = ref('')
const activeChange = ref('')
const previewBusy = ref(false)
const previewFailed = ref(false)
let previewRequest: AbortController | undefined
let dragStart = 0
let dragWidth = 0
let dragging = false

const directoryName = computed(() => props.directory.split(/[\\/]/).filter(Boolean).pop() || props.directory)
const gitGroups = computed(() => {
  const changes = props.status?.changes ?? []
  return [
    { key: 'staged', items: changes.filter(item => !!item.staged).map(item => ({ ...item, code: item.staged, staged: true })) },
    { key: 'changes', items: changes.filter(item => !!item.unstaged).map(item => ({ ...item, code: item.unstaged, staged: false })) },
    { key: 'untracked', items: changes.filter(item => item.untracked).map(item => ({ ...item, code: '?', staged: false })) },
  ].filter(group => group.items.length)
})

const visibleEntries = computed(() => {
  const result: { entry: WorkspaceTreeEntry; depth: number }[] = []
  function visit(path: string, depth: number) {
    for (const entry of folders.get(path) ?? []) {
      result.push({ entry, depth })
      if (expanded.has(entry.path)) visit(entry.path, depth + 1)
    }
  }
  visit('', 0)
  return result
})

const fileText = computed(() => (file.value?.content ?? '').replace(/\r\n/g, '\n'))
const fileLines = computed(() => Array.from({ length: fileText.value.split('\n').length }, (_, index) => index + 1).join('\n'))
const highlightedCode = computed(() => {
  const language = file.value?.language
  if (!language || !hljs.getLanguage(language)) return undefined
  return hljs.highlight(fileText.value, { language, ignoreIllegals: true }).value
})

function clearFiles() {
  for (const request of folderRequests.values()) request.abort()
  folderRequests.clear()
  folders.clear()
  expanded.clear()
  loading.clear()
  folderErrors.clear()
}

function clearPreview() {
  previewRequest?.abort()
  previewRequest = undefined
  activeChange.value = activePath.value = ''
  previewBusy.value = previewFailed.value = false
  file.value = diff.value = undefined
}

async function loadFolder(path: string) {
  if (folderRequests.has(path)) return
  const request = new AbortController()
  folderRequests.set(path, request)
  loading.add(path)
  folderErrors.delete(path)
  try {
    const items = await workspaceAPI.tree(props.sessionId, path, hidden.value, request.signal)
    if (!request.signal.aborted) folders.set(path, items)
  } catch {
    if (!request.signal.aborted) folderErrors.add(path)
  } finally {
    if (folderRequests.get(path) === request) {
      folderRequests.delete(path)
      loading.delete(path)
    }
  }
}

function toggleFolder(path: string) {
  if (expanded.delete(path)) return
  expanded.add(path)
  if (!folders.has(path)) void loadFolder(path)
}

async function selectFile(path: string) {
  clearPreview()
  activePath.value = path
  const request = new AbortController()
  previewRequest = request
  previewBusy.value = true
  try {
    const content = await workspaceAPI.file(props.sessionId, path, request.signal)
    if (!request.signal.aborted) file.value = content
  } catch {
    if (!request.signal.aborted) previewFailed.value = true
  } finally {
    if (!request.signal.aborted) previewBusy.value = false
  }
}

async function selectChange(path: string, staged: boolean) {
  clearPreview()
  activeChange.value = `${staged}:${path}`
  const request = new AbortController()
  previewRequest = request
  previewBusy.value = true
  try {
    const content = await workspaceAPI.diff(props.sessionId, path, staged, request.signal)
    if (!request.signal.aborted) diff.value = content
  } catch {
    if (!request.signal.aborted) previewFailed.value = true
  } finally {
    if (!request.signal.aborted) previewBusy.value = false
  }
}

function refresh() {
  emit('refresh')
  clearFiles()
  if (props.tab === 'files') {
    void loadFolder('')
    if (activePath.value) void selectFile(activePath.value)
  } else if (activeChange.value) {
    const [staged, ...path] = activeChange.value.split(':')
    void selectChange(path.join(':'), staged === 'true')
  }
}

watch(() => props.sessionId, () => {
  clearFiles()
  clearPreview()
  if (props.tab === 'files') void loadFolder('')
}, { immediate: true })
watch(() => props.tab, tab => {
  clearPreview()
  if (tab === 'files' && !folders.has('')) void loadFolder('')
})
watch(hidden, () => {
  clearFiles()
  clearPreview()
  void loadFolder('')
})
watch(() => panel.value, async () => {
  await nextTick()
  panel.value?.focus()
}, { flush: 'post' })
onBeforeUnmount(() => { clearFiles(); clearPreview() })

async function copyPath(path: string) {
  try {
    await navigator.clipboard.writeText(path)
    toast.success(t('messageCopySuccess'))
  } catch { toast.error(t('messageCopyFailed')) }
}

function resize(value: number) {
  width.value = Math.min(640, Math.max(320, value))
}
function startResize(event: PointerEvent) {
  dragStart = event.clientX
  dragWidth = width.value
  dragging = true
  ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
}
function drag(event: PointerEvent) {
  if (dragging) resize(dragWidth + dragStart - event.clientX)
}
function onKey(event: KeyboardEvent) {
  if (props.modal && !ownsModalKeyboard(panel.value)) return
  if (event.key === 'Escape') {
    event.preventDefault()
    event.stopPropagation()
    emit('close')
  }
  if (event.key !== 'Tab' || !panel.value || !props.modal) return
  const elements = modalFocusableElements(panel.value)
  const first = elements[0]
  const last = elements[elements.length - 1]
  if (event.shiftKey && (document.activeElement === first || document.activeElement === panel.value)) {
    event.preventDefault()
    last?.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first?.focus()
  }
}
</script>

<template>
  <aside ref="panel" tabindex="-1" class="workspace-drawer" :style="{ '--workspace-width': `${width}px` }" :aria-label="t('workspaceDrawer')" :role="modal ? 'dialog' : undefined" :aria-modal="modal || undefined" data-modal-layer="40" @keydown="onKey">
    <div
      class="workspace-resize" role="separator" tabindex="0" aria-orientation="vertical" :aria-label="t('workspaceResize')"
      @pointerdown.prevent="startResize" @pointermove="drag" @lostpointercapture="dragging = false"
      @keydown.left.prevent="resize(width + 16)" @keydown.right.prevent="resize(width - 16)"
    />
    <header class="workspace-header">
      <div class="workspace-tabs" role="group" :aria-label="t('workspaceDrawer')">
        <button type="button" :aria-pressed="tab === 'git'" @click="emit('update:tab', 'git')">{{ t('workspaceGit') }} <span v-if="status?.changes.length" class="workspace-count">{{ status.changes.length }}</span></button>
        <button type="button" :aria-pressed="tab === 'files'" @click="emit('update:tab', 'files')">{{ t('workspaceFiles') }}</button>
      </div>
      <code class="workspace-path" :title="directory">{{ directoryName }}</code>
      <button type="button" class="workspace-icon" :title="t('workspaceRefresh')" :aria-label="t('workspaceRefresh')" @click="refresh"><MdiIcon :path="mdiRefresh" :size="17" /></button>
      <button type="button" class="workspace-icon" :aria-label="t('close')" @click="emit('close')"><MdiIcon :path="mdiClose" :size="17" /></button>
    </header>
    <template v-if="tab === 'git'">
      <p v-if="statusLoading && !status" class="workspace-state">{{ t('loading') }}</p>
      <p v-else-if="statusFailed" class="workspace-state" role="alert">{{ t('workspaceLoadFailed') }}</p>
      <p v-else-if="!status?.isRepo" class="workspace-state">{{ t('workspaceNoGit') }}</p>
      <template v-else>
        <div class="workspace-summary">
          <MdiIcon :path="mdiSourceBranch" :size="15" /><code>{{ status.branch }}</code>
          <span v-if="status.ahead" :title="t('workspaceAhead')">↑{{ status.ahead }}</span>
          <span v-if="status.behind" :title="t('workspaceBehind')">↓{{ status.behind }}</span>
          <span>{{ t('workspaceChangeCount', { count: status.changes.length, suffix: status.truncated ? '+' : '' }) }}</span>
        </div>
        <p v-if="!gitGroups.length" class="workspace-state">{{ t('workspaceClean') }}</p>
        <div v-else class="workspace-list sb-scrollbar">
          <div v-for="group in gitGroups" :key="group.key">
            <div class="workspace-group-title">{{ t(`workspaceGitGroup.${group.key}`) }} ({{ group.items.length }})</div>
            <button v-for="change in group.items" :key="change.path" type="button" class="workspace-row" :aria-pressed="activeChange === `${change.staged}:${change.path}`" :title="change.oldPath ? `${change.oldPath} → ${change.path}` : change.path" @click="selectChange(change.path, change.staged)">
              <span class="workspace-change" :data-code="change.code">{{ change.code }}</span>
              <span class="truncate">{{ change.path }}</span>
            </button>
          </div>
        </div>
        <div v-if="activeChange" class="workspace-preview sb-scrollbar">
          <p v-if="previewBusy" class="workspace-state">{{ t('loading') }}</p>
          <p v-else-if="previewFailed" class="workspace-state" role="alert">{{ t('workspaceLoadFailed') }}</p>
          <p v-else-if="diff?.binary" class="workspace-state">{{ t('workspaceBinaryDiff') }}</p>
          <template v-else-if="diff">
            <p v-if="diff.truncated" class="workspace-truncated" role="status">{{ t('workspaceTruncated') }}</p>
            <CheckpointDiff v-if="diff.diff" class="workspace-diff" :diff="diff.diff" />
            <p v-else-if="!diff.truncated" class="workspace-state">{{ t('workspaceEmptyDiff') }}</p>
          </template>
        </div>
      </template>
    </template>
    <template v-else>
      <div class="workspace-file-tools">
        <button type="button" class="workspace-icon" :title="t('workspaceHidden')" :aria-label="t('workspaceHidden')" :aria-pressed="hidden" @click="hidden = !hidden"><MdiIcon :path="hidden ? mdiEyeOutline : mdiEyeOffOutline" :size="16" /></button>
        <button type="button" class="workspace-icon" :title="t('workspaceCollapse')" :aria-label="t('workspaceCollapse')" @click="expanded.clear()"><MdiIcon :path="mdiArrowCollapseUp" :size="16" /></button>
      </div>
      <div class="workspace-list workspace-tree sb-scrollbar" :class="{ 'workspace-tree--full': !activePath }">
        <p v-if="loading.has('')" class="workspace-state">{{ t('loading') }}</p>
        <p v-else-if="folderErrors.has('')" class="workspace-state" role="alert">{{ t('workspaceLoadFailed') }}</p>
        <p v-else-if="!visibleEntries.length" class="workspace-state">{{ t('workspaceEmptyDirectory') }}</p>
        <template v-for="{ entry, depth } in visibleEntries" :key="entry.path">
          <button
            type="button" class="workspace-row" :style="{ paddingLeft: `${8 + depth * 14}px` }" :title="entry.path"
            :aria-expanded="entry.isDirectory ? expanded.has(entry.path) : undefined" :aria-pressed="entry.isDirectory ? undefined : activePath === entry.path"
            @click="entry.isDirectory ? toggleFolder(entry.path) : selectFile(entry.path)"
          >
            <MdiIcon v-if="entry.isDirectory" :path="mdiChevronRight" :size="13" class="shrink-0" :class="{ 'rotate-90': expanded.has(entry.path) }" />
            <span v-else class="w-[13px] shrink-0" />
            <MdiIcon :path="entry.isDirectory ? mdiFolderOutline : mdiFileOutline" :size="15" class="shrink-0" />
            <span class="truncate">{{ entry.name }}</span>
          </button>
          <p v-if="expanded.has(entry.path) && (loading.has(entry.path) || folderErrors.has(entry.path) || folders.get(entry.path)?.length === 0)" class="workspace-tree-hint" :style="{ paddingLeft: `${36 + depth * 14}px` }">{{ t(loading.has(entry.path) ? 'loading' : folderErrors.has(entry.path) ? 'workspaceLoadFailed' : 'workspaceEmptyDirectory') }}</p>
        </template>
      </div>
      <div v-if="activePath" class="workspace-preview">
        <div class="workspace-preview-bar">
          <code class="workspace-path" :title="activePath">{{ activePath }}</code>
          <button type="button" class="workspace-icon" :title="t('workspaceCopyPath')" :aria-label="t('workspaceCopyPath')" @click="copyPath(activePath)"><MdiIcon :path="mdiContentCopy" :size="15" /></button>
          <button type="button" class="workspace-icon" :disabled="!canQuote" :title="t('workspaceQuotePath')" :aria-label="t('workspaceQuotePath')" @click="emit('quote', activePath)"><MdiIcon :path="mdiChatPlusOutline" :size="17" /></button>
        </div>
        <p v-if="previewBusy" class="workspace-state">{{ t('loading') }}</p>
        <p v-else-if="previewFailed" class="workspace-state" role="alert">{{ t('workspaceLoadFailed') }}</p>
        <p v-else-if="file?.binary" class="workspace-state">{{ t('workspaceBinary', { size: formatSize(file.size) }) }}</p>
        <template v-else-if="file">
          <p v-if="file.truncated" class="workspace-truncated" role="status">{{ t('workspaceTruncated') }}</p>
          <div class="workspace-code sb-scrollbar" tabindex="0" :aria-label="activePath">
            <pre aria-hidden="true" class="workspace-lines">{{ fileLines }}</pre>
            <pre v-if="highlightedCode !== undefined"><code class="hljs" v-html="highlightedCode" /></pre>
            <pre v-else><code>{{ fileText }}</code></pre>
          </div>
        </template>
      </div>
    </template>
  </aside>
</template>

<style scoped>
.workspace-drawer { position: relative; z-index: 10; display: flex; flex-direction: column; flex: 0 0 min(var(--workspace-width), 47vw); width: min(var(--workspace-width), 47vw); min-height: 0; border-left: 1px solid var(--border-subtle); background: var(--bg-main); color: var(--text-primary); outline: none; font-size: 13px; }
.workspace-resize { position: absolute; z-index: 2; top: 0; bottom: 0; left: -2px; width: 4px; cursor: col-resize; touch-action: none; }
.workspace-resize:hover, .workspace-resize:focus-visible { border-left: 2px solid var(--sb-brand); }
.workspace-header, .workspace-preview-bar, .workspace-summary, .workspace-file-tools { display: flex; align-items: center; gap: 6px; min-width: 0; flex-shrink: 0; min-height: 34px; padding: 0 8px; border-bottom: 1px solid var(--border-subtle); }
.workspace-header { min-height: 44px; }
.workspace-tabs { display: flex; padding: 2px; gap: 2px; background: var(--bg-subtle); border-radius: 8px; }
.workspace-tabs button { padding: 4px 7px; border-radius: 6px; color: var(--text-secondary); font-size: 12px; white-space: nowrap; cursor: pointer; }
.workspace-tabs button[aria-pressed="true"], .workspace-icon[aria-pressed="true"] { background: var(--bg-active); color: var(--text-primary); }
.workspace-count { padding: 0 2px; font: 11px var(--font-mono); }
.workspace-path { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); font: 12px var(--font-mono); }
.workspace-icon { display: grid; place-items: center; flex: 0 0 26px; height: 26px; border-radius: 6px; color: var(--text-secondary); cursor: pointer; }
.workspace-icon:hover, .workspace-row:hover { background: var(--bg-hover); }
.workspace-icon:disabled { opacity: .45; cursor: default; }
.workspace-summary { color: var(--text-secondary); font-size: 12px; }
.workspace-summary code { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.workspace-summary span:last-child { margin-left: auto; white-space: nowrap; }
.workspace-list { flex-shrink: 0; max-height: 38%; overflow: auto; padding: 2px 4px 6px; }
.workspace-tree--full { flex: 1; max-height: none; }
.workspace-group-title { padding: 6px 8px 2px; font-size: 11px; font-weight: 600; color: var(--text-muted); }
.workspace-row { display: flex; align-items: center; gap: 5px; width: 100%; height: 28px; min-width: 0; padding: 0 8px; border-radius: 6px; text-align: left; font: 12px var(--font-mono); cursor: pointer; }
.workspace-row[aria-pressed="true"] { background: var(--bg-active); }
.workspace-change { flex: 0 0 16px; color: var(--text-secondary); text-align: center; font-weight: 600; }
.workspace-change[data-code="M"] { color: var(--color-warning); }
.workspace-change[data-code="A"], .workspace-change[data-code="?"] { color: var(--color-success); }
.workspace-change[data-code="D"], .workspace-change[data-code="U"] { color: var(--color-danger); }
.workspace-change[data-code="R"] { color: var(--sb-brand); }
.workspace-preview { display: flex; flex-direction: column; flex: 1; min-height: 0; border-top: 1px solid var(--border-subtle); overflow: auto; }
.workspace-diff { flex: 1; max-height: none; border: 0; border-radius: 0; }
.workspace-state, .workspace-tree-hint { padding: 10px 12px; color: var(--text-secondary); font-size: 12px; }
.workspace-tree-hint { padding-top: 2px; padding-bottom: 4px; }
.workspace-truncated { padding: 6px 8px; flex-shrink: 0; background: var(--warning-alpha-12); color: var(--tool-pending-text); font-size: 12px; }
.workspace-code { display: flex; flex: 1; min-height: 0; overflow: auto; color: var(--text-primary); background: var(--bg-subtle); font: 12px/1.6 var(--font-mono); }
.workspace-code pre { flex: 0 0 auto; min-width: max-content; padding: 8px; tab-size: 4; }
.workspace-code :deep(.hljs-keyword), .workspace-code :deep(.hljs-type) { color: var(--sb-brand); }
.workspace-code :deep(.hljs-string) { color: var(--tool-success-text); }
.workspace-code :deep(.hljs-comment) { color: var(--text-muted); }
.workspace-code :deep(.hljs-number) { color: var(--tool-pending-text); }
.workspace-code .workspace-lines { position: sticky; left: 0; padding-inline: 10px; text-align: right; color: var(--text-muted); background: var(--bg-subtle); user-select: none; }
.workspace-drawer button:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: -2px; }
@media (max-width: 1023px) {
  .workspace-drawer { position: absolute; z-index: 40; inset: 0 0 0 auto; width: min(480px, 100%); }
  .workspace-resize { display: none; }
}
@media (max-width: 767px) { .workspace-drawer { inset: 0; width: 100%; } }
</style>
