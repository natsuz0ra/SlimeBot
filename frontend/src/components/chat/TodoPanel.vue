<script setup lang="ts">
import { computed } from 'vue'
import { mdiChevronRight, mdiFormatListChecks } from '@mdi/js'

import MdiIcon from '@/components/ui/MdiIcon.vue'
import type { RuntimeTodoItem } from '@/api/chatSocket'

const props = defineProps<{
  items: RuntimeTodoItem[]
  open: boolean
  note?: string
}>()

const emit = defineEmits<{
  toggle: []
}>()

const visible = computed(() => props.items.length > 0)

function symbolFor(status: RuntimeTodoItem['status']) {
  if (status === 'completed') return '✔'
  if (status === 'in_progress') return '◼'
  return '◻'
}
</script>

<template>
  <Transition name="todo-panel-shell">
    <aside
      v-if="visible"
      class="todo-panel"
      :class="{ 'todo-panel--open': open }"
      aria-label="Runtime todo list"
    >
      <button
        type="button"
        class="todo-panel-toggle"
        :aria-label="open ? 'Hide todo list' : 'Show todo list'"
        @click="emit('toggle')"
      >
        <MdiIcon v-if="open" :path="mdiChevronRight" :size="18" />
        <MdiIcon v-else :path="mdiFormatListChecks" :size="18" />
      </button>

      <div class="todo-panel-body">
        <div class="todo-panel-head">
          <MdiIcon :path="mdiFormatListChecks" :size="17" />
          <span>Todo</span>
        </div>
        <p v-if="note" class="todo-panel-note">{{ note }}</p>
        <ul class="todo-panel-list">
          <li
            v-for="item in items"
            :key="item.id"
            class="todo-panel-item"
            :class="`todo-panel-item--${item.status}`"
          >
            <span class="todo-panel-mark">{{ symbolFor(item.status) }}</span>
            <span class="todo-panel-text">{{ item.content }}</span>
          </li>
        </ul>
      </div>
    </aside>
  </Transition>
</template>

<style scoped>
.todo-panel {
  position: absolute;
  top: 72px;
  right: 0;
  z-index: 35;
  width: min(320px, calc(100vw - 32px));
  max-height: calc(100% - 184px);
  transform: translateX(calc(100% - 42px));
  transition: transform 180ms ease;
  pointer-events: none;
}

.todo-panel--open {
  transform: translateX(0);
}

.todo-panel-toggle {
  position: absolute;
  left: 0;
  top: 18px;
  width: 42px;
  height: 42px;
  border: 1px solid var(--card-border);
  border-right: 0;
  border-radius: 10px 0 0 10px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--text-primary);
  background: var(--menu-bg);
  box-shadow: var(--floating-elevation-shadow);
  cursor: pointer;
  pointer-events: auto;
}

.todo-panel-body {
  max-height: 100%;
  margin-left: 42px;
  padding: 14px 14px 16px;
  border: 1px solid var(--card-border);
  border-right: 0;
  border-radius: 10px 0 0 10px;
  background: var(--menu-bg);
  box-shadow: var(--floating-elevation-shadow);
  pointer-events: auto;
  overflow-y: auto;
}

.todo-panel-head {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 700;
  color: var(--text-primary);
}

.todo-panel-note {
  margin: 8px 0 0;
  font-size: 12px;
  line-height: 1.45;
  color: var(--text-muted);
}

.todo-panel-list {
  margin: 12px 0 0;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 9px;
}

.todo-panel-item {
  display: grid;
  grid-template-columns: 20px minmax(0, 1fr);
  gap: 8px;
  align-items: start;
  font-size: 13px;
  line-height: 1.45;
  color: var(--text-primary);
}

.todo-panel-mark {
  font-weight: 700;
  color: var(--text-muted);
}

.todo-panel-item--completed .todo-panel-mark {
  color: #16a34a;
}

.todo-panel-item--in_progress .todo-panel-mark {
  color: var(--tool-running-dot);
}

.todo-panel-item--completed .todo-panel-text {
  color: var(--text-muted);
  text-decoration: line-through;
}

.todo-panel-text {
  min-width: 0;
  overflow-wrap: anywhere;
}
</style>
