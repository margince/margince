// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Corner gate: one ladder. Every corner reads it: `var(--r-*)`, `0`,
// `inherit`, `50%`, or a `calc()` over a rung. A raw px value is a rung nobody
// declared.
//
// The ladder is read from tokens.css rather than restated here. A unit ships
// CSS into the same page from the same tokens, so the extension sheets count.

import { join } from "node:path";
import { PHONE_MAX_WIDTH } from "../../src/lib/viewport";
import { withoutComments } from "../../src/testing/css";
import { type CensusFile, type CensusRule, srcDir } from "../tree";

const tokensFile = join(srcDir, "design-system", "tokens.css");

/** The ladder, as `tokens.css` itself declares it. */
export function ladder(tokens: string): Map<string, string> {
  const found = new Map<string, string>();
  const decl = /(--r-[a-z]+)\s*:\s*([^;]+);/g;
  for (let m = decl.exec(tokens); m; m = decl.exec(tokens)) {
    found.set(m[1], m[2].trim());
  }
  return found;
}

type AtRule = { text: string; start: number; end: number };

/** Every at-rule opened by `prelude`, each read to its own closing brace. */
function atRules(tokens: string, prelude: string): AtRule[] {
  const out: AtRule[] = [];
  for (let at = tokens.indexOf(prelude); at >= 0; ) {
    let depth = 0;
    let end = tokens.length;
    for (let i = tokens.indexOf("{", at); i >= 0 && i < tokens.length; i += 1) {
      if (tokens[i] === "{") depth += 1;
      if (tokens[i] === "}") depth -= 1;
      if (depth === 0) {
        end = i + 1;
        break;
      }
    }
    out.push({ text: tokens.slice(at, end), start: at, end });
    at = tokens.indexOf(prelude, end);
  }
  return out;
}

function rungDeclarations(block: string): Map<string, string> {
  const found = new Map<string, string>();
  const decl = /(--r-[a-z]+)\s*:\s*([^;]+);/g;
  for (let m = decl.exec(block); m; m = decl.exec(block)) {
    found.set(m[1], m[2].trim());
  }
  return found;
}

/** The rung a rung's value names, where the rung is an alias of another. */
export function aliasOf(value: string): string | undefined {
  return /^var\((--r-[a-z]+)\)$/.exec(value)?.[1];
}

export const PHONE = `@media (max-width: ${PHONE_MAX_WIDTH}px)`;

/** The phone blocks that redeclare a rung, each with where it sits. */
export function phoneRungBlocks(
  tokens: string,
): { rungs: Map<string, string>; start: number }[] {
  return atRules(tokens, PHONE)
    .map((block) => ({
      rungs: rungDeclarations(withoutComments(block.text)),
      start: block.start,
    }))
    .filter((block) => block.rungs.size > 0);
}

type Block = { body: string; at: number };

/**
 * Every innermost rule block in a stylesheet. A block nested in `@media` or
 * `@supports` is innermost too, and a raw radius hides as well there.
 */
export function blocks(text: string): Block[] {
  const out: Block[] = [];
  const rule = /\{([^{}]*)\}/g;
  for (let m = rule.exec(text); m; m = rule.exec(text)) {
    out.push({ body: m[1], at: m.index });
  }
  return out;
}

type Radius = { property: string; value: string; waived: boolean };

/**
 * The radius declarations in one block, with any in-line waiver noted. The scan
 * reads a copy with comments blanked, since a commented-out declaration draws
 * nothing. Offsets survive the blanking, so the waiver is read from the
 * original text.
 */
export function radii(body: string): Radius[] {
  const out: Radius[] = [];
  const active = withoutComments(body);
  const decl = /(border(?:-[a-z]+)*-radius)\s*:\s*([^;}]+)(;?)/g;
  for (let m = decl.exec(active); m; m = decl.exec(active)) {
    const lineEnd = body.indexOf("\n", m.index + m[0].length);
    out.push({
      property: m[1],
      value: m[2].trim(),
      waived: waivedWithReason(
        body.slice(m.index + m[0].length, lineEnd === -1 ? undefined : lineEnd),
      ),
    });
  }
  return out;
}

/**
 * A waiver is the sentence, not the marker. `/* ds:ignore *\/` alone is a
 * finding: it says a rule is skipped and not why.
 */
function waivedWithReason(trailing: string): boolean {
  const comment = /\/\*([\s\S]*?)\*\//.exec(trailing);
  if (!comment) {
    return false;
  }
  const reason = comment[1]
    .trim()
    .replace(/^ds:ignore\b/, "")
    .trim();
  return /^ds:ignore\b/.test(comment[1].trim()) && reason.length > 0;
}

/** Stands in for a masked calc() while a shorthand is split on its spaces. */
const CALC = "@calc";

/**
 * True where every corner in a value names the ladder, or has no radius at
 * all. A shorthand is read corner by corner, since one raw corner can sit
 * among three tokens.
 */
export function readsTheLadder(value: string, rungs: Set<string>): boolean {
  // A calc() is one corner with its own spaces, so it is masked before the
  // split. It stands where it adjusts a rung: inner = outer minus padding.
  const calls: string[] = [];
  const masked = value.replace(
    /calc\([^()]*(?:\([^()]*\)[^()]*)*\)/g,
    (whole) => {
      calls.push(whole);
      return `${CALC}${calls.length - 1}`;
    },
  );
  // The `/` of an elliptical corner separates the across radii from the down.
  return masked
    .trim()
    .split(/[\s/]+/)
    .filter((corner) => corner !== "")
    .every((corner) => {
      const call = new RegExp(`^${CALC}(\\d+)$`).exec(corner);
      if (call) {
        return namedRungs(calls[Number(call[1])], rungs).length > 0;
      }
      if (corner === "0" || corner === "inherit" || corner === "50%") {
        return true;
      }
      return (
        namedRungs(corner, rungs).length === 1 &&
        /^var\(--r-[a-z]+\)$/.test(corner)
      );
    });
}

/** The ladder rungs a fragment of a value names. */
function namedRungs(text: string, rungs: Set<string>): string[] {
  const out: string[] = [];
  const use = /var\((--r-[a-z]+)\)/g;
  for (let m = use.exec(text); m; m = use.exec(text)) {
    if (rungs.has(m[1])) {
      out.push(m[1]);
    }
  }
  return out;
}

type Sheet = Pick<CensusFile, "rel" | "text">;

function strays(sheets: readonly Sheet[], rungs: Set<string>): string[] {
  return sheets.flatMap((file) =>
    blocks(file.text).flatMap((block) =>
      radii(block.body)
        .filter(
          (radius) => !radius.waived && !readsTheLadder(radius.value, rungs),
        )
        .map(
          (radius) =>
            `${file.rel}: \`${radius.property}: ${radius.value}\` names a length, not a rung (${[...rungs].join(", ")})`,
        ),
    ),
  );
}

export const rule: CensusRule = {
  name: "corners",
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        (file.area === "src" || file.area === "extensions") &&
        file.rel.endsWith(".css") &&
        file.path !== tokensFile,
    ),
  floors: (corpus, tree) => [
    ...(corpus.length > 40 ? [] : ["fewer than 40 stylesheets were read"]),
    ...(tree.at(tokensFile) ? [] : ["design-system/tokens.css was not read"]),
  ],
  across: (corpus, tree) => {
    const tokens = tree.at(tokensFile)?.text ?? "";
    const rungs = new Set(ladder(tokens).keys());
    return strays(corpus, rungs);
  },
  advice:
    "A corner reads a rung of the ladder in design-system/tokens.css; pick the " +
    "nearest rather than adding a value nobody chose, or waive a genuine " +
    "one-off in line: /* ds:ignore <reason> */.",
};
