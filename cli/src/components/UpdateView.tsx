import React from "react";
import { Box, Text } from "ink";
import type { UpdateCheckResult, UpdateJobStatus } from "../types.js";
import { renderMarkdownLines } from "../utils/markdownRenderer.js";
import { CLI_ACCENT_COLOR } from "../utils/terminal.js";
import { CLI_THEME } from "../utils/theme.js";

interface UpdateViewProps {
	check: UpdateCheckResult | null;
	job: UpdateJobStatus | null;
	loading: boolean;
	applying: boolean;
	confirming?: boolean;
	columns?: number;
}

interface SummaryInput {
	check: Partial<UpdateCheckResult> | null;
	job: Partial<UpdateJobStatus> | null;
	loading?: boolean;
	confirming?: boolean;
}

const SUMMARY_LABEL_WIDTH = 10;

export function isUpdateActive(job: Partial<UpdateJobStatus> | null): boolean {
	const phase = job?.phase;
	return phase === "checking" || phase === "downloading" || phase === "installing" || phase === "restarting";
}

export function getInitialUpdateJobForView<T extends Partial<UpdateJobStatus> | null>(job: T): T | null {
	return isUpdateActive(job) ? job : null;
}

export function formatUpdateHeaderLine(check: Partial<UpdateCheckResult> | null): string {
	const current = check?.current || "(unknown)";
	const latest = check?.latest || "(unknown)";
	let mode = "latest";
	if (check?.updateAvailable) mode = check.canApply ? "auto" : "manual";
	return check?.updateAvailable ? `Update  ${current} -> ${latest}  ${mode}` : `Update  ${current}  ${mode}`;
}

export function formatUpdateSummaryLines({ check, job, loading = false, confirming = false }: SummaryInput): string[] {
	const lines: string[] = [];
	if (loading) {
		lines.push(formatSummaryLine("Status", "checking for updates"));
		return lines;
	}
	if (check) {
		lines.push(formatSummaryLine("Status", formatUpdateStatusText(check, job, confirming)));
		if (check.releaseUrl) lines.push(formatSummaryLine("Release", check.releaseUrl));
		if (check.reason) lines.push(formatSummaryLine("Reason", check.reason));
		if (check.manualHint && !check.canApply) lines.push(formatSummaryLine("Manual", check.manualHint));
	} else {
		lines.push(formatSummaryLine("Status", "no update check loaded"));
	}
	if (job && job.phase && job.phase !== "idle") {
		const progress = formatUpdateProgressLine(job);
		if (progress) lines.push(progress);
		if (job.manualHint) lines.push(formatSummaryLine("Manual", job.manualHint));
		if (job.error) lines.push(formatSummaryLine("Error", job.error));
	}
	return lines;
}

export function formatUpdateReleaseNotesLines(releaseNotes = "", columns = 80): string[] {
	const notes = releaseNotes.trim();
	if (!notes) return [];
	const rendered = renderMarkdownLines(notes, Math.max(20, columns), true);
	return ["Notes", ...rendered];
}

export function formatUpdateProgressLine(job: Partial<UpdateJobStatus> | null): string {
	if (!job || (job.phase && job.phase !== "downloading")) return "";
	const downloaded = Math.max(0, Math.trunc(job.downloadedBytes || 0));
	const total = Math.max(0, Math.trunc(job.totalBytes || 0));
	if (total > 0) {
		const percent = Math.min(100, Math.max(0, Math.trunc(job.progressPercent ?? downloaded * 100 / total)));
		return formatSummaryLine("Progress", `${renderProgressBar(percent, 18)} ${percent}% (${formatBytes(downloaded)}/${formatBytes(total)})`);
	}
	if (downloaded > 0) return formatSummaryLine("Progress", `${formatBytes(downloaded)} downloaded`);
	return formatSummaryLine("Progress", "in progress");
}

export function formatUpdateHint(canApply: boolean, active: boolean, confirming = false): string {
	if (active) return "Updating... · Esc close";
	if (confirming) return "Y confirm · N cancel · Esc close";
	return canApply ? "C recheck · U update · Esc close" : "C recheck · Esc close";
}

export function UpdateView({ check, job, loading, applying, confirming = false, columns = 80 }: UpdateViewProps): React.ReactElement {
	const active = applying || isUpdateActive(job);
	const canApply = Boolean(check?.canApply) && !active && !loading;
	const titleColor = check?.updateAvailable ? "#facc15" : CLI_ACCENT_COLOR;
	const hint = formatUpdateHint(canApply, active, confirming);
	const lines = formatUpdateSummaryLines({ check, job, loading, confirming });
	const releaseNoteLines = formatUpdateReleaseNotesLines(check?.releaseNotes || "", columns);
	const divider = formatDividerLine(columns);

	return (
		<Box flexDirection="column" paddingX={1} width={Math.min(columns || 80, 96)}>
			<Text bold color={titleColor}>{formatUpdateHeaderLine(check)}</Text>
			<Text color="#334155">{divider}</Text>
			{lines.map((line, index) => (
				<Text key={`${index}-${line}`} color={formatLineColor(line)}>
					{line}
				</Text>
			))}
			{releaseNoteLines.length > 0 && (
				<>
					<Text> </Text>
					{releaseNoteLines.map((line, index) => (
						<Text key={`note-${index}-${line}`} color={index === 0 ? CLI_ACCENT_COLOR : "#cbd5e1"}>
							{line}
						</Text>
					))}
				</>
			)}
			{confirming && (
				<>
					<Text> </Text>
					<Text color="#facc15">Confirm update? SlimeBot may restart while installing.</Text>
				</>
			)}
			{check?.updateAvailable && check.canApply && !active && !confirming && (
				<>
					<Text> </Text>
					<Text color="#94a3b8">Press U to review and confirm the update.</Text>
				</>
			)}
			{active && job?.phase && (job.phase === "installing" || job.phase === "restarting") && (
				<Text color="#94a3b8">The service may restart and disconnect this TUI.</Text>
			)}
			<Text> </Text>
			<Text color={CLI_THEME.hint}>{loading ? "Checking for updates..." : hint}</Text>
		</Box>
	);
}

function formatUpdateStatusText(check: Partial<UpdateCheckResult>, job: Partial<UpdateJobStatus> | null, confirming: boolean): string {
	if (job?.phase && job.phase !== "idle") {
		if (job.phase === "checking") return "checking for updates";
		if (job.phase === "downloading") return job.message || "downloading update";
		if (job.phase === "installing") return job.message || "installing update";
		if (job.phase === "restarting") return job.message || "restarting service";
		if (job.phase === "succeeded") return job.message || "updated";
		if (job.phase === "failed") return job.message || "failed";
	}
	if (!check.updateAvailable) return "latest";
	if (!check.canApply) return "manual update required";
	if (confirming) return "waiting for confirmation";
	return "waiting for confirmation";
}

function formatSummaryLine(label: string, value: string): string {
	return `${label.padEnd(SUMMARY_LABEL_WIDTH, " ")}${value}`;
}

function formatDividerLine(columns: number): string {
	const width = Math.max(24, Math.min(46, (columns || 80) - 2));
	return "-".repeat(width);
}

function formatLineColor(line: string): string {
	if (line.startsWith("Error")) return "#f87171";
	if (line.startsWith("Status")) return "#facc15";
	if (line.startsWith("Progress")) return CLI_ACCENT_COLOR;
	return "#cbd5e1";
}

function renderProgressBar(percent: number, width: number): string {
	const filled = Math.max(0, Math.min(width, Math.trunc(percent * width / 100)));
	return `[${"=".repeat(filled)}${"-".repeat(width - filled)}]`;
}

function formatBytes(value: number): string {
	if (value < 1024) return `${value} B`;
	if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`;
	return `${(value / 1024 / 1024).toFixed(1)} MiB`;
}
