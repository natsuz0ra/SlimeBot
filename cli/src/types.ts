// ===== Domain types =====

export interface Session {
  id: string;
  name: string;
  updatedAt: string;
}

export interface Message {
  id: string;
  sessionId: string;
  role: "user" | "assistant" | "system";
  content: string;
  seq?: number;
  isInterrupted?: boolean;
  isStopPlaceholder?: boolean;
  createdAt: string;
}

export interface LLMConfig {
  id: string;
  name: string;
  provider: string;
  baseUrl: string;
  model: string;
  createdAt: string;
  updatedAt: string;
}

export interface MCPConfig {
  id: string;
  name: string;
  config: string;
  isEnabled: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface Skill {
  id: string;
  name: string;
  description: string;
  relativePath: string;
}

export interface Settings {
  defaultModel: string;
  [key: string]: unknown;
}

// ===== API response types =====

export interface SessionListResponse {
  sessions: Session[];
  hasMore: boolean;
}

export interface SessionHistoryPayload {
  messages: Message[];
  toolCallsByAssistantMessageId: Record<string, ToolCallHistoryItem[]>;
  hasMore: boolean;
}

export interface ToolCallHistoryItem {
  toolCallId: string;
  toolName: string;
  command: string;
  params: Record<string, string>;
  status: string;
  requiresApproval: boolean;
  output?: string;
  error?: string;
  startedAt: string;
  finishedAt?: string;
}

// ===== WebSocket message types =====

export type ToolCallStatus =
  | "pending"
  | "rejected"
  | "executing"
  | "completed"
  | "error";

export interface ToolCallStartData {
  toolCallId: string;
  toolName: string;
  command: string;
  params: Record<string, string>;
  requiresApproval: boolean;
  preamble?: string;
}

export interface ToolCallResultData {
  toolCallId: string;
  toolName: string;
  command: string;
  requiresApproval: boolean;
  status: ToolCallStatus;
  output: string;
  error: string;
}

// ===== UI state types =====

export type ViewMode = "chat" | "menu" | "mcp-editor" | "approval";

export type MenuKind =
  | "session"
  | "model"
  | "skills"
  | "mcp"
  | "help";

export interface TimelineEntry {
  kind: "user" | "assistant" | "system" | "tool";
  content: string;
  toolCallId?: string;
  toolName?: string;
  command?: string;
  params?: Record<string, string>;
  status?: ToolCallStatus;
  output?: string;
  error?: string;
}

export interface MenuItem {
  title: string;
  desc: string;
  data: unknown;
}

export interface MCPConfigItem {
  id: string;
  name: string;
  config: string;
  isEnabled: boolean;
}

// ===== Command types =====

export interface CommandMeta {
  command: string;
  description: string;
}

export const SUPPORTED_COMMANDS: CommandMeta[] = [
  { command: "/new", description: "Create a new chat session" },
  { command: "/session", description: "Open session menu to switch or delete" },
  { command: "/model", description: "Choose the default model" },
  { command: "/skills", description: "View and manage installed skills" },
  { command: "/mcp", description: "Manage MCP configurations" },
  { command: "/help", description: "Show available commands" },
];

// ===== App state =====

export interface AppState {
  view: ViewMode;

  // Chat
  sessionId: string;
  sessionName: string;
  modelId: string;
  modelName: string;
  timeline: TimelineEntry[];
  streaming: boolean;
  assistantWaiting: boolean;
  liveAssistant: string;
  blinkOn: boolean;
  compact: boolean;
  toolOutputExpanded: boolean;

  // Input
  inputValue: string;
  inputKey: number;

  // Menu
  menuKind: MenuKind | null;
  menuTitle: string;
  menuItems: MenuItem[];
  menuCursor: number;
  menuHint: string;

  // MCP Editor
  mcpEditorId: string;
  mcpEditorName: string;
  mcpEditorConfig: string;
  mcpEditorEnabled: boolean;
  mcpEditorFocusName: boolean;

  // Approval
  approvalToolCallId: string;
  approvalToolName: string;
  approvalCommand: string;
  approvalParams: Record<string, string>;
  approvalReplyCh: ((approved: boolean) => void) | null;

  // Connection
  apiURL: string;
  cliToken: string;
  version: string;
  cwd: string;
}

export type AppAction =
  | { type: "SET_VIEW"; view: ViewMode }
  | { type: "SET_INPUT"; value: string }
  | { type: "SET_INPUT_WITH_KEY"; value: string }
  | { type: "SET_SESSION"; sessionId: string; sessionName?: string }
  | { type: "SET_SESSION_NAME"; sessionName: string }
  | { type: "SET_MODEL"; modelId: string; modelName: string }
  | { type: "STREAM_START" }
  | { type: "STREAM_CHUNK"; chunk: string }
  | { type: "STREAM_DONE"; error: string | null }
  | { type: "TOGGLE_COMPACT" }
  | { type: "TOGGLE_TOOL_OUTPUT" }
  | { type: "UPSERT_TOOL_ENTRY"; entry: TimelineEntry }
  | { type: "APPEND_ENTRY"; entry: TimelineEntry }
  | { type: "RESET_SESSION" }
  | { type: "BLINK_TOGGLE" }
  | { type: "CLEAR_TIMELINE" }
  | { type: "SET_MENU"; kind: MenuKind; title: string; items: MenuItem[]; hint: string }
  | { type: "MENU_NAV"; delta: number }
  | { type: "CLOSE_MENU" }
  | { type: "SET_MCP_EDITOR"; id: string; name: string; config: string; enabled: boolean }
  | { type: "SET_MCP_EDITOR_NAME"; name: string }
  | { type: "SET_MCP_EDITOR_CONFIG"; config: string }
  | { type: "TOGGLE_MCP_EDITOR_ENABLED" }
  | { type: "TOGGLE_MCP_EDITOR_FOCUS" }
  | { type: "SET_APPROVAL"; toolCallId: string; toolName: string; command: string; params: Record<string, string>; replyCh: (approved: boolean) => void }
  | { type: "CLEAR_APPROVAL" }
  | { type: "LOAD_HISTORY"; entries: TimelineEntry[] };
