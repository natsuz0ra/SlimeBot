import type { AppAction, AppState, MenuKind, ViewMode } from "../types.js";

/** Input, menu, and view-local navigation state. Unhandled actions belong to another slice. */
export function navigationReducer(state: AppState, action: AppAction): AppState | null {
  switch (action.type) {
    case "SET_VIEW":
      return { ...state, view: action.view as ViewMode };
    case "SET_INPUT":
      return { ...state, inputValue: action.value };
    case "SET_INPUT_WITH_KEY":
      return { ...state, inputValue: action.value, inputKey: state.inputKey + 1 };
    case "SET_SESSION":
      return {
        ...state,
        sessionId: action.sessionId,
        sessionName: action.sessionName !== undefined
          ? action.sessionName
          : (action.sessionId !== state.sessionId ? "" : state.sessionName),
        contextUsage: action.sessionId !== state.sessionId ? null : state.contextUsage,
      };
    case "SET_SESSION_NAME":
      return { ...state, sessionName: action.sessionName };
    case "APPLY_SESSION_TITLE":
      if (!action.sessionId || action.sessionId !== state.sessionId || !action.title.trim()) return state;
      return { ...state, sessionName: action.title };
    case "TOGGLE_COMPACT":
      return { ...state, compact: !state.compact };
    case "TOGGLE_TOOL_OUTPUT":
      return { ...state, toolOutputExpanded: !state.toolOutputExpanded };
    case "BLINK_TOGGLE":
      return { ...state, blinkOn: !state.blinkOn };
    case "SET_MENU":
      return {
        ...state,
        view: "menu",
        menuKind: action.kind as MenuKind,
        menuTitle: action.title,
        menuItems: action.items,
        menuCursor: 0,
        menuHint: action.hint,
      };
    case "MENU_NAV":
      return {
        ...state,
        menuCursor: Math.max(0, Math.min(state.menuItems.length - 1, state.menuCursor + action.delta)),
      };
    case "CLOSE_MENU":
      return {
        ...state,
        view: "chat",
        menuKind: null,
        menuTitle: "",
        menuItems: [],
        menuCursor: 0,
      };
    default:
      return null;
  }
}
