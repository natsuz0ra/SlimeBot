import type { ACPConfig } from '@/api/acp'
import type { SelectOption } from '@/components/ui/AppSelect.vue'

export function acpEngineId(agentId: string) {
  return `acp:${agentId}`
}

export function isACPEngine(value: string) {
  return value.startsWith('acp:') && value.length > 4
}

export function acpAgentIdFromEngine(value: string | undefined) {
  return value && isACPEngine(value) ? value.slice(4) : undefined
}

export function composeEngineOptions(local: SelectOption[], external: ACPConfig[], localGroup: string, externalGroup: string) {
  const agents = external.filter(agent => agent.isEnabled)
  if (!agents.length) return local
  return [
    ...local.map(option => ({ ...option, group: localGroup })),
    ...agents.map(agent => ({ value: acpEngineId(agent.id), label: agent.name, group: externalGroup })),
  ]
}

export function chooseEngine(options: SelectOption[], selected: string | undefined, boundAgent: string | undefined, remembered: string | null) {
  if (selected && options.some(option => option.value === selected)) return selected
  if (boundAgent) return acpEngineId(boundAgent)
  if (remembered && options.some(option => option.value === remembered)) return remembered
  return options[0]?.value || ''
}

export function parseACPArguments(input: string): string[] {
  if (!input.trim()) return []
  if (input.trimStart().startsWith('[')) {
    const args: unknown = JSON.parse(input)
    if (!Array.isArray(args) || args.some(arg => typeof arg !== 'string')) throw new TypeError('Expected a JSON array of strings')
    return args
  }
  return input.split(/\r?\n/).filter(line => line !== '')
}
