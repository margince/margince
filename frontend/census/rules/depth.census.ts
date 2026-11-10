// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Depth, source-wide. This product has three of it, for one light source above:
//
//   --shadow-rest  a surface or a filled verb has a top side
//   --shadow-well  a field is a place to put something, so it has a floor
//   --shadow-pop   a popover, menu or drawer is above the plane
//
// Change a token and every surface follows, in both themes. A rule that spells
// its own shadow does not follow, and nobody sees it in a diff or a screenshot.
// The register below lists box-shadows that are not depth, each with its why.

import type { CensusRule } from "../tree";

// `none` and `0` are the suppression half: a control with no fill takes the
// resting layer back off, which is a decision and not a drift.
const HOUSE_SHADOW =
  /^(none|0|var\(--shadow-(rest|well|pop)\)|var\(--focus-glow(-danger|-ai)?\))$/;

/**
 * Whether every layer of a shadow is a rule rather than depth. A flat inset is
 * a rail or a ring drawn inside the box, and says nothing about height.
 *
 * Depth is what has blur, and a blurred inset is a well spelled by hand. So the
 * third length is zero or absent in every layer, not only the first. A flat
 * ring can open a value that then adds an outer bloom.
 */
function drawsOnlyFlatInsets(value: string): boolean {
  return topLevelLayers(value).every((layer) => {
    if (!/^inset\b/.test(layer)) {
      return false;
    }
    const lengths = layer
      .slice("inset".length)
      .trim()
      .split(/\s+/)
      .filter((token) => /^-?(\d*\.)?\d+(px|rem|em)?$/.test(token));
    // Fewer than three lengths is offset-only: no blur to be depth with.
    return lengths.length < 3 || /^0(px|rem|em)?$/.test(lengths[2]);
  });
}

/**
 * One shadow's comma-separated layers, split only at paren depth zero, since
 * `color-mix(in srgb, …)` carries commas of its own.
 */
function topLevelLayers(value: string): string[] {
  const layers: string[] = [];
  let depth = 0;
  let current = "";
  for (const character of value) {
    if (character === "(") depth += 1;
    if (character === ")") depth -= 1;
    if (character === "," && depth === 0) {
      layers.push(current.trim());
      current = "";
      continue;
    }
    current += character;
  }
  layers.push(current.trim());
  return layers.filter((layer) => layer !== "");
}

/**
 * Every box-shadow value in one stylesheet that is not a house spelling, as
 * `path: box-shadow: <value>`. Statements, not lines: the formatter wraps a
 * multi-layer shadow. Comments go first, so prose about a shadow is not one.
 */
export function spelledShadows(rel: string, css: string): string[] {
  const text = css.replace(/\/\*[\s\S]*?\*\//g, "");
  return [...text.matchAll(/box-shadow:\s*([^;}]+)[;}]/g)]
    .map((match) => match[1].trim().replace(/\s+/g, " "))
    .filter((value) => !HOUSE_SHADOW.test(value) && !drawsOnlyFlatInsets(value))
    .map((value) => `${rel}: box-shadow: ${value}`);
}

// Each spelled shadow that is not depth, and why.
export const NOT_DEPTH: readonly string[] = [
  // The agent panel's state pip is a light, not a box: a 6px dot with a
  // halo in the tone the orb is running. A halo is a thing being lit rather
  // than a thing at a height, and no elevation token carries it.
  "src/app/agentrail.css: box-shadow: 0 0 6px -1px var(--arTone)",
  // Not a shadow at all. A phone sheet's sticky action row stops at the
  // sheet's padding. Its own ground, cast downward, fills that padding, so
  // text scrolling under the row does not show beneath the buttons.
  "src/design-system/atoms.css: box-shadow: 0 var(--space-6) 0 var(--ground)",
  // The ring separating two overlapping faces, in the colour of the surface
  // behind them. A shadow rather than a border so a stacked face stays the
  // size of the lone chip beside it (avatarstack.css says why).
  "src/design-system/avatarstack.css: box-shadow: 0 0 0 2px var(--bgElevated)",
  // The verdict a reader gave a drafted card, held as a coloured ring
  // until the card springs back. A ring rather than a tint because the card
  // underneath carries prose somebody is still reading.
  "src/design-system/decisiondeck.css: box-shadow: 0 0 0 2px var(--accent)",
  "src/design-system/decisiondeck.css: box-shadow: 0 0 0 2px var(--ai)",
  "src/design-system/decisiondeck.css: box-shadow: 0 0 0 2px var(--borderStrong)",
  "src/design-system/decisiondeck.css: box-shadow: 0 0 0 2px var(--danger)",
  // The agent orb is a lit sphere, not a surface at a depth: inner shading,
  // a hairline inside the edge, and the ball's own bloom. The second value
  // is the same chrome at the small container rung.
  "src/design-system/margince-core.css: box-shadow: inset 0 -40px 60px -46px var(--coreC1), inset 0 0 0 1px color-mix(in srgb, var(--overlayLight) 8%, transparent), 0 0 44px -22px var(--coreC2)",
  "src/design-system/margince-core.css: box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--overlayLight) 6%, transparent), 0 0 12px -7px var(--coreC2)",
  // The capture mark while the agent is reading: a halo in the same tint as
  // the mark's fill. That is the state, not an elevation.
  "src/features/contacts/contactnetwork.css: box-shadow: 0 0 0 4px var(--accentLight)",
  // The pulse marking the finding a jump landed on: a ring that holds for a
  // beat and fades, layered beside the browser's own focus ring. Four values:
  // the keyframe set, plus the static ring that replaces it under reduced
  // motion.
  "src/features/onboarding/onboarding-conversation/conversation.css: box-shadow: 0 0 0 2px var(--aiText)",
  "src/features/onboarding/onboarding-conversation/conversation.css: box-shadow: 0 0 0 3px transparent",
  "src/features/onboarding/onboarding-conversation/conversation.css: box-shadow: 0 0 0 3px var(--aiText)",
  "src/features/onboarding/onboarding-conversation/conversation.css: box-shadow: 0 0 0 3px var(--aiText)",
  // Not a shadow at all: a sticky bar's full-bleed fill and the hairline
  // behind it. Only a spread reaches past the bar's own box, and
  // conversation.css explains the ordering that keeps the hairline to one pixel.
  "src/features/onboarding/onboarding-conversation/conversation.css: box-shadow: 0 calc(100vw + 1px) 0 100vw var(--bgElevated), 0 0 0 100vw var(--borderSubtle)",
  // The one drop shadow outside --shadow-pop. The reason is the ground: this
  // dialog floats on the rail's own dark brand surface. There
  // --shadow-pop is tinted for the page and reads as a grey smear. Tinted
  // to --obDialogShadow so it is the shadow that ground would actually cast.
  "src/features/onboarding/onboarding.css: box-shadow: 0 18px 46px color-mix(in srgb, var(--obDialogShadow) 44%, transparent)",
];

/** The entries of `found` beyond `allowed`, and of `allowed` beyond `found`. */
export function registerDrift(
  found: readonly string[],
  allowed: readonly string[],
): string[] {
  const left = [...allowed];
  const extra: string[] = [];
  for (const entry of found) {
    const at = left.indexOf(entry);
    if (at < 0) extra.push(entry);
    else left.splice(at, 1);
  }
  return [
    ...extra,
    ...left.map((entry) => `${entry} is registered and no sheet spells it`),
  ];
}

export const rule: CensusRule = {
  name: "depth",
  // A unit's screen ships on the same page, so the extension tier counts.
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        (file.area === "src" || file.area === "extensions") &&
        file.rel.endsWith(".css"),
    ),
  floors: (corpus) =>
    corpus.length > 50
      ? []
      : ["the depth sweep found almost no stylesheets, which is a broken walk"],
  // tokens.css is where the values live; a literal there is its job.
  across: (corpus) =>
    registerDrift(
      corpus
        .filter((file) => !file.rel.endsWith("tokens.css"))
        .flatMap((file) => spelledShadows(file.rel, file.text))
        .sort(),
      NOT_DEPTH,
    ),
  advice:
    "A shadow reads `var(--shadow-rest)` for a top side, `var(--shadow-well)` " +
    "for a field's floor, or `var(--shadow-pop)` above the plane. A shadow that " +
    "is not depth (a ring, a halo, a fill trick) goes in NOT_DEPTH in " +
    "census/rules/depth.census.ts with the reason.",
};
