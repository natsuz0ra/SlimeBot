/**
 * Unified CLI TUI color tokens designed for both dark and light terminal themes.
 * Avoids hardcoded "white" so foreground text adapts to the host terminal theme.
 */

export const CLI_THEME = {
  /** Indigo brand accent aligned with the Web/Desktop palette. */
  accent: "#818cf8",
  /** Softer accent tint for secondary tags and active indicators. */
  accentSoft: "#a5b4fc",
  /** Pixel Slime logo brand purple matching the SlimeBot icon. */
  logo: "#8b5cf6",
  /** Pure white eyes on the pixel Slime logo. */
  logoEye: "#ffffff",
  /** Readable secondary slate for descriptions, metadata, and table labels. */
  secondary: "#94a3b8",
  /** Balanced slate for footer hints and shortcut legends. */
  hint: "#7c8ba1",
  /** Quiet slate for horizontal rules and panel borders. */
  border: "#475569",
  /** Muted gray for inactive cursors and subtle guides. */
  muted: "gray",
  /** Semantic status colors with high contrast on dark and light backgrounds. */
  success: "green",
  warning: "yellow",
  error: "red",
  /** Distinct warm amber for Plan mode framing. */
  plan: "#f59e0b",
} as const;

export const CLI_SHIMMER_BASE = { r: 129, g: 140, b: 248 } as const;
export const CLI_SHIMMER_HIGHLIGHT = { r: 199, g: 210, b: 254 } as const;
