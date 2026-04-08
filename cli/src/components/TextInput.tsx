import React from "react";
import { Box, Text, useInput } from "ink";
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
  multiline?: boolean;
  enableCtrlShortcuts?: boolean;
  mask?: string;
}

export function TextInput({
  value,
  onChange,
  onSubmit,
  onTab,
  onEscape,
  focus,
  columns,
  multiline = true,
  enableCtrlShortcuts = true,
  mask,
}: TextInputProps): React.ReactElement {
  const inputState = useTextInput({
    value,
    onChange,
    onSubmit,
    onTab,
    onEscape,
    multiline,
    enableCtrlShortcuts,
    mask: mask ?? "",
    cursorChar: focus ? " " : "",
    invert: chalk.inverse,
    columns,
  });

  useInput(inputState.onInput, { isActive: focus });

  return (
    <Box flexDirection="column">
      <Text>{inputState.renderedValue}</Text>
    </Box>
  );
}
