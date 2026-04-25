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
