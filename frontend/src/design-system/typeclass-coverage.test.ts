// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { filesMatching } from "../../scripts/lib/source-tree";

// Fitness function for a type class that does not exist.
//
// `base.css` owns the type scale, and a component asks for a step by name:
// `t-caption`, `t-label`, `t-mono`. Ask for a name the scale does not carry and
// CSS says nothing at all — no warning, no fallback, no failing build. The
// element simply inherits, so the figure a card exists to show renders at body
// size and the provenance line under a fact renders at the same weight as the
// fact. That is not a style disagreement, it is a component whose type is
// decided by whatever happens to be around it.
//
// It has happened three times. `t-h3` was named by `StatCard` and declared
// nowhere, so every tile's reading rendered at body size; `t-meta` was asked
// for at eleven sites across six screens, and `t-muted` at one, and neither was
// ever a rule — both meant what `t-caption` already spells, so both were
// replaced rather than declared. A fourth spelling of quiet grey 12px would
// have been the wrong fix for any of them.
//
// The corpus is derived from the tree on both sides — every `.tsx` for the ask,
// every `.css` for the declaration — so a new screen is covered the day it is
// written, and so is a step added to the scale.
//
// WHAT THIS DOES NOT CATCH: the same defect in a class that is not part of the
// type scale. `.evidence-verdict` is asked for by a screen and declared by no
// sheet today, and this gate cannot see it, because "every class name in the
// tree resolves" needs a census of every stylesheet's selectors — including the
// ones only ever written as a descendant of something else — and a gate with
// that many exceptions is one that gets waived rather than fixed. The type
// scale is the bounded claim: ONE owner, one flat namespace, and a name that
// says out loud which family it belongs to.
const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");

const TYPE_CLASS = /^t-[a-z0-9-]+$/;

/** Every `t-*` class a component asks for, and where it asks. */
function asked(): Map<string, string[]> {
  const sites = new Map<string, string[]>();
  for (const file of filesMatching(root, /\.tsx$/)) {
    const source = readFileSync(file, "utf8");
    // Every spelling a className takes: a plain string, a string inside an
    // expression container (`className={"t-label"}`, either quote), and a
    // template literal that composes one. The literal's `${...}` holes are
    // split on with the whitespace, so a computed half contributes no token
    // rather than a half-token that matches nothing.
    //
    // The expression-container forms were missing, and that is the one way this
    // gate must not break: a component spelling an undeclared class as
    // `className={"t-meta"}` was simply not read, and a scan that cannot see a
    // defect reports PASS over it.
    for (const match of source.matchAll(
      /className=(?:"([^"]*)"|\{\s*"([^"]*)"\s*\}|\{\s*'([^']*)'\s*\}|\{`([^`]*)`\})/g,
    )) {
      const line = source.slice(0, match.index).split("\n").length;
      const text = match[1] ?? match[2] ?? match[3] ?? match[4] ?? "";
      for (const token of text.split(/[\s{}$`]+/)) {
        if (TYPE_CLASS.test(token)) {
          sites.set(token, [
            ...(sites.get(token) ?? []),
            `${file.slice(root.length + 1)}:${line}`,
          ]);
        }
      }
    }
  }
  return sites;
}

/** Every `t-*` class a stylesheet declares. */
function declared(): Set<string> {
  const names = new Set<string>();
  for (const file of filesMatching(root, /\.css$/)) {
    for (const match of readFileSync(file, "utf8").matchAll(
      /\.(t-[a-z0-9-]+)/g,
    )) {
      names.add(match[1]);
    }
  }
  return names;
}

describe("the type scale", () => {
  it("declares every step a component asks for", () => {
    const have = declared();
    const missing = [...asked()]
      .filter(([name]) => !have.has(name))
      .map(([name, sites]) => `${name} — ${sites.slice(0, 3).join(", ")}`);
    expect(missing).toEqual([]);
  });

  // Both halves have to be able to fail, and under-recognition is the one way
  // this gate must not break: a regex that stopped matching would report an
  // empty ask, find nothing missing, and pass. So it asserts it can still see
  // the scale that is in the tree today.
  it("finds the asks and the declarations", () => {
    expect(asked().size).toBeGreaterThanOrEqual(10);
    expect(declared().size).toBeGreaterThanOrEqual(10);
  });
});
