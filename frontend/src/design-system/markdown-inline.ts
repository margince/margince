// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Inline } from "./markdown-parse";

// The inline half of `<Markdown>`: a span of source text in, a closed list of
// inline nodes out. `markdown-parse.ts` calls it for each block's text.

// The opening run only. A backreference to the close, /^(`+)(.+?)\1/, retries
// every prefix length at every offset when no closing run exists, which is cubic.
// A line of 8,000 backticks took five seconds of the main thread inside render,
// and a tenant member can put a file in a corpus. takeCode scans for the close
// instead, which is linear.
const CODE_OPEN = /^`+/;
// A destination may carry one level of balanced parentheses, as CommonMark
// allows. `alert(1)` is a plausible payload, and the refusal test should see the
// real shape of one.
const LINK = /^\[([^\][]*)\]\([ \t]*((?:[^()\s]|\([^()\s]*\))*)[ \t]*\)/;
const STRONG = /^(\*\*)([\s\S]+?)\1/;
const EM = /^(\*)([^\s*_][\s\S]*?)\1/;
// A letter or digit in any script. An underscore inside a word is literal, never
// emphasis (CommonMark's flanking rule for `_`). So customer_id, a_b_c,
// utm_source and a file or URL path keep theirs.
const WORD_CHAR = /[\p{L}\p{N}]/u;
const ESCAPABLE = /^[\\`*_{}[\]()#+\-.!|>~]$/;

export function parseInline(text: string): Inline[] {
  const nodes: Inline[] = [];
  // Where an underscore scan already found no closer, per fence (see
  // takeUnderscored). Absolute positions in `text`.
  const scan: UnderscoreScan = { exhausted: new Map() };
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
    scan.at = i;
    const taken = takeInline(rest, i > 0 ? (text[i - 1] ?? "") : "", scan);
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

function takeInline(
  rest: string,
  before: string,
  scan: UnderscoreScan,
): { node: Inline; length: number } | null {
  return (
    takeCode(rest) ??
    takeLink(rest) ??
    takeWrapped(rest, STRONG, "strong") ??
    takeUnderscored(rest, before, "__", "strong", scan) ??
    takeWrapped(rest, EM, "em") ??
    takeUnderscored(rest, before, "_", "em", scan)
  );
}

/**
 * What one parseInline call has learned about underscore closers.
 *
 * Whether a position can close does not depend on the opener: it asks only
 * about the characters around the closer. So once a scan from one opener finds
 * no closer, no later opener can find one either, and its scan is skipped.
 * Without this, " _a _a _a…" rescans the rest of the line at every underscore.
 * That is quadratic work on the main thread.
 */
type UnderscoreScan = {
  at?: number;
  exhausted: Map<string, number>;
};

// takeUnderscored reads `_em_` or `__strong__` only where the underscores stand
// outside a word. The opener has no letter or digit before it and no space after
// it. The closer has no space before it and no letter or digit after it. An
// underscore inside a word is text.
function takeUnderscored(
  rest: string,
  before: string,
  fence: "_" | "__",
  kind: "strong" | "em",
  scan: UnderscoreScan,
): { node: Inline; length: number } | null {
  if (!opensUnderscore(rest, before, fence)) return null;
  const from = scan.at ?? 0;
  const exhausted = scan.exhausted.get(fence);
  if (exhausted !== undefined && from >= exhausted) return null;
  for (let at = fence.length + 1; at < rest.length; at++) {
    if (!closesUnderscore(rest, at, fence)) continue;
    return {
      node: { kind, children: parseInline(rest.slice(fence.length, at)) },
      length: at + fence.length,
    };
  }
  scan.exhausted.set(fence, from);
  return null;
}

/** Whether `rest` opens with `fence` outside a word, before a non-space. */
function opensUnderscore(rest: string, before: string, fence: string): boolean {
  if (!rest.startsWith(fence) || WORD_CHAR.test(before)) return false;
  const first = rest[fence.length] ?? "";
  return first !== "" && !/\s/.test(first) && first !== "_";
}

/** Whether `fence` at `at` closes: no space before it, no letter, digit or
 *  further underscore after it. It does not depend on the opener. */
function closesUnderscore(rest: string, at: number, fence: string): boolean {
  if (!rest.startsWith(fence, at)) return false;
  const after = rest[at + fence.length] ?? "";
  return (
    !/\s/.test(rest[at - 1] ?? "") && !WORD_CHAR.test(after) && after !== "_"
  );
}

function takeCode(rest: string): { node: Inline; length: number } | null {
  const opener = CODE_OPEN.exec(rest);
  if (opener === null) return null;
  const fence = opener[0];
  // The closing run is the same length and no longer: scanned for, never
  // backtracked into. A run of the wrong length is content, so the search
  // continues past it rather than giving up on the span.
  for (let at = fence.length; at < rest.length; ) {
    const found = rest.indexOf(fence, at);
    if (found < 0) return null;
    const end = found + fence.length;
    if (rest[end] === "`") {
      // A longer run than the opener: not this span's close, skip the whole run.
      at = end;
      while (rest[at] === "`") at += 1;
      continue;
    }
    // Code is a leaf. A span holds the literal characters between the backticks,
    // emphasis and brackets included.
    return {
      node: { kind: "code", text: rest.slice(fence.length, found).trim() },
      length: end,
    };
  }
  return null;
}

function takeWrapped(
  rest: string,
  pattern: RegExp,
  kind: "strong" | "em",
): { node: Inline; length: number } | null {
  const match = pattern.exec(rest);
  if (match === null) return null;
  return {
    node: { kind, children: parseInline(match[2] ?? "") },
    length: match[0].length,
  };
}

function takeLink(rest: string): { node: Inline; length: number } | null {
  const match = LINK.exec(rest);
  if (match === null) return null;
  const href = safeHref(match[2] ?? "");
  const children = parseInline(match[1] ?? "");
  // A refused href leaves the label standing, and nothing else. A reader does not
  // want the target of a `javascript:` link beside the words it was hidden behind.
  // Printing it would put the payload back on the page as visible, copyable text,
  // the one form that still invites a paste.
  if (href === null) {
    return { node: { kind: "em", children }, length: match[0].length };
  }
  return {
    node: { kind: "link", href, children },
    length: match[0].length,
  };
}

const HAS_SCHEME = /^[a-z][a-z0-9+.-]*:/i;
const SAFE_SCHEME = /^(https?|mailto):/i;

/**
 * The href allowlist: `http`, `https` and `mailto`, and nothing else. The test is
 * what the list admits, so `javascript:`, `data:` and any later scheme all fail.
 *
 * A schemeless reference is refused too, as a matter of destination, not safety.
 * This app has no resolver for a corpus-relative path such as `capture.md#filing`.
 * The label survives, and a resolver would let the reference become a link again.
 *
 * Whitespace and control characters come out before the scheme is read, because
 * a browser ignores them when it resolves a URL.
 */
function safeHref(raw: string): string | null {
  // `java\nscript:alert(1)` is a javascript URL to the navigator. A check that
  // left the control characters in would read a different string than it does.
  // biome-ignore lint/suspicious/noControlCharactersInRegex: see just above
  const href = raw.replace(/[\u0000-\u0020\u007F]/g, "");
  if (href === "") return null;
  // Schemeless goes with the rest. This also closes the protocol-relative shape.
  // The WHATWG parser reads a backslash as a slash for a special scheme. So
  // `\\evil.example`, `/\evil.example` and `\/evil.example` reach another origin
  // while reading as a relative path.
  if (!HAS_SCHEME.test(href)) return null;
  return SAFE_SCHEME.test(href) ? href : null;
}
