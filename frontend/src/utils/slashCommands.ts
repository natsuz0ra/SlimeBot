export const builtInCommands = ['terminal', 'workspace', 'compact', 'checkpoints', 'export', 'plan', 'clear-privacy', 'help'] as const
export type WorkspaceCommand = typeof builtInCommands[number]

export type SlashCommand =
  | { id: string; kind: 'workbench'; slash: WorkspaceCommand; description: string }
  | { id: string; kind: 'skill'; slash: string; description: string; content: string }

interface SlashSkill {
  id: string
  name: string
  description: string
  enabled: boolean
}

export function slashQuery(text: string, caret: number): string | undefined {
  if (caret !== text.length) return undefined
  return /^\/([a-z\d_-]*)$/i.exec(text)?.[1]?.toLowerCase()
}

export function getSlashCommands(
  skills: SlashSkill[],
  describeBuiltin: (command: WorkspaceCommand) => string,
  skillPrompt: (name: string) => string,
): SlashCommand[] {
  const systems: SlashCommand[] = builtInCommands.map(slash => ({
    id: `workbench:${slash}`, kind: 'workbench', slash, description: describeBuiltin(slash),
  }))
  const occupied = new Set<string>(builtInCommands)
  const skillCommands: SlashCommand[] = []
  for (const skill of skills) {
    if (!skill.enabled) continue
    const normalized = skill.name.toLowerCase().replace(/[^a-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 32)
    const base = /^[a-z0-9]/.test(normalized) ? normalized : 'skill'
    let slash = base
    for (let suffix = 2; occupied.has(slash); suffix++) {
      slash = `${base.slice(0, 24)}-${suffix}`
    }
    occupied.add(slash)
    skillCommands.push({
      id: `skill:${skill.id}`, kind: 'skill', slash, description: skill.description || skill.name,
      content: skillPrompt(skill.name),
    })
  }
  return [...systems, ...skillCommands]
}

export function filterSlashCommands(commands: SlashCommand[], query: string) {
  const term = query.toLowerCase()
  return commands.filter(command => command.slash.includes(term) || command.description.toLowerCase().includes(term))
}
