import type { CooperativeAgent, CooperativeTurn } from '@/api/cooperative'

export function latestAgentTurns(turns: CooperativeTurn[]) {
  const latest = new Map<string, CooperativeTurn>()
  for (const turn of turns) {
    const old = latest.get(turn.agentId)
    if (!old || Date.parse(turn.createdAt) > Date.parse(old.createdAt) || (turn.id === old.id && turn.revision > old.revision)) latest.set(turn.agentId, turn)
  }
  return latest
}
export function orderedAgentTree(agents: CooperativeAgent[], root: string) {
  const ordered: CooperativeAgent[] = []
  const seen = new Set<string>()
  const visit = (parent: string) => {
    for (const agent of agents.filter(a => a.parentId === parent)) {
      if (seen.has(agent.id)) continue
      seen.add(agent.id); ordered.push(agent); visit(agent.id)
    }
  }
  visit(root)
  for (const agent of agents) if (!seen.has(agent.id)) ordered.push(agent)
  return ordered
}
export function cooperativeStatus(status: string, english = false) {
  const labels: Record<string, [string, string]> = {
    running: ['运行中', 'Running'], succeeded: ['已完成', 'Completed'], completed: ['已完成', 'Completed'],
    pending: ['待领取', 'Pending'], in_progress: ['处理中', 'In progress'], waiting: ['等待中', 'Waiting'],
    failed: ['失败', 'Failed'], canceled: ['已停止', 'Stopped'], interrupted: ['已中断', 'Interrupted'],
    needs_attention: ['需要处理', 'Needs attention'], awaiting_integration: ['等待集成', 'Awaiting integration'],
    ready: ['待集成', 'Ready'], working: ['修改中', 'Working'], integrated: ['已集成', 'Integrated'],
    conflict: ['有冲突', 'Conflict'], validation_failed: ['验证失败', 'Validation failed'], stopping: ['停止中', 'Stopping'],
  }
  return labels[status]?.[english ? 1 : 0] || status
}

// Artifact routing metadata is displayed by the artifact card, separately from
// the agent's report, so an old captured state does not contradict live status.
export function cooperativeReport(answer: string) {
  return answer.replace(/\n\nArtifact: [0-9a-f-]+ \([a-z_]+\); requires integration before the write task is complete\.$/i, '')
}

export function cooperativeError(error: unknown, english = false): string {
  const value = error as { response?: { data?: { error?: unknown } }; message?: string }
  const server = value?.response?.data?.error
  const message = typeof server === 'string' ? server : value?.message || String(error)
  const labels: Array<[string, string, string]> = [
    ['artifact must pass validation', '成果尚未通过验证，请先运行验证。', 'Verify this result before integrating it.'],
    ['agent is still using this workspace', '代理仍在使用工作目录，请等待当前轮次结束。', 'Wait for the agent to finish using this workspace.'],
    ['parent agent is still using this workspace', '父代理仍在使用工作目录，请等待其轮次结束。', 'Wait for the parent agent to finish using this workspace.'],
    ['validation modified', '验证命令修改了源文件，当前工作区未被覆盖。请检查并重新验证。', 'Verification changed source files. Review the changes and verify again.'],
    ['artifact validation failed', '成果验证未通过，请查看验证记录。', 'Verification failed. Check the validation record.'],
    ['inbox_full', '接收队列已满，请等待当前任务处理后再发送。', 'The inbox is full. Wait for current work to finish.'],
    ['capacity_exceeded', '代理数量已达上限，请结束当前请求后再分派任务。', 'Agent capacity is full. Finish the current request before delegating again.'],
    ['network error', '连接暂不可用，请稍后重试。', 'Connection unavailable. Please retry shortly.'],
  ]
  return labels.find(([match]) => message.toLowerCase().includes(match))?.[english ? 2 : 1] || message
}
