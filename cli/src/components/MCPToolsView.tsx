/**
 * MCPToolsView — readonly list of tools advertised by one MCP config.
 */

import React from "react";
import { Box, Text, useStdout } from "ink";
import { CLI_ACCENT_COLOR } from "../utils/terminal.js";
import { CLI_THEME } from "../utils/theme.js";
import type { MCPConfig, MCPToolListResponse } from "../types.js";

export const MCP_TOOLS_COLORS = {
  title: CLI_ACCENT_COLOR,
  hint: CLI_THEME.hint,
  text: "#f8fafc",
  muted: CLI_THEME.secondary,
  success: "#22c55e",
  warning: "#fbbf24",
  error: "#fb7185",
} as const;

export function formatMCPToolParameters(tool: Pick<MCPToolListResponse["tools"][number], "parameterCount" | "requiredParameters">): string {
  const count = `${tool.parameterCount} ${tool.parameterCount === 1 ? "param" : "params"}`;
  const requiredParameters = Array.isArray(tool.requiredParameters) ? tool.requiredParameters : [];
  if (!requiredParameters.length) return count;
  return `${count} · required ${requiredParameters.join(", ")}`;
}

export function getMCPToolsStatusLine(result: Pick<MCPToolListResponse, "status" | "toolCount" | "error"> | null): string {
  if (!result) return "loading tools...";
  if (result.status === "disabled") return "disabled · tools not loaded";
  if (result.status === "error") return `tools load failed: ${result.error || "unknown error"}`;
  return `${result.toolCount} ${result.toolCount === 1 ? "tool" : "tools"} loaded`;
}

export function truncateMCPToolDescription(value: string, maxLength: number): string {
  const normalized = value.replace(/\s+/g, " ").trim();
  if (normalized.length <= maxLength) return normalized;
  if (maxLength <= 1) return "…";
  return `${normalized.slice(0, maxLength - 1)}…`;
}

interface MCPToolsViewProps {
  config: MCPConfig | null;
  result: MCPToolListResponse | null;
  loading: boolean;
  error: string;
}

export function MCPToolsView({ config, result, loading, error }: MCPToolsViewProps): React.ReactElement {
  const { stdout } = useStdout();
  const width = Math.max(40, stdout?.columns || 80);
  const descWidth = Math.max(24, Math.min(84, width - 8));
  const status = error
    ? `tools load failed: ${error}`
    : loading
      ? "loading tools..."
      : getMCPToolsStatusLine(result);
  const statusColor = error || result?.status === "error"
    ? MCP_TOOLS_COLORS.error
    : result?.status === "loaded"
      ? MCP_TOOLS_COLORS.success
      : MCP_TOOLS_COLORS.muted;

  return (
    <Box flexDirection="column">
      <Text bold color={MCP_TOOLS_COLORS.title}>
        MCP Tools{config?.name ? ` · ${config.name}` : ""}
      </Text>
      <Text color={MCP_TOOLS_COLORS.hint}>Esc return · R refresh</Text>
      <Text> </Text>
      <Text color={statusColor}>{status}</Text>
      <Text> </Text>

      {result?.status === "loaded" && result.tools.length === 0 && (
        <Text color={MCP_TOOLS_COLORS.muted}>(no tools)</Text>
      )}

      {result?.status === "loaded" && result.tools.map((tool) => (
        <Box key={tool.name} flexDirection="column" marginBottom={1}>
          <Text bold color={MCP_TOOLS_COLORS.text}>{tool.name}</Text>
          <Text color={MCP_TOOLS_COLORS.muted}>
            {truncateMCPToolDescription(tool.description || "-", descWidth)}
          </Text>
          <Text color={MCP_TOOLS_COLORS.hint}>
            schema: {formatMCPToolParameters(tool)}
          </Text>
        </Box>
      ))}
    </Box>
  );
}
