<script setup lang="ts">
import {
  mdiAlert,
  mdiChevronDown,
  mdiClose,
  mdiCogOutline,
  mdiDeleteOutline,
  mdiDotsHorizontal,
  mdiMenu,
  mdiPencilOutline,
  mdiPlus,
  mdiSend,
  mdiWeatherNight,
  mdiWeatherSunny,
} from '@mdi/js'

import MdiIcon from '@/components/MdiIcon.vue'
import TypingDots from '@/components/chat/TypingDots.vue'
import SettingsPanel from '@/components/settings/SettingsPanel.vue'
import ToolCallCard from '@/components/chat/ToolCallCard.vue'
import BaseDialog from '@/components/ui/BaseDialog.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import SlimeBotLogo from '@/components/ui/SlimeBotLogo.vue'
import { renderMarkdown } from '@/utils/markdown'
import { useHomeChatPage } from '@/composables/useHomeChatPage'
import { useTheme } from '@/composables/useTheme'

const {
  t,
  store,
  drawerOpen,
  renameVisible,
  renameValue,
  inputValue,
  loading,
  isEmptySession,
  settingsVisible,
  toolDetailVisible,
  toolDetailDialogWidth,
  activeSessionMenu,
  topMenuVisible,
  modelOptions,
  selectedModelId,
  modelSelectOptions,
  setMessagesRef,
  setSidebarListRef,
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
  confirmDeleteSession,
  deleteConfirmVisible,
  pickSession,
  createSession,
  sendMessage,
  renameFromFloatingMenu,
  deleteFromFloatingMenu,
  onModelChange,
} = useHomeChatPage()

const { isDark, toggleTheme } = useTheme()

function onTextareaKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    sendMessage()
  }
}

function onTextareaInput(e: Event) {
  const el = e.target as HTMLTextAreaElement
  el.style.height = 'auto'
  el.style.height = `${el.scrollHeight}px`
}
</script>

<template>
  <div class="h-screen flex items-center justify-center p-2 sm:p-3 transition-colors duration-300" style="background: var(--bg-base)">
    <!-- 外层卡片容器 -->
    <div
      class="relative w-full h-full rounded-2xl flex overflow-hidden"
      style="border: 1px solid var(--card-border); background: var(--bg-main); box-shadow: 0 25px 50px rgba(0,0,0,0.15)"
    >

      <!-- ───── 侧边栏 ───── -->
      <Transition name="sidebar">
        <aside
          v-if="drawerOpen"
          class="absolute inset-y-0 left-0 w-64 flex flex-col z-30 backdrop-blur-xl"
          style="background: var(--sidebar-bg); border-right: 1px solid var(--sidebar-border)"
        >
          <!-- 侧边栏顶部：Logo + 新建按钮 -->
          <div class="flex items-center justify-between px-4 h-14" style="border-bottom: 1px solid var(--sidebar-border)">
            <!-- Logo -->
            <div class="flex items-center gap-2.5">
              <SlimeBotLogo :size="36" />
              <span class="text-sm font-semibold" style="color: var(--text-primary)">SlimeBot</span>
            </div>

            <div class="flex items-center gap-1">
              <!-- 新建会话 -->
              <button
                type="button"
                class="w-8 h-8 flex items-center justify-center rounded-lg transition-all duration-150 cursor-pointer group"
                style="color: var(--text-muted)"
                @click="createSession"
              >
                <MdiIcon :path="mdiPlus" :size="20" class="group-hover:scale-110 transition-transform duration-150" />
              </button>
              <!-- 关闭侧边栏 -->
              <button
                type="button"
                class="w-8 h-8 flex items-center justify-center rounded-lg transition-all duration-150 cursor-pointer"
                style="color: var(--text-muted)"
                @click="drawerOpen = false"
              >
                <MdiIcon :path="mdiClose" :size="20" />
              </button>
            </div>
          </div>

          <!-- 会话列表 -->
          <div :ref="setSidebarListRef" class="scroll-area flex-1 overflow-y-auto py-2 px-2">
            <div
              v-for="item in store.sessions"
              :key="item.id"
              class="group relative flex items-center gap-1 px-3 h-9 rounded-xl cursor-pointer transition-all duration-150 mb-0.5"
              :class="item.id === store.currentSessionId
                ? 'session-item-active'
                : 'session-item'"
              @click="pickSession(item.id)"
            >
              <!-- 激活指示条 -->
              <span
                v-if="item.id === store.currentSessionId"
                class="absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-5 rounded-r-full"
                style="background: #6366f1"
              />
              <span class="flex-1 truncate text-sm" style="color: var(--text-primary)">{{ item.name }}</span>
              <button
                type="button"
                class="w-6 h-6 flex items-center justify-center rounded-md transition-colors duration-150 cursor-pointer opacity-0 group-hover:opacity-100 flex-shrink-0"
                :class="item.id === store.currentSessionId ? '!opacity-100' : ''"
                style="color: var(--text-muted)"
                @click.stop="toggleSessionMenu(item.id, $event as MouseEvent)"
              >
                <MdiIcon :path="mdiDotsHorizontal" :size="15" />
              </button>
            </div>
          </div>

          <!-- 侧边栏底部：主题切换 + 设置 -->
          <div class="p-2" style="border-top: 1px solid var(--sidebar-border)">
            <div class="flex items-center gap-1">
              <!-- 主题切换 -->
              <button
                type="button"
                class="w-9 h-9 flex items-center justify-center rounded-xl transition-all duration-150 cursor-pointer flex-shrink-0"
                style="color: var(--text-muted)"
                @click="toggleTheme"
              >
                <MdiIcon :path="isDark ? mdiWeatherSunny : mdiWeatherNight" :size="20" />
              </button>
              <!-- 设置 -->
              <button
                type="button"
                class="flex-1 flex items-center gap-2.5 px-3 h-9 rounded-xl text-sm transition-all duration-150 cursor-pointer"
                style="color: var(--text-secondary)"
                @click="settingsVisible = true"
              >
                <MdiIcon :path="mdiCogOutline" :size="19" />
                <span class="font-medium">{{ t('settings') }}</span>
              </button>
            </div>
          </div>
        </aside>
      </Transition>

      <!-- 侧边栏遮罩 -->
      <Transition name="mask-fade">
        <div
          v-if="drawerOpen"
          class="absolute inset-0 z-20"
          style="background: rgba(0,0,0,0.35)"
          @click="drawerOpen = false"
        />
      </Transition>

      <!-- ───── 主内容区 ───── -->
      <main class="relative z-0 flex-1 flex flex-col min-w-0">

        <!-- 顶栏 -->
        <header
          class="relative z-30 flex items-center justify-center h-14 flex-shrink-0 backdrop-blur-sm"
          style="background: var(--header-bg); border-bottom: 1px solid var(--card-border)"
        >
          <!-- 左侧菜单按钮 -->
          <button
            type="button"
            class="absolute left-3 w-9 h-9 flex items-center justify-center rounded-xl transition-all duration-150 cursor-pointer"
            style="color: var(--text-muted)"
            @click.stop="toggleSidebar"
          >
            <MdiIcon :path="mdiMenu" :size="19" />
          </button>

          <!-- 标题 + 下拉菜单 -->
          <button
            v-if="currentSession"
            type="button"
            class="flex items-center gap-1.5 px-3 py-1.5 rounded-xl transition-all duration-150 cursor-pointer max-w-[260px] header-title-btn"
            @click.stop="topMenuVisible = !topMenuVisible"
          >
            <span class="text-sm font-semibold truncate" style="color: var(--text-primary)">{{ currentSession.name }}</span>
            <MdiIcon
              :path="mdiChevronDown"
              :size="14"
              class="flex-shrink-0 transition-transform duration-200"
              :class="topMenuVisible ? 'rotate-180' : ''"
              style="color: var(--text-muted)"
            />
          </button>

          <!-- 网络状态 badge -->
          <div
            v-if="networkStatusText"
            class="absolute right-3 flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium"
            :class="store.connectionStatus === 'reconnecting'
              ? 'status-warning'
              : 'status-error'"
          >
            <span class="w-1.5 h-1.5 rounded-full animate-pulse" :class="store.connectionStatus === 'reconnecting' ? 'bg-amber-400' : 'bg-red-400'" />
            {{ networkStatusText }}
          </div>

          <!-- 顶部下拉菜单 -->
          <Transition name="top-menu-pop">
            <div
              v-if="topMenuVisible"
              class="absolute top-[52px] left-1/2 -translate-x-1/2 w-40 rounded-xl py-1 overflow-hidden z-[85] top-menu-glass"
              @click.stop
            >
              <button
                v-if="currentSession"
                type="button"
                class="w-full flex items-center gap-2.5 px-3 h-9 text-sm transition-colors duration-150 cursor-pointer menu-item"
                @click="openRename(currentSession.id, currentSession.name); topMenuVisible = false"
              >
                <MdiIcon :path="mdiPencilOutline" :size="14" />
                <span>{{ t('rename') }}</span>
              </button>
              <button
                v-if="currentSession"
                type="button"
                class="w-full flex items-center gap-2.5 px-3 h-9 text-sm transition-colors duration-150 cursor-pointer menu-item-danger"
                @click="removeSession(currentSession.id)"
              >
                <MdiIcon :path="mdiDeleteOutline" :size="14" />
                <span>{{ t('delete') }}</span>
              </button>
            </div>
          </Transition>
        </header>

        <!-- ───── 空会话：居中欢迎 + 输入框 ───── -->
        <template v-if="isEmptySession">
          <div class="flex-1 flex flex-col items-center justify-center px-4 pb-8">
            <!-- 加载中 -->
            <div v-if="loading" class="flex items-center gap-2 text-sm mb-8" style="color: var(--text-muted)">
              <svg class="animate-spin w-4 h-4" style="color: #6366f1" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
              </svg>
              {{ t('loading') }}
            </div>

            <template v-else>
              <!-- AI 渐变图标 -->
              <img src="/slime-icon.svg" alt="SlimeBot AI" class="w-20 h-20 mb-2 object-contain drop-shadow-lg" />

              <!-- 欢迎标题 -->
              <h2 class="text-2xl font-bold mb-2 text-center welcome-title">{{ t('welcomeTitle') }}</h2>
              <p class="text-sm mb-6 text-center" style="color: var(--text-muted)">{{ t('welcomeSubtitle') }}</p>

              <!-- 居中输入框 -->
              <div class="w-full max-w-[640px]">
                <div class="input-container rounded-2xl overflow-hidden">
                  <textarea
                    v-model="inputValue"
                    class="w-full resize-none border-0 outline-none bg-transparent px-4 pt-3.5 pb-12 text-sm leading-relaxed min-h-[88px] max-h-[260px] overflow-y-auto"
                    style="color: var(--text-primary)"
                    :placeholder="t('inputPlaceholder')"
                    rows="1"
                    @keydown="onTextareaKeydown"
                    @input="onTextareaInput"
                  />
                  <div class="absolute bottom-3 left-3 right-3 flex items-center justify-between gap-2">
                    <AppSelect
                      :model-value="selectedModelId"
                      :options="modelSelectOptions"
                      :disabled="modelOptions.length === 0"
                      variant="ghost"
                      size="xs"
                      @update:model-value="onModelChange"
                    />
                    <button
                      type="button"
                      class="w-8 h-8 flex items-center justify-center rounded-xl transition-all duration-150 cursor-pointer flex-shrink-0"
                      :class="sendDisabled ? 'send-btn-disabled' : 'send-btn'"
                      :disabled="sendDisabled"
                      @click="sendMessage"
                    >
                      <MdiIcon :path="mdiSend" :size="15" />
                    </button>
                  </div>
                </div>
              </div>
            </template>
          </div>
        </template>

        <!-- ───── 有消息：消息列表 + 底部输入框 ───── -->
        <template v-else>
          <!-- 消息区 -->
          <section
            :ref="setMessagesRef"
            class="scroll-area flex-1 overflow-y-auto px-4 py-6"
            style="background: var(--bg-main)"
          >
            <div class="flex flex-col gap-5 max-w-[720px] mx-auto">
              <div
                v-for="item in store.messages"
                :key="item.id"
                class="flex gap-3 message-animate"
                :class="[
                  item.role === 'user' ? 'flex-row-reverse' : 'flex-row',
                  item.role === 'user' && store.isFailedUserMessage(item.id)
                    ? 'items-end'
                    : (item.role === 'assistant' && isEmptyPlaceholder(item.id) && store.waiting
                        ? 'items-center'
                        : 'items-start'),
                ]"
              >
                <!-- AI 头像 -->
                <div
                  v-if="item.role === 'assistant'"
                  class="flex-shrink-0 w-10 h-10 flex items-center justify-center"
                >
                  <img src="/slime-icon.svg" alt="SlimeBot AI" class="w-10 h-10 object-contain" />
                </div>

                <!-- 失败图标（用户消息） -->
                <div
                  v-if="item.role === 'user' && store.isFailedUserMessage(item.id)"
                  class="flex-shrink-0"
                  :title="t('sendBlockedOffline')"
                  style="color: #ef4444"
                >
                  <MdiIcon :path="mdiAlert" :size="15" />
                </div>

                <!-- 消息气泡 -->
                <div
                  class="text-sm leading-relaxed"
                  :class="[
                    item.role === 'user'
                      ? 'user-bubble max-w-[calc(100%-52px)] rounded-2xl rounded-tr-sm px-4 py-2.5'
                      : 'w-full',
                    item.role === 'assistant' && store.isAssistantErrorMessage(item.id)
                      ? 'error-bubble rounded-xl px-4 py-3'
                      : '',
                  ]"
                >
                  <template v-if="item.role === 'assistant'">
                    <!-- 工具调用摘要按钮 -->
                    <div v-if="getReplyToolCount(item.id) > 0" class="mb-3">
                      <button
                        type="button"
                        class="tool-summary-btn inline-flex items-center gap-2 px-3 py-1.5 text-xs rounded-full transition-all duration-150 cursor-pointer"
                        @click="openToolDetail(item.id)"
                      >
                        <svg class="w-3.5 h-3.5 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                          <path stroke-linecap="round" stroke-linejoin="round" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
                          <path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                        </svg>
                        {{ t('toolExecutionCount', { count: getReplyToolCount(item.id) }) }}
                      </button>
                    </div>

                    <!-- 工具内联列表 -->
                    <div
                      v-if="getReplyToolCount(item.id) > 0 && !isReplyToolCollapsed(item.id)"
                      class="flex flex-col gap-2 mb-3"
                    >
                      <template v-for="entry in getReplyTimeline(item.id)" :key="entry.id">
                        <div v-if="entry.kind === 'text'" class="bubble-markdown" style="color: var(--text-primary)" v-html="renderMarkdown(entry.content)" />
                        <div v-else-if="entry.kind === 'tool_start'" class="w-full">
                          <ToolCallCard
                            v-if="getReplyToolItem(item.id, entry.toolCallId)"
                            :item="getReplyToolItem(item.id, entry.toolCallId)!"
                            :show-preamble="false"
                            @approve="store.approveToolCall($event, true)"
                            @reject="store.approveToolCall($event, false)"
                          />
                        </div>
                        <div v-else class="text-xs py-0.5" style="color: var(--text-muted)">
                          {{ t('toolExecutionFinished') }}
                        </div>
                      </template>
                    </div>

                    <!-- 普通消息内容 -->
                    <div
                      v-if="getReplyToolCount(item.id) === 0 || isReplyToolCollapsed(item.id)"
                      class="bubble-markdown"
                      style="color: var(--text-primary)"
                      v-html="renderMarkdown(item.content)"
                    />

                    <!-- 打字指示器 -->
                    <TypingDots v-if="isEmptyPlaceholder(item.id) && store.waiting" />
                  </template>

                  <template v-else>
                    {{ item.content }}
                  </template>
                </div>
              </div>
            </div>
          </section>

          <!-- 底部输入区 -->
          <footer
            class="flex-shrink-0 px-4 py-3"
            style="background: var(--header-bg); border-top: 1px solid var(--card-border); backdrop-filter: blur(12px)"
          >
            <div class="max-w-[680px] mx-auto">
              <div class="relative input-container rounded-2xl overflow-hidden">
                <textarea
                  v-model="inputValue"
                  class="w-full resize-none border-0 outline-none bg-transparent px-4 pt-3.5 pb-12 text-sm leading-relaxed min-h-[88px] max-h-[260px] overflow-y-auto"
                  style="color: var(--text-primary)"
                  :placeholder="t('inputPlaceholder')"
                  rows="1"
                  @keydown="onTextareaKeydown"
                  @input="onTextareaInput"
                />
                <div class="absolute bottom-3 left-3 right-3 flex items-center justify-between gap-2">
                  <AppSelect
                    :model-value="selectedModelId"
                    :options="modelSelectOptions"
                    :disabled="modelOptions.length === 0"
                    variant="ghost"
                    size="xs"
                    @update:model-value="onModelChange"
                  />
                  <button
                    type="button"
                    class="w-8 h-8 flex items-center justify-center rounded-xl transition-all duration-150 cursor-pointer flex-shrink-0"
                    :class="sendDisabled ? 'send-btn-disabled' : 'send-btn'"
                    :disabled="sendDisabled"
                    @click="sendMessage"
                  >
                    <MdiIcon :path="mdiSend" :size="15" />
                  </button>
                </div>
              </div>
            </div>
          </footer>
        </template>
      </main>
    </div>

    <!-- ───── 重命名弹窗 ───── -->
    <BaseDialog
      v-model:visible="renameVisible"
      :title="t('rename')"
      :confirm-text="t('confirm')"
      :cancel-text="t('cancel')"
      width="360px"
      @confirm="confirmRename"
    >
      <input
        v-model="renameValue"
        type="text"
        class="w-full px-3 py-2.5 text-sm rounded-xl outline-none transition-all duration-150 dialog-input"
        @keydown.enter="confirmRename"
      />
    </BaseDialog>

    <!-- ───── 删除确认弹窗 ───── -->
    <BaseDialog
      v-model:visible="deleteConfirmVisible"
      :title="t('delete')"
      :confirm-text="t('confirm')"
      :cancel-text="t('cancel')"
      :confirm-danger="true"
      width="360px"
      @confirm="confirmDeleteSession"
    >
      <p class="text-sm" style="color: var(--text-secondary)">{{ t('confirmDelete') }}</p>
    </BaseDialog>

    <!-- ───── 工具调用详情弹窗 ───── -->
    <BaseDialog
      v-model:visible="toolDetailVisible"
      :title="t('toolExecutionDetailTitle')"
      :cancel-text="t('close')"
      :width="toolDetailDialogWidth"
      hide-footer
    >
      <div class="flex flex-col gap-3 max-h-[60vh] overflow-y-auto pr-1">
        <template v-for="entry in toolDetailToolTimeline" :key="entry.id">
          <ToolCallCard
            v-if="entry.kind === 'tool_start' && toolDetailItems.find((tc) => tc.toolCallId === entry.toolCallId)"
            :item="toolDetailItems.find((tc) => tc.toolCallId === entry.toolCallId)!"
            :show-preamble="true"
            @approve="store.approveToolCall($event, true)"
            @reject="store.approveToolCall($event, false)"
          />
          <div v-else-if="entry.kind === 'tool_result'" class="text-xs py-0.5" style="color: var(--text-muted)">
            {{ t('toolExecutionFinished') }}
          </div>
        </template>
      </div>
      <div class="flex justify-end mt-4 pt-3" style="border-top: 1px solid var(--card-border)">
        <button
          type="button"
          class="px-4 py-1.5 text-sm rounded-xl transition-all duration-150 cursor-pointer dialog-cancel-btn"
          @click="toolDetailVisible = false"
        >
          {{ t('close') }}
        </button>
      </div>
    </BaseDialog>

    <!-- ───── 设置弹窗 ───── -->
    <Transition name="overlay-fade">
      <div
        v-if="settingsVisible"
        class="fixed inset-0 z-[100] flex items-center justify-center p-4 sm:p-6"
        style="background: rgba(0,0,0,0.45)"
        @click.self="settingsVisible = false"
      >
        <div
          class="w-full rounded-2xl overflow-hidden settings-modal"
          style="max-width: min(86vw, 1080px); height: min(80vh, 760px)"
          @click.stop
        >
          <SettingsPanel @close="settingsVisible = false" @llm-changed="refreshModelOptions" />
        </div>
      </div>
    </Transition>

    <!-- ───── 浮动会话菜单 ───── -->
    <Transition name="session-menu-pop">
      <div
        v-if="activeSessionMenu"
        class="fixed z-[80] w-40 rounded-xl py-1 overflow-hidden"
        :style="{ left: `${activeSessionMenu.x}px`, top: `${activeSessionMenu.y}px`, background: 'var(--menu-bg)', border: '1px solid var(--menu-border)', boxShadow: 'var(--menu-shadow)' }"
        @click.stop
      >
        <button
          type="button"
          class="w-full flex items-center gap-2.5 px-3 h-9 text-sm transition-colors duration-150 cursor-pointer menu-item"
          @click="renameFromFloatingMenu"
        >
          <MdiIcon :path="mdiPencilOutline" :size="14" />
          <span>{{ t('rename') }}</span>
        </button>
        <button
          type="button"
          class="w-full flex items-center gap-2.5 px-3 h-9 text-sm transition-colors duration-150 cursor-pointer menu-item-danger"
          @click="deleteFromFloatingMenu"
        >
          <MdiIcon :path="mdiDeleteOutline" :size="14" />
          <span>{{ t('delete') }}</span>
        </button>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
@import './home-page.css';

/* ── 会话项 ── */
.session-item {
  color: var(--text-secondary);
}
.session-item:hover {
  background: rgba(99, 102, 241, 0.08);
  color: var(--text-primary);
}
.session-item-active {
  background: rgba(99, 102, 241, 0.12);
  color: var(--text-primary);
}

/* ── 顶栏标题按钮 ── */
.header-title-btn:hover {
  background: rgba(99, 102, 241, 0.08);
}

/* ── 菜单项 ── */
.menu-item {
  color: var(--text-secondary);
}
.menu-item:hover {
  background: rgba(99, 102, 241, 0.08);
  color: var(--text-primary);
}
.menu-item-danger {
  color: #ef4444;
}
.menu-item-danger:hover {
  background: rgba(239, 68, 68, 0.08);
}

/* ── 状态 badge ── */
.status-warning {
  background: rgba(245, 158, 11, 0.12);
  color: #f59e0b;
  border: 1px solid rgba(245, 158, 11, 0.2);
}
.status-error {
  background: rgba(239, 68, 68, 0.12);
  color: #ef4444;
  border: 1px solid rgba(239, 68, 68, 0.2);
}

/* ── 欢迎标题渐变 ── */
.welcome-title {
  background: linear-gradient(135deg, #6366f1 0%, #a78bfa 50%, #818cf8 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

/* ── 输入框容器 ── */
.input-container {
  position: relative;
  background: var(--input-bg);
  border: 1px solid var(--input-border);
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}
.input-container:focus-within {
  border-color: #6366f1;
  box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.12);
}

textarea::placeholder {
  color: var(--text-muted);
}

/* ── 发送按钮 ── */
.send-btn {
  background: linear-gradient(135deg, #10b981 0%, #059669 100%);
  color: white;
  box-shadow: 0 2px 8px rgba(16, 185, 129, 0.35);
}
.send-btn:hover {
  box-shadow: 0 4px 12px rgba(16, 185, 129, 0.45);
  transform: scale(1.05);
}
.send-btn-disabled {
  background: rgba(99, 102, 241, 0.08);
  color: var(--text-muted);
  cursor: not-allowed;
}

/* ── 用户气泡 ── */
.user-bubble {
  background: var(--user-bubble-bg);
  color: var(--user-bubble-text);
  box-shadow: 0 2px 12px rgba(99, 102, 241, 0.25);
}

/* ── 错误消息气泡 ── */
.error-bubble {
  background: rgba(239, 68, 68, 0.08);
  border: 1px solid rgba(239, 68, 68, 0.2);
  color: #ef4444;
}

/* ── 工具调用摘要按钮 ── */
.tool-summary-btn {
  background: rgba(99, 102, 241, 0.08);
  border: 1px solid rgba(99, 102, 241, 0.2);
  color: #6366f1;
}
.tool-summary-btn:hover {
  background: rgba(99, 102, 241, 0.14);
}

/* ── 设置弹窗 ── */
.settings-modal {
  background: var(--bg-main);
  border: 1px solid var(--card-border);
  box-shadow: 0 25px 60px rgba(0, 0, 0, 0.3);
}

/* ── Dialog 输入框 ── */
.dialog-input {
  background: var(--input-bg);
  border: 1px solid var(--input-border);
  color: var(--text-primary);
}
.dialog-input:focus {
  border-color: #6366f1;
  box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.12);
}

/* ── Dialog 取消按钮 ── */
.dialog-cancel-btn {
  background: var(--input-bg);
  border: 1px solid var(--input-border);
  color: var(--text-secondary);
}
.dialog-cancel-btn:hover {
  background: rgba(99, 102, 241, 0.08);
}
</style>
