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

/** A leaf or a wrapper inside one line of prose. */
export type Inline =
  | { kind: "text"; text: string }
  | { kind: "code"; text: string }
  | { kind: "strong"; children: Inline[] }
  | { kind: "em"; children: Inline[] }
  | { kind: "link"; href: string; external: boolean; children: Inline[] }
  | { kind: "mark"; children: Inline[] };

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
    | { kind: "list"; ordered: boolean; items: Run[] }
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
const HEADING = /^ {0,3}(#{1,6})\s+(.*)$/;
const RULE = /^ {0,3}([-*_])(?:[ \t]*\1){2,}[ \t]*$/;
const QUOTE = /^ {0,3}>[ \t]?/;
const BULLET = /^ {0,3}[-*+][ \t]+(.*)$/;
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

/** `first` is the 1-based source line that `lines[0]` is, so a blockquote's
 *  inner blocks keep the line numbers the document counts, not their own. */
function parseBlocks(lines: string[], first: number): Block[] {
  const blocks: Block[] = [];
  let i = 0;
  while (i < lines.length) {
    if (lines[i].trim() === "") {
      i++;
      continue;
    }
    const taken = takeBlock(lines, i, first);
    blocks.push(taken.block);
    i = taken.next;
  }
  return blocks;
}

function takeBlock(lines: string[], i: number, first: number): Taken {
  const line = lines[i];
  if (FENCE.test(line)) return takeFence(lines, i, first);
  if (HEADING.test(line)) return takeHeading(lines, i, first);
  if (RULE.test(line)) {
    return {
      block: { kind: "rule", line: first + i, endLine: first + i },
      next: i + 1,
    };
  }
  if (QUOTE.test(line)) return takeQuote(lines, i, first);
  if (BULLET.test(line) || ORDERED.test(line)) return takeList(lines, i, first);
  if (startsTable(lines, i)) return takeTable(lines, i, first);
  return takeParagraph(lines, i, first);
}

/** Whether line `i` opens a block of its own, and so ends the one above it. */
function startsBlock(lines: string[], i: number): boolean {
  const line = lines[i];
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
  const open = lines[i].trim().slice(0, 3);
  let j = i + 1;
  while (j < lines.length && lines[j].trim() !== open) j++;
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
  const match = HEADING.exec(lines[i]);
  const level = match === null ? 1 : match[1].length;
  const text = match === null ? lines[i] : match[2].replace(/\s+#+\s*$/, "");
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

function takeQuote(lines: string[], i: number, first: number): Taken {
  let j = i;
  const inner: string[] = [];
  while (j < lines.length && QUOTE.test(lines[j])) {
    inner.push(lines[j].replace(QUOTE, ""));
    j++;
  }
  return {
    block: {
      kind: "quote",
      // The inner lines sit one-for-one over the outer ones, so the nested
      // blocks carry the document's own line numbers.
      blocks: parseBlocks(inner, first + i),
      line: first + i,
      endLine: first + j - 1,
    },
    next: j,
  };
}

function takeList(lines: string[], i: number, first: number): Taken {
  const ordered = ORDERED.test(lines[i]);
  const pattern = ordered ? ORDERED : BULLET;
  const items: string[][] = [];
  let j = i;
  while (j < lines.length && lines[j].trim() !== "") {
    const match = pattern.exec(lines[j]);
    if (match !== null) items.push([match[1]]);
    else if (startsBlock(lines, j) || items.length === 0) break;
    // A wrapped line belongs to the item above it. Deeper indentation reads the
    // same way: this tier ships no nested list, so a nested one arrives as the
    // text it is rather than as a level nothing would draw.
    else items[items.length - 1].push(lines[j].trim());
    j++;
  }
  return {
    block: {
      kind: "list",
      ordered,
      items: items.map((item) => ({ nodes: parseInline(item.join("\n")) })),
      line: first + i,
      endLine: first + j - 1,
    },
    next: j,
  };
}

/** A GFM table is a header row and the dashed rule under it, together. */
function startsTable(lines: string[], i: number): boolean {
  if (!lines[i].includes("|")) return false;
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
  while (j < lines.length && lines[j].includes("|") && lines[j].trim() !== "") {
    rows.push(cellsOf(lines[j]));
    j++;
  }
  return {
    block: {
      kind: "table",
      header: cellsOf(lines[i]),
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
  while (j < lines.length && lines[j].trim() !== "" && !startsBlock(lines, j)) {
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

const CODE_SPAN = /^(`+)([\s\S]+?)\1/;
// A destination may carry one level of balanced parentheses, as CommonMark
// allows: `alert(1)` is a plausible payload and a reader of the refusal test
// should see the real shape of one.
const LINK = /^\[([^\][]*)\]\([ \t]*((?:[^()\s]|\([^()\s]*\))*)[ \t]*\)/;
const STRONG = /^(\*\*|__)([\s\S]+?)\1/;
const EM = /^([*_])([^\s*_][\s\S]*?)\1/;
const ESCAPABLE = /^[\\`*_{}[\]()#+\-.!|>~]$/;

function parseInline(text: string): Inline[] {
  const nodes: Inline[] = [];
  let plain = "";
  const flush = (): void => {
    if (plain !== "") nodes.push({ kind: "text", text: plain });
    plain = "";
  };
  let i = 0;
  while (i < text.length) {
    const rest = text.slice(i);
    if (rest[0] === "\\" && ESCAPABLE.test(rest[1] ?? "")) {
      plain += rest[1];
      i += 2;
      continue;
    }
    const taken = takeInline(rest);
    if (taken === null) {
      plain += text[i];
      i++;
      continue;
    }
    flush();
    nodes.push(taken.node);
    i += taken.length;
  }
  flush();
  return nodes;
}

function takeInline(rest: string): { node: Inline; length: number } | null {
  return (
    takeCode(rest) ??
    takeLink(rest) ??
    takeWrapped(rest, STRONG, "strong") ??
    takeWrapped(rest, EM, "em")
  );
}

function takeCode(rest: string): { node: Inline; length: number } | null {
  const match = CODE_SPAN.exec(rest);
  if (match === null) return null;
  // Code is a leaf: what a span holds is the literal characters between the
  // backticks, emphasis and brackets included, which is the whole point of it.
  return {
    node: { kind: "code", text: match[2].trim() },
    length: match[0].length,
  };
}

function takeWrapped(
  rest: string,
  pattern: RegExp,
  kind: "strong" | "em",
): { node: Inline; length: number } | null {
  const match = pattern.exec(rest);
  if (match === null) return null;
  return {
    node: { kind, children: parseInline(match[2]) },
    length: match[0].length,
  };
}

function takeLink(rest: string): { node: Inline; length: number } | null {
  const match = LINK.exec(rest);
  if (match === null) return null;
  const href = safeHref(match[2]);
  const children = parseInline(match[1]);
  // A refused href leaves the LABEL standing, and nothing else. The target of a
  // `javascript:` link is not information a reader wants beside the words it
  // was hidden behind, and printing it would put the payload back on the page
  // in the one form — visible, copyable text — that still invites a paste.
  if (href === null) {
    return { node: { kind: "em", children }, length: match[0].length };
  }
  return {
    node: { kind: "link", href, external: /^https?:/i.test(href), children },
    length: match[0].length,
  };
}

const HAS_SCHEME = /^[a-z][a-z0-9+.-]*:/i;
const SAFE_SCHEME = /^(https?|mailto):/i;

/**
 * The href allowlist. `http`, `https` and `mailto` are navigable; a
 * schemeless reference — the handbook's own `capture.md#filing` — is too,
 * because it carries no scheme to be dangerous with. Everything else is
 * refused: `javascript:`, `data:`, `vbscript:`, and any scheme invented after
 * this was written, because the test is what the list ADMITS rather than what
 * it has heard of.
 *
 * Whitespace and control characters come out BEFORE the scheme is read. A
 * browser ignores them when it resolves a URL, so `java\nscript:alert(1)` is a
 * javascript URL to the navigator and would be a schemeless one to a check
 * that read it as written.
 */
function safeHref(raw: string): string | null {
  // The control characters ARE the subject: a browser strips them before it
  // resolves a URL, so a check that left them in would read a different string
  // than the navigator does.
  // biome-ignore lint/suspicious/noControlCharactersInRegex: see just above
  const href = raw.replace(/[\u0000-\u0020\u007F]/g, "");
  if (href === "") return null;
  if (!HAS_SCHEME.test(href)) return href.startsWith("//") ? null : href;
  return SAFE_SCHEME.test(href) ? href : null;
}
