import type { SubagentThinking, TimelineEntry } from "../types.js";

export function getSubagentThinkings(entry: TimelineEntry): SubagentThinking[] {
  return entry.subagentThinkings ?? (entry.subagentThinking ? [entry.subagentThinking] : []);
}

export function withSubagentThinkings(entry: TimelineEntry, segments: SubagentThinking[]): TimelineEntry {
  return { ...entry, subagentThinkings: segments, subagentThinking: segments.at(-1) };
}
