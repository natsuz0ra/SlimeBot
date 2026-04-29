import assert from "node:assert/strict";
import test from "node:test";
import {
	MENU_ITEM_COLORS,
	MENU_TITLE_GAP_LINES,
	formatMenuDescriptionLines,
	truncateMenuDescription,
	truncateMenuTitle,
} from "./MenuView";

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
