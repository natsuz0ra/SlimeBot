import assert from "node:assert/strict";
import test from "node:test";
import type React from "react";
import {
	PLAN_CONFIRM_COLORS,
	PLAN_CONFIRM_HINT_GAP_LINES,
	PLAN_CONFIRM_OPTION_GAP_LINES,
	PLAN_CONFIRM_SECTION_GAP_LINES,
	PLAN_CONFIRM_TITLE_GAP_LINES,
	PlanConfirmView,
} from "./PlanConfirmView";

function visibleChildren(element: React.ReactElement): React.ReactElement[] {
	return (Array.isArray(element.props.children)
		? element.props.children
		: [element.props.children]
	).filter(
		(child): child is React.ReactElement =>
			typeof child === "object" && child !== null,
	);
}

function textContent(node: React.ReactNode): string {
	if (typeof node === "string" || typeof node === "number") {
		return String(node);
	}
	if (!node || typeof node !== "object") {
		return "";
	}
	if (Array.isArray(node)) {
		return node.map(textContent).join("");
	}
	if ("props" in node) {
		return textContent((node as React.ReactElement).props.children);
	}
	return "";
}

test("plan confirmation menu leaves breathing room between sections", () => {
	assert.equal(PLAN_CONFIRM_SECTION_GAP_LINES, 1);
	assert.equal(PLAN_CONFIRM_TITLE_GAP_LINES, 1);
	assert.equal(PLAN_CONFIRM_HINT_GAP_LINES, 1);
});

test("plan confirmation keeps adjacent action options together", () => {
	assert.equal(PLAN_CONFIRM_OPTION_GAP_LINES, 0);
});

test("plan confirmation renders a title gap and no option gap", () => {
	const view = PlanConfirmView({
		cursor: 0,
		feedback: "",
		feedbackKey: 0,
		onFeedbackChange: () => {},
		onFeedbackSubmit: () => {},
		onEscape: () => {},
		columns: 80,
	});

	const lines = visibleChildren(view).map((child) =>
		textContent(child.props.children),
	);

	assert.equal(lines[0], "Plan Generated - Choose an action");
	assert.equal(lines[1], " ");
	assert.equal(lines[2], "  > 1. Execute Plan");
	assert.equal(lines[3], "    2. Type feedback to modify plan...");
	assert.equal(lines[4], " ");
	assert.equal(lines[5], "Arrow keys to navigate | Enter to select | Esc to cancel");
});

test("plan confirmation palette separates selected, idle, and hint text", () => {
	assert.notEqual(PLAN_CONFIRM_COLORS.selected, PLAN_CONFIRM_COLORS.idle);
	assert.notEqual(PLAN_CONFIRM_COLORS.title, PLAN_CONFIRM_COLORS.hint);
});
