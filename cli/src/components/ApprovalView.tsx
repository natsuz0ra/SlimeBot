/**
 * ApprovalView — tool approval dialog.
 * Shows tool name, command, and args for user approval or denial.
 */

import React from "react";
import { Box, Text } from "ink";
import { filterToolParamsForDetail, formatToolCallSummary } from "../utils/format.js";

interface ApprovalViewProps {
  toolName: string;
  command: string;
  params: Record<string, string>;
  items?: Array<{
    toolCallId: string;
    toolName: string;
    command: string;
    params: Record<string, string>;
  }>;
  cursor?: number;
}

export function ApprovalView({
  toolName,
  command,
  params,
  items,
  cursor = 0,
}: ApprovalViewProps): React.ReactElement {
  const approvalItems = items && items.length > 0
    ? items
    : [{ toolCallId: "", toolName, command, params }];
  return (
    <Box flexDirection="column">
      <Text bold color="yellow">
        Tool Approval Required{approvalItems.length > 1 ? ` (${approvalItems.length})` : ""}
      </Text>
      {approvalItems.map((item, index) => {
        const detailParams = filterToolParamsForDetail(item.toolName, item.command, item.params) || {};
        const itemParamStr = Object.keys(detailParams).length > 0
          ? Object.entries(detailParams).map(([k, v]) => `${k}=${v}`).join(", ")
          : "";
        const summary = formatToolCallSummary(item.toolName, item.command, item.params);
        const selected = index === cursor;
        return (
          <Box key={item.toolCallId || `${item.toolName}-${index}`} flexDirection="column">
            <Text>
              <Text color={selected ? "cyan" : "gray"}>{selected ? "❯ " : "  "}</Text>
              Tool: <Text bold>{item.toolName}</Text>
              {summary ? <Text color="gray">{` ${summary}`}</Text> : null}
            </Text>
            {itemParamStr && (
              <Text color="gray">
                {"  "}Params: {itemParamStr}
              </Text>
            )}
          </Box>
        );
      })}
      <Text color="gray">
        ↑/↓ select | y approve | n/Esc reject | a approve all | r reject all
      </Text>
    </Box>
  );
}
