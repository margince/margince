// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Block, Inline, Run } from "./markdown-parse";

/**
 * The highlight pass over a parsed document: find the cited passage, and wrap
 * it.
 *
 * Its own file because it is a different question from reading markdown. The
 * parser answers "what does this document say"; this answers "where in it is
 * the sentence a citation quoted", and the two are joined only by the tree
 * between them — which is why this module takes a parsed document rather than
 * source text and never looks at the syntax again.
 */

/**
 * Fold every run of whitespace into one space, and trim.
 *
 * This is the frontend spelling of `claims.CollapseSpace` — the server locates
 * a claim under exactly this normalisation (`compose/corpusaskreply.go`,
 * `locateClaim`), so a viewer comparing under any other one would mark a
 * different passage than the one the citation was checked against.
 */
export function collapseSpace(text: string): string {
  return text
    .split(/\s+/)
    .filter((part) => part !== "")
    .join(" ");
}
/**
 * Wrap the first run whose text contains `quote` once whitespace is collapsed.
 *
 * A run rather than the whole document: a quote that crosses two paragraphs has
 * no single element to wrap, and reporting a miss there is what the `line`
 * fallback beside it is for. That miss is ordinary rather than exceptional —
 * roughly one citation in four comes back re-wrapped or collapsed — which is
 * why this returns whether it hit instead of throwing.
 */
export function markQuote(blocks: Block[], quote: string): boolean {
  // Two spellings of the same quote, tried in order. The citation was cut from
  // the document's SOURCE, so it carries that source's markup — the handbook
  // quotes "**Full seat.** Can read…", asterisks and all. What is on screen is
  // the RENDERED text, where those asterisks are a font weight and not
  // characters, so the literal quote matches nothing on a page that plainly
  // contains it. Stripping the markers is what makes the common citation land;
  // trying the raw form first is what keeps a quote whose own text contains an
  // asterisk from being mangled into a miss.
  for (const needle of [
    collapseSpace(quote),
    collapseSpace(withoutMarkers(quote)),
  ]) {
    if (needle === "") continue;
    for (const run of runsOf(blocks)) {
      const span = locateCollapsed(runText(run.nodes), needle);
      if (span === null) continue;
      run.nodes = markNodes(run.nodes, span.start, span.end, { at: 0 });
      return true;
    }
  }
  return false;
}

/**
 * The quote as the page renders it: inline emphasis and code markers dropped.
 *
 * Only the markers a run can carry, and only where they wrap something — a
 * lone asterisk in prose is prose. Block syntax is not touched, because a run
 * never spans a heading's own hashes or a list's bullet.
 */
function withoutMarkers(quote: string): string {
  return quote
    .replace(/\*\*(.+?)\*\*/g, "$1")
    .replace(/(^|[\s(])[*_](\S(?:.*?\S)?)[*_](?=[\s).,;:!?]|$)/g, "$1$2")
    .replace(/`([^`]+)`/g, "$1");
}

/** Mark the whole block that contains `line`, the coarse answer to a miss. */
export function markLine(blocks: Block[], line: number | undefined): boolean {
  if (line === undefined) return false;
  const block = blocks.find(
    (candidate) => candidate.line <= line && line <= candidate.endLine,
  );
  if (block === undefined) return false;
  block.marked = true;
  return true;
}

/** Every run in the document, in reading order. */
function runsOf(blocks: Block[]): Run[] {
  return blocks.flatMap((block) => {
    if (block.kind === "list") return block.items;
    if (block.kind === "quote") return runsOf(block.blocks);
    if (block.kind === "table") return [...block.header, ...block.rows.flat()];
    if (block.kind === "rule") return [];
    return [block.run];
  });
}

/** The plain text of a run: what the collapsed match is taken over, and what a
 *  caller naming a region out of the document's own words reads. */
export function runText(nodes: readonly Inline[]): string {
  return nodes
    .map((node) =>
      node.kind === "text" || node.kind === "code"
        ? node.text
        : runText(node.children),
    )
    .join("");
}

/**
 * Find `needle` — already collapsed — in `raw`, and report the span in RAW
 * offsets.
 *
 * The offsets have to come back in the raw string's own coordinates because
 * that is what the marking pass slices: an offset into the collapsed form would
 * land short by every space the collapse removed, and the highlight would run
 * off the end of the quote by that much.
 */
function locateCollapsed(
  raw: string,
  needle: string,
): { start: number; end: number } | null {
  const collapsed: string[] = [];
  const starts: number[] = [];
  const ends: number[] = [];
  let i = 0;
  while (i < raw.length) {
    const from = i;
    if (/\s/.test(raw[i])) {
      while (i < raw.length && /\s/.test(raw[i])) i++;
      // A leading or trailing run collapses to nothing, which is what trimming
      // means; only a run BETWEEN two words becomes the single space.
      if (collapsed.length === 0 || i === raw.length) continue;
      collapsed.push(" ");
    } else {
      collapsed.push(raw[i]);
      i++;
    }
    starts.push(from);
    ends.push(i);
  }
  const at = collapsed.join("").indexOf(needle);
  if (at < 0) return null;
  return { start: starts[at], end: ends[at + needle.length - 1] };
}

/**
 * Re-cut `nodes` so the characters in `[start, end)` sit inside `mark` nodes.
 *
 * A quote that begins in plain text and ends inside a bold phrase produces two
 * marks rather than one, because a single element cannot span the boundary
 * without dropping the emphasis the document actually carries. `cursor` walks
 * the same text `runText` concatenated, so the offsets mean the same thing on
 * both sides.
 */
function markNodes(
  nodes: readonly Inline[],
  start: number,
  end: number,
  cursor: { at: number },
): Inline[] {
  return nodes.flatMap((node) => markNode(node, start, end, cursor));
}

function markNode(
  node: Inline,
  start: number,
  end: number,
  cursor: { at: number },
): Inline[] {
  if (node.kind !== "text" && node.kind !== "code") {
    return [
      { ...node, children: markNodes(node.children, start, end, cursor) },
    ];
  }
  const from = cursor.at;
  const to = from + node.text.length;
  cursor.at = to;
  const lo = Math.max(start, from);
  const hi = Math.min(end, to);
  if (hi <= lo) return [node];
  const cut = (a: number, b: number): Inline => ({
    ...node,
    text: node.text.slice(a - from, b - from),
  });
  return [
    ...(lo > from ? [cut(from, lo)] : []),
    { kind: "mark", children: [cut(lo, hi)] },
    ...(hi < to ? [cut(hi, to)] : []),
  ];
}
