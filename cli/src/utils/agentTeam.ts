import type { AgentTeamMemberRun, AgentTeamRun, TimelineEntry } from "../types.js";
import { wrapText } from "./format.js";
import { stringWidth } from "./stringWidth.js";

export type AgentTeamDisplayLine = {
  text: string;
  color: "gray" | "green" | "yellow" | "red" | "cyan";
  active?: boolean;
};

function compareTimeAndId(leftAt: string | undefined, leftId: string, rightAt: string | undefined, rightId: string): number {
  const left = Date.parse(leftAt || "");
  const right = Date.parse(rightAt || "");
  return (Number.isFinite(left) ? left : 0) - (Number.isFinite(right) ? right : 0) || leftId.localeCompare(rightId);
}

function sortMembers(members: AgentTeamMemberRun[]): AgentTeamMemberRun[] {
  return [...members].sort((left, right) => compareTimeAndId(left.createdAt || left.startedAt, left.id, right.createdAt || right.startedAt, right.id));
}

function mergeText(previous: string | undefined, next: string | undefined): string | undefined {
  return next === undefined || next === "" ? previous : next;
}

function placeholderRun(teamRunId: string, member?: AgentTeamMemberRun): AgentTeamRun {
  const startedAt = member?.createdAt || member?.startedAt || "";
  return {
    id: teamRunId,
    sessionId: "",
    requestId: "",
    status: "running",
    maxMembers: 8,
    maxParallel: 4,
    startedAt,
    createdAt: startedAt,
    updatedAt: member?.updatedAt || startedAt,
    members: [],
  };
}

function teamEntryIndex(entries: TimelineEntry[], teamRunId: string): number {
  return entries.findIndex((entry) => entry.kind === "team" && entry.teamRun?.id === teamRunId);
}

export function upsertAgentTeamRun(
  entries: TimelineEntry[],
  incoming: Omit<AgentTeamRun, "members"> & { members?: AgentTeamMemberRun[] },
): TimelineEntry[] {
  const next = [...entries];
  const index = teamEntryIndex(next, incoming.id);
  if (index < 0) {
    next.push({ kind: "team", content: "", teamRun: { ...incoming, members: sortMembers(incoming.members || []) } });
    return next;
  }
  const previous = next[index].teamRun!;
  let members = previous.members;
  if (incoming.members) {
    for (const member of incoming.members) {
      const memberIndex = members.findIndex((item) => item.id === member.id);
      members = memberIndex < 0
        ? [...members, member]
        : members.map((item, itemIndex) => itemIndex === memberIndex ? { ...item, ...member } : item);
    }
  }
  next[index] = {
    kind: "team",
    content: "",
    teamRun: {
      ...previous,
      ...incoming,
      sessionId: mergeText(previous.sessionId, incoming.sessionId) || "",
      requestId: mergeText(previous.requestId, incoming.requestId) || "",
      createdAt: incoming.createdAt || previous.createdAt || incoming.startedAt,
      updatedAt: incoming.updatedAt || incoming.finishedAt || previous.updatedAt,
      members: sortMembers(members),
    },
  };
  return next;
}

export function upsertAgentTeamMember(entries: TimelineEntry[], incoming: AgentTeamMemberRun): TimelineEntry[] {
  let next = [...entries];
  let index = teamEntryIndex(next, incoming.teamRunId);
  if (index < 0) {
    next = upsertAgentTeamRun(next, placeholderRun(incoming.teamRunId, incoming));
    index = teamEntryIndex(next, incoming.teamRunId);
  }
  const run = next[index].teamRun!;
  const memberIndex = run.members.findIndex((member) => member.id === incoming.id);
  const members = [...run.members];
  if (memberIndex < 0) {
    members.push(incoming);
  } else {
    const previous = members[memberIndex]!;
    members[memberIndex] = {
      ...previous,
      ...incoming,
      title: mergeText(previous.title, incoming.title) || "",
      task: mergeText(previous.task, incoming.task) || "",
      subagentRunId: mergeText(previous.subagentRunId, incoming.subagentRunId),
      createdAt: incoming.createdAt || previous.createdAt || incoming.startedAt,
      updatedAt: incoming.updatedAt || incoming.finishedAt || previous.updatedAt,
    };
  }
  next[index] = { kind: "team", content: "", teamRun: { ...run, members: sortMembers(members) } };
  return next;
}

export function getAgentTeamStatusLabel(status: string): string {
  return ({
    running: "Running",
    succeeded: "Completed",
    partial_failed: "Partial failure",
    failed: "Failed",
    canceled: "Canceled",
    interrupted: "Interrupted",
  } as Record<string, string>)[status] || status;
}

function memberStatusLabel(status: string): string {
  return ({ queued: "Queued", running: "Running", succeeded: "Completed", failed: "Failed", canceled: "Canceled", interrupted: "Interrupted" } as Record<string, string>)[status] || status;
}

function statusSymbol(status: string): string {
  return status === "running" ? "●" : status === "queued" ? "○" : status === "succeeded" ? "✓" : status === "failed" || status === "partial_failed" ? "✗" : "−";
}

function statusColor(status: string): AgentTeamDisplayLine["color"] {
  if (status === "succeeded") return "green";
  if (status === "running" || status === "queued") return "yellow";
  if (status === "failed" || status === "partial_failed") return "red";
  return "gray";
}

function memberPriority(status: string): number {
  return ({ failed: 0, running: 1, queued: 2, interrupted: 3, canceled: 3, succeeded: 4 } as Record<string, number>)[status] ?? 5;
}

function durationMs(startedAt: string | undefined, finishedAt: string | undefined, now: number): number {
  const start = Date.parse(startedAt || "");
  if (!Number.isFinite(start)) return 0;
  const finish = Date.parse(finishedAt || "");
  return Math.max(0, (Number.isFinite(finish) ? finish : now) - start);
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${Math.round(ms)}ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1).replace(/\.0$/, "")}s`;
  const minutes = Math.floor(ms / 60_000);
  const seconds = Math.floor((ms % 60_000) / 1000);
  return seconds ? `${minutes}m ${seconds}s` : `${minutes}m`;
}

function truncateToWidth(value: string, maxWidth: number): string {
  const width = Math.max(1, Math.floor(maxWidth));
  if (stringWidth(value) <= width) return value;
  const suffix = width > 1 ? "…" : "";
  const target = width - stringWidth(suffix);
  let result = "";
  for (const character of Array.from(value)) {
    if (stringWidth(result + character) > target) break;
    result += character;
  }
  return result + suffix;
}

export function getAgentTeamRuns(entries: TimelineEntry[]): AgentTeamRun[] {
  return entries
    .filter((entry) => entry.kind === "team" && entry.teamRun)
    .map((entry) => entry.teamRun!);
}

export function clampAgentTeamCursor(cursor: number, length: number): number {
  if (length <= 0) return 0;
  return Math.min(length - 1, Math.max(0, Math.trunc(cursor)));
}

export function buildAgentTeamResultPreview(value = "", maxLength = 320): string {
  const paragraph = value.trim().split(/\n\s*\n/).find((item) => item.trim())?.replace(/\s+/g, " ").trim() || "";
  if (paragraph.length <= maxLength) return paragraph;
  if (maxLength <= 0) return "";
  return `${paragraph.slice(0, Math.max(0, maxLength - 1)).trimEnd()}…`;
}

export function formatAgentTeamRows(run: AgentTeamRun, maxWidth: number, now = Date.now()): AgentTeamDisplayLine[] {
  const width = Math.max(10, maxWidth);
  const completed = run.members.filter((member) => member.status !== "queued" && member.status !== "running").length;
  const status = getAgentTeamStatusLabel(run.status);
  const duration = formatDuration(durationMs(run.startedAt, run.finishedAt, now));
  const compactSummary = `${statusSymbol(run.status)} ${status} · ${completed}/${run.members.length}`;
  const fullSummary = `${statusSymbol(run.status)} Agent Team · ${status} · ${completed}/${run.members.length} · ${duration}`;
  const summary = stringWidth(fullSummary) <= width ? fullSummary : compactSummary;
  const orderedMembers = run.members
    .map((member, index) => ({ member, index }))
    .sort((left, right) => memberPriority(left.member.status) - memberPriority(right.member.status) || left.index - right.index)
    .map(({ member }) => member);
  const prefix = "  ";
  const shown: string[] = [];

  for (const member of orderedMembers) {
    const candidate = `${statusSymbol(member.status)} ${member.title || "Team member"}`;
    const next = [...shown, candidate];
    const remaining = orderedMembers.length - next.length;
    const preview = `${prefix}${next.join(" · ")}${remaining > 0 ? ` · +${remaining}` : ""}`;
    if (stringWidth(preview) > width) break;
    shown.push(candidate);
  }

  let memberSummary: string;
  if (orderedMembers.length === 0) {
    memberSummary = `${prefix}No members yet`;
  } else if (shown.length === 0) {
    const remaining = orderedMembers.length - 1;
    const suffix = remaining > 0 ? ` · +${remaining}` : "";
    const available = Math.max(1, width - stringWidth(prefix + suffix));
    const first = `${statusSymbol(orderedMembers[0]!.status)} ${orderedMembers[0]!.title || "Team member"}`;
    memberSummary = `${prefix}${truncateToWidth(first, available)}${suffix}`;
  } else {
    const remaining = orderedMembers.length - shown.length;
    memberSummary = `${prefix}${shown.join(" · ")}${remaining > 0 ? ` · +${remaining}` : ""}`;
  }

  return [{
    text: truncateToWidth(summary, width),
    color: statusColor(run.status),
    active: run.status === "running",
  }, {
    text: truncateToWidth(memberSummary, width),
    color: orderedMembers[0] ? statusColor(orderedMembers[0].status) : "gray",
    active: orderedMembers.some((member) => member.status === "running"),
  }];
}

function formatDetailField(
  label: string,
  value: string,
  maxWidth: number,
  color: AgentTeamDisplayLine["color"],
): AgentTeamDisplayLine[] {
  const prefix = `${label}: `;
  const continuation = " ".repeat(prefix.length);
  const available = Math.max(1, maxWidth - stringWidth(prefix));
  return wrapText(value, available).split("\n").map((line, index) => ({
    text: truncateToWidth(`${index === 0 ? prefix : continuation}${line}`, maxWidth),
    color,
  }));
}

export function formatAgentTeamMemberDetail(
  member: AgentTeamMemberRun,
  childTools: TimelineEntry[],
  maxWidth: number,
  now = Date.now(),
): AgentTeamDisplayLine[] {
  const width = Math.max(10, maxWidth);
  const duration = formatDuration(durationMs(member.startedAt || member.createdAt, member.finishedAt, now));
  const result = buildAgentTeamResultPreview(member.error || member.answer || "") || "No result yet";
  const toolCount = childTools.filter((entry) => entry.kind === "tool").length;

  return [
    ...formatDetailField("Status", `${memberStatusLabel(member.status)} · ${duration}`, width, statusColor(member.status)),
    ...formatDetailField("Task", member.task || "No task description", width, "gray"),
    ...formatDetailField(member.error ? "Error" : "Result", result, width, member.error ? "red" : "gray"),
    ...formatDetailField("Tools", `${toolCount} ${toolCount === 1 ? "tool" : "tools"}`, width, "cyan"),
  ];
}
