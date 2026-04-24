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
  approvalMode?: string;
  [key: string]: unknown;
}

// Thinking level cycle order
export const THINKING_LEVELS = ["off", "low", "medium", "high"] as const;
export type ThinkingLevel = (typeof THINKING_LEVELS)[number];

// ===== API response types =====

export interface SessionListResponse {
  sessions: Session[];
  hasMore: boolean;
}

export interface SessionHistoryPayload {
  messages: Message[];
  toolCallsByAssistantMessageId: Record<string, ToolCallHistoryItem[]>;
  thinkingByAssistantMessageId: Record<string, ThinkingHistoryItem[]>;
  hasMore: boolean;
}

export interface ToolCallHistoryItem {
  toolCallId: string;
  toolName: string;
  command: string;
  params: Record<string, string>;
  status: string;
  requiresApproval: boolean;
  parentToolCallId?: string;
  subagentRunId?: string;
  output?: string;
  error?: string;
  startedAt: string;
  finishedAt?: string;
}

export interface ThinkingHistoryItem {
  thinkingId: string;
  content: string;
  status: string;
  startedAt?: string;
  finishedAt?: string;
  durationMs?: number;
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
  parentToolCallId?: string;
  subagentRunId?: string;
}

export interface ToolCallResultData {
  toolCallId: string;
  toolName: string;
  command: string;
  requiresApproval: boolean;
  status: ToolCallStatus;
  output: string;
  error: string;
  parentToolCallId?: string;
  subagentRunId?: string;
}

export interface SubagentChunkData {
  parentToolCallId: string;
  subagentRunId: string;
  content: string;
}

// ===== UI state types =====

export type ViewMode = "chat" | "menu" | "mcp-editor" | "mcp-template" | "model-editor" | "approval" | "thinking-detail" | "plan-confirm";

export type MenuKind =
  | "session"
  | "model"
  | "skills"
  | "mcp"
  | "effort"
  | "help";

// ===== MCP Template types =====

export type MCPTransportKind = "stdio" | "sse" | "streamable_http";

export interface MCPTemplate {
  kind: MCPTransportKind;
  label: string;
  description: string;
  template: string;
}

export const MCP_TEMPLATES: MCPTemplate[] = [
  {
    kind: "stdio",
    label: "Stdio",
    description: "Local process via stdin/stdout (e.g. Python, Node)",
    template: '{\n  "command": "python",\n  "args": ["-m", "your_module"]\n}',
  },
  {
    kind: "sse",
    label: "SSE",
    description: "Server-Sent Events HTTP transport",
    template: '{\n  "transport": "sse",\n  "url": "https://your-mcp-server-url",\n  "headers": {},\n  "timeout": 5,\n  "sse_read_timeout": 300\n}',
  },
  {
    kind: "streamable_http",
    label: "Streamable HTTP",
    description: "Streamable HTTP transport (newest MCP protocol)",
    template: '{\n  "transport": "streamable_http",\n  "url": "https://your-mcp-server-url",\n  "headers": {},\n  "timeout": 5,\n  "sse_read_timeout": 300\n}',
  },
];

// ===== Model provider types =====

export type ModelProvider = "openai" | "anthropic";

export interface TimelineEntry {
  kind: "user" | "assistant" | "system" | "tool" | "thinking" | "plan";
  content: string;
  toolCallId?: string;
  toolName?: string;
  command?: string;
  params?: Record<string, string>;
  status?: ToolCallStatus;
  output?: string;
  error?: string;
  parentToolCallId?: string;
  subagentRunId?: string;
  /** Accumulated nested agent stream (parent run_subagent only). */
  subagentStream?: string;
  /** Thinking entry: whether thinking is complete. */
  thinkingDone?: boolean;
  /** Thinking entry: started timestamp (ms since epoch). */
  thinkingStartedAt?: number;
  /** Thinking entry: persisted duration for history replay. */
  thinkingDurationMs?: number;
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
  { command: "/mode", description: "Toggle approval mode (standard/auto)" },
  { command: "/effort", description: "Toggle thinking level (off/low/medium/high)" },
  { command: "/skills", description: "View and manage installed skills" },
  { command: "/mcp", description: "Manage MCP configurations" },
  { command: "/plan", description: "Toggle plan mode (on/off)" },
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
  thinkingLevel: string;
  approvalMode: string;
  timeline: TimelineEntry[];
  streaming: boolean;
  assistantWaiting: boolean;
  liveAssistant: string;
  blinkOn: boolean;
  compact: boolean;
  toolOutputExpanded: boolean;
  planMode: boolean;
  planGenerating: boolean;
  planReceived: boolean;

  // Thinking detail view
  thinkingDetailContent: string;

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

  // MCP Template Picker
  mcpTemplateCursor: number;

  // Model Editor
  modelEditorName: string;
  modelEditorProvider: ModelProvider;
  modelEditorBaseUrl: string;
  modelEditorApiKey: string;
  modelEditorModel: string;
  modelEditorFocusIndex: number;
  modelEditorProviderSelect: boolean;

  // Approval
  approvalToolCallId: string;
  approvalToolName: string;
  approvalCommand: string;
  approvalParams: Record<string, string>;
  approvalReplyCh: ((approved: boolean) => void) | null;

  // Plan confirmation
  pendingPlanId: string;
  pendingPlanContent: string;
  planConfirmCursor: number;
  planModifyInput: string;
  planModifyInputKey: number;

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
  | { type: "APPEND_SUBAGENT_STREAM"; parentToolCallId: string; content: string }
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
  | { type: "SET_MCP_TEMPLATE_VIEW" }
  | { type: "MCP_TEMPLATE_NAV"; delta: number }
  | { type: "SET_MODEL_EDITOR_VIEW" }
  | { type: "SET_MODEL_EDITOR_NAME"; name: string }
  | { type: "SET_MODEL_EDITOR_PROVIDER"; provider: ModelProvider }
  | { type: "SET_MODEL_EDITOR_BASE_URL"; baseUrl: string }
  | { type: "SET_MODEL_EDITOR_API_KEY"; apiKey: string }
  | { type: "SET_MODEL_EDITOR_MODEL"; model: string }
  | { type: "MODEL_EDITOR_NEXT_FIELD" }
  | { type: "MODEL_EDITOR_PREV_FIELD" }
  | { type: "TOGGLE_MODEL_EDITOR_PROVIDER_SELECT" }
  | { type: "SET_APPROVAL"; toolCallId: string; toolName: string; command: string; params: Record<string, string>; replyCh: (approved: boolean) => void }
  | { type: "CLEAR_APPROVAL" }
  | { type: "SET_APPROVAL_MODE"; mode: string }
  | { type: "SET_THINKING_LEVEL"; level: string }
  | { type: "LOAD_HISTORY"; entries: TimelineEntry[] }
  | { type: "THINKING_START" }
  | { type: "THINKING_CHUNK"; chunk: string }
  | { type: "THINKING_DONE"; finishedAt?: number }
  | { type: "TOGGLE_PLAN_MODE" }
  | { type: "SET_PLAN_CONFIRMATION"; planId: string; content: string }
  | { type: "PLAN_CONFIRM_NAV"; delta: number }
  | { type: "SET_PLAN_MODIFY_INPUT"; value: string }
  | { type: "CLEAR_PLAN_CONFIRMATION" }
  | { type: "PLAN_BODY"; planBody: string; narration?: string }
  | { type: "PLAN_START" }
  | { type: "VIEW_THINKING_DETAIL"; content: string }
  | { type: "FLUSH_AND_WAIT" };
