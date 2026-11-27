import React, { useCallback, useEffect, useMemo, useState } from "react";
import { Box, Text, useInput } from "ink";
import type { UsageGroup, UsageSummary, UsageTimeRange } from "../types.js";
import { CLI_THEME } from "../utils/theme.js";
import {
  formatCompactNumber,
  formatUsageRowLabel,
  resolveUsageTimeRange,
  summarizeUsageRows,
} from "../utils/usage.js";

const GROUPS: UsageGroup[] = ["session", "model", "project"];
const RANGES: Array<{ key: UsageTimeRange; label: string }> = [
  { key: "all", label: "1:All" },
  { key: "today", label: "2:Today" },
  { key: "7d", label: "3:7d" },
  { key: "30d", label: "4:30d" },
];

export interface UsageViewProps {
  columns: number;
  height: number;
  outputBudgetTokens?: number;
  loadSummary: (groupBy: UsageGroup, options?: { since?: string; until?: string }) => Promise<UsageSummary[]>;
  onClose: () => void;
}

export function UsageView({
  columns,
  height,
  outputBudgetTokens,
  loadSummary,
  onClose,
}: UsageViewProps): React.ReactElement {
  const [groupIndex, setGroupIndex] = useState(0);
  const [range, setRange] = useState<UsageTimeRange>("7d");
  const [rows, setRows] = useState<UsageSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [scroll, setScroll] = useState(0);

  const group = GROUPS[groupIndex] || "session";

  const refresh = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const bounds = resolveUsageTimeRange(range);
      const items = await loadSummary(group, bounds);
      setRows(items);
      setScroll(0);
    } catch (err) {
      setError((err as Error).message || "Failed to load token usage.");
    } finally {
      setLoading(false);
    }
  }, [group, loadSummary, range]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const totals = useMemo(() => summarizeUsageRows(rows), [rows]);
  const maxVisibleRows = Math.max(3, height - 9);

  useInput((input, key) => {
    if (key.escape) {
      onClose();
      return;
    }
    if (key.tab || key.rightArrow) {
      setGroupIndex((prev) => (prev + 1) % GROUPS.length);
      return;
    }
    if (key.leftArrow) {
      setGroupIndex((prev) => (prev - 1 + GROUPS.length) % GROUPS.length);
      return;
    }
    if (input === "1") { setRange("all"); return; }
    if (input === "2") { setRange("today"); return; }
    if (input === "3") { setRange("7d"); return; }
    if (input === "4") { setRange("30d"); return; }
    if (input === "r" || input === "R") {
      void refresh();
      return;
    }
    if (key.upArrow) setScroll((prev) => Math.max(0, prev - 1));
    else if (key.downArrow) setScroll((prev) => Math.min(Math.max(0, rows.length - maxVisibleRows), prev + 1));
  });

  const labelWidth = Math.max(16, Math.min(44, columns - 38));
  const fit = (text: string, width: number) => (text.length <= width ? text.padEnd(width, " ") : `${text.slice(0, Math.max(1, width - 1))}…`);
  const col = (text: string, width = 8) => text.padStart(width, " ");

  return (
    <Box flexDirection="column" width={columns}>
      <Box justifyContent="space-between">
        <Text bold color={CLI_THEME.accent}>Token Usage Summary</Text>
        <Text color={CLI_THEME.secondary}>
          {outputBudgetTokens && outputBudgetTokens > 0
            ? `Session budget: ${formatCompactNumber(outputBudgetTokens)} out`
            : "Session budget: unlimited"}
        </Text>
      </Box>

      <Box marginTop={1} justifyContent="space-between">
        <Text>
          {GROUPS.map((g, idx) => (
            <React.Fragment key={g}>
              {idx > 0 ? <Text color={CLI_THEME.hint}>  </Text> : null}
              <Text bold={idx === groupIndex} color={idx === groupIndex ? CLI_THEME.accent : CLI_THEME.secondary}>
                {idx === groupIndex ? `[${g}]` : g}
              </Text>
            </React.Fragment>
          ))}
        </Text>
        <Text>
          {RANGES.map((item, idx) => (
            <React.Fragment key={item.key}>
              {idx > 0 ? <Text color={CLI_THEME.hint}> </Text> : null}
              <Text bold={item.key === range} color={item.key === range ? CLI_THEME.accent : CLI_THEME.secondary}>
                {item.label}
              </Text>
            </React.Fragment>
          ))}
        </Text>
      </Box>

      <Text color={CLI_THEME.secondary} wrap="truncate">
        Total: {formatCompactNumber(totals.requests)} req · {formatCompactNumber(totals.input)} in · {formatCompactNumber(totals.output)} out · {formatCompactNumber(totals.cacheRead)} cache
      </Text>
      <Text> </Text>

      {error ? (
        <Text color={CLI_THEME.error}>{error}</Text>
      ) : loading ? (
        <Text color={CLI_THEME.secondary}>Loading usage...</Text>
      ) : rows.length === 0 ? (
        <Text color={CLI_THEME.secondary}>No usage recorded in this time range.</Text>
      ) : (
        <Box flexDirection="column">
          <Text color={CLI_THEME.secondary} wrap="truncate">
            {fit(group.toUpperCase(), labelWidth)} {col("REQ", 5)} {col("INPUT")} {col("OUTPUT")} {col("CACHE")}
          </Text>
          {rows.slice(scroll, scroll + maxVisibleRows).map((row) => (
            <Text key={row.groupKey} wrap="truncate">
              <Text>{fit(formatUsageRowLabel(group, row), labelWidth)}</Text>
              <Text color={CLI_THEME.secondary}> {col(formatCompactNumber(row.requestCount), 5)}</Text>
              <Text> {col(formatCompactNumber(row.inputTokens))}</Text>
              <Text color={CLI_THEME.accent}> {col(formatCompactNumber(row.outputTokens))}</Text>
              <Text color={CLI_THEME.secondary}> {col(formatCompactNumber(row.cacheReadInputTokens))}</Text>
            </Text>
          ))}
          {rows.length > maxVisibleRows ? (
            <Text color={CLI_THEME.hint}>
              Showing {scroll + 1}-{Math.min(rows.length, scroll + maxVisibleRows)} of {rows.length}
            </Text>
          ) : null}
        </Box>
      )}

      <Text> </Text>
      <Text color={CLI_THEME.hint} wrap="truncate">
        Tab/←→ group · 1-4 range · ↑↓ scroll · R refresh · /usage budget · Esc close
      </Text>
    </Box>
  );
}
