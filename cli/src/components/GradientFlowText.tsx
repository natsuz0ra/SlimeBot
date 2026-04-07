/**
 * GradientFlowText — 流光扫过渐变色文字组件。
 * 一道高光从左到右反复扫过文字，基于 banner logo 紫色 (#a78bfa)。
 * 参考了 Claude Code Spinner 的 shimmer 实现。
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

/** 在两个 RGB 颜色之间线性插值 */
function interpolateColor(c1: RGB, c2: RGB, t: number): RGB {
  return {
    r: Math.round(c1.r + (c2.r - c1.r) * t),
    g: Math.round(c1.g + (c2.g - c1.g) * t),
    b: Math.round(c1.b + (c2.b - c1.b) * t),
  };
}

/** RGB 对象转 Ink 可识别的 rgb() 字符串 */
function toRgbString(c: RGB): string {
  return `rgb(${c.r},${c.g},${c.b})`;
}

const DEFAULT_BASE: RGB = { r: 130, g: 115, b: 160 };   // #8273a0 可读中紫
const DEFAULT_SHIMMER: RGB = { r: 167, g: 139, b: 250 }; // #a78bfa logo紫

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

  // 流光宽度（高光覆盖的字符范围）
  const shimmerWidth = 8;
  // 淡入/淡出长度（让流光从无到有平滑出现）
  const fadeLength = shimmerWidth + 2;
  // 总循环：淡入 → 扫过 → 淡出 → 暗场停顿
  const sweepLength = text.length + shimmerWidth;
  const darkPause = 6;
  const cycleLength = fadeLength + sweepLength + fadeLength + darkPause;
  // 当前位置
  const pos = offset % cycleLength;

  // 整体强度包络：淡入 → 满亮度 → 淡出 → 暗
  let envelope: number;
  if (pos < fadeLength) {
    // 淡入阶段
    envelope = pos / fadeLength;
  } else if (pos < fadeLength + sweepLength) {
    // 满亮度扫过阶段
    envelope = 1.0;
  } else if (pos < fadeLength + sweepLength + fadeLength) {
    // 淡出阶段
    const fadeProgress = pos - fadeLength - sweepLength;
    envelope = 1.0 - fadeProgress / fadeLength;
  } else {
    // 暗场停顿
    envelope = 0;
  }
  // 平滑包络曲线
  envelope = (Math.sin(envelope * Math.PI - Math.PI / 2) + 1) / 2;

  // 流光中心（在文本坐标中的位置，负值表示还在左侧外面）
  const center = pos - fadeLength;

  return (
    <Text>
      {text.split("").map((char, i) => {
        // 字符到高光中心的距离
        const distance = Math.abs(i - center);
        // 高光衰减
        const rawT = distance < shimmerWidth
          ? Math.max(0, 1 - distance / shimmerWidth)
          : 0;
        // 乘以整体包络强度
        const t = rawT * envelope;
        // sin 平滑
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
