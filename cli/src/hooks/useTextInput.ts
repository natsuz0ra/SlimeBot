import { useEffect, useMemo, useState } from "react";
import stripAnsi from "strip-ansi";
import type { Key } from "ink";
import {
  Cursor,
  getLastKill,
  pushToKillRing,
  recordYank,
  resetKillAccumulation,
  resetYankState,
  updateYankLength,
  yankPop,
} from "../utils/Cursor.js";

type MaybeCursor = void | Cursor;
type InputHandler = (input: string) => MaybeCursor;
type InputMapper = (input: string) => MaybeCursor;
const NOOP_HANDLER: InputHandler = () => {};

/** Returns true when input is a raw control character (0x01–0x1A) that
 *  Ink failed to flag with `key.ctrl`.  Prevents the character from being
 *  inserted as plain text on terminals/OS combos where the ctrl flag is
 *  unreliable (e.g. Windows). */
function isRawControlChar(input: string): boolean {
  const code = input.charCodeAt(0);
  return code >= 1 && code <= 26;
}

function mapInput(inputMap: Array<[string, InputHandler]>): InputMapper {
  const map = new Map(inputMap);
  return (input: string): MaybeCursor => {
    return (map.get(input) ?? NOOP_HANDLER)(input);
  };
}

export interface UseTextInputProps {
  value: string;
  onChange: (value: string) => void;
  onSubmit?: (value: string) => void;
  onEscape?: () => void;
  onTab?: () => string | undefined;
  multiline?: boolean;
  enableCtrlShortcuts?: boolean;
  mask?: string;
  cursorChar: string;
  invert: (text: string) => string;
  columns: number;
  maxVisibleLines?: number;
}

export interface TextInputState {
  onInput: (input: string, key: Key) => void;
  renderedValue: string;
  offset: number;
  setOffset: (offset: number) => void;
  cursorLine: number;
  cursorColumn: number;
}

export function useTextInput({
  value,
  onChange,
  onSubmit,
  onEscape,
  onTab,
  multiline = true,
  enableCtrlShortcuts = true,
  mask = "",
  cursorChar,
  invert,
  columns,
  maxVisibleLines,
}: UseTextInputProps): TextInputState {
  const [offset, setOffset] = useState(() => value.length);

  useEffect(() => {
    setOffset((current) => Math.max(0, Math.min(value.length, current)));
  }, [value]);

  const cursor = useMemo(() => {
    return Cursor.fromText(value, columns, offset);
  }, [columns, offset, value]);

  function killToLineEnd(): Cursor {
    const { cursor: newCursor, killed } = cursor.deleteToLineEnd();
    pushToKillRing(killed, "append");
    return newCursor;
  }

  function killToLineStart(): Cursor {
    const { cursor: newCursor, killed } = cursor.deleteToLineStart();
    pushToKillRing(killed, "prepend");
    return newCursor;
  }

  function killWordBefore(): Cursor {
    const { cursor: newCursor, killed } = cursor.deleteWordBefore();
    pushToKillRing(killed, "prepend");
    return newCursor;
  }

  function yank(): Cursor {
    const text = getLastKill();
    if (text.length === 0) {
      return cursor;
    }
    const startOffset = cursor.offset;
    const newCursor = cursor.insert(text);
    recordYank(startOffset, text.length);
    return newCursor;
  }

  function handleYankPop(): Cursor {
    const popResult = yankPop();
    if (!popResult) {
      return cursor;
    }
    const { text, start, length } = popResult;
    const before = cursor.text.slice(0, start);
    const after = cursor.text.slice(start + length);
    const newText = before + text + after;
    const newOffset = start + text.length;
    updateYankLength(text.length);
    return Cursor.fromText(newText, columns, newOffset);
  }

  function upOrHistoryUp(): Cursor {
    const byWrappedLine = cursor.up();
    if (!byWrappedLine.equals(cursor)) {
      return byWrappedLine;
    }
    if (multiline) {
      const byLogicalLine = cursor.upLogicalLine();
      if (!byLogicalLine.equals(cursor)) {
        return byLogicalLine;
      }
    }
    return cursor;
  }

  function downOrHistoryDown(): Cursor {
    const byWrappedLine = cursor.down();
    if (!byWrappedLine.equals(cursor)) {
      return byWrappedLine;
    }
    if (multiline) {
      const byLogicalLine = cursor.downLogicalLine();
      if (!byLogicalLine.equals(cursor)) {
        return byLogicalLine;
      }
    }
    return cursor;
  }

  function handleEnter(key: Key): MaybeCursor {
    if (multiline && (key.meta || key.shift || key.ctrl)) {
      return cursor.insert("\n");
    }
    onSubmit?.(value);
    return cursor;
  }

  const handleCtrl = mapInput([
    ["a", () => cursor.startOfLine()],
    ["b", () => cursor.left()],
    ["d", () => cursor.del()],
    ["e", () => cursor.endOfLine()],
    ["f", () => cursor.right()],
    ["h", () => cursor.deleteTokenBefore() ?? cursor.backspace()],
    ["k", killToLineEnd],
    ["n", downOrHistoryDown],
    ["p", upOrHistoryUp],
    ["u", killToLineStart],
    ["w", killWordBefore],
    ["y", yank],
  ]);

  const handleMeta = mapInput([
    ["b", () => cursor.prevWord()],
    ["f", () => cursor.nextWord()],
    ["d", () => cursor.deleteWordAfter()],
    ["y", handleYankPop],
  ]);

  function mapKey(key: Key, input: string): InputMapper {
    switch (true) {
      case key.escape:
        return () => {
          onEscape?.();
          return cursor;
        };
      case key.leftArrow && (key.ctrl || key.meta):
        return () => cursor.prevWord();
      case key.rightArrow && (key.ctrl || key.meta):
        return () => cursor.nextWord();
      case key.backspace || key.delete:
        // ink maps \x7f (Backspace on most terminals) to key.delete instead of
        // key.backspace. Treat both as backspace so the physical Backspace key
        // deletes the character *before* the cursor on all platforms.
        return key.meta || key.ctrl
          ? killWordBefore
          : () => cursor.deleteTokenBefore() ?? cursor.backspace();
      case key.ctrl:
        return enableCtrlShortcuts ? handleCtrl : () => cursor;
      case key.home:
        return () => cursor.startOfLine();
      case key.end:
        return () => cursor.endOfLine();
      case key.pageDown:
        return () => cursor.endOfLine();
      case key.pageUp:
        return () => cursor.startOfLine();
      case key.return:
        return () => handleEnter(key);
      case key.meta:
        return handleMeta;
      case key.tab:
        return () => {
          const next = onTab?.();
          if (!next || next === value) {
            return cursor;
          }
          return Cursor.fromText(next, columns, next.length);
        };
      case key.upArrow && !key.shift:
        return upOrHistoryUp;
      case key.downArrow && !key.shift:
        return downOrHistoryDown;
      case key.leftArrow:
        return () => cursor.left();
      case key.rightArrow:
        return () => cursor.right();
      case isRawControlChar(input):
        // Raw control character (e.g. ctrl+O on Windows where key.ctrl is unset).
        // Don't insert as text — let the App-level handler process it.
        return () => cursor;
      default:
        return (input: string): MaybeCursor => {
          const text = stripAnsi(input).replace(/\r/g, "\n");
          if (text.length === 0) {
            return cursor;
          }
          return cursor.insert(text);
        };
    }
  }

  function isKillKey(key: Key, input: string): boolean {
    if (key.ctrl && (input === "k" || input === "u" || input === "w")) {
      return true;
    }
    if (key.meta && (key.backspace || key.delete)) {
      return true;
    }
    return false;
  }

  function isYankKey(key: Key, input: string): boolean {
    return (key.ctrl || key.meta) && input === "y";
  }

  function onInput(input: string, key: Key): void {
    if (!isKillKey(key, input)) {
      resetKillAccumulation();
    }
    if (!isYankKey(key, input)) {
      resetYankState();
    }

    const nextCursor = mapKey(key, input)(input);
    if (!nextCursor) {
      return;
    }

    if (cursor.text !== nextCursor.text) {
      onChange(nextCursor.text);
    }
    if (offset !== nextCursor.offset) {
      setOffset(nextCursor.offset);
    }
  }

  const position = cursor.getPosition();

  return {
    onInput,
    renderedValue: cursor.render(cursorChar, mask, invert, undefined, maxVisibleLines),
    offset,
    setOffset,
    cursorLine: position.line - cursor.getViewportStartLine(maxVisibleLines),
    cursorColumn: position.column,
  };
}
