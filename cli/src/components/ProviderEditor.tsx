import React from "react";
import { Box, Text, useStdout } from "ink";
import { TextInput } from "./TextInput.js";
import { formatModelFieldValue } from "./ModelEditor.js";
import type { ModelProvider } from "../types.js";
import { CLI_ACCENT_COLOR } from "../utils/terminal.js";

const PROVIDERS: { value: ModelProvider; label: string }[] = [
  { value: "openai", label: "OpenAI Compatible" },
  { value: "anthropic", label: "Anthropic" },
  { value: "deepseek", label: "DeepSeek" },
];

interface ProviderEditorProps {
  name: string;
  protocol: ModelProvider;
  baseUrl: string;
  apiKey: string;
  focusIndex: number;
  protocolSelect: boolean;
  protocolCursor: number;
  onNameChange: (value: string) => void;
  onBaseUrlChange: (value: string) => void;
  onApiKeyChange: (value: string) => void;
}

export function ProviderEditor({ name, protocol, baseUrl, apiKey, focusIndex, protocolSelect, protocolCursor, onNameChange, onBaseUrlChange, onApiKeyChange }: ProviderEditorProps): React.ReactElement {
  const { stdout } = useStdout();
  const valueColumns = Math.max(12, (stdout?.columns || 80) - 26);
  const label = (index: number, text: string) => <Box width={14}><Text color={focusIndex === index ? "white" : "gray"}>{focusIndex === index ? "> " : "  "}{text}</Text></Box>;
  const field = (index: number, text: string, value: string, onChange: (value: string) => void, mask = false) => (
    <Box>
      {label(index, text)}
      {focusIndex === index && !protocolSelect
        ? <TextInput value={value} onChange={onChange} focus columns={valueColumns} multiline={false} enableCtrlShortcuts={false} mask={mask ? "*" : undefined} />
        : <Text color="gray">{formatModelFieldValue(value, valueColumns, { mask })}</Text>}
    </Box>
  );
  return (
    <Box flexDirection="column">
      <Text bold color={CLI_ACCENT_COLOR}>Provider Editor</Text>
      {field(0, "Name", name, onNameChange)}
      <Box>{label(1, "Protocol")}<Text color={CLI_ACCENT_COLOR}>{PROVIDERS.find(item => item.value === protocol)?.label || protocol}</Text>{focusIndex === 1 && <Text color="gray">  Enter to change</Text>}</Box>
      {focusIndex === 1 && protocolSelect && <Box marginLeft={14} flexDirection="column">{PROVIDERS.map((item, index) => <Text key={item.value} color={index === protocolCursor ? "white" : "gray"}>{index === protocolCursor ? "❯ " : "  "}{item.label}</Text>)}</Box>}
      {field(2, "Base URL", baseUrl, onBaseUrlChange)}
      {field(3, "API Key", apiKey, onApiKeyChange, true)}
      <Text color="gray">Editing: blank keeps the key; "-" clears it for keyless endpoints.</Text>
    </Box>
  );
}
