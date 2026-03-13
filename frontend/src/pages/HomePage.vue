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
  mdiRobotOutline,
  mdiSend,
} from '@mdi/js'

import MdiIcon from '@/components/MdiIcon.vue'
import TypingDots from '@/components/chat/TypingDots.vue'
import SettingsPanel from '@/components/settings/SettingsPanel.vue'
import ToolCallCard from '@/components/chat/ToolCallCard.vue'
import BaseDialog from '@/components/ui/BaseDialog.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import { renderMarkdown } from '@/utils/markdown'
import { useHomeChatPage } from '@/composables/useHomeChatPage'

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
  <div class="h-screen bg-gray-50 flex items-center justify-center p-2 sm:p-3">
    <!-- 外层卡片容器 -->
    <div class="relative w-full h-full bg-white rounded-xl border border-gray-200 flex overflow-hidden shadow-sm">

      <!-- ───── 侧边栏 ───── -->
      <Transition name="sidebar">
        <aside
          v-if="drawerOpen"
          class="absolute inset-y-0 left-0 w-60 bg-gray-50 border-r border-gray-200 flex flex-col z-30"
        >
          <!-- 侧边栏顶部 -->
          <div class="flex items-center justify-between px-3 h-12 border-b border-gray-100">
            <button
              type="button"
              class="w-8 h-8 flex items-center justify-center rounded-lg text-gray-500 hover:text-gray-800 hover:bg-gray-200 transition-colors duration-150 cursor-pointer"
              @click="createSession"
            >
              <MdiIcon :path="mdiPlus" :size="18" />
            </button>
            <button
              type="button"
              class="w-8 h-8 flex items-center justify-center rounded-lg text-gray-500 hover:text-gray-800 hover:bg-gray-200 transition-colors duration-150 cursor-pointer"
              @click="drawerOpen = false"
            >
              <MdiIcon :path="mdiClose" :size="18" />
            </button>
          </div>

          <!-- 会话列表 -->
          <div :ref="setSidebarListRef" class="scroll-area flex-1 overflow-y-auto py-2 px-2">
            <div
              v-for="item in store.sessions"
              :key="item.id"
              class="group relative flex items-center gap-1 px-2.5 h-9 rounded-lg cursor-pointer transition-colors duration-150 mb-0.5"
              :class="item.id === store.currentSessionId
                ? 'bg-gray-200 text-gray-900'
                : 'text-gray-700 hover:bg-gray-100'"
              @click="pickSession(item.id)"
            >
              <span class="flex-1 truncate text-sm">{{ item.name }}</span>
              <button
                type="button"
                class="w-6 h-6 flex items-center justify-center rounded-md text-gray-400 hover:text-gray-700 hover:bg-gray-300 transition-colors duration-150 cursor-pointer opacity-0 group-hover:opacity-100 flex-shrink-0"
                :class="item.id === store.currentSessionId ? 'opacity-100' : ''"
                @click.stop="toggleSessionMenu(item.id, $event as MouseEvent)"
              >
                <MdiIcon :path="mdiDotsHorizontal" :size="16" />
              </button>
            </div>
          </div>

          <!-- 侧边栏底部 -->
          <div class="border-t border-gray-200 p-2">
            <button
              type="button"
              class="w-full flex items-center gap-2 px-3 h-9 rounded-lg text-sm text-gray-600 hover:bg-gray-200 hover:text-gray-900 transition-colors duration-150 cursor-pointer"
              @click="settingsVisible = true"
            >
              <MdiIcon :path="mdiCogOutline" :size="18" />
              <span>{{ t('settings') }}</span>
            </button>
          </div>
        </aside>
      </Transition>

      <!-- 侧边栏遮罩 -->
      <Transition name="mask-fade">
        <div
          v-if="drawerOpen"
          class="absolute inset-0 bg-black/20 z-20"
          @click="drawerOpen = false"
        />
      </Transition>

      <!-- ───── 主内容区 ───── -->
      <main class="flex-1 flex flex-col min-w-0 overflow-hidden">

        <!-- 顶栏 -->
        <header class="relative flex items-center justify-center h-12 border-b border-gray-100 bg-white flex-shrink-0">
          <!-- 左侧菜单按钮 -->
          <button
            type="button"
            class="absolute left-3 w-8 h-8 flex items-center justify-center rounded-lg text-gray-500 hover:text-gray-800 hover:bg-gray-100 transition-colors duration-150 cursor-pointer"
            @click.stop="toggleSidebar"
          >
            <MdiIcon :path="mdiMenu" :size="18" />
          </button>

          <!-- 标题 + 下拉菜单（仅在有会话时显示） -->
          <button
            v-if="currentSession"
            type="button"
            class="flex items-center gap-1 px-2 py-1 rounded-lg text-gray-800 hover:bg-gray-100 transition-colors duration-150 cursor-pointer max-w-xs"
            @click.stop="topMenuVisible = !topMenuVisible"
          >
            <span class="text-sm font-medium truncate">{{ currentSession.name }}</span>
            <MdiIcon :path="mdiChevronDown" :size="14" class="flex-shrink-0 text-gray-500" />
          </button>

          <!-- 网络状态 -->
          <span
            v-if="networkStatusText"
            class="absolute right-3 text-xs font-medium"
            :class="store.connectionStatus === 'reconnecting' ? 'text-amber-500' : 'text-red-500'"
          >
            {{ networkStatusText }}
          </span>

          <!-- 顶部菜单 -->
          <Transition name="top-menu-pop">
            <div
              v-if="topMenuVisible"
              class="absolute top-11 left-1/2 -translate-x-1/2 w-36 bg-white border border-gray-200 rounded-xl shadow-lg z-50 py-1 overflow-hidden"
              @click.stop
            >
              <button
                v-if="currentSession"
                type="button"
                class="w-full flex items-center gap-2 px-3 h-9 text-sm text-gray-700 hover:bg-gray-50 transition-colors duration-150 cursor-pointer"
                @click="openRename(currentSession.id, currentSession.name); topMenuVisible = false"
              >
                <MdiIcon :path="mdiPencilOutline" :size="14" />
                <span>{{ t('rename') }}</span>
              </button>
              <button
                v-if="currentSession"
                type="button"
                class="w-full flex items-center gap-2 px-3 h-9 text-sm text-red-500 hover:bg-red-50 transition-colors duration-150 cursor-pointer"
                @click="removeSession(currentSession.id)"
              >
                <MdiIcon :path="mdiDeleteOutline" :size="14" />
                <span>{{ t('delete') }}</span>
              </button>
            </div>
          </Transition>
        </header>

        <!-- ───── 空会话：居中输入框 ───── -->
        <template v-if="isEmptySession">
          <div class="flex-1 flex flex-col items-center justify-center px-4 pb-8">
            <!-- 加载中 -->
            <div v-if="loading" class="flex items-center text-sm text-gray-400 mb-8">
              <svg class="animate-spin w-4 h-4 mr-2 text-blue-500" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
              </svg>
              {{ t('loading') }}
            </div>

            <template v-else>
              <!-- 欢迎标题 -->
              <h2 class="text-xl font-semibold text-gray-800 mb-6 text-center">{{ t('welcomeTitle') }}</h2>

              <!-- 居中输入框 -->
              <div class="w-full max-w-[620px]">
                <div class="relative border border-gray-200 rounded-2xl bg-white focus-within:border-blue-400 focus-within:ring-2 focus-within:ring-blue-100 transition-all duration-200 overflow-hidden shadow-sm">
                  <textarea
                    v-model="inputValue"
                    class="w-full resize-none border-0 outline-none bg-transparent px-4 pt-3 pb-10 text-sm text-gray-800 placeholder-gray-400 leading-relaxed min-h-[80px] max-h-[260px] overflow-y-auto"
                    :placeholder="t('inputPlaceholder')"
                    rows="1"
                    @keydown="onTextareaKeydown"
                    @input="onTextareaInput"
                  />
                  <div class="absolute bottom-2 left-3 right-3 flex items-center justify-end gap-2">
                    <!-- 模型选择 -->
                    <AppSelect
                      :model-value="selectedModelId"
                      :options="modelSelectOptions"
                      :disabled="modelOptions.length === 0"
                      variant="ghost"
                      size="xs"
                      @update:model-value="onModelChange"
                    />

                    <!-- 发送按钮 -->
                    <button
                      type="button"
                      class="w-7 h-7 flex items-center justify-center rounded-full transition-colors duration-150 cursor-pointer flex-shrink-0"
                      :class="sendDisabled
                        ? 'bg-gray-200 text-gray-400 cursor-not-allowed'
                        : 'bg-blue-500 text-white hover:bg-blue-600'"
                      :disabled="sendDisabled"
                      @click="sendMessage"
                    >
                      <MdiIcon :path="mdiSend" :size="14" />
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
            class="scroll-area flex-1 overflow-y-auto px-4 py-6 bg-white"
          >
            <!-- 消息列表 -->
            <div class="flex flex-col gap-6 max-w-[688px] mx-auto">
              <div
                v-for="item in store.messages"
                :key="item.id"
                class="flex gap-3"
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
                  class="flex-shrink-0 w-8 h-8 rounded-full bg-gray-100 border border-gray-200 flex items-center justify-center text-gray-600"
                >
                  <MdiIcon :path="mdiRobotOutline" :size="18" />
                </div>

                <!-- 失败图标（用户消息） -->
                <div
                  v-if="item.role === 'user' && store.isFailedUserMessage(item.id)"
                  class="flex-shrink-0 text-red-500"
                  :title="t('sendBlockedOffline')"
                >
                  <MdiIcon :path="mdiAlert" :size="14" />
                </div>

                <!-- 消息气泡 -->
                <div
                  class="text-sm leading-relaxed"
                  :class="[
                    item.role === 'user'
                      ? 'max-w-[calc(100%-44px)] bg-gray-100 text-gray-900 rounded-2xl rounded-tr-sm px-4 py-2.5'
                      : 'w-full text-gray-900',
                    item.role === 'assistant' && store.isAssistantErrorMessage(item.id)
                      ? 'bg-red-50 border border-red-200 rounded-xl px-4 py-3 text-red-700'
                      : '',
                  ]"
                >
                  <template v-if="item.role === 'assistant'">
                    <!-- 工具调用摘要 -->
                    <div v-if="getReplyToolCount(item.id) > 0" class="mb-2">
                      <button
                        type="button"
                        class="inline-flex items-center gap-1.5 px-3 py-1 text-xs rounded-full border border-gray-200 bg-gray-50 text-gray-600 hover:bg-gray-100 transition-colors duration-150 cursor-pointer"
                        @click="openToolDetail(item.id)"
                      >
                        <svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
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
                        <div v-if="entry.kind === 'text'" class="bubble-markdown" v-html="renderMarkdown(entry.content)" />
                        <div v-else-if="entry.kind === 'tool_start'" class="w-full">
                          <ToolCallCard
                            v-if="getReplyToolItem(item.id, entry.toolCallId)"
                            :item="getReplyToolItem(item.id, entry.toolCallId)!"
                            :show-preamble="false"
                            @approve="store.approveToolCall($event, true)"
                            @reject="store.approveToolCall($event, false)"
                          />
                        </div>
                        <div v-else class="text-xs text-gray-400 py-0.5">
                          {{ t('toolExecutionFinished') }}
                        </div>
                      </template>
                    </div>

                    <!-- 普通消息内容 -->
                    <div
                      v-if="getReplyToolCount(item.id) === 0 || isReplyToolCollapsed(item.id)"
                      class="bubble-markdown"
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

          <!-- 输入区 -->
          <footer class="flex-shrink-0 px-4 py-3 bg-white border-t border-gray-100">
            <div class="max-w-[620px] mx-auto">
              <div class="relative border border-gray-200 rounded-2xl bg-white focus-within:border-blue-400 focus-within:ring-2 focus-within:ring-blue-100 transition-all duration-200 overflow-hidden">
                <textarea
                  v-model="inputValue"
                  class="w-full resize-none border-0 outline-none bg-transparent px-4 pt-3 pb-10 text-sm text-gray-800 placeholder-gray-400 leading-relaxed min-h-[80px] max-h-[260px] overflow-y-auto"
                  :placeholder="t('inputPlaceholder')"
                  rows="1"
                  @keydown="onTextareaKeydown"
                  @input="onTextareaInput"
                />
                <div class="absolute bottom-2 left-3 right-3 flex items-center justify-end gap-2">
                  <!-- 模型选择 -->
                  <AppSelect
                    :model-value="selectedModelId"
                    :options="modelSelectOptions"
                    :disabled="modelOptions.length === 0"
                    variant="ghost"
                    size="xs"
                    @update:model-value="onModelChange"
                  />

                  <!-- 发送按钮 -->
                  <button
                    type="button"
                    class="w-7 h-7 flex items-center justify-center rounded-full transition-colors duration-150 cursor-pointer flex-shrink-0"
                    :class="sendDisabled
                      ? 'bg-gray-200 text-gray-400 cursor-not-allowed'
                      : 'bg-blue-500 text-white hover:bg-blue-600'"
                    :disabled="sendDisabled"
                    @click="sendMessage"
                  >
                    <MdiIcon :path="mdiSend" :size="14" />
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
        class="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-100 transition-all duration-150"
        @keydown.enter="confirmRename"
      />
    </BaseDialog>

    <!-- ───── 删除会话确认弹窗 ───── -->
    <BaseDialog
      v-model:visible="deleteConfirmVisible"
      :title="t('delete')"
      :confirm-text="t('confirm')"
      :cancel-text="t('cancel')"
      :confirm-danger="true"
      width="360px"
      @confirm="confirmDeleteSession"
    >
      <p class="text-sm text-gray-700">{{ t('confirmDelete') }}</p>
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
          <div v-else-if="entry.kind === 'tool_result'" class="text-xs text-gray-400 py-0.5">
            {{ t('toolExecutionFinished') }}
          </div>
        </template>
      </div>
      <div class="flex justify-end mt-4 pt-3 border-t border-gray-100">
        <button
          type="button"
          class="px-4 py-1.5 text-sm rounded-lg border border-gray-200 text-gray-600 bg-white hover:bg-gray-50 transition-colors duration-150 cursor-pointer"
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
        style="background: rgba(0,0,0,0.3)"
        @click.self="settingsVisible = false"
      >
        <div
          class="w-full bg-white rounded-xl shadow-2xl overflow-hidden"
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
        class="fixed z-[80] w-36 bg-white border border-gray-200 rounded-xl shadow-lg py-1 overflow-hidden"
        :style="{ left: `${activeSessionMenu.x}px`, top: `${activeSessionMenu.y}px` }"
        @click.stop
      >
        <button
          type="button"
          class="w-full flex items-center gap-2 px-3 h-9 text-sm text-gray-700 hover:bg-gray-50 transition-colors duration-150 cursor-pointer"
          @click="renameFromFloatingMenu"
        >
          <MdiIcon :path="mdiPencilOutline" :size="14" />
          <span>{{ t('rename') }}</span>
        </button>
        <button
          type="button"
          class="w-full flex items-center gap-2 px-3 h-9 text-sm text-red-500 hover:bg-red-50 transition-colors duration-150 cursor-pointer"
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
</style>
