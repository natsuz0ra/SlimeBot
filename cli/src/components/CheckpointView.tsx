import React, { useCallback, useEffect, useMemo, useState } from "react";
import { Box, Text, useInput } from "ink";
import type {
  Checkpoint,
  CheckpointDetail,
  CheckpointDiff,
  CheckpointRollbackMode,
} from "../types.js";
import { renderColorDiff, type ColorDiffLine } from "../native/colorDiff.js";
import { formatTimestamp } from "../utils/format.js";
import { CLI_THEME } from "../utils/theme.js";

export interface CheckpointViewProps {
  sessionId: string;
  busy: boolean;
  columns: number;
  height: number;
  listCheckpoints: (sessionId: string) => Promise<Checkpoint[]>;
  getCheckpoint: (sessionId: string, checkpointId: string) => Promise<CheckpointDetail>;
  diffCheckpoint: (sessionId: string, checkpointId: string, filePath: string) => Promise<CheckpointDiff>;
  rollbackCheckpoint: (sessionId: string, checkpointId: string, mode: CheckpointRollbackMode) => Promise<void>;
  onClose: () => void;
  onRolledBack?: (message: string) => void;
}

type Stage = "list" | "files" | "diff" | "confirm";

export function parseUnifiedDiffLines(rawDiff: string): ColorDiffLine[] {
  const result: ColorDiffLine[] = [];
  let oldLine = 0;
  let newLine = 0;
  for (const line of rawDiff.replace(/\r\n/g, "\n").split("\n")) {
    const hunk = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(line);
    if (hunk) {
      oldLine = Number(hunk[1]);
      newLine = Number(hunk[2]);
      continue;
    }
    if (line.startsWith("---") || line.startsWith("+++") || line.startsWith("diff --git") || line.startsWith("index ")) {
      continue;
    }
    if (line.startsWith("+")) {
      result.push({ kind: "added", newLine: newLine++, text: line.slice(1) });
    } else if (line.startsWith("-")) {
      result.push({ kind: "removed", oldLine: oldLine++, text: line.slice(1) });
    } else if (line.startsWith(" ")) {
      result.push({ kind: "context", oldLine: oldLine++, newLine: newLine++, text: line.slice(1) });
    }
  }
  return result;
}

function checkpointKindLabel(kind: Checkpoint["kind"]): string {
  if (kind === "undo") return "undo";
  if (kind === "restore") return "restore";
  return "turn";
}

function fileChangeBadge(kind: "added" | "modified" | "deleted"): { symbol: string; color: string } {
  if (kind === "added") return { symbol: "+", color: CLI_THEME.success };
  if (kind === "deleted") return { symbol: "-", color: CLI_THEME.error };
  return { symbol: "~", color: CLI_THEME.accent };
}

export function CheckpointView({
  sessionId,
  busy,
  columns,
  height,
  listCheckpoints,
  getCheckpoint,
  diffCheckpoint,
  rollbackCheckpoint,
  onClose,
  onRolledBack,
}: CheckpointViewProps): React.ReactElement {
  const [stage, setStage] = useState<Stage>("list");
  const [checkpoints, setCheckpoints] = useState<Checkpoint[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [statusMessage, setStatusMessage] = useState("");
  const [cursor, setCursor] = useState(0);
  const [detail, setDetail] = useState<CheckpointDetail | null>(null);
  const [fileCursor, setFileCursor] = useState(0);
  const [diff, setDiff] = useState<CheckpointDiff | null>(null);
  const [diffPath, setDiffPath] = useState("");
  const [diffScroll, setDiffScroll] = useState(0);
  const [pendingMode, setPendingMode] = useState<CheckpointRollbackMode | null>(null);
  const [rollingBack, setRollingBack] = useState(false);

  const loadList = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const items = await listCheckpoints(sessionId);
      setCheckpoints(items);
      setCursor((prev) => Math.max(0, Math.min(prev, Math.max(0, items.length - 1))));
    } catch (err) {
      setError((err as Error).message || "Failed to load checkpoints.");
    } finally {
      setLoading(false);
    }
  }, [listCheckpoints, sessionId]);

  useEffect(() => {
    void loadList();
  }, [loadList]);

  const selectedCheckpoint = checkpoints[cursor];

  const openDetail = useCallback(async (cp: Checkpoint) => {
    setLoading(true);
    setError("");
    try {
      const loaded = await getCheckpoint(sessionId, cp.id);
      setDetail(loaded);
      setFileCursor(0);
      setStage("files");
    } catch (err) {
      setError((err as Error).message || "Failed to load checkpoint files.");
    } finally {
      setLoading(false);
    }
  }, [getCheckpoint, sessionId]);

  const openDiff = useCallback(async (cpId: string, filePath: string) => {
    setLoading(true);
    setError("");
    try {
      const loaded = await diffCheckpoint(sessionId, cpId, filePath);
      setDiff(loaded);
      setDiffPath(filePath);
      setDiffScroll(0);
      setStage("diff");
    } catch (err) {
      setError((err as Error).message || "Failed to load file diff.");
    } finally {
      setLoading(false);
    }
  }, [diffCheckpoint, sessionId]);

  const renderedDiffLines = useMemo(() => {
    if (!diff) return [];
    if (diff.binary) return ["(Binary file — diff preview unavailable)"];
    const parsed = parseUnifiedDiffLines(diff.diff || "");
    if (parsed.length === 0) return (diff.diff || "(No diff lines)").split("\n");
    return renderColorDiff({
      filePath: diffPath,
      lines: parsed,
      width: Math.max(24, columns - 2),
    });
  }, [columns, diff, diffPath]);

  const bodyRows = Math.max(4, height - 6);

  const confirmRollback = useCallback(async () => {
    const target = stage === "confirm" ? (detail ?? selectedCheckpoint) : undefined;
    if (!target || !pendingMode || rollingBack) return;
    setRollingBack(true);
    setError("");
    try {
      await rollbackCheckpoint(sessionId, target.id, pendingMode);
      const label = pendingMode === "undo" ? "Undid checkpoint" : "Restored workspace to checkpoint";
      const message = `${label} ${target.id.slice(0, 8)} (${target.fileCount} file${target.fileCount === 1 ? "" : "s"}).`;
      setStatusMessage(message);
      onRolledBack?.(message);
      setPendingMode(null);
      setStage("list");
      await loadList();
    } catch (err) {
      setError((err as Error).message || "Rollback failed.");
      setPendingMode(null);
      setStage(detail ? "files" : "list");
    } finally {
      setRollingBack(false);
    }
  }, [detail, loadList, onRolledBack, pendingMode, rollbackCheckpoint, rollingBack, selectedCheckpoint, sessionId, stage]);

  useInput((input, key) => {
    if (rollingBack) return;

    if (stage === "confirm") {
      if (input === "y" || input === "Y" || key.return) {
        void confirmRollback();
        return;
      }
      if (input === "n" || input === "N" || key.escape) {
        setPendingMode(null);
        setStage(detail ? "files" : "list");
      }
      return;
    }

    if (stage === "diff") {
      const maxScroll = Math.max(0, renderedDiffLines.length - bodyRows);
      if (key.escape || input === "b" || input === "B") {
        setStage("files");
        return;
      }
      if (key.upArrow) setDiffScroll((prev) => Math.max(0, prev - 1));
      else if (key.downArrow) setDiffScroll((prev) => Math.min(maxScroll, prev + 1));
      else if (key.pageUp) setDiffScroll((prev) => Math.max(0, prev - bodyRows));
      else if (key.pageDown) setDiffScroll((prev) => Math.min(maxScroll, prev + bodyRows));
      return;
    }

    if (stage === "files") {
      const files = detail?.files ?? [];
      if (key.escape) {
        setDetail(null);
        setStage("list");
        return;
      }
      if (key.upArrow) {
        setFileCursor((prev) => Math.max(0, prev - 1));
        return;
      }
      if (key.downArrow) {
        setFileCursor((prev) => Math.min(Math.max(0, files.length - 1), prev + 1));
        return;
      }
      if ((key.return || input === "d" || input === "D") && detail && files[fileCursor]) {
        void openDiff(detail.id, files[fileCursor].path);
        return;
      }
      if ((input === "u" || input === "U" || input === "r" || input === "R") && detail && !detail.unavailable) {
        if (busy) {
          setError("Rollback is disabled while a turn or background task is running.");
          return;
        }
        setPendingMode(input.toLowerCase() === "u" ? "undo" : "restore");
        setStage("confirm");
      }
      return;
    }

    // stage === "list"
    if (key.escape) {
      onClose();
      return;
    }
    if (input === "r" || input === "R") {
      if (key.shift && selectedCheckpoint && !selectedCheckpoint.unavailable) {
        if (busy) {
          setError("Rollback is disabled while a turn or background task is running.");
          return;
        }
        setDetail(null);
        setPendingMode("restore");
        setStage("confirm");
        return;
      }
      void loadList();
      return;
    }
    if ((input === "u" || input === "U") && selectedCheckpoint && !selectedCheckpoint.unavailable) {
      if (busy) {
        setError("Rollback is disabled while a turn or background task is running.");
        return;
      }
      setDetail(null);
      setPendingMode("undo");
      setStage("confirm");
      return;
    }
    if (key.upArrow) {
      setCursor((prev) => Math.max(0, prev - 1));
      return;
    }
    if (key.downArrow) {
      setCursor((prev) => Math.min(Math.max(0, checkpoints.length - 1), prev + 1));
      return;
    }
    if (key.return && selectedCheckpoint && !selectedCheckpoint.unavailable) {
      void openDetail(selectedCheckpoint);
    }
  });

  const visibleStart = Math.max(0, Math.min(cursor - Math.floor(bodyRows / 2), Math.max(0, checkpoints.length - bodyRows)));
  const visibleCheckpoints = checkpoints.slice(visibleStart, visibleStart + bodyRows);

  return (
    <Box flexDirection="column" width={columns}>
      <Box justifyContent="space-between">
        <Text bold color={CLI_THEME.accent}>
          {stage === "list"
            ? "File Checkpoints"
            : stage === "files"
              ? `Checkpoint ${detail?.id.slice(0, 8) || ""} · Files`
              : stage === "diff"
                ? `Diff · ${diffPath}`
                : "Confirm Rollback"}
        </Text>
        {busy && <Text color={CLI_THEME.warning}>workspace busy</Text>}
      </Box>
      <Text> </Text>

      {error ? <Text color={CLI_THEME.error}>{error}</Text> : null}
      {statusMessage ? <Text color={CLI_THEME.success}>{statusMessage}</Text> : null}

      {loading ? (
        <Text color={CLI_THEME.secondary}>Loading...</Text>
      ) : stage === "confirm" ? (
        <Box flexDirection="column">
          <Text color={CLI_THEME.warning} bold>
            {pendingMode === "undo"
              ? `Undo changes recorded in checkpoint ${(detail ?? selectedCheckpoint)?.id.slice(0, 8)}?`
              : `Restore workspace files to checkpoint ${(detail ?? selectedCheckpoint)?.id.slice(0, 8)}?`}
          </Text>
          <Text color={CLI_THEME.secondary}>
            This creates a new rollback checkpoint so you can undo it later.
          </Text>
          <Text> </Text>
          <Text color={CLI_THEME.hint} wrap="truncate">Y / Enter confirm · N / Esc cancel</Text>
        </Box>
      ) : stage === "diff" ? (
        <Box flexDirection="column">
          {diff?.truncated && <Text color={CLI_THEME.warning}>Diff truncated for large file.</Text>}
          {renderedDiffLines.slice(diffScroll, diffScroll + bodyRows).map((line, idx) => (
            <Text key={idx} wrap="truncate">{line}</Text>
          ))}
          <Text> </Text>
          <Text color={CLI_THEME.hint} wrap="truncate">
            Lines {Math.min(renderedDiffLines.length, diffScroll + 1)}-{Math.min(renderedDiffLines.length, diffScroll + bodyRows)}/{renderedDiffLines.length} · ↑↓ / PgUp / PgDn scroll · Esc back
          </Text>
        </Box>
      ) : stage === "files" ? (
        <Box flexDirection="column">
          {!detail || detail.files.length === 0 ? (
            <Text color={CLI_THEME.secondary}>(No file changes in this checkpoint)</Text>
          ) : (
            detail.files.slice(0, bodyRows).map((file, idx) => {
              const selected = idx === fileCursor;
              const badge = fileChangeBadge(file.kind);
              return (
                <Text key={file.path} wrap="truncate">
                  <Text color={selected ? CLI_THEME.accent : CLI_THEME.muted}>{selected ? "❯ " : "  "}</Text>
                  <Text color={badge.color} bold>{badge.symbol} </Text>
                  <Text bold={selected}>{file.path}</Text>
                  <Text color={CLI_THEME.secondary}> ({file.kind})</Text>
                </Text>
              );
            })
          )}
          <Text> </Text>
          <Text color={CLI_THEME.hint} wrap="truncate">
            ↑↓ navigate · Enter/D diff · U undo · R restore · Esc back
          </Text>
        </Box>
      ) : (
        <Box flexDirection="column">
          {checkpoints.length === 0 ? (
            <Text color={CLI_THEME.secondary}>No file checkpoints recorded for this session yet.</Text>
          ) : (
            visibleCheckpoints.map((cp, idx) => {
              const absoluteIdx = visibleStart + idx;
              const selected = absoluteIdx === cursor;
              return (
                <Text key={cp.id} wrap="truncate">
                  <Text color={selected ? CLI_THEME.accent : CLI_THEME.muted}>{selected ? "❯ " : "  "}</Text>
                  <Text bold={selected}>{cp.id.slice(0, 8)}</Text>
                  <Text color={CLI_THEME.secondary}> · {checkpointKindLabel(cp.kind)} · {cp.fileCount} file{cp.fileCount === 1 ? "" : "s"} · {formatTimestamp(cp.createdAt)}</Text>
                  {cp.unavailable ? <Text color={CLI_THEME.error}> [unavailable]</Text> : null}
                  {cp.limited ? <Text color={CLI_THEME.warning}> [partial]</Text> : null}
                </Text>
              );
            })
          )}
          <Text> </Text>
          <Text color={CLI_THEME.hint} wrap="truncate">
            ↑↓ navigate · Enter files · U undo · Shift+R restore · R refresh · Esc close
          </Text>
        </Box>
      )}
    </Box>
  );
}
