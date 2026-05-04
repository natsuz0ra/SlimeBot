/**
 * Command definitions, matching, and tab completion.
 */

import { SUPPORTED_COMMANDS, type CommandMeta } from "../types.js";

const MAX_HINTS = 5;

function isCommandPrefixInput(input: string): boolean {
  const trimmedStart = input.trimStart();
  return trimmedStart.startsWith("/") && !/\s/.test(trimmedStart);
}

function clampSelectedIndex(index: number, length: number): number {
  if (length <= 0) return 0;
  return Math.max(0, Math.min(length - 1, index));
}

/** Return command hints matching the prefix */
export function matchCommandHints(input: string): CommandMeta[] {
  const trimmed = input.trim();
  if (!isCommandPrefixInput(input)) return [];
  const matched: CommandMeta[] = [];
  for (const cmd of SUPPORTED_COMMANDS) {
    if (cmd.command.startsWith(trimmed)) {
      matched.push(cmd);
      if (matched.length >= MAX_HINTS) break;
    }
  }
  return matched;
}

/** Tab completion: first matching full command */
export function completeCommand(input: string, selectedIndex = 0): string | null {
  const matched = matchCommandHints(input);
  if (matched.length === 0) return null;
  return matched[clampSelectedIndex(selectedIndex, matched.length)].command;
}

/** Move selected command hint cursor with wrap-around */
export function moveCommandHintCursor(current: number, delta: number, total: number): number {
  if (total <= 0) return 0;
  return (current + delta + total) % total;
}

/** Whether input is a command (starts with /) */
export function isCommand(input: string): boolean {
  return input.trim().startsWith("/");
}
