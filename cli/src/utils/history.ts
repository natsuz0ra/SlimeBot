import { projectChatHistory, type ProjectedThinking, type ProjectedTool } from "../../../shared/chat-protocol/timeline.js";
import type {
  AgentTeamMemberRun,
  AgentTeamRun,
  Message,
  ThinkingHistoryItem,
  TimelineEntry,
  ToolCallHistoryItem,
} from "../types.js";

function subagentHistoryFields(tc: ToolCallHistoryItem): Partial<TimelineEntry> {
  if ((tc.toolName || "").trim().toLowerCase() !== "run_subagent") return {};
  const title = String(tc.subagentTitle ?? tc.params?.title ?? "").trim();
  const task = String(tc.subagentTask ?? tc.params?.task ?? "").trim();
  return {
    ...(title ? { subagentTitle: title } : {}),
    ...(task ? { subagentTask: task } : {}),
  };
}

function timelineToolEntry({ record: tc, subagentThinkings }: ProjectedTool<ToolCallHistoryItem>): TimelineEntry {
  const thinkings = subagentThinkings.map(thinkingFields);
  const subagentRunId = tc.subagentRunId ?? subagentThinkings.find(thinking => thinking.subagentRunId)?.subagentRunId;
  return {
    kind: "tool",
    content: tc.output || tc.error || "",
    toolCallId: tc.toolCallId,
    toolName: tc.toolName,
    command: tc.command,
    params: tc.params,
    status: tc.status,
    output: tc.output,
    error: tc.error,
    metadata: tc.metadata,
    ...(tc.parentToolCallId ? { parentToolCallId: tc.parentToolCallId } : {}),
    ...(subagentRunId ? { subagentRunId } : {}),
    ...(tc.teamRunId ? { teamRunId: tc.teamRunId } : {}),
    ...(tc.memberRunId ? { memberRunId: tc.memberRunId } : {}),
    ...subagentHistoryFields(tc),
    ...(thinkings.length > 0 ? { subagentThinkings: thinkings } : {}),
  };
}

function thinkingFields(thinking: ProjectedThinking) {
  return {
    content: thinking.content,
    thinkingDone: thinking.done,
    thinkingDurationMs: thinking.durationMs,
    thinkingStartedAt: thinking.startedAt,
  };
}

export function mapHistoryMessages(
  messages: Message[],
  toolCallsByMsgId: Record<string, ToolCallHistoryItem[]>,
  thinkingByMsgId: Record<string, ThinkingHistoryItem[]> = {},
  teamRuns: Array<Omit<AgentTeamRun, "members"> & { members?: AgentTeamMemberRun[] }> = [],
  teamMemberRuns: AgentTeamMemberRun[] = [],
): TimelineEntry[] {
  const ordered = [...messages].sort((a, b) => (a.seq || 0) - (b.seq || 0));
  const entries: TimelineEntry[] = [];

  for (const msg of ordered) {
    if (msg.isStopPlaceholder) continue;

    if (msg.role === "user") {
      entries.push({ kind: "user", content: msg.content });
      continue;
    }
    if (msg.role !== "assistant") {
      entries.push({ kind: "system", content: msg.content });
      continue;
    }

    const messageTeams = teamRuns.filter((run) => run.assistantMessageId === msg.id);
    for (const run of messageTeams) {
      const members = teamMemberRuns
        .filter((member) => member.teamRunId === run.id)
        .sort((left, right) => {
          const leftAt = Date.parse(left.createdAt || left.startedAt || "");
          const rightAt = Date.parse(right.createdAt || right.startedAt || "");
          return (Number.isFinite(leftAt) ? leftAt : 0) - (Number.isFinite(rightAt) ? rightAt : 0)
            || left.id.localeCompare(right.id);
        });
      entries.push({
        kind: "team",
        content: "",
        teamRun: { ...run, members: run.members?.length ? run.members : members },
      });
    }

    const projection = projectChatHistory(
      msg.content, toolCallsByMsgId[msg.id] ?? [], thinkingByMsgId[msg.id] ?? [], !!msg.isInterrupted,
    );
    for (const entry of projection.timeline) {
      switch (entry.kind) {
        case "tool":
          entries.push(timelineToolEntry(entry.tool));
          break;
        case "thinking":
          entries.push({ kind: "thinking", ...thinkingFields(entry.thinking) });
          break;
        case "text":
          entries.push({ kind: "assistant", content: entry.content });
          break;
        case "plan":
          entries.push({ kind: "plan", content: entry.content });
          break;
      }
    }
    // The CLI view groups these entries under their parent during rendering.
    const toolIds = new Set(projection.toolCalls.map(({ record }) => record.toolCallId));
    for (const tool of projection.toolCalls) {
      if (tool.record.parentToolCallId && toolIds.has(tool.record.parentToolCallId)) {
        entries.push(timelineToolEntry(tool));
      }
    }
  }

  return entries;
}
