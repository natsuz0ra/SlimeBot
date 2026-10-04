/**
 * Banner — top bar showing product name, version, model, and working directory.
 */

import React from "react";
import { Box, Text } from "ink";

interface BannerProps {
  version: string;
  modelName: string;
  cwd: string;
  approvalMode?: string;
  thinkingLevel?: string;
  updateAvailable?: boolean;
}

export function Banner({ version, modelName, cwd, approvalMode, thinkingLevel, updateAvailable }: BannerProps): React.ReactElement {
  const logoLines = [
    "██████████",
    "███ ██ ███",
    "██████████",
  ];

  return (
    <Box flexDirection="row">
      <Box flexDirection="column" marginRight={2} width={10} flexShrink={0}>
        {logoLines.map((line, i) => (
          <Text key={i} color="#a78bfa">
            {line}
          </Text>
        ))}
      </Box>

      <Box flexDirection="column" flexGrow={1} flexShrink={1} minWidth={0}>
        <Text>
          <Text bold color="white">
            SlimeBot CLI{" "}
          </Text>
          <Text color="#94a3b8">{version}</Text>
          {updateAvailable && <Text color="#facc15"> [new]</Text>}
          {approvalMode === "auto_review" && <Text color="#eab308"> [auto review]</Text>}
          {approvalMode === "auto" && <Text color="#eab308"> [auto]</Text>}
        </Text>
        <Text color="#9ca3af">{modelName || "(none)"}{thinkingLevel && thinkingLevel !== "off" ? <Text color="#a78bfa"> [think:{thinkingLevel}]</Text> : ""}</Text>
        <Text color="#9ca3af">{cwd}</Text>
      </Box>
    </Box>
  );
}
