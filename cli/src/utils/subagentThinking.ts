import type { SubagentThinking, TimelineEntry } from "../types.js";

export function getSubagentThinkings(entry: TimelineEntry): SubagentThinking[] {
  return entry.subagentThinkings ?? [];
}

export function withSubagentThinkings(entry: TimelineEntry, segments: SubagentThinking[]): TimelineEntry {
  return { ...entry, subagentThinkings: segments };
}
