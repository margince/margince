// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A layer that hangs over the page hides what is under it. Level 1's pane is
// see-through in dark, so a layer that falls back to it shows the page through.

import {
  alternativesOf,
  classesOf,
  compounds,
  declaredValues,
  type Rule,
  selectorList,
  specificity,
  splitTopLevel,
} from "../../scripts/lib/css-rules";
import { themes } from "../../src/design-system/tokens-testing";
import { type CensusRule, isAppSheet, sheetRules } from "../tree";

const OPAQUE = /^#[0-9a-f]{6}$/i;
const NAMED = /^var\((--[\w-]+)\)$/;
// Every rung's ground is opaque in both themes (surfaces.test.ts).
const GROUND = "var(--ground)";

// Layers the tree floats today, so a detector that goes blind fails.
export const KNOWN_LAYERS = [
  ".notifbell-loose",
  ".popover-panel",
  ".accountmenu",
];

const casts = (body: string) =>
  declaredValues(body, "box-shadow").some((v) => v.includes("--shadow-pop"));
// Back in the flow, or without its shadow: a phone layout that docks the layer.
const lands = (body: string) =>
  declaredValues(body, "position").includes("static") ||
  declaredValues(body, "box-shadow").includes("none");
// Out of the flow, or raised over its neighbours in it (a row being dragged).
const hangs = (body: string) =>
  declaredValues(body, "position").some(
    (v) => v === "fixed" || v === "absolute",
  ) || declaredValues(body, "z-index").some((v) => /^[1-9]\d*$/.test(v));

/** A layer hangs over the page when it is lifted and casts the pop shadow. */
export function floatingLayers(rules: readonly Rule[]): string[] {
  const casting = new Set<string>();
  const hanging = new Set<string>();
  for (const rule of rules) {
    for (const alternative of alternativesOf(rule.selector)) {
      if (casts(rule.body)) casting.add(alternative);
      if (hangs(rule.body)) hanging.add(alternative);
    }
  }
  return [...casting].filter((layer) => hanging.has(layer)).sort();
}

// The ground that wins, from the layer's own rules and those of a class it
// compounds (`.rail` for `.rail.collapsed`). A rule that lands it is no float.
function groundOf(layer: string, rules: readonly Rule[]): string | undefined {
  const own = classesOf(compounds(layer).at(-1) ?? "");
  const paints = (alternative: string) => {
    if (alternative === layer) return true;
    const parts = compounds(alternative);
    const classes = classesOf(parts[0] ?? "");
    return (
      parts.length === 1 &&
      compounds(layer).length === 1 &&
      classes.size > 0 &&
      [...classes].every((name) => own.has(name))
    );
  };
  let winner: { weight: number; ground: string } | undefined;
  for (const rule of rules.filter((one) => !lands(one.body))) {
    const fills = [
      ...declaredValues(rule.body, "background"),
      ...declaredValues(rule.body, "background-color"),
    ];
    const ground = splitTopLevel(fills.at(-1) ?? "", ",")
      .at(-1)
      ?.trim();
    if (!ground) continue;
    for (const piece of selectorList(rule.selector)) {
      if (!alternativesOf(piece).some(paints)) continue;
      const weight = specificity(piece);
      if (winner === undefined || weight >= winner.weight) {
        winner = { weight, ground };
      }
    }
  }
  return winner?.ground;
}

/** Every floating layer whose ground is not opaque, with the theme it fails in. */
export function seeThrough(rules: readonly Rule[]): string[] {
  return floatingLayers(rules).flatMap((layer) => {
    const ground = groundOf(layer, rules);
    if (ground === undefined) return [`${layer}: no ground`];
    if (ground === GROUND) return [];
    const name = NAMED.exec(ground)?.[1];
    return Object.entries(themes)
      .filter(
        ([, palette]) => !OPAQUE.test(name ? (palette[name] ?? "") : ground),
      )
      .map(([theme]) => `${layer}: ${ground} in ${theme}`);
  });
}

export const rule: CensusRule = {
  name: "floatinglayers",
  corpus: (tree) => tree.files.filter(isAppSheet),
  floors: (corpus) => {
    const found = floatingLayers(sheetRules(corpus));
    return KNOWN_LAYERS.filter((known) => !found.includes(known)).map(
      (known) => `${known} no longer reads as a floating layer`,
    );
  },
  across: (corpus) => seeThrough(sheetRules(corpus)),
  advice:
    "A layer that hangs over the page must paint an opaque ground in both themes.",
};
