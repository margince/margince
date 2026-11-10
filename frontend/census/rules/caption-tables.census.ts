// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A table of English sentences outside the catalogs reaches every reader in
// English, whatever language they chose. Its captions belong in en, de and vi.
//
// The corpus is what the shipped bundle loads. A story, a test or a module
// nothing imports holds no caption a reader sees.

import { join, relative } from "node:path";
import ts from "typescript";
import { type Module, reachedModulesIn } from "../../scripts/lib/bundle-reach";
import { parseSource } from "../../scripts/lib/source-tree";
import { LOCALES } from "../../src/i18n/locale";
import {
  type CensusFile,
  type CensusRule,
  type CensusTree,
  srcDir,
} from "../tree";

// A capital or a placeholder, then at least one lowercase word: "Loading
// pipeline…" and "{name} is typing" are sentences, "Microsoft Teams" is not.
const WORD = String.raw`[A-Za-z%{][\w%'’{}-]*[,;:]?`;
const LOWER = String.raw`[a-z]{3,}[\w'’-]*[,;:]?`;
const SENTENCE = new RegExp(
  String.raw`^(?:[A-Z][a-z]+[,;:]?|\{\w+\})(?: ${WORD})*(?: ${LOWER})(?: ${WORD})*[.…!?]?$`,
);
// One word is a caption when it says what is going on: "Thinking", "Failed".
// The shape also fits nouns ("String", "Seed"), so a one-word state counts
// only in a table that already holds a sentence.
// Blind spot: a table of one-word states alone.
const STATE_WORD = /^[A-Z][a-z]+(?:ing|ed)[.…]?$/;

function unwrapped(node: ts.Expression): ts.Expression {
  return ts.isAsExpression(node) || ts.isSatisfiesExpression(node)
    ? unwrapped(node.expression)
    : node;
}

/** The strings a table holds, at any depth, in source order. */
function stringsIn(table: ts.Node): string[] {
  const found: string[] = [];
  const collect = (node: ts.Node): void => {
    if (ts.isStringLiteralLike(node)) {
      found.push(node.text);
    }
    ts.forEachChild(node, collect);
  };
  collect(table);
  return found;
}

/** A table's captions: its sentences, and its one-word states beside them. */
function captionsOf(table: ts.Node): string[] {
  const strings = stringsIn(table);
  const captionTable = strings.some((text) => SENTENCE.test(text));
  return strings.filter(
    (text) => SENTENCE.test(text) || (captionTable && STATE_WORD.test(text)),
  );
}

/** The captions a module's top-level object and array tables hold. */
export function captionsIn(path: string, text: string): string[] {
  const found: string[] = [];
  for (const statement of parseSource(path, text).statements) {
    if (!ts.isVariableStatement(statement)) continue;
    for (const declaration of statement.declarationList.declarations) {
      if (declaration.initializer === undefined) continue;
      const table = unwrapped(declaration.initializer);
      if (
        ts.isObjectLiteralExpression(table) ||
        ts.isArrayLiteralExpression(table)
      ) {
        found.push(...captionsOf(table));
      }
    }
  }
  return found;
}

/** The modules of `src/`, as the bundle reach reads them. */
export const srcModules = (tree: CensusTree): readonly CensusFile[] =>
  tree.files.filter((file) => file.area === "src" && file.source);

/**
 * The modules the shipped bundle loads, read from the census tree. The entry
 * documents are HTML files, which the tree does not hold, so bundle-reach finds
 * them on disk.
 */
export function shippedModules(
  corpus: readonly CensusFile[],
  catalogs: ReadonlySet<string>,
): ReadonlyMap<string, Module> {
  return reachedModulesIn(
    new Map(corpus.map((file) => [file.path, file.text])),
    { catalogs },
  );
}

export const catalogFiles = (): Set<string> =>
  new Set([
    ...LOCALES.map((locale) => join(srcDir, "i18n", `${locale}.ts`)),
    join(srcDir, "i18n", "bootstrapcopy_gen.ts"),
  ]);

// The floors and the findings read one reach of the same corpus.
const reached = new WeakMap<readonly CensusFile[], string[]>();

const shipped = (corpus: readonly CensusFile[]): string[] => {
  const known = reached.get(corpus);
  if (known) return known;
  const catalogs = catalogFiles();
  const paths = [...shippedModules(corpus, catalogs).keys()].filter(
    (path) => !catalogs.has(path),
  );
  reached.set(corpus, paths);
  return paths;
};

export const rule: CensusRule = {
  name: "caption-tables",
  corpus: srcModules,
  // A census that read a smaller tree would pass by reading nothing.
  floors: (corpus) => {
    const read = shipped(corpus);
    return [
      ...(read.length > 1000
        ? []
        : [`the bundle reach loaded ${read.length} modules, not over 1000`]),
      ...(read.includes(join(srcDir, "app", "agentrail-copy.ts"))
        ? []
        : ["the bundle reach missed the rail's own tables"]),
    ];
  },
  across: (corpus, tree) =>
    shipped(corpus).flatMap((path) =>
      captionsIn(path, tree.at(path)?.text ?? "").map(
        (caption) => `${relative(srcDir, path)}: ${caption}`,
      ),
    ),
  advice:
    "A table of English captions outside the catalogs reaches every reader " +
    "in English. Move each caption into en, de and vi and render it with t().",
};
