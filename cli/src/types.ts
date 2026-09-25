import type { ApprovalMode, QAAnswer, QAQuestion } from "./types/uiTypes.js";
import type { MemoryConsoleEditField, MemoryConsoleMode } from "./utils/memoryConsole.js";

export type { ApprovalMode, QAAnswer, QAQuestion } from "./types/uiTypes.js";

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
  providerId: string;
  providerName?: string;
  provider: string;
  baseUrl: string;
  model: string;
  contextSize?: number;
  createdAt: string;
  updatedAt: string;
}

export interface LLMProvider {
  id: string;
  name: string;
  protocol: ModelProvider;
  baseUrl: string;
  hasApiKey: boolean;
}

export interface DiscoveredModel {
  id: string;
  name: string;
  contextSize?: number;
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
  source?: string;
  sourceLabel?: string;
  provider?: string;
  readOnly?: boolean;
  enabled?: boolean;
  absolutePath?: string;
}

export interface Settings {
  defaultModel: string;
  approvalMode?: string;
  memoryEnabled?: boolean;
  memoryUserProfileEnabled?: boolean;
  memoryCharLimit?: number;
  memoryUserCharLimit?: number;
  memoryNudgeInterval?: number;
  sandboxMode?: string;
  sandboxWritableRoots?: string[];
  sandboxNetworkEnabled?: boolean;
  sandboxNetworkAllowedDomains?: string[];
  cliSandboxMode?: string;
  cliSandboxWritableRoots?: string[];
  cliSandboxNetworkEnabled?: boolean;
  cliSandboxNetworkAllowedDomains?: string[];
  [key: string]: unknown;
}

export type MemoryTarget = "memory" | "user";

export interface MemoryTargetState {
  target: MemoryTarget;
  entries: string[];
  usageChars: number;
  charLimit: number;
  entryCount: number;
  enabled: boolean;
}

export interface MemorySnapshot {
  memory: MemoryTargetState;
  user: MemoryTargetState;
  memoryEnabled: boolean;
  memoryUserProfileEnabled: boolean;
  memoryNudgeInterval: number;
  memoryDirectory: string;
}

export type UpdatePhase =
  | "idle"
  | "checking"
  | "downloading"
  | "installing"
  | "restarting"
  | "succeeded"
  | "failed";

export interface UpdateCheckResult {
  current: string;
  latest: string;
  updateAvailable: boolean;
  canApply: boolean;
  reason?: string;
  releaseName?: string;
  releaseNotes?: string;
  releaseUrl?: string;
  publishedAt?: string;
  assetName?: string;
  manualHint?: string;
}

export interface UpdateJobStatus {
  phase: UpdatePhase;
  current?: string;
  target?: string;
  message?: string;
  error?: string;
  manualHint?: string;
  downloadedBytes?: number;
  totalBytes?: number;
  progressPercent?: number;
  updatedAt?: string;
}

// Thinking level cycle order
export const THINKING_LEVELS = ["off", "low", "medium", "high", "max"] as const;
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
  teamRuns?: Array<Omit<AgentTeamRun, "members"> & { members?: AgentTeamMemberRun[] }>;
  teamMemberRuns?: AgentTeamMemberRun[];
  hasMore: boolean;
}

export type AgentTeamStatus = "running" | "succeeded" | "partial_failed" | "failed" | "canceled" | "interrupted";
export type AgentTeamMemberStatus = "queued" | "running" | "succeeded" | "failed" | "canceled" | "interrupted";

export interface AgentTeamMemberRun {
  id: string;
  teamRunId: string;
  toolCallId: string;
  subagentRunId?: string;
  title: string;
  task: string;
  modelConfigId?: string;
  status: AgentTeamMemberStatus;
  answer?: string;
  error?: string;
  startedAt?: string;
  finishedAt?: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface AgentTeamRun {
  id: string;
  sessionId: string;
  requestId: string;
  assistantMessageId?: string;
  status: AgentTeamStatus;
  maxMembers: number;
  maxParallel: number;
  lastError?: string;
  startedAt: string;
  finishedAt?: string;
  createdAt?: string;
  updatedAt?: string;
  members: AgentTeamMemberRun[];
}

export interface ToolCallHistoryItem {
  toolCallId: string;
  toolName: string;
  command: string;
  params: Record<string, unknown>;
  status: string;
  requiresApproval: boolean;
  parentToolCallId?: string;
  subagentRunId?: string;
  teamRunId?: string;
  memberRunId?: string;
  metadata?: unknown;
  subagentTitle?: string;
  subagentTask?: string;
  output?: string;
  error?: string;
  startedAt: string;
  finishedAt?: string;
}

export interface ThinkingHistoryItem {
  thinkingId: string;
  parentToolCallId?: string;
  subagentRunId?: string;
  teamRunId?: string;
  memberRunId?: string;
  content: string;
  status: string;
  startedAt?: string;
  finishedAt?: string;
  durationMs?: number;
}

// ===== WebSocket message types =====

export type ToolCallStatus =
  | "pending"
  | "reviewing"
  | "rejected"
  | "executing"
  | "completed"
  | "error";

export interface ToolCallStartData {
  toolCallId: string;
  toolName: string;
  command: string;
  params: Record<string, unknown>;
  requiresApproval: boolean;
  reviewStatus?: string;
  reviewRisk?: string;
  reviewReason?: string;
  preamble?: string;
  parentToolCallId?: string;
  subagentRunId?: string;
  teamRunId?: string;
  memberRunId?: string;
}

export interface ToolCallReviewData {
  toolCallId: string;
  toolName: string;
  command: string;
  reviewStatus: string;
  reviewRisk?: string;
  reviewReason?: string;
  parentToolCallId?: string;
  subagentRunId?: string;
  teamRunId?: string;
  memberRunId?: string;
}

export interface ToolApprovalRequiredData extends ToolCallReviewData {
  params: Record<string, unknown>;
  requiresApproval: boolean;
}

export interface ToolCallResultData {
  toolCallId: string;
  toolName: string;
  command: string;
  requiresApproval: boolean;
  status: ToolCallStatus;
  output: string;
  error: string;
  metadata?: unknown;
  parentToolCallId?: string;
  subagentRunId?: string;
  teamRunId?: string;
  memberRunId?: string;
}

export interface SubagentChunkData {
  parentToolCallId: string;
  subagentRunId: string;
  content: string;
  teamRunId?: string;
  memberRunId?: string;
}

export interface SubagentStartData {
  parentToolCallId: string;
  subagentRunId: string;
  title: string;
  task: string;
  teamRunId?: string;
  memberRunId?: string;
}

export interface SubagentDoneData {
  parentToolCallId: string;
  subagentRunId: string;
  error?: string;
  teamRunId?: string;
  memberRunId?: string;
}

export type TodoItemStatus = "pending" | "in_progress" | "completed";

export interface RuntimeTodoItem {
  id: string;
  content: string;
  status: TodoItemStatus;
}

export interface TodoUpdateData {
  items: RuntimeTodoItem[];
  note?: string;
  updatedAt?: string;
}

export interface ContextUsage {
  sessionId: string;
  modelConfigId: string;
  usedTokens: number;
  totalTokens: number;
  usedPercent: number;
  availablePercent: number;
  isCompacted: boolean;
  compactedAt?: string;
}

// ===== UI state types =====

export type ViewMode = "chat" | "menu" | "mcp-editor" | "mcp-template" | "mcp-tools" | "model-editor" | "provider-editor" | "approval" | "thinking-detail" | "plan-confirm" | "question-answer" | "update" | "memory-console" | "team-detail";

export type MenuKind =
  | "session"
  | "model"
  | "provider"
  | "provider-model"
  | "discovered-model"
  | "skills"
  | "mcp"
  | "effort"
  | "subagent_model"
  | "sandbox"
  | "update"
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

export type ModelProvider = "openai" | "anthropic" | "deepseek";

export interface TimelineEntry {
  kind: "user" | "assistant" | "system" | "tool" | "thinking" | "plan" | "team";
  content: string;
  toolCallId?: string;
  toolName?: string;
  command?: string;
  params?: Record<string, unknown>;
  status?: ToolCallStatus;
  output?: string;
  error?: string;
  metadata?: unknown;
  parentToolCallId?: string;
  subagentRunId?: string;
  teamRunId?: string;
  memberRunId?: string;
  teamRun?: AgentTeamRun;
  subagentTitle?: string;
  subagentTask?: string;
  /** Accumulated nested agent stream (parent run_subagent only). */
  subagentStream?: string;
  /** Thinking generated by a nested sub-agent, scoped to the parent run_subagent tool. */
  subagentThinking?: {
    content: string;
    thinkingDone?: boolean;
    thinkingStartedAt?: number;
    thinkingDurationMs?: number;
  };
  /** Thinking entry: whether thinking is complete. */
  thinkingDone?: boolean;
  /** Thinking entry: started timestamp (ms since epoch). */
  thinkingStartedAt?: number;
  /** Thinking entry: persisted duration for history replay. */
  thinkingDurationMs?: number;
}

export interface PendingApprovalItem {
  toolCallId: string;
  toolName: string;
  command: string;
  params: Record<string, unknown>;
}

export type ApprovalReviewStatus = "pending" | "approved" | "rejected";

export interface ApprovalReviewItem extends PendingApprovalItem {
  approvalStatus: ApprovalReviewStatus;
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

export type MCPToolLoadStatus = "loaded" | "error" | "disabled";

export interface MCPToolParameterSummary {
  name: string;
  type: string;
  required: boolean;
  description: string;
}

export interface MCPToolItem {
  name: string;
  functionName: string;
  description: string;
  parameterCount: number;
  requiredParameters: string[];
  parameters: MCPToolParameterSummary[];
  inputSchema: Record<string, unknown>;
}

export interface MCPToolListResponse {
  configId: string;
  name: string;
  isEnabled: boolean;
  status: MCPToolLoadStatus;
  toolCount: number;
  loadedAt: string;
  tools: MCPToolItem[];
  error: string;
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
  { command: "/provider", description: "Manage model providers and available models" },
  { command: "/memory", description: "Open memory console" },
  { command: "/team", description: "Open Agent Team details" },
  { command: "/subagent_model", description: "Choose sub-agent model" },
  { command: "/approval", description: "Toggle approval mode (standard/auto review/auto)" },
  { command: "/effort", description: "Toggle thinking level (off/low/medium/high)" },
  { command: "/sandbox", description: "Configure sandbox mode and network access" },
  { command: "/update", description: "Check and apply SlimeBot updates" },
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
  subagentModelId: string;
  subagentModelName: string;
  thinkingLevel: ThinkingLevel;
  approvalMode: ApprovalMode;
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
  turnStartedAt?: number;
  turnElapsedMs: number;
  turnTokenEstimate: number;
  turnThoughtDurationMs?: number;
  contextUsage: ContextUsage | null;
  runtimeTodos: RuntimeTodoItem[];
  runtimeTodosNote: string;
  runtimeTodosUpdatedAt?: number;
  updateCheck: UpdateCheckResult | null;
  updateJob: UpdateJobStatus | null;
  updateLoading: boolean;
  updateApplying: boolean;
  updateConfirming: boolean;

  // Memory console
  memorySnapshot: MemorySnapshot | null;
  memoryLoading: boolean;
  memoryCursor: number;
  memoryMode: MemoryConsoleMode;
  memoryEditingField: MemoryConsoleEditField | null;
  memoryDraft: string;
  memoryViewTarget: MemoryTarget | null;
  memoryMessage: string;

  // Agent Team detail
  teamRunCursor: number;
  teamMemberCursor: number;

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

  // MCP Tools view
  mcpToolsConfig: MCPConfig | null;
  mcpToolsResult: MCPToolListResponse | null;
  mcpToolsLoading: boolean;
  mcpToolsError: string;

  // MCP Template Picker
  mcpTemplateCursor: number;

  // Model Editor
  modelEditorId: string;
  modelEditorProviderId: string;
  modelEditorName: string;
  modelEditorProvider: ModelProvider;
  modelEditorBaseUrl: string;
  modelEditorApiKey: string;
  modelEditorModel: string;
  modelEditorContextSize: string;
  modelEditorFocusIndex: number;
  modelEditorProviderSelect: boolean;

  // Approval
  approvalToolCallId: string;
  approvalToolName: string;
  approvalCommand: string;
  approvalParams: Record<string, unknown>;
  approvalReplyCh: ((approved: boolean) => void) | null;
  pendingApprovals: PendingApprovalItem[];
  approvalReviewItems: ApprovalReviewItem[];
  approvalCursor: number;
  markedApprovalIds: string[];

  // Plan confirmation
  pendingPlanId: string;
  pendingPlanContent: string;
  planConfirmCursor: number;
  planModifyInput: string;
  planModifyInputKey: number;

  // Question-answer
  qaToolCallId: string;
  qaQuestions: QAQuestion[];
  qaCurrentIndex: number;
  qaAnswers: QAAnswer[];
  qaStep: "questions" | "confirm";
  qaCursor: number;
  qaCustomInput: string;
  qaCustomInputKey: number;
  qaEditingFromConfirm: boolean;

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
  | { type: "APPLY_SESSION_TITLE"; sessionId?: string; title: string }
  | { type: "SET_MODEL"; modelId: string; modelName: string }
  | { type: "STREAM_START"; startedAt?: number }
  | { type: "STREAM_CHUNK"; chunk: string }
  | { type: "STREAM_DONE"; error: string | null }
  | { type: "CONTEXT_USAGE"; usage: ContextUsage }
  | { type: "CONTEXT_COMPACTED"; usage: ContextUsage }
  | { type: "TURN_STATS_TICK"; now?: number }
  | { type: "TOGGLE_COMPACT" }
  | { type: "TOGGLE_TOOL_OUTPUT" }
  | { type: "UPSERT_TOOL_ENTRY"; entry: TimelineEntry }
  | { type: "UPSERT_TEAM_RUN"; run: Omit<AgentTeamRun, "members"> & { members?: AgentTeamMemberRun[] } }
  | { type: "UPSERT_TEAM_MEMBER"; member: AgentTeamMemberRun }
  | { type: "OPEN_TEAM_DETAIL" }
  | { type: "TEAM_DETAIL_NAV_TEAM"; delta: number }
  | { type: "TEAM_DETAIL_NAV_MEMBER"; delta: number }
  | { type: "APPEND_SUBAGENT_STREAM"; parentToolCallId: string; content: string }
  | { type: "SUBAGENT_DONE"; parentToolCallId: string; error?: string; finishedAt?: number }
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
  | { type: "SET_MCP_TOOLS_VIEW"; config: MCPConfig }
  | { type: "SET_MCP_TOOLS_LOADING"; loading: boolean }
  | { type: "SET_MCP_TOOLS_RESULT"; result: MCPToolListResponse | null; error: string }
  | { type: "SET_MODEL_EDITOR_VIEW"; providerId?: string }
  | { type: "SET_PROVIDER_EDITOR_VIEW" }
  | { type: "SET_PROVIDER_EDITOR"; provider: LLMProvider }
  | { type: "SET_MODEL_EDITOR"; config: LLMConfig }
  | { type: "SET_MODEL_EDITOR_NAME"; name: string }
  | { type: "SET_MODEL_EDITOR_PROVIDER"; provider: ModelProvider }
  | { type: "SET_MODEL_EDITOR_BASE_URL"; baseUrl: string }
  | { type: "SET_MODEL_EDITOR_API_KEY"; apiKey: string }
  | { type: "SET_MODEL_EDITOR_MODEL"; model: string }
  | { type: "SET_MODEL_EDITOR_CONTEXT_SIZE"; contextSize: string }
  | { type: "MODEL_EDITOR_NEXT_FIELD" }
  | { type: "MODEL_EDITOR_PREV_FIELD" }
  | { type: "TOGGLE_MODEL_EDITOR_PROVIDER_SELECT" }
  | { type: "SET_APPROVAL"; toolCallId: string; toolName: string; command: string; params: Record<string, unknown>; replyCh: (approved: boolean) => void }
  | { type: "CLEAR_APPROVAL" }
  | { type: "ADD_PENDING_APPROVAL"; item: PendingApprovalItem }
  | { type: "REMOVE_PENDING_APPROVAL"; toolCallId: string; approved?: boolean }
  | { type: "CLEAR_PENDING_APPROVALS" }
  | { type: "APPROVAL_NAV"; delta: number }
  | { type: "TOGGLE_APPROVAL_MARK"; toolCallId: string }
  | { type: "SET_APPROVAL_MODE"; mode: ApprovalMode }
  | { type: "SET_THINKING_LEVEL"; level: ThinkingLevel }
  | { type: "SET_SUBAGENT_MODEL"; modelId: string; modelName: string }
  | { type: "LOAD_HISTORY"; entries: TimelineEntry[] }
  | { type: "THINKING_START"; parentToolCallId?: string; subagentRunId?: string; startedAt?: number }
  | { type: "THINKING_CHUNK"; chunk: string; parentToolCallId?: string; subagentRunId?: string }
  | { type: "THINKING_DONE"; finishedAt?: number; parentToolCallId?: string; subagentRunId?: string }
  | { type: "TOGGLE_PLAN_MODE" }
  | { type: "SET_PLAN_CONFIRMATION"; planId: string; content: string }
  | { type: "PLAN_CONFIRM_NAV"; delta: number }
  | { type: "SET_PLAN_MODIFY_INPUT"; value: string }
  | { type: "CLEAR_PLAN_CONFIRMATION" }
  | { type: "PLAN_CHUNK"; chunk: string }
  | { type: "PLAN_BODY"; planBody: string; narration?: string }
  | { type: "PLAN_START" }
  | { type: "TODO_UPDATE"; items: RuntimeTodoItem[]; note?: string; updatedAt?: number }
  | { type: "SET_UPDATE_STATE"; check?: UpdateCheckResult | null; job?: UpdateJobStatus | null; loading?: boolean; applying?: boolean; confirming?: boolean }
  | { type: "SET_MEMORY_CONSOLE"; snapshot?: MemorySnapshot | null; loading?: boolean; message?: string }
  | { type: "MEMORY_CONSOLE_NAV"; delta: number }
  | { type: "MEMORY_CONSOLE_SET_MODE"; mode: MemoryConsoleMode; cursor?: number; message?: string }
  | { type: "MEMORY_CONSOLE_START_EDIT"; field: MemoryConsoleEditField; draft: string }
  | { type: "MEMORY_CONSOLE_SET_DRAFT"; draft: string }
  | { type: "MEMORY_CONSOLE_VIEW_TARGET"; target: MemoryTarget }
  | { type: "MEMORY_CONSOLE_MESSAGE"; message: string }
  | { type: "VIEW_THINKING_DETAIL"; content: string }
  | { type: "SET_QA"; toolCallId: string; questions: QAQuestion[] }
  | { type: "QA_NAV"; delta: number }
  | { type: "QA_NAV_TO"; cursor: number }
  | { type: "QA_SELECT"; optionIndex: number }
  | { type: "QA_SET_CUSTOM_INPUT"; value: string }
  | { type: "QA_SUBMIT_CUSTOM"; value: string }
  | { type: "QA_NEXT_QUESTION" }
  | { type: "QA_PREV_QUESTION" }
  | { type: "QA_EDIT_QUESTION"; index: number }
  | { type: "QA_STEP_CONFIRM" }
  | { type: "QA_STEP_BACK" }
  | { type: "QA_CONFIRM_NAV"; delta: number }
  | { type: "CLEAR_QA" }
  | { type: "FLUSH_AND_WAIT" };
