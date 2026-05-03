/**
 * ModelEditor — LLM configuration editor.
 * Six fields (name/provider/baseUrl/apiKey/model/contextSize); Tab cycles fields.
 * Keyboard events are dispatched by App; this component only renders and edits fields.
 */

import React from "react";
import { Box, Text, useStdout } from "ink";
import { TextInput } from "./TextInput.js";
import type { ModelProvider } from "../types.js";
import { clampContextSize, formatContextSize, renderContextSizeBar } from "../utils/contextSize.js";
import { stringWidth } from "../utils/stringWidth.js";

/** Field index map: 0=name, 1=provider, 2=baseUrl, 3=apiKey, 4=model, 5=contextSize */
const FIELD_COUNT = 6;
const LABEL_WIDTH = 14;

const PROVIDER_OPTIONS: { value: ModelProvider; label: string }[] = [
  { value: "openai", label: "OpenAI Compatible" },
  { value: "anthropic", label: "Anthropic" },
  { value: "deepseek", label: "DeepSeek" },
];

export function truncateDisplayValue(value: string, maxWidth: number): string {
  if (maxWidth <= 0) return "";
  if (stringWidth(value) <= maxWidth) return value;
  if (maxWidth === 1) return "…";

  let output = "";
  let width = 0;
  const ellipsisWidth = 1;
  for (const char of value) {
    const charWidth = stringWidth(char);
    if (width + charWidth + ellipsisWidth > maxWidth) break;
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
  provider: ModelProvider;
  baseUrl: string;
  apiKey: string;
  model: string;
  contextSize: string;
  focusIndex: number;
  providerSelect: boolean;
  providerCursor: number;
  onNameChange: (name: string) => void;
  onProviderChange: (provider: ModelProvider) => void;
  onBaseUrlChange: (url: string) => void;
  onApiKeyChange: (key: string) => void;
  onModelChange: (model: string) => void;
  onContextSizeChange: (contextSize: string) => void;
}

export function ModelEditor({
  name,
  provider,
  baseUrl,
  apiKey,
  model,
  contextSize,
  focusIndex,
  providerSelect,
  providerCursor,
  onNameChange,
  onProviderChange,
  onBaseUrlChange,
  onApiKeyChange,
  onModelChange,
  onContextSizeChange,
}: ModelEditorProps): React.ReactElement {
  const { stdout } = useStdout();
  const columns = Math.max(20, (stdout?.columns || 80) - 12);
  const valueColumns = Math.max(12, columns - LABEL_WIDTH);
  const currentProvider = PROVIDER_OPTIONS.find((opt) => opt.value === provider) || PROVIDER_OPTIONS[0];

  const renderFieldLabel = (idx: number, label: string, forceActive = false) => {
    const active = forceActive || (focusIndex === idx && !providerSelect);
    return (
      <Box width={LABEL_WIDTH}>
        <Text color={active ? "white" : "gray"}>
          {active ? "> " : "  "}
        </Text>
        <Text bold={active} color={active ? "white" : "gray"}>
          {label}
        </Text>
      </Box>
    );
  };

  const renderField = (
    idx: number,
    label: string,
    value: string,
    onChange: (v: string) => void,
    opts?: { mask?: string },
  ) => {
    const active = focusIndex === idx && !providerSelect;
    return (
      <Box>
        {renderFieldLabel(idx, label)}
        {active ? (
          <Box>
            <TextInput
              value={value}
              onChange={onChange}
              focus={true}
              columns={valueColumns}
              multiline={false}
              enableCtrlShortcuts={false}
              mask={opts?.mask}
            />
          </Box>
        ) : (
          <Box width={valueColumns}>
            <Text color="gray">
              {formatModelFieldValue(value, valueColumns, { mask: Boolean(opts?.mask) })}
            </Text>
          </Box>
        )}
      </Box>
    );
  };

  const renderProviderField = () => {
    const active = focusIndex === 1;
    return (
      <Box flexDirection="column">
        <Box>
          {renderFieldLabel(1, "Provider", active)}
          <Box width={valueColumns}>
            <Text color={provider === "anthropic" ? "#d97706" : provider === "deepseek" ? "#14b8a6" : "white"}>
              {currentProvider.label}
            </Text>
            {active && (
              <Text color="gray"> {"  Enter to change"}</Text>
            )}
          </Box>
        </Box>
        {active && providerSelect && (
          <Box flexDirection="column" marginLeft={LABEL_WIDTH}>
            {PROVIDER_OPTIONS.map((opt, i) => (
              <Text key={opt.value}>
                <Text color={i === providerCursor ? "white" : "gray"}>
                  {i === providerCursor ? "\u276F" : " "}
                </Text>
                <Text>{" "}</Text>
                <Text bold={i === providerCursor} color={i === providerCursor ? "white" : "white"}>
                  {opt.label}
                </Text>
              </Text>
            ))}
          </Box>
        )}
      </Box>
    );
  };

  const renderContextSizeField = () => {
    const active = focusIndex === 5 && !providerSelect;
    const clamped = clampContextSize(contextSize);
    const formatted = formatContextSize(clamped);
    const barWidth = Math.min(24, Math.max(8, valueColumns - stringWidth(formatted) - 4));
    return (
      <Box flexDirection="column">
        <Box>
          {renderFieldLabel(5, "Context")}
          <Text color="cyan">[{renderContextSizeBar(clamped, barWidth)}]</Text>
          <Text color="gray"> {formatted}</Text>
        </Box>
        {active ? (
          <Box marginLeft={LABEL_WIDTH}>
            <TextInput
              value={contextSize}
              onChange={onContextSizeChange}
              focus={true}
              columns={Math.max(10, valueColumns - 18)}
              multiline={false}
              enableCtrlShortcuts={false}
            />
            <Text color="gray"> {"  ←/→ 1K  ↑/↓ 32K"}</Text>
          </Box>
        ) : null}
      </Box>
    );
  };

  return (
    <Box flexDirection="column">
      <Text bold color="86">
        Model Editor
      </Text>
      {renderField(0, "Name", name, onNameChange)}
      {renderProviderField()}
      {renderField(2, "Base URL", baseUrl, onBaseUrlChange)}
      {renderField(3, "API Key", apiKey, onApiKeyChange, { mask: "*" })}
      {renderField(4, "Model", model, onModelChange)}
      {renderContextSizeField()}
    </Box>
  );
}
