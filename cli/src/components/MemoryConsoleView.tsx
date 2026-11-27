import { Box, Text } from "ink";
import type React from "react";
import type { MemorySnapshot, MemoryTarget, MemoryTargetState } from "../types.js";
import {
  MEMORY_CONSOLE_ACTIONS,
  MEMORY_CONSOLE_EDIT_LIMITS,
  MEMORY_CONSOLE_RESET_ACTIONS,
  type MemoryConsoleEditField,
  type MemoryConsoleMode,
} from "../utils/memoryConsole.js";
import { wrapText } from "../utils/format.js";
import { CLI_ACCENT_COLOR } from "../utils/terminal.js";
import { CLI_THEME } from "../utils/theme.js";
import { MENU_ITEM_COLORS } from "./MenuView.js";

export const MEMORY_CONSOLE_COLORS = {
  title: CLI_ACCENT_COLOR,
  activeCursor: CLI_ACCENT_COLOR,
  activeText: "#f8fafc",
  inactiveText: "#cbd5e1",
  description: CLI_THEME.secondary,
  success: "#34d399",
  warning: "#fbbf24",
  danger: "#fb7185",
  hint: CLI_THEME.hint,
} as const;

interface MemoryConsoleViewProps {
  snapshot: MemorySnapshot | null;
  loading: boolean;
  cursor: number;
  mode: MemoryConsoleMode;
  editingField: MemoryConsoleEditField | null;
  draft: string;
  viewTarget: MemoryTarget | null;
  message: string;
  columns: number;
}

function statusText(enabled: boolean): string {
  return enabled ? "ON" : "OFF";
}

export function formatMemoryUsageBar(state: MemoryTargetState, width = 10): string {
  const percent = state.charLimit > 0 ? Math.min(100, Math.round((state.usageChars / state.charLimit) * 100)) : 0;
  const filled = Math.max(0, Math.min(width, Math.round((percent / 100) * width)));
  return `${"█".repeat(filled)}${"░".repeat(width - filled)} ${percent}%`;
}

function targetTitle(target: MemoryTarget): string {
  return target === "memory" ? "Personal notes" : "User profile";
}

function renderTargetCard(state: MemoryTargetState): React.ReactElement {
  return (
    <Box flexDirection="column" borderStyle="round" borderColor="#334155" paddingX={1} width={30}>
      <Text bold color={MEMORY_CONSOLE_COLORS.activeText}>{targetTitle(state.target)}</Text>
      <Text color={MEMORY_CONSOLE_COLORS.activeCursor}>{state.usageChars} / {state.charLimit} chars</Text>
      <Text color={MENU_ITEM_COLORS.description}>{formatMemoryUsageBar(state)}</Text>
      <Text color={state.enabled ? MEMORY_CONSOLE_COLORS.success : MEMORY_CONSOLE_COLORS.warning}>
        {state.entryCount} entries · {statusText(state.enabled)}
      </Text>
    </Box>
  );
}

function renderActions(cursor: number): React.ReactElement {
  return (
    <Box flexDirection="column" marginTop={1}>
      {MEMORY_CONSOLE_ACTIONS.map((item, index) => {
        const active = cursor === index;
        return (
          <Box key={item.action} flexDirection="column">
            <Text>
              <Text color={active ? MEMORY_CONSOLE_COLORS.activeCursor : MEMORY_CONSOLE_COLORS.hint}>
                {active ? "❯ " : "  "}
              </Text>
              <Text bold={active} color={active ? MEMORY_CONSOLE_COLORS.activeText : MEMORY_CONSOLE_COLORS.inactiveText}>
                {item.title}
              </Text>
            </Text>
            <Text color={MEMORY_CONSOLE_COLORS.description}>  {item.desc}</Text>
          </Box>
        );
      })}
    </Box>
  );
}

function renderReset(cursor: number): React.ReactElement {
  return (
    <Box flexDirection="column" marginTop={1}>
      <Text bold color={MEMORY_CONSOLE_COLORS.warning}>Reset memory</Text>
      {MEMORY_CONSOLE_RESET_ACTIONS.map((item, index) => {
        const active = cursor === index;
        return (
          <Box key={item.action} flexDirection="column">
            <Text>
              <Text color={active ? MEMORY_CONSOLE_COLORS.activeCursor : MEMORY_CONSOLE_COLORS.hint}>
                {active ? "❯ " : "  "}
              </Text>
              <Text bold={active} color={item.action === "cancel" ? MEMORY_CONSOLE_COLORS.inactiveText : MEMORY_CONSOLE_COLORS.danger}>
                {item.title}
              </Text>
            </Text>
            <Text color={MEMORY_CONSOLE_COLORS.description}>  {item.desc}</Text>
          </Box>
        );
      })}
    </Box>
  );
}

function renderEdit(field: MemoryConsoleEditField | null, draft: string): React.ReactElement {
  if (!field) return <Text color={MEMORY_CONSOLE_COLORS.warning}>No field selected.</Text>;
  const limits = MEMORY_CONSOLE_EDIT_LIMITS[field];
  return (
    <Box flexDirection="column" marginTop={1}>
      <Text bold color={MEMORY_CONSOLE_COLORS.activeText}>Edit {limits.label}</Text>
      <Text color={MEMORY_CONSOLE_COLORS.description}>Allowed range: {limits.min} - {limits.max} {limits.unit}</Text>
      <Box marginTop={1}>
        <Text color={MEMORY_CONSOLE_COLORS.activeCursor}>Value: </Text>
        <Text color={MEMORY_CONSOLE_COLORS.activeText}>{draft || "(empty)"}</Text>
      </Box>
    </Box>
  );
}

function renderEntries(snapshot: MemorySnapshot, target: MemoryTarget, width: number): React.ReactElement {
  const state = target === "memory" ? snapshot.memory : snapshot.user;
  return (
    <Box flexDirection="column" marginTop={1}>
      <Text bold color={MEMORY_CONSOLE_COLORS.activeText}>{targetTitle(target)} entries</Text>
      {state.entries.length === 0 ? (
        <Text color={MEMORY_CONSOLE_COLORS.description}>  (empty)</Text>
      ) : (
        state.entries.map((entry, index) => (
          <Box key={`${target}-${index}`} flexDirection="column">
            {wrapText(`${index + 1}. ${entry.replace(/\n/g, " ")}`, Math.max(20, width - 4)).split("\n").map((line) => (
              <Text key={`${target}-${index}-${line}`} color={MEMORY_CONSOLE_COLORS.inactiveText}>  {line}</Text>
            ))}
          </Box>
        ))
      )}
    </Box>
  );
}

export function memoryConsoleHint(mode: MemoryConsoleMode): string {
  if (mode === "edit") return "Type digits | Enter save | Esc cancel";
  if (mode === "reset") return "↑/↓ select | Enter confirm | Esc cancel";
  if (mode === "view") return "Esc return";
  return "↑/↓ select | Enter apply/edit | R reset | Esc close";
}

export default function MemoryConsoleView({
  snapshot,
  loading,
  cursor,
  mode,
  editingField,
  draft,
  viewTarget,
  message,
  columns,
}: MemoryConsoleViewProps): React.ReactElement {
  const width = Math.min(columns || 80, 96);

  if (loading) {
    return (
      <Box flexDirection="column" paddingX={1} width={width}>
        <Text bold color={MEMORY_CONSOLE_COLORS.title}>Memory Console</Text>
        <Text color={MEMORY_CONSOLE_COLORS.description}>Loading memory settings...</Text>
      </Box>
    );
  }

  if (!snapshot) {
    return (
      <Box flexDirection="column" paddingX={1} width={width}>
        <Text bold color={MEMORY_CONSOLE_COLORS.title}>Memory Console</Text>
        <Text color={MEMORY_CONSOLE_COLORS.danger}>{message || "Memory data is unavailable."}</Text>
        <Text color={MEMORY_CONSOLE_COLORS.hint}>Esc return</Text>
      </Box>
    );
  }

  return (
    <Box flexDirection="column" paddingX={1} width={width}>
      <Text bold color={MEMORY_CONSOLE_COLORS.title}>Memory Console</Text>
      <Box flexDirection="column" borderStyle="round" borderColor="#334155" paddingX={1} marginTop={1}>
        <Text color={snapshot.memoryEnabled ? MEMORY_CONSOLE_COLORS.success : MEMORY_CONSOLE_COLORS.warning}>
          Long-term memory: {statusText(snapshot.memoryEnabled)}
        </Text>
        <Text color={snapshot.memoryUserProfileEnabled ? MEMORY_CONSOLE_COLORS.success : MEMORY_CONSOLE_COLORS.warning}>
          User profile:     {statusText(snapshot.memoryUserProfileEnabled)}
        </Text>
        <Text color={MEMORY_CONSOLE_COLORS.inactiveText}>Review interval:  {snapshot.memoryNudgeInterval} turns</Text>
      </Box>

      <Box marginTop={1} flexWrap="wrap">
        <Box marginRight={2}>{renderTargetCard(snapshot.memory)}</Box>
        {renderTargetCard(snapshot.user)}
      </Box>

      {mode === "actions" && renderActions(cursor)}
      {mode === "reset" && renderReset(cursor)}
      {mode === "edit" && renderEdit(editingField, draft)}
      {mode === "view" && viewTarget && renderEntries(snapshot, viewTarget, width)}

      {message && (
        <Box marginTop={1}>
          <Text color={message.toLowerCase().includes("failed") || message.toLowerCase().includes("must") ? MEMORY_CONSOLE_COLORS.danger : MEMORY_CONSOLE_COLORS.success}>
            {message}
          </Text>
        </Box>
      )}

      <Box marginTop={1}>
        <Text color={MEMORY_CONSOLE_COLORS.hint}>{memoryConsoleHint(mode)}</Text>
      </Box>
    </Box>
  );
}
