/**
 * Banner — top bar showing pixel Slime logo, product name, version, model, session, and working directory.
 */

import React from "react";
import { Box, Text } from "ink";
import { CLI_THEME } from "../utils/theme.js";
import { shortenHomePath } from "../utils/terminal.js";

interface BannerProps {
  version: string;
  modelName: string;
  cwd: string;
  sessionName?: string;
  approvalMode?: string;
  thinkingLevel?: string;
  updateAvailable?: boolean;
}

export function Banner({
  version,
  modelName,
  cwd,
  sessionName,
  approvalMode,
  thinkingLevel,
  updateAvailable,
}: BannerProps): React.ReactElement {
  const shortCwd = shortenHomePath(cwd);
  const trimmedSession = sessionName?.trim();

  return (
    <Box flexDirection="row">
      <Box flexDirection="column" marginRight={2} width={10} flexShrink={0}>
        <Text>
          {" "}
          <Text backgroundColor={CLI_THEME.logo}>{"        "}</Text>
          {" "}
        </Text>
        <Text>
          <Text backgroundColor={CLI_THEME.logo}>{"   "}</Text>
          <Text backgroundColor={CLI_THEME.logoEye}>{" "}</Text>
          <Text backgroundColor={CLI_THEME.logo}>{"  "}</Text>
          <Text backgroundColor={CLI_THEME.logoEye}>{" "}</Text>
          <Text backgroundColor={CLI_THEME.logo}>{"   "}</Text>
        </Text>
        <Text backgroundColor={CLI_THEME.logo}>{"          "}</Text>
      </Box>

      <Box flexDirection="column" flexGrow={1} flexShrink={1} minWidth={0}>
        <Text wrap="truncate">
          <Text bold>SlimeBot CLI </Text>
          <Text color={CLI_THEME.secondary}>{version}</Text>
          {updateAvailable && <Text color={CLI_THEME.warning}> [new]</Text>}
          {approvalMode === "auto_review" && <Text color={CLI_THEME.warning}> [auto review]</Text>}
          {approvalMode === "auto" && <Text color={CLI_THEME.warning}> [auto]</Text>}
        </Text>
        <Text wrap="truncate">
          <Text color={CLI_THEME.secondary}>{modelName || "(none)"}</Text>
          {thinkingLevel && thinkingLevel !== "off" ? (
            <Text color={CLI_THEME.accent}> [think:{thinkingLevel}]</Text>
          ) : null}
          {trimmedSession ? <Text color={CLI_THEME.hint}> · {trimmedSession}</Text> : null}
        </Text>
        <Text color={CLI_THEME.hint} wrap="truncate">
          {shortCwd || cwd}
        </Text>
      </Box>
    </Box>
  );
}
