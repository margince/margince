// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The screen files the guards read: every `.tsx` under the screen folders, less
// tests, testkits and stories. A census that lost a folder would read a smaller
// tree and pass every guard, so the floors hold it to the tree it knows.

import { existsSync } from "node:fs";
import { join } from "node:path";
import { SCREEN_ROOTS } from "../../src/testing/screenfiles";
import {
  type CensusFile,
  type CensusRule,
  type CensusTree,
  srcDir,
} from "../tree";

const NOT_A_SCREEN = /\.(test|stories|testkit)\.tsx$/;

/** The tree's screen files, as `src/testing/screenfiles.ts` defines them. */
export function screenCorpus(tree: CensusTree): readonly CensusFile[] {
  return tree.files.filter(
    (file) =>
      file.area === "src" &&
      SCREEN_ROOTS.some((root) => file.rel.startsWith(`src/${root}/`)) &&
      file.path.endsWith(".tsx") &&
      !NOT_A_SCREEN.test(file.path),
  );
}

// A root that is gone reads as a smaller tree. A root may hold no screen yet,
// as a fork's custom/ folder does before its first screen.
export const missingRoots = (roots: readonly string[]): string[] =>
  roots
    .filter((root) => !existsSync(join(srcDir, root)))
    .map((root) => `src/${root} does not exist`);

export const rule: CensusRule = {
  name: "screenfiles",
  corpus: screenCorpus,
  floors: (corpus) => [
    ...(corpus.length > 500 ? [] : ["500 screen files or fewer were read"]),
    ...(corpus.some((file) => file.rel.endsWith("/contacts.tsx"))
      ? []
      : ["no contacts.tsx was read"]),
    ...(corpus.some((file) => file.rel.includes("/deal360/"))
      ? []
      : ["no file under deal360/ was read"]),
  ],
  across: () => missingRoots(SCREEN_ROOTS),
  advice:
    "A screen folder in SCREEN_ROOTS is gone. Update SCREEN_ROOTS " +
    "in src/testing/screenfiles.ts to the folders screen code lives in.",
};
