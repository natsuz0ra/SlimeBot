/**
 * CommandHints — shows available commands when input starts with /.
 */

import React from "react";
import { Box, Text } from "ink";
import { matchCommandHints } from "../utils/commands.js";

interface CommandHintsProps {
  input: string;
  selectedIndex: number;
}

export function CommandHints({ input, selectedIndex }: CommandHintsProps): React.ReactElement | null {
  if (!input.trimStart().startsWith("/")) return null;

  const hints = matchCommandHints(input);
  if (hints.length === 0) return null;

  return (
    <Box flexDirection="column">
      {hints.map((h, index) => {
        const selected = index === selectedIndex;
        return (
          <Text key={h.command}>
            <Text color={selected ? "cyan" : "gray"}>{selected ? "❯ " : "  "}</Text>
            <Text color="cyan">{h.command}</Text>
            <Text color="gray"> - {h.description}</Text>
          </Text>
        );
      })}
    </Box>
  );
}
