// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Web Storage has one spelling, `lib/storage.ts`: a key stored anywhere else is
// missing from its registry, so a sign-out cannot forget it.

// Matched on the parsed name, so a comment never counts and an alias does. A
// store reached by a run-time key (`window[area]`) is invisible to any parser.

import { relative } from "node:path";
import ts from "typescript";
import { type CensusFile, type CensusRule, repoRoot } from "../tree";

export const OWNER = "frontend/src/lib/storage.ts";

const STORAGE_GLOBALS: ReadonlySet<string> = new Set([
  "localStorage",
  "sessionStorage",
]);

// A test or a story seeds storage as a fixture; neither ships.
export const FIXTURE = /\.(test|stories)\.[cm]?[jt]sx?$/;

// Parsing only the files that name a store costs a fiftieth of parsing all.
export function mayReachForStorage(text: string): boolean {
  return text.includes("localStorage") || text.includes("sessionStorage");
}

export function storageReachesIn(source: ts.SourceFile): number[] {
  const lines = new Set<number>();
  const visit = (node: ts.Node) => {
    if (
      (ts.isIdentifier(node) || ts.isStringLiteralLike(node)) &&
      STORAGE_GLOBALS.has(node.text)
    ) {
      lines.add(source.getLineAndCharacterOfPosition(node.getStart()).line + 1);
    }
    ts.forEachChild(node, visit);
  };
  ts.forEachChild(source, visit);
  return [...lines];
}

const fromRepo = (file: CensusFile) => relative(repoRoot, file.path);

const inCorpus = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  file.source !== undefined &&
  !FIXTURE.test(file.path);

export const rule: CensusRule = {
  name: "storage-spelling",
  corpus: (tree) => tree.files.filter(inCorpus),
  // A census that judged nothing certifies nothing.
  floors: (corpus) => {
    const owner = corpus.find((file) => fromRepo(file) === OWNER);
    return [
      ...(corpus.length > 500 ? [] : ["500 modules or fewer were read"]),
      ...(corpus.some((file) => file.area === "extensions")
        ? []
        : ["the census covered frontend/src but no extension frontend layer"]),
      ...(owner?.source && storageReachesIn(owner.source).length > 0
        ? []
        : [`${OWNER} does not reach for storage, so the census reads nothing`]),
    ];
  },
  visit: (file) => {
    const where = fromRepo(file);
    if (where === OWNER || !mayReachForStorage(file.text) || !file.source) {
      return [];
    }
    return storageReachesIn(file.source).map((line) => `${where}:${line}`);
  },
  advice:
    "Reach Web Storage only through src/lib/storage.ts, so the key joins its " +
    "registry and a sign-out clears it.",
};
