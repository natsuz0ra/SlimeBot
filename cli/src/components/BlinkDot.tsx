/**
 * BlinkDot — 闪烁圆点组件。
 * blinkOn=true 时渲染彩色 ●，blinkOn=false 时渲染等宽空格。
 * 两者视觉宽度一致（都是 1 个字符），因此不会导致文本抖动。
 */

import React from "react";
import { Text } from "ink";
import { DOT } from "../utils/terminal.js";

interface BlinkDotProps {
  color: string;
  blinkOn: boolean;
}

/** 圆点颜色映射 */
const DOT_COLORS: Record<string, string> = {
  white: "white",
  yellow: "yellow",
  green: "green",
  red: "red",
};

export function BlinkDot({ color, blinkOn }: BlinkDotProps): React.ReactElement {
  const inkColor = DOT_COLORS[color] || "white";

  return (
    <Text bold color={blinkOn ? inkColor : undefined}>
      {blinkOn ? DOT : " "}
    </Text>
  );
}

/** 获取 AI 等待时的圆点状态 */
export function assistantDotState(waiting: boolean): { color: string; blinkOn: boolean } {
  return { color: "white", blinkOn: waiting };
}

/** 获取工具调用的圆点状态 */
export function toolDotState(status: string): { color: string; blinkOn: boolean } {
  switch (status.trim()) {
    case "pending":
      return { color: "yellow", blinkOn: false };
    case "executing":
      return { color: "yellow", blinkOn: true };
    case "completed":
      return { color: "green", blinkOn: false };
    case "error":
    case "rejected":
      return { color: "red", blinkOn: false };
    default:
      return { color: "white", blinkOn: false };
  }
}
