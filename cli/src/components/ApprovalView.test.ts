import assert from "node:assert/strict";
import test from "node:test";
import type React from "react";
import { stripAnsi } from "../utils/terminal";
import {
	ApprovalView,
	buildApprovalDetailLines,
	buildApprovalProgressDots,
	buildApprovalQueueRows,
	formatApprovalTitle,
} from "./ApprovalView";

function visibleChildren(element: React.ReactElement<{ children?: React.ReactNode }>): React.ReactElement<{ children?: React.ReactNode }>[] {
	return (Array.isArray(element.props.children)
		? element.props.children
		: [element.props.children]
	).filter(
		(child): child is React.ReactElement<{ children?: React.ReactNode }> =>
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
		return textContent((node as React.ReactElement<{ children?: React.ReactNode }>).props.children);
	}
	return "";
}

test("formatApprovalTitle renders focused tool position", () => {
	assert.equal(formatApprovalTitle(1, 3), "Tool approval  2 / 3");
});

test("buildApprovalQueueRows renders cursor, mark, risk, and summary", () => {
	const rows = buildApprovalQueueRows([
		{ toolCallId: "call-a", toolName: "exec", command: "run", params: { command: "npm test" } },
		{ toolCallId: "call-b", toolName: "file_edit", command: "edit", params: { file_path: "cli/src/components/ApprovalView.tsx" } },
	], 1, ["call-b"]);

	assert.equal(rows[0].cursor, " ");
	assert.equal(rows[0].mark, "☐");
	assert.equal(rows[0].riskLabel, "EXEC");
	assert.match(rows[0].summary, /npm test/);
	assert.equal(rows[1].cursor, "❯");
	assert.equal(rows[1].mark, "☑");
	assert.equal(rows[1].riskLabel, "WRITE");
	assert.match(rows[1].summary, /ApprovalView\.tsx/);
});

test("buildApprovalProgressDots highlights only the current pending item", () => {
	assert.deepEqual(
		buildApprovalProgressDots([
			{ approvalStatus: "approved" },
			{ approvalStatus: "rejected" },
			{ approvalStatus: "pending" },
			{ approvalStatus: "pending" },
		], 2),
		[
			{ symbol: "●", color: "green" },
			{ symbol: "●", color: "red" },
			{ symbol: "●", color: "yellow" },
			{ symbol: "○", color: "gray" },
		],
	);
});

test("buildApprovalDetailLines keeps ask_questions compact and shows params for normal tools", () => {
	assert.deepEqual(
		buildApprovalDetailLines({
			toolCallId: "ask",
			toolName: "ask_questions",
			command: "ask",
			params: { questions: [{ id: "q1", question: "Q?" }] },
		}).some((line) => line.includes("Q?") || line.includes("questionId") || line.includes("[{")),
		false,
	);

	const lines = buildApprovalDetailLines({
		toolCallId: "exec",
		toolName: "exec",
		command: "run",
		params: { command: "npm test", description: "Run tests" },
	});

	assert.ok(lines.some((line) => line.includes("Command")));
	assert.ok(lines.some((line) => line.includes("npm test")));
});

test("approval view renders one focused tool with stable shortcut hints", () => {
	const view = ApprovalView({
		toolName: "",
		command: "",
		params: {},
		items: [
			{ toolCallId: "call-a", toolName: "exec", command: "run", params: { command: "npm --prefix cli test -- --watch=false --filter=approval-keyboard-with-a-super-long-command-that-wraps-cleanly" } },
			{ toolCallId: "call-b", toolName: "file_read", command: "read", params: { file_path: "README.md" } },
		],
		approvalReviewItems: [
			{ toolCallId: "call-a", toolName: "exec", command: "run", params: {}, approvalStatus: "approved" },
			{ toolCallId: "call-b", toolName: "file_read", command: "read", params: {}, approvalStatus: "pending" },
		],
		cursor: 1,
		columns: 100,
	});

	const text = visibleChildren(view as React.ReactElement<{ children?: React.ReactNode }>)
		.map((child) => textContent(child.props.children))
		.join("\n");

	assert.match(text, /Tool approval  2 \/ 2/);
	assert.match(text, /Progress/);
	assert.match(text, /file_read\.read/);
	assert.match(text, /README\.md/);
	assert.match(text, /Y approve/);
	assert.match(text, /N reject/);
	assert.match(text, /A approve all/);
	assert.match(text, /R reject all/);
	assert.doesNotMatch(text, /Selected details/);
	assert.doesNotMatch(text, /Space mark/);
});

test("approval view renders exec command with compact labels", () => {
	const view = ApprovalView({
		toolName: "exec",
		command: "run",
		params: {
			command: "npm --prefix cli test",
			cwd: "/Users/natsuzora/Documents/gitCode/SlimeBot",
		},
		columns: 100,
	});

	const text = stripAnsi(textContent(view));

	assert.match(text, /command:\s+npm --prefix cli test/);
	assert.match(text, /cwd:\s+\/Users\/natsuzora\/Documents\/gitCode\/SlimeBot/);
	assert.doesNotMatch(text, /command\s{3,}npm --prefix cli test/);
});

test("approval view renders file edit preview with success-style diff gutter", () => {
	const view = ApprovalView({
		toolName: "file_edit",
		command: "edit",
		params: {
			file_path: "cli/src/components/ApprovalView.tsx",
			old_string: "const labelWidth = 10;\n<Box width={10}>",
			new_string: "const labelWidth = 8;\n<Box width={labelWidth}>",
		},
		columns: 100,
	});

	const text = stripAnsi(textContent(view));

	assert.match(text, /file:\s+cli\/src\/components\/ApprovalView\.tsx/);
	assert.match(text, /change:\s+Updated .*ApprovalView\.tsx/);
	assert.match(text, /─{8,}/);
	assert.match(text, /\+ 1 const labelWidth = 8;/);
	assert.match(text, /- 1 const labelWidth = 10;/);
	assert.doesNotMatch(text, /^\+\s{8,}/m);
	assert.doesNotMatch(text, /^-\s{8,}/m);
});
