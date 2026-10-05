import React from "react";
import { Box, Text, useInput } from "ink";
import type { CooperativeSnapshot } from "../types/cooperative.js";
import { TextInput } from "./TextInput.js";
import { cooperativeRows, cooperativeWorkspaceLines, cooperativeReportPage } from "../utils/cooperative.js";
interface CooperativeViewProps {
  snapshot: CooperativeSnapshot | null;
  refresh: () => void;
  initialTab: number;
  columns: number;
  height: number;
  loading?: boolean;
  error?: string;
  onClose: () => void;
  onCommand: (command: string) => Promise<void>;
  loadReport: (agentId: string, turnId: string) => Promise<string>;
}
export function CooperativeWorkspace({ lines, columns }: {
  lines: ReturnType<typeof cooperativeWorkspaceLines>;
  columns: number;
}): React.ReactElement {
  return (
    <Box flexDirection="column" paddingX={1} width={Math.min(columns, 96)}>
      {lines.map((line, index) => (
        <Text key={index} color={line.color} bold={Boolean(line.bold)} wrap="truncate">
          {line.text || " "}
        </Text>
      ))}
    </Box>
  );
}
export default function CooperativeView(props: CooperativeViewProps): React.ReactElement {
  const { snapshot, refresh, initialTab, columns, height, onClose, onCommand, loadReport } = props;
  const [tab, setTab] = React.useState(initialTab);
  const [selectedId, setSelectedId] = React.useState("");
  const [detail, setDetail] = React.useState(false);
  const [detailId, setDetailId] = React.useState("");
  const [offset, setOffset] = React.useState(0);
  const [report, setReport] = React.useState<string | undefined>();
  const [draft, setDraft] = React.useState<string | null>(null);
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState("");
  const revision = React.useRef(0);
  const rows = cooperativeRows(snapshot, tab);
  const cursor = Math.max(0, rows.findIndex(row => row.id === selectedId));
  const selected = rows[cursor];
  const resetDetail = () => {
    revision.current++;
    setDetail(false);
    setOffset(0);
    setReport(undefined);
    setError("");
    setBusy(false);
  };
  React.useEffect(() => { setTab(initialTab); setSelectedId(""); resetDetail(); setDraft(null); }, [initialTab]);
  React.useEffect(() => () => { revision.current++; }, []);
  React.useEffect(() => {
    if (detail && selected?.id !== detailId) resetDetail();
  }, [selected?.id, detailId, detail]);
  const changeTab = (next: number) => { setTab((next + 4) % 4); setSelectedId(""); resetDetail(); };
  const openDetail = async () => {
    if (!selected)
      return;
    setSelectedId(selected.id);
    setDetailId(selected.id);
    setDetail(true);
    setOffset(0);
    setReport(undefined);
    setError("");
    const epoch = ++revision.current;
    if (tab === 0 && selected.turnId) {
      setBusy(true);
      try {
        const text = await loadReport(selected.id, selected.turnId);
        if (revision.current === epoch)
          setReport(text);
      } catch (e) {
        if (revision.current === epoch)
          setError((e as Error).message);
      } finally {
        if (revision.current === epoch)
          setBusy(false);
      }
    }
  };
  const submit = async () => {
    if (draft === null || !draft.trim() || busy)
      return;
    setBusy(true);
    setError("");
    try {
      await onCommand(draft.replace(/^\s*\/agents(?:\s+|$)/, ""));
      setDraft(null);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  useInput((input, key) => {
    if (draft !== null)
      return;
    // Ink can also mark a plain Escape as Meta. Handle it before modifiers.
    if (key.escape) {
      if (detail)
        resetDetail();
      else
        onClose();
      return;
    }
    if (key.ctrl || key.meta)
      return;
    if (busy)
      return;
    if (key.leftArrow || key.rightArrow || key.tab) {
      changeTab(tab + (key.leftArrow || key.shift ? -1 : 1));
      return;
    }
    if (input === "r" || input === "R") {
      refresh();
      return;
    }
    if (input === ":") {
      setDraft("");
      return;
    }
    if (detail) {
      const page = cooperativeReportPage(report ?? selected?.report ?? "", Math.min(columns, 96) - 2, Math.max(1, height - 9 - ((props.loading || error || props.error) ? 1 : 0)), offset);
      if (key.upArrow || key.downArrow)
        setOffset(Math.max(0, page.offset + (key.downArrow ? 1 : -1)));
      if (key.pageUp || key.pageDown)
        setOffset(Math.max(0, page.offset + (key.pageDown ? 1 : -1) * page.size));
      if (key.home)
        setOffset(0);
      if (key.end)
        setOffset(Math.max(0, page.total - page.size));
      return;
    }
    if (key.upArrow || key.downArrow) {
      setSelectedId(rows[Math.max(0, Math.min(rows.length - 1, cursor + (key.downArrow ? 1 : -1)))]?.id || "");
      return;
    }
    if (key.return) {
      void openDetail();
      return;
    }
    if (tab === 1 && input === "n") {
      setDraft("task create ");
      return;
    }
    if (!selected)
      return;
    if (tab === 0 && input === "s")
      setDraft(`send ${selected.id} `);
    if (tab === 0 && input === "x")
      setDraft(`stop ${selected.id}`);
    if (tab === 2 && input === "v")
      setDraft(`artifact ${selected.id} validate `);
    if (tab === 2 && input === "i")
      setDraft(`artifact ${selected.id} integrate`);
    if (tab === 3 && (input === "y" || input === "n"))
      setDraft(`approve ${selected.id} ${input === "y" ? "yes" : "no"}`);
  });
  const width = Math.min(columns, 96);
  const lines = cooperativeWorkspaceLines({
    snapshot, tab, cursor, columns: width,
    height: Math.max(8, height - (draft !== null ? 2 : 0)),
    detail, offset, report,
    loading: props.loading || busy,
    error: error || props.error,
    command: draft !== null,
  });
  return (
    <Box flexDirection="column">
      <CooperativeWorkspace lines={lines} columns={width} />
      {draft !== null && (
        <Box flexDirection="column" paddingX={1} width={width}>
          <Text color="#94a3b8" wrap="truncate">/agents · review command, then Enter</Text>
          <TextInput
            value={draft} onChange={setDraft}
            onSubmit={() => void submit()}
            onEscape={() => { if (!busy) { setDraft(null); setError(""); } }}
            columns={width - 2} focus={!busy} multiline={false} maxVisibleLines={1}
          />
        </Box>
      )}
    </Box>
  );
}
