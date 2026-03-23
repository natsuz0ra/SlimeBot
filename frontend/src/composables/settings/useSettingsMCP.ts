import { computed, ref, type Ref } from 'vue'
import { json } from '@codemirror/lang-json'
import { oneDark } from '@codemirror/theme-one-dark'
import { lineNumbers } from '@codemirror/view'
import { mcpAPI } from '@/api/mcp'

type ToastLike = {
  error(message: string): void
}

type Translate = (key: string) => string

type MCPItem = any
type MCPTransport = 'stdio' | 'sse' | 'streamable_http'

export function useSettingsMCP(options: {
  mcpList: Ref<MCPItem[]>
  mcpDialogVisible: Ref<boolean>
  mcpSubmitting: Ref<boolean>
  toast: ToastLike
  t: Translate
}) {
  const { mcpList, mcpDialogVisible, mcpSubmitting, toast, t } = options
  const mcpEditingID = ref('')
  const mcpForm = ref({ name: '', config: '', isEnabled: true })
  const mcpTemplateType = ref<MCPTransport>('stdio')
  const mcpEditorExtensions = [lineNumbers(), json(), oneDark]
  const mcpRows = computed(() => mcpList.value || [])
  const mcpDialogTitle = computed(() => (mcpEditingID.value ? t('editMcp') : t('addMcp')))

  async function refreshMCP() {
    mcpList.value = await mcpAPI.list()
  }

  function buildTemplate(transport: MCPTransport) {
    if (transport === 'stdio') {
      return JSON.stringify({ command: 'python', args: ['-m', 'your_module'] }, null, 2)
    }
    return JSON.stringify(
      { transport, url: 'https://your-mcp-server-url', headers: {}, timeout: 5, sse_read_timeout: 300 },
      null,
      2,
    )
  }

  function applyTemplate(transport: MCPTransport) {
    mcpTemplateType.value = transport
    mcpForm.value.config = buildTemplate(transport)
  }

  function openMCPDialog() {
    mcpEditingID.value = ''
    mcpForm.value = { name: '', config: buildTemplate('stdio'), isEnabled: true }
    mcpTemplateType.value = 'stdio'
    mcpDialogVisible.value = true
  }

  function openMCPEditDialog(item: MCPItem) {
    mcpEditingID.value = item.id
    mcpForm.value = { name: item.name, config: item.config, isEnabled: item.isEnabled }
    mcpDialogVisible.value = true
  }

  async function saveMCP() {
    if (!mcpForm.value.name || !mcpForm.value.config) {
      toast.error(t('mcpFormIncomplete'))
      return
    }
    let parsed: any
    try {
      parsed = JSON.parse(mcpForm.value.config)
    } catch {
      toast.error(t('mcpJsonInvalid'))
      return
    }
    if (parsed?.mcpServers) {
      toast.error(t('mcpWrapperNotSupported'))
      return
    }

    mcpSubmitting.value = true
    try {
      const payload = {
        name: mcpForm.value.name.trim(),
        config: JSON.stringify(parsed, null, 2),
        isEnabled: mcpForm.value.isEnabled,
      }
      if (mcpEditingID.value) {
        await mcpAPI.update(mcpEditingID.value, payload)
      } else {
        await mcpAPI.create(payload)
      }
      mcpForm.value = { name: '', config: buildTemplate('stdio'), isEnabled: true }
      await refreshMCP()
      mcpDialogVisible.value = false
    } finally {
      mcpSubmitting.value = false
    }
  }

  async function updateMCP(item: MCPItem) {
    await mcpAPI.update(item.id, { name: item.name, config: item.config, isEnabled: item.isEnabled })
  }

  async function deleteMCP(id: string) {
    await mcpAPI.remove(id)
    await refreshMCP()
  }

  function mcpPreview(item: MCPItem) {
    try {
      const cfg = JSON.parse(item.config || '{}')
      const transport = cfg.transport || 'stdio'
      if (transport === 'stdio') return `${transport} 路 ${cfg.command || '-'}`
      return `${transport} 路 ${cfg.url || '-'}`
    } catch {
      return t('mcpJsonInvalid')
    }
  }

  return {
    mcpForm,
    mcpRows,
    mcpDialogTitle,
    mcpTemplateType,
    mcpEditorExtensions,
    applyTemplate,
    openMCPDialog,
    openMCPEditDialog,
    saveMCP,
    updateMCP,
    deleteMCP,
    mcpPreview,
  }
}
