import type { AppAction, AppState, SubagentThinking, TimelineEntry } from "../types.js";
import { estimateTokens } from "../utils/format.js";
import {
  clampAgentTeamCursor,
  getAgentTeamRuns,
  upsertAgentTeamMember,
  upsertAgentTeamRun,
} from "../utils/agentTeam.js";
import { getSubagentThinkings, withSubagentThinkings } from "../utils/subagentThinking.js";

const SOCKET_INTERRUPTION = "WebSocket connection interrupted.";

function clearTurnStats() {
  return { turnStartedAt: undefined, turnElapsedMs: 0, turnTokenEstimate: 0, turnThoughtDurationMs: undefined };
}

function clearRuntimeTodos() {
  return { runtimeTodos: [], runtimeTodosNote: "", runtimeTodosUpdatedAt: undefined };
}

function clearPendingApprovals() {
  return {
    pendingApprovals: [],
    approvalReviewItems: [],
    approvalCursor: 0,
    markedApprovalIds: [],
    approvalToolCallId: "",
    approvalToolName: "",
    approvalCommand: "",
    approvalParams: {},
    approvalReplyCh: null,
  };
}

function clearQuestionAnswer() {
  return {
    qaToolCallId: "",
    qaQuestions: [],
    qaCurrentIndex: 0,
    qaAnswers: [],
    qaStep: "questions" as const,
    qaCursor: 0,
    qaCustomInput: "",
    qaEditingFromConfirm: false,
  };
}

function entryTokenText(entry: TimelineEntry): string {
  return [
    entry.content,
    entry.output,
    entry.error,
    entry.subagentStream,
    ...getSubagentThinkings(entry).map(thinking => thinking.content),
    entry.toolName,
    entry.command,
    entry.teamRun?.lastError,
    ...(entry.teamRun?.members.flatMap(member => [member.title, member.task, member.answer, member.error]) || []),
    ...(entry.params ? Object.values(entry.params) : []),
  ].filter(Boolean).join("\n");
}

function estimateTimelineTokens(entries: TimelineEntry[], liveAssistant = ""): number {
  return estimateTokens([...entries.map(entryTokenText), liveAssistant].filter(Boolean).join("\n"));
}

function estimateTurnTokens(state: AppState, entries = state.timeline, liveAssistant = state.liveAssistant): number {
  return estimateTimelineTokens(entries, liveAssistant);
}

function addTokenEstimate(state: AppState, chunk: string): number {
  return Math.max(0, state.turnTokenEstimate + estimateTokens(chunk));
}

function restoreQuestionCursor(state: AppState, index: number): Pick<AppState, "qaCurrentIndex" | "qaCursor" | "qaCustomInput"> {
  const safeIndex = Math.max(0, Math.min(index, Math.max(0, state.qaQuestions.length - 1)));
  const question = state.qaQuestions[safeIndex];
  const answer = state.qaAnswers[safeIndex];
  const customCursor = question?.options.length ?? 0;
  const selectedOption = answer?.selectedOption ?? -2;
  return {
    qaCurrentIndex: safeIndex,
    qaCursor: selectedOption >= 0 ? Math.min(selectedOption, Math.max(0, customCursor)) : customCursor,
    qaCustomInput: selectedOption === -1 ? answer?.customAnswer ?? "" : "",
  };
}

function isOpenToolStatus(status?: string): boolean {
  return status === "pending" || status === "reviewing" || status === "executing";
}

function finishSubagentSegment(thinking: SubagentThinking, finishedAt: number): SubagentThinking {
  if (thinking.thinkingDone) return thinking;
  const startedAt = thinking.thinkingStartedAt;
  return {
    ...thinking,
    thinkingDone: true,
    thinkingDurationMs: startedAt !== undefined ? Math.max(0, finishedAt - startedAt) : thinking.thinkingDurationMs,
  };
}

function finishThinkingEntry(entry: TimelineEntry, finishedAt: number): TimelineEntry {
  if (entry.kind === "thinking" && !entry.thinkingDone) {
    const startedAt = entry.thinkingStartedAt;
    return {
      ...entry,
      thinkingDone: true,
      thinkingDurationMs: startedAt !== undefined ? Math.max(0, finishedAt - startedAt) : entry.thinkingDurationMs,
    };
  }
  if (entry.kind === "tool") {
    const segments = getSubagentThinkings(entry);
    if (segments.some(thinking => !thinking.thinkingDone)) {
      return withSubagentThinkings(entry, segments.map(thinking => finishSubagentSegment(thinking, finishedAt)));
    }
  }
  return entry;
}

function finalizeOpenRuntimeEntries(entries: TimelineEntry[], error = "Execution cancelled.", finishedAt = Date.now()): TimelineEntry[] {
  return entries.map(entry => {
    const finished = finishThinkingEntry(entry, finishedAt);
    if (finished.kind !== "tool" || !isOpenToolStatus(finished.status)) return finished;
    return { ...finished, status: "error", error: finished.error || error, content: finished.content || finished.error || error };
  });
}

function finalizeSubagentEntry(entries: TimelineEntry[], parentToolCallId: string, error?: string, finishedAt = Date.now()): TimelineEntry[] {
  return entries.map(entry => {
    if (entry.kind !== "tool" || entry.toolCallId !== parentToolCallId) return entry;
    const finished = finishThinkingEntry(entry, finishedAt);
    if (!error || !isOpenToolStatus(finished.status)) return finished;
    return { ...finished, status: "error", error, content: finished.content || error };
  });
}

/** Streaming chat, tools, subagents, approvals, Q&A, plans, and Team state. */
export function chatReducer(state: AppState, action: AppAction): AppState | null {
  switch (action.type) {
    case "STREAM_START":
      return {
        ...state,
        streaming: true,
        assistantWaiting: true,
        liveAssistant: "",
        planReceived: false,
        turnStartedAt: action.startedAt ?? Date.now(),
        turnElapsedMs: 0,
        turnTokenEstimate: estimateTurnTokens(state, state.timeline, ""),
        turnThoughtDurationMs: undefined,
        ...clearRuntimeTodos(),
      };
    case "STREAM_CHUNK": {
      const liveAssistant = state.liveAssistant + action.chunk;
      return {
        ...state,
        streaming: true,
        assistantWaiting: false,
        liveAssistant,
        turnTokenEstimate: addTokenEstimate(state, action.chunk),
      };
    }
    case "CONTEXT_USAGE":
      return { ...state, contextUsage: { ...action.usage } };
    case "CONTEXT_COMPACTED":
      return {
        ...state,
        contextUsage: { ...action.usage, isCompacted: true },
        timeline: [...state.timeline, { kind: "system", content: "Context compacted; continuing with summary + recent messages." }],
      };
    case "SOCKET_DISCONNECTED":
    case "STREAM_DONE": {
      const disconnected = action.type === "SOCKET_DISCONNECTED";
      const error = disconnected ? SOCKET_INTERRUPTION : action.error;
      const entries = finalizeOpenRuntimeEntries(state.timeline, error || "Execution cancelled.");
      // PLAN_BODY already flushed narration and cleared liveAssistant. Any remaining text
      // therefore belongs to the post-plan execution response, not to the plan itself.
      if (state.liveAssistant.trim()) entries.push({ kind: "assistant", content: state.liveAssistant });
      if (error) entries.push({ kind: "system", content: `Error: ${error}` });
      return {
        ...state,
        streaming: false,
        assistantWaiting: false,
        liveAssistant: "",
        timeline: entries,
        planGenerating: false,
        planReceived: false,
        ...clearTurnStats(),
        ...clearRuntimeTodos(),
        ...(disconnected ? {
          ...clearPendingApprovals(),
          ...clearQuestionAnswer(),
          view: state.view === "approval" || state.view === "question-answer" ? "chat" : state.view,
        } : {}),
      };
    }
    case "TURN_STATS_TICK": {
      if (!state.streaming || state.turnStartedAt === undefined) return state;
      return { ...state, turnElapsedMs: Math.max(0, (action.now ?? Date.now()) - state.turnStartedAt) };
    }
    case "UPSERT_TOOL_ENTRY": {
      const nextEntry: TimelineEntry = { ...action.entry, kind: "tool" };
      if (!nextEntry.toolCallId) {
        const timeline = [...state.timeline, nextEntry];
        return { ...state, timeline, turnTokenEstimate: state.streaming ? estimateTurnTokens(state, timeline) : state.turnTokenEstimate };
      }
      const entries = [...state.timeline];
      const index = entries.findIndex(entry => entry.kind === "tool" && entry.toolCallId === nextEntry.toolCallId);
      if (index === -1) entries.push(nextEntry);
      else {
        const previous = entries[index];
        entries[index] = {
          ...previous,
          ...nextEntry,
          subagentStream: nextEntry.subagentStream ?? previous.subagentStream,
          subagentThinkings: nextEntry.subagentThinkings ?? previous.subagentThinkings,
          subagentThinking: nextEntry.subagentThinking ?? previous.subagentThinking,
        };
      }
      return { ...state, timeline: entries, turnTokenEstimate: state.streaming ? estimateTurnTokens(state, entries) : state.turnTokenEstimate };
    }
    case "UPSERT_TEAM_RUN": {
      const timeline = upsertAgentTeamRun(state.timeline, action.run);
      return { ...state, timeline, turnTokenEstimate: state.streaming ? estimateTurnTokens(state, timeline) : state.turnTokenEstimate };
    }
    case "UPSERT_TEAM_MEMBER": {
      const timeline = upsertAgentTeamMember(state.timeline, action.member);
      return { ...state, timeline, turnTokenEstimate: state.streaming ? estimateTurnTokens(state, timeline) : state.turnTokenEstimate };
    }
    case "OPEN_TEAM_DETAIL": {
      const runs = getAgentTeamRuns(state.timeline);
      return {
        ...state,
        view: "team-detail",
        teamRunCursor: clampAgentTeamCursor(runs.length - 1, runs.length),
        teamMemberCursor: 0,
      };
    }
    case "TEAM_DETAIL_NAV_TEAM": {
      const runs = getAgentTeamRuns(state.timeline);
      return {
        ...state,
        teamRunCursor: clampAgentTeamCursor(state.teamRunCursor + action.delta, runs.length),
        teamMemberCursor: 0,
      };
    }
    case "TEAM_DETAIL_NAV_MEMBER": {
      const runs = getAgentTeamRuns(state.timeline);
      const teamRunCursor = clampAgentTeamCursor(state.teamRunCursor, runs.length);
      const memberCount = runs[teamRunCursor]?.members.length || 0;
      return {
        ...state,
        teamRunCursor,
        teamMemberCursor: clampAgentTeamCursor(state.teamMemberCursor + action.delta, memberCount),
      };
    }
    case "APPEND_SUBAGENT_STREAM": {
      const entries = [...state.timeline];
      const index = entries.findIndex(entry => entry.kind === "tool" && entry.toolCallId === action.parentToolCallId);
      if (index === -1) return state;
      const previous = entries[index];
      entries[index] = { ...previous, subagentStream: (previous.subagentStream || "") + action.content };
      return {
        ...state,
        timeline: entries,
        turnTokenEstimate: state.streaming ? addTokenEstimate(state, action.content) : state.turnTokenEstimate,
      };
    }
    case "SUBAGENT_DONE": {
      const entries = finalizeSubagentEntry(state.timeline, action.parentToolCallId, action.error, action.finishedAt ?? Date.now());
      return { ...state, timeline: entries, turnTokenEstimate: state.streaming ? estimateTurnTokens(state, entries) : state.turnTokenEstimate };
    }
    case "APPEND_ENTRY":
      return { ...state, timeline: [...state.timeline, { ...action.entry } as TimelineEntry] };
    case "RESET_SESSION":
      return {
        ...state,
        sessionId: "",
        sessionName: "",
        timeline: [],
        liveAssistant: "",
        streaming: false,
        assistantWaiting: false,
        toolOutputExpanded: false,
        planMode: false,
        planGenerating: false,
        planReceived: false,
        ...clearTurnStats(),
        ...clearRuntimeTodos(),
        updateLoading: false,
        updateApplying: false,
        updateConfirming: false,
        contextUsage: null,
        thinkingDetailContent: "",
        teamRunCursor: 0,
        teamMemberCursor: 0,
        view: "chat",
        pendingApprovals: [],
        approvalCursor: 0,
        markedApprovalIds: [],
        menuKind: null,
        menuTitle: "",
        menuItems: [],
        menuCursor: 0,
      };
    case "CLEAR_TIMELINE":
      return { ...state, timeline: [] };
    case "SET_APPROVAL":
      return {
        ...state,
        view: "approval",
        approvalToolCallId: action.toolCallId,
        approvalToolName: action.toolName,
        approvalCommand: action.command,
        approvalParams: action.params,
        approvalReplyCh: action.replyCh,
      };
    case "CLEAR_APPROVAL":
      return {
        ...state,
        view: "chat",
        approvalToolCallId: "",
        approvalToolName: "",
        approvalCommand: "",
        approvalParams: {},
        approvalReplyCh: null,
      };
    case "ADD_PENDING_APPROVAL": {
      const existing = state.pendingApprovals.find(item => item.toolCallId === action.item.toolCallId);
      const pendingApprovals = existing
        ? state.pendingApprovals.map(item => item.toolCallId === action.item.toolCallId ? action.item : item)
        : [...state.pendingApprovals, action.item];
      const existingReview = state.approvalReviewItems.find(item => item.toolCallId === action.item.toolCallId);
      const approvalReviewItems = existingReview
        ? state.approvalReviewItems.map(item => item.toolCallId === action.item.toolCallId ? { ...action.item, approvalStatus: item.approvalStatus } : item)
        : [...state.approvalReviewItems, { ...action.item, approvalStatus: "pending" as const }];
      return {
        ...state,
        view: "approval",
        pendingApprovals,
        approvalReviewItems,
        approvalCursor: Math.min(state.approvalCursor, Math.max(0, pendingApprovals.length - 1)),
        approvalToolCallId: action.item.toolCallId,
        approvalToolName: action.item.toolName,
        approvalCommand: action.item.command,
        approvalParams: action.item.params,
      };
    }
    case "REMOVE_PENDING_APPROVAL": {
      const pendingApprovals = state.pendingApprovals.filter(item => item.toolCallId !== action.toolCallId);
      const settledStatus = action.approved === undefined ? undefined : action.approved ? "approved" as const : "rejected" as const;
      const approvalReviewItems = settledStatus
        ? state.approvalReviewItems.map(item => item.toolCallId === action.toolCallId ? { ...item, approvalStatus: settledStatus } : item)
        : state.approvalReviewItems;
      return {
        ...state,
        view: pendingApprovals.length > 0 ? "approval" : "chat",
        pendingApprovals,
        approvalReviewItems: pendingApprovals.length > 0 ? approvalReviewItems : [],
        markedApprovalIds: state.markedApprovalIds.filter(id => id !== action.toolCallId),
        approvalCursor: Math.min(state.approvalCursor, Math.max(0, pendingApprovals.length - 1)),
        ...(pendingApprovals.length === 0 ? {
          approvalToolCallId: "", approvalToolName: "", approvalCommand: "", approvalParams: {}, approvalReplyCh: null,
        } : {}),
      };
    }
    case "CLEAR_PENDING_APPROVALS":
      return { ...state, view: "chat", ...clearPendingApprovals() };
    case "APPROVAL_NAV": {
      const maxIndex = Math.max(0, state.pendingApprovals.length - 1);
      return { ...state, approvalCursor: Math.max(0, Math.min(maxIndex, state.approvalCursor + action.delta)) };
    }
    case "TOGGLE_APPROVAL_MARK": {
      if (!action.toolCallId) return state;
      const exists = state.markedApprovalIds.includes(action.toolCallId);
      return {
        ...state,
        markedApprovalIds: exists ? state.markedApprovalIds.filter(id => id !== action.toolCallId) : [...state.markedApprovalIds, action.toolCallId],
      };
    }
    case "LOAD_HISTORY":
      return {
        ...state,
        timeline: action.afterReconnect
          ? [...finalizeOpenRuntimeEntries(action.entries, SOCKET_INTERRUPTION), { kind: "system", content: "WebSocket reconnected. Session history refreshed." }]
          : [...action.entries],
        streaming: false,
        assistantWaiting: false,
        liveAssistant: "",
        planGenerating: false,
        planReceived: false,
        ...clearTurnStats(),
        ...clearRuntimeTodos(),
      };
    case "THINKING_START": {
      if (action.parentToolCallId && action.subagentRunId) {
        const entries = [...state.timeline];
        const index = entries.findIndex(entry => entry.kind === "tool" && entry.toolCallId === action.parentToolCallId);
        if (index === -1) return state;
        const previous = entries[index];
        entries[index] = withSubagentThinkings(previous, [...getSubagentThinkings(previous), {
          content: "", thinkingDone: false, thinkingStartedAt: action.startedAt ?? Date.now(),
        }]);
        return { ...state, timeline: entries, turnTokenEstimate: state.streaming ? estimateTurnTokens(state, entries) : state.turnTokenEstimate };
      }
      const entries = [...state.timeline];
      if (state.liveAssistant.trim()) entries.push({ kind: "assistant", content: state.liveAssistant });
      entries.push({ kind: "thinking", content: "", thinkingDone: false, thinkingStartedAt: action.startedAt ?? Date.now() });
      return {
        ...state,
        timeline: entries,
        liveAssistant: "",
        turnTokenEstimate: state.streaming ? estimateTurnTokens(state, entries, "") : state.turnTokenEstimate,
      };
    }
    case "THINKING_CHUNK": {
      if (action.parentToolCallId && action.subagentRunId) {
        const entries = [...state.timeline];
        const index = entries.findIndex(entry => entry.kind === "tool" && entry.toolCallId === action.parentToolCallId);
        if (index === -1) return state;
        const previous = entries[index];
        const segments = getSubagentThinkings(previous);
        const latest = segments.at(-1);
        entries[index] = withSubagentThinkings(previous, latest && !latest.thinkingDone
          ? [...segments.slice(0, -1), { ...latest, content: latest.content + action.chunk }]
          : [...segments, { content: action.chunk, thinkingDone: false, thinkingStartedAt: action.startedAt ?? Date.now() }]);
        return { ...state, timeline: entries, turnTokenEstimate: state.streaming ? addTokenEstimate(state, action.chunk) : state.turnTokenEstimate };
      }
      const entries = [...state.timeline];
      for (let index = entries.length - 1; index >= 0; index--) {
        if (entries[index].kind === "thinking" && !entries[index].thinkingDone) {
          entries[index] = { ...entries[index], content: entries[index].content + action.chunk };
          break;
        }
      }
      return { ...state, timeline: entries, turnTokenEstimate: state.streaming ? addTokenEstimate(state, action.chunk) : state.turnTokenEstimate };
    }
    case "THINKING_DONE": {
      if (action.parentToolCallId && action.subagentRunId) {
        const entries = [...state.timeline];
        const index = entries.findIndex(entry => entry.kind === "tool" && entry.toolCallId === action.parentToolCallId);
        if (index === -1) return state;
        const previous = entries[index];
        const segments = getSubagentThinkings(previous);
        const thinking = segments.at(-1);
        if (!thinking || thinking.thinkingDone) return state;
        entries[index] = withSubagentThinkings(previous, [
          ...segments.slice(0, -1), finishSubagentSegment(thinking, action.finishedAt ?? Date.now()),
        ]);
        return { ...state, timeline: entries, turnTokenEstimate: state.streaming ? estimateTurnTokens(state, entries) : state.turnTokenEstimate };
      }
      const entries = [...state.timeline];
      for (let index = entries.length - 1; index >= 0; index--) {
        if (entries[index].kind === "thinking" && !entries[index].thinkingDone) {
          const startedAt = entries[index].thinkingStartedAt;
          const durationMs = startedAt !== undefined ? Math.max(0, (action.finishedAt ?? Date.now()) - startedAt) : entries[index].thinkingDurationMs;
          entries[index] = { ...entries[index], thinkingDone: true, thinkingDurationMs: durationMs };
          return { ...state, timeline: entries, turnThoughtDurationMs: durationMs };
        }
      }
      return { ...state, timeline: entries };
    }
    case "VIEW_THINKING_DETAIL":
      return { ...state, view: "thinking-detail", thinkingDetailContent: action.content };
    case "TOGGLE_PLAN_MODE":
      return { ...state, planMode: !state.planMode };
    case "SET_PLAN_CONFIRMATION":
      return {
        ...state,
        view: "plan-confirm",
        pendingPlanId: action.planId,
        pendingPlanContent: action.content,
        planConfirmCursor: 0,
        planModifyInput: "",
      };
    case "PLAN_CONFIRM_NAV":
      return { ...state, planConfirmCursor: Math.max(0, Math.min(1, state.planConfirmCursor + action.delta)) };
    case "SET_PLAN_MODIFY_INPUT":
      return { ...state, planModifyInput: action.value };
    case "CLEAR_PLAN_CONFIRMATION":
      return { ...state, view: "chat", pendingPlanId: "", pendingPlanContent: "", planConfirmCursor: 0, planModifyInput: "" };
    case "PLAN_START": {
      const entries = [...state.timeline];
      if (state.liveAssistant.trim()) entries.push({ kind: "assistant", content: state.liveAssistant });
      return {
        ...state,
        timeline: entries,
        liveAssistant: "",
        planGenerating: true,
        turnTokenEstimate: state.streaming ? estimateTurnTokens(state, entries, "") : state.turnTokenEstimate,
      };
    }
    case "TODO_UPDATE":
      return {
        ...state,
        runtimeTodos: action.items.map(item => ({ ...item })),
        runtimeTodosNote: action.note || "",
        runtimeTodosUpdatedAt: action.updatedAt,
      };
    case "PLAN_CHUNK": {
      if (action.chunk === "") return state;
      const entries = [...state.timeline];
      if (!state.planGenerating && state.liveAssistant.trim()) entries.push({ kind: "assistant", content: state.liveAssistant });
      return {
        ...state,
        timeline: entries,
        liveAssistant: "",
        planGenerating: true,
        planReceived: false,
        turnTokenEstimate: state.streaming ? estimateTurnTokens(state, entries, "") : state.turnTokenEstimate,
      };
    }
    case "PLAN_BODY": {
      const entries = [...state.timeline];
      if (state.liveAssistant.trim()) entries.push({ kind: "assistant", content: state.liveAssistant });
      entries.push({ kind: "plan", content: action.planBody });
      return {
        ...state,
        timeline: entries,
        liveAssistant: "",
        planGenerating: false,
        planReceived: true,
        turnTokenEstimate: state.streaming ? estimateTurnTokens(state, entries, "") : state.turnTokenEstimate,
      };
    }
    case "SET_QA":
      return {
        ...state,
        view: "question-answer",
        qaToolCallId: action.toolCallId,
        qaQuestions: action.questions,
        qaCurrentIndex: 0,
        qaAnswers: action.questions.map(question => ({ questionId: question.id, selectedOption: -1, customAnswer: "" })),
        qaStep: "questions",
        qaCursor: 0,
        qaCustomInput: "",
        qaCustomInputKey: 0,
        qaEditingFromConfirm: false,
      };
    case "QA_NAV": {
      const question = state.qaQuestions[state.qaCurrentIndex];
      const maxIndex = question ? question.options.length : 0;
      return { ...state, qaCursor: Math.max(0, Math.min(state.qaCursor + action.delta, maxIndex)) };
    }
    case "QA_NAV_TO": {
      const question = state.qaQuestions[state.qaCurrentIndex];
      const maxIndex = question ? question.options.length : 0;
      return { ...state, qaCursor: Math.max(0, Math.min(action.cursor, maxIndex)) };
    }
    case "QA_SELECT": {
      const answers = [...state.qaAnswers];
      const current = answers[state.qaCurrentIndex];
      const isCustom = action.optionIndex === -1;
      answers[state.qaCurrentIndex] = { ...current, selectedOption: action.optionIndex, customAnswer: isCustom ? current.customAnswer : "" };
      return { ...state, qaAnswers: answers, qaCustomInput: isCustom ? current.customAnswer || "" : "" };
    }
    case "QA_SET_CUSTOM_INPUT":
      return { ...state, qaCustomInput: action.value };
    case "QA_SUBMIT_CUSTOM": {
      const value = action.value.trim();
      if (!value) return state;
      const answers = [...state.qaAnswers];
      answers[state.qaCurrentIndex] = { ...answers[state.qaCurrentIndex], selectedOption: -1, customAnswer: value };
      return { ...state, qaAnswers: answers, qaCustomInput: value };
    }
    case "QA_NEXT_QUESTION": {
      if (state.qaEditingFromConfirm) {
        return { ...state, qaStep: "confirm", qaCursor: state.qaQuestions.length, qaCustomInput: "", qaEditingFromConfirm: false };
      }
      const nextIndex = state.qaCurrentIndex + 1;
      if (nextIndex >= state.qaQuestions.length) return { ...state, qaStep: "confirm", qaCursor: state.qaQuestions.length };
      return { ...state, qaCurrentIndex: nextIndex, qaCursor: 0, qaCustomInput: "" };
    }
    case "QA_PREV_QUESTION":
      if (state.qaCurrentIndex <= 0) return state;
      return { ...state, qaCurrentIndex: state.qaCurrentIndex - 1, qaCursor: 0, qaCustomInput: "" };
    case "QA_EDIT_QUESTION":
      return { ...state, ...restoreQuestionCursor(state, action.index), qaStep: "questions", qaEditingFromConfirm: true };
    case "QA_STEP_CONFIRM":
      return { ...state, qaStep: "confirm", qaCursor: state.qaQuestions.length, qaEditingFromConfirm: false };
    case "QA_STEP_BACK":
      return {
        ...state,
        ...restoreQuestionCursor(state, Math.max(0, state.qaQuestions.length - 1)),
        qaStep: "questions",
        qaEditingFromConfirm: false,
      };
    case "QA_CONFIRM_NAV":
      return { ...state, qaCursor: Math.max(0, Math.min(state.qaCursor + action.delta, state.qaQuestions.length)) };
    case "CLEAR_QA":
      return { ...state, view: "chat", ...clearQuestionAnswer() };
    case "FLUSH_AND_WAIT": {
      const entries = [...state.timeline];
      if (state.liveAssistant.trim()) entries.push({ kind: "assistant", content: state.liveAssistant });
      return {
        ...state,
        assistantWaiting: true,
        liveAssistant: "",
        timeline: entries,
        turnTokenEstimate: state.streaming ? estimateTurnTokens(state, entries, "") : state.turnTokenEstimate,
      };
    }
    default:
      return null;
  }
}
