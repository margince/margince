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

function marksAtCaret(): Marks {
  // happy-dom and jsdom carry no editing commands; there no mark is ever on.
  if (typeof document.queryCommandState !== "function") {
    return NO_MARKS;
  }
  return {
    bold: document.queryCommandState(MARK_COMMANDS.bold),
    italic: document.queryCommandState(MARK_COMMANDS.italic),
    bulletList: document.queryCommandState(MARK_COMMANDS.bulletList),
    numberList: document.queryCommandState(MARK_COMMANDS.numberList),
    heading: headingAtCaret(),
  };
}

// The same object back when nothing changed, so a caret move renders nothing.
function sameOrNext(shown: Marks, next: Marks): Marks {
  return MARKS.every((mark) => shown[mark] === next[mark]) ? shown : next;
}

/** The marks on at the caret in `editor`, read while the selection is in it.
 * `refresh` follows a command, which on a bare caret moves no selection. */
export function useMarksAtCaret(
  editor: RefObject<HTMLElement | null>,
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
  return [marks, () => showMarks(marksAtCaret())];
}
