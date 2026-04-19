/**
 * PlanConfirmView — plan confirmation dialog for CLI.
 * Two options: Execute Plan + Feedback input, ESC to cancel.
 */

import React from "react";
import { Box, Text } from "ink";
import { TextInput } from "./TextInput.js";

const PLACEHOLDER = "Type feedback to modify plan...";

interface PlanConfirmViewProps {
  cursor: number;
  feedback: string;
  feedbackKey: number;
  onFeedbackChange: (value: string) => void;
  onFeedbackSubmit: (value: string) => void;
  onEscape: () => void;
  columns: number;
}

export function PlanConfirmView({
  cursor,
  feedback,
  feedbackKey,
  onFeedbackChange,
  onFeedbackSubmit,
  onEscape,
  columns,
}: PlanConfirmViewProps): React.ReactElement {
  const inputFocused = cursor === 1;

  return (
    <Box flexDirection="column">
      <Text bold color="cyan">
        Plan Generated — Choose an action
      </Text>
      <Text>
        {cursor === 0 ? (
          <Text bold color="white">
            {"  > 1. Execute Plan"}
          </Text>
        ) : (
          <Text color="gray">
            {"    1. Execute Plan"}
          </Text>
        )}
      </Text>
      <Box>
        {inputFocused ? (
          <Text>
            <Text bold color="white">
              {"  > 2. "}
            </Text>
          </Text>
        ) : (
          <Text color="gray">
            {"    2. "}
          </Text>
        )}
        {inputFocused ? (
          <Box>
            <TextInput
              key={feedbackKey}
              value={feedback}
              onChange={onFeedbackChange}
              onSubmit={onFeedbackSubmit}
              onEscape={onEscape}
              focus={true}
              columns={Math.max(20, columns - 8)}
              cursorChar={feedback ? undefined : ""}
            />
            {!feedback && <Text color="white" dimColor>{PLACEHOLDER}</Text>}
          </Box>
        ) : (
          <Text color="gray">{feedback || PLACEHOLDER}</Text>
        )}
      </Box>
      <Text color="gray">
        Arrow keys to navigate | Enter to select | Esc to cancel
      </Text>
    </Box>
  );
}
