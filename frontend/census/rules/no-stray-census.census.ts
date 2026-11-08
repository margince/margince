// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A whole-tree census is a rule in census/rules/. A test file elsewhere that
// imports a tree walker parses the tree again in its own module graph.
//
// The import is matched in the syntax tree by the exported name, so an alias
// counts. A walk the file writes itself counts too. census/stray-census-waivers.txt lists the files not moved yet. It
// only shrinks: an entry that no longer imports a walker is a finding.

import { readFileSync } from "node:fs";
import { join, relative, sep } from "node:path";
import ts from "typescript";
import { resolveRelative } from "../../scripts/lib/source-tree";
import {
  type CensusFile,
  type CensusRule,
  frontendRoot,
  scriptsDir,
  srcDir,
} from "../tree";

const lib = (name: string) => join(scriptsDir, "lib", name);

const walker = (
  module: string,
  ...names: string[]
): [string, ReadonlySet<string>] => [module, new Set(names)];

// Each module that walks the tree, and the exports that do the walking.
export const WALKERS: ReadonlyMap<string, ReadonlySet<string>> = new Map([
  walker(
    lib("source-tree.ts"),
    "filesUnder",
    "filesMatching",
    "extensionFilesMatching",
    "extensionFrontendFiles",
  ),
  walker(join(srcDir, "testing", "screenfiles.ts"), "screenFiles"),
  walker(
    lib("conformance-corpus.ts"),
    "sourceFiles",
    "extensionFrontends",
    "files",
  ),
  walker(lib("css-rules.ts"), "appStylesheets"),
  walker(lib("dialoglayout.corpus.ts"), "markupFiles", "sheetTexts"),
  walker(lib("dialoglayout.ts"), "markupFiles", "sheetTexts"),
  walker(lib("import-reach.ts"), "productionModulesUnder"),
  walker(lib("bundle-reach.ts"), "reachedModules", "documentEntries"),
  walker(lib("locale-catalogs.ts"), "localeCatalogs"),
  walker(lib("rendered-inside.ts"), "renderedInside", "drawnElements"),
  walker(lib("story-files.ts"), "storyCensus"),
  walker(lib("essential-copy.ts"), "essentialKeys"),
  walker(
    join(scriptsDir, "ext-imports.ts"),
    "auditLayers",
    "layersThatReadNothing",
  ),
  walker(
    join(scriptsDir, "feature-boundaries.ts"),
    "readGraph",
    "shippedInTree",
  ),
  walker(join(srcDir, "format", "zone-by-purpose.testkit.ts"), "sourceFiles"),
]);

// Tests of a walker itself: each walks a temporary directory it plants, not
// the tree, so it costs no parse of the tree.
const WALKER_TESTS: ReadonlySet<string> = new Set([
  "scripts/lib/essential-copy.test.ts",
  "scripts/lib/import-reach.test.ts",
  "scripts/lib/source-tree.test.ts",
  "scripts/lib/story-files.test.ts",
]);

const TEST_FILE = /\.test\.(ts|tsx|mts|cts|js|jsx|mjs|cjs)$/;

const waiverFile = join(frontendRoot, "census", "stray-census-waivers.txt");

export function readWaivers(text: string): Set<string> {
  return new Set(
    text
      .split("\n")
      .map((line) => line.trim())
      .filter((line) => line !== "" && !line.startsWith("#")),
  );
}

const shown = (module: string) =>
  relative(frontendRoot, module).split(sep).join("/");

function importedWalkers(
  module: string,
  bindings: ts.NamedImportBindings,
): string[] {
  if (ts.isNamespaceImport(bindings)) {
    return [`imports all of ${shown(module)}`];
  }
  return bindings.elements
    .filter((element) => !element.isTypeOnly)
    .map((element) => (element.propertyName ?? element.name).text)
    .filter((name) => WALKERS.get(module)?.has(name))
    .map((name) => `imports ${name} from ${shown(module)}`);
}

/**
 * The walker imports in one test file, one line each. A namespace import or an
 * `import()` of a walker module reaches every walker in it, so it counts.
 */
export function strayImports(path: string, source: ts.SourceFile): string[] {
  const found: string[] = [];
  const walkerAt = (specifier: string) => {
    const module = resolveRelative(path, specifier);
    return module !== null && WALKERS.has(module) ? module : undefined;
  };
  const visit = (node: ts.Node) => {
    if (
      ts.isImportDeclaration(node) &&
      ts.isStringLiteral(node.moduleSpecifier) &&
      node.importClause?.phaseModifier !== ts.SyntaxKind.TypeKeyword
    ) {
      const module = walkerAt(node.moduleSpecifier.text);
      const bindings = node.importClause?.namedBindings;
      if (module && bindings) found.push(...importedWalkers(module, bindings));
    }
    if (
      ts.isCallExpression(node) &&
      node.expression.kind === ts.SyntaxKind.ImportKeyword &&
      node.arguments[0] &&
      ts.isStringLiteralLike(node.arguments[0])
    ) {
      const module = walkerAt(node.arguments[0].text);
      if (module) found.push(`imports all of ${shown(module)}`);
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

const calleeName = (call: ts.CallExpression): string | undefined =>
  ts.isIdentifier(call.expression)
    ? call.expression.text
    : ts.isPropertyAccessExpression(call.expression)
      ? call.expression.name.text
      : undefined;

function callsIn(node: ts.Node): string[] {
  const names: string[] = [];
  const visit = (child: ts.Node) => {
    if (ts.isCallExpression(child)) names.push(calleeName(child) ?? "");
    ts.forEachChild(child, visit);
  };
  ts.forEachChild(node, visit);
  return names;
}

function declaredFunction(node: ts.Node): string | undefined {
  if (ts.isFunctionDeclaration(node)) return node.name?.text;
  if (
    ts.isVariableDeclaration(node) &&
    ts.isIdentifier(node.name) &&
    node.initializer &&
    (ts.isArrowFunction(node.initializer) ||
      ts.isFunctionExpression(node.initializer))
  ) {
    return node.name.text;
  }
  return undefined;
}

// The local names `readdirSync` goes by in this file, aliases included.
function readdirNames(source: ts.SourceFile): Set<string> {
  const names = new Set(["readdirSync"]);
  for (const statement of source.statements) {
    const bindings = ts.isImportDeclaration(statement)
      ? statement.importClause?.namedBindings
      : undefined;
    if (!bindings || !ts.isNamedImports(bindings)) continue;
    for (const element of bindings.elements) {
      if ((element.propertyName ?? element.name).text === "readdirSync") {
        names.add(element.name.text);
      }
    }
  }
  return names;
}

const recurses = (argument: ts.Expression) =>
  ts.isObjectLiteralExpression(argument) &&
  argument.properties.some(
    (property) =>
      ts.isPropertyAssignment(property) &&
      property.name.getText() === "recursive" &&
      property.initializer.kind === ts.SyntaxKind.TrueKeyword,
  );

const isMetaGlob = (call: ts.CallExpression) =>
  ts.isPropertyAccessExpression(call.expression) &&
  call.expression.name.text === "glob" &&
  ts.isMetaProperty(call.expression.expression);

const globsDeep = (argument: ts.Expression): boolean =>
  ts.isStringLiteralLike(argument)
    ? argument.text.includes("**")
    : ts.isArrayLiteralExpression(argument) &&
      argument.elements.some((element) => globsDeep(element));

/**
 * A walk a test file writes for itself: a function that calls `readdirSync`
 * and itself, a `readdirSync` asked to recurse, or an `import.meta.glob` over
 * `**`.
 */
export function ownWalks(source: ts.SourceFile): string[] {
  const readdir = readdirNames(source);
  const found: string[] = [];
  const visit = (node: ts.Node) => {
    const name = declaredFunction(node);
    const calls = name ? callsIn(node) : [];
    if (name && calls.includes(name) && calls.some((c) => readdir.has(c))) {
      found.push(`walks the tree itself in ${name}()`);
    }
    if (ts.isCallExpression(node)) {
      if (
        readdir.has(calleeName(node) ?? "") &&
        node.arguments.some(recurses)
      ) {
        found.push("walks the tree itself with a recursive readdirSync");
      }
      if (isMetaGlob(node) && node.arguments.some(globsDeep)) {
        found.push("walks the tree itself with import.meta.glob over **");
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

// The waiver list's length. It may only fall: a move deletes a line and lowers
// this number in the same change.
export const WAIVED_FILES = 97;

export function waiverCount(
  waived: ReadonlySet<string>,
  pinned = WAIVED_FILES,
): string[] {
  if (waived.size > pinned) {
    return [
      `census/stray-census-waivers.txt lists ${waived.size} files and the pin is ${pinned}; the list only shrinks, so move the census instead`,
    ];
  }
  if (waived.size < pinned) {
    return [
      `census/stray-census-waivers.txt lists ${waived.size} files; lower WAIVED_FILES in no-stray-census.census.ts to ${waived.size}`,
    ];
  }
  return [];
}

const strays = (file: CensusFile) =>
  file.source
    ? [...strayImports(file.path, file.source), ...ownWalks(file.source)]
    : [];

export const rule: CensusRule = {
  name: "no-stray-census",
  corpus: (tree) =>
    tree.files.filter(
      (file) => TEST_FILE.test(file.path) && !WALKER_TESTS.has(file.rel),
    ),
  floors: (corpus, tree) => [
    ...(corpus.length > 500 ? [] : ["fewer than 500 test files were read"]),
    ...[...WALKER_TESTS]
      .filter((rel) => !tree.at(join(frontendRoot, rel)))
      .map((rel) => `${rel} is listed as a walker's own test and is gone`),
  ],
  across: (corpus) => {
    const waived = readWaivers(readFileSync(waiverFile, "utf8"));
    const byRel = new Map(corpus.map((file) => [file.rel, file]));
    const stale = [...waived]
      .filter((rel) => {
        const file = byRel.get(rel);
        return file === undefined || strays(file).length === 0;
      })
      .map(
        (rel) =>
          `${rel} is waived in census/stray-census-waivers.txt and walks no tree; delete its line`,
      );
    const found = corpus
      .filter((file) => !waived.has(file.rel))
      .flatMap((file) => strays(file).map((what) => `${file.rel}: ${what}`));
    return [...waiverCount(waived), ...found, ...stale];
  },
  advice:
    "A test file outside census/ walks the tree. Move the census into a rule in " +
    "census/rules/<name>.census.ts and keep its planted cases beside it in " +
    "census/rules/<name>.test.ts; census/census.test.ts runs it over the one parsed tree.",
};
