/**
 * MCPTemplatePicker — MCP transport template picker.
 * Shows three templates; keyboard events are dispatched by App.
 */

import React from "react";
import { Box, Text } from "ink";
import { MCP_TEMPLATES } from "../types.js";
import type { MCPTemplate } from "../types.js";
import { CLI_ACCENT_COLOR } from "../utils/terminal.js";
import { CLI_THEME } from "../utils/theme.js";

export const MCP_TEMPLATE_TITLE_GAP_LINES = 1;
export const MCP_TEMPLATE_HINT = "↑↓ navigate · Enter select · Esc cancel";
export const MCP_TEMPLATE_COLORS = {
  title: CLI_ACCENT_COLOR,
  cursor: CLI_ACCENT_COLOR,
  idleCursor: CLI_THEME.hint,
  selectedTitle: "#f8fafc",
  idleTitle: "#cbd5e1",
  description: CLI_THEME.secondary,
  hint: CLI_THEME.hint,
} as const;

interface MCPTemplatePickerProps {
  cursor: number;
}

export function MCPTemplatePicker({ cursor }: MCPTemplatePickerProps): React.ReactElement {
  return (
    <Box flexDirection="column">
      <Text bold color={MCP_TEMPLATE_COLORS.title}>
        Select MCP Transport
      </Text>
      {MCP_TEMPLATE_TITLE_GAP_LINES > 0 && <Text> </Text>}
      {MCP_TEMPLATES.map((tpl: MCPTemplate, i: number) => (
        <Box key={tpl.kind} flexDirection="column">
          <Text>
            <Text color={i === cursor ? MCP_TEMPLATE_COLORS.cursor : MCP_TEMPLATE_COLORS.idleCursor}>
              {i === cursor ? "\u276F" : " "}
            </Text>
            <Text>{" "}</Text>
            <Text
              bold={i === cursor}
              color={i === cursor ? MCP_TEMPLATE_COLORS.selectedTitle : MCP_TEMPLATE_COLORS.idleTitle}
            >
              {tpl.label}
            </Text>
          </Text>
          <Text color={MCP_TEMPLATE_COLORS.description}>{`  ${tpl.description}`}</Text>
        </Box>
      ))}
      <Text> </Text>
      <Text color={MCP_TEMPLATE_COLORS.hint}>{MCP_TEMPLATE_HINT}</Text>
    </Box>
  );
}
