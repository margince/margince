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

import { existsSync, readdirSync } from "node:fs";
import { join } from "node:path";

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
export function extensionLayers(root: string): string[] {
  if (!existsSync(root)) return [];
  return readdirSync(root, { withFileTypes: true }).flatMap((entry) => {
    if (!entry.isDirectory() || entry.name === "node_modules") return [];
    const full = join(root, entry.name);
    return entry.name === "frontend" ? [full] : extensionLayers(full);
  });
}

// A unit's screen is shipped UI in the same bundle, so a gate stopping at
// frontend/src would hold the core to a standard the extension tier escapes.
export function extensionFrontendFiles(extensionsDir: string): string[] {
  return extensionLayers(extensionsDir).flatMap(filesUnder);
}
