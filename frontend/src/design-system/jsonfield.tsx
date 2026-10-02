// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ComponentPropsWithoutRef, useRef } from "react";
import { identifierNumber } from "../format/format";
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
          if (gutter.current)
            gutter.current.scrollTop = e.currentTarget.scrollTop;
        }}
        onKeyDown={(e) => {
          if (e.key !== "Tab" || e.shiftKey) return;
          e.preventDefault();
          const box = e.currentTarget;
          const { selectionStart, selectionEnd } = box;
          onChange(
            `${value.slice(0, selectionStart)}  ${value.slice(selectionEnd)}`,
          );
          requestAnimationFrame(() =>
            box.setSelectionRange(selectionStart + 2, selectionStart + 2),
          );
        }}
      />
    </div>
  );
}

/**
 * Where `text` stops parsing; null when it parses or is empty. The engines
 * report a position or a line, never both in one shape, so both are read.
 */
export function parseProblem(text: string): ParseProblem | null {
  if (text.trim() === "") return null;
  try {
    JSON.parse(text);
    return null;
  } catch (error) {
    const said = error instanceof Error ? error.message : "";
    const byLine = /line (\d+)/.exec(said);
    if (byLine) return { line: Number(byLine[1]) };
    const byPosition = /position (\d+)/.exec(said);
    return {
      line: byPosition ? lineAt(text, Number(byPosition[1])) : undefined,
    };
  }
}

/**
 * The line a dotted key path (`provider.sort.by`, `provider.only[1]`) is
 * written on, found by walking its keys in order through the text; undefined
 * when a key is not in it. An index names the key that holds the list.
 */
export function lineOfPath(text: string, path: string): number | undefined {
  let from = 0;
  let found = -1;
  for (const segment of path.split(".")) {
    const key = segment.replace(/\[\d+\]$/, "");
    if (key === "") continue;
    const at = text.indexOf(JSON.stringify(key), from);
    if (at < 0) return found < 0 ? undefined : lineAt(text, found);
    found = at;
    from = at + key.length;
  }
  return found < 0 ? undefined : lineAt(text, found);
}

function lineAt(text: string, offset: number): number {
  return text.slice(0, offset).split("\n").length;
}
