/**
 * MCPTemplatePicker — MCP transport template picker.
 * Shows three templates; keyboard events are dispatched by App.
 */

import React from "react";
import { Box, Text } from "ink";
import { MCP_TEMPLATES } from "../types.js";
import type { MCPTemplate } from "../types.js";

interface MCPTemplatePickerProps {
  cursor: number;
}

export function MCPTemplatePicker({ cursor }: MCPTemplatePickerProps): React.ReactElement {
  return (
    <Box flexDirection="column">
      <Text bold color="86">
        Select MCP Transport Template
      </Text>
      <Text> </Text>
      {MCP_TEMPLATES.map((tpl: MCPTemplate, i: number) => (
        <Box key={tpl.kind} flexDirection="column">
          <Text>
            <Text color={i === cursor ? "white" : "gray"}>
              {i === cursor ? "\u276F" : " "}
            </Text>
            <Text>{" "}</Text>
            <Text bold={i === cursor} color={i === cursor ? "white" : "white"}>
              {tpl.label}
            </Text>
          </Text>
          <Text color="gray">{`  ${tpl.description}`}</Text>
        </Box>
      ))}
      <Text> </Text>
      <Text color="gray">
        Arrow keys to navigate | Enter to select | Esc to cancel
      </Text>
    </Box>
  );
}
