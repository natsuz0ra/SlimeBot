/**
 * 命令定义、匹配和 tab 补全。
 */

import { SUPPORTED_COMMANDS, type CommandMeta } from "../types.js";

const MAX_HINTS = 5;

/** 按前缀匹配返回命令提示列表 */
export function matchCommandHints(input: string): CommandMeta[] {
  const trimmed = input.trim();
  if (!trimmed.startsWith("/")) return [];
  const matched: CommandMeta[] = [];
  for (const cmd of SUPPORTED_COMMANDS) {
    if (cmd.command.startsWith(trimmed)) {
      matched.push(cmd);
      if (matched.length >= MAX_HINTS) break;
    }
  }
  return matched;
}

/** Tab 补全：返回第一个匹配的完整命令 */
export function completeCommand(input: string): string | null {
  const matched = matchCommandHints(input);
  if (matched.length === 0) return null;
  return matched[0].command;
}

/** 判断输入是否是命令（以 / 开头） */
export function isCommand(input: string): boolean {
  return input.trim().startsWith("/");
}
