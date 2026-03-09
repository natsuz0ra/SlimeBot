<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { mdiChevronDown, mdiClose, mdiCogOutline, mdiDeleteOutline, mdiDotsHorizontal, mdiMenu, mdiPencilOutline, mdiPlus, mdiRobotOutline, mdiSend } from '@mdi/js'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'

import MdiIcon from '../components/MdiIcon.vue'
import TypingDots from '../components/TypingDots.vue'
import SettingsPanel from '../components/SettingsPanel.vue'
import { llmAPI, sessionAPI, type LLMConfig } from '../api'
import { useChatStore } from '../stores/chat'
import { renderMarkdown } from '../utils/markdown'

const { t } = useI18n()
const store = useChatStore()
const MODEL_STORAGE_KEY = 'corner:selectedModelId'

const drawerOpen = ref(false)
const renameVisible = ref(false)
const renameValue = ref('')
const renameTargetId = ref('')
const inputValue = ref('')
const loading = ref(false)
const settingsVisible = ref(false)
const hasConnectedOnce = ref(false)

const activeSessionMenu = ref<{ id: string; x: number; y: number } | null>(null)
const topMenuVisible = ref(false)
const modelOptions = ref<LLMConfig[]>([])
const selectedModelId = ref('')
const messagesRef = ref<HTMLElement | null>(null)

const currentSession = computed(() => store.sessions.find((item) => item.id === store.currentSessionId))
const hasModel = computed(() => modelOptions.value.length > 0)
const sendDisabled = computed(() => !hasModel.value || !selectedModelId.value || !store.currentSessionId || !inputValue.value.trim() || store.waiting || !store.isSocketReady)
const networkStatusText = computed(() => {
  if (store.connectionStatus === 'reconnecting') return t('networkReconnecting')
  if (store.connectionStatus === 'disconnected') return t('networkDisconnected')
  return ''
})

function onGlobalClick() {
  activeSessionMenu.value = null
  topMenuVisible.value = false
}

function toggleSidebar() {
  drawerOpen.value = !drawerOpen.value
}

function toggleSessionMenu(sessionId: string, event: MouseEvent) {
  const target = event.currentTarget as HTMLElement | null
  if (!target) return
  if (activeSessionMenu.value?.id === sessionId) {
    activeSessionMenu.value = null
    return
  }
  const rect = target.getBoundingClientRect()
  activeSessionMenu.value = { id: sessionId, x: rect.right + 6, y: rect.top }
}

function syncModelToLocal(modelId: string) {
  if (!modelId) return
  localStorage.setItem(MODEL_STORAGE_KEY, modelId)
}

function resolveInitialModelId(items: LLMConfig[]) {
  const first = items[0]
  if (!first) return ''
  const remembered = localStorage.getItem(MODEL_STORAGE_KEY)
  const matched = remembered ? items.find((item) => item.id === remembered) : undefined
  return matched?.id || first.id
}

function showWarning(message: string) {
  MessagePlugin.warning({
    content: message,
    placement: 'top-right',
  })
}

function showError(message: string) {
  MessagePlugin.error({
    content: message,
    placement: 'top-right',
  })
}

function scrollMessagesToBottom() {
  const el = messagesRef.value
  if (!el) return
  el.scrollTop = el.scrollHeight
}

function queueScrollMessagesToBottom() {
  void nextTick(() => {
    scrollMessagesToBottom()
  })
}

async function boot() {
  loading.value = true
  try {
    modelOptions.value = await llmAPI.list()
    selectedModelId.value = resolveInitialModelId(modelOptions.value)
    syncModelToLocal(selectedModelId.value)

    await store.loadSessions()
    if (store.sessions.length === 0) {
      await store.createSession()
    } else {
      const first = store.sessions[0]
      if (first) await store.selectSession(first.id)
    }
    await nextTick()
    scrollMessagesToBottom()
    store.connectSocket()
  } finally {
    loading.value = false
  }
}

function openRename(sessionId: string, oldName: string) {
  renameTargetId.value = sessionId
  renameValue.value = oldName
  renameVisible.value = true
}

async function confirmRename() {
  if (!renameTargetId.value || !renameValue.value.trim()) return
  await sessionAPI.rename(renameTargetId.value, renameValue.value.trim())
  await store.loadSessions()
  renameVisible.value = false
}

async function removeSession(id: string) {
  if (!window.confirm(t('confirmDelete'))) return
  try {
    await sessionAPI.remove(id)
    await store.loadSessions()
    const first = store.sessions[0]
    if (first) {
      await store.selectSession(first.id)
    } else {
      await store.createSession()
    }
  } catch {
    showError('删除失败')
  } finally {
    activeSessionMenu.value = null
    topMenuVisible.value = false
  }
}

async function pickSession(id: string) {
  await store.selectSession(id)
  await nextTick()
  scrollMessagesToBottom()
  drawerOpen.value = false
}

async function createSession() {
  await store.createSession()
  drawerOpen.value = false
}

async function sendMessage() {
  if (sendDisabled.value) return
  const sent = await store.sendMessage(inputValue.value.trim(), selectedModelId.value)
  if (!sent) {
    showWarning(t('sendBlockedOffline'))
    return
  }
  inputValue.value = ''
}

function renameFromFloatingMenu() {
  const menu = activeSessionMenu.value
  if (!menu) return
  const name = store.sessions.find((s) => s.id === menu.id)?.name || ''
  openRename(menu.id, name)
  activeSessionMenu.value = null
}

function deleteFromFloatingMenu() {
  const menu = activeSessionMenu.value
  if (!menu) return
  void removeSession(menu.id)
}

async function onModelChange(modelId: string) {
  selectedModelId.value = modelId
  syncModelToLocal(modelId)
}

onMounted(() => {
  void boot()
  document.addEventListener('click', onGlobalClick)
})

watch(
  () => store.connectionStatus,
  (status, prev) => {
    if (status === 'connected') {
      hasConnectedOnce.value = true
      return
    }
    if (status === prev || !hasConnectedOnce.value) return
    showWarning(t(status === 'reconnecting' ? 'networkReconnecting' : 'networkDisconnected'))
  },
)

watch(
  () => store.currentSessionId,
  () => {
    queueScrollMessagesToBottom()
  },
)

watch(
  () => store.messages.length,
  () => {
    queueScrollMessagesToBottom()
  },
)

watch(
  () => store.messages[store.messages.length - 1]?.content,
  () => {
    queueScrollMessagesToBottom()
  },
)

watch(
  () => [store.waiting, store.streamingStarted],
  () => {
    queueScrollMessagesToBottom()
  },
)

onUnmounted(() => {
  store.disconnectSocket()
  document.removeEventListener('click', onGlobalClick)
})
</script>

<template>
  <div class="home-page">
    <div class="canvas-shell">
      <aside class="sidebar" :class="{ open: drawerOpen }">
        <div class="sidebar-head">
          <t-button variant="text" shape="square" @click="createSession">
            <MdiIcon :path="mdiPlus" />
          </t-button>
          <t-button variant="text" shape="square" @click="drawerOpen = false">
            <MdiIcon :path="mdiClose" />
          </t-button>
        </div>
        <div class="session-list">
          <div
            v-for="item in store.sessions"
            :key="item.id"
            class="session-row"
            :class="{ active: item.id === store.currentSessionId }"
            @click="pickSession(item.id)"
          >
            <span class="session-text">{{ item.name }}</span>
            <t-button variant="text" shape="square" class="dots-btn" @click.stop="toggleSessionMenu(item.id, $event as MouseEvent)">
              <MdiIcon :path="mdiDotsHorizontal" />
            </t-button>
          </div>
        </div>
        <div class="sidebar-foot">
          <t-button variant="text" block class="setting-btn" @click="settingsVisible = true">
            <template #icon><MdiIcon :path="mdiCogOutline" /></template>
            {{ t('settings') }}
          </t-button>
        </div>
      </aside>

      <div v-if="drawerOpen" class="mask" @click="drawerOpen = false" />

      <main class="main-panel">
        <header class="topbar">
          <t-button variant="text" shape="square" class="menu-toggle" @click.stop="toggleSidebar">
            <MdiIcon :path="mdiMenu" />
          </t-button>
          <button class="title-trigger" type="button" @click.stop="topMenuVisible = !topMenuVisible">
            <span class="chat-title">{{ currentSession?.name || t('newSession') }}</span>
            <MdiIcon :path="mdiChevronDown" :size="16" />
          </button>
          <span v-if="networkStatusText" class="network-state" :class="store.connectionStatus">
            {{ networkStatusText }}
          </span>
          <div v-if="topMenuVisible" class="menu-card top-menu" @click.stop>
            <button
              v-if="currentSession"
              type="button"
              class="menu-item"
              @click="openRename(currentSession.id, currentSession.name); topMenuVisible = false"
            >
              <MdiIcon :path="mdiPencilOutline" :size="14" />
              <span>{{ t('rename') }}</span>
            </button>
            <button v-if="currentSession" type="button" class="menu-item danger" @click="removeSession(currentSession.id)">
              <MdiIcon :path="mdiDeleteOutline" :size="14" />
              <span>{{ t('delete') }}</span>
            </button>
          </div>
        </header>

        <section ref="messagesRef" class="messages">
          <div v-if="loading" class="loading-text">{{ t('loading') }}</div>
          <div v-for="item in store.messages" :key="item.id" class="message" :class="item.role">
            <div v-if="item.role === 'assistant'" class="avatar">
              <MdiIcon :path="mdiRobotOutline" :size="30" />
            </div>
            <div class="bubble">
              <template v-if="item.role === 'assistant'">
                <div class="bubble-markdown" v-html="renderMarkdown(item.content)" />
              </template>
              <template v-else>
                {{ item.content }}
              </template>
            </div>
          </div>
          <div v-if="store.waiting && !store.streamingStarted" class="message assistant assistant-waiting">
            <div class="avatar"><MdiIcon :path="mdiRobotOutline" :size="30" /></div>
            <div class="bubble"><TypingDots /></div>
          </div>
        </section>

        <footer class="composer-wrap">
          <div class="composer">
            <textarea v-model="inputValue" class="chat-input" :placeholder="t('inputPlaceholder')" />
            <div class="composer-foot">
              <t-select
                :model-value="selectedModelId"
                class="model-select"
                size="small"
                placeholder="无"
                :disabled="modelOptions.length === 0"
                @change="onModelChange($event as string)"
              >
                <t-option v-if="modelOptions.length === 0" value="" label="无" />
                <t-option v-for="item in modelOptions" :key="item.id" :value="item.id" :label="item.name" />
              </t-select>
              <button class="send-btn" type="button" :disabled="sendDisabled" @click="sendMessage">
                <MdiIcon :path="mdiSend" :size="16" />
              </button>
            </div>
          </div>
        </footer>
      </main>
    </div>

    <t-dialog
      v-model:visible="renameVisible"
      :header="t('rename')"
      :confirm-btn="t('confirm')"
      :cancel-btn="t('cancel')"
      @confirm="confirmRename"
    >
      <t-input v-model="renameValue" />
    </t-dialog>

    <div v-if="settingsVisible" class="settings-overlay" @click.self="settingsVisible = false">
      <div class="settings-modal" @click.stop>
        <SettingsPanel @close="settingsVisible = false" />
      </div>
    </div>

    <div
      v-if="activeSessionMenu"
      class="menu-card floating-session-menu"
      :style="{ left: `${activeSessionMenu.x}px`, top: `${activeSessionMenu.y}px` }"
      @click.stop
    >
      <button type="button" class="menu-item" @click="renameFromFloatingMenu">
        <MdiIcon :path="mdiPencilOutline" :size="14" />
        <span>{{ t('rename') }}</span>
      </button>
      <button type="button" class="menu-item danger" @click="deleteFromFloatingMenu">
        <MdiIcon :path="mdiDeleteOutline" :size="14" />
        <span>{{ t('delete') }}</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.home-page {
  height: 100vh;
  background: #f2f2f2;
  padding: 8px;
  box-sizing: border-box;
}

.canvas-shell {
  height: 100%;
  border: 1px solid #cfcfcf;
  border-radius: 6px;
  overflow: visible;
  display: flex;
  background: #efefef;
  position: relative;
}

.sidebar {
  width: 240px;
  background: #e9e9e9;
  border-right: 1px solid #d8d8d8;
  border-top-right-radius: 6px;
  border-bottom-right-radius: 6px;
  display: flex;
  flex-direction: column;
  z-index: 30;
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  transform: translateX(-100%);
  transition: transform 0.2s ease;
  box-shadow: 4px 0 18px rgba(0, 0, 0, 0.12);
}

.sidebar.open {
  transform: translateX(0);
}

.sidebar-head {
  height: 40px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 8px;
}

.session-list {
  flex: 1;
  overflow-y: auto;
  overflow-x: visible;
  padding: 0 8px;
}

.session-row {
  position: relative;
  height: 34px;
  border: 1px solid transparent;
  border-radius: 8px;
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 0 8px;
  margin-bottom: 6px;
  cursor: pointer;
}

.session-row.active {
  border-color: #c9c9c9;
  background: #f7f7f7;
}

.session-row:hover {
  background: #f1f1f1;
}

.session-text {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 14px;
}

.dots-btn {
  color: #2f2f2f;
  opacity: 0;
  pointer-events: none;
  transition: opacity 0.15s ease;
}

.session-row:hover .dots-btn,
.session-row.active .dots-btn {
  opacity: 1;
  pointer-events: auto;
}

.sidebar-foot {
  border-top: 1px solid #d5d5d5;
  padding: 0;
}

.setting-btn {
  justify-content: flex-start;
  height: 34px;
  border-radius: 0;
}

.main-panel {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  background: #ffffff;
  border-top-right-radius: 6px;
  border-bottom-right-radius: 6px;
  overflow: hidden;
}

.topbar {
  position: relative;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-bottom: 1px solid #dcdcdc;
  background: #ffffff;
  gap: 8px;
}

.title-trigger {
  background: transparent;
  border: none;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  cursor: pointer;
  color: #222;
  font-size: 14px;
}

.menu-toggle {
  position: absolute;
  left: 8px;
}

.chat-title {
  max-width: 280px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.network-state {
  position: absolute;
  right: 10px;
  font-size: 12px;
  color: #d54941;
}

.network-state.reconnecting {
  color: #c78112;
}

.messages {
  flex: 1;
  overflow: auto;
  padding: 14px 18px;
  background: #ffffff;
}

.loading-text {
  color: #8a8a8a;
  font-size: 14px;
}

.message {
  display: flex;
  gap: 10px;
  margin-bottom: 16px;
}

.message.user {
  justify-content: flex-end;
}

.message.user .avatar {
  order: 2;
}

.avatar {
  color: #333;
}

.bubble {
  max-width: min(70%, 640px);
  background: transparent;
  border-radius: 8px;
  padding: 9px 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  font-size: 14px;
  color: #1d1d1d;
}

.message.user .bubble {
  background: #e7e7e7;
}

.message.assistant .bubble {
  border-radius: 0;
  padding: 0;
}

.message.assistant .avatar {
  margin-left: -8px;
}

.message.assistant-waiting {
  align-items: center;
}

.message.assistant-waiting .bubble {
  line-height: 1;
}

.bubble-markdown :deep(p) {
  margin: 0 0 6px;
}

.bubble-markdown :deep(p:last-child) {
  margin-bottom: 0;
}

.bubble-markdown :deep(ul),
.bubble-markdown :deep(ol) {
  margin: 0 0 6px;
  padding-left: 20px;
}

.bubble-markdown :deep(li + li) {
  margin-top: 2px;
}

.bubble-markdown :deep(blockquote) {
  margin: 0 0 6px;
  padding-left: 10px;
  border-left: 3px solid #b7b7b7;
  color: #3a3a3a;
}

.bubble-markdown :deep(pre) {
  margin: 6px 0;
  padding: 10px;
  border-radius: 6px;
  background: #1f2329;
  color: #e6edf3;
  overflow-x: auto;
  white-space: pre;
}

.bubble-markdown :deep(pre > code) {
  display: block;
}

.bubble-markdown :deep(pre > code .hljs) {
  display: block;
  padding: 0;
  background: transparent;
  color: inherit;
}

.bubble-markdown {
  white-space: normal;
  font-size: 14px;
  line-height: 1.6;
}

.bubble-markdown :deep(> :first-child) {
  margin-top: 0;
}

.bubble-markdown :deep(> :last-child) {
  margin-bottom: 0;
}

.bubble-markdown :deep(code) {
  font-family: 'Consolas', 'Courier New', monospace;
  font-size: 0.9em;
}

.bubble-markdown :deep(:not(pre) > code) {
  padding: 1px 4px;
  border-radius: 4px;
  background: rgba(31, 35, 41, 0.08);
  color: #222;
}

.bubble-markdown :deep(a) {
  color: #1f6feb;
  text-decoration: underline;
  word-break: break-all;
}

@media (min-width: 901px) {
  .message {
    width: min(100%, 688px);
    margin-left: auto;
    margin-right: auto;
    margin-bottom: 12px;
  }

  .message.assistant .avatar {
    width: 30px;
    flex: 0 0 30px;
    display: flex;
    justify-content: center;
    align-items: flex-start;
  }

  .message.assistant-waiting .avatar {
    align-items: center;
  }

  .bubble {
    max-width: calc(100% - 24px - 10px);
  }
}

.composer-wrap {
  padding: 10px 0;
  display: flex;
  justify-content: center;
}

.composer {
  width: min(100%, 470px);
  border: 1px solid #cccccc;
  border-radius: 8px;
  background: #ffffff;
  padding: 10px;
  min-height: 92px;
  position: relative;
}

.chat-input {
  width: 100%;
  min-height: 72px;
  border: 0;
  outline: none;
  resize: none;
  background: transparent;
  padding: 0 0 24px;
  font-size: 13px;
  font-family: inherit;
  color: #222;
}

.composer-foot {
  position: absolute;
  left: 10px;
  right: 10px;
  bottom: 8px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.model-select {
  width: 112px;
}

.send-btn {
  width: 24px;
  height: 24px;
  border: none;
  border-radius: 50%;
  background: #7dc8f4;
  color: white;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
}

.send-btn:disabled {
  background: #b6b6b6;
  cursor: not-allowed;
}

.menu-card {
  width: 94px;
  background: #ffffff;
  border: 1px solid #d8d8d8;
  border-radius: 8px;
  padding: 6px;
  box-shadow: 0 6px 16px rgba(0, 0, 0, 0.1);
}

.top-menu {
  position: absolute;
  top: 36px;
  left: 50%;
  transform: translateX(-50%);
  z-index: 60;
}

.menu-item {
  width: 100%;
  border: 0;
  background: transparent;
  border-radius: 4px;
  height: 28px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 0 6px;
  cursor: pointer;
  color: #1f1f1f;
}

.menu-item:hover {
  background: #f2f2f2;
}

.menu-item.danger {
  color: #d54941;
}

.mask {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.26);
  z-index: 20;
}

.settings-overlay {
  position: fixed;
  inset: 0;
  z-index: 40;
  background: rgba(0, 0, 0, 0.24);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  box-sizing: border-box;
}

.settings-modal {
  width: min(86vw, 1080px);
  height: min(80vh, 760px);
}

.floating-session-menu {
  position: fixed;
  z-index: 80;
}
</style>
