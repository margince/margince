// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useEffect, useReducer } from "react";
import { selectionIn } from "./richtext-html";

const MARKS = [
  "bold",
  "italic",
  "bulletList",
  "numberList",
  "heading",
] as const;
type Marks = Readonly<Record<(typeof MARKS)[number], boolean>>;
const NO_MARKS: Marks = {
  bold: false,
  italic: false,
  bulletList: false,
  numberList: false,
  heading: false,
};

export const MARK_COMMANDS = {
  bold: "bold",
  italic: "italic",
  bulletList: "insertUnorderedList",
  numberList: "insertOrderedList",
} as const;

// Some engines throw for a command they cannot answer at this caret.
function engineSays(read: () => boolean): boolean {
  try {
    return read();
  } catch {
    return false;
  }
}

const commandOn = (command: string) =>
  engineSays(() => document.queryCommandState(command));

export const headingAtCaret = () =>
  engineSays(() => document.queryCommandValue("formatBlock") === "h2");

function marksAtCaret(): Marks {
  // A host without editing commands shows no mark on.
  if (typeof document.queryCommandState !== "function") {
    return NO_MARKS;
  }
  return {
    bold: commandOn(MARK_COMMANDS.bold),
    italic: commandOn(MARK_COMMANDS.italic),
    bulletList: commandOn(MARK_COMMANDS.bulletList),
    numberList: commandOn(MARK_COMMANDS.numberList),
    heading: headingAtCaret(),
  };
}

// The keys whose release ends a mark chord, released in either order.
const CHORD_KEYS = new Set(["b", "i", "control", "meta"]);

// The same object back when nothing changed, so a caret move renders nothing.
function sameOrNext(shown: Marks, next: Marks): Marks {
  return MARKS.every((mark) => shown[mark] === next[mark]) ? shown : next;
}

/** The marks on at the caret in `editor`, none while `disabled`. `refresh`
 * follows a command, since one on a bare caret moves no selection. */
export function useMarksAtCaret(
  editor: RefObject<HTMLElement | null>,
  disabled: boolean,
): readonly [Marks, () => void] {
  const [marks, showMarks] = useReducer(sameOrNext, NO_MARKS);
  useEffect(() => {
    const follow = () => {
      const node = editor.current;
      // With no caret in the editor, no mark is on in it.
      showMarks(node && selectionIn(node) ? marksAtCaret() : NO_MARKS);
    };
    // Ctrl or Cmd with B or I on a bare caret changes the pending mark and moves
    // no selection. Read on keyup: on keydown the engine has not applied it yet.
    const chord = (event: KeyboardEvent) => {
      if (CHORD_KEYS.has(event.key.toLowerCase())) follow();
    };
    const node = editor.current;
    document.addEventListener("selectionchange", follow);
    node?.addEventListener("keyup", chord);
    return () => {
      document.removeEventListener("selectionchange", follow);
      node?.removeEventListener("keyup", chord);
    };
  }, [editor]);
  return [disabled ? NO_MARKS : marks, () => showMarks(marksAtCaret())];
}
