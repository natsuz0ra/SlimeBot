/**
 * MenuView — shared menu for session / model / skills / mcp / help.
 */

import React from "react";
import { Box, Text, useStdout } from "ink";
import type { MenuItem, MenuKind } from "../types.js";
import { wrapText } from "../utils/format.js";

interface MenuViewProps {
  title: string;
  items: MenuItem[];
  cursor: number;
  hint: string;
  kind: MenuKind;
  onSelect: (item: MenuItem, index: number) => void;
  onBack: () => void;
  onDelete?: (item: MenuItem, index: number) => void;
  onAdd?: () => void;
  onEdit?: (item: MenuItem, index: number) => void;
  onToggle?: (item: MenuItem, index: number) => void;
}

const MAX_MENU_TITLE_LENGTH = 25;
const MAX_MENU_DESC_LENGTH = 80;

export function truncateMenuTitle(title: string, maxLen = MAX_MENU_TITLE_LENGTH): string {
  const normalized = (title ?? "").trim();
  if (normalized.length <= maxLen) return normalized;
  if (maxLen <= 1) return "…";
  return `${normalized.slice(0, maxLen - 1)}…`;
}

export function truncateMenuDescription(desc: string, maxLen = MAX_MENU_DESC_LENGTH): string {
  const normalized = (desc ?? "").replace(/\s+/g, " ").trim();
  if (!normalized) return "(No description)";
  if (normalized.length <= maxLen) return normalized;
  if (maxLen <= 1) return "…";
  return `${normalized.slice(0, maxLen - 1)}…`;
}

export function formatMenuDescriptionLines(desc: string, terminalWidth: number): string[] {
  const text = truncateMenuDescription(desc);
  const lineWidth = Math.max(10, Math.min(MAX_MENU_DESC_LENGTH, terminalWidth - 2));
  return wrapText(text, lineWidth).split("\n");
}

export function MenuView({
  title,
  items,
  cursor,
  hint,
  kind: _kind,
  onSelect: _onSelect,
  onBack: _onBack,
  onDelete: _onDelete,
  onAdd: _onAdd,
  onEdit: _onEdit,
  onToggle: _onToggle,
}: MenuViewProps): React.ReactElement {
  const { stdout } = useStdout();
  const terminalWidth = Math.max(20, stdout?.columns || 80);

  return (
    <Box flexDirection="column">
      <Text bold color="white">
        {title}
      </Text>
      {items.length === 0 ? (
        <Text color="gray">(empty)</Text>
      ) : (
        items.map((item, i) => (
          <Box key={i} flexDirection="column">
            <Text>
              <Text color={i === cursor ? "white" : "gray"}>
                {i === cursor ? "\u276F" : " "}
              </Text>
              <Text>{" "}</Text>
              <Text bold={i === cursor} color={i === cursor ? "white" : "white"}>
                {truncateMenuTitle(item.title)}
              </Text>
            </Text>
            {formatMenuDescriptionLines(item.desc, terminalWidth).map((line, index) => (
              <Text key={`${i}-desc-${index}`} color="gray">
                {`  ${line}`}
              </Text>
            ))}
          </Box>
        ))
      )}
      {hint && (
        <Box flexDirection="column">
          <Text> </Text>
          <Text color="gray">{hint}</Text>
        </Box>
      )}
    </Box>
  );
}
