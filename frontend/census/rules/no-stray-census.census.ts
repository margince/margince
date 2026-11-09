// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A whole-tree census is a rule in census/rules/. A test file elsewhere that
// imports a tree walker parses the tree again in its own module graph.
//
// The import is matched in the syntax tree by the exported name, so an alias
// counts. A walk the file writes itself counts too, and so does a walk a
// helper module does for it. The rule has no waiver list.

import { join, relative, sep } from "node:path";
import ts from "typescript";
import { isModuleOnDisk, resolveRelative } from "../../scripts/lib/source-tree";
import {
  type CensusFile,
  type CensusRule,
  type CensusTree,
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
]);

// Tests of a walker itself: each walks a temporary directory it plants, not
// the tree, so it costs no parse of the tree.
const WALKER_TESTS: ReadonlySet<string> = new Set([
  "scripts/ext-imports.test.ts",
  "scripts/feature-boundaries.test.ts",
  "scripts/lib/essential-copy.test.ts",
  "scripts/lib/import-reach.test.ts",
  "scripts/lib/source-tree.test.ts",
  "scripts/lib/story-files.test.ts",
]);

const TEST_FILE = /\.test\.(ts|tsx|mts|cts|js|jsx|mjs|cjs)$/;

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
      node.arguments[0]
    ) {
      const specifier = node.arguments[0];
      if (!ts.isStringLiteralLike(specifier)) {
        // A computed specifier can name any module, a walker included, and
        // nothing short of running the test says which.
        found.push(
          `imports a module whose name is computed: import(${specifier.getText(source)})`,
        );
      } else {
        const module = walkerAt(specifier.text);
        if (module) found.push(`imports all of ${shown(module)}`);
      }
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

const walks = (file: CensusFile) =>
  file.source
    ? [...strayImports(file.path, file.source), ...ownWalks(file.source)]
    : [];

type Imported = Readonly<{
  path: string;
  /** The imported file as the tree holds it, if it does. */
  module: CensusFile | undefined;
  /** Each imported name and the local name it binds; `*` for a namespace. */
  names: Map<string, string>;
}>;

// Each module's imports, read once a run: helpers are shared by many tests.
const importsRead = new WeakMap<CensusFile, Imported[]>();

/** The non-type imports of one module by a relative specifier. */
function localImports(file: CensusFile, tree: CensusTree): Imported[] {
  const known = importsRead.get(file);
  if (known) return known;
  const found = readImports(file, tree);
  importsRead.set(file, found);
  return found;
}

function readImports(file: CensusFile, tree: CensusTree): Imported[] {
  if (!file.source) return [];
  const out: Imported[] = [];
  for (const statement of file.source.statements) {
    if (
      !ts.isImportDeclaration(statement) ||
      !ts.isStringLiteral(statement.moduleSpecifier) ||
      statement.importClause?.phaseModifier === ts.SyntaxKind.TypeKeyword
    ) {
      continue;
    }
    const path = resolveRelative(
      file.path,
      statement.moduleSpecifier.text,
      (candidate) =>
        tree.at(candidate) !== undefined || isModuleOnDisk(candidate),
    );
    if (path === null) continue;
    const names = new Map<string, string>();
    const bindings = statement.importClause?.namedBindings;
    if (bindings && ts.isNamedImports(bindings)) {
      for (const element of bindings.elements) {
        if (element.isTypeOnly) continue;
        names.set(
          (element.propertyName ?? element.name).text,
          element.name.text,
        );
      }
    }
    if (bindings && ts.isNamespaceImport(bindings)) {
      names.set("*", bindings.name.text);
    }
    out.push({ path, module: tree.at(path), names });
  }
  return out;
}

/** The local names a module binds to walking exports of a walker module. */
function walkerNames(file: CensusFile, tree: CensusTree): Set<string> {
  const names = new Set<string>();
  for (const { path, names: bound } of localImports(file, tree)) {
    const walking = WALKERS.get(path);
    if (!walking) continue;
    for (const [imported, local] of bound) {
      if (imported === "*" || walking.has(imported)) names.add(local);
    }
  }
  return names;
}

const enclosingFunction = (node: ts.Node): ts.Node | undefined =>
  ts.findAncestor(node.parent, ts.isFunctionLike);

/** The calls in a module to the names in `walkers`. Each comes with the
 * function it sits in, or none for a call made when the module loads. */
function walkerCalls(
  source: ts.SourceFile,
  walkers: ReadonlySet<string>,
): { call: ts.CallExpression; inside: ts.Node | undefined }[] {
  const out: { call: ts.CallExpression; inside: ts.Node | undefined }[] = [];
  const visit = (node: ts.Node) => {
    if (ts.isCallExpression(node)) {
      const callee = ts.isPropertyAccessExpression(node.expression)
        ? node.expression.expression
        : node.expression;
      if (ts.isIdentifier(callee) && walkers.has(callee.text)) {
        out.push({ call: node, inside: enclosingFunction(node) });
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return out;
}

const exported = (node: ts.Node) =>
  (ts.getCombinedModifierFlags(node as ts.Declaration) &
    ts.ModifierFlags.Export) !==
  0;

/** The name a module exports a function under, if it does: an exported
 * declaration, or an exported const that holds the function. */
function exportedName(fn: ts.Node): string | undefined {
  if (ts.isFunctionDeclaration(fn) && fn.name && exported(fn)) {
    return fn.name.text;
  }
  const holder = fn.parent;
  return ts.isVariableDeclaration(holder) &&
    ts.isIdentifier(holder.name) &&
    exported(holder)
    ? holder.name.text
    : undefined;
}

/** The exported functions of a module that call a walker in their body. */
function walkingExports(calls: { inside: ts.Node | undefined }[]): Set<string> {
  const names = new Set<string>();
  for (const { inside } of calls) {
    for (let at = inside; at; at = enclosingFunction(at)) {
      const name = exportedName(at);
      if (name) names.add(name);
    }
  }
  return names;
}

// Each helper's walking exports, read once a run.
const walkingRead = new WeakMap<CensusFile, Set<string>>();

function exportsThatWalk(module: CensusFile, tree: CensusTree): Set<string> {
  const known = walkingRead.get(module);
  if (known) return known;
  const walkers = walkerNames(module, tree);
  const found =
    walkers.size === 0 || !module.source
      ? new Set<string>()
      : walkingExports(walkerCalls(module.source, walkers));
  walkingRead.set(module, found);
  return found;
}

/**
 * A non-test module a test imports that walks the tree for it. It walks when
 * it loads, by a call to a walker outside any function or by importing a
 * module that does. It also walks when the test imports one of its functions
 * that calls a walker. A walker module itself counts only for the walking names a test
 * imports, which `walks` reads.
 */
function helperWalks(
  file: CensusFile,
  tree: CensusTree,
  known: Map<string, string | null>,
): string[] {
  const onLoad = (helper: CensusFile): string | null => {
    const cached = known.get(helper.path);
    if (cached !== undefined) return cached;
    known.set(helper.path, null);
    let found: string | null = null;
    if (helper.source) {
      const loading = walkerCalls(
        helper.source,
        walkerNames(helper, tree),
      ).find(({ inside }) => inside === undefined);
      if (loading) {
        found = `${helper.rel} calls ${loading.call.expression.getText(helper.source)} when it loads`;
      }
    }
    for (const { path, module } of localImports(helper, tree)) {
      if (found) break;
      if (!module || WALKERS.has(path) || TEST_FILE.test(path)) continue;
      const deeper = onLoad(module);
      if (deeper) found = `${helper.rel} imports ${deeper}`;
    }
    known.set(helper.path, found);
    return found;
  };
  return localImports(file, tree).flatMap(({ path, module, names }) => {
    if (!module || WALKERS.has(path) || TEST_FILE.test(path)) return [];
    const loading = onLoad(module);
    if (loading) return [`walks the tree through ${loading}`];
    if (!module.source) return [];
    const walking = exportsThatWalk(module, tree);
    return [...names.keys()]
      .filter((name) => walking.has(name))
      .map((name) => `walks the tree through ${name}() in ${module.rel}`);
  });
}

const strays = (
  file: CensusFile,
  tree: CensusTree,
  known: Map<string, string | null>,
) => [...walks(file), ...helperWalks(file, tree, known)];

export const rule: CensusRule = {
  name: "no-stray-census",
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        TEST_FILE.test(file.path) &&
        !WALKER_TESTS.has(file.rel) &&
        !file.rel.startsWith("census/"),
    ),
  floors: (corpus, tree) => [
    ...(corpus.length > 500 ? [] : ["fewer than 500 test files were read"]),
    ...[...WALKER_TESTS]
      .filter((rel) => !tree.at(join(frontendRoot, rel)))
      .map((rel) => `${rel} is listed as a walker's own test and is gone`),
  ],
  across: (corpus, tree) => {
    const known = new Map<string, string | null>();
    return corpus.flatMap((file) =>
      strays(file, tree, known).map((what) => `${file.rel}: ${what}`),
    );
  },
  advice:
    "A test file outside census/ walks the tree. Move the census into a rule in " +
    "census/rules/<name>.census.ts and keep its planted cases beside it in " +
    "census/rules/<name>.test.ts; census/census.test.ts runs it over the one parsed tree.",
};
