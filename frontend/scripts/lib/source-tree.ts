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
export function filesUnder(dir: string): string[] {
  if (!existsSync(dir)) return [];
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = join(dir, entry.name);
    if (entry.isDirectory()) {
      return entry.name === "node_modules" ? [] : filesUnder(full);
    }
    return MODULE_FILE.test(entry.name) ? [full] : [];
  });
}

// extensionLayers returns every directory named `frontend` under `root`, at ANY
// depth. The shell gates these replaced globbed `extensions/*/frontend`, so a
// unit that nested one was invisible to them — latent in today's tree, which
// has only top-level layers, and latent is exactly how a walk-shape hole
// survives to bite somebody later.
export function extensionLayers(
  root: string,
  seen = new Set<string>(),
): string[] {
  if (!existsSync(root)) return [];
  // Keyed on the REAL path, so a link and its target are one place. Without it
  // a cycle — `extensions/a/loop -> ..` is enough — recurses until the stack
  // gives out, and the gate dies instead of reporting.
  const here = realpathSync(root);
  if (seen.has(here)) return [];
  seen.add(here);
  return readdirSync(root, { withFileTypes: true }).flatMap((entry) => {
    if (entry.name === "node_modules") return [];
    const full = join(root, entry.name);
    // A symlink is FOLLOWED, layer or not. `Dirent.isDirectory()` is false for
    // one, so both a linked layer and a unit hidden behind a linked parent were
    // absent from the walk rather than refused — and the census floor stays
    // green off the other units, so nothing notices. Git commits symlinks, so
    // such a unit ships in a PR that looks like two ordinary files.
    const isDir = entry.isSymbolicLink()
      ? existsSync(full) && statSync(full).isDirectory()
      : entry.isDirectory();
    if (!isDir) return [];
    return entry.name === "frontend" ? [full] : extensionLayers(full, seen);
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
  return extensionLayers(extensionsDir).flatMap(filesUnder);
}
