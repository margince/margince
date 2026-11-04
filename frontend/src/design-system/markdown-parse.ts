// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * The reader behind `<Markdown>`: hostile source text in, a closed tree of
 * blocks out.
 *
 * It is a separate module from the renderer for one reason: the security
 * property this primitive exists to hold is "the renderer can only build
 * elements it names itself", and that is only true while the thing between the
 * source and React is a DATA structure with a closed set of shapes. A parser
 * that returned markup could grow a hole; one that returns `Inline | Block`
 * cannot, because there is no shape in the union that means "raw HTML".
 *
 * The corollary, and the reason nothing here strips or sanitises: text reaches
 * the renderer as text and React escapes it. A `<script>` in the corpus is a
 * paragraph whose text happens to be `<script>`, and it draws on the page as
 * those eight characters. Nothing is dropped, nothing executes.
 *
 * ## What it supports, and why exactly that
 *
 * The shipped handbook (`backend/internal/modules/knowledge/handbook/*.md`) is
 * the corpus this opens: ATX headings, paragraphs, bold, italic, inline code,
 * fenced code, bullet and ordered lists, blockquotes, GFM tables, links and
 * horizontal rules. Nothing else is recognised — and the failure mode of not
 * recognising something is that its source line renders as the plain text it
 * is, which is the honest reading of a document form we do not claim to know.
 */

import { markLine, markQuote } from "./markdown-highlight";
import { parseInline } from "./markdown-inline";

/** A leaf or a wrapper inside one line of prose. */
export type Inline =
  | { kind: "text"; text: string }
  | { kind: "code"; text: string }
  | { kind: "strong"; children: Inline[] }
  | { kind: "em"; children: Inline[] }
  | { kind: "link"; href: string; children: Inline[] }
  | { kind: "mark"; children: Inline[] }
  // A line break kept as one: only a note's body has them (hardBreakBlocks).
  | { kind: "break" };

/**
 * One stretch of inline content — a paragraph's, a heading's, a list item's, a
 * table cell's. It is an OBJECT rather than an array so the highlight pass can
 * replace the nodes of the one run it matched in, after the tree that points at
 * that run is already built.
 */
export type Run = { nodes: Inline[] };

/**
 * Where a block came from, and whether the highlight fell back to it.
 *
 * `line` and `endLine` are 1-based source lines, inclusive, so that a caller
 * holding a line number from the server — which counts a document the same way
 * — can be answered without a second coordinate system.
 */
type Located = { line: number; endLine: number; marked?: boolean };

export type Block = Located &
  (
    | { kind: "heading"; level: number; run: Run }
    | { kind: "paragraph"; run: Run }
    | { kind: "code"; run: Run }
    | { kind: "rule" }
    | { kind: "list"; ordered: boolean; start: number; items: Run[] }
    | { kind: "quote"; blocks: Block[] }
    | { kind: "table"; header: Run[]; rows: Run[][] }
  );

/** Which of the three highlight outcomes the document settled on. */
export type MarkdownHighlightOutcome = "quote" | "line" | "none";

export type MarkdownHighlight = {
  /** The passage to mark, as the citation quotes it. */
  quote: string;
  /** The source line the quote was found on, used when the quote is not. */
  line?: number;
};

export type MarkdownDocument = {
  blocks: Block[];
  outcome: MarkdownHighlightOutcome;
};

const FENCE = /^ {0,3}(```|~~~)/;
const HEADING = /^ {0,3}(#{1,6})[ \t]+(.*)$/;
const RULE = /^ {0,3}([-*_])(?:[ \t]*\1){2,}[ \t]*$/;
const QUOTE = /^ {0,3}>[ \t]?/;
const BULLET = /^ {0,3}[-*+][ \t]+(.*)$/;
const COMMENT_LINE = /^ {0,3}<!--.*-->[ \t]*$/;
const ORDERED = /^ {0,3}\d{1,9}[.)][ \t]+(.*)$/;

/** Read a document, and mark the passage `highlight` asks for if it is there. */
export function readMarkdown(
  source: string,
  highlight?: MarkdownHighlight,
): MarkdownDocument {
  const blocks = parseBlocks(source.replace(/\r\n?/g, "\n").split("\n"), 1);
  if (highlight === undefined) return { blocks, outcome: "none" };
  if (markQuote(blocks, highlight.quote)) return { blocks, outcome: "quote" };
  if (markLine(blocks, highlight.line)) return { blocks, outcome: "line" };
  return { blocks, outcome: "none" };
}

// ---------------------------------------------------------------------------
// Blocks
// ---------------------------------------------------------------------------

type Taken = { block: Block; next: number };

function lineAt(lines: string[], i: number): string {
  return lines[i] ?? "";
}

/** `first` is the 1-based source line that `lines[0]` is, so a blockquote's
 *  inner blocks keep the line numbers the document counts, not their own. */
// Each quote level is one level of recursion. Past this depth the rest reads as
// text, so no stored body can exhaust the call stack.
const MAX_QUOTE_DEPTH = 32;

function parseBlocks(lines: string[], first: number, depth = 0): Block[] {
  const blocks: Block[] = [];
  let i = 0;
  while (i < lines.length) {
    // A whole-line comment is markup for tools, as the handbook's prose
    // markers are; it reads as a blank line, never as text.
    const line = lineAt(lines, i);
    if (line.trim() === "" || COMMENT_LINE.test(line)) {
      i++;
      continue;
    }
    const taken = takeBlock(lines, i, first, depth);
    blocks.push(taken.block);
    i = taken.next;
  }
  return blocks;
}

function takeBlock(
  lines: string[],
  i: number,
  first: number,
  depth: number,
): Taken {
  const line = lineAt(lines, i);
  if (FENCE.test(line)) return takeFence(lines, i, first);
  if (HEADING.test(line)) return takeHeading(lines, i, first);
  if (RULE.test(line)) {
    return {
      block: { kind: "rule", line: first + i, endLine: first + i },
      next: i + 1,
    };
  }
  if (QUOTE.test(line)) return takeQuote(lines, i, first, depth);
  if (BULLET.test(line) || ORDERED.test(line)) return takeList(lines, i, first);
  if (startsTable(lines, i)) return takeTable(lines, i, first);
  return takeParagraph(lines, i, first);
}

/** Whether line `i` opens a block of its own, and so ends the one above it. */
function startsBlock(lines: string[], i: number): boolean {
  const line = lineAt(lines, i);
  return (
    FENCE.test(line) ||
    HEADING.test(line) ||
    RULE.test(line) ||
    QUOTE.test(line) ||
    BULLET.test(line) ||
    ORDERED.test(line) ||
    startsTable(lines, i)
  );
}

function takeFence(lines: string[], i: number, first: number): Taken {
  const open = lineAt(lines, i).trim().slice(0, 3);
  let j = i + 1;
  while (j < lines.length && lineAt(lines, j).trim() !== open) j++;
  const body = lines.slice(i + 1, j).join("\n");
  // An unclosed fence ends at the document's end rather than swallowing the
  // rest as a paragraph: a truncated upload is a corpus document too.
  const endLine = first + Math.min(j, lines.length - 1);
  return {
    block: {
      kind: "code",
      run: { nodes: [{ kind: "text", text: body }] },
      line: first + i,
      endLine,
    },
    next: j + 1,
  };
}

function takeHeading(lines: string[], i: number, first: number): Taken {
  const match = HEADING.exec(lineAt(lines, i));
  const level = match?.[1]?.length ?? 1;
  const text = match?.[2]?.replace(/\s+#+\s*$/, "") ?? lineAt(lines, i);
  return {
    block: {
      kind: "heading",
      level,
      run: { nodes: parseInline(text) },
      line: first + i,
      endLine: first + i,
    },
    next: i + 1,
  };
}

function takeQuote(
  lines: string[],
  i: number,
  first: number,
  depth: number,
): Taken {
  let j = i;
  const inner: string[] = [];
  while (j < lines.length && QUOTE.test(lineAt(lines, j))) {
    inner.push(lineAt(lines, j).replace(QUOTE, ""));
    j++;
  }
  return {
    block: {
      kind: "quote",
      // The inner lines sit one-for-one over the outer ones, so the nested
      // blocks carry the document's own line numbers.
      blocks:
        depth < MAX_QUOTE_DEPTH
          ? parseBlocks(inner, first + i, depth + 1)
          : [
              {
                kind: "paragraph",
                run: { nodes: parseInline(inner.join("\n")) },
                line: first + i,
                endLine: first + j - 1,
              },
            ],
      line: first + i,
      endLine: first + j - 1,
    },
    next: j,
  };
}

function takeList(lines: string[], i: number, first: number): Taken {
  const ordered = ORDERED.test(lineAt(lines, i));
  const pattern = ordered ? ORDERED : BULLET;
  const items: string[][] = [];
  let j = i;
  while (j < lines.length && lineAt(lines, j).trim() !== "") {
    const item = pattern.exec(lineAt(lines, j))?.[1];
    if (item !== undefined) items.push([item]);
    else if (startsBlock(lines, j) || items.length === 0) break;
    // A wrapped line belongs to the item above it. Deeper indentation reads the
    // same way: this tier ships no nested list, so a nested one arrives as the
    // text it is rather than as a level nothing would draw.
    else items.at(-1)?.push(lineAt(lines, j).trim());
    j++;
  }
  return {
    block: {
      kind: "list",
      ordered,
      // The number the list was written from: "5. Fifth" is the fifth step.
      start: ordered ? Number.parseInt(lineAt(lines, i).trim(), 10) : 1,
      items: items.map((item) => ({ nodes: parseInline(item.join("\n")) })),
      line: first + i,
      endLine: first + j - 1,
    },
    next: j,
  };
}

/** A GFM table is a header row and the dashed rule under it, together. */
function startsTable(lines: string[], i: number): boolean {
  if (!lineAt(lines, i).includes("|")) return false;
  const under = lines[i + 1] ?? "";
  return (
    under.includes("|") &&
    under.includes("-") &&
    /^[\s:|-]+$/.test(under.trim())
  );
}

function takeTable(lines: string[], i: number, first: number): Taken {
  let j = i + 2;
  const rows: Run[][] = [];
  while (
    j < lines.length &&
    lineAt(lines, j).includes("|") &&
    lineAt(lines, j).trim() !== ""
  ) {
    rows.push(cellsOf(lineAt(lines, j)));
    j++;
  }
  return {
    block: {
      kind: "table",
      header: cellsOf(lineAt(lines, i)),
      rows,
      line: first + i,
      endLine: first + j - 1,
    },
    next: j,
  };
}

function cellsOf(line: string): Run[] {
  return line
    .trim()
    .replace(/^\|/, "")
    .replace(/\|$/, "")
    .split("|")
    .map((cell) => ({ nodes: parseInline(cell.trim()) }));
}

function takeParagraph(lines: string[], i: number, first: number): Taken {
  let j = i + 1;
  while (
    j < lines.length &&
    lineAt(lines, j).trim() !== "" &&
    !startsBlock(lines, j)
  ) {
    j++;
  }
  return {
    block: {
      kind: "paragraph",
      run: { nodes: parseInline(lines.slice(i, j).join("\n")) },
      line: first + i,
      endLine: first + j - 1,
    },
    next: j,
  };
}

// ---------------------------------------------------------------------------
// Inline
// ---------------------------------------------------------------------------
