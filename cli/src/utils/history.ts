import { hasContentMarkers, parseContentMarkers } from "./contentMarkers.js";
import { splitNarrationAndPlan } from "./planUtils.js";
import type {
  Message,
  ThinkingHistoryItem,
  TimelineEntry,
  ToolCallHistoryItem,
  ToolCallStatus,
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

function normalizeHistoryToolCall(tc: ToolCallHistoryItem, interrupted: boolean): ToolCallHistoryItem {
  if (!interrupted || (tc.status !== "pending" && tc.status !== "executing")) {
    return tc;
  }
  return {
    ...tc,
    status: "error",
    error: tc.error || "Execution cancelled.",
  };
}

function normalizeHistoryThinking(thinking: ThinkingHistoryItem, interrupted: boolean): ThinkingHistoryItem {
  if (!interrupted || thinking.status !== "streaming") {
    return thinking;
  }
  return {
    ...thinking,
    status: "completed",
  };
}

function timelineToolEntry(tc: ToolCallHistoryItem, subagentThinking?: ThinkingHistoryItem): TimelineEntry {
  return {
    kind: "tool",
    content: tc.output || tc.error || "",
    toolCallId: tc.toolCallId,
    toolName: tc.toolName,
    command: tc.command,
    params: tc.params,
    status: (tc.status || "completed") as ToolCallStatus,
    output: tc.output,
    error: tc.error,
    ...(tc.parentToolCallId ? { parentToolCallId: tc.parentToolCallId } : {}),
    ...(tc.subagentRunId ? { subagentRunId: tc.subagentRunId } : {}),
    ...subagentHistoryFields(tc),
    ...(subagentThinking ? {
      subagentThinking: {
        content: subagentThinking.content || "",
        thinkingDone: subagentThinking.status !== "streaming",
        thinkingDurationMs: subagentThinking.durationMs,
        thinkingStartedAt: subagentThinking.startedAt ? new Date(subagentThinking.startedAt).getTime() : undefined,
      },
    } : {}),
  };
}

function timelineThinkingEntry(thinking: ThinkingHistoryItem): TimelineEntry {
  return {
    kind: "thinking",
    content: thinking.content || "",
    thinkingDone: thinking.status !== "streaming",
    thinkingDurationMs: thinking.durationMs,
  };
}

export function mapHistoryMessages(
  messages: Message[],
  toolCallsByMsgId: Record<string, ToolCallHistoryItem[]>,
  thinkingByMsgId: Record<string, ThinkingHistoryItem[]> = {},
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

    const interrupted = !!msg.isInterrupted;
    const toolCalls = [...(toolCallsByMsgId[msg.id] || [])].map((tc) => normalizeHistoryToolCall(tc, interrupted)).sort((a, b) => {
      return new Date(a.startedAt).getTime() - new Date(b.startedAt).getTime();
    });
    const thinkingRecords = [...(thinkingByMsgId[msg.id] || [])].map((thinking) => normalizeHistoryThinking(thinking, interrupted)).sort((a, b) => {
      return new Date(a.startedAt || 0).getTime() - new Date(b.startedAt || 0).getTime();
    });

    if (hasContentMarkers(msg.content)) {
      const toolCallMap = new Map(toolCalls.map(tc => [tc.toolCallId, tc]));
      const thinkingMap = new Map(thinkingRecords
        .filter(item => !item.parentToolCallId && !item.subagentRunId)
        .map(item => [item.thinkingId, item]));
      const subagentThinkingByParent = new Map(thinkingRecords
        .filter(item => item.parentToolCallId || item.subagentRunId)
        .map(item => [item.parentToolCallId || "", item]));
      const segments = parseContentMarkers(msg.content);
      const markerIds = new Set<string>();
      const thinkingMarkerIds = new Set<string>();
      let inPlan = false;
      const planParts: string[] = [];
      const flushPlan = () => {
        if (planParts.length > 0) {
          entries.push({ kind: "plan", content: planParts.join("") });
          planParts.length = 0;
        }
      };
      const pushThinking = (thinkingId: string) => {
        thinkingMarkerIds.add(thinkingId);
        const thinking = thinkingMap.get(thinkingId);
        if (thinking) {
          entries.push(timelineThinkingEntry(thinking));
        }
      };
      const pushToolCall = (toolCallId: string) => {
        markerIds.add(toolCallId);
        const tc = toolCallMap.get(toolCallId);
        if (tc) {
          entries.push(timelineToolEntry(tc, subagentThinkingByParent.get(tc.toolCallId)));
        }
      };
      for (const seg of segments) {
        if (seg.type === "plan_start") {
          inPlan = true;
          continue;
        }
        if (seg.type === "plan_end") {
          flushPlan();
          inPlan = false;
          continue;
        }
        if (inPlan) {
          if (seg.type === "text") planParts.push(seg.content);
          if (seg.type === "thinking_marker" && seg.thinkingId) {
            flushPlan();
            pushThinking(seg.thinkingId);
          }
          if (seg.type === "tool_call_marker" && seg.toolCallId) {
            flushPlan();
            pushToolCall(seg.toolCallId);
          }
          continue;
        }
        if (seg.type === "text") {
          entries.push({ kind: "assistant", content: seg.content });
        } else if (seg.type === "thinking_marker" && seg.thinkingId) {
          pushThinking(seg.thinkingId);
        } else if (seg.type === "tool_call_marker" && seg.toolCallId) {
          pushToolCall(seg.toolCallId);
        }
      }
      if (inPlan) flushPlan();
      for (const thinking of thinkingRecords) {
        if (thinking.parentToolCallId || thinking.subagentRunId) continue;
        if (!thinkingMarkerIds.has(thinking.thinkingId)) {
          entries.push(timelineThinkingEntry(thinking));
        }
      }
      for (const tc of toolCalls) {
        if (!markerIds.has(tc.toolCallId)) {
          entries.push(timelineToolEntry(tc, subagentThinkingByParent.get(tc.toolCallId)));
        }
      }
      continue;
    }

    for (const thinking of thinkingRecords) {
      if (thinking.parentToolCallId || thinking.subagentRunId) continue;
      entries.push(timelineThinkingEntry(thinking));
    }
    for (const tc of toolCalls) {
      const subagentThinking = thinkingRecords.find((item) => item.parentToolCallId === tc.toolCallId);
      entries.push(timelineToolEntry(tc, subagentThinking));
    }
    const { narration, planBody } = splitNarrationAndPlan(msg.content);
    if (planBody && planBody !== msg.content) {
      if (narration) {
        entries.push({ kind: "assistant", content: narration });
      }
      entries.push({ kind: "plan", content: planBody });
    } else {
      entries.push({ kind: "assistant", content: msg.content });
    }
  }

  return entries;
}
