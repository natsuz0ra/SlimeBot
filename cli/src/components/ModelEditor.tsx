import React from "react";
import { Box, Text, useStdout } from "ink";
import { TextInput } from "./TextInput.js";
import { clampContextSize, formatContextSize, renderContextSizeBar } from "../utils/contextSize.js";
import { stringWidth } from "../utils/stringWidth.js";
import { CLI_ACCENT_COLOR } from "../utils/terminal.js";

const LABEL_WIDTH = 14;

export function truncateDisplayValue(value: string, maxWidth: number): string {
  if (maxWidth <= 0) return "";
  if (stringWidth(value) <= maxWidth) return value;
  if (maxWidth === 1) return "…";
  let output = "";
  let width = 0;
  for (const char of value) {
    const charWidth = stringWidth(char);
    if (width + charWidth + 1 > maxWidth) break;
    output += char;
    width += charWidth;
  }
  return `${output}…`;
}

export function maskModelApiKey(value: string): string {
  if (!value) return "(empty)";
  return "*".repeat(Math.min(value.length, 20));
}

export function formatModelFieldValue(value: string, maxWidth: number, opts?: { mask?: boolean }): string {
  const displayValue = opts?.mask ? maskModelApiKey(value) : value.trim() || "(empty)";
  return truncateDisplayValue(displayValue, maxWidth);
}

export function formatContextSizeDisplay(contextSize: string): string {
  return formatContextSize(clampContextSize(contextSize));
}

interface ModelEditorProps {
  name: string;
  model: string;
  contextSize: string;
  contextSizeSource: "auto" | "detected" | "fallback" | "manual";
  focusIndex: number;
  onNameChange: (name: string) => void;
  onModelChange: (model: string) => void;
  onContextSizeChange: (contextSize: string) => void;
}

export function ModelEditor({ name, model, contextSize, contextSizeSource, focusIndex, onNameChange, onModelChange, onContextSizeChange }: ModelEditorProps): React.ReactElement {
  const { stdout } = useStdout();
  const valueColumns = Math.max(12, (stdout?.columns || 80) - LABEL_WIDTH - 12);
  const field = (index: number, label: string, value: string, onChange: (value: string) => void) => (
    <Box>
      <Box width={LABEL_WIDTH}><Text color={focusIndex === index ? "white" : "gray"}>{focusIndex === index ? "> " : "  "}{label}</Text></Box>
      {focusIndex === index
        ? <TextInput value={value} onChange={onChange} focus columns={valueColumns} multiline={false} enableCtrlShortcuts={false} />
        : <Text color="gray">{formatModelFieldValue(value, valueColumns)}</Text>}
    </Box>
  );
  const clamped = clampContextSize(contextSize);
  const auto = contextSizeSource !== "manual";
  const contextLabel = contextSizeSource === "detected" ? `Auto · ${formatContextSize(Number(contextSize))} detected` : contextSizeSource === "fallback" ? `Auto · ${formatContextSize(Number(contextSize))} default estimate` : auto ? "Auto · detect on save" : `Custom · ${formatContextSize(clamped)}`;
  const barWidth = Math.min(20, valueColumns - stringWidth(contextLabel) - 4);
  const showBar = !auto && barWidth >= 8;
  const contextText = `${showBar ? `[${renderContextSizeBar(clamped, barWidth)}] ` : ""}${truncateDisplayValue(contextLabel, showBar ? valueColumns - barWidth - 3 : valueColumns)}`;
  return (
    <Box flexDirection="column">
      <Text bold color={CLI_ACCENT_COLOR}>Model Editor</Text>
      {field(0, "Name", name, onNameChange)}
      {field(1, "Model ID", model, onModelChange)}
      <Box>
        <Box width={LABEL_WIDTH}><Text color={focusIndex === 2 ? "white" : "gray"}>{focusIndex === 2 ? "> " : "  "}Context</Text></Box>
        <Text color={CLI_ACCENT_COLOR}>{contextText}</Text>
      </Box>
      {focusIndex === 2 && <Box marginLeft={LABEL_WIDTH}><TextInput value={auto ? "" : contextSize} onChange={onContextSizeChange} focus columns={Math.max(10, valueColumns - 18)} multiline={false} enableCtrlShortcuts={false} /><Text color="gray"> {"  type to override · Ctrl+A auto"}</Text></Box>}
    </Box>
  );
}
