/**
 * ModelEditor — LLM configuration editor.
 * Five fields (name/provider/baseUrl/apiKey/model); Tab cycles fields.
 * Keyboard events are dispatched by App; this component only renders and edits fields.
 */

import React from "react";
import { Box, Text, useStdout } from "ink";
import { TextInput } from "./TextInput.js";
import type { ModelProvider } from "../types.js";

/** Field index map: 0=name, 1=provider, 2=baseUrl, 3=apiKey, 4=model */
const FIELD_COUNT = 5;

const PROVIDER_OPTIONS: { value: ModelProvider; label: string }[] = [
  { value: "openai", label: "OpenAI Compatible" },
  { value: "anthropic", label: "Anthropic" },
];

interface ModelEditorProps {
  name: string;
  provider: ModelProvider;
  baseUrl: string;
  apiKey: string;
  model: string;
  focusIndex: number;
  providerSelect: boolean;
  providerCursor: number;
  onNameChange: (name: string) => void;
  onProviderChange: (provider: ModelProvider) => void;
  onBaseUrlChange: (url: string) => void;
  onApiKeyChange: (key: string) => void;
  onModelChange: (model: string) => void;
}

export function ModelEditor({
  name,
  provider,
  baseUrl,
  apiKey,
  model,
  focusIndex,
  providerSelect,
  providerCursor,
  onNameChange,
  onProviderChange,
  onBaseUrlChange,
  onApiKeyChange,
  onModelChange,
}: ModelEditorProps): React.ReactElement {
  const { stdout } = useStdout();
  const columns = Math.max(20, (stdout?.columns || 80) - 12);

  const renderField = (
    idx: number,
    label: string,
    value: string,
    onChange: (v: string) => void,
    opts?: { mask?: string },
  ) => {
    const active = focusIndex === idx && !providerSelect;
    return (
      <Box flexDirection="column">
        <Box>
          <Text color={active ? "white" : "gray"}>
            {active ? "> " : "  "}
          </Text>
          <Text bold color={active ? "white" : "white"}>
            {label}
          </Text>
        </Box>
        {active ? (
          <Box marginLeft={2}>
            <TextInput
              value={value}
              onChange={onChange}
              focus={true}
              columns={columns}
              multiline={false}
              enableCtrlShortcuts={false}
              mask={opts?.mask}
            />
          </Box>
        ) : (
          <Box marginLeft={2}>
            <Text color="gray">
              {opts?.mask && value ? "*".repeat(Math.min(value.length, 20)) : value || "(empty)"}
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
          <Text color={active ? "white" : "gray"}>
            {active ? "> " : "  "}
          </Text>
          <Text bold color={active ? "white" : "white"}>
            Provider
          </Text>
        </Box>
        {active && providerSelect ? (
          <Box flexDirection="column" marginLeft={2}>
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
        ) : (
          <Box marginLeft={2}>
            <Text color={provider === "openai" ? "white" : "#d97706"}>
              {provider === "openai" ? "OpenAI Compatible" : "Anthropic"}
            </Text>
            {active && (
              <Text color="gray"> {"  Enter to change"}</Text>
            )}
          </Box>
        )}
      </Box>
    );
  };

  const separator = (
    <Text color="gray" dimColor>
      {"  \u2500".repeat(20)}
    </Text>
  );

  return (
    <Box flexDirection="column">
      <Text bold color="86">
        Model Editor
      </Text>
      <Text> </Text>
      {renderField(0, "Name", name, onNameChange)}
      <Text> </Text>
      {separator}
      <Text> </Text>
      {renderProviderField()}
      <Text> </Text>
      {separator}
      <Text> </Text>
      {renderField(2, "Base URL", baseUrl, onBaseUrlChange)}
      <Text> </Text>
      {separator}
      <Text> </Text>
      {renderField(3, "API Key", apiKey, onApiKeyChange, { mask: "*" })}
      <Text> </Text>
      {separator}
      <Text> </Text>
      {renderField(4, "Model", model, onModelChange)}
    </Box>
  );
}
