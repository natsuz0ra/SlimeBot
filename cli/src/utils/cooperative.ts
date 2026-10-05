import type { CooperativeSnapshot, CooperativeTurn } from "../types/cooperative.js";
import { wrapText } from "./format.js";
import { getGraphemeSegmenter } from "./intl.js";
import { stringWidth } from "./stringWidth.js";
import { CLI_ACCENT_COLOR } from "./terminal.js";
export const COOPERATIVE_COLORS = {
  accent: CLI_ACCENT_COLOR, text: "#f8fafc", secondary: "#cbd5e1", muted: "#94a3b8",
  hint: "#94a3b8", success: "#34d399", warning: "#fbbf24", danger: "#fb7185",
} as const;
export interface CooperativeRow {
  id: string;
  title: string;
  status: string;
  subtitle: string;
  report: string;
  turnId?: string;
}
export interface CooperativeLine {
  text: string;
  color: string;
  bold?: boolean;
}

const labels: Record<string, string> = {
  pending: "Pending", running: "Running", waiting: "Waiting", stopping: "Stopping", succeeded: "Completed", completed: "Completed",
  failed: "Failed", canceled: "Stopped", interrupted: "Interrupted", in_progress: "In progress", needs_attention: "Needs attention",
  awaiting_integration: "Awaiting merge", ready: "Ready", working: "Working", integrated: "Integrated", conflict: "Conflict",
  validation_failed: "Check failed", approved: "Approved", rejected: "Rejected",
};
export function cooperativeStatus(status: string): string { return labels[status] || status || "Pending"; }
export function cooperativeStatusColor(status: string): string {
  if (["completed", "succeeded", "integrated", "approved"].includes(status))
    return COOPERATIVE_COLORS.success;
  if (["failed", "conflict", "validation_failed", "rejected"].includes(status))
    return COOPERATIVE_COLORS.danger;
  if (["running", "waiting", "stopping", "needs_attention", "awaiting_integration", "ready", "pending"].includes(status))
    return COOPERATIVE_COLORS.warning;
  return COOPERATIVE_COLORS.muted;
}
export function latestCooperativeTurns(turns: CooperativeTurn[]): Map<string, CooperativeTurn> {
  const latest = new Map<string, CooperativeTurn>();
  for (const turn of turns) {
    const old = latest.get(turn.agentId);
    if (!old || Date.parse(turn.createdAt) > Date.parse(old.createdAt) || (turn.id === old.id && (turn.revision || 0) >= (old.revision || 0)))
      latest.set(turn.agentId, turn);
  }
  return latest;
}
function approvalPayload(payload: string): {
  title: string;
  report: string;
} {
  try {
    const value = JSON.parse(payload);
    return { title: [value.toolName, value.command].filter(Boolean).join(" · ") || "Tool execution", report: JSON.stringify(value.params || value, null, 2) };
  }
  catch {
    return { title: "Tool execution", report: payload };
  }
}
export function cooperativeRows(snapshot: CooperativeSnapshot | null, tab: number): CooperativeRow[] {
  if (!snapshot)
    return [];
  const latest = latestCooperativeTurns(snapshot.turns);
  if (tab === 0)
    return snapshot.agents.map(agent => {
      const turn = latest.get(agent.id);
      return { id: agent.id, title: agent.title, status: turn?.status || "pending", subtitle: `${agent.profile} · ${agent.workspace || "No workspace"}`, report: [agent.task, turn?.error, turn?.answer || "Waiting for the agent report."].filter(Boolean).join("\n\n"), turnId: turn?.id };
    });
  if (tab === 1)
    return snapshot.tasks.map(task => ({ id: task.id, title: task.title, status: task.status, subtitle: `Owner: ${snapshot.agents.find(a => a.id === task.ownerId)?.title || (task.ownerId ? "Lead agent" : "Unassigned")} · revision ${task.revision}`, report: [task.description, task.acceptance, task.result].filter(Boolean).join("\n\n") || "No task details yet." }));
  if (tab === 2)
    return snapshot.artifacts.map(artifact => ({ id: artifact.id, title: snapshot.agents.find(a => a.id === artifact.agentId)?.title || "Worker result", status: artifact.status, subtitle: artifact.workspace, report: [artifact.report, artifact.validation && `Validation\n${artifact.validation}`].filter(Boolean).join("\n\n") || "No result report yet." }));
  return snapshot.approvals.filter(a => a.status === "pending").map(approval => {
    const payload = approvalPayload(approval.payload);
    return { id: approval.id, title: payload.title, status: approval.status, subtitle: `Agent: ${snapshot.agents.find(a => a.id === approval.agentId)?.title || approval.agentId}`, report: payload.report };
  });
}
// Bound by display cells, not UTF-16 length. Commands and reports may contain
// Chinese, emoji, ANSI sequences or newlines from a tool result.
export function fitCooperativeText(value: string, columns: number): string {
  const text = String(value || "").replace(/\x1b\[[0-?]*[ -/]*[@-~]/g, "").replace(/[\x00-\x1f\x7f-\x9f]/g, " ");
  const width = Math.max(1, columns);
  if (stringWidth(text) <= width)
    return text;
  let out = "";
  for (const { segment: character } of getGraphemeSegmenter().segment(text)) {
    if (stringWidth(out + character) > width - 1)
      break;
    out += character;
  }
  return out + "…";
}
export function cooperativeReportPage(report: string, columns: number, height: number, offset: number) {
  const safe = report.replace(/\x1b\[[0-?]*[ -/]*[@-~]/g, "").replace(/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f-\x9f]/g, "");
  const lines = wrapText(safe, Math.max(1, columns)).split("\n");
  const size = Math.max(1, height), start = Math.min(Math.max(0, offset), Math.max(0, lines.length - size));
  return { lines: lines.slice(start, start + size), offset: start, total: lines.length, size };
}
export function cooperativeWorkspaceLines(input: {
  snapshot: CooperativeSnapshot | null;
  tab: number;
  cursor: number;
  columns: number;
  height: number;
  detail?: boolean;
  offset?: number;
  report?: string;
  loading?: boolean;
  error?: string;
  command?: boolean;
}): CooperativeLine[] {
  const width = Math.max(8, input.columns - 2), height = Math.max(8, input.height);
  const rows = cooperativeRows(input.snapshot, input.tab), cursor = Math.max(0, Math.min(rows.length - 1, input.cursor)), selected = rows[cursor];
  const c = COOPERATIVE_COLORS;
  const lines: CooperativeLine[] = [];
  const add = (text: string, color: string = c.secondary, bold = false) => lines.push({ text: fitCooperativeText(text, width), color, bold });
  add("Collaboration", c.accent, true);
  const counts = [input.snapshot?.agents.length || 0, input.snapshot?.tasks.length || 0, input.snapshot?.artifacts.length || 0, input.snapshot?.approvals.filter(a => a.status === "pending").length || 0];
  const names = width < 55 ? ["Agents", "Tasks", "Files", "Approve"] : ["Agents", "Tasks", "Artifacts", "Approvals"];
  add(width < 36 ? `${input.tab + 1}/4  ${names[input.tab]} · ${counts[input.tab]}` : names.map((name, index) => `${index === input.tab ? "[" : " "}${name}${width >= 55 ? ` ${counts[index]}` : ""}${index === input.tab ? "]" : " "}`).join(" "), c.accent);
  add("─".repeat(width), c.hint);
  const hints = input.command ? ["Enter execute · Esc cancel"] : input.detail ? ["↑↓ scroll · PgUp/PgDn page", "Esc list · r refresh"] : ["←→ views · ↑↓ select · Enter details", input.tab === 0 ? "s follow-up · x stop · : command" : input.tab === 2 ? "v validate · i integrate · : command" : input.tab === 3 ? "y approve · n reject · : command" : "n new task · : command", "r refresh · Esc back"];
  const bodyLimit = height - lines.length - hints.length - 1;
  const body: CooperativeLine[] = [];
  const b = (text: string, color: string = c.secondary, bold = false) => body.push({ text: fitCooperativeText(text, width), color, bold });
  if (input.error)
    b(input.error, c.danger);
  else if (input.loading)
    b("Refreshing…", c.muted);
  if (input.detail && selected) {
    b(selected.title, c.text, true);
    b(`${cooperativeStatus(selected.status)} · ${selected.id}`, cooperativeStatusColor(selected.status));
    const page = cooperativeReportPage(input.report ?? selected.report, width, Math.max(1, bodyLimit - body.length - 1), input.offset || 0);
    for (const line of page.lines)
      b(line, c.secondary);
    b(`Lines ${page.offset + 1}–${page.offset + page.lines.length}/${page.total}`, c.muted);
  }
  else if (!rows.length) {
    b(input.snapshot ? ["No agents yet.", "No shared tasks yet.", "No worker results yet.", "No pending approvals."][input.tab] : "Loading collaboration…", c.muted);
    b(["Delegate a task from the chat to begin.", "Press n to create a shared task.", "Worker results appear after delegation.", "Tool requests appear here when approval is needed."][input.tab], c.muted);
  }
  else {
    const limit = Math.max(1, Math.min(6, bodyLimit - body.length - 3));
    const start = Math.max(0, Math.min(cursor - Math.floor(limit / 2), rows.length - limit));
    for (let index = start; index < Math.min(rows.length, start + limit); index++) {
      const row = rows[index]!, status = cooperativeStatus(row.status), prefix = index === cursor ? "❯ " : "  ";
      const title = fitCooperativeText(row.title, Math.max(1, width - stringWidth(status) - 3));
      const gap = " ".repeat(Math.max(1, width - stringWidth(prefix + title + status)));
      b(`${prefix}${title}${gap}${status}`, index === cursor ? c.text : cooperativeStatusColor(row.status), index === cursor);
    }
    b(`${cursor + 1}/${rows.length} · ${selected?.subtitle || ""}`, c.muted);
    if (selected)
      b(`ID: ${selected.id}`, c.hint);
    const pending = counts[3];
    const active = [...latestCooperativeTurns(input.snapshot?.turns || []).values()].filter(t => ["running", "waiting", "stopping"].includes(t.status)).length;
    b(`${active} active · ${pending} pending approval${pending === 1 ? "" : "s"}`, pending ? c.warning : c.muted);
  }
  lines.push(...body.slice(0, Math.max(1, bodyLimit)));
  add("─".repeat(width), c.hint);
  for (const hint of hints)
    add(hint, c.hint);
  return lines.slice(0, height);
}
