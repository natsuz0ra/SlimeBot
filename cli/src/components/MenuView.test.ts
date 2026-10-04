import assert from "node:assert/strict";
import test from "node:test";
import {
	CLI_HINT_COLOR,
	MENU_ITEM_COLORS,
	MENU_TITLE_GAP_LINES,
	MENU_VISIBLE_LIMIT,
	formatMenuDescriptionLines,
	getVisibleMenuItems,
	getMenuViewport,
	MenuView,
	truncateMenuDescription,
	truncateMenuTitle,
} from "./MenuView";
import type { MenuItem } from "../types";
import React from "react";
import { Box, Text } from "ink";
import { Banner } from "./Banner.js";
import { terminalWorkspaceLayout } from "../utils/terminal.js";
import { PassThrough } from "node:stream";
import { renderSync, forceRedraw } from "ink";
import stripAnsi from "strip-ansi";
import { createInitialState, reducer } from "../reducer.js";
import { useCliKeyboard } from "../hooks/useCliKeyboard.js";
import type { AppState } from "../types.js";

function menuItems(count: number): MenuItem[] {
	return Array.from({ length: count }, (_, index) => ({
		title: `Session ${index + 1}`,
		desc: `Updated ${index + 1}`,
		data: { id: `session-${index + 1}` },
	}));
}

test("truncateMenuTitle truncates long titles with ellipsis", () => {
	const input = "12345678901234567890123456";
	assert.equal(truncateMenuTitle(input), "123456789012345678901234…");
});

test("truncateMenuDescription limits description to 80 characters", () => {
	const input = "a".repeat(100);
	const output = truncateMenuDescription(input);

	assert.equal(output.length, 80);
	assert.ok(output.endsWith("…"));
});

test("formatMenuDescriptionLines wraps by terminal width", () => {
	const lines = formatMenuDescriptionLines(
		"Use when user asks to run a Python script locally to write files.",
		24,
	);

	assert.ok(lines.length > 1);
	assert.ok(lines.every((line) => line.length <= 22));
});

test("menu spacing leaves a blank line after the title", () => {
	assert.equal(MENU_TITLE_GAP_LINES, 1);
});

test("menu palette gives active and inactive items distinct colors", () => {
	assert.notEqual(MENU_ITEM_COLORS.activeTitle, MENU_ITEM_COLORS.inactiveTitle);
	assert.notEqual(
		MENU_ITEM_COLORS.activeCursor,
		MENU_ITEM_COLORS.inactiveCursor,
	);
});

test("menu hint uses the shared CLI hint color", () => {
	assert.equal(MENU_ITEM_COLORS.hint, CLI_HINT_COLOR);
});

test("getVisibleMenuItems returns all items below the visible limit", () => {
	const visible = getVisibleMenuItems(menuItems(3), 0, MENU_VISIBLE_LIMIT);

	assert.deepEqual(
		visible.items.map((item) => item.title),
		["Session 1", "Session 2", "Session 3"],
	);
	assert.equal(visible.startIndex, 0);
});

test("getVisibleMenuItems scrolls down to keep the sixth item visible", () => {
	const visible = getVisibleMenuItems(menuItems(8), 5, MENU_VISIBLE_LIMIT);

	assert.deepEqual(
		visible.items.map((item) => item.title),
		["Session 2", "Session 3", "Session 4", "Session 5", "Session 6"],
	);
	assert.equal(visible.startIndex, 1);
});

test("getVisibleMenuItems anchors the final window near the end", () => {
	const visible = getVisibleMenuItems(menuItems(8), 7, MENU_VISIBLE_LIMIT);

	assert.deepEqual(
		visible.items.map((item) => item.title),
		["Session 4", "Session 5", "Session 6", "Session 7", "Session 8"],
	);
	assert.equal(visible.startIndex, 3);
});

test("model menu windows keep the selected model visible after scrolling and resizing", () => {
	const items = menuItems(40).map(item => ({ ...item, title: item.title + " · 中文 🚀", desc: "Provider · deepseek-flash-with-a-long-model-id · 1,000,000 tokens" }));
	for (const columns of [20, 40, 80, 120]) for (const height of [8, 12, 18]) for (const cursor of [0, 1, 15, 39]) {
		const window = getMenuViewport(items, cursor, columns, height, "Enter switch | A providers | E edit | D delete | Esc close");
		assert.ok(window.items.some(row => row.index === cursor));
		assert.ok(window.items.length < items.length);
		assert.match(window.position, new RegExp(`^${cursor + 1}/40`));
		assert.ok(window.startIndex <= cursor && window.endIndex > cursor);
	}
	assert.deepEqual(getMenuViewport([], 0, 40, 8, "Esc close").items, []);
});

test("actual Ink model menus with many models fit the whole terminal", async () => {
	const { renderToScreen } = await import(new URL("../../packages/ink/src/ink/render-to-screen.ts", import.meta.url).href);
	const { cellAt } = await import(new URL("../../packages/ink/src/ink/screen.ts", import.meta.url).href);
	const cwd = "/workspace/" + "long-project-directory/".repeat(6);
	const items = menuItems(40).map(item => ({ ...item, desc: "Provider · deepseek-flash-with-a-long-model-id · 1,000,000 tokens" }));
	for (const [columns, rows] of [[20, 12], [40, 16], [80, 24], [120, 32]]) for (const cursor of [0, 19, 39]) {
		const layout = terminalWorkspaceLayout(columns!, rows!, ["SlimeBot CLI 1.34.0 [new] [auto review]", "DeepSeek Flash", cwd], true);
		const banner = layout.compactBanner
			? React.createElement(Text, { wrap: "truncate" }, "SlimeBot CLI 1.34.0")
			: React.createElement(Banner, { version: "1.34.0", modelName: "DeepSeek Flash", cwd, approvalMode: "auto_review", updateAvailable: true });
		const result = renderToScreen(React.createElement(Box, { flexDirection: "column" },
			banner, React.createElement(Text, null, " "), React.createElement(Text, null, "Context usage"), React.createElement(Text, null, "─".repeat(columns!)),
			React.createElement(MenuView, { title: "Model Menu", items, cursor, columns: columns!, maxHeight: layout.height, hint: "Enter switch | A providers | E edit | D delete | Esc close" }),
		), columns!);
		assert.ok(result.height <= rows!, `${columns}×${rows} rendered ${result.height} rows`);
		const text = Array.from({ length: result.height }, (_, y) => Array.from({ length: columns! }, (_, x) => cellAt(result.screen, x, y).char).join("")).join("\n");
		assert.match(text, new RegExp(`❯ Session ${cursor + 1}(?: |$)`));
		assert.match(text, new RegExp(`${cursor + 1}/40`));
	}
});

test("real Ink model-menu keys scroll, page, resize and select the visible model", async () => {
	const items = menuItems(40);
	const stdout = Object.assign(new PassThrough(), { isTTY: true, columns: 40, rows: 16, getWindowSize: () => [40, 16] });
	const stdin = Object.assign(new PassThrough(), { isTTY: true, setRawMode() {}, ref() {}, unref() {} });
	const stderr = new PassThrough();
	let output = "";
	stdout.on("data", chunk => { output += chunk.toString(); });
	let current: AppState;
	let selected: MenuItem | undefined;
	const overflow: unknown[] = [];
	const noop = () => {};
	const asyncNoop = async () => {};
	function Harness({ columns, height }: { columns: number; height: number }) {
		const [state, dispatch] = React.useReducer(reducer, {
			...createInitialState("http://127.0.0.1:8080", "test", "/tmp", "1.34.0"),
			view: "menu", menuKind: "model", menuItems: items, menuTitle: "Model Menu", menuHint: "Enter switch | Esc close",
		} as AppState);
		current = state;
		const viewport = getMenuViewport(items, state.menuCursor, columns, height, state.menuHint);
		useCliKeyboard({
			state, dispatch, menuPageSize: viewport.items.length, socketRef: { current: null }, exit: noop,
			handleMenuSelect: async item => { selected = item; }, handleMenuDelete: asyncNoop,
			handleMenuAdd: noop, handleMenuEdit: noop, handleMenuToggle: asyncNoop, handleMenuTools: asyncNoop, handleMenuDiscover: noop,
			loadUpdate: asyncNoop, applyUpdate: asyncNoop, loadMemory: asyncNoop, handleMemoryConsoleSelect: asyncNoop, saveMemoryConsoleDraft: asyncNoop,
			loadMCPConfigs: asyncNoop, loadModels: asyncNoop, loadProviders: asyncNoop, loadProviderModels: asyncNoop, refreshMCPTools: noop,
			saveMCPConfig: asyncNoop, saveModelConfig: asyncNoop, saveProviderConfig: asyncNoop, selectMCPTemplate: noop, moveModelProvider: value => value,
		});
		return state.view === "menu"
			? React.createElement(MenuView, { title: state.menuTitle, items, cursor: state.menuCursor, hint: state.menuHint, columns, maxHeight: height })
			: React.createElement(Text, null, "Chat restored");
	}
	const instance = renderSync(React.createElement(Harness, { columns: 40, height: 12 }), {
		stdout: stdout as never, stdin: stdin as never, stderr: stderr as never, exitOnCtrlC: false, patchConsole: false,
		onFrame: event => { overflow.push(...event.flickers.filter(frame => frame.reason === "offscreen")); },
	});
	async function press(value: string) { stdin.write(value); await new Promise(resolve => setTimeout(resolve, 120)); }
	function paint() { const start = output.length; forceRedraw(stdout as never); return stripAnsi(output.slice(start)).replace(/\s/g, ""); }
	try {
		for (let i = 0; i < 6; i++) await press("\x1b[B");
		assert.equal(current!.menuCursor, 6);
		assert.match(paint(), /❯Session7/);
		await press("\x1b[6~");
		assert.ok(current!.menuCursor > 6);
		await press("\x1b[H");
		assert.equal(current!.menuCursor, 0);
		await press("\x1b[F");
		assert.equal(current!.menuCursor, 39);
		assert.match(paint(), /❯Session40/);
		instance.rerender(React.createElement(Harness, { columns: 20, height: 8 }));
		assert.match(paint(), /40\/40/);
		await press("\r");
		assert.deepEqual(selected, items[39]);
		await press("\x1b");
		assert.equal(current!.view, "chat");
		assert.match(paint(), /Chatrestored/);
		assert.deepEqual(overflow, []);
	} finally {
		instance.unmount(); instance.cleanup();
		stdin.destroy(); stdout.destroy(); stderr.destroy();
	}
});
