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

import {
  mkdirSync,
  mkdtempSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import ts from "typescript";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { extensionLayers, filesUnder, scriptKindFor } from "./source-tree";

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

  it("finds a layer that is itself a symlink", () => {
    // `Dirent.isDirectory()` is false for a symlink, so a unit whose frontend/
    // is a link was absent from the walk rather than refused — and the census
    // floor stays green off the other units, so nothing notices. Git commits
    // symlinks, so that unit ships in a PR that looks like two ordinary files.
    const real = join(dir, "elsewhere");
    mkdirSync(real, { recursive: true });
    writeFileSync(join(real, "s.tsx"), "");
    mkdirSync(join(dir, "unit"), { recursive: true });
    symlinkSync(real, join(dir, "unit", "frontend"), "dir");
    expect(rel(extensionLayers(dir))).toEqual(["unit/frontend"]);
    // And the walk reads THROUGH it, or finding the layer buys nothing.
    expect(filesUnder(join(dir, "unit", "frontend")).length).toBe(1);
  });

  it("finds a layer hidden behind a symlinked parent", () => {
    // Refusing to traverse an intermediate link would only move the hiding
    // place one directory up, so links are followed wherever they sit.
    //
    // The target sits OUTSIDE the walked root, so the link is the only route to
    // it. With it inside, the walk reaches the layer by its real path anyway
    // and the case passes without following anything.
    const outside = mkdtempSync(join(tmpdir(), "source-tree-away-"));
    try {
      mkdirSync(join(outside, "frontend"), { recursive: true });
      symlinkSync(outside, join(dir, "hop"), "dir");
      expect(rel(extensionLayers(dir))).toEqual(["hop/frontend"]);
    } finally {
      rmSync(outside, { recursive: true, force: true });
    }
  });

  it("reports one layer when a SIBLING link reaches it", () => {
    // The parents are distinct real paths, so both are walked and the layer is
    // reached twice — a shape the parent-link case below cannot produce, and
    // the one that made the layer come back doubled.
    mkdirSync(join(dir, "a", "frontend"), { recursive: true });
    symlinkSync(join(dir, "a", "frontend"), join(dir, "b"), "dir");
    mkdirSync(join(dir, "c"), { recursive: true });
    symlinkSync(join(dir, "a", "frontend"), join(dir, "c", "frontend"), "dir");
    expect(extensionLayers(dir).length).toBe(1);
  });

  it("still finds a layer whose target sits beside it", () => {
    // The case a single visited set breaks: `elsewhere` is walked through on
    // the way to nothing, and its real path is also the layer's target, so the
    // layer is marked seen before it is ever returned. Walked-through and
    // emitted are different questions.
    const real = join(dir, "elsewhere");
    mkdirSync(real, { recursive: true });
    writeFileSync(join(real, "s.tsx"), "");
    mkdirSync(join(dir, "unit"), { recursive: true });
    symlinkSync(real, join(dir, "unit", "frontend"), "dir");
    expect(rel(extensionLayers(dir))).toEqual(["unit/frontend"]);
  });

  it("reports one layer when two paths reach it", () => {
    // Keyed on the REAL path, so a link and its target are one place rather
    // than two readings of one layer — which would judge every file in it
    // twice and report every finding twice.
    mkdirSync(join(dir, "unit", "frontend"), { recursive: true });
    symlinkSync(join(dir, "unit"), join(dir, "alias"), "dir");
    expect(extensionLayers(dir).length).toBe(1);
  });

  it("survives a symlink cycle instead of dying on it", () => {
    // Following links is what makes a cycle reachable, so the guard is part of
    // the same change. `loop -> ..` is the whole of it, and without the
    // realpath-keyed visited set the walk recurses until the stack gives out —
    // the gate dies rather than reporting, which reads as a broken gate.
    mkdirSync(join(dir, "unit", "frontend"), { recursive: true });
    symlinkSync(dir, join(dir, "unit", "loop"), "dir");
    expect(rel(extensionLayers(dir))).toEqual(["unit/frontend"]);
  });

  it("picks a dialect a file can actually be parsed as", () => {
    // ONE answer, because there were two: the native-control gate read a `.js`
    // as TypeScript and this one read it as JavaScript, neither stating why.
    // `.ts` must never be asked for TSX — `<T>()` there is a type argument, and
    // asking would misparse every ordinary generic.
    expect(scriptKindFor("a.tsx")).toBe(ts.ScriptKind.TSX);
    expect(scriptKindFor("a.jsx")).toBe(ts.ScriptKind.TSX);
    expect(scriptKindFor("a.js")).toBe(ts.ScriptKind.JS);
    expect(scriptKindFor("a.mjs")).toBe(ts.ScriptKind.JS);
    expect(scriptKindFor("a.cjs")).toBe(ts.ScriptKind.JS);
    expect(scriptKindFor("a.ts")).toBe(ts.ScriptKind.TS);
    expect(scriptKindFor("a.mts")).toBe(ts.ScriptKind.TS);
    expect(scriptKindFor("a.cts")).toBe(ts.ScriptKind.TS);
    // The property the .ts arm exists for, rather than the enum value: a
    // generic call in a .ts file parses, and would be a syntax error as TSX.
    const generic = ts.createSourceFile(
      "a.ts",
      "const x = f<T>();",
      ts.ScriptTarget.ES2022,
      true,
      scriptKindFor("a.ts"),
    );
    expect(generic.statements.length).toBe(1);
  });

  it("reads files behind a symlinked subdirectory", () => {
    // `frontend/vendor -> ../../elsewhere` — the files under it were never
    // collected and so never judged, while a bundler follows the link and
    // ships what is behind it. The gate read past them in silence, which is
    // the failure mode this whole file exists to refuse.
    const outside = mkdtempSync(join(tmpdir(), "source-tree-vendor-"));
    try {
      writeFileSync(join(outside, "shipped.tsx"), "");
      writeFileSync(join(dir, "own.ts"), "");
      symlinkSync(outside, join(dir, "vendor"), "dir");
      expect(rel(filesUnder(dir))).toEqual(["own.ts", "vendor/shipped.tsx"]);
    } finally {
      rmSync(outside, { recursive: true, force: true });
    }
  });

  it("survives a cycle while reading files, not only while finding layers", () => {
    // Following links is what makes a cycle reachable, and filesUnder follows
    // them now too — so it needs the guard for the same reason extensionLayers
    // does. Without it the walk recurses until the stack gives out, and a gate
    // that dies reads as a broken gate rather than as a finding.
    writeFileSync(join(dir, "own.ts"), "");
    mkdirSync(join(dir, "deep"), { recursive: true });
    symlinkSync(dir, join(dir, "deep", "loop"), "dir");
    expect(rel(filesUnder(dir))).toEqual(["own.ts"]);
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
