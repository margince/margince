// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The shared walk's own test, beside the walk rather than inside one of its two
// callers. Both source-wide gates read a smaller tree than they think if any of
// this is wrong, and both report the same word for it: PASS.
//
// Against a FIXTURE, because the real tree holds only .ts and .tsx at the top
// level of two flat layers — so every interesting shape here (a .jsx unit, a
// nested layer, a linked node_modules) is one the real tree cannot exercise.
// Dropping `.jsx` from the pattern was a mutant that survived a census.

import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { extensionLayers, filesUnder } from "./source-tree";

describe("the walk the source-wide gates share", () => {
  let dir = "";
  const rel = (paths: string[]) =>
    paths.map((p) => p.slice(dir.length + 1)).sort();

  beforeEach(() => {
    dir = mkdtempSync(join(tmpdir(), "source-tree-"));
  });
  afterEach(() => {
    rmSync(dir, { recursive: true, force: true });
  });

  it("collects every extension a bundler resolves, and nothing else", () => {
    // All eight, because the two gates that share this had drifted to four
    // against eight — so a unit shipping a .cjs was gated by one of them and
    // invisible to the other, for no reason either author chose.
    const modules = [
      "a.ts",
      "b.tsx",
      "c.mts",
      "d.cts",
      "e.js",
      "f.jsx",
      "g.mjs",
      "h.cjs",
    ];
    for (const n of [...modules, "skip.md", "skip.css", "skip.json"]) {
      writeFileSync(join(dir, n), "");
    }
    expect(rel(filesUnder(dir))).toEqual(modules.sort());
  });

  it("reaches a file at any depth", () => {
    mkdirSync(join(dir, "one", "two"), { recursive: true });
    writeFileSync(join(dir, "one", "two", "deep.tsx"), "");
    expect(rel(filesUnder(dir))).toEqual(["one/two/deep.tsx"]);
  });

  it("never reads node_modules, at any depth", () => {
    // pnpm links the host into extensions/<unit>/frontend/node_modules, so a
    // walk that followed it reads the entire core tree as the unit's own —
    // hundreds of findings that are not the unit's doing, in the gate that
    // exists to say what the unit did.
    mkdirSync(join(dir, "node_modules"), { recursive: true });
    writeFileSync(join(dir, "node_modules", "dep.ts"), "");
    mkdirSync(join(dir, "nested", "node_modules"), { recursive: true });
    writeFileSync(join(dir, "nested", "node_modules", "dep.ts"), "");
    writeFileSync(join(dir, "nested", "own.ts"), "");
    expect(rel(filesUnder(dir))).toEqual(["nested/own.ts"]);
  });

  it("returns nothing for a directory that is not there", () => {
    // A gate pointed at a path that does not exist must not throw its way to a
    // red run that reads like a finding; its own census floor is what catches
    // the miswiring, and it can only do that if the walk returns.
    expect(filesUnder(join(dir, "absent"))).toEqual([]);
    expect(extensionLayers(join(dir, "absent"))).toEqual([]);
  });

  it("finds a frontend layer at any depth, flat or nested", () => {
    mkdirSync(join(dir, "flat", "frontend"), { recursive: true });
    writeFileSync(join(dir, "flat", "frontend", "s.tsx"), "");
    mkdirSync(join(dir, "unit", "panel", "frontend"), { recursive: true });
    writeFileSync(join(dir, "unit", "panel", "frontend", "s.tsx"), "");
    expect(rel(extensionLayers(dir))).toEqual([
      "flat/frontend",
      "unit/panel/frontend",
    ]);
  });

  it("does not take a directory merely NAMED like a layer", () => {
    // The containment rule both gates depend on: `frontend-lib` is a sibling of
    // the layer, not the layer, and a prefix test without a separator says
    // otherwise.
    mkdirSync(join(dir, "unit", "frontend-lib"), { recursive: true });
    writeFileSync(join(dir, "unit", "frontend-lib", "s.ts"), "");
    expect(extensionLayers(dir)).toEqual([]);
  });

  it("does not descend into a node_modules looking for layers", () => {
    // A linked host package has a frontend/ of its own; reading it would hand
    // both gates the core tree wearing a unit's name.
    mkdirSync(join(dir, "node_modules", "pkg", "frontend"), {
      recursive: true,
    });
    expect(extensionLayers(dir)).toEqual([]);
  });
});
