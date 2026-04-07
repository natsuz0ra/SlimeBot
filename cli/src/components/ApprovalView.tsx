/**
 * ApprovalView — 工具审批对话框。
 * 显示工具名、命令和参数，让用户确认或拒绝。
 */

import React from "react";
import { Box, Text } from "ink";

interface ApprovalViewProps {
  toolName: string;
  command: string;
  params: Record<string, string>;
}

export function ApprovalView({
  toolName,
  command,
  params,
}: ApprovalViewProps): React.ReactElement {
  const paramStr = Object.keys(params).length > 0
    ? Object.entries(params)
        .map(([k, v]) => `${k}=${v}`)
        .join(", ")
    : "";

  return (
    <Box flexDirection="column">
      <Text bold color="yellow">
        Tool Approval Required
      </Text>
      <Text>
        Tool: <Text bold>{toolName}.{command}</Text>
      </Text>
      {paramStr && (
        <Text>
          Params: {paramStr}
        </Text>
      )}
      <Text color="gray">
        Press y to approve, n or Esc to reject.
      </Text>
    </Box>
  );
}
