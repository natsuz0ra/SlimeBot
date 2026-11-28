const shellLanguages = new Set(['bash', 'sh', 'zsh', 'shell', 'console', 'powershell', 'ps1', 'pwsh', 'cmd', 'bat'])

export function isShellLanguage(language: string) {
  return shellLanguages.has(language.toLowerCase())
}

export function extractTerminalCommand(code: string, language: string): string {
  const text = code.replace(/\r\n/g, '\n').replace(/\n+$/, '')
  if (language.toLowerCase() === 'console') {
    const lines = text.split('\n')
    const commands = lines.filter(line => /^\s*[$>] /.test(line)).map(line => line.replace(/^\s*[$>] /, ''))
    if (commands.length) return commands.join('\n')
  }
  return text
}

export function isDangerousShellCommand(code: string): boolean {
  return /\b(?:mkfs(?:\.\w+)?|format(?:\.com)?|shutdown(?:\.exe)?|reboot|halt|poweroff)\b|\bdd\b[^\n;|]*\bif\s*=|\brm\b(?=[^\n;|]*\s(?:-[a-z]*r|--recursive)\w*)(?=[^\n;|]*\s(?:-[a-z]*f|--force)\w*)|\b(?:remove-item|ri)\b(?=[^\n;|]*\s-recurse\b)(?=[^\n;|]*\s-force\b)/i.test(code)
}

export function canStageTerminalCommand(code: string, bracketedPaste: boolean): boolean {
  // Escape sequences, carriage returns and terminal editing shortcuts must
  // never come from a rendered message.
  return Boolean(code.trim())
    && code.length <= 8192
    && !/[\u0000-\u0009\u000b-\u001f\u007f-\u009f\u2028\u2029]/.test(code)
    && (bracketedPaste || !code.includes('\n'))
    && !isDangerousShellCommand(code)
}

export function formatTerminalOutputForChat(output: string, cwd: string, exitCode?: number) {
  let fenceLength = 3
  for (const match of output.matchAll(/`+/g)) fenceLength = Math.max(fenceLength, match[0].length + 1)
  const fence = '`'.repeat(fenceLength)
  const location = cwd.replace(/[\r\n\u0000-\u001f]/g, ' ')
  return `Terminal (${location}${exitCode === undefined ? '' : `, exit ${exitCode}`}):\n${fence}text\n${output.trimEnd()}\n${fence}`
}
