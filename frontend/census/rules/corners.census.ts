// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Corner gate: one ladder, and the squircle that rides on it.
//
// 1. Every corner reads the ladder: `var(--r-*)`, `0`, `inherit`, `50%`, or a
//    `calc()` over a rung. A raw px value is a rung nobody declared.
// 2. A round thing opts out of the squircle. tokens.css sets
//    `corner-shape: squircle` on `:where(*)`, so a pill or a circle cancels it
//    with `corner-shape: round`, or an avatar draws as a squircle tile.
// 3. Every cornered pseudo-element is named beside the universal selector, and
//    `squircle` is declared nowhere else.
//
// The ladder is read from tokens.css rather than restated here. A unit ships
// CSS into the same page from the same tokens, so the extension sheets count.

import { join } from "node:path";
import { selectorList } from "../../scripts/lib/css-rules";
import { PHONE_MAX_WIDTH } from "../../src/lib/viewport";
import { withoutComments } from "../../src/testing/css";
import { type CensusFile, type CensusRule, srcDir } from "../tree";

const tokensFile = join(srcDir, "design-system", "tokens.css");

/** The ladder, as `tokens.css` itself declares it. */
export function ladder(tokens: string): Map<string, string> {
  const found = new Map<string, string>();
  const base = tokens.slice(0, tokens.indexOf("@supports"));
  const decl = /(--r-[a-z]+)\s*:\s*([^;]+);/g;
  for (let m = decl.exec(base); m; m = decl.exec(base)) {
    found.set(m[1], m[2].trim());
  }
  return found;
}

type AtRule = { text: string; start: number; end: number };

/** Every at-rule opened by `prelude`, each read to its own closing brace. */
export function atRules(tokens: string, prelude: string): AtRule[] {
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

export const SQUIRCLE = "@supports (corner-shape: squircle)";
export const PHONE = `@media (max-width: ${PHONE_MAX_WIDTH}px)`;

/** The same ladder as the squircle block redeclares it. */
export function squircleLadder(tokens: string): Map<string, string> {
  return rungDeclarations(atRules(tokens, SQUIRCLE)[0]?.text ?? "");
}

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

/**
 * The selectors the squircle rule is declared against, read from the rule that
 * sets `corner-shape` rather than from the text of the file.
 */
export function squircleSelectors(tokens: string): string[] {
  const block = withoutComments(
    tokens.slice(tokens.indexOf("@supports (corner-shape: squircle)")),
  );
  const rule = /([^{}]*)\{([^{}]*)\}/g;
  for (let m = rule.exec(block); m; m = rule.exec(block)) {
    // Declaration boundaries, not a substring: `--corner-shape: squircle` paints
    // nothing, and `squircle-variant` is a different value.
    if (
      !/(?:^|;)\s*corner-shape\s*:\s*squircle\s*(?:!important\s*)?(?:;|$)/.test(
        m[2],
      )
    ) {
      continue;
    }
    // Whitespace inside a selector is the descendant combinator, so it is
    // collapsed and not removed: `:where(*) ::before` is not the subject's own
    // generated box.
    return selectorList(m[1]).map((one) => one.replace(/\s+/g, " "));
  }
  return [];
}

/**
 * The pseudo-elements a rule gives a corner to. `*` matches elements only, so
 * each cornered generated box is named beside the universal selector. The
 * doubled tokens reach it anyway: a custom property inherits and
 * `corner-shape` does not.
 */
export function corneredPseudoElements(text: string): string[] {
  const out = new Set<string>();
  const rule = /([^{}]*)\{([^{}]*)\}/g;
  for (let m = rule.exec(text); m; m = rule.exec(text)) {
    const selector = m[1].replace(/\/\*[\s\S]*?\*\//g, "");
    if (!selector.includes("::") || radii(m[2]).length === 0) {
      continue;
    }
    for (const pseudo of selector.match(/::[a-z-]+/g) ?? []) {
      out.add(pseudo);
    }
  }
  return [...out];
}

/** A corner that is fully round, and so has to cancel the squircle. */
export function isRound(value: string): boolean {
  return /var\(--r-full\)|50%/.test(value);
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

function squareRounds(sheets: readonly Sheet[]): string[] {
  return sheets.flatMap((file) =>
    blocks(file.text).flatMap((block) => {
      const round = radii(block.body).filter(
        (radius) => isRound(radius.value) && !radius.waived,
      );
      if (round.length === 0 || /corner-shape\s*:\s*round/.test(block.body)) {
        return [];
      }
      return [
        `${file.rel}: \`${round[0].property}: ${round[0].value}\` is round and does not cancel the squircle; add \`corner-shape: round;\``,
      ];
    }),
  );
}

// The universal form, not the bare name: `.card::before` under the query
// covers none of the tree's own generated boxes.
function unlistedPseudos(sheets: readonly Sheet[], tokens: string): string[] {
  const listed = squircleSelectors(tokens);
  return sheets.flatMap((file) =>
    corneredPseudoElements(file.text)
      .filter((pseudo) => !listed.includes(`:where(*)${pseudo}`))
      .map(
        (pseudo) =>
          `${file.rel}: \`${pseudo}\` has a corner and is not named in the squircle rule; add \`:where(*)${pseudo}\` to design-system/tokens.css`,
      ),
  );
}

function otherShapes(sheets: readonly Sheet[]): string[] {
  return sheets.flatMap((file) =>
    [...file.text.matchAll(/corner-shape\s*:\s*([^;}]+)/g)]
      .filter((m) => m[1].trim() !== "round")
      .map(
        (m) =>
          `${file.rel}: \`${m[0]}\`; corner-shape is declared once, on :where(*) in tokens.css, and a call site says only \`round\``,
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
    return [
      ...strays(corpus, rungs),
      ...squareRounds(corpus),
      ...[...new Set(unlistedPseudos(corpus, tokens))],
      ...otherShapes(corpus),
    ];
  },
  advice:
    "A corner reads a rung of the ladder in design-system/tokens.css; pick the " +
    "nearest rather than adding a value nobody chose, or waive a genuine " +
    "one-off in line: /* ds:ignore <reason> */. A pill or circle adds " +
    "`corner-shape: round;` beside its radius. A cornered pseudo-element is " +
    "named as `:where(*)<pseudo>` in the squircle rule, and nothing else " +
    "declares `squircle`.",
};
