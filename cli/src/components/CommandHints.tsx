/**
 * CommandHints — 命令提示组件，在输入 / 时显示可用命令列表。
 */

import React from "react";
import { Box, Text } from "ink";
import { matchCommandHints } from "../utils/commands.js";

interface CommandHintsProps {
  input: string;
}

export function CommandHints({ input }: CommandHintsProps): React.ReactElement | null {
  const trimmed = input.trim();
  if (!trimmed.startsWith("/")) return null;

  const hints = matchCommandHints(trimmed);
  if (hints.length === 0) return null;

  return (
    <Box flexDirection="column">
      {hints.map((h) => (
        <Text key={h.command}>
          <Text color="cyan">{h.command}</Text>
          <Text color="gray"> - {h.description}</Text>
        </Text>
      ))}
    </Box>
  );
}
