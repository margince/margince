// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The walk the source-wide fitness functions share: which files a gate reads,
// and where an extension's frontend layer is.
//
// It exists because there are two of them now — the native-control gate in
// src/design-system/native-controls.test.ts and the extension-import gate in
// scripts/ext-imports.test.ts — and both had, independently, to answer the same
// three questions: skip node_modules, find every directory named `frontend` at
// any depth, and collect every extension a bundler resolves. A second answer to
// one question is two answers that drift until they disagree, and a walk is the
// worst place for that: the gate whose walk is narrower reads a smaller tree and
// reports the same word, PASS.
//
// They had already drifted before this file existed. One collected four
// extensions and the other eight, so a unit shipping a `.cjs` was gated by one
// of the two and invisible to the other — for no reason either author chose.
// There is ONE set here, and it is the wide one.

import type { Dirent } from "node:fs";
import { existsSync, readdirSync, realpathSync, statSync } from "node:fs";
import { join } from "node:path";
import ts from "typescript";

// Every extension a bundler resolves. Not just the two a well-behaved unit
// writes: a `"main": "screen.jsx"` is a legal unit, and a gate whose coverage
// depends on which extension a file happens to carry is a gate with a way
// around it.
export const MODULE_FILE = /\.(ts|tsx|mts|cts|js|jsx|mjs|cjs)$/;

// filesUnder walks `dir` and returns every module file beneath it.
//
// node_modules is skipped by NAME at every level, which matters most inside an
// extension: pnpm links the host into
// extensions/<unit>/frontend/node_modules/@margince/frontend, so a walk that
// followed it would read the entire core tree as though the unit shipped it.
export function filesUnder(dir: string, seen = new Set<string>()): string[] {
  return filesMatching(dir, MODULE_FILE, seen);
}

// filesMatching is the walk itself, for a gate whose corpus is not modules —
// the type-scale gate reads `.css` as well as `.tsx`, and a second walker for
// it would be a second answer to node_modules and symlinks both.
export function filesMatching(
  dir: string,
  pattern: RegExp,
  seen = new Set<string>(),
): string[] {
  if (!existsSync(dir)) return [];
  const here = realpathSync(dir);
  if (seen.has(here)) return [];
  seen.add(here);
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = join(dir, entry.name);
    // A symlinked subdirectory is DESCENDED, for the same reason a symlinked
    // layer is followed: `Dirent.isDirectory()` is false for one, so the files
    // under `frontend/vendor -> ../../elsewhere` were never collected and so
    // never judged — the gate read past them in silence. A bundler follows the
    // link and ships what is behind it.
    if (isDirectory(entry, full)) {
      return entry.name === "node_modules"
        ? []
        : filesMatching(full, pattern, seen);
    }
    return pattern.test(entry.name) ? [full] : [];
  });
}

// isDirectory answers for a symlink what `Dirent.isDirectory()` will not: it is
// false for every link, so both walks used to treat a linked directory as a
// file and skip it. A broken link answers false rather than throwing.
function isDirectory(entry: Dirent, full: string): boolean {
  return entry.isSymbolicLink()
    ? existsSync(full) && statSync(full).isDirectory()
    : entry.isDirectory();
}

// extensionLayers returns every directory named `frontend` under `root`, at ANY
// depth. The shell gates these replaced globbed `extensions/*/frontend`, so a
// unit that nested one was invisible to them — latent in today's tree, which
// has only top-level layers, and latent is exactly how a walk-shape hole
// survives to bite somebody later.
export function extensionLayers(
  root: string,
  visited = { walked: new Set<string>(), emitted: new Set<string>() },
): string[] {
  if (!existsSync(root)) return [];
  // TWO sets, keyed on real paths, and they are not interchangeable.
  //
  //   walked  — directories recursed THROUGH. Without it a cycle (`a/loop -> ..`
  //             is enough) recurses until the stack gives out, and the gate dies
  //             instead of reporting.
  //   emitted — layers already RETURNED. Two parents can reach one layer —
  //             `a/frontend` real, `b/frontend` a link to it — and those parents
  //             are distinct real paths, so both are visited and the layer came
  //             back twice: every file judged twice, every finding reported
  //             twice.
  //
  // One set for both jobs looks like the tidier version and drops a real layer:
  // a directory walked through on the way to nothing, whose real path is also a
  // layer's target, marks that layer seen before it is ever returned. That is
  // `unit/frontend -> elsewhere` where `elsewhere` sits beside it, and the walk
  // reaches `elsewhere` first.
  const here = realpathSync(root);
  if (visited.walked.has(here)) return [];
  visited.walked.add(here);
  return readdirSync(root, { withFileTypes: true }).flatMap((entry) => {
    if (entry.name === "node_modules") return [];
    const full = join(root, entry.name);
    // A symlink is FOLLOWED, layer or not. `Dirent.isDirectory()` is false for
    // one, so both a linked layer and a unit hidden behind a linked parent were
    // absent from the walk rather than refused — and the census floor stays
    // green off the other units, so nothing notices. Git commits symlinks, so
    // such a unit ships in a PR that looks like two ordinary files.
    if (!isDirectory(entry, full)) return [];
    if (entry.name !== "frontend") return extensionLayers(full, visited);
    const real = realpathSync(full);
    if (visited.emitted.has(real)) return [];
    visited.emitted.add(real);
    return [full];
  });
}

// scriptKindFor picks the dialect to parse a file as. ONE answer, because there
// were two: the native-control gate read a `.js` as TypeScript and the
// extension-import gate read it as JavaScript, neither stating why, in the pair
// this module exists to stop from drifting.
//
// `.ts` must NOT be asked for TSX — `<T>()` there is a type argument, and
// asking would misparse every ordinary generic. That is the whole reason the
// distinction exists; everything else follows from it.
export function scriptKindFor(path: string): ts.ScriptKind {
  if (/\.(tsx|jsx)$/.test(path)) return ts.ScriptKind.TSX;
  if (/\.(js|mjs|cjs)$/.test(path)) return ts.ScriptKind.JS;
  return ts.ScriptKind.TS;
}

// A unit's screen is shipped UI in the same bundle, so a gate stopping at
// frontend/src would hold the core to a standard the extension tier escapes.
export function extensionFrontendFiles(extensionsDir: string): string[] {
  // A fresh visited set per layer, and NOT `.flatMap(filesUnder)` — a bare
  // reference hands flatMap's INDEX to the second parameter, so layer 1 walks
  // with `1` where a Set belongs. The compiler catches it; the shape is worth
  // naming because the point-free version is the one that looks tidier.
  return extensionLayers(extensionsDir).flatMap((layer) => filesUnder(layer));
}
