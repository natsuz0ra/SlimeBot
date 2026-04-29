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
