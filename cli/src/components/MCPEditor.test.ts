import assert from "node:assert/strict";
import test from "node:test";
import {
	MCP_EDITOR_COLORS,
	buildMCPConfigDivider,
	getMCPConfigStatus,
	getMCPEditorModeLabel,
} from "./MCPEditor";

test("MCP editor labels new and edit modes", () => {
	assert.equal(getMCPEditorModeLabel("", ""), "new config");
	assert.equal(getMCPEditorModeLabel("mcp-1", "filesystem"), "edit: filesystem");
	assert.equal(getMCPEditorModeLabel("mcp-1", "   "), "edit config");
});

test("MCP editor status marks stdio command configs as valid", () => {
	const status = getMCPConfigStatus('{"command":"npx","args":["-y","server"]}');

	assert.equal(status.state, "valid");
	assert.equal(status.transport, "stdio");
	assert.equal(status.detail, "ready to save");
});

test("MCP editor status marks explicit transports as valid", () => {
	const status = getMCPConfigStatus('{"transport":"streamable_http","url":"https://example.com/mcp"}');

	assert.equal(status.state, "valid");
	assert.equal(status.transport, "streamable_http");
});

test("MCP editor status reports invalid JSON without throwing", () => {
	const status = getMCPConfigStatus('{"command":');

	assert.equal(status.state, "invalid");
	assert.equal(status.transport, "unknown");
	assert.match(status.detail, /Unexpected|JSON/i);
});

test("MCP editor divider uses only a stable horizontal rule", () => {
	const divider = buildMCPConfigDivider(80);

	assert.equal(divider.length, 52);
	assert.equal(divider.includes("┌"), false);
	assert.equal(divider.includes("│"), false);
	assert.equal(divider.includes("└"), false);
	assert.match(divider, /^─+$/);
});

test("MCP editor palette keeps active fields and hints visually distinct", () => {
	assert.notEqual(MCP_EDITOR_COLORS.active, MCP_EDITOR_COLORS.inactive);
	assert.notEqual(MCP_EDITOR_COLORS.title, MCP_EDITOR_COLORS.hint);
	assert.notEqual(MCP_EDITOR_COLORS.valid, MCP_EDITOR_COLORS.invalid);
});
