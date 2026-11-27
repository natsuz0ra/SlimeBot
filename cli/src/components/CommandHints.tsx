/**
 * CommandHints — aligned slash-command autocomplete list when input starts with /.
 */

import React from "react";
import { Box, Text } from "ink";
import { getVisibleCommandHints, matchCommandHints } from "../utils/commands.js";
import { CLI_THEME } from "../utils/theme.js";

interface CommandHintsProps {
  input: string;
  selectedIndex: number;
}

export function CommandHints({ input, selectedIndex }: CommandHintsProps): React.ReactElement | null {
  if (!input.trimStart().startsWith("/")) return null;

  const hints = matchCommandHints(input);
  if (hints.length === 0) return null;
  const visible = getVisibleCommandHints(hints, selectedIndex);
  const cmdWidth = Math.max(12, ...visible.hints.map((h) => h.command.length)) + 2;

  return (
    <Box flexDirection="column">
      {visible.hints.map((h, index) => {
        const absoluteIndex = visible.startIndex + index;
        const selected = absoluteIndex === selectedIndex;
        return (
          <Text key={h.command} wrap="truncate">
            <Text color={selected ? CLI_THEME.accent : CLI_THEME.muted}>{selected ? "❯ " : "  "}</Text>
            <Text color={CLI_THEME.accent} bold={selected}>{h.command.padEnd(cmdWidth, " ")}</Text>
            <Text color={CLI_THEME.secondary}>{h.description}</Text>
          </Text>
        );
      })}
      {(visible.aboveCount > 0 || visible.belowCount > 0) && (
        <Text color={CLI_THEME.hint}>
          {"  "}
          {visible.startIndex + 1}–{visible.startIndex + visible.hints.length} / {hints.length}
          {visible.aboveCount > 0 ? `  ↑${visible.aboveCount}` : ""}
          {visible.belowCount > 0 ? `  ↓${visible.belowCount}` : ""}
        </Text>
      )}
    </Box>
  );
}
