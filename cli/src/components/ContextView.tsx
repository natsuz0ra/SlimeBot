import React from "react";
import { Box, Text, useInput } from "ink";
import type { ContextUsage } from "../types.js";
import { formatContextTokenCount } from "../utils/contextSize.js";
import { CLI_THEME } from "../utils/theme.js";
import { getOutputBudgetStatus } from "../utils/usage.js";

export interface ContextViewProps {
  usage: ContextUsage | null;
  modelName: string;
  streaming: boolean;
  compacting: boolean;
  columns: number;
  onCompact: () => void;
  onRefresh: () => void;
  onClose: () => void;
}

const LAYER_LABELS: Record<string, string> = {
  system: "System prompt",
  project: "Project instructions",
  tools: "Tool schemas",
  skill_catalog: "Skill catalog",
  memory: "Long-term memory",
  runtime: "Runtime context",
  compaction: "Compaction summary",
  recent_conversation: "Recent conversation",
  current_turn: "Current turn",
  runtime_state: "Runtime state",
};

function renderBar(percent: number, width: number): string {
  const clamped = Math.max(0, Math.min(100, Math.round(percent)));
  const len = Math.max(10, width);
  const filled = Math.round((clamped / 100) * len);
  return `${"█".repeat(filled)}${"░".repeat(len - filled)}`;
}

export function ContextView({
  usage,
  modelName,
  streaming,
  compacting,
  columns,
  onCompact,
  onRefresh,
  onClose,
}: ContextViewProps): React.ReactElement {
  useInput((input, key) => {
    if (key.escape) {
      onClose();
      return;
    }
    if ((input === "c" || input === "C") && !streaming && !compacting) {
      onCompact();
      return;
    }
    if (input === "r" || input === "R") {
      onRefresh();
    }
  });

  const budget = getOutputBudgetStatus(usage);
  const barWidth = Math.max(12, Math.min(32, columns - 28));

  return (
    <Box flexDirection="column" width={columns}>
      <Box justifyContent="space-between">
        <Text bold color={CLI_THEME.accent}>Context Window</Text>
        <Text color={CLI_THEME.secondary}>{modelName || "(no model)"}</Text>
      </Box>
      <Text> </Text>

      {!usage ? (
        <Text color={CLI_THEME.secondary}>No context usage available for this session yet.</Text>
      ) : (
        <Box flexDirection="column">
          <Text>
            <Text color={usage.usedPercent >= 90 ? CLI_THEME.error : usage.usedPercent >= 70 ? CLI_THEME.warning : CLI_THEME.success}>
              {renderBar(usage.usedPercent, barWidth)}
            </Text>
            <Text bold> {usage.usedPercent}%</Text>
            <Text color={CLI_THEME.secondary}>
              {" "}({formatContextTokenCount(usage.usedTokens)} / {formatContextTokenCount(usage.totalTokens)})
            </Text>
          </Text>

          <Text color={CLI_THEME.secondary}>
            Source: {usage.source === "provider-reported" ? "provider-reported" : "estimated"}
            {" · "}Status: {compacting || usage.state === "compacting" ? "compacting" : usage.state === "cooldown" ? "cooldown" : usage.isCompacted ? "compacted" : "full"}
            {usage.inputBudget ? ` · Input budget: ${formatContextTokenCount(usage.inputBudget)}` : ""}
            {usage.outputReserve ? ` · Output reserve: ${formatContextTokenCount(usage.outputReserve)}` : ""}
          </Text>

          {budget && (
            <Box marginTop={1} flexDirection="column">
              <Text>
                <Text bold>Session output budget: </Text>
                <Text color={budget.tone === "danger" ? CLI_THEME.error : budget.tone === "warning" ? CLI_THEME.warning : CLI_THEME.success}>
                  {budget.percent}%
                </Text>
                <Text color={CLI_THEME.secondary}>
                  {" "}({formatContextTokenCount(budget.used)} / {formatContextTokenCount(budget.budget)})
                </Text>
              </Text>
            </Box>
          )}

          {usage.layers && usage.layers.length > 0 && (
            <Box marginTop={1} flexDirection="column">
              <Text bold>Layers</Text>
              {usage.layers.map((layer) => {
                const label = (LAYER_LABELS[layer.layer] || layer.layer).padEnd(22, " ");
                const pct = usage.totalTokens > 0 ? Math.round((layer.tokens / usage.totalTokens) * 100) : 0;
                return (
                  <Text key={layer.layer} wrap="truncate">
                    <Text color={CLI_THEME.secondary}>  {label} </Text>
                    <Text>{formatContextTokenCount(layer.tokens).padStart(14, " ")}</Text>
                    <Text color={CLI_THEME.hint}> ({pct}%)</Text>
                  </Text>
                );
              })}
            </Box>
          )}

          {(usage.compactedAt || usage.compactionFailure) && (
            <Box marginTop={1} flexDirection="column">
              <Text bold>Last compaction</Text>
              {usage.compactedAt && (
                <Text color={CLI_THEME.secondary}>
                  {"  "}
                  {usage.compactionBeforeTokens !== undefined && usage.compactionAfterTokens !== undefined
                    ? `${formatContextTokenCount(usage.compactionBeforeTokens)} → ${formatContextTokenCount(usage.compactionAfterTokens)}`
                    : "Completed"}
                  {usage.compactionDurationMs !== undefined ? ` · ${usage.compactionDurationMs}ms` : ""}
                  {usage.compactionReason ? ` · ${usage.compactionReason}` : ""}
                </Text>
              )}
              {usage.compactionFailure && (
                <Text color={CLI_THEME.error}>  {usage.compactionFailure}</Text>
              )}
            </Box>
          )}
        </Box>
      )}

      <Text> </Text>
      <Text color={CLI_THEME.hint} wrap="truncate">
        {compacting
          ? "Compacting context..."
          : streaming
            ? "R refresh · Esc close (wait for turn before compacting)"
            : "C compact context · R refresh · Esc close"}
      </Text>
    </Box>
  );
}
