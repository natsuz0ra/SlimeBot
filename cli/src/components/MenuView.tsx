/**
 * MenuView — shared menu for session / model / skills / mcp / help.
 */

import { Box, Text, useStdout } from "ink";
import type React from "react";
import type { MenuItem } from "../types.js";
import { wrapText } from "../utils/format.js";
import { CLI_ACCENT_COLOR } from "../utils/terminal.js";
import { stringWidth } from "../utils/stringWidth.js";
import { getGraphemeSegmenter } from "../utils/intl.js";

interface MenuViewProps {
	title: string;
	items: MenuItem[];
	cursor: number;
	hint: string;
	maxVisibleItems?: number;
	maxHeight?: number;
	columns?: number;
}

const MAX_MENU_TITLE_LENGTH = 25;
const MAX_MENU_DESC_LENGTH = 80;

export const MENU_TITLE_GAP_LINES = 1;
export const MENU_VISIBLE_LIMIT = 5;
export const CLI_HINT_COLOR = "#64748b";
export const MENU_ITEM_COLORS = {
	title: CLI_ACCENT_COLOR,
	activeCursor: CLI_ACCENT_COLOR,
	inactiveCursor: "#64748b",
	activeTitle: "#f8fafc",
	inactiveTitle: "#cbd5e1",
	description: "#94a3b8",
	empty: "#94a3b8",
	hint: CLI_HINT_COLOR,
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

function clampMenuCursor(cursor: number, length: number): number {
	if (length <= 0) return 0;
	return Math.max(0, Math.min(length - 1, cursor));
}

export function getVisibleMenuItems(
	items: MenuItem[],
	cursor: number,
	maxVisible = items.length,
): { items: MenuItem[]; startIndex: number } {
	if (items.length === 0 || maxVisible <= 0) {
		return { items: [], startIndex: 0 };
	}

	const visibleCount = Math.min(maxVisible, items.length);
	const selected = clampMenuCursor(cursor, items.length);
	const maxStart = items.length - visibleCount;
	const startIndex = Math.max(
		0,
		Math.min(selected - visibleCount + 1, maxStart),
	);

	return {
		items: items.slice(startIndex, startIndex + visibleCount),
		startIndex,
	};
}

function fitMenuLine(value: string, columns: number): string {
	const text = value.replace(/\x1b\[[0-?]*[ -/]*[@-~]/g, "").replace(/[\x00-\x1f\x7f-\x9f]/g, " ");
	if (stringWidth(text) <= columns) return text;
	let result = "";
	for (const { segment } of getGraphemeSegmenter().segment(text)) {
		if (stringWidth(result + segment) > columns - 1) break;
		result += segment;
	}
	return result + "…";
}

// Budget display rows, including descriptions and footer, rather than merely
// counting items. The selected model always remains inside the visible window.
export function getMenuViewport(items: MenuItem[], cursor: number, columns: number, maxHeight: number, hint: string, maxVisibleItems = items.length) {
	const width = Math.max(8, columns);
	const height = Math.max(6, Math.floor(maxHeight));
	const selected = clampMenuCursor(cursor, items.length);
	const allHints = hint ? wrapText(hint, width).split("\n") : [];
	const hintLimit = Math.max(0, height - 5);
	const hintLines = allHints.slice(0, hintLimit);
	if (allHints.length > hintLines.length && hintLines.length) {
		hintLines[hintLines.length - 1] = fitMenuLine(hintLines.at(-1)! + "…", width);
	}
	const bodyHeight = Math.max(1, height - 2 - 1 - (hintLines.length ? hintLines.length + 1 : 0));
	const formatted = items.map((item, index) => {
		const allLines = formatMenuDescriptionLines(item.desc, width);
		const descriptionLines = allLines.slice(0, Math.min(2, bodyHeight - 1));
		if (allLines.length > descriptionLines.length && descriptionLines.length) {
			descriptionLines[descriptionLines.length - 1] = fitMenuLine(descriptionLines.at(-1)! + "…", width - 2);
		}
		return { item, index, title: fitMenuLine(item.title, width - 2), descriptionLines };
	});
	let startIndex = selected;
	let endIndex = Math.min(items.length, selected + 1);
	let usedRows = formatted[selected] ? 1 + formatted[selected]!.descriptionLines.length : 0;
	const itemLimit = Math.max(1, maxVisibleItems);
	while (startIndex > 0 && endIndex - startIndex < itemLimit) {
		const cost = 1 + formatted[startIndex - 1]!.descriptionLines.length;
		if (usedRows + cost > bodyHeight) break;
		usedRows += cost;
		startIndex--;
	}
	while (endIndex < items.length && endIndex - startIndex < itemLimit) {
		const cost = 1 + formatted[endIndex]!.descriptionLines.length;
		if (usedRows + cost > bodyHeight) break;
		usedRows += cost;
		endIndex++;
	}
	return {
		items: formatted.slice(startIndex, endIndex), startIndex, endIndex,
		hintLines,
		position: items.length ? fitMenuLine(`${selected + 1}/${items.length} · ↑↓ · PgUp/PgDn · Enter · Esc`, width) : "",
	};
}

export function MenuView({
	title,
	items,
	cursor,
	hint,
	maxVisibleItems,
	maxHeight,
	columns,
}: MenuViewProps): React.ReactElement {
	const { stdout } = useStdout();
	const terminalWidth = Math.max(20, columns || stdout?.columns || 80);
	const visible = getVisibleMenuItems(
		items,
		cursor,
		maxVisibleItems ?? items.length,
	);
	const viewport = maxHeight === undefined ? undefined : getMenuViewport(items, cursor, terminalWidth, maxHeight, hint, maxVisibleItems);
	const rows = viewport?.items ?? visible.items.map((item, index) => ({
		item, index: visible.startIndex + index,
		title: truncateMenuTitle(item.title),
		descriptionLines: formatMenuDescriptionLines(item.desc, terminalWidth),
	}));

	return (
		<Box flexDirection="column" width={viewport ? terminalWidth : undefined}>
			<Text bold color={MENU_ITEM_COLORS.title} wrap={viewport ? "truncate" : "wrap"}>
				{title}
			</Text>
			{MENU_TITLE_GAP_LINES > 0 && <Text> </Text>}
			{items.length === 0 ? (
				<Text color={MENU_ITEM_COLORS.empty}>(empty)</Text>
			) : (
				rows.map(({ item, index, title: itemTitle, descriptionLines }) => {
					const selected = index === clampMenuCursor(cursor, items.length);
					return (
						<Box key={menuItemKey(item)} flexDirection="column">
							<Text wrap={viewport ? "truncate" : "wrap"}>
								<Text
									color={
										selected
											? MENU_ITEM_COLORS.activeCursor
											: MENU_ITEM_COLORS.inactiveCursor
									}
								>
									{selected ? "\u276F" : " "}
								</Text>
								<Text> </Text>
								<Text
									bold={selected}
									color={
										selected
											? MENU_ITEM_COLORS.activeTitle
											: MENU_ITEM_COLORS.inactiveTitle
									}
								>
									{itemTitle}
								</Text>
							</Text>
							{descriptionLines.map(
								(line, lineIndex) => (
									<Text
										key={`${item.title}-desc-${lineIndex}`}
										color={MENU_ITEM_COLORS.description}
										wrap={viewport ? "truncate" : "wrap"}
									>
										{`  ${line}`}
									</Text>
								),
							)}
						</Box>
					);
				})
			)}
			{viewport?.position && <Text color={MENU_ITEM_COLORS.description} wrap="truncate">{viewport.position}</Text>}
			{(viewport ? viewport.hintLines.length > 0 : hint) && (
				<Box flexDirection="column">
					<Text> </Text>
					{(viewport?.hintLines ?? [hint]).map((line, index) => <Text key={index} color={MENU_ITEM_COLORS.hint} wrap={viewport ? "truncate" : "wrap"}>{line}</Text>)}
				</Box>
			)}
		</Box>
	);
}
