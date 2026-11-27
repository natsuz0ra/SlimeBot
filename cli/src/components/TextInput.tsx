import React from "react";
import { Box, Text, useInput } from "ink";
import type { Key } from "ink";
import chalk from "chalk";
import { useTextInput } from "../hooks/useTextInput.js";

export interface TextInputProps {
  value: string;
  onChange: (value: string) => void;
  onSubmit?: (value: string) => void;
  onTab?: () => string | undefined;
  onEscape?: () => void;
  focus: boolean;
  columns: number;
  prompt?: string;
  placeholder?: string;
  multiline?: boolean;
  maxVisibleLines?: number;
  enableCtrlShortcuts?: boolean;
  mask?: string;
  cursorChar?: string;
  onBeforeInput?: (input: string, key: Key) => boolean;
  onUnhandledInput?: (input: string, key: Key) => void;
}

export function TextInput({
  value,
  onChange,
  onSubmit,
  onTab,
  onEscape,
  focus,
  columns,
  placeholder,
  multiline = true,
  maxVisibleLines,
  enableCtrlShortcuts = true,
  mask,
  cursorChar: cursorCharProp,
  onBeforeInput,
  onUnhandledInput,
}: TextInputProps): React.ReactElement {
  const inputState = useTextInput({
    value,
    onChange,
    onSubmit,
    onTab,
    onEscape,
    multiline,
    maxVisibleLines,
    enableCtrlShortcuts,
    mask: mask ?? "",
    cursorChar: cursorCharProp ?? (focus ? " " : ""),
    invert: chalk.inverse,
    columns,
    onUnhandledInput,
  });

  useInput((input, key) => {
    if (onBeforeInput?.(input, key)) {
      return;
    }
    inputState.onInput(input, key);
  }, { isActive: focus });

  if (!value && placeholder) {
    return (
      <Box flexDirection="column">
        <Text wrap="truncate">
          {focus ? chalk.inverse(placeholder[0] || " ") : (placeholder[0] || "")}
          <Text dimColor>{placeholder.slice(1)}</Text>
        </Text>
      </Box>
    );
  }

  return (
    <Box flexDirection="column">
      <Text>{inputState.renderedValue}</Text>
    </Box>
  );
}
