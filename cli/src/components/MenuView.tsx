/**
 * MenuView — shared menu for session / model / skills / mcp / help.
 */

import { Box, Text, useStdout } from "ink";
import type React from "react";
import type { MenuItem, MenuKind } from "../types.js";
import { wrapText } from "../utils/format.js";

interface MenuViewProps {
	title: string;
	items: MenuItem[];
	cursor: number;
	hint: string;
	kind: MenuKind;
	onSelect: (item: MenuItem, index: number) => void;
	onBack: () => void;
	onDelete?: (item: MenuItem, index: number) => void;
	onAdd?: () => void;
	onEdit?: (item: MenuItem, index: number) => void;
	onToggle?: (item: MenuItem, index: number) => void;
}

const MAX_MENU_TITLE_LENGTH = 25;
const MAX_MENU_DESC_LENGTH = 80;

export const MENU_TITLE_GAP_LINES = 1;
export const MENU_ITEM_COLORS = {
	title: "#67e8f9",
	activeCursor: "#22d3ee",
	inactiveCursor: "#64748b",
	activeTitle: "#f8fafc",
	inactiveTitle: "#cbd5e1",
	description: "#94a3b8",
	empty: "#94a3b8",
	hint: "#64748b",
} as const;

export function truncateMenuTitle(
	title: string,
	maxLen = MAX_MENU_TITLE_LENGTH,
): string {
	const normalized = (title ?? "").trim();
	if (normalized.length <= maxLen) return normalized;
	if (maxLen <= 1) return "…";
	return `${normalized.slice(0, maxLen - 1)}…`;
}

export function truncateMenuDescription(
	desc: string,
	maxLen = MAX_MENU_DESC_LENGTH,
): string {
	const normalized = (desc ?? "").replace(/\s+/g, " ").trim();
	if (!normalized) return "(No description)";
	if (normalized.length <= maxLen) return normalized;
	if (maxLen <= 1) return "…";
	return `${normalized.slice(0, maxLen - 1)}…`;
}

export function formatMenuDescriptionLines(
	desc: string,
	terminalWidth: number,
): string[] {
	const text = truncateMenuDescription(desc);
	const lineWidth = Math.max(
		10,
		Math.min(MAX_MENU_DESC_LENGTH, terminalWidth - 2),
	);
	return wrapText(text, lineWidth).split("\n");
}

function menuItemKey(item: MenuItem): string {
	const data = item.data;
	if (
		data &&
		typeof data === "object" &&
		"id" in data &&
		typeof data.id === "string"
	) {
		return data.id;
	}
	return `${item.title}:${item.desc}`;
}

export function MenuView({
	title,
	items,
	cursor,
	hint,
	kind: _kind,
	onSelect: _onSelect,
	onBack: _onBack,
	onDelete: _onDelete,
	onAdd: _onAdd,
	onEdit: _onEdit,
	onToggle: _onToggle,
}: MenuViewProps): React.ReactElement {
	const { stdout } = useStdout();
	const terminalWidth = Math.max(20, stdout?.columns || 80);

	return (
		<Box flexDirection="column">
			<Text bold color={MENU_ITEM_COLORS.title}>
				{title}
			</Text>
			{MENU_TITLE_GAP_LINES > 0 && <Text> </Text>}
			{items.length === 0 ? (
				<Text color={MENU_ITEM_COLORS.empty}>(empty)</Text>
			) : (
				items.map((item, i) => (
					<Box key={menuItemKey(item)} flexDirection="column">
						<Text>
							<Text
								color={
									i === cursor
										? MENU_ITEM_COLORS.activeCursor
										: MENU_ITEM_COLORS.inactiveCursor
								}
							>
								{i === cursor ? "\u276F" : " "}
							</Text>
							<Text> </Text>
							<Text
								bold={i === cursor}
								color={
									i === cursor
										? MENU_ITEM_COLORS.activeTitle
										: MENU_ITEM_COLORS.inactiveTitle
								}
							>
								{truncateMenuTitle(item.title)}
							</Text>
						</Text>
						{formatMenuDescriptionLines(item.desc, terminalWidth).map(
							(line) => (
								<Text
									key={`${item.title}-desc-${line}`}
									color={MENU_ITEM_COLORS.description}
								>
									{`  ${line}`}
								</Text>
							),
						)}
					</Box>
				))
			)}
			{hint && (
				<Box flexDirection="column">
					<Text> </Text>
					<Text color={MENU_ITEM_COLORS.hint}>{hint}</Text>
				</Box>
			)}
		</Box>
	);
}
