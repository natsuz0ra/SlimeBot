import React, { useMemo } from "react";
import { Box, Text } from "ink";
import { renderMarkdownLines } from "../utils/markdownRenderer.js";

type MarkdownProps = {
  content: string;
  maxWidth: number;
  compact?: boolean;
  preserveTrailingBlanks?: boolean;
  renderPrefix?: (lineIndex: number) => React.ReactNode;
};

export function Markdown({
  content,
  maxWidth,
  compact = false,
  preserveTrailingBlanks = false,
  renderPrefix,
}: MarkdownProps): React.ReactElement {
  const contentWidth = Math.max(1, Math.floor(maxWidth));
  const lines = useMemo(
    () => renderMarkdownLines(content, contentWidth, compact, preserveTrailingBlanks),
    [content, contentWidth, compact, preserveTrailingBlanks],
  );

  return (
    <Box flexDirection="column">
      {lines.map((line, index) => (
        <Text key={`${index}-${line}`}>
          {renderPrefix?.(index)}
          <Text>{line}</Text>
        </Text>
      ))}
    </Box>
  );
}

export function StreamingMarkdown(
  props: Omit<MarkdownProps, "preserveTrailingBlanks">,
): React.ReactElement {
  return <Markdown {...props} preserveTrailingBlanks={false} />;
}
