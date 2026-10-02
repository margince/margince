// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ComponentPropsWithoutRef, useRef } from "react";
import { identifierNumber } from "../format/format";
import { scanJson } from "./jsonscan";
import "./jsonfield.css";

/** One problem the field shows: where it is, and what to do about it. */
export type JsonProblem = Readonly<{
  line?: number;
  path?: string;
  message: string;
}>;

/** Where text stops parsing; the caller words it, the engine's own text never reaches a reader. */
export type ParseProblem = Readonly<{ line?: number }>;

/**
 * A JSON value an advanced reader edits by hand, with a line-number gutter that
 * marks the lines a problem points at.
 *
 * Deliberately not a code editor: no highlighting and no completion. The value
 * it holds is validated by the server, which names each problem by the key's
 * path; the field derives the LINE from that path (`lineOfPath`) so a problem
 * can be found in the text, and checks only that the text parses
 * (`parseProblem`). A `<textarea>` with `.code-block`, so the mono face stays
 * the code face and nothing else.
 *
 * Tab indents two spaces rather than leaving the field: a reader indenting a
 * nested object would otherwise lose their place on every keystroke. Shift+Tab
 * still moves focus on.
 */
export function JsonField({
  value,
  onChange,
  problemLines = [],
  invalid = false,
  ...rest
}: Readonly<
  Omit<ComponentPropsWithoutRef<"textarea">, "value" | "onChange"> & {
    value: string;
    onChange: (next: string) => void;
    /** 1-based lines the gutter marks. */
    problemLines?: readonly number[];
    invalid?: boolean;
  }
>) {
  const gutter = useRef<HTMLDivElement>(null);
  const lines = Math.max(value.split("\n").length, 1);
  const marked = new Set(problemLines);
  return (
    <div className={`json-field${invalid ? " json-field-invalid" : ""}`}>
      <div className="json-field-gutter" ref={gutter} aria-hidden="true">
        {Array.from({ length: lines }, (_, i) => (
          <span
            // biome-ignore lint/suspicious/noArrayIndexKey: a gutter number IS its line's position
            key={i}
            className={marked.has(i + 1) ? "json-field-mark" : undefined}
          >
            {identifierNumber(i + 1)}
          </span>
        ))}
      </div>
      <textarea
        {...rest}
        className="code-block json-field-text"
        spellCheck={false}
        aria-invalid={invalid || undefined}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onScroll={(e) => {
          rest.onScroll?.(e);
          if (gutter.current)
            gutter.current.scrollTop = e.currentTarget.scrollTop;
        }}
        onKeyDown={(e) => {
          rest.onKeyDown?.(e);
          if (e.defaultPrevented || e.key !== "Tab" || e.shiftKey) return;
          e.preventDefault();
          indent(e.currentTarget, value, onChange);
        }}
      />
    </div>
  );
}

/**
 * Two spaces at the caret; a selection that spans lines is indented line by
 * line and stays selected, rather than replaced.
 */
function indent(
  box: HTMLTextAreaElement,
  value: string,
  onChange: (next: string) => void,
) {
  const { selectionStart: start, selectionEnd: end } = box;
  const selected = value.slice(start, end);
  const block = selected.includes("\n");
  const inserted = block ? selected.replaceAll(/^/gm, "  ") : "  ";
  onChange(`${value.slice(0, start)}${inserted}${value.slice(end)}`);
  requestAnimationFrame(() =>
    block
      ? box.setSelectionRange(start, start + inserted.length)
      : box.setSelectionRange(start + 2, start + 2),
  );
}

/** Where `text` stops parsing; null when it parses or is empty. */
export function parseProblem(text: string): ParseProblem | null {
  if (text.trim() === "") return null;
  const { error } = scanJson(text);
  return error === undefined ? null : { line: lineAt(text, error) };
}

/**
 * The line a dotted key path (`provider.sort.by`, `provider.only[1]`) is
 * written on. A path the text does not hold falls back to its nearest written
 * parent, so a missing key still marks the object it belongs in.
 */
export function lineOfPath(text: string, path: string): number | undefined {
  const { keys } = scanJson(text);
  for (let at = path; at !== ""; at = parentOf(at)) {
    const offset = keys.get(at);
    if (offset !== undefined) return lineAt(text, offset);
  }
  return undefined;
}

function parentOf(path: string): string {
  const cut = Math.max(path.lastIndexOf("."), path.lastIndexOf("["));
  return cut < 0 ? "" : path.slice(0, cut);
}

function lineAt(text: string, offset: number): number {
  return text.slice(0, offset).split("\n").length;
}
