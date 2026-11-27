<script setup lang="ts">
import {
  mdiAlert,
  mdiClose,
  mdiContentCopy,
  mdiFile,
  mdiFileCodeOutline,
  mdiFileDocumentOutline,
  mdiFileExcelOutline,
  mdiFileImageOutline,
  mdiMusic,
  mdiFolderZipOutline,
  mdiPencilOutline,
  mdiSend,
} from '@mdi/js'
import { computed, nextTick, ref, unref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MessageItem } from '@/api/chat'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import TruncationTooltip from '@/components/ui/TruncationTooltip.vue'
import AssistantMessageBody from '@/components/chat/AssistantMessageBody.vue'
import { formatSize } from '@/utils/format'
import { useChatContext } from '@/composables/chat/useChatContext'
import { useToast } from '@/composables/useToast'

const props = defineProps<{
  item: MessageItem
}>()

const ctx = useChatContext()
const { t } = useI18n()
const toast = useToast()
type BubbleMode = 'view' | 'edit'
type BubbleTransitionStage = 'idle' | 'fade-out' | 'morph' | 'fade-in'
const bubbleContentFadeMs = 140

const bubbleMode = ref<BubbleMode>('view')
const bubbleTransitionStage = ref<BubbleTransitionStage>('idle')
const draftContent = ref('')
const messageItemRef = ref<HTMLElement | null>(null)
const bubbleRef = ref<HTMLElement | null>(null)
const textareaRef = ref<HTMLTextAreaElement | null>(null)

const isEditing = computed(() => bubbleMode.value === 'edit')
const isBubbleAnimating = computed(() => bubbleTransitionStage.value !== 'idle')
const isBubbleContentHidden = computed(
  () => bubbleTransitionStage.value === 'fade-out' || bubbleTransitionStage.value === 'morph',
)
const canEdit = computed(() => props.item.role === 'user' && props.item.id === unref(ctx.latestEditableUserMessageId))
const canSendEdit = computed(() => draftContent.value.trim() !== '' && !unref(ctx.waiting) && !isBubbleAnimating.value)

function wait(ms: number) {
  return new Promise<void>(resolve => window.setTimeout(resolve, ms))
}

function nextFrame() {
  return new Promise<void>(resolve => window.requestAnimationFrame(() => resolve()))
}

function waitForSizeTransition(el: HTMLElement) {
  return new Promise<void>(resolve => {
    let done = false
    const finish = () => {
      if (done) return
      done = true
      el.removeEventListener('transitionend', onEnd)
      resolve()
    }
    const onEnd = (event: TransitionEvent) => {
      if (event.target === el && (event.propertyName === 'width' || event.propertyName === 'height')) {
        finish()
      }
    }
    el.addEventListener('transitionend', onEnd)
    window.setTimeout(finish, 430)
  })
}

function clearSizeTransitionStyles(el: HTMLElement) {
  el.style.transition = ''
  el.style.width = ''
  el.style.height = ''
  el.style.overflow = ''
  el.style.willChange = ''
}

async function switchBubbleMode(nextMode: BubbleMode, options: { clearDraftAfter?: boolean; focusAfter?: boolean } = {}) {
  if (isBubbleAnimating.value || bubbleMode.value === nextMode) return false

  const bubble = bubbleRef.value
  const messageItem = messageItemRef.value
  bubbleTransitionStage.value = 'fade-out'
  await wait(bubbleContentFadeMs)

  const fromItemRect = messageItem?.getBoundingClientRect()
  const fromRect = bubble?.getBoundingClientRect()
  bubbleMode.value = nextMode
  await nextTick()
  const toItemRect = messageItem?.getBoundingClientRect()
  const toRect = bubble?.getBoundingClientRect()

  bubbleTransitionStage.value = 'morph'
  const canAnimateBubble = bubble && fromRect && toRect && toRect.width > 0 && toRect.height > 0
  const canAnimateItem = messageItem && fromItemRect && toItemRect && toItemRect.height > 0
  if (canAnimateBubble || canAnimateItem) {
    if (messageItem && fromItemRect && toItemRect) {
      messageItem.style.transition = 'none'
      messageItem.style.height = `${fromItemRect.height}px`
      messageItem.style.overflow = 'hidden'
      messageItem.style.willChange = 'height'
    }
    if (bubble && fromRect && toRect) {
      bubble.style.transition = 'none'
      bubble.style.width = `${fromRect.width}px`
      bubble.style.height = `${fromRect.height}px`
      bubble.style.willChange = 'width, height'
    }
    await nextFrame()
    const sizeTransition = '360ms cubic-bezier(0.22, 1, 0.36, 1)'
    const transitions: Promise<void>[] = []
    if (messageItem && toItemRect) {
      messageItem.style.transition = `height ${sizeTransition}`
      messageItem.style.height = `${toItemRect.height}px`
      transitions.push(waitForSizeTransition(messageItem))
    }
    if (bubble && toRect) {
      bubble.style.transition = `width ${sizeTransition}, height ${sizeTransition}`
      bubble.style.width = `${toRect.width}px`
      bubble.style.height = `${toRect.height}px`
      transitions.push(waitForSizeTransition(bubble))
    }
    await Promise.all(transitions)
    if (messageItem) clearSizeTransitionStyles(messageItem)
    if (bubble) clearSizeTransitionStyles(bubble)
  } else {
    await wait(360)
  }

  bubbleTransitionStage.value = 'fade-in'
  await wait(bubbleContentFadeMs)
  bubbleTransitionStage.value = 'idle'

  if (options.clearDraftAfter) {
    draftContent.value = ''
  }
  if (options.focusAfter) {
    await nextTick()
    textareaRef.value?.focus()
  }
  return true
}

function attachmentIcon(iconType?: string, category?: string) {
  if (iconType === 'audio' || category === 'audio') {
    return mdiMusic
  }
  switch (iconType) {
    case 'image':
      return mdiFileImageOutline
    case 'pdf':
      return mdiFileDocumentOutline
    case 'word':
      return mdiFileDocumentOutline
    case 'excel':
      return mdiFileExcelOutline
    case 'archive':
      return mdiFolderZipOutline
    case 'code':
      return mdiFileCodeOutline
    default:
      if (category === 'document') {
        return mdiFileDocumentOutline
      }
      return mdiFile
  }
}

async function copyMessage() {
  try {
    await navigator.clipboard.writeText(props.item.content)
    toast.success(t('messageCopySuccess'))
  } catch {
    toast.error(t('messageCopyFailed'))
  }
}

async function startEdit() {
  if (!canEdit.value || isBubbleAnimating.value) return
  draftContent.value = props.item.content
  await switchBubbleMode('edit', { focusAfter: true })
}

async function cancelEdit() {
  if (isBubbleAnimating.value) return
  await switchBubbleMode('view', { clearDraftAfter: true })
}

async function submitEdit() {
  if (isBubbleAnimating.value) return
  if (!canSendEdit.value) {
    toast.warning(t('messageEditEmpty'))
    return
  }
  const sent = await ctx.sendEditedMessage(props.item.id, draftContent.value)
  if (sent) {
    await switchBubbleMode('view', { clearDraftAfter: true })
    return
  }
  toast.error(t('messageEditFailed'))
}

function handleEditKeydown(event: KeyboardEvent) {
  if (event.isComposing) return
  if (event.key === 'Escape') {
    event.preventDefault()
    void cancelEdit()
    return
  }
  if (event.key !== 'Enter' || event.shiftKey) return
  event.preventDefault()
  void submitEdit()
}
</script>

<template>
  <div
    ref="messageItemRef"
    class="flex min-w-0 message-animate"
    :class="[
      item.role === 'user' ? 'user-message-item flex-row-reverse gap-3' : 'flex-row',
      item.role === 'user' && ctx.isFailedUserMessage(item.id)
        ? 'items-end'
        : 'items-start',
    ]"
  >
    <div
      v-if="item.role === 'user' && ctx.isFailedUserMessage(item.id)"
      class="failed-user-icon flex-shrink-0"
      :title="unref(ctx.sendBlockedOfflineText)"
    >
      <MdiIcon :path="mdiAlert" :size="15" />
    </div>

    <div
      class="min-w-0 flex flex-col text-sm leading-relaxed"
      :class="[
        item.role === 'user'
          ? ['user-message-shell items-end max-w-[calc(100%-52px)]', isEditing ? 'user-message-shell--editing' : 'user-message-shell--actions']
          : 'w-full',
      ]"
    >
      <div
        ref="bubbleRef"
        class="min-w-0"
        :class="[
          item.role === 'user'
            ? [
                'user-bubble rounded-2xl rounded-tr-sm',
                isEditing ? 'user-bubble--editing w-full' : 'w-full',
                isBubbleAnimating ? 'user-bubble--animating' : '',
                isEditing
                  ? 'px-4 py-[12.8px]'
                  : [
                      'px-4',
                      item.attachments && item.attachments.length > 0 ? 'py-2.5' : 'py-2',
                    ],
              ]
            : 'w-full',
          item.role === 'assistant' && ctx.isAssistantErrorMessage(item.id)
            ? 'error-bubble rounded-xl px-4 py-3'
            : '',
        ]"
      >
        <AssistantMessageBody v-if="item.role === 'assistant'" :item="item" />
        <template v-else>
          <div
            class="user-bubble-content"
            :class="isBubbleContentHidden ? 'user-bubble-content--hidden' : ''"
          >
            <div
              v-if="item.attachments && item.attachments.length > 0"
              class="user-attachments-row"
              :class="item.content === '' && !isEditing ? 'user-attachments-row--solo' : ''"
            >
              <div
                v-for="(file, idx) in item.attachments"
                :key="`${file.name}-${idx}`"
                class="user-attachment-card"
              >
                <div class="group/tip flex min-w-0 items-center gap-2 mb-1">
                  <MdiIcon class="flex-shrink-0" :path="attachmentIcon(file.iconType, file.category)" :size="16" />
                  <TruncationTooltip
                    inherit-group
                    :text="file.name"
                    wrapper-class="min-w-0 flex-1"
                    content-class="user-attachment-name text-xs font-medium"
                  />
                </div>
                <div class="text-[11px] opacity-80">
                  {{ file.ext }} · {{ formatSize(file.sizeBytes) }}
                </div>
              </div>
            </div>
            <div v-if="isEditing" class="message-edit-box">
              <textarea
                ref="textareaRef"
                v-model="draftContent"
                class="message-edit-input"
                rows="4"
                @keydown="handleEditKeydown"
              />
              <div class="message-edit-actions">
                <button type="button" class="message-action-btn message-action-btn--text" :title="t('cancel')" @click="cancelEdit">
                  <MdiIcon :path="mdiClose" :size="14" />
                  <span>{{ t('cancel') }}</span>
                </button>
                <button type="button" class="message-action-btn message-action-btn--primary" :disabled="!canSendEdit" :title="t('send')" @click="submitEdit">
                  <MdiIcon :path="mdiSend" :size="14" />
                  <span>{{ t('send') }}</span>
                </button>
              </div>
            </div>
            <div v-else-if="item.content !== ''" class="user-message-content">{{ item.content }}</div>
            <div v-else class="user-message-content user-message-content--empty" />
          </div>
        </template>
      </div>
      <div
        v-if="item.role === 'user' && !isEditing"
        class="user-message-actions"
        :class="isBubbleContentHidden ? 'user-message-actions--hidden' : ''"
      >
        <button type="button" class="message-action-icon" :title="t('copyMessage')" @click="copyMessage">
          <MdiIcon :path="mdiContentCopy" :size="14" />
        </button>
        <button v-if="canEdit" type="button" class="message-action-icon" :disabled="isBubbleAnimating" :title="t('editMessage')" @click="startEdit">
          <MdiIcon :path="mdiPencilOutline" :size="14" />
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.user-message-item {
  margin-block: 2px 8px;
}

.user-message-shell {
  position: relative;
}

.user-bubble {
  position: relative;
  overflow: hidden;
  transform-origin: top right;
  border: 1px solid var(--user-bubble-border);
  transition:
    color 0.25s cubic-bezier(0.22, 1, 0.36, 1),
    border-color 0.25s cubic-bezier(0.22, 1, 0.36, 1),
    box-shadow 0.25s cubic-bezier(0.22, 1, 0.36, 1);
}

.user-bubble::before {
  content: '';
  position: absolute;
  inset: 0;
  z-index: 0;
  pointer-events: none;
  background: var(--bg-main);
  opacity: 0;
  transition: opacity 0.25s cubic-bezier(0.22, 1, 0.36, 1);
}

.user-bubble--editing::before {
  opacity: 1;
}

.user-bubble--animating {
  overflow: hidden;
}

.user-bubble-content {
  position: relative;
  z-index: 1;
  opacity: 1;
  transition: opacity 0.14s ease;
}

.user-bubble-content--hidden {
  opacity: 0;
  pointer-events: none;
}

.user-message-shell--editing {
  width: 100%;
}

.user-message-shell--actions {
  padding-bottom: 0;
}

.user-message-shell--actions::after {
  content: '';
  position: absolute;
  top: 100%;
  right: 0;
  width: min(100%, 72px);
  height: 26px;
  pointer-events: auto;
}

.user-message-actions {
  position: absolute;
  top: calc(100% + 5px);
  right: 2px;
  z-index: 1;
  display: flex;
  gap: 4px;
  min-height: 16px;
  opacity: 0;
  pointer-events: none;
  transition: opacity 0.22s ease;
}

.user-message-shell:hover .user-message-actions,
.user-message-shell:focus-within .user-message-actions {
  opacity: 1;
  pointer-events: auto;
}

.user-message-actions--hidden,
.user-message-shell:hover .user-message-actions--hidden,
.user-message-shell:focus-within .user-message-actions--hidden {
  opacity: 0;
  pointer-events: none;
}

@media (hover: none), (pointer: coarse) {
  .user-message-actions {
    opacity: 1;
    pointer-events: auto;
  }

  .user-message-actions--hidden {
    opacity: 0;
    pointer-events: none;
  }
}

.message-action-icon,
.message-action-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 0;
  background: color-mix(in srgb, var(--bg-main) 88%, transparent);
  color: var(--text-secondary);
  transition: all 0.15s ease;
  cursor: pointer;
}

.message-action-icon {
  width: 16px;
  height: 16px;
  padding: 0;
  border-radius: 0;
  background: transparent;
}

.message-action-icon:hover {
  color: var(--color-primary);
  background: transparent;
}

.message-action-btn:hover:not(:disabled) {
  color: var(--color-primary);
  background: color-mix(in srgb, var(--color-primary) 8%, var(--bg-main));
}

.message-edit-box {
  display: flex;
  flex-direction: column;
  width: 100%;
  min-height: 50px;
}

.user-bubble--editing {
  color: var(--text-primary);
  border-color: var(--sb-brand);
  box-shadow: var(--focus-ring-shadow);
}

.message-edit-input {
  width: 100%;
  min-height: 50px;
  resize: none;
  border: 0;
  border-radius: 0;
  background: transparent;
  color: inherit;
  padding: 0;
  outline: none;
  line-height: 1.6;
  display: block;
  flex: 1 1 auto;
}

.message-edit-input:focus {
  box-shadow: none;
}

.message-edit-actions {
  display: flex;
  justify-content: flex-end;
  gap: 6px;
  margin-top: 8px;
}

.message-action-btn {
  gap: 4px;
  min-height: 26px;
  border-radius: 6px;
  padding: 0 9px;
  font-size: 12px;
}

.message-action-btn--primary {
  background: var(--color-primary);
  color: #fff;
}

.user-bubble--editing .message-action-btn {
  background: var(--bg-subtle);
  color: var(--text-secondary);
  border: 1px solid var(--border-default);
}

.user-bubble--editing .message-action-btn:hover:not(:disabled) {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.user-bubble--editing .message-action-btn--primary {
  background: var(--color-primary);
  color: #fff;
  border-color: transparent;
}

.user-bubble--editing .message-action-btn--primary:hover:not(:disabled) {
  background: var(--color-primary-hover);
  color: #fff;
}

.message-action-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

:global(.dark) .user-bubble--editing {
  color: var(--text-primary);
  border-color: var(--sb-brand);
  box-shadow: var(--focus-ring-shadow);
}

:global(.dark) .user-bubble::before {
  background: var(--bg-elevated);
}

:global(.dark) .user-bubble--editing .message-action-btn {
  background: var(--bg-subtle);
  color: var(--text-secondary);
  border-color: var(--border-default);
}

:global(.dark) .user-bubble--editing .message-action-btn:hover:not(:disabled) {
  background: var(--bg-hover);
  color: var(--text-primary);
}

:global(.dark) .user-bubble--editing .message-action-btn--primary {
  background: var(--color-primary);
  color: #fff;
  border-color: transparent;
}

:global(.dark) .user-bubble--editing .message-action-btn--primary:hover:not(:disabled) {
  background: var(--color-primary-hover);
  color: #fff;
}
</style>
