import React from "react";
import { Box, Text } from "ink";
import type { AgentTeamRun, TimelineEntry } from "../types.js";
import {
  clampAgentTeamCursor,
  formatAgentTeamMemberDetail,
  formatAgentTeamRows,
} from "../utils/agentTeam.js";
import { CLI_ACCENT_COLOR } from "../utils/terminal.js";
import { CLI_THEME } from "../utils/theme.js";

interface TeamViewProps {
  runs: AgentTeamRun[];
  entries: TimelineEntry[];
  teamCursor: number;
  memberCursor: number;
  columns: number;
  now?: number;
}

const MEMBER_COLORS = {
  selected: "#f8fafc",
  inactive: CLI_THEME.secondary,
  success: "#34d399",
  warning: "#fbbf24",
  danger: "#fb7185",
  hint: CLI_THEME.hint,
} as const;

function memberSymbol(status: string): string {
  if (status === "running") return "●";
  if (status === "queued") return "○";
  if (status === "succeeded") return "✓";
  if (status === "failed") return "✗";
  return "−";
}

function memberColor(status: string): string {
  if (status === "succeeded") return MEMBER_COLORS.success;
  if (status === "running" || status === "queued") return MEMBER_COLORS.warning;
  if (status === "failed") return MEMBER_COLORS.danger;
  return MEMBER_COLORS.inactive;
}

export default function TeamView({
  runs,
  entries,
  teamCursor,
  memberCursor,
  columns,
  now = Date.now(),
}: TeamViewProps): React.ReactElement {
  const width = Math.max(20, Math.min(columns || 80, 96));

  if (runs.length === 0) {
    return (
      <Box flexDirection="column" paddingX={1} width={width}>
        <Text bold color={CLI_ACCENT_COLOR}>Agent Team</Text>
        <Text color={MEMBER_COLORS.inactive}>No Agent Team runs in this session.</Text>
      </Box>
    );
  }

  const safeTeamCursor = clampAgentTeamCursor(teamCursor, runs.length);
  const run = runs[safeTeamCursor]!;
  const safeMemberCursor = clampAgentTeamCursor(memberCursor, run.members.length);
  const selectedMember = run.members[safeMemberCursor];
  const teamSummary = formatAgentTeamRows(run, Math.max(10, width - 2), now)[0];
  const childTools = selectedMember
    ? entries.filter((entry) => entry.kind === "tool" && entry.parentToolCallId === selectedMember.toolCallId)
    : [];
  const detailLines = selectedMember
    ? formatAgentTeamMemberDetail(selectedMember, childTools, Math.max(10, width - 4), now)
    : [];

  return (
    <Box flexDirection="column" paddingX={1} width={width}>
      <Text bold color={CLI_ACCENT_COLOR}>Agent Team {safeTeamCursor + 1}/{runs.length}</Text>
      {teamSummary && (
        <Text color={teamSummary.color}>{teamSummary.text}</Text>
      )}

      <Box flexDirection="column" marginTop={1}>
        {run.members.length === 0 ? (
          <Text color={MEMBER_COLORS.inactive}>No members yet.</Text>
        ) : run.members.map((member, index) => {
          const selected = index === safeMemberCursor;
          return (
            <Text key={member.id} bold={selected} color={selected ? MEMBER_COLORS.selected : MEMBER_COLORS.inactive}>
              <Text color={selected ? CLI_ACCENT_COLOR : MEMBER_COLORS.hint}>{selected ? "❯ " : "  "}</Text>
              <Text color={memberColor(member.status)}>{memberSymbol(member.status)} </Text>
              {member.title || "Team member"}
            </Text>
          );
        })}
      </Box>

      {selectedMember && (
        <Box flexDirection="column" marginTop={1} paddingLeft={2}>
          {detailLines.map((line, index) => (
            <Text key={`${index}-${line.text}`} color={line.color}>{line.text}</Text>
          ))}
        </Box>
      )}
    </Box>
  );
}
