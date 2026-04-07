/**
 * Banner — 顶部信息栏，显示产品名 + 版本 + 模型 + 工作目录。
 */

import React from "react";
import { Box, Text } from "ink";

interface BannerProps {
  version: string;
  modelName: string;
  cwd: string;
}

export function Banner({ version, modelName, cwd }: BannerProps): React.ReactElement {
  const logoLines = [
    "██████████",
    "███ ██ ███",
    "██████████",
  ];

  return (
    <Box flexDirection="row">
      <Box flexDirection="column" marginRight={2}>
        {logoLines.map((line, i) => (
          <Text key={i} color="#a78bfa">
            {line}
          </Text>
        ))}
      </Box>

      <Box flexDirection="column">
        <Text>
          <Text bold color="white">
            SlimeBot CLI{" "}
          </Text>
          <Text color="#94a3b8">v{version}</Text>
        </Text>
        <Text color="#9ca3af">{modelName || "(none)"}</Text>
        <Text color="#9ca3af">{cwd}</Text>
      </Box>
    </Box>
  );
}
