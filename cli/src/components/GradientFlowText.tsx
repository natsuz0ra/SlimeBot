/**
 * GradientFlowText — gradient text with a sweeping highlight.
 * A highlight sweeps left-to-right on loop, using banner logo purple (#a78bfa).
 * Inspired by Claude Code Spinner shimmer.
 */

import React, { useEffect, useRef, useState } from "react";
import { Text } from "ink";

interface RGB {
  r: number;
  g: number;
  b: number;
}

interface GradientFlowTextProps {
  text: string;
  enabled: boolean;
  baseColor?: RGB;
  shimmerColor?: RGB;
}

/** Linear interpolation between two RGB colors */
function interpolateColor(c1: RGB, c2: RGB, t: number): RGB {
  return {
    r: Math.round(c1.r + (c2.r - c1.r) * t),
    g: Math.round(c1.g + (c2.g - c1.g) * t),
    b: Math.round(c1.b + (c2.b - c1.b) * t),
  };
}

/** RGB object to Ink rgb() string */
function toRgbString(c: RGB): string {
  return `rgb(${c.r},${c.g},${c.b})`;
}

const DEFAULT_BASE: RGB = { r: 169, g: 141, b: 238 };   // #a98dee bright readable purple
const DEFAULT_SHIMMER: RGB = { r: 233, g: 213, b: 255 }; // #e9d5ff pale purple highlight

export function GradientFlowText({
  text,
  enabled,
  baseColor = DEFAULT_BASE,
  shimmerColor = DEFAULT_SHIMMER,
}: GradientFlowTextProps): React.ReactElement {
  const [offset, setOffset] = useState(0);
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
      setOffset((prev) => prev + 1);
    }, 80);

    return () => {
      if (timerRef.current) {
        clearInterval(timerRef.current);
        timerRef.current = null;
      }
    };
  }, [enabled]);

  if (!enabled || !text) {
    return <Text color="gray">{text}</Text>;
  }

  // Shimmer width (characters covered by the highlight)
  const shimmerWidth = 8;
  // Fade in/out length (smooth appearance of the highlight)
  const fadeLength = shimmerWidth + 2;
  // Full cycle: fade in → sweep → fade out → dark pause
  const sweepLength = text.length + shimmerWidth;
  const darkPause = 6;
  const cycleLength = fadeLength + sweepLength + fadeLength + darkPause;
  // Current position in cycle
  const pos = offset % cycleLength;

  // Overall intensity envelope: fade in → full → fade out → dark
  let envelope: number;
  if (pos < fadeLength) {
    // Fade-in phase
    envelope = pos / fadeLength;
  } else if (pos < fadeLength + sweepLength) {
    // Full-brightness sweep
    envelope = 1.0;
  } else if (pos < fadeLength + sweepLength + fadeLength) {
    // Fade-out phase
    const fadeProgress = pos - fadeLength - sweepLength;
    envelope = 1.0 - fadeProgress / fadeLength;
  } else {
    // Dark pause
    envelope = 0;
  }
  // Smooth envelope curve
  envelope = (Math.sin(envelope * Math.PI - Math.PI / 2) + 1) / 2;

  // Highlight center in text coordinates (negative = still left of text)
  const center = pos - fadeLength;

  return (
    <Text>
      {text.split("").map((char, i) => {
        // Distance from char to highlight center
        const distance = Math.abs(i - center);
        // Highlight falloff
        const rawT = distance < shimmerWidth
          ? Math.max(0, 1 - distance / shimmerWidth)
          : 0;
        // Scale by overall envelope
        const t = rawT * envelope;
        // Sin smoothstep
        const smoothT = (Math.sin(t * Math.PI - Math.PI / 2) + 1) / 2;
        const color = interpolateColor(baseColor, shimmerColor, smoothT);

        return (
          <Text key={i} color={toRgbString(color)}>
            {char}
          </Text>
        );
      })}
    </Text>
  );
}
