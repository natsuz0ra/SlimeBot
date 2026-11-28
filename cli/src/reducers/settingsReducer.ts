import type { AppAction, AppState, ModelProvider } from "../types.js";
import { CONTEXT_SIZE_DEFAULT } from "../utils/contextSize.js";

/** Model selection plus model/provider/MCP editor state. */
export function settingsReducer(state: AppState, action: AppAction): AppState | null {
  switch (action.type) {
    case "SET_MODEL":
      return { ...state, modelId: action.modelId, modelName: action.modelName };
    case "SET_MCP_EDITOR":
      return {
        ...state,
        view: "mcp-editor",
        mcpEditorId: action.id,
        mcpEditorName: action.name,
        mcpEditorConfig: action.config,
        mcpEditorEnabled: action.enabled,
        mcpEditorFocusName: true,
      };
    case "SET_MCP_EDITOR_NAME":
      return { ...state, mcpEditorName: action.name };
    case "SET_MCP_EDITOR_CONFIG":
      return { ...state, mcpEditorConfig: action.config };
    case "TOGGLE_MCP_EDITOR_ENABLED":
      return { ...state, mcpEditorEnabled: !state.mcpEditorEnabled };
    case "TOGGLE_MCP_EDITOR_FOCUS":
      return { ...state, mcpEditorFocusName: !state.mcpEditorFocusName };
    case "SET_MCP_TEMPLATE_VIEW":
      return { ...state, view: "mcp-template", mcpTemplateCursor: 0 };
    case "MCP_TEMPLATE_NAV":
      return { ...state, mcpTemplateCursor: Math.max(0, Math.min(2, state.mcpTemplateCursor + action.delta)) };
    case "SET_MCP_TOOLS_VIEW":
      return {
        ...state,
        view: "mcp-tools",
        mcpToolsConfig: action.config,
        mcpToolsResult: null,
        mcpToolsLoading: true,
        mcpToolsError: "",
      };
    case "SET_MCP_TOOLS_LOADING":
      return { ...state, mcpToolsLoading: action.loading };
    case "SET_MCP_TOOLS_RESULT":
      return {
        ...state,
        mcpToolsResult: action.result,
        mcpToolsLoading: false,
        mcpToolsError: action.error,
      };
    case "SET_MODEL_EDITOR_VIEW":
      return {
        ...state,
        view: "model-editor",
        modelEditorId: "",
        modelEditorProviderId: action.providerId || "",
        modelEditorName: "",
        modelEditorProvider: "openai" as ModelProvider,
        modelEditorBaseUrl: "",
        modelEditorApiKey: "",
        modelEditorModel: "",
        modelEditorContextSize: "",
        modelEditorContextSizeSource: "auto",
        modelEditorFocusIndex: 0,
        modelEditorProviderSelect: false,
      };
    case "SET_PROVIDER_EDITOR_VIEW":
      return {
        ...state,
        view: "provider-editor",
        modelEditorId: "",
        modelEditorProviderId: "",
        modelEditorName: "",
        modelEditorProvider: "openai" as ModelProvider,
        modelEditorBaseUrl: "",
        modelEditorApiKey: "",
        modelEditorFocusIndex: 0,
        modelEditorProviderSelect: false,
      };
    case "SET_PROVIDER_EDITOR":
      return {
        ...state,
        view: "provider-editor",
        modelEditorId: action.provider.id,
        modelEditorProviderId: "",
        modelEditorName: action.provider.name,
        modelEditorProvider: action.provider.protocol,
        modelEditorBaseUrl: action.provider.baseUrl,
        modelEditorApiKey: "",
        modelEditorFocusIndex: 0,
        modelEditorProviderSelect: false,
      };
    case "SET_MODEL_EDITOR":
      return {
        ...state,
        view: "model-editor",
        modelEditorId: action.config.id,
        modelEditorProviderId: action.config.providerId,
        modelEditorName: action.config.name,
        modelEditorModel: action.config.model,
        modelEditorContextSize: String(action.config.contextSize || CONTEXT_SIZE_DEFAULT),
        modelEditorContextSizeSource: action.config.contextSizeSource || "manual",
        modelEditorFocusIndex: 0,
        modelEditorProviderSelect: false,
      };
    case "SET_MODEL_EDITOR_NAME":
      return { ...state, modelEditorName: action.name };
    case "SET_MODEL_EDITOR_PROVIDER":
      return { ...state, modelEditorProvider: action.provider };
    case "SET_MODEL_EDITOR_BASE_URL":
      return { ...state, modelEditorBaseUrl: action.baseUrl };
    case "SET_MODEL_EDITOR_API_KEY":
      return { ...state, modelEditorApiKey: action.apiKey };
    case "SET_MODEL_EDITOR_MODEL":
      return {
        ...state,
        modelEditorModel: action.model,
        ...(state.modelEditorContextSizeSource === "manual" ? {} : { modelEditorContextSize: "", modelEditorContextSizeSource: "auto" as const }),
      };
    case "SET_MODEL_EDITOR_CONTEXT_SIZE":
      return { ...state, modelEditorContextSize: action.contextSize, modelEditorContextSizeSource: "manual" };
    case "SET_MODEL_EDITOR_CONTEXT_AUTO":
      return { ...state, modelEditorContextSize: "", modelEditorContextSizeSource: "auto" };
    case "MODEL_EDITOR_NEXT_FIELD": {
      const maxIndex = state.view === "provider-editor" ? 3 : 2;
      const next = state.modelEditorFocusIndex >= maxIndex ? 0 : state.modelEditorFocusIndex + 1;
      return { ...state, modelEditorFocusIndex: next, modelEditorProviderSelect: false };
    }
    case "MODEL_EDITOR_PREV_FIELD": {
      const maxIndex = state.view === "provider-editor" ? 3 : 2;
      const previous = state.modelEditorFocusIndex <= 0 ? maxIndex : state.modelEditorFocusIndex - 1;
      return { ...state, modelEditorFocusIndex: previous, modelEditorProviderSelect: false };
    }
    case "TOGGLE_MODEL_EDITOR_PROVIDER_SELECT":
      return { ...state, modelEditorProviderSelect: !state.modelEditorProviderSelect };
    case "SET_APPROVAL_MODE":
      return { ...state, approvalMode: action.mode };
    case "SET_THINKING_LEVEL":
      return { ...state, thinkingLevel: action.level };
    case "SET_SUBAGENT_MODEL":
      return { ...state, subagentModelId: action.modelId, subagentModelName: action.modelName };
    default:
      return null;
  }
}
