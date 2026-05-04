/**
 * MCPEditor — MCP configuration editor.
 * Keyboard events are dispatched by App; this component only renders and edits fields.
 */

import React from "react";
import { Box, Text, useStdout } from "ink";
import { TextInput } from "./TextInput.js";
import { truncateDisplayValue } from "./ModelEditor.js";

const LABEL_WIDTH = 14;
const JSON_DIVIDER_MIN_WIDTH = 24;
const JSON_DIVIDER_MAX_WIDTH = 52;

export const MCP_EDITOR_COLORS = {
  title: "#67e8f9",
  mode: "#94a3b8",
  active: "#f8fafc",
  inactive: "#94a3b8",
  enabled: "#22c55e",
  disabled: "#fb7185",
  divider: "#475569",
  hint: "#64748b",
  valid: "#22c55e",
  invalid: "#fb7185",
} as const;

export interface MCPConfigStatus {
  state: "valid" | "invalid";
  transport: string;
  detail: string;
}

function asPlainObject(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) return null;
  return value as Record<string, unknown>;
}

function normalizeJsonError(error: unknown): string {
  const message = error instanceof Error ? error.message : String(error);
  return message.replace(/\s+/g, " ").trim() || "Invalid JSON";
}

export function getMCPConfigStatus(config: string): MCPConfigStatus {
  try {
    const parsed = asPlainObject(JSON.parse(config));
    const transportValue = parsed && typeof parsed.transport === "string" ? parsed.transport.trim() : "";
    const hasCommand = parsed && typeof parsed.command === "string" && parsed.command.trim().length > 0;
    const transport = transportValue || (hasCommand ? "stdio" : "unknown");

    return {
      state: "valid",
      transport,
      detail: "ready to save",
    };
  } catch (error) {
    return {
      state: "invalid",
      transport: "unknown",
      detail: normalizeJsonError(error),
    };
  }
}

export function getMCPEditorModeLabel(id: string, name: string): string {
  if (!id) return "new config";
  const trimmedName = name.trim();
  return trimmedName ? `edit: ${trimmedName}` : "edit config";
}

export function buildMCPConfigDivider(terminalWidth: number): string {
  const width = Math.max(
    JSON_DIVIDER_MIN_WIDTH,
    Math.min(JSON_DIVIDER_MAX_WIDTH, terminalWidth - 28),
  );
  return "─".repeat(width);
}

interface MCPEditorProps {
  id?: string;
  name: string;
  config: string;
  enabled: boolean;
  focusName: boolean;
  onNameChange: (name: string) => void;
  onConfigChange: (config: string) => void;
  onToggleEnabled: () => void;
  onToggleFocus: () => void;
  onSave: () => void;
  onBack: () => void;
}

export function MCPEditor({
  id = "",
  name,
  config,
  enabled,
  focusName,
  onNameChange,
  onConfigChange,
  onToggleEnabled,
  onToggleFocus,
  onSave,
  onBack,
}: MCPEditorProps): React.ReactElement {
  const { stdout } = useStdout();
  const terminalWidth = Math.max(20, stdout?.columns || 80);
  const columns = Math.max(20, terminalWidth - 12);
  const valueColumns = Math.max(12, columns - LABEL_WIDTH);
  const divider = buildMCPConfigDivider(terminalWidth);
  const status = getMCPConfigStatus(config);
  const modeLabel = truncateDisplayValue(getMCPEditorModeLabel(id, name), Math.max(12, valueColumns));
  const statusColor = status.state === "valid" ? MCP_EDITOR_COLORS.valid : MCP_EDITOR_COLORS.invalid;
  const statusDetail = truncateDisplayValue(status.detail, Math.max(16, valueColumns));

  const renderFieldLabel = (active: boolean, label: string) => (
    <Box width={LABEL_WIDTH}>
      <Text color={active ? MCP_EDITOR_COLORS.active : MCP_EDITOR_COLORS.inactive}>
        {active ? "> " : "  "}
      </Text>
      <Text bold={active} color={active ? MCP_EDITOR_COLORS.active : MCP_EDITOR_COLORS.inactive}>
        {label}
      </Text>
    </Box>
  );

  return (
    <Box flexDirection="column">
      <Box>
        <Text bold color={MCP_EDITOR_COLORS.title}>MCP Editor</Text>
        <Text color={MCP_EDITOR_COLORS.mode}>  {modeLabel}</Text>
      </Box>
      <Text> </Text>

      <Box>
        {renderFieldLabel(focusName, "Name")}
        {focusName ? (
          <TextInput
            value={name}
            onChange={onNameChange}
            focus={focusName}
            columns={valueColumns}
            multiline={false}
            enableCtrlShortcuts={false}
          />
        ) : (
          <Box width={valueColumns}>
            <Text color={MCP_EDITOR_COLORS.inactive}>
              {truncateDisplayValue(name.trim() || "(empty)", valueColumns)}
            </Text>
          </Box>
        )}
      </Box>

      <Box>
        {renderFieldLabel(false, "Enabled")}
        <Text bold color={enabled ? MCP_EDITOR_COLORS.enabled : MCP_EDITOR_COLORS.disabled}>
          {enabled ? "on" : "off"}
        </Text>
        <Text color={MCP_EDITOR_COLORS.hint}>  Ctrl+E toggle</Text>
      </Box>

      <Text> </Text>

      <Box>
        {renderFieldLabel(!focusName, "Config JSON")}
        <Text bold color={statusColor}>{status.state}</Text>
        <Text color={MCP_EDITOR_COLORS.inactive}>  {status.transport}</Text>
        <Text color={MCP_EDITOR_COLORS.hint}>  {statusDetail}</Text>
      </Box>

      <Box marginLeft={LABEL_WIDTH}>
        <Text color={MCP_EDITOR_COLORS.divider}>{divider}</Text>
      </Box>

      <Box marginLeft={LABEL_WIDTH} flexDirection="column">
        {!focusName ? (
          <TextInput
            value={config}
            onChange={onConfigChange}
            focus={!focusName}
            columns={valueColumns}
            multiline={true}
            enableCtrlShortcuts={false}
          />
        ) : (
          <Text color={MCP_EDITOR_COLORS.inactive}>{config}</Text>
        )}
      </Box>

      <Box marginLeft={LABEL_WIDTH}>
        <Text color={MCP_EDITOR_COLORS.divider}>{divider}</Text>
      </Box>

      <Text> </Text>
      <Text color={MCP_EDITOR_COLORS.hint}>
        Tab field | Ctrl+S save | Esc back
      </Text>
    </Box>
  );
}
