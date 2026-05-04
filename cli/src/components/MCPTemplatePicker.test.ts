import assert from "node:assert/strict";
import test from "node:test";
import {
	MCP_TEMPLATE_COLORS,
	MCP_TEMPLATE_HINT,
	MCP_TEMPLATE_TITLE_GAP_LINES,
} from "./MCPTemplatePicker";
import { MCP_TEMPLATES } from "../types";

test("MCP template picker keeps the three transport templates", () => {
	assert.deepEqual(
		MCP_TEMPLATES.map((tpl) => tpl.kind),
		["stdio", "sse", "streamable_http"],
	);
});

test("MCP template picker leaves a single title gap", () => {
	assert.equal(MCP_TEMPLATE_TITLE_GAP_LINES, 1);
});

test("MCP template picker hint preserves existing keyboard operations", () => {
	assert.equal(MCP_TEMPLATE_HINT, "Arrow keys to navigate | Enter to select | Esc to cancel");
});

test("MCP template picker palette separates selected, idle, and hint text", () => {
	assert.notEqual(MCP_TEMPLATE_COLORS.selectedTitle, MCP_TEMPLATE_COLORS.idleTitle);
	assert.notEqual(MCP_TEMPLATE_COLORS.cursor, MCP_TEMPLATE_COLORS.idleCursor);
	assert.notEqual(MCP_TEMPLATE_COLORS.title, MCP_TEMPLATE_COLORS.hint);
});
