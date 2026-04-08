/**
 * MCPTemplatePicker — MCP 传输模板选择组件。
 * 展示三种模板供用户选择，键盘事件由 App 统一分发。
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
            <Text color={i === cursor ? "#22d3ee" : "gray"}>
              {i === cursor ? "\u276F" : " "}
            </Text>
            <Text>{" "}</Text>
            <Text bold={i === cursor} color={i === cursor ? "#22d3ee" : "white"}>
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
