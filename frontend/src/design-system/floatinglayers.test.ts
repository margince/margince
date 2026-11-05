// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import {
  alternativesOf,
  appStylesheets,
  classesOf,
  compounds,
  declaredValues,
  type Rule,
  rulesOf,
  selectorList,
  specificity,
  splitTopLevel,
} from "../../scripts/lib/css-rules";
import { themes } from "./tokens-testing";

// A layer that hangs over the page hides what is under it. Level 1's pane is
// see-through in dark, so a layer that falls back to it shows the page through.

const frontendRoot = join(import.meta.dirname, "..", "..");
const OPAQUE = /^#[0-9a-f]{6}$/i;
const NAMED = /^var\((--[\w-]+)\)$/;
// Every rung's ground is opaque in both themes (surfaces.test.ts).
const GROUND = "var(--ground)";

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

// A layer hangs over the page when it is lifted and casts the pop shadow.
function floatingLayers(rules: readonly Rule[]): string[] {
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

function seeThrough(rules: readonly Rule[]): string[] {
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

const probe = (selector: string, body: string): Rule => ({
  file: "probe.css",
  selector,
  body,
});

describe("a layer that hangs over the page hides it", () => {
  const rules = rulesOf(appStylesheets(frontendRoot));

  it("finds a layer by its shadow and its position, across rules", () => {
    expect(
      floatingLayers([
        probe(".pop", "position: fixed; box-shadow: var(--shadow-pop)"),
        probe(".split", "position: absolute"),
        probe(".split", "box-shadow: 0 0 0 1px red, var(--shadow-pop)"),
        probe(".flow", "position: relative; box-shadow: var(--shadow-pop)"),
        probe(".lifted", "z-index: 1; box-shadow: var(--shadow-pop)"),
        probe(".flat", "position: fixed; box-shadow: var(--shadow-rest)"),
      ]),
    ).toEqual([".lifted", ".pop", ".split"]);
  });

  it("reads a ground from a compounded class and under an image layer", () => {
    expect(
      seeThrough([
        probe(":where(.bar)", "background-color: var(--bgPage)"),
        probe(".bar", "background-color: var(--pane)"),
        probe(".bar.open", "position: fixed; box-shadow: var(--shadow-pop)"),
        probe(".bar.open", "position: static; background: transparent"),
        probe(".orb", "position: absolute; box-shadow: var(--shadow-pop)"),
        probe(".orb", "background: radial-gradient(red, blue), var(--bgPage)"),
        probe(".bare", "position: fixed; box-shadow: var(--shadow-pop)"),
      ]),
    ).toEqual([".bar.open: var(--pane) in dark", ".bare: no ground"]);
  });

  it("reads every layer the tree floats", () => {
    const found = floatingLayers(rules);
    for (const known of [
      ".notifbell-loose",
      ".popover-panel",
      ".accountmenu",
    ]) {
      expect(found).toContain(known);
    }
  });

  it("paints every one of them an opaque ground in both themes", () => {
    expect(seeThrough(rules)).toEqual([]);
  });
});
