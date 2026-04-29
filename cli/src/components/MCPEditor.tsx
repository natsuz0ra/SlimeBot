/**
 * MCPEditor — MCP configuration editor.
 * Keyboard events are dispatched by App; this component only renders and edits fields.
 */

import React from "react";
import { Box, Text, useStdout } from "ink";
import { TextInput } from "./TextInput.js";

interface MCPEditorProps {
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
  const columns = Math.max(20, (stdout?.columns || 80) - 8);

  return (
    <Box flexDirection="column">
      <Text bold color="86">
        MCP Editor
      </Text>

      <Box>
        <Text>{focusName ? "> " : "  "}</Text>
        <Text bold>Name: </Text>
        {focusName ? (
          <TextInput
            value={name}
            onChange={onNameChange}
            focus={focusName}
            columns={columns}
            multiline={false}
            enableCtrlShortcuts={false}
          />
        ) : (
          <Text>{name}</Text>
        )}
      </Box>

      <Text>
        Enabled:{" "}
        <Text bold color={enabled ? "green" : "red"}>
          {enabled ? "true" : "false"}
        </Text>
      </Text>

      <Box flexDirection="column">
        <Text>{!focusName ? "> " : "  "}</Text>
        {!focusName ? (
          <TextInput
            value={config}
            onChange={onConfigChange}
            focus={!focusName}
            columns={columns}
            multiline={true}
            enableCtrlShortcuts={false}
          />
        ) : (
          <Text>{config}</Text>
        )}
      </Box>

      <Text color="gray">
        Tab to switch focus | Ctrl+E to enable/disable | Ctrl+S to save | Esc to return
      </Text>
    </Box>
  );
}
