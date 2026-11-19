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

export const headingAtCaret = () =>
  document.queryCommandValue("formatBlock") === "h2";

// Some engines throw for a command they cannot answer at this caret.
function commandOn(command: string): boolean {
  try {
    return document.queryCommandState(command);
  } catch {
    return false;
  }
}

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
      if (node && selectionIn(node)) {
        showMarks(marksAtCaret());
      }
    };
    document.addEventListener("selectionchange", follow);
    return () => document.removeEventListener("selectionchange", follow);
  }, [editor]);
  return [disabled ? NO_MARKS : marks, () => showMarks(marksAtCaret())];
}
