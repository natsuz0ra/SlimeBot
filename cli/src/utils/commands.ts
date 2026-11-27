/**
 * Command definitions, matching, and tab completion.
 */

import { SUPPORTED_COMMANDS, type CommandMeta } from "../types.js";

export const COMMAND_HINT_VISIBLE_LIMIT = 5;

export interface VisibleCommandHints {
  hints: CommandMeta[];
  startIndex: number;
  aboveCount: number;
  belowCount: number;
}

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
    }
  }
  return matched;
}

/** Return the visible hint window while keeping the selected hint in view. */
export function getVisibleCommandHints(
  hints: CommandMeta[],
  selectedIndex: number,
  maxVisible = COMMAND_HINT_VISIBLE_LIMIT,
): VisibleCommandHints {
  if (hints.length === 0 || maxVisible <= 0) {
    return { hints: [], startIndex: 0, aboveCount: 0, belowCount: 0 };
  }

  const visibleCount = Math.min(maxVisible, hints.length);
  const selected = clampSelectedIndex(selectedIndex, hints.length);
  const maxStart = hints.length - visibleCount;
  const startIndex = Math.max(0, Math.min(selected - visibleCount + 1, maxStart));
  const endIndex = startIndex + visibleCount;

  return {
    hints: hints.slice(startIndex, endIndex),
    startIndex,
    aboveCount: startIndex,
    belowCount: hints.length - endIndex,
  };
}

/** Tab completion: first matching full command */
export function completeCommand(input: string, selectedIndex = 0): string | null {
  const matched = matchCommandHints(input);
  if (matched.length === 0) return null;
  return matched[clampSelectedIndex(selectedIndex, matched.length)].command;
}

export function getCommandMeta(command: string): CommandMeta | undefined {
  return SUPPORTED_COMMANDS.find((item) => item.command === command);
}

/**
 * Resolve Enter behavior while slash-command hints are visible:
 * - Zero-arg commands execute immediately in a single Enter.
 * - Args-capable commands execute if already typed in full, otherwise fill with trailing space.
 */
export function resolveCommandHintEnter(
  input: string,
  selectedIndex = 0,
): { action: "run" | "fill"; command: string } | null {
  const matched = matchCommandHints(input);
  if (matched.length === 0) return null;
  const selected = matched[clampSelectedIndex(selectedIndex, matched.length)];
  if (!selected) return null;
  const trimmed = input.trim();
  if (!selected.acceptsArgs || trimmed === selected.command) {
    return { action: "run", command: selected.command };
  }
  return { action: "fill", command: `${selected.command} ` };
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
