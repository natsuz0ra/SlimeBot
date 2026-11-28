<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Braces, ChevronDown, ChevronUp, Pencil, Plus, RefreshCw, Trash2, Wrench } from 'lucide-vue-next'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import ToggleSwitch from '@/components/ui/ToggleSwitch.vue'
import type { MCPConfig, MCPToolItem, MCPToolListResponse } from '@/types/settings'
import { formatMCPToolParameters, getMCPToolsBadge } from '@/utils/mcpTools'

defineProps<{
  mcpRows: MCPConfig[]
  mcpPreview: (item: MCPConfig) => string
  updateMcp: (item: MCPConfig) => void
  expandedMcpToolsId: string
  mcpToolLoading: Record<string, boolean>
  mcpToolResponses: Record<string, MCPToolListResponse | undefined>
  mcpToolQueries: Record<string, string>
  visibleMcpTools: (id: string) => MCPToolItem[]
  toggleMcpTools: (item: MCPConfig) => void
  loadMcpTools: (item: MCPConfig, force?: boolean) => void
  setMcpToolsQuery: (id: string, query: string) => void
}>()

const emit = defineEmits<{
  add: []
  edit: [item: MCPConfig]
  delete: [id: string]
}>()

const { t } = useI18n()
const schemaExpanded = ref<Record<string, boolean>>({})

function readInput(event: Event) {
  return (event.target as HTMLInputElement).value
}

function schemaKey(configID: string, toolName: string) {
  return `${configID}:${toolName}`
}

function toggleSchema(configID: string, toolName: string) {
  const key = schemaKey(configID, toolName)
  schemaExpanded.value[key] = !schemaExpanded.value[key]
}

function schemaText(tool: MCPToolItem) {
  return JSON.stringify(tool.inputSchema || {}, null, 2)
}
</script>

<template>
  <div>
    <div class="flex justify-end mb-4">
      <button type="button" class="btn-primary action-btn flex items-center gap-1.5 px-3 py-1.5 rounded-xl cursor-pointer settings-action-text" @click="emit('add')">
        <Plus :size="13" :stroke-width="1.75" aria-hidden="true" />
        {{ t('add') }}
      </button>
    </div>
    <div class="flex flex-col gap-2">
      <div v-for="item in mcpRows" :key="item.id" class="settings-card rounded-xl">
        <div class="flex items-center gap-3 px-4 py-3.5">
          <div class="flex-1 min-w-0">
            <div class="settings-item-name">{{ item.name }}</div>
            <div class="settings-item-sub mt-0.5">{{ mcpPreview(item) }}</div>
            <div class="flex flex-wrap items-center gap-1.5 mt-2">
              <span class="mcp-tool-badge" :class="item.isEnabled ? 'mcp-tool-badge-ok' : ''">
                {{ item.isEnabled ? '已启用' : '未启用' }}
              </span>
              <span
                class="mcp-tool-badge"
                :class="`mcp-tool-badge-${getMCPToolsBadge(item.isEnabled, !!mcpToolLoading[item.id], mcpToolResponses[item.id]).tone}`"
              >
                {{ getMCPToolsBadge(item.isEnabled, !!mcpToolLoading[item.id], mcpToolResponses[item.id]).label }}
              </span>
            </div>
          </div>
          <div class="flex items-center gap-2 flex-shrink-0">
            <ToggleSwitch
              :model-value="item.isEnabled"
              @update:model-value="
                (v) => {
                  item.isEnabled = v
                  updateMcp(item)
                }
              "
            />
            <button
              type="button"
              class="w-7 h-7 flex items-center justify-center rounded-lg transition-all duration-150 cursor-pointer action-icon-btn"
              :class="expandedMcpToolsId === item.id ? 'mcp-tool-btn-active' : ''"
              :title="'查看 tools'"
              @click="toggleMcpTools(item)"
            >
              <Wrench :size="14" :stroke-width="1.75" aria-hidden="true" />
            </button>
            <button
              v-if="expandedMcpToolsId === item.id && item.isEnabled"
              type="button"
              class="w-7 h-7 flex items-center justify-center rounded-lg transition-all duration-150 cursor-pointer action-icon-btn"
              :title="'刷新 tools'"
              :disabled="mcpToolLoading[item.id]"
              @click="loadMcpTools(item, true)"
            >
              <LoadingSpinner v-if="mcpToolLoading[item.id]" size-class="w-3 h-3" />
              <RefreshCw v-else :size="14" :stroke-width="1.75" aria-hidden="true" />
            </button>
            <button type="button" class="w-7 h-7 flex items-center justify-center rounded-lg transition-all duration-150 cursor-pointer edit-btn" @click="emit('edit', item)">
              <Pencil :size="14" :stroke-width="1.75" aria-hidden="true" />
            </button>
            <button type="button" class="w-7 h-7 flex items-center justify-center rounded-lg transition-all duration-150 cursor-pointer delete-btn" @click="emit('delete', item.id)">
              <Trash2 :size="15" :stroke-width="1.75" aria-hidden="true" />
            </button>
          </div>
        </div>

        <div v-if="expandedMcpToolsId === item.id" class="mcp-tools-panel mx-4 mb-3 pt-3">
          <div v-if="!item.isEnabled" class="mcp-tools-state">该 MCP 未启用，暂不加载 tools。</div>
          <div v-else-if="mcpToolLoading[item.id]" class="mcp-tools-state flex items-center gap-2">
            <LoadingSpinner size-class="w-3.5 h-3.5" />
            正在加载 MCP tools...
          </div>
          <div v-else-if="mcpToolResponses[item.id]?.status === 'error'" class="mcp-tools-state mcp-tools-state-warn">
            {{ mcpToolResponses[item.id]?.error || 'MCP tools 加载失败' }}
          </div>
          <template v-else-if="mcpToolResponses[item.id]?.status === 'loaded'">
            <div class="flex items-center justify-between gap-3 mb-2">
              <div class="mcp-tools-title">支持的 tools</div>
              <input
                class="mcp-tools-search"
                :value="mcpToolQueries[item.id] || ''"
                placeholder="搜索 tools"
                @input="setMcpToolsQuery(item.id, readInput($event))"
              >
            </div>
            <div v-if="visibleMcpTools(item.id).length === 0" class="mcp-tools-state">没有匹配的 tools。</div>
            <div v-else class="mcp-tools-list">
              <div v-for="tool in visibleMcpTools(item.id)" :key="tool.name" class="mcp-tool-row">
                <div class="min-w-0">
                  <div class="mcp-tool-name">{{ tool.name }}</div>
                  <div class="mcp-tool-desc">{{ tool.description || '-' }}</div>
                  <div class="mcp-tool-meta">{{ formatMCPToolParameters(tool) }}</div>
                </div>
                <button type="button" class="mcp-schema-btn" @click="toggleSchema(item.id, tool.name)">
                  <Braces :size="13" :stroke-width="1.75" aria-hidden="true" />
                  <component :is="schemaExpanded[schemaKey(item.id, tool.name)] ? ChevronUp : ChevronDown" :size="13" :stroke-width="1.75" aria-hidden="true" />
                </button>
                <pre v-if="schemaExpanded[schemaKey(item.id, tool.name)]" class="mcp-schema-block">{{ schemaText(tool) }}</pre>
              </div>
            </div>
          </template>
          <div v-else class="mcp-tools-state">点击刷新加载该 MCP 支持的 tools。</div>
        </div>
      </div>
      <div v-if="mcpRows.length === 0" class="empty-state text-center py-10 rounded-xl">{{ t('add') }} MCP</div>
    </div>
  </div>
</template>

<style scoped>
.mcp-tool-badge {
  border: 1px solid var(--card-border);
  border-radius: 999px;
  color: var(--text-muted);
  font-size: 11px;
  line-height: 1;
  padding: 3px 7px;
}

.mcp-tool-badge-ok {
  color: var(--color-accent);
}

.mcp-tool-badge-warn {
  color: var(--color-warning);
}

.mcp-tool-badge-muted {
  color: var(--text-muted);
}

.mcp-tool-btn-active {
  background: var(--primary-alpha-10);
  color: var(--sb-brand);
}

.mcp-tools-panel {
  border-top: 1px solid var(--card-border);
}

.mcp-tools-title {
  color: var(--text-secondary);
  font-size: 13px;
  font-weight: 600;
  line-height: 18px;
}

.mcp-tools-search {
  background: var(--input-bg);
  border: 1px solid var(--input-border);
  border-radius: 10px;
  color: var(--text-primary);
  font-size: 12px;
  height: 30px;
  outline: none;
  padding: 0 10px;
  width: 180px;
}

.mcp-tools-search:focus {
  border-color: var(--sb-brand);
}

.mcp-tools-state {
  color: var(--text-muted);
  font-size: 13px;
  line-height: 18px;
}

.mcp-tools-state-warn {
  color: var(--color-warning);
}

.mcp-tools-list {
  background: var(--primary-alpha-04);
  border: 1px solid var(--card-border);
  border-radius: 12px;
  overflow: hidden;
}

.mcp-tool-row {
  display: grid;
  gap: 8px;
  grid-template-columns: minmax(0, 1fr) auto;
  padding: 10px 12px;
}

.mcp-tool-row + .mcp-tool-row {
  border-top: 1px solid var(--card-border);
}

.mcp-tool-name {
  color: var(--text-primary);
  font-family: var(--font-mono);
  font-size: 12px;
  font-weight: 600;
  line-height: 18px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mcp-tool-desc,
.mcp-tool-meta {
  color: var(--text-muted);
  font-size: 12px;
  line-height: 18px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mcp-schema-btn {
  align-items: center;
  color: var(--text-muted);
  cursor: pointer;
  display: inline-flex;
  gap: 2px;
  height: 26px;
}

.mcp-schema-btn:hover {
  color: var(--sb-brand);
}

.mcp-schema-block {
  background: var(--input-bg);
  border: 1px solid var(--input-border);
  border-radius: 10px;
  color: var(--text-secondary);
  font-family: var(--font-mono);
  font-size: 12px;
  grid-column: 1 / -1;
  line-height: 18px;
  margin: 0;
  max-height: 220px;
  overflow: auto;
  padding: 10px;
}
</style>
