<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { mdiAlertCircleOutline, mdiArrowUp, mdiFolderOutline, mdiHomeOutline, mdiMonitor, mdiSourceBranch } from '@mdi/js'
import { sessionAPI } from '@/api/chat'
import AppDialog from '@/components/ui/AppDialog.vue'
import MdiIcon from '@/components/ui/MdiIcon.vue'

const props = defineProps<{ path: string; editable: boolean }>()
const emit = defineEmits<{ select: [path: string] }>()
const { t } = useI18n()
const isDesktop = Boolean(window.slimebotDesktop)
const open = ref(false)
const busy = ref(false)
const choosing = ref(false)
const browsing = ref(false)
const error = ref('')
const browserPath = ref('')
const parentPath = ref('')
const locationInput = ref('')
const directories = ref<{ name: string; path: string }[]>([])
const places = ref<{ kind: string; path: string }[]>([])
const recent = ref<string[]>([])
const branch = ref('')
const pathAvailable = ref(true)
let validationRequest = 0
let browseRequest = 0
const recentKey = 'slimebot:recent-working-directories'
const name = computed(() => props.path.replace(/[\\/]+$/, '').split(/[\\/]/).filter(Boolean).pop() || props.path)
const placeLabelKeys: Record<string, string> = {
  home: 'workspacePlaceHome', desktop: 'workspacePlaceDesktop', documents: 'workspacePlaceDocuments',
  downloads: 'workspacePlaceDownloads', projects: 'workspacePlaceProjects',
  onedrive: 'workspacePlaceOneDrive', computer: 'workspacePlaceComputer',
}
const folderName = (path: string) => path.replace(/[\\/]+$/, '').split(/[\\/]/).pop() || path

watch(() => props.editable, (editable) => { if (!editable) open.value = false })
watch(() => props.path, async (path) => {
  const request = ++validationRequest
  branch.value = ''
  if (!path) { pathAvailable.value = true; return }
  try {
    const result = await sessionAPI.validateWorkingDirectory(path)
    if (request === validationRequest) {
      pathAvailable.value = true
      branch.value = result.branch
    }
  } catch {
    if (request === validationRequest) pathAvailable.value = false
  }
}, { immediate: true })

onMounted(() => {
  try {
    const stored = JSON.parse(window.localStorage.getItem(recentKey) || '[]')
    recent.value = Array.isArray(stored) ? stored.filter((item): item is string => typeof item === 'string') : []
  } catch { recent.value = [] }
})

async function browseDirectory(path = '') {
  const request = ++browseRequest
  browsing.value = true
  error.value = ''
  try {
    const result = await sessionAPI.browseWorkingDirectory(path)
    if (request !== browseRequest || !open.value) return
    browserPath.value = result.path
    parentPath.value = result.parent
    locationInput.value = result.path
    directories.value = result.directories
    places.value = result.places
  } catch {
    if (request === browseRequest) error.value = t('workspaceInvalid')
  } finally {
    if (request === browseRequest) browsing.value = false
  }
}

async function selectPath(path: string) {
  if (!props.editable || busy.value) return
  busy.value = true
  error.value = ''
  try {
    const result = await sessionAPI.validateWorkingDirectory(path)
    if (!props.editable) return
    emit('select', result.path)
    recent.value = [result.path, ...recent.value.filter((item) => item !== result.path)].slice(0, 8)
    try { window.localStorage.setItem(recentKey, JSON.stringify(recent.value)) } catch { /* Storage may be unavailable. */ }
    open.value = false
  } catch {
    error.value = t('workspaceInvalid')
  } finally {
    busy.value = false
  }
}

async function openPicker() {
  if (!props.editable || busy.value || choosing.value) return
  error.value = ''
  if (isDesktop) {
    choosing.value = true
    try {
      const path = await window.slimebotDesktop!.chooseWorkingDirectory(props.path || recent.value[0])
      if (path) await selectPath(path)
    } catch {
      error.value = t('workspaceInvalid')
    } finally {
      choosing.value = false
    }
    return
  }
  open.value = true
  await browseDirectory(props.path)
}
</script>

<template>
  <div class="workspace-context-bar relative z-0 mx-3 flex h-8 min-w-0 items-center gap-1 rounded-t-xl px-2.5 text-xs">
    <button
      type="button"
      class="workspace-project flex min-w-0 items-center gap-1.5 rounded-md px-1.5 py-1 cursor-pointer"
      :title="path || t('workspaceChoose')"
      :aria-label="t('workspaceChoose')"
      @click="openPicker"
    >
      <MdiIcon :path="mdiFolderOutline" :size="14" class="flex-shrink-0" />
      <span class="truncate font-medium">{{ path ? name : t('workspaceChoose') }}</span>
    </button>
    <span class="workspace-detail flex flex-shrink-0 items-center gap-1 px-1.5">
      <MdiIcon :path="mdiMonitor" :size="13" />{{ t('workspaceLocal') }}
    </span>
    <span v-if="branch" class="workspace-detail flex min-w-0 items-center gap-1 px-1.5" :title="branch">
      <MdiIcon :path="mdiSourceBranch" :size="13" class="flex-shrink-0" />
      <span class="truncate">{{ branch }}</span>
    </span>
    <MdiIcon v-if="error || (path && !pathAvailable)" :path="mdiAlertCircleOutline" :size="14" class="ml-auto flex-shrink-0 text-red-500" :title="t('workspaceInvalid')" />
  </div>

  <AppDialog
    v-if="!isDesktop"
    v-model:visible="open"
    :title="t('workspaceChoose')"
    :confirm-text="t('workspaceSelectFolder')"
    :confirm-loading="busy || browsing"
    width="640px"
    @confirm="selectPath(browserPath)"
  >
    <p class="workspace-hint mb-3 text-xs">{{ t('workspaceServerPathHint') }}</p>
    <div class="mb-2 flex items-center gap-2">
      <button type="button" class="workspace-nav" :aria-label="t('workspaceHome')" @click="browseDirectory()"><MdiIcon :path="mdiHomeOutline" :size="17" /></button>
      <button type="button" class="workspace-nav" :disabled="browserPath === parentPath" :aria-label="t('workspaceParent')" @click="browseDirectory(parentPath)"><MdiIcon :path="mdiArrowUp" :size="17" /></button>
      <input v-model="locationInput" class="workspace-location min-w-0 flex-1 rounded-lg px-3 py-2 text-sm outline-none" :placeholder="t('workspacePathPlaceholder')" @keydown.enter.prevent="browseDirectory(locationInput)">
      <button type="button" class="workspace-go rounded-lg px-3 py-2 text-xs font-medium cursor-pointer" @click="browseDirectory(locationInput)">{{ t('workspaceGo') }}</button>
    </div>
    <div class="flex flex-col gap-2 sm:flex-row">
      <nav class="workspace-shortcuts flex flex-shrink-0 gap-1 overflow-x-auto pb-1 sm:max-h-64 sm:w-40 sm:flex-col sm:overflow-y-auto sm:pb-0" :aria-label="t('workspacePlaces')">
        <div class="workspace-hint hidden px-2 pb-1 text-xs font-medium sm:block">{{ t('workspacePlaces') }}</div>
        <button v-for="place in places" :key="place.path" type="button" class="workspace-shortcut" :class="browserPath === place.path ? 'workspace-shortcut-active' : ''" :title="place.path" @click="browseDirectory(place.path)">
          <MdiIcon :path="place.kind === 'home' ? mdiHomeOutline : place.kind === 'computer' ? mdiMonitor : mdiFolderOutline" :size="15" class="flex-shrink-0" />
          <span class="truncate">{{ t(placeLabelKeys[place.kind] || 'workspaceChoose') }}</span>
        </button>
        <div v-if="recent.length" class="workspace-hint hidden border-t px-2 pb-1 pt-2 text-xs font-medium sm:block">{{ t('workspaceRecent') }}</div>
        <button v-for="item in recent.slice(0, 4)" :key="item" type="button" class="workspace-shortcut" :title="item" @click="browseDirectory(item)">
          <MdiIcon :path="mdiFolderOutline" :size="15" class="flex-shrink-0" />
          <span class="truncate">{{ folderName(item) }}</span>
        </button>
      </nav>
      <div class="workspace-list min-h-40 max-h-64 flex-1 overflow-y-auto rounded-xl p-1">
        <div v-if="browsing" class="workspace-hint px-3 py-5 text-center text-sm">{{ t('loading') }}</div>
        <div v-else-if="!directories.length" class="workspace-hint px-3 py-5 text-center text-sm">{{ t('workspaceNoFolders') }}</div>
        <button v-for="item in browsing ? [] : directories" :key="item.path" type="button" class="workspace-row flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-sm cursor-pointer" :title="item.path" @click="browseDirectory(item.path)">
          <MdiIcon :path="mdiFolderOutline" :size="17" class="flex-shrink-0" />
          <span class="truncate">{{ item.name }}</span>
        </button>
      </div>
    </div>
    <p v-if="error" class="mt-2 text-xs text-red-500" role="alert">{{ error }}</p>
    <p v-else class="workspace-hint mt-2 truncate text-xs" :title="browserPath">{{ browserPath }}</p>
  </AppDialog>
</template>

<style scoped>
.workspace-context-bar { color: var(--text-secondary); background: var(--primary-alpha-05); border: 1px solid var(--input-border); border-bottom: 0; }
.workspace-project { color: var(--text-primary); transition: background 150ms ease; }
.workspace-project:hover, .workspace-project:focus-visible, .workspace-nav:hover, .workspace-nav:focus-visible, .workspace-row:hover, .workspace-row:focus-visible { background: var(--primary-alpha-10); outline: none; }
.workspace-project:focus-visible, .workspace-nav:focus-visible, .workspace-row:focus-visible, .workspace-go:focus-visible, .workspace-shortcut:focus-visible { box-shadow: 0 0 0 2px var(--sb-brand); }
.workspace-detail, .workspace-hint { color: var(--text-muted); }
.workspace-nav { display: flex; flex: 0 0 auto; align-items: center; justify-content: center; width: 34px; height: 34px; border-radius: 8px; cursor: pointer; }
.workspace-nav:disabled { opacity: .35; cursor: default; }
.workspace-location, .workspace-list { color: var(--text-primary); background: var(--input-bg); border: 1px solid var(--input-border); }
.workspace-location:focus { border-color: var(--sb-brand); }
.workspace-go { color: white; background: var(--sb-brand); }
.workspace-shortcut { display: flex; align-items: center; flex: 0 0 auto; gap: 7px; min-width: 0; padding: 7px 9px; border-radius: 8px; color: var(--text-secondary); font-size: 12px; text-align: left; cursor: pointer; }
.workspace-shortcut:hover, .workspace-shortcut-active { color: var(--text-primary); background: var(--primary-alpha-10); }
.workspace-shortcuts .border-t { border-color: var(--input-border); }
.workspace-row { color: var(--text-primary); }
</style>
