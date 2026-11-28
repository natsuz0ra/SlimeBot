import test from 'node:test'
import assert from 'node:assert/strict'
import { canStageTerminalCommand, extractTerminalCommand, formatTerminalOutputForChat, isShellLanguage } from '../src/utils/terminalLink'

test('console transcripts only stage prompted commands', () => {
  assert.equal(extractTerminalCommand('$ pwd\r\n/tmp\r\n> git status\r\nclean\r\n', 'console'), 'pwd\ngit status')
  assert.equal(isShellLanguage('PowerShell'), true)
  assert.equal(isShellLanguage('javascript'), false)
})

test('staging rejects controls and hazardous commands and requires bracketed paste for multiple lines', () => {
  for (const command of ['rm -rf ./files', 'rm --force --recursive .', 'dd if=disk', 'sudo mkfs.ext4 disk', 'Remove-Item -Force -Recurse .', 'x\r', '\x1b[201~']) {
    assert.equal(canStageTerminalCommand(command, true), false, command)
  }
  assert.equal(canStageTerminalCommand('pwd', false), true)
  assert.equal(canStageTerminalCommand('pwd\nls', false), false)
  assert.equal(canStageTerminalCommand('pwd\nls', true), true)
})

test('terminal output cannot close its own fence', () => {
  assert.equal(formatTerminalOutputForChat('example\n```\nerror ', '/tmp', 2), 'Terminal (/tmp, exit 2):\n````text\nexample\n```\nerror\n````')
})
