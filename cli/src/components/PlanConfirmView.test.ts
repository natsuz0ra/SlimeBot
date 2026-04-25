import assert from "node:assert/strict";
import test from "node:test";
import {
	PLAN_CONFIRM_COLORS,
	PLAN_CONFIRM_OPTION_GAP_LINES,
	PLAN_CONFIRM_SECTION_GAP_LINES,
} from "./PlanConfirmView";

test("plan confirmation menu leaves breathing room between sections", () => {
	assert.equal(PLAN_CONFIRM_SECTION_GAP_LINES, 1);
});

test("plan confirmation keeps adjacent action options together", () => {
	assert.equal(PLAN_CONFIRM_OPTION_GAP_LINES, 0);
});

test("plan confirmation palette separates selected, idle, and hint text", () => {
	assert.notEqual(PLAN_CONFIRM_COLORS.selected, PLAN_CONFIRM_COLORS.idle);
	assert.notEqual(PLAN_CONFIRM_COLORS.title, PLAN_CONFIRM_COLORS.hint);
});
