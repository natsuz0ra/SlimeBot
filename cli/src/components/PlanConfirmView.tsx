/**
 * PlanConfirmView - plan confirmation dialog for CLI.
 * Two options: Execute Plan + Feedback input, ESC to cancel.
 */

import { Box, Text } from "ink";
import type React from "react";
import { TextInput } from "./TextInput.js";

const PLACEHOLDER = "Type feedback to modify plan...";

export const PLAN_CONFIRM_SECTION_GAP_LINES = 1;
export const PLAN_CONFIRM_TITLE_GAP_LINES = PLAN_CONFIRM_SECTION_GAP_LINES;
export const PLAN_CONFIRM_OPTION_GAP_LINES = 0;
export const PLAN_CONFIRM_HINT_GAP_LINES = PLAN_CONFIRM_SECTION_GAP_LINES;
export const PLAN_CONFIRM_COLORS = {
	title: "#a78bfa",
	selectedCursor: "#22d3ee",
	selected: "#f8fafc",
	idle: "#cbd5e1",
	feedback: "#e2e8f0",
	placeholder: "#94a3b8",
	hint: "#64748b",
} as const;

interface PlanConfirmViewProps {
	cursor: number;
	feedback: string;
	feedbackKey: number;
	onFeedbackChange: (value: string) => void;
	onFeedbackSubmit: (value: string) => void;
	onEscape: () => void;
	columns: number;
}

export function PlanConfirmView({
	cursor,
	feedback,
	feedbackKey,
	onFeedbackChange,
	onFeedbackSubmit,
	onEscape,
	columns,
}: PlanConfirmViewProps): React.ReactElement {
	const inputFocused = cursor === 1;

	return (
		<Box flexDirection="column">
			<Text bold color={PLAN_CONFIRM_COLORS.title}>
				Plan Generated - Choose an action
			</Text>
			{PLAN_CONFIRM_TITLE_GAP_LINES > 0 && <Text> </Text>}
			<Text>
				{cursor === 0 ? (
					<Text>
						<Text bold color={PLAN_CONFIRM_COLORS.selectedCursor}>
							{"  > "}
						</Text>
						<Text bold color={PLAN_CONFIRM_COLORS.selected}>
							{"1. Execute Plan"}
						</Text>
					</Text>
				) : (
					<Text color={PLAN_CONFIRM_COLORS.idle}>{"    1. Execute Plan"}</Text>
				)}
			</Text>
			{PLAN_CONFIRM_OPTION_GAP_LINES > 0 && <Text> </Text>}
			<Box>
				{inputFocused ? (
					<Text>
						<Text bold color={PLAN_CONFIRM_COLORS.selectedCursor}>
							{"  > "}
						</Text>
						<Text bold color={PLAN_CONFIRM_COLORS.selected}>
							{"2. "}
						</Text>
					</Text>
				) : (
					<Text color={PLAN_CONFIRM_COLORS.idle}>{"    2. "}</Text>
				)}
				{inputFocused ? (
					<Box>
						<TextInput
							key={feedbackKey}
							value={feedback}
							onChange={onFeedbackChange}
							onSubmit={onFeedbackSubmit}
							onEscape={onEscape}
							focus={true}
							columns={Math.max(20, columns - 8)}
							cursorChar={feedback ? undefined : ""}
						/>
						{!feedback && (
							<Text color={PLAN_CONFIRM_COLORS.placeholder}>{PLACEHOLDER}</Text>
						)}
					</Box>
				) : (
					<Text
						color={
							feedback
								? PLAN_CONFIRM_COLORS.feedback
								: PLAN_CONFIRM_COLORS.placeholder
						}
					>
						{feedback || PLACEHOLDER}
					</Text>
				)}
			</Box>
			{PLAN_CONFIRM_HINT_GAP_LINES > 0 && <Text> </Text>}
			<Text color={PLAN_CONFIRM_COLORS.hint}>
				Arrow keys to navigate | Enter to select | Esc to cancel
			</Text>
		</Box>
	);
}
