<script setup lang="ts">
import { mdiAlert, mdiChevronDown, mdiClose, mdiCogOutline, mdiDeleteOutline, mdiDotsHorizontal, mdiMenu, mdiPencilOutline, mdiPlus, mdiRobotOutline, mdiSend } from '@mdi/js'

import MdiIcon from '../../shared/components/MdiIcon.vue'
import TypingDots from '../../features/chat/components/TypingDots.vue'
import SettingsPanel from '../../features/settings/components/SettingsPanel.vue'
import ToolCallCard from '../../features/chat/components/ToolCallCard.vue'
import { renderMarkdown } from '../../shared/utils/markdown'
import { useHomeChatPage } from '../../features/chat/composables/useHomeChatPage'

const {
  t,
  store,
  drawerOpen,
  renameVisible,
  renameValue,
  inputValue,
  loading,
  settingsVisible,
  toolDetailVisible,
  toolDetailDialogWidth,
  activeSessionMenu,
  topMenuVisible,
  modelOptions,
  selectedModelId,
  setMessagesRef,
  currentSession,
  sendDisabled,
  networkStatusText,
  getReplyToolCount,
  getReplyTimeline,
  getReplyToolItem,
  isReplyToolCollapsed,
  isEmptyPlaceholder,
  openToolDetail,
  toolDetailItems,
  toolDetailToolTimeline,
  toggleSidebar,
  toggleSessionMenu,
  refreshModelOptions,
  openRename,
  confirmRename,
  removeSession,
  pickSession,
  createSession,
  sendMessage,
  renameFromFloatingMenu,
  deleteFromFloatingMenu,
  onModelChange,
} = useHomeChatPage()
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

        <section :ref="setMessagesRef" class="messages">
          <div v-if="loading" class="loading-text">{{ t('loading') }}</div>
          <div
            v-for="item in store.messages"
            :key="item.id"
            class="message"
            :class="[
              item.role,
              {
                'assistant-waiting-inline': item.role === 'assistant' && isEmptyPlaceholder(item.id) && store.waiting,
                'assistant-error': item.role === 'assistant' && store.isAssistantErrorMessage(item.id),
                'user-send-failed': item.role === 'user' && store.isFailedUserMessage(item.id),
              },
            ]"
          >
            <div v-if="item.role === 'assistant'" class="avatar">
              <MdiIcon :path="mdiRobotOutline" :size="30" />
            </div>
            <div v-if="item.role === 'user' && store.isFailedUserMessage(item.id)" class="user-failed-icon" :title="t('sendBlockedOffline')">
              <MdiIcon :path="mdiAlert" :size="14" />
            </div>
            <div class="bubble">
              <template v-if="item.role === 'assistant'">
                <div
                  v-if="getReplyToolCount(item.id) > 0"
                  class="tool-summary-line"
                >
                  <button
                    type="button"
                    class="tool-summary-btn"
                    @click="openToolDetail(item.id)"
                  >
                    {{ t('toolExecutionCount', { count: getReplyToolCount(item.id) }) }}
                  </button>
                </div>
                <div
                  v-if="getReplyToolCount(item.id) > 0 && !isReplyToolCollapsed(item.id)"
                  class="tool-inline-list"
                >
                  <template v-for="entry in getReplyTimeline(item.id)" :key="entry.id">
                    <div v-if="entry.kind === 'text'" class="timeline-text bubble-markdown" v-html="renderMarkdown(entry.content)" />
                    <div v-else-if="entry.kind === 'tool_start'" class="timeline-tool-row">
                      <ToolCallCard
                        v-if="getReplyToolItem(item.id, entry.toolCallId)"
                        :item="getReplyToolItem(item.id, entry.toolCallId)!"
                        :show-preamble="false"
                        @approve="store.approveToolCall($event, true)"
                        @reject="store.approveToolCall($event, false)"
                      />
                    </div>
                    <div v-else class="timeline-tool-result">
                      {{ t('toolExecutionFinished') }}
                    </div>
                  </template>
                </div>
                <div
                  v-if="getReplyToolCount(item.id) === 0 || isReplyToolCollapsed(item.id)"
                  class="bubble-markdown"
                  v-html="renderMarkdown(item.content)"
                />
                <TypingDots v-if="isEmptyPlaceholder(item.id) && store.waiting" />
              </template>
              <template v-else>
                {{ item.content }}
              </template>
            </div>
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

    <t-dialog
      v-model:visible="toolDetailVisible"
      class="tool-detail-dialog"
      :header="t('toolExecutionDetailTitle')"
      :confirm-btn="null"
      :cancel-btn="t('close')"
      :width="toolDetailDialogWidth"
    >
      <div class="tool-detail-list">
        <template v-for="entry in toolDetailToolTimeline" :key="entry.id">
          <ToolCallCard
            v-if="entry.kind === 'tool_start' && toolDetailItems.find((tc) => tc.toolCallId === entry.toolCallId)"
            :item="toolDetailItems.find((tc) => tc.toolCallId === entry.toolCallId)!"
            :show-preamble="true"
            @approve="store.approveToolCall($event, true)"
            @reject="store.approveToolCall($event, false)"
          />
          <div v-else-if="entry.kind === 'tool_result'" class="timeline-tool-result">
            {{ t('toolExecutionFinished') }}
          </div>
        </template>
      </div>
    </t-dialog>

    <div v-if="settingsVisible" class="settings-overlay" @click.self="settingsVisible = false">
      <div class="settings-modal" @click.stop>
        <SettingsPanel @close="settingsVisible = false" @llm-changed="refreshModelOptions" />
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

<style scoped src="../../features/chat/styles/home-page.css"></style>
