export interface PromptTemplate {
  id: string
  slash: string
  title: string
  description?: string
  content: string
}

export const builtInCommands = ['terminal', 'workspace', 'compact', 'checkpoints', 'export', 'plan', 'clear-privacy', 'help'] as const
export type WorkspaceCommand = typeof builtInCommands[number]

export type SlashCommand =
  | { id: string; kind: 'workbench'; slash: WorkspaceCommand; description: string }
  | { id: string; kind: 'template' | 'skill'; slash: string; description: string; content: string }

interface SlashSkill {
  id: string
  name: string
  description: string
  enabled: boolean
}

export function validTemplateSlash(slash: string) {
  return /^[a-z0-9][a-z0-9_-]{0,31}$/.test(slash) && !builtInCommands.some(command => command === slash)
}

export function slashQuery(text: string, caret: number): string | undefined {
  if (caret !== text.length) return undefined
  return /^\/([a-z\d_-]*)$/i.exec(text)?.[1]?.toLowerCase()
}

export function getSlashCommands(
  templates: PromptTemplate[],
  skills: SlashSkill[],
  describeBuiltin: (command: WorkspaceCommand) => string,
  skillPrompt: (name: string) => string,
): SlashCommand[] {
  const systems: SlashCommand[] = builtInCommands.map(slash => ({
    id: `workbench:${slash}`, kind: 'workbench', slash, description: describeBuiltin(slash),
  }))
  const prompts: SlashCommand[] = templates.map(template => ({
    id: `template:${template.id}`, kind: 'template', slash: template.slash,
    description: template.description || template.title, content: template.content,
  }))
  const occupied = new Set<string>([...builtInCommands, ...templates.map(template => template.slash)])
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
  return [...systems, ...prompts, ...skillCommands]
}

export function filterSlashCommands(commands: SlashCommand[], query: string) {
  const term = query.toLowerCase()
  return commands.filter(command => command.slash.includes(term) || command.description.toLowerCase().includes(term))
}

export function expandSlashTemplate(content: string) {
  const cursor = content.indexOf('{{cursor}}')
  return cursor < 0 ? { text: content, caret: content.length }
    : { text: content.replace(/\{\{cursor\}\}/g, ''), caret: cursor }
}
