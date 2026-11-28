<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { SlashCommand } from '@/utils/slashCommands'

const props = defineProps<{ id: string; commands: SlashCommand[]; active: number }>()
const emit = defineEmits<{ select: [command: SlashCommand]; highlight: [index: number] }>()
const { t } = useI18n()
const panel = ref<HTMLElement>()
const groups = computed(() => (['workbench', 'skill'] as const).map(kind => ({
  kind, commands: props.commands.map((command, index) => ({ command, index })).filter(item => item.command.kind === kind),
})).filter(group => group.commands.length))

watch(() => props.active, async () => {
  await nextTick()
  const box = panel.value
  const option = box?.querySelector<HTMLElement>('[aria-selected="true"]')
  if (!box || !option) return
  const item = option.getBoundingClientRect()
  const viewport = box.getBoundingClientRect()
  if (item.bottom > viewport.bottom) box.scrollTop += item.bottom - viewport.bottom
  if (item.top < viewport.top) box.scrollTop -= viewport.top - item.top
})
</script>

<template>
  <div class="slash-menu">
    <div :id="id" ref="panel" class="slash-options sb-scrollbar" role="listbox" :aria-label="t('slashMenu')">
      <div v-for="group in groups" :key="group.kind" role="group" :aria-label="t('slashGroup.' + group.kind)">
        <div class="slash-group" aria-hidden="true">{{ t('slashGroup.' + group.kind) }}</div>
        <button
          v-for="{ command, index } in group.commands" :id="`${id}-${index}`" :key="command.id"
          type="button" role="option" tabindex="-1" :aria-selected="index === active"
          @pointermove="emit('highlight', index)" @mousedown.prevent @click="emit('select', command)"
        >
          <strong>/{{ command.slash }}</strong><span>{{ command.description }}</span>
        </button>
      </div>
    </div>
    <div class="slash-hint">{{ t('slashKeys') }}</div>
  </div>
</template>

<style scoped>
.slash-menu { position: absolute; z-index: 45; bottom: calc(100% + 6px); left: 0; right: 0; max-width: 460px; border: 1px solid var(--menu-border); border-radius: 10px; background: var(--menu-bg); box-shadow: var(--menu-shadow); overflow: hidden; }
.slash-options { max-height: min(280px, 38dvh); overflow-y: auto; padding: 6px; }
.slash-group { padding: 6px 8px 4px; font-size: 11px; font-weight: 600; color: var(--text-muted); }
.slash-options button { display: flex; align-items: center; gap: 10px; width: 100%; height: 36px; padding: 0 8px; border-radius: 6px; color: var(--text-primary); text-align: left; cursor: pointer; }
.slash-options button[aria-selected="true"] { background: var(--bg-active); }
.slash-options strong { flex: 0 1 auto; min-width: 0; font: 600 13px var(--font-mono); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.slash-options span { flex: 1; min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; color: var(--text-secondary); font-size: 12px; }
.slash-hint { min-height: 26px; padding: 4px 12px; border-top: 1px solid var(--border-subtle); color: var(--text-muted); font-size: 11px; }
</style>
