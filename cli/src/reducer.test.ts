import assert from "node:assert/strict";
import test from "node:test";
import { createInitialState, reducer } from "./reducer";
import type { AppAction, AppState } from "./types";

function initState() {
	return createInitialState(
		"http://127.0.0.1:8080",
		"token",
		"C:/repo",
		"1.0.0",
	);
}

function reduce(state: AppState, action: AppAction): AppState {
	return reducer(state, action);
}

test("createInitialState initializes empty input value", () => {
	const state = initState();
	assert.equal(state.inputValue, "");
	assert.equal(state.sessionName, "");
});

test("SET_INPUT updates value", () => {
	const state = reduce(initState(), {
		type: "SET_INPUT",
		value: "hello",
	});

	assert.equal(state.inputValue, "hello");
});

test("RESET_SESSION keeps input value and clears session state", () => {
	let state = reduce(initState(), {
		type: "SET_INPUT",
		value: "cursor",
	});
	state = reduce(state, {
		type: "SET_SESSION",
		sessionId: "sid-1",
		sessionName: "My Session",
	});
	state = reduce(state, {
		type: "APPEND_ENTRY",
		entry: { kind: "system", content: "x" },
	});

	state = reduce(state, { type: "RESET_SESSION" });
	assert.equal(state.inputValue, "cursor");
	assert.equal(state.sessionId, "");
	assert.equal(state.sessionName, "");
	assert.equal(state.timeline.length, 0);
});

test("SET_SESSION_NAME updates current session title", () => {
	let state = reduce(initState(), {
		type: "SET_SESSION",
		sessionId: "sid-2",
		sessionName: "First Name",
	});
	state = reduce(state, {
		type: "SET_SESSION_NAME",
		sessionName: "Renamed",
	});
	assert.equal(state.sessionName, "Renamed");
});

test("APPLY_SESSION_TITLE updates only the matching current session", () => {
	let state = reduce(initState(), {
		type: "SET_SESSION",
		sessionId: "sid-2",
		sessionName: "New Chat",
	});
	state = reduce(state, {
		type: "APPLY_SESSION_TITLE",
		sessionId: "sid-other",
		title: "Other Title",
	});
	assert.equal(state.sessionName, "New Chat");

	state = reduce(state, {
		type: "APPLY_SESSION_TITLE",
		sessionId: "sid-2",
		title: "Generated Title",
	});
	assert.equal(state.sessionName, "Generated Title");
});

test("TOGGLE_COMPACT switches compact mode on and off", () => {
	let state = initState();
	assert.equal(state.compact, true);

	state = reduce(state, { type: "TOGGLE_COMPACT" });
	assert.equal(state.compact, false);

	state = reduce(state, { type: "TOGGLE_COMPACT" });
	assert.equal(state.compact, true);
});

test("TOGGLE_TOOL_OUTPUT switches tool output expanded on and off", () => {
	let state = initState();
	assert.equal(state.toolOutputExpanded, false);

	state = reduce(state, { type: "TOGGLE_TOOL_OUTPUT" });
	assert.equal(state.toolOutputExpanded, true);

	state = reduce(state, { type: "TOGGLE_TOOL_OUTPUT" });
	assert.equal(state.toolOutputExpanded, false);
});

test("SET_MODEL_EDITOR_VIEW initializes context size defaults", () => {
	const state = reduce(initState(), { type: "SET_MODEL_EDITOR_VIEW" });

	assert.equal(state.view, "model-editor");
	assert.equal(state.modelEditorId, "");
	assert.equal(state.modelEditorContextSize, "1000000");
	assert.equal(state.modelEditorFocusIndex, 0);
});

test("SET_MODEL_EDITOR preloads existing model config for editing", () => {
	const state = reduce(initState(), {
		type: "SET_MODEL_EDITOR",
		config: {
			id: "model-1",
			name: "Claude",
			provider: "anthropic",
			baseUrl: "https://api.anthropic.com",
			apiKey: "secret",
			model: "claude",
			contextSize: 128_000,
			createdAt: "",
			updatedAt: "",
		},
	});

	assert.equal(state.view, "model-editor");
	assert.equal(state.modelEditorId, "model-1");
	assert.equal(state.modelEditorProvider, "anthropic");
	assert.equal(state.modelEditorContextSize, "128000");
});

test("THINKING_DONE stores a fixed thinking duration", () => {
	const state: AppState = {
		...initState(),
		timeline: [
			{
				kind: "thinking",
				content: "reasoning",
				thinkingDone: false,
				thinkingStartedAt: 1_000,
			},
		],
	};

	const next = reduce(state, {
		type: "THINKING_DONE",
		finishedAt: 2_750,
	});
	const entry = next.timeline[0];

	assert.equal(entry.kind, "thinking");
	assert.equal(entry.thinkingDone, true);
	assert.equal(entry.thinkingDurationMs, 1_750);
});

test("THINKING_START flushes live assistant text before new thinking block", () => {
	const state: AppState = {
		...initState(),
		liveAssistant: "spoken before more thought",
	};

	const next = reduce(state, { type: "THINKING_START" });

	assert.deepEqual(
		next.timeline.map((entry) => entry.kind),
		["assistant", "thinking"],
	);
	assert.equal(next.timeline[0].content, "spoken before more thought");
	assert.equal(next.liveAssistant, "");
});

test("approval queue supports multiple pending approvals and cursor bounds", () => {
	let state = initState();
	state = reduce(state, {
		type: "ADD_PENDING_APPROVAL",
		item: {
			toolCallId: "call-a",
			toolName: "exec",
			command: "run",
			params: { command: "npm test" },
		},
	});
	state = reduce(state, {
		type: "ADD_PENDING_APPROVAL",
		item: {
			toolCallId: "call-b",
			toolName: "exec",
			command: "run",
			params: { command: "go test ./..." },
		},
	});

	assert.equal(state.view, "approval");
	assert.deepEqual(state.pendingApprovals.map((item) => item.toolCallId), ["call-a", "call-b"]);

	state = reduce(state, { type: "APPROVAL_NAV", delta: 10 });
	assert.equal(state.approvalCursor, 1);

	state = reduce(state, { type: "REMOVE_PENDING_APPROVAL", toolCallId: "call-b" });
	assert.equal(state.view, "approval");
	assert.equal(state.approvalCursor, 0);
	assert.deepEqual(state.pendingApprovals.map((item) => item.toolCallId), ["call-a"]);

	state = reduce(state, { type: "REMOVE_PENDING_APPROVAL", toolCallId: "call-a" });
	assert.equal(state.view, "chat");
	assert.deepEqual(state.pendingApprovals, []);
});

test("CLEAR_PENDING_APPROVALS returns to chat view", () => {
	let state = reduce(initState(), {
		type: "ADD_PENDING_APPROVAL",
		item: {
			toolCallId: "call-a",
			toolName: "exec",
			command: "run",
			params: {},
		},
	});

	state = reduce(state, { type: "CLEAR_PENDING_APPROVALS" });

	assert.equal(state.view, "chat");
	assert.deepEqual(state.pendingApprovals, []);
	assert.equal(state.approvalCursor, 0);
});

test("STREAM_START initializes current turn stats", () => {
	const state = reduce(initState(), { type: "STREAM_START", startedAt: 1_000 });

	assert.equal(state.turnStartedAt, 1_000);
	assert.equal(state.turnElapsedMs, 0);
	assert.equal(state.turnTokenEstimate, 0);
	assert.equal(state.turnThoughtDurationMs, undefined);
});

test("TODO_UPDATE replaces runtime todos and clears on turn/session boundaries", () => {
	let state = reduce(initState(), {
		type: "TODO_UPDATE",
		items: [
			{ id: "a", content: "Inspect", status: "completed" },
			{ id: "b", content: "Implement", status: "in_progress" },
		],
		note: "working",
		updatedAt: 1_000,
	});

	assert.equal(state.runtimeTodosNote, "working");
	assert.equal(state.runtimeTodosUpdatedAt, 1_000);
	assert.deepEqual(state.runtimeTodos.map((item) => item.status), ["completed", "in_progress"]);

	state = reduce(state, { type: "STREAM_START", startedAt: 2_000 });
	assert.deepEqual(state.runtimeTodos, []);

	state = reduce(state, {
		type: "TODO_UPDATE",
		items: [{ id: "c", content: "Verify", status: "pending" }],
	});
	state = reduce(state, { type: "STREAM_DONE", error: null });
	assert.deepEqual(state.runtimeTodos, []);

	state = reduce(state, {
		type: "TODO_UPDATE",
		items: [{ id: "d", content: "Hidden", status: "pending" }],
	});
	state = reduce(state, { type: "RESET_SESSION" });
	assert.deepEqual(state.runtimeTodos, []);
});

test("TURN_STATS_TICK updates elapsed while streaming", () => {
	let state = reduce(initState(), { type: "STREAM_START", startedAt: 1_000 });

	state = reduce(state, { type: "TURN_STATS_TICK", now: 3_750 });

	assert.equal(state.turnElapsedMs, 2_750);
});

test("STREAM_DONE and RESET_SESSION clear current turn stats", () => {
	let state = reduce(initState(), { type: "STREAM_START", startedAt: 1_000 });
	state = reduce(state, { type: "TURN_STATS_TICK", now: 3_000 });
	state = reduce(state, { type: "STREAM_DONE", error: null });

	assert.equal(state.turnStartedAt, undefined);
	assert.equal(state.turnElapsedMs, 0);
	assert.equal(state.turnTokenEstimate, 0);
	assert.equal(state.turnThoughtDurationMs, undefined);

	state = reduce(initState(), { type: "STREAM_START", startedAt: 1_000 });
	state = reduce(state, { type: "RESET_SESSION" });

	assert.equal(state.turnStartedAt, undefined);
	assert.equal(state.turnElapsedMs, 0);
	assert.equal(state.turnTokenEstimate, 0);
	assert.equal(state.turnThoughtDurationMs, undefined);
});

test("QA_SUBMIT_CUSTOM persists custom answer and selects custom option", () => {
	let state = initState();
	state = reduce(state, {
		type: "SET_QA",
		toolCallId: "tool-1",
		questions: [{ id: "q1", question: "Pick", options: ["A", "B"] }],
	});
	state = reduce(state, { type: "QA_SET_CUSTOM_INPUT", value: "  my answer  " });
	state = reduce(state, { type: "QA_SUBMIT_CUSTOM", value: state.qaCustomInput });

	assert.equal(state.qaAnswers[0]?.selectedOption, -1);
	assert.equal(state.qaAnswers[0]?.customAnswer, "my answer");
	assert.equal(state.qaCustomInput, "my answer");
});

test("QA_SUBMIT_CUSTOM ignores empty input", () => {
	let state = initState();
	state = reduce(state, {
		type: "SET_QA",
		toolCallId: "tool-1",
		questions: [{ id: "q1", question: "Pick", options: ["A", "B"] }],
	});
	const before = state.qaAnswers[0];
	state = reduce(state, { type: "QA_SUBMIT_CUSTOM", value: "   " });

	assert.deepEqual(state.qaAnswers[0], before);
	assert.equal(state.qaCustomInput, "");
});

test("QA_SELECT on preset option clears custom answer", () => {
	let state = initState();
	state = reduce(state, {
		type: "SET_QA",
		toolCallId: "tool-1",
		questions: [{ id: "q1", question: "Pick", options: ["A", "B"] }],
	});
	state = reduce(state, { type: "QA_SUBMIT_CUSTOM", value: "custom" });
	state = reduce(state, { type: "QA_SELECT", optionIndex: 0 });

	assert.equal(state.qaAnswers[0]?.selectedOption, 0);
	assert.equal(state.qaAnswers[0]?.customAnswer, "");
	assert.equal(state.qaCustomInput, "");
});

test("THINKING_DONE stores top-level duration for waiting stats", () => {
	let state: AppState = {
		...initState(),
		turnStartedAt: 500,
		streaming: true,
		timeline: [
			{
				kind: "thinking",
				content: "reasoning",
				thinkingDone: false,
				thinkingStartedAt: 1_000,
			},
		],
	};

	state = reduce(state, {
		type: "THINKING_DONE",
		finishedAt: 2_750,
	});

	assert.equal(state.turnThoughtDurationMs, 1_750);
});

test("tagged subagent thinking updates parent tool instead of global timeline", () => {
	let state: AppState = {
		...initState(),
		timeline: [{
			kind: "tool",
			content: "",
			toolCallId: "parent-tool",
			toolName: "run_subagent",
			command: "delegate",
			status: "executing",
		}],
	};

	state = reduce(state, {
		type: "THINKING_START",
		parentToolCallId: "parent-tool",
		subagentRunId: "sub-run",
		startedAt: 1_000,
	});
	state = reduce(state, {
		type: "THINKING_CHUNK",
		chunk: "child reasoning",
		parentToolCallId: "parent-tool",
		subagentRunId: "sub-run",
	});
	state = reduce(state, {
		type: "THINKING_DONE",
		parentToolCallId: "parent-tool",
		subagentRunId: "sub-run",
		finishedAt: 1_400,
	});

	assert.deepEqual(state.timeline.map((entry) => entry.kind), ["tool"]);
	assert.equal(state.timeline[0].subagentThinking?.content, "child reasoning");
	assert.equal(state.timeline[0].subagentThinking?.thinkingDone, true);
	assert.equal(state.timeline[0].subagentThinking?.thinkingDurationMs, 400);
});

test("SUBAGENT_DONE closes open subagent thinking and marks failed parent tool", () => {
	let state: AppState = {
		...initState(),
		streaming: true,
		timeline: [{
			kind: "tool",
			content: "",
			toolCallId: "parent-tool",
			toolName: "run_subagent",
			command: "delegate",
			status: "executing",
			subagentThinking: {
				content: "child reasoning",
				thinkingDone: false,
				thinkingStartedAt: 1_000,
			},
		}],
	};

	state = reduce(state, {
		type: "SUBAGENT_DONE",
		parentToolCallId: "parent-tool",
		error: "context canceled",
		finishedAt: 1_500,
	});

	assert.equal(state.timeline[0].status, "error");
	assert.equal(state.timeline[0].error, "context canceled");
	assert.equal(state.timeline[0].subagentThinking?.thinkingDone, true);
	assert.equal(state.timeline[0].subagentThinking?.thinkingDurationMs, 500);
});

test("STREAM_DONE closes open subagent thinking and open tool states", () => {
	const state: AppState = {
		...initState(),
		streaming: true,
		timeline: [{
			kind: "tool",
			content: "",
			toolCallId: "parent-tool",
			toolName: "run_subagent",
			command: "delegate",
			status: "executing",
			subagentThinking: {
				content: "child reasoning",
				thinkingDone: false,
				thinkingStartedAt: 1_000,
			},
		}],
	};

	const next = reduce(state, { type: "STREAM_DONE", error: null });

	assert.equal(next.timeline[0].status, "error");
	assert.equal(next.timeline[0].error, "Execution cancelled.");
	assert.equal(next.timeline[0].subagentThinking?.thinkingDone, true);
});

test("STREAM_DONE keeps final assistant when previous timeline already has a plan", () => {
	const state: AppState = {
		...initState(),
		timeline: [{ kind: "plan", content: "# Existing plan" }],
		liveAssistant: "execution finished",
		streaming: true,
	};

	const next = reduce(state, { type: "STREAM_DONE", error: null });

	assert.deepEqual(
		next.timeline.map((entry) => entry.kind),
		["plan", "assistant"],
	);
	assert.equal(next.timeline[1].content, "execution finished");
});

test("STREAM_DONE does not duplicate a plan body received in the current turn", () => {
	let state: AppState = {
		...initState(),
		liveAssistant: "# Plan",
		streaming: true,
	};

	state = reduce(state, { type: "PLAN_BODY", planBody: "# Plan" });
	state = reduce(state, { type: "STREAM_DONE", error: null });

	assert.deepEqual(
		state.timeline.map((entry) => entry.kind),
		["assistant", "plan"],
	);
	assert.equal(
		state.timeline.filter((entry) => entry.kind === "plan").length,
		1,
	);
});

test("PLAN_CHUNK starts plan generation without showing streamed plan content", () => {
	let state: AppState = {
		...initState(),
		streaming: true,
	};

	state = reduce(state, { type: "PLAN_CHUNK", chunk: "# Plan\n" });
	state = reduce(state, { type: "PLAN_CHUNK", chunk: "\nDo the thing." });

	assert.equal(state.planGenerating, true);
	assert.equal(state.planReceived, false);
	assert.deepEqual(state.timeline, []);
	assert.equal(state.liveAssistant, "");
});

test("PLAN_CHUNK flushes narration before hiding plan chunks", () => {
	let state: AppState = {
		...initState(),
		liveAssistant: "Narration before plan.",
		streaming: true,
	};

	state = reduce(state, { type: "PLAN_CHUNK", chunk: "# Draft" });

	assert.deepEqual(
		state.timeline.map((entry) => entry.kind),
		["assistant"],
	);
	assert.equal(state.timeline[0].content, "Narration before plan.");
	assert.equal(state.liveAssistant, "");
	assert.equal(state.planGenerating, true);
});

test("PLAN_BODY appends the final plan after hidden streamed chunks", () => {
	let state: AppState = {
		...initState(),
		streaming: true,
	};

	state = reduce(state, { type: "PLAN_CHUNK", chunk: "# Draft" });
	state = reduce(state, { type: "PLAN_BODY", planBody: "# Final Plan" });

	assert.equal(state.planGenerating, false);
	assert.equal(state.planReceived, true);
	assert.deepEqual(
		state.timeline.map((entry) => entry.kind),
		["plan"],
	);
	assert.equal(state.timeline[0].content, "# Final Plan");
});

test("PLAN_BODY appends a new plan without replacing an existing plan", () => {
	let state: AppState = {
		...initState(),
		timeline: [{ kind: "plan", content: "# Existing Plan" }],
		streaming: true,
	};

	state = reduce(state, { type: "PLAN_CHUNK", chunk: "# Draft" });
	state = reduce(state, { type: "PLAN_BODY", planBody: "# Final Plan" });

	assert.deepEqual(
		state.timeline.map((entry) => entry.kind),
		["plan", "plan"],
	);
	assert.equal(state.timeline[0].content, "# Existing Plan");
	assert.equal(state.timeline[1].content, "# Final Plan");
});

test("STREAM_DONE clears plan generation after an interrupted plan stream", () => {
	let state: AppState = {
		...initState(),
		streaming: true,
	};

	state = reduce(state, { type: "PLAN_CHUNK", chunk: "# Draft" });
	state = reduce(state, { type: "STREAM_DONE", error: null });

	assert.equal(state.planGenerating, false);
	assert.equal(state.planReceived, false);
	assert.deepEqual(state.timeline, []);
});

test("SET_PLAN_CONFIRMATION enters plan-confirm view", () => {
	let state = initState();
	state = reduce(state, {
		type: "SET_PLAN_CONFIRMATION",
		planId: "plan-1",
		content: "# Plan\n\nDo it.",
	});

	assert.equal(state.view, "plan-confirm");
	assert.equal(state.pendingPlanId, "plan-1");
	assert.equal(state.pendingPlanContent, "# Plan\n\nDo it.");
	assert.equal(state.planConfirmCursor, 0);
});

test("PLAN_BODY updates visible plan content without entering plan-confirm view", () => {
	let state: AppState = {
		...initState(),
		view: "chat",
		streaming: true,
	};

	state = reduce(state, { type: "PLAN_BODY", planBody: "# Plan\n\nDo it." });

	assert.equal(state.view, "chat");
	assert.equal(state.pendingPlanId, "");
	assert.equal(state.pendingPlanContent, "");
	assert.deepEqual(state.timeline.map((entry) => entry.kind), ["plan"]);
});
