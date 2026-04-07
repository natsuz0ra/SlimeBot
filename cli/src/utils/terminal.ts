/**
 * 终端操作工具。
 */

/** 清除终端屏幕并将光标移至左上角 */
export function clearScreen(): void {
  process.stdout.write("\x1b[2J\x1b[3J\x1b[H");
}

/** 获取终端宽度（默认 80） */
export function terminalWidth(): number {
  return process.stdout.columns || 80;
}

/** 获取终端高度（默认 24） */
export function terminalHeight(): number {
  return process.stdout.rows || 24;
}

/**
 * 将文本按前缀缩进多行。多行文本除首行外使用空格填充以对齐前缀。
 * 等价于 Go 版本的 indentMultilineANSI。
 */
export function indentMultiline(prefix: string, body: string): string {
  if (!body) return prefix.trimEnd();
  const lines = body.split("\n");
  const pad = " ".repeat(stripAnsi(prefix).length);
  return lines.map((line, i) => (i === 0 ? prefix : pad) + line).join("\n");
}

/** 粗略去除 ANSI 转义序列以计算可视宽度 */
export function stripAnsi(str: string): string {
  // eslint-disable-next-line no-control-regex
  return str.replace(/\x1b\[[0-9;]*[a-zA-Z]/g, "");
}

/** 获取文本的可视宽度（去除 ANSI 后的长度） */
export function visualWidth(str: string): number {
  return stripAnsi(str).length;
}

/** 检测当前是否支持真彩色 */
export function supportsTrueColor(): boolean {
  const term = process.env.TERM || "";
  const colorterm = process.env.COLORTERM || "";
  return colorterm === "truecolor" || colorterm === "24bit" || term === "xterm-256color";
}

/** 设置终端标题（使用 OSC 序列） */
export function setTerminalTitle(title: string): void {
  if (!process.stdout.isTTY) return;
  process.stdout.write(`\x1b]2;${title}\x07`);
}

export const DOT = "\u25CF"; // ●
