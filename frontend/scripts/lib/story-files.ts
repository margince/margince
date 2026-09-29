// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join } from "node:path";
import ts from "typescript";
import { filesMatching, parseSource } from "./source-tree.ts";
import { defaultExportObject, literalAt } from "./story-title.ts";

// The endings `.storybook/main.ts`'s `stories` globs match, or null for a glob
// this cannot turn into endings, so a census built on it fails closed.
export function storySuffixes(path: string, text: string): string[] | null {
  const stories = literalAt(defaultExportObject(parseSource(path, text)), [
    "stories",
  ]);
  if (stories === undefined || !ts.isArrayLiteralExpression(stories)) {
    return null;
  }
  const suffixes: string[] = [];
  for (const element of stories.elements) {
    if (!ts.isStringLiteralLike(element)) return null;
    const suffix = globSuffixes(element.text);
    if (suffix === null) return null;
    suffixes.push(...suffix);
  }
  return suffixes.length > 0 ? suffixes : null;
}

function globSuffixes(glob: string): string[] | null {
  const alternatives = /^\.\.\/src\/\*\*\/\*(\.[\w.]+)\.@\(([\w|]+)\)$/.exec(
    glob,
  );
  if (alternatives !== null) {
    return alternatives[2]
      .split("|")
      .map((extension) => `${alternatives[1]}.${extension}`);
  }
  const bare = /^\.\.\/src\/\*\*\/\*(\.[\w.]+)$/.exec(glob);
  return bare === null ? null : [bare[1]];
}

export type StoryCensus = { suffixes: string[] | null; files: string[] };

// Every file under `frontend/src` that Storybook loads, read off main.ts's globs
// rather than restated, so a new kind of story file joins every gate's corpus.
export function storyCensus(frontendRoot: string): StoryCensus {
  const main = join(frontendRoot, ".storybook", "main.ts");
  const suffixes = storySuffixes(main, readFileSync(main, "utf8"));
  if (suffixes === null) return { suffixes, files: [] };
  const escaped = suffixes.map((suffix) =>
    suffix.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"),
  );
  return {
    suffixes,
    files: filesMatching(
      join(frontendRoot, "src"),
      new RegExp(`(${escaped.join("|")})$`),
    ),
  };
}

// A docs page rather than a file of stories: it renders prose, not a component.
export function isDocsPage(path: string): boolean {
  return path.endsWith(".mdx");
}
