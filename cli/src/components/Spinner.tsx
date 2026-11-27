/**
 * Spinner — Braille character rotation animation.
 * Uses a Braille dot-pattern sequence for a rotating arc effect.
 */

import React, { useEffect, useRef, useState } from "react";
import { Text } from "ink";
import { CLI_THEME } from "../utils/theme.js";

const FRAMES = ["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"];
const FRAME_INTERVAL = 80; // Same refresh rate as GradientFlowText

interface SpinnerProps {
  enabled: boolean;
  color?: string;
}

export function Spinner({ enabled, color = CLI_THEME.accent }: SpinnerProps): React.ReactElement {
  const [frameIndex, setFrameIndex] = useState(0);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    if (!enabled) {
      if (timerRef.current) {
        clearInterval(timerRef.current);
        timerRef.current = null;
      }
      return;
    }

    timerRef.current = setInterval(() => {
      setFrameIndex((prev) => (prev + 1) % FRAMES.length);
    }, FRAME_INTERVAL);

    return () => {
      if (timerRef.current) {
        clearInterval(timerRef.current);
        timerRef.current = null;
      }
    };
  }, [enabled]);

  if (!enabled) {
    return <Text />;
  }

  return (
    <Text bold color={color}>
      {FRAMES[frameIndex]}
    </Text>
  );
}
