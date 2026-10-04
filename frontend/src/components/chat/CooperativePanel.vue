<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { mdiAccountGroupOutline, mdiChevronRight, mdiClose, mdiStopCircleOutline, mdiSourceBranch, mdiSendOutline, mdiAlertCircleOutline, mdiClipboardCheckOutline, mdiFileDocumentOutline, mdiPlus, mdiCogOutline, mdiArrowRight, mdiRobotOutline, mdiClockOutline } from '@mdi/js'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import { cooperativeAPI, type CooperativeSnapshot } from '@/api/cooperative'
import { cooperativeStatus, cooperativeReport, cooperativeError, latestAgentTurns, orderedAgentTree } from '@/utils/cooperative'
import { settingAPI } from '@/api/settings'
import { renderMarkdown } from '@/utils/markdown'

const props = defineProps<{ sessionId?: string }>()
const emit = defineEmits<{ settled: [sessionId: string] }>()
const { locale } = useI18n()
const english = computed(() => locale.value.startsWith('en'))
const text = (zh: string, en: string) => english.value ? en : zh
const empty = (): CooperativeSnapshot => ({ agents: [], turns: [], tasks: [], artifacts: [], approvals: [], events: [] })
const snapshot = ref(empty())
const visible = ref(false)
const tab = ref<'agents' | 'tasks' | 'artifacts'>('agents')
const fullTurns = ref<import('@/api/cooperative').CooperativeTurn[]>([])
const selected = ref('')
const message = ref('')
const delivery = ref('steer')
const busy = ref(false)
const error = ref('')
const diff = ref('')
const inspectedArtifact = ref('')
const creatingTask = ref(false)
const panel = ref<HTMLElement>()
const taskTitle = ref('')
const taskDescription = ref('')
const maxDepth = ref(2)
const validationCommands = ref<Record<string, string>>({})
const rootRunning = computed(() => snapshot.value.roots?.some(r => ['running', 'waiting'].includes(r.status)))
const budget = computed(() => snapshot.value.roots?.[0])
const groups = computed(() => [
  { id: 'pending', label: text('待处理', 'Pending'), states: ['pending', 'needs_attention'] },
  { id: 'active', label: text('处理中', 'In progress'), states: ['in_progress', 'awaiting_integration'] },
  { id: 'done', label: text('已结束', 'Finished'), states: ['completed', 'canceled'] },
].map(g => ({ ...g, tasks: snapshot.value.tasks.filter(t => g.states.includes(t.status)) })))
const profileLabel = (profile: string) => ({ researcher: text('研究', 'Research'), reviewer: text('审查', 'Review'), worker: text('执行', 'Worker') })[profile] || profile
function dependencies(raw: string): string[] { try { return JSON.parse(raw) } catch { return [] } }
let cursor = 0
const latest = computed(() => latestAgentTurns([...fullTurns.value, ...snapshot.value.turns]))
const tree = computed(() => orderedAgentTree(snapshot.value.agents, props.sessionId || ''))
const active = computed(() => [...latest.value.values()].filter(t => ['running', 'waiting', 'stopping'].includes(t.status)).length)
const agent = computed(() => snapshot.value.agents.find(a => a.id === selected.value))
const turn = computed(() => latest.value.get(selected.value))
const history = computed(() => [...new Map([...snapshot.value.turns, ...fullTurns.value].map(t => [t.id, t])).values()].filter(t => t.agentId === selected.value).sort((a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt)))
const completedTasks = computed(() => snapshot.value.tasks.filter(t => t.status === 'completed').length)
const tabs = computed(() => [
  { id: 'agents' as const, label: text('代理', 'Agents'), icon: mdiAccountGroupOutline, count: snapshot.value.agents.length },
  { id: 'tasks' as const, label: text('任务', 'Tasks'), icon: mdiClipboardCheckOutline, count: snapshot.value.tasks.length },
  { id: 'artifacts' as const, label: text('成果', 'Artifacts'), icon: mdiFileDocumentOutline, count: snapshot.value.artifacts.length },
])
const ownerLabel = (id?: string) => id === props.sessionId ? text('主代理', 'Lead agent') : snapshot.value.agents.find(a => a.id === id)?.title || text('未分派', 'Unassigned')
const workspaceName = (path: string) => path.split(/[\\/]/).filter(Boolean).pop() || path
const agentState = (id: string) => latest.value.get(id)?.status || 'pending'
const approvals = computed(() => snapshot.value.approvals.filter(a => a.status === 'pending'))
let timer: ReturnType<typeof setTimeout> | undefined
let request: AbortController | undefined
let generation = 0
let trigger: HTMLElement | undefined

async function refresh(epoch = generation, wait = false) {
  if (!props.sessionId) return
  request?.abort(); const controller = new AbortController(); request = controller
  try {
    const data = await cooperativeAPI.snapshot(props.sessionId, cursor, controller.signal, wait ? 20000 : 0)
    if (epoch !== generation) return
    error.value = ''
    const eventMap = new Map([...snapshot.value.events, ...(data.events || [])].map(e => [e.seq, e]))
    cursor = Math.max(cursor, ...(data.events || []).map(e => e.seq))
    const wasRunning = rootRunning.value
    const previousRequest = budget.value?.id
    snapshot.value = { ...empty(), ...data, agents: data.agents || [], turns: data.turns || [], tasks: data.tasks || [], artifacts: data.artifacts || [], approvals: data.approvals || [], events: [...eventMap.values()].sort((a, b) => a.seq - b.seq).slice(-128) }
    if (!rootRunning.value && budget.value && (wasRunning || previousRequest !== budget.value.id)) emit('settled', props.sessionId)
    if (!snapshot.value.agents.some(a => a.id === selected.value)) selected.value = snapshot.value.agents[0]?.id || ''
  } catch (e) { if (epoch === generation && !controller.signal.aborted && visible.value) error.value = cooperativeError(e, english.value) }
}
async function poll(epoch: number) {
  await refresh(epoch, visible.value || active.value > 0 || Boolean(rootRunning.value))
  if (epoch === generation) { clearTimeout(timer); timer = setTimeout(() => void poll(epoch), active.value || visible.value || rootRunning.value ? 250 : 5000) }
}
watch(() => props.sessionId, () => {
  document.removeEventListener('keydown', onKey)
  busy.value = false; message.value = ''; taskTitle.value = ''; taskDescription.value = ''; creatingTask.value = false; validationCommands.value = {}
  generation++; cursor = 0; clearTimeout(timer); request?.abort(); snapshot.value = empty(); fullTurns.value = []; selected.value = ''; error.value = ''; diff.value = ''; visible.value = false
  if (props.sessionId) void poll(generation)
}, { immediate: true })
onUnmounted(() => { generation++; clearTimeout(timer); request?.abort(); document.removeEventListener('keydown', onKey) })
watch(selected, () => { message.value = '' })
async function open(event: MouseEvent) { trigger = event.currentTarget as HTMLElement; visible.value = true; document.addEventListener('keydown', onKey); await nextTick(); panel.value?.querySelector<HTMLButtonElement>('button')?.focus(); void refresh(); try { maxDepth.value = (await settingAPI.get()).subagentMaxDepth ?? 2 } catch { /* current delegation still works with saved defaults */ } }
function close() { visible.value = false; document.removeEventListener('keydown', onKey); trigger?.focus() }
function onKey(event: KeyboardEvent) {
  if (event.key === 'Escape') { event.preventDefault(); close() }
  if (event.key !== 'Tab' || !panel.value) return
  const elements = [...panel.value.querySelectorAll<HTMLElement>('button:not(:disabled), textarea:not(:disabled), select:not(:disabled), input:not(:disabled), summary, [tabindex="0"]')].filter(e => e.offsetParent !== null && [...panel.value!.querySelectorAll('details:not([open])')].every(details => !details.contains(e) || details.querySelector(':scope > summary')?.contains(e)))
  const first = elements[0], last = elements[elements.length - 1]
  if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
  else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
}
async function act(fn: () => Promise<unknown>) {
  const epoch = generation
  busy.value = true; error.value = ''
  try {
    await fn()
    if (epoch === generation) await refresh(epoch)
  } catch (e) {
    if (epoch === generation) error.value = cooperativeError(e, english.value)
  } finally {
    if (epoch === generation) busy.value = false
  }
}
async function createTask() {
  if (!props.sessionId || !taskTitle.value.trim()) return
  const epoch = generation, session = props.sessionId
  await act(async () => { await cooperativeAPI.task(session, { action: 'create', title: taskTitle.value.trim(), description: taskDescription.value }); if (epoch === generation) { taskTitle.value = ''; taskDescription.value = ''; creatingTask.value = false } })
}
async function reassign(taskId: string, revision: number, event: Event) {
  const ownerId = (event.target as HTMLSelectElement).value
  if (ownerId) await act(() => cooperativeAPI.task(props.sessionId!, { action: 'reassign', taskId, expectedRevision: revision, ownerId }))
}
async function send() {
  if (!selected.value || !message.value.trim()) return
  const body = message.value, id = selected.value, epoch = generation
  await act(async () => { await cooperativeAPI.action(id, { action: 'message', message: body, delivery: delivery.value, clientMessageId: crypto.randomUUID() }); if (epoch === generation && selected.value === id && message.value === body) message.value = '' })
}
async function loadAgentHistory() {
  const id = selected.value, epoch = generation
  await act(async () => {
    const turns = await cooperativeAPI.turns(id)
    if (epoch === generation) fullTurns.value = [...fullTurns.value.filter(t => t.agentId !== id), ...turns]
  })
}
async function inspectArtifact(id: string) {
  const epoch = generation, session = props.sessionId!
  await act(async () => { const result = await cooperativeAPI.artifact(id, session); if (epoch === generation) { diff.value = result.diff || text('没有文件变更', 'No file changes'); inspectedArtifact.value = id } })
}
function activity(id: string) {
  const event = [...snapshot.value.events].reverse().find(e => e.agentId === id && e.kind === 'agent_activity')
  if (latest.value.get(id)?.status !== 'running') return ''
  if (!event) return latest.value.get(id)?.activity || ''
  try { const p = JSON.parse(event.payload); return [p.tool, p.command].filter(Boolean).join(' · ') } catch { return '' }
}
function approvalParams(payload: string) { try { const p = JSON.parse(payload); return JSON.stringify(p.params || {}, null, 2) } catch { return payload } }
function approvalLabel(payload: string) { try { const p = JSON.parse(payload); return `${p.toolName || ''} · ${p.command || ''}` } catch { return text('工具执行', 'Tool execution') } }
</script>

<template>
  <div v-if="sessionId" class="cooperative-entry">
    <button type="button" class="coop-trigger" :aria-expanded="visible" @click="open">
      <MdiIcon :path="mdiAccountGroupOutline" :size="17" />
      <span>{{ text('协作', 'Collaboration') }}</span>
      <span v-if="snapshot.agents.length" class="coop-count">{{ snapshot.agents.length }}</span>
      <span v-if="active" class="coop-running"><i class="coop-dot" />{{ active }} {{ text('运行中', 'running') }}</span>
      <span v-if="approvals.length" class="coop-attention">{{ text('待批准', 'Approval needed') }}</span>
      <MdiIcon :path="mdiChevronRight" :size="14" />
    </button>
  </div>
  <Teleport to="body">
    <div v-if="visible" class="coop-overlay" @click.self="close">
      <section ref="panel" class="coop-panel" role="dialog" aria-modal="true" :aria-label="text('协作工作区', 'Collaboration workspace')">
        <header class="coop-header">
          <div class="coop-heading">
            <span class="coop-brand"><MdiIcon :path="mdiAccountGroupOutline" :size="25" /></span>
            <div><h2>{{ text('协作工作区', 'Collaboration workspace') }}</h2><p>{{ text('任务有序推进，成果随时可见', 'Follow the work. Review the results.') }}</p></div>
          </div>
          <div class="coop-header-actions">
            <button v-if="rootRunning" type="button" class="coop-stop" :disabled="busy" @click="act(() => cooperativeAPI.stop(sessionId!))"><MdiIcon :path="mdiStopCircleOutline" :size="16" />{{ text('停止全部', 'Stop all') }}</button>
            <button type="button" class="coop-icon-button" :aria-label="text('关闭', 'Close')" @click="close"><MdiIcon :path="mdiClose" :size="21" /></button>
          </div>
        </header>
        <div class="coop-overview">
          <div><span class="coop-overview-label"><i class="coop-dot" :class="{ live: active > 0 }" />{{ text('正在协作', 'Active agents') }}</span><strong>{{ active }}<small>/ {{ snapshot.agents.length }}</small></strong></div>
          <div><span class="coop-overview-label">{{ text('任务完成', 'Tasks completed') }}</span><strong>{{ completedTasks }}<small>/ {{ snapshot.tasks.length }}</small></strong></div>
          <div><span class="coop-overview-label">{{ text('交付成果', 'Artifacts') }}</span><strong>{{ snapshot.artifacts.length }}<small>{{ text('份', 'total') }}</small></strong></div>
        </div>
        <nav class="coop-tabs" :aria-label="text('协作视图', 'Collaboration views')">
          <button v-for="item in tabs" :key="item.id" type="button" :class="{ selected: tab === item.id }" :aria-current="tab === item.id ? 'page' : undefined" @click="tab = item.id; diff = ''">
            <MdiIcon :path="item.icon" :size="17" /><span>{{ item.label }}</span><span class="coop-tab-count">{{ item.count }}</span>
          </button>
        </nav>
        <p v-if="error" class="coop-error" role="alert">{{ error }}</p>
        <div v-if="approvals.length" class="coop-approvals">
          <div v-for="approval in approvals" :key="approval.id" class="coop-approval">
            <MdiIcon :path="mdiAlertCircleOutline" :size="20" />
            <details class="coop-approval-detail"><summary><span class="coop-eyebrow">{{ text('需要你的批准', 'Approval needed') }}</span>{{ approvalLabel(approval.payload) }}</summary><pre>{{ approvalParams(approval.payload) }}</pre></details>
            <div class="coop-approval-actions"><button :disabled="busy" @click="act(() => cooperativeAPI.approve(approval.id, false))">{{ text('拒绝', 'Reject') }}</button><button class="coop-primary" :disabled="busy" @click="act(() => cooperativeAPI.approve(approval.id, true))">{{ text('批准', 'Approve') }}</button></div>
          </div>
        </div>
        <template v-if="tab === 'agents'">
          <div v-if="!tree.length" class="coop-empty"><span class="coop-empty-icon"><MdiIcon :path="mdiAccountGroupOutline" :size="30" /></span><h3>{{ text('让任务一起向前', 'Work together') }}</h3><p>{{ text('代理开始协作后，进度、报告和追问都会出现在这里。', 'Agent progress, reports and follow-ups appear here when work is delegated.') }}</p></div>
          <div v-else class="coop-body">
            <nav class="coop-agent-list" :aria-label="text('代理列表', 'Agents')">
              <p class="coop-list-caption">{{ text('协作成员', 'Team members') }}</p>
              <button v-for="item in tree" :key="item.id" :class="{ selected: selected === item.id }" :aria-current="selected === item.id ? 'true' : undefined" @click="selected = item.id; diff = ''; message = ''">
                <span class="coop-agent-row" :style="{ paddingLeft: `${Math.max(0, item.depth - 1) * 10}px` }">
                  <span class="coop-avatar"><MdiIcon :path="item.contextMode === 'fork' ? mdiSourceBranch : mdiRobotOutline" :size="20" /></span>
                  <span class="coop-agent-name"><strong>{{ item.title }}</strong><span>{{ profileLabel(item.profile) }}<span v-if="item.queued"> · {{ item.queued }} {{ text('排队', 'queued') }}</span></span></span>
                </span>
                <span class="coop-agent-meta"><span class="coop-status" :data-status="agentState(item.id)"><i class="coop-dot" />{{ cooperativeStatus(agentState(item.id), english) }}</span></span>
                <span v-if="activity(item.id)" class="coop-activity">{{ activity(item.id) }}</span>
              </button>
            </nav>
            <div v-if="agent" class="coop-agent-detail">
              <div class="coop-detail-title"><div><span class="coop-eyebrow">{{ text('代理报告', 'Agent report') }}</span><h3>{{ agent.title }}</h3></div><button v-if="turn && ['running', 'waiting'].includes(turn.status)" class="coop-icon-button" :disabled="busy" :aria-label="text('中断当前轮', 'Interrupt turn')" @click="act(() => cooperativeAPI.action(agent!.id, { action: 'interrupt', scope: 'turn' }))"><MdiIcon :path="mdiStopCircleOutline" :size="20" /></button></div>
              <div class="coop-badges"><span>{{ profileLabel(agent.profile) }}</span><span>{{ agent.contextMode === 'fork' ? text('继承上下文', 'Forked context') : text('独立上下文', 'Isolated context') }}</span><span class="coop-status" :data-status="turn?.status"><i class="coop-dot" />{{ cooperativeStatus(turn?.status || 'pending', english) }}</span></div>
              <details class="coop-task-details coop-scope"><summary>{{ text('任务与工作范围', 'Task and scope') }}</summary><p>{{ agent.task }}</p><code>{{ agent.workspace }}</code></details>
              <div v-if="turn?.error" class="coop-error">{{ turn.error }}</div>
              <article v-if="turn?.answer" class="coop-report markdown-body" v-html="renderMarkdown(cooperativeReport(turn.answer))" />
              <div v-else class="coop-report-pending"><MdiIcon :path="mdiClockOutline" :size="24" /><p>{{ activity(agent.id) || text('等待代理提交报告', 'Waiting for the agent report') }}</p></div>
              <details v-if="history.length > 1" class="coop-task-details"><summary>{{ text('历史轮次', 'Earlier turns') }} · {{ history.length - 1 }}</summary><div v-for="past in history.slice(1)" :key="past.id" class="coop-history"><span class="coop-status" :data-status="past.status">{{ cooperativeStatus(past.status, english) }}</span><p>{{ past.answer || past.error }}</p></div></details>
              <button class="coop-history-button" :disabled="busy" @click="loadAgentHistory"><MdiIcon :path="mdiClockOutline" :size="15" />{{ text('加载完整报告与历史', 'Load full reports and history') }}</button>
              <form v-if="agent.mode === 'continuable'" class="coop-followup" @submit.prevent="send">
                <label for="cooperative-message">{{ text('继续协作', 'Continue working') }}</label>
                <textarea id="cooperative-message" v-model="message" rows="3" :placeholder="text('补充要求，或向此代理继续追问…', 'Add details or ask this agent a follow-up…')" :disabled="busy" />
                <div class="coop-composer-actions"><select v-model="delivery" :aria-label="text('投递方式', 'Delivery')"><option value="steer">{{ text('补充当前任务', 'Steer current work') }}</option><option value="queue">{{ text('加入下一轮', 'Queue next turn') }}</option></select><button class="coop-primary" type="submit" :disabled="busy || !message.trim()"><MdiIcon :path="mdiSendOutline" :size="16" />{{ text('发送', 'Send') }}</button></div>
              </form>
            </div>
          </div>
        </template>
        <div v-else-if="tab === 'tasks'" class="coop-collection">
          <div class="coop-section-heading"><div><h3>{{ text('任务看板', 'Task board') }}</h3><p>{{ text('从分派到完成，跟进每一项工作', 'Track each task from assignment to completion') }}</p></div><button :aria-expanded="creatingTask" @click="creatingTask = !creatingTask"><MdiIcon :path="creatingTask ? mdiClose : mdiPlus" :size="16" />{{ creatingTask ? text('收起', 'Close') : text('新建任务', 'New task') }}</button></div>
          <form v-if="creatingTask" class="coop-new-task" @submit.prevent="createTask"><label for="cooperative-task-title">{{ text('任务名称', 'Task name') }}</label><input id="cooperative-task-title" v-model="taskTitle" :placeholder="text('这项任务需要完成什么？', 'What needs to be done?')" maxlength="120" /><label for="cooperative-task-description">{{ text('目标与验收要求', 'Goal and acceptance criteria') }}</label><textarea id="cooperative-task-description" v-model="taskDescription" :placeholder="text('补充任务说明…', 'Add task details…')" rows="2" /><div class="coop-task-actions"><button class="coop-primary" :disabled="busy || !taskTitle.trim()"><MdiIcon :path="mdiPlus" :size="16" />{{ text('添加任务', 'Add task') }}</button></div></form>
          <div class="coop-board">
            <section v-for="group in groups" :key="group.id" class="coop-lane" :data-lane="group.id">
              <h3><i class="coop-dot" />{{ group.label }}<span>{{ group.tasks.length }}</span></h3>
              <div v-if="!group.tasks.length" class="coop-lane-empty"><MdiIcon :path="group.id === 'done' ? mdiClipboardCheckOutline : mdiClockOutline" :size="23" /><p>{{ text('暂无任务', 'No tasks') }}</p></div>
              <article v-for="task in group.tasks" :key="task.id" class="coop-task-card">
                <span class="coop-status" :data-status="task.status">{{ cooperativeStatus(task.status, english) }}</span><h4>{{ task.title }}</h4>
                <p v-if="task.description" class="coop-card-description">{{ task.description }}</p>
                <p v-if="task.acceptance" class="coop-secondary">{{ text('验收', 'Acceptance') }} · {{ task.acceptance }}</p>
                <p v-for="dep in dependencies(task.dependencies)" :key="dep" class="coop-dependency"><MdiIcon :path="mdiSourceBranch" :size="14" />{{ snapshot.tasks.find(t => t.id === dep)?.title || dep }}</p>
                <div v-if="['completed', 'canceled'].includes(task.status)" class="coop-owner-static"><MdiIcon :path="mdiRobotOutline" :size="15" />{{ ownerLabel(task.ownerId) }}</div>
                <label v-else class="coop-owner"><span>{{ text('负责人', 'Owner') }}</span><select :value="task.ownerId || ''" :disabled="busy" @change="reassign(task.id, task.revision, $event)"><option value="">{{ text('未分派', 'Unassigned') }}</option><option :value="sessionId">{{ text('主代理', 'Lead agent') }}</option><option v-for="member in snapshot.agents.filter(a => a.mode === 'continuable')" :key="member.id" :value="member.id">{{ member.title }}</option></select></label>
                <details v-if="task.result" class="coop-task-details"><summary>{{ text('查看结果', 'View result') }}</summary><p>{{ task.result }}</p></details>
                <div class="coop-task-actions"><button v-if="['needs_attention', 'completed', 'canceled'].includes(task.status)" :disabled="busy" @click="act(() => cooperativeAPI.task(sessionId!, { action: 'reopen', taskId: task.id, expectedRevision: task.revision }))">{{ text('重新打开', 'Reopen') }}</button><button v-if="!['completed', 'canceled'].includes(task.status)" class="coop-text-button" :disabled="busy" @click="act(() => cooperativeAPI.task(sessionId!, { action: 'cancel', taskId: task.id, expectedRevision: task.revision }))">{{ text('取消任务', 'Cancel task') }}</button></div>
              </article>
            </section>
          </div>
        </div>
        <div v-else class="coop-collection">
          <div class="coop-section-heading"><div><h3>{{ text('协作成果', 'Artifacts') }}</h3><p>{{ text('查看变更、验证结果，再集成到当前工作区', 'Review changes, verify results, then integrate') }}</p></div></div>
          <div v-if="!snapshot.artifacts.length" class="coop-empty"><span class="coop-empty-icon"><MdiIcon :path="mdiFileDocumentOutline" :size="30" /></span><h3>{{ text('等待第一份成果', 'Awaiting the first artifact') }}</h3><p>{{ text('代理完成的文件修改会在这里汇总。', 'File changes from agents will appear here.') }}</p></div>
          <article v-for="artifact in snapshot.artifacts" :key="artifact.id" class="coop-artifact-card">
            <div class="coop-detail-title"><div class="coop-artifact-heading"><span class="coop-avatar"><MdiIcon :path="mdiSourceBranch" :size="21" /></span><div><h3>{{ snapshot.agents.find(a => a.id === artifact.agentId)?.title || text('代理成果', 'Agent artifact') }}</h3><p>{{ text('文件修改成果', 'File changes') }}</p></div></div><span class="coop-status" :data-status="artifact.status"><i class="coop-dot" />{{ cooperativeStatus(artifact.status, english) }}</span></div>
            <p v-if="artifact.report" class="coop-artifact-report">{{ artifact.status === 'integrated' ? text('文件变更已集成到当前工作区。', 'Changes have been integrated into the current workspace.') : artifact.report }}</p>
            <details class="coop-task-details"><summary>{{ text('工作目录', 'Workspace') }} · {{ workspaceName(artifact.parentWorkspace) }}</summary><code>{{ artifact.workspace }}</code></details>
            <details v-if="artifact.validation" class="coop-task-details"><summary><MdiIcon :path="mdiClipboardCheckOutline" :size="16" />{{ text('验证记录', 'Validation record') }}<span v-if="artifact.validatedCommit" class="coop-validation-ok">{{ text('已通过', 'Passed') }}</span></summary><pre class="coop-diff">{{ artifact.validation }}</pre></details>
            <details v-if="['ready', 'conflict', 'validation_failed'].includes(artifact.status)" class="coop-task-details"><summary>{{ text('运行验证', 'Run verification') }}</summary><form class="coop-create" @submit.prevent="act(() => cooperativeAPI.validate(artifact.id, sessionId!, validationCommands[artifact.id] || artifact.validationCommand || ''))"><input v-model="validationCommands[artifact.id]" :aria-label="text('验证命令', 'Validation command')" :placeholder="text('例如 go test ./...', 'e.g. go test ./...')" /><button :disabled="busy || !(validationCommands[artifact.id] || artifact.validationCommand)?.trim()">{{ text('验证', 'Verify') }}</button></form></details>
            <div class="coop-artifact-actions"><button :disabled="busy || artifact.status === 'working'" @click="inspectArtifact(artifact.id)"><MdiIcon :path="mdiFileDocumentOutline" :size="16" />{{ text('查看变更', 'View changes') }}</button><button v-if="['ready', 'conflict', 'validation_failed'].includes(artifact.status)" class="coop-primary" :disabled="busy || !artifact.validatedCommit || artifact.validatedCommit !== artifact.resultCommit" :title="artifact.validatedCommit === artifact.resultCommit ? undefined : text('验证通过后即可集成', 'Verify the result before integrating')" @click="act(() => cooperativeAPI.artifact(artifact.id, sessionId!, true))">{{ text('集成成果', 'Integrate') }}<MdiIcon :path="mdiArrowRight" :size="16" /></button></div>
            <pre v-if="diff && inspectedArtifact === artifact.id" class="coop-diff">{{ diff }}</pre>
          </article>
        </div>
        <footer class="coop-footer">
          <details class="coop-settings"><summary><MdiIcon :path="mdiCogOutline" :size="16" />{{ text('协作偏好', 'Preferences') }}</summary><label>{{ text('最大委派深度', 'Maximum delegation depth') }}<select v-model="maxDepth" :disabled="busy" @change="act(() => settingAPI.update({ subagentMaxDepth: maxDepth }))"><option v-for="n in [0,1,2,3,4]" :key="n" :value="n">{{ n === 0 ? text('关闭', 'Off') : n }}</option></select></label></details>
          <span v-if="budget" class="coop-usage">{{ text('本次用量', 'Request usage') }}<strong>{{ budget.consumed.toLocaleString() }}</strong><span>/ {{ budget.budget.toLocaleString() }} tokens</span></span>
        </footer>
      </section>
    </div>
  </Teleport>
</template>

<style scoped>
.cooperative-entry { display: flex; justify-content: flex-end; padding: 0 16px 8px; flex-shrink: 0; }
.coop-trigger { display: flex; align-items: center; gap: 7px; padding: 6px 10px; border: 1px solid var(--card-border); border-radius: 9px; background: var(--bg-main); color: var(--text-secondary); font-size: 12px; cursor: pointer; transition: background 160ms; }
.coop-trigger:hover { background: var(--primary-alpha-06); }
.coop-count { font-variant-numeric: tabular-nums; border-radius: 5px; background: var(--primary-alpha-08); padding: 0 5px; }
.coop-running { display: flex; align-items: center; gap: 5px; color: var(--tool-success-text); }
.coop-attention { color: var(--tool-pending-text); }
.coop-overlay { position: fixed; inset: 0; z-index: 90; display: flex; justify-content: flex-end; background: var(--overlay-backdrop); backdrop-filter: blur(3px); }
.coop-panel { width: min(880px, 94vw); height: 100%; display: flex; flex-direction: column; background: var(--bg-main); color: var(--text-primary); box-shadow: -16px 0 60px rgba(0, 0, 0, .16); overflow: hidden; font-size: 13px; animation: coop-enter 220ms var(--ease-out-smooth); }
.coop-panel button { display: inline-flex; align-items: center; justify-content: center; gap: 6px; min-height: 34px; padding: 7px 11px; border: 1px solid var(--card-border); border-radius: 8px; background: var(--bg-main); color: var(--text-secondary); font: inherit; cursor: pointer; transition: background 160ms, border-color 160ms; }
.coop-panel button:hover:not(:disabled) { background: var(--primary-alpha-06); border-color: var(--primary-alpha-22); }
.coop-panel button:disabled { opacity: .45; cursor: default; }
.coop-panel button:focus-visible, .coop-panel summary:focus-visible, .coop-trigger:focus-visible { outline: 2px solid var(--color-primary); outline-offset: 3px; }
.coop-panel .coop-primary { background: var(--btn-primary-gradient); border-color: transparent; color: #fff; font-weight: 600; }
.coop-panel .coop-primary:hover:not(:disabled) { background: var(--color-primary-hover); border-color: transparent; }
.coop-panel .coop-icon-button { padding: 7px; width: 36px; height: 36px; border: none; background: transparent; }
.coop-panel .coop-stop { color: var(--color-danger); background: var(--danger-alpha-04); border-color: var(--danger-alpha-12); }
.coop-header { padding: 26px 28px 21px; display: flex; justify-content: space-between; align-items: center; gap: 12px; }
.coop-heading, .coop-header-actions { display: flex; align-items: center; gap: 12px; }
.coop-brand { width: 48px; height: 48px; border-radius: 15px; background: var(--primary-alpha-10); color: var(--color-primary); display: flex; align-items: center; justify-content: center; box-shadow: inset 0 0 0 1px var(--primary-alpha-10); }
.coop-header h2 { font-size: 20px; font-weight: 650; letter-spacing: -.4px; margin: 0; }
.coop-header p, .coop-section-heading p, .coop-artifact-heading p { font-size: 12px; color: var(--text-secondary); margin: 6px 0 0; }
.coop-overview { margin: 0 28px 22px; display: grid; grid-template-columns: repeat(3, 1fr); background: var(--primary-alpha-04); border: 1px solid var(--card-border); border-radius: 13px; padding: 16px 0; }
.coop-overview > div { padding: 0 22px; border-right: 1px solid var(--card-border); }
.coop-overview > div:last-child { border: none; }
.coop-overview-label { display: flex; align-items: center; gap: 6px; font-size: 12px; color: var(--text-secondary); margin-bottom: 8px; }
.coop-overview strong { font-size: 25px; font-weight: 650; font-variant-numeric: tabular-nums; line-height: 1; }
.coop-overview small { font-size: 12px; font-weight: 400; color: var(--text-secondary); margin-left: 9px; }
.coop-tabs { display: flex; gap: 6px; padding: 0 28px 12px; border-bottom: 1px solid var(--card-border); }
.coop-tabs button { flex: 1; border-color: transparent; background: transparent; font-weight: 500; }
.coop-tabs button.selected { background: var(--primary-alpha-10); color: var(--color-primary); border-color: var(--primary-alpha-12); }
.coop-tab-count { margin-left: 3px; padding: 0 6px; border-radius: 5px; font-size: 11px; background: var(--primary-alpha-08); font-variant-numeric: tabular-nums; }
.coop-dot { display: inline-block; width: 6px; height: 6px; border-radius: 50%; background: currentColor; flex-shrink: 0; }
.coop-overview-label .coop-dot { color: var(--text-muted); }
.coop-overview-label .coop-dot.live { color: var(--tool-success-dot); box-shadow: 0 0 0 3px var(--tool-success-bg); }
.coop-status { display: inline-flex; align-items: center; gap: 5px; width: fit-content; color: var(--text-secondary); font-size: 11px; line-height: 1.5; background: var(--primary-alpha-06); border-radius: 6px; padding: 3px 7px; }
.coop-status[data-status="succeeded"], .coop-status[data-status="completed"], .coop-status[data-status="integrated"] { color: var(--tool-success-text); background: var(--tool-success-bg); }
.coop-status[data-status="running"], .coop-status[data-status="in_progress"] { color: var(--color-primary); background: var(--primary-alpha-10); }
.coop-status[data-status="failed"], .coop-status[data-status="conflict"], .coop-status[data-status="validation_failed"] { color: var(--color-danger); background: var(--danger-alpha-08); }
.coop-status[data-status="waiting"], .coop-status[data-status="needs_attention"], .coop-status[data-status="awaiting_integration"], .coop-status[data-status="ready"] { color: var(--tool-pending-text); background: var(--warning-alpha-12); }
.coop-body { display: grid; grid-template-columns: 245px minmax(0, 1fr); flex: 1; min-height: 0; }
.coop-agent-list { overflow-y: auto; padding: 20px 12px; border-right: 1px solid var(--card-border); background: var(--primary-alpha-04); }
.coop-list-caption { margin: 0 12px 14px; color: var(--text-secondary); font-size: 11px; font-weight: 600; }
.coop-agent-list button { width: 100%; display: flex; flex-direction: column; align-items: stretch; padding: 14px 12px; margin-bottom: 8px; border-color: transparent; border-radius: 11px; text-align: left; background: transparent; gap: 10px; }
.coop-agent-list button.selected { background: var(--bg-main); border-color: var(--primary-alpha-20); box-shadow: 0 3px 12px var(--primary-alpha-06); }
.coop-agent-row { display: flex; align-items: center; gap: 10px; min-width: 0; }
.coop-avatar { flex-shrink: 0; display: flex; align-items: center; justify-content: center; width: 35px; height: 35px; border-radius: 10px; background: var(--primary-alpha-08); color: var(--color-primary); }
.coop-agent-name { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
.coop-agent-name strong { font-size: 13px; color: var(--text-primary); font-weight: 600; overflow-wrap: anywhere; }
.coop-agent-name > span { font-size: 11px; color: var(--text-secondary); }
.coop-agent-meta { padding-left: 45px; }
.coop-activity { font-size: 11px; color: var(--text-secondary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.coop-agent-detail { padding: 26px; overflow-y: auto; min-width: 0; }
.coop-detail-title { display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-bottom: 14px; }
.coop-detail-title h3 { font-size: 17px; font-weight: 650; margin: 5px 0 0; overflow-wrap: anywhere; }
.coop-eyebrow { display: block; font-size: 11px; font-weight: 500; color: var(--text-secondary); margin-bottom: 5px; }
.coop-badges { display: flex; gap: 6px; flex-wrap: wrap; align-items: center; margin-bottom: 18px; }
.coop-badges > span:not(.coop-status) { border: 1px solid var(--card-border); padding: 3px 7px; border-radius: 6px; font-size: 11px; color: var(--text-secondary); }
.coop-task-details { margin: 12px 0; color: var(--text-secondary); font-size: 12px; }
.coop-task-details summary { cursor: pointer; line-height: 1.6; overflow-wrap: anywhere; }
.coop-task-details summary > svg { display: inline-block; vertical-align: middle; margin-right: 5px; }
.coop-task-details p { white-space: pre-wrap; line-height: 1.8; overflow-wrap: anywhere; margin: 10px 0; }
.coop-task-details code { display: block; overflow-wrap: anywhere; padding-top: 8px; font-size: 11px; }
.coop-scope { border: 1px solid var(--card-border); border-radius: 10px; padding: 12px 14px; background: var(--primary-alpha-04); }
.coop-report { margin: 24px 0; font-size: 14px; line-height: 1.85; overflow-wrap: anywhere; }
.coop-report-pending { padding: 40px 12px; text-align: center; color: var(--text-secondary); }
.coop-report-pending svg { margin: 0 auto; color: var(--color-primary); }
.coop-panel .coop-history-button { border-color: transparent; background: transparent; padding-left: 0; font-size: 12px; }
.coop-history { border-left: 2px solid var(--card-border); padding: 6px 0 6px 12px; margin-top: 12px; }
.coop-followup { margin-top: 24px; padding: 15px; border: 1px solid var(--input-border); border-radius: 12px; background: var(--input-bg); }
.coop-followup label { display: block; font-size: 12px; font-weight: 600; margin-bottom: 10px; }
.coop-panel input, .coop-panel textarea, .coop-panel select { font: inherit; color: var(--text-primary); background: var(--input-bg); border: 1px solid var(--input-border); border-radius: 8px; padding: 9px 10px; max-width: 100%; }
.coop-panel input:focus, .coop-panel textarea:focus, .coop-panel select:focus { outline: none; border-color: var(--color-primary); box-shadow: var(--focus-ring-shadow); }
.coop-panel input::placeholder, .coop-panel textarea::placeholder { color: var(--text-muted); }
.coop-panel textarea { width: 100%; resize: vertical; line-height: 1.65; }
.coop-followup textarea { padding: 0; border: none; border-radius: 0; background: transparent; box-shadow: none; }
.coop-followup:focus-within { border-color: var(--color-primary); box-shadow: var(--focus-ring-shadow); }
.coop-composer-actions { display: flex; justify-content: space-between; align-items: center; gap: 10px; margin-top: 12px; }
.coop-composer-actions select { font-size: 11px; border-color: transparent; background: var(--primary-alpha-06); padding: 6px 8px; }
.coop-collection { flex: 1; min-height: 0; overflow-y: auto; padding: 24px 28px; }
.coop-section-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 22px; }
.coop-section-heading h3 { font-size: 15px; font-weight: 600; margin: 0; }
.coop-board { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 13px; align-items: start; }
.coop-lane { background: var(--primary-alpha-04); border: 1px solid var(--card-border); border-radius: 12px; padding: 12px 10px; min-width: 0; }
.coop-lane > h3 { display: flex; align-items: center; gap: 7px; font-size: 12px; margin: 2px 3px 14px; font-weight: 600; }
.coop-lane > h3 .coop-dot { color: var(--text-muted); }
.coop-lane[data-lane="active"] > h3 .coop-dot { color: var(--color-primary); }
.coop-lane[data-lane="done"] > h3 .coop-dot { color: var(--tool-success-dot); }
.coop-lane > h3 span { margin-left: auto; background: var(--primary-alpha-08); border-radius: 5px; padding: 1px 6px; font-size: 11px; font-variant-numeric: tabular-nums; }
.coop-lane-empty { text-align: center; padding: 30px 4px; color: var(--text-secondary); font-size: 11px; }
.coop-lane-empty svg { margin: 0 auto; opacity: .55; }
.coop-lane-empty p { margin: 8px 0 0; }
.coop-task-card { border: 1px solid var(--card-border); border-radius: 10px; padding: 14px; background: var(--bg-main); margin-bottom: 10px; box-shadow: 0 2px 5px var(--primary-alpha-04); overflow-wrap: anywhere; }
.coop-task-card:last-child { margin-bottom: 0; }
.coop-task-card h4 { margin: 12px 0 8px; font-size: 14px; font-weight: 600; line-height: 1.6; }
.coop-card-description { font-size: 12px; line-height: 1.75; color: var(--text-secondary); white-space: pre-wrap; margin: 0 0 14px; }
.coop-secondary, .coop-dependency { font-size: 11px; color: var(--text-secondary); margin: 8px 0; line-height: 1.6; }
.coop-dependency { display: flex; align-items: center; gap: 5px; }
.coop-owner { display: flex; flex-direction: column; gap: 6px; font-size: 11px; color: var(--text-secondary); margin: 14px 0; }
.coop-owner select { width: 100%; font-size: 11px; padding: 7px; }
.coop-owner-static { display: flex; align-items: center; gap: 6px; font-size: 11px; color: var(--text-secondary); margin: 15px 0; }
.coop-task-actions { display: flex; gap: 7px; flex-wrap: wrap; justify-content: flex-end; margin-top: 14px; }
.coop-task-card .coop-task-actions { justify-content: flex-start; border-top: 1px solid var(--card-border); padding-top: 12px; }
.coop-task-card .coop-task-actions button { padding: 5px 8px; font-size: 11px; min-height: 30px; }
.coop-panel .coop-text-button { border-color: transparent; background: transparent; padding-left: 0; }
.coop-new-task { display: flex; flex-direction: column; gap: 10px; padding: 18px; margin-bottom: 22px; border: 1px solid var(--input-border); border-radius: 12px; background: var(--primary-alpha-04); }
.coop-new-task label { font-size: 12px; font-weight: 500; }
.coop-artifact-card { border: 1px solid var(--card-border); border-radius: 14px; padding: 20px; background: var(--bg-main); margin-bottom: 16px; }
.coop-artifact-heading { display: flex; align-items: center; gap: 11px; min-width: 0; }
.coop-artifact-heading h3 { font-size: 14px; margin: 0; }
.coop-artifact-report { font-size: 13px; line-height: 1.85; white-space: pre-wrap; overflow-wrap: anywhere; margin: 16px 0; }
.coop-artifact-actions { display: flex; justify-content: space-between; align-items: center; border-top: 1px solid var(--card-border); padding-top: 15px; margin-top: 16px; gap: 10px; }
.coop-create { display: flex; gap: 8px; margin-top: 12px; }
.coop-create input { flex: 1; min-width: 0; font-family: var(--font-mono); font-size: 12px; }
.coop-validation-ok { color: var(--tool-success-text); margin-left: 10px; }
.coop-diff { padding: 14px; margin-top: 12px; border: 1px solid var(--card-border); border-radius: 9px; font-family: var(--font-mono); font-size: 11px; line-height: 1.7; background: var(--primary-alpha-04); color: var(--text-primary); overflow: auto; max-height: 380px; tab-size: 2; }
.coop-empty { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; padding: 50px 25px; min-height: 250px; text-align: center; }
.coop-empty-icon { display: flex; align-items: center; justify-content: center; width: 68px; height: 68px; border-radius: 22px; background: var(--primary-alpha-08); color: var(--color-primary); margin-bottom: 18px; }
.coop-empty h3 { font-size: 16px; font-weight: 600; margin: 0 0 10px; }
.coop-empty p { font-size: 13px; color: var(--text-secondary); line-height: 1.8; max-width: 320px; margin: 0; }
.coop-approvals { max-height: 30%; overflow: auto; padding: 14px 28px 0; flex-shrink: 0; }
.coop-approval { display: flex; align-items: flex-start; gap: 10px; border: 1px solid var(--warning-alpha-20); background: var(--warning-alpha-12); border-radius: 10px; padding: 13px; margin-bottom: 10px; color: var(--tool-pending-text); }
.coop-approval > svg { flex-shrink: 0; margin-top: 3px; }
.coop-approval-detail { flex: 1; min-width: 0; }
.coop-approval-detail summary { cursor: pointer; overflow-wrap: anywhere; font-size: 12px; }
.coop-approval-detail summary::marker { font-size: 10px; }
.coop-approval-detail .coop-eyebrow { display: inline-block; width: calc(100% - 16px); color: var(--tool-pending-text); font-weight: 600; }
.coop-approval-detail pre { max-height: 180px; overflow: auto; font-size: 11px; white-space: pre-wrap; overflow-wrap: anywhere; }
.coop-approval-actions { display: flex; gap: 6px; flex-shrink: 0; }
.coop-error { flex-shrink: 0; max-height: 110px; overflow: auto; font-size: 12px; color: var(--color-danger); padding: 12px 15px; margin: 12px 28px; background: var(--danger-alpha-08); border-radius: 9px; overflow-wrap: anywhere; }
.coop-agent-detail .coop-error { margin: 12px 0; }
.coop-footer { display: flex; align-items: center; justify-content: space-between; flex-shrink: 0; gap: 12px; padding: 14px 28px; border-top: 1px solid var(--card-border); color: var(--text-secondary); font-size: 11px; background: var(--primary-alpha-04); }
.coop-settings { position: relative; }
.coop-settings summary { cursor: pointer; display: flex; align-items: center; gap: 6px; min-height: 28px; list-style: none; }
.coop-settings summary::-webkit-details-marker { display: none; }
.coop-settings label { position: absolute; bottom: calc(100% + 16px); left: 0; padding: 16px; border: 1px solid var(--card-border); border-radius: 12px; box-shadow: var(--page-card-shadow); background: var(--bg-main); display: flex; align-items: center; gap: 15px; white-space: nowrap; font-size: 12px; }
.coop-settings select { padding: 5px 8px; }
.coop-usage { display: flex; align-items: center; gap: 7px; font-variant-numeric: tabular-nums; flex-wrap: wrap; justify-content: flex-end; }
.coop-usage strong { color: var(--text-primary); font-weight: 600; }
@keyframes coop-enter { from { opacity: .5; transform: translateX(18px); } to { opacity: 1; transform: translateX(0); } }
@media (max-width: 700px) {
  .coop-panel { width: 100%; }
  .coop-header { padding: 20px 18px 18px; }
  .coop-header h2 { font-size: 18px; }
  .coop-header p { font-size: 11px; }
  .coop-brand { width: 40px; height: 40px; border-radius: 12px; }
  .coop-heading { gap: 10px; }
  .coop-header-actions { gap: 3px; }
  .coop-header-actions .coop-stop { font-size: 11px; padding: 6px; }
  .coop-overview { margin: 0 18px 18px; }
  .coop-overview > div { padding: 0 13px; }
  .coop-overview strong { font-size: 22px; }
  .coop-overview small { margin-left: 5px; font-size: 10px; }
  .coop-overview-label { font-size: 10px; }
  .coop-tabs { padding: 0 18px 12px; }
  .coop-body { display: flex; flex-direction: column; }
  .coop-agent-list { flex-shrink: 0; display: flex; gap: 8px; overflow: auto; max-height: 145px; padding: 12px 18px; border-right: none; border-bottom: 1px solid var(--card-border); }
  .coop-list-caption { display: none; }
  .coop-agent-list button { width: 200px; flex-shrink: 0; margin: 0; padding: 10px; }
  .coop-agent-name strong { white-space: nowrap; text-overflow: ellipsis; overflow: hidden; }
  .coop-agent-detail { padding: 20px 18px; }
  .coop-collection { padding: 22px 18px; }
  .coop-section-heading p { font-size: 11px; max-width: 195px; line-height: 1.6; }
  .coop-section-heading button { font-size: 11px; white-space: nowrap; }
  .coop-board { grid-template-columns: 1fr; gap: 18px; }
  .coop-lane-empty { padding: 15px 4px; }
  .coop-lane-empty svg { display: none; }
  .coop-task-card { padding: 16px; }
  .coop-artifact-card { padding: 16px; }
  .coop-artifact-card .coop-detail-title { align-items: flex-start; }
  .coop-artifact-heading { gap: 8px; }
  .coop-approvals { padding: 12px 18px 0; }
  .coop-approval { flex-wrap: wrap; }
  .coop-approval-detail { flex-basis: calc(100% - 36px); }
  .coop-approval-actions { margin-left: auto; }
  .coop-footer { padding: 12px 18px; }
  .coop-usage { gap: 4px; font-size: 10px; }
  .coop-usage > span { display: none; }
  .coop-error { margin: 12px 18px; }
}
@media (prefers-reduced-motion: reduce) { .coop-panel { animation: none; } .coop-panel button, .coop-trigger { transition: none; } }
</style>
