<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { mdiClose } from '@mdi/js'
import MdiIcon from '@/components/MdiIcon.vue'

const props = withDefaults(defineProps<{
  visible: boolean
  title?: string
  confirmText?: string
  cancelText?: string
  confirmLoading?: boolean
  confirmDanger?: boolean
  width?: string
  hideFooter?: boolean
}>(), {
  confirmText: '确认',
  cancelText: '取消',
  confirmLoading: false,
  confirmDanger: false,
  width: '480px',
  hideFooter: false,
})

const emit = defineEmits<{
  'update:visible': [value: boolean]
  confirm: []
  cancel: []
}>()

function close() {
  emit('update:visible', false)
  emit('cancel')
}

function onConfirm() {
  emit('confirm')
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.visible) close()
}

onMounted(() => document.addEventListener('keydown', onKeydown))
onUnmounted(() => document.removeEventListener('keydown', onKeydown))
</script>

<template>
  <Teleport to="body">
    <Transition name="dialog-fade">
      <div
        v-if="visible"
        class="fixed inset-0 z-[200] flex items-center justify-center p-4"
        style="background: rgba(0,0,0,0.3)"
        @click.self="close"
      >
        <div
          class="relative bg-white rounded-xl shadow-xl flex flex-col overflow-hidden"
          :style="{ width: '100%', maxWidth: width, maxHeight: '90vh' }"
          @click.stop
        >
          <!-- 头部 -->
          <div class="flex items-center justify-between px-5 py-3.5 border-b border-gray-100">
            <span class="text-sm font-semibold text-gray-900">{{ title }}</span>
            <button
              type="button"
              class="w-7 h-7 flex items-center justify-center rounded-md text-gray-400 hover:text-gray-600 hover:bg-gray-100 transition-colors duration-150 cursor-pointer"
              @click="close"
            >
              <MdiIcon :path="mdiClose" :size="16" />
            </button>
          </div>

          <!-- 内容 -->
          <div class="flex-1 overflow-y-auto px-5 py-4">
            <slot />
          </div>

          <!-- 底部 -->
          <div v-if="!hideFooter" class="flex items-center justify-end gap-2 px-5 py-3 border-t border-gray-100">
            <button
              type="button"
              class="px-4 py-1.5 text-sm rounded-lg border border-gray-200 text-gray-600 bg-white hover:bg-gray-50 transition-colors duration-150 cursor-pointer"
              @click="close"
            >
              {{ cancelText }}
            </button>
            <button
              type="button"
              class="px-4 py-1.5 text-sm rounded-lg text-white transition-colors duration-150 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1.5"
              :class="confirmDanger ? 'bg-red-600 hover:bg-red-700' : 'bg-blue-600 hover:bg-blue-700'"
              :disabled="confirmLoading"
              @click="onConfirm"
            >
              <svg v-if="confirmLoading" class="animate-spin w-3.5 h-3.5" viewBox="0 0 24 24" fill="none">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
              </svg>
              {{ confirmText }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.dialog-fade-enter-active,
.dialog-fade-leave-active {
  transition: opacity 150ms ease-out;
}
.dialog-fade-enter-active > div,
.dialog-fade-leave-active > div {
  transition: transform 150ms ease-out, opacity 150ms ease-out;
}
.dialog-fade-enter-from,
.dialog-fade-leave-to {
  opacity: 0;
}
.dialog-fade-enter-from > div,
.dialog-fade-leave-to > div {
  transform: scale(0.97) translateY(-4px);
  opacity: 0;
}
</style>
